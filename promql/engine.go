// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/gate"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/stats"
)

const (
	namespace = "prometheus"
	subsystem = "engine"
	queryTag  = "query"
	env       = "query execution"

	// The largest SampleValue that can be converted to an int64 without overflow.
	maxInt64 = 9223372036854774784
	// The smallest SampleValue that can be converted to an int64 without underflow.
	minInt64 = -9223372036854775808
)

var (
	// LookbackDelta determines the time since the last sample after which a time
	// series is considered stale.
	LookbackDelta = 5 * time.Minute

	// DefaultEvaluationInterval is the default evaluation interval of
	// a subquery in milliseconds.
	DefaultEvaluationInterval int64
)

// SetDefaultEvaluationInterval sets DefaultEvaluationInterval.
func SetDefaultEvaluationInterval(ev time.Duration) {
	atomic.StoreInt64(&DefaultEvaluationInterval, durationToInt64Millis(ev))
}

// GetDefaultEvaluationInterval returns the DefaultEvaluationInterval as time.Duration.
func GetDefaultEvaluationInterval() int64 {
	return atomic.LoadInt64(&DefaultEvaluationInterval)
}

type engineMetrics struct {
	currentQueries       prometheus.Gauge
	maxConcurrentQueries prometheus.Gauge
	queryQueueTime       prometheus.Summary
	queryPrepareTime     prometheus.Summary
	queryInnerEval       prometheus.Summary
	queryResultSort      prometheus.Summary
}

// convertibleToInt64 returns true if v does not over-/underflow an int64.
func convertibleToInt64(v float64) bool {
	return v <= maxInt64 && v >= minInt64
}

type (
	// ErrQueryTimeout is returned if a query timed out during processing.
	ErrQueryTimeout string
	// ErrQueryCanceled is returned if a query was canceled during processing.
	ErrQueryCanceled string
	// ErrTooManySamples is returned if a query would load more than the maximum allowed samples into memory.
	ErrTooManySamples string
	// ErrStorage is returned if an error was encountered in the storage layer
	// during query handling.
	ErrStorage struct{ Err error }
)

func (e ErrQueryTimeout) Error() string {
	return fmt.Sprintf("query timed out in %s", string(e))
}
func (e ErrQueryCanceled) Error() string {
	return fmt.Sprintf("query was canceled in %s", string(e))
}
func (e ErrTooManySamples) Error() string {
	return fmt.Sprintf("query processing would load too many samples into memory in %s", string(e))
}
func (e ErrStorage) Error() string {
	return e.Err.Error()
}

// A Query is derived from an a raw query string and can be run against an engine
// it is associated with.
type Query interface {
	// Exec processes the query. Can only be called once.
	Exec(ctx context.Context) *Result
	// Close recovers memory used by the query result.
	Close()
	// Statement returns the parsed statement of the query.
	Statement() Statement
	// Stats returns statistics about the lifetime of the query.
	Stats() *stats.QueryTimers
	// Cancel signals that a running query execution should be aborted.
	Cancel()
}

// query implements the Query interface.
type query struct {
	// Underlying data provider.
	queryable storage.Queryable
	// The original query string.
	q string
	// Statement of the parsed query.
	stmt Statement
	// Timer stats for the query execution.
	stats *stats.QueryTimers
	// Result matrix for reuse.
	matrix Matrix
	// Cancellation function for the query.
	cancel func()

	// The engine against which the query is executed.
	ng *Engine
}

// Statement implements the Query interface.
func (q *query) Statement() Statement {
	return q.stmt
}

// Stats implements the Query interface.
func (q *query) Stats() *stats.QueryTimers {
	return q.stats
}

// Cancel implements the Query interface.
func (q *query) Cancel() {
	if q.cancel != nil {
		q.cancel()
	}
}

// Close implements the Query interface.
func (q *query) Close() {
	for _, s := range q.matrix {
		putPointSlice(s.Points)
	}
}

// Exec implements the Query interface.
func (q *query) Exec(ctx context.Context) *Result {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		span.SetTag(queryTag, q.stmt.String())
	}

	// Log query in active log.
	var queryIndex int
	if q.ng.activeQueryTracker != nil {
		queryIndex = q.ng.activeQueryTracker.Insert(q.q)
	}

	// Exec query.
	res, warnings, err := q.ng.exec(ctx, q)

	// Delete query from active log.
	if q.ng.activeQueryTracker != nil {
		q.ng.activeQueryTracker.Delete(queryIndex)
	}
	return &Result{Err: err, Value: res, Warnings: warnings}
}

// contextDone returns an error if the context was canceled or timed out.
func contextDone(ctx context.Context, env string) error {
	if err := ctx.Err(); err != nil {
		return contextErr(err, env)
	}
	return nil
}

func contextErr(err error, env string) error {
	switch err {
	case context.Canceled:
		return ErrQueryCanceled(env)
	case context.DeadlineExceeded:
		return ErrQueryTimeout(env)
	default:
		return err
	}
}

// EngineOpts contains configuration options used when creating a new Engine.
type EngineOpts struct {
	Logger             log.Logger
	Reg                prometheus.Registerer
	MaxConcurrent      int
	MaxSamples         int
	Timeout            time.Duration
	ActiveQueryTracker *ActiveQueryTracker
}

// Engine handles the lifetime of queries from beginning to end.
// It is connected to a querier.
type Engine struct {
	logger             log.Logger
	metrics            *engineMetrics
	timeout            time.Duration
	gate               *gate.Gate
	maxSamplesPerQuery int
	activeQueryTracker *ActiveQueryTracker
}

// NewEngine returns a new engine.
func NewEngine(opts EngineOpts) *Engine {
	if opts.Logger == nil {
		opts.Logger = log.NewNopLogger()
	}

	metrics := &engineMetrics{
		currentQueries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queries",
			Help:      "The current number of queries being executed or waiting.",
		}),
		maxConcurrentQueries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queries_concurrent_max",
			Help:      "The max number of concurrent queries.",
		}),
		queryQueueTime: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "queue_time"},
			Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		queryPrepareTime: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "prepare_time"},
			Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		queryInnerEval: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "inner_eval"},
			Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		queryResultSort: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "result_sort"},
			Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
	}
	metrics.maxConcurrentQueries.Set(float64(opts.MaxConcurrent))

	if opts.Reg != nil {
		opts.Reg.MustRegister(
			metrics.currentQueries,
			metrics.maxConcurrentQueries,
			metrics.queryQueueTime,
			metrics.queryPrepareTime,
			metrics.queryInnerEval,
			metrics.queryResultSort,
		)
	}

	return &Engine{
		gate:               gate.New(opts.MaxConcurrent),
		timeout:            opts.Timeout,
		logger:             opts.Logger,
		metrics:            metrics,
		maxSamplesPerQuery: opts.MaxSamples,
		activeQueryTracker: opts.ActiveQueryTracker,
	}
}

// NewInstantQuery returns an evaluation query for the given expression at the given time.
func (ng *Engine) NewInstantQuery(q storage.Queryable, qs string, ts time.Time) (Query, error) {
	expr, err := ParseExpr(qs)
	if err != nil {
		return nil, err
	}
	qry := ng.newQuery(q, expr, ts, ts, 0)
	qry.q = qs

	return qry, nil
}

// NewRangeQuery returns an evaluation query for the given time range and with
// the resolution set by the interval.
func (ng *Engine) NewRangeQuery(q storage.Queryable, qs string, start, end time.Time, interval time.Duration) (Query, error) {
	expr, err := ParseExpr(qs)
	if err != nil {
		return nil, err
	}
	if expr.Type() != ValueTypeVector && expr.Type() != ValueTypeScalar {
		return nil, errors.Errorf("invalid expression type %q for range query, must be Scalar or instant Vector", documentedType(expr.Type()))
	}
	qry := ng.newQuery(q, expr, start, end, interval)
	qry.q = qs

	return qry, nil
}

func (ng *Engine) newQuery(q storage.Queryable, expr Expr, start, end time.Time, interval time.Duration) *query {
	es := &EvalStmt{
		Expr:     expr,
		Start:    start,
		End:      end,
		Interval: interval,
	}
	qry := &query{
		stmt:      es,
		ng:        ng,
		stats:     stats.NewQueryTimers(),
		queryable: q,
	}
	return qry
}

// testStmt is an internal helper statement that allows execution
// of an arbitrary function during handling. It is used to test the Engine.
type testStmt func(context.Context) error

func (testStmt) String() string { return "test statement" }
func (testStmt) stmt()          {}

func (ng *Engine) newTestQuery(f func(context.Context) error) Query {
	qry := &query{
		q:     "test statement",
		stmt:  testStmt(f),
		ng:    ng,
		stats: stats.NewQueryTimers(),
	}
	return qry
}

// exec executes the query.
//
// At this point per query only one EvalStmt is evaluated. Alert and record
// statements are not handled by the Engine.
func (ng *Engine) exec(ctx context.Context, q *query) (Value, storage.Warnings, error) {
	ng.metrics.currentQueries.Inc()
	defer ng.metrics.currentQueries.Dec()

	ctx, cancel := context.WithTimeout(ctx, ng.timeout)
	q.cancel = cancel

	execSpanTimer, ctx := q.stats.GetSpanTimer(ctx, stats.ExecTotalTime)
	defer execSpanTimer.Finish()

	queueSpanTimer, _ := q.stats.GetSpanTimer(ctx, stats.ExecQueueTime, ng.metrics.queryQueueTime)

	if err := ng.gate.Start(ctx); err != nil {
		return nil, nil, contextErr(err, "query queue")
	}
	defer ng.gate.Done()

	queueSpanTimer.Finish()

	// Cancel when execution is done or an error was raised.
	defer q.cancel()

	const env = "query execution"

	evalSpanTimer, ctx := q.stats.GetSpanTimer(ctx, stats.EvalTotalTime)
	defer evalSpanTimer.Finish()

	// The base context might already be canceled on the first iteration (e.g. during shutdown).
	if err := contextDone(ctx, env); err != nil {
		return nil, nil, err
	}

	switch s := q.Statement().(type) {
	case *EvalStmt:
		return ng.execEvalStmt(ctx, q, s)
	case testStmt:
		return nil, nil, s(ctx)
	}

	panic(errors.Errorf("promql.Engine.exec: unhandled statement of type %T", q.Statement()))
}

func timeMilliseconds(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond/time.Nanosecond)
}

func durationMilliseconds(d time.Duration) int64 {
	return int64(d / (time.Millisecond / time.Nanosecond))
}

// execEvalStmt evaluates the expression of an evaluation statement for the given time range.
func (ng *Engine) execEvalStmt(ctx context.Context, query *query, s *EvalStmt) (Value, storage.Warnings, error) {
	prepareSpanTimer, ctxPrepare := query.stats.GetSpanTimer(ctx, stats.QueryPreparationTime, ng.metrics.queryPrepareTime)
	querier, warnings, err := ng.populateSeries(ctxPrepare, query.queryable, s)
	prepareSpanTimer.Finish()

	// XXX(fabxc): the querier returned by populateSeries might be instantiated
	// we must not return without closing irrespective of the error.
	// TODO: make this semantically saner.
	if querier != nil {
		defer querier.Close()
	}

	if err != nil {
		return nil, warnings, err
	}

	evalSpanTimer, ctxInnerEval := query.stats.GetSpanTimer(ctx, stats.InnerEvalTime, ng.metrics.queryInnerEval)
	// Instant evaluation. This is executed as a range evaluation with one step.
	if s.Start == s.End && s.Interval == 0 {
		start := timeMilliseconds(s.Start)
		evaluator := &evaluator{
			startTimestamp:      start,
			endTimestamp:        start,
			interval:            1,
			ctx:                 ctxInnerEval,
			maxSamples:          ng.maxSamplesPerQuery,
			defaultEvalInterval: GetDefaultEvaluationInterval(),
			logger:              ng.logger,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, warnings, err
		}

		evalSpanTimer.Finish()

		mat, ok := val.(Matrix)
		if !ok {
			panic(errors.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
		}
		query.matrix = mat
		switch s.Expr.Type() {
		case ValueTypeVector:
			// Convert matrix with one value per series into vector.
			vector := make(Vector, len(mat))
			for i, s := range mat {
				// Point might have a different timestamp, force it to the evaluation
				// timestamp as that is when we ran the evaluation.
				vector[i] = Sample{Metric: s.Metric, Point: Point{V: s.Points[0].V, T: start}}
			}
			return vector, warnings, nil
		case ValueTypeScalar:
			return Scalar{V: mat[0].Points[0].V, T: start}, warnings, nil
		case ValueTypeMatrix:
			return mat, warnings, nil
		default:
			panic(errors.Errorf("promql.Engine.exec: unexpected expression type %q", s.Expr.Type()))
		}

	}

	// Range evaluation.
	evaluator := &evaluator{
		startTimestamp:      timeMilliseconds(s.Start),
		endTimestamp:        timeMilliseconds(s.End),
		interval:            durationMilliseconds(s.Interval),
		ctx:                 ctxInnerEval,
		maxSamples:          ng.maxSamplesPerQuery,
		defaultEvalInterval: GetDefaultEvaluationInterval(),
		logger:              ng.logger,
	}
	val, err := evaluator.Eval(s.Expr)
	if err != nil {
		return nil, warnings, err
	}
	evalSpanTimer.Finish()

	mat, ok := val.(Matrix)
	if !ok {
		panic(errors.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
	}
	query.matrix = mat

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, warnings, err
	}

	// TODO(fabxc): order ensured by storage?
	// TODO(fabxc): where to ensure metric labels are a copy from the storage internals.
	sortSpanTimer, _ := query.stats.GetSpanTimer(ctx, stats.ResultSortTime, ng.metrics.queryResultSort)
	sort.Sort(mat)
	sortSpanTimer.Finish()

	return mat, warnings, nil
}

// cumulativeSubqueryOffset returns the sum of range and offset of all subqueries in the path.
func (ng *Engine) cumulativeSubqueryOffset(path []Node) time.Duration {
	var subqOffset time.Duration
	for _, node := range path {
		switch n := node.(type) {
		case *SubqueryExpr:
			subqOffset += n.Range + n.Offset
		}
	}
	return subqOffset
}

func (ng *Engine) populateSeries(ctx context.Context, q storage.Queryable, s *EvalStmt) (storage.Querier, storage.Warnings, error) {
	var maxOffset time.Duration
	Inspect(s.Expr, func(node Node, path []Node) error {
		subqOffset := ng.cumulativeSubqueryOffset(path)
		switch n := node.(type) {
		case *VectorSelector:
			if maxOffset < LookbackDelta+subqOffset {
				maxOffset = LookbackDelta + subqOffset
			}
			if n.Offset+LookbackDelta+subqOffset > maxOffset {
				maxOffset = n.Offset + LookbackDelta + subqOffset
			}
		case *MatrixSelector:
			if maxOffset < n.Range+subqOffset {
				maxOffset = n.Range + subqOffset
			}
			if n.Offset+n.Range+subqOffset > maxOffset {
				maxOffset = n.Offset + n.Range + subqOffset
			}
		}
		return nil
	})

	mint := s.Start.Add(-maxOffset)

	querier, err := q.Querier(ctx, timestamp.FromTime(mint), timestamp.FromTime(s.End))
	if err != nil {
		return nil, nil, err
	}

	var warnings storage.Warnings

	Inspect(s.Expr, func(node Node, path []Node) error {
		var set storage.SeriesSet
		var wrn storage.Warnings
		params := &storage.SelectParams{
			Start: timestamp.FromTime(s.Start),
			End:   timestamp.FromTime(s.End),
			Step:  durationToInt64Millis(s.Interval),
		}

		// We need to make sure we select the timerange selected by the subquery.
		// TODO(gouthamve): cumulativeSubqueryOffset gives the sum of range and the offset
		// we can optimise it by separating out the range and offsets, and subtracting the offsets
		// from end also.
		subqOffset := ng.cumulativeSubqueryOffset(path)
		offsetMilliseconds := durationMilliseconds(subqOffset)
		params.Start = params.Start - offsetMilliseconds

		switch n := node.(type) {
		case *VectorSelector:
			params.Start = params.Start - durationMilliseconds(LookbackDelta)
			params.Func = extractFuncFromPath(path)
			if n.Offset > 0 {
				offsetMilliseconds := durationMilliseconds(n.Offset)
				params.Start = params.Start - offsetMilliseconds
				params.End = params.End - offsetMilliseconds
			}

			set, wrn, err = querier.Select(params, n.LabelMatchers...)
			warnings = append(warnings, wrn...)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error selecting series set", "err", err)
				return err
			}
			n.unexpandedSeriesSet = set

		case *MatrixSelector:
			params.Func = extractFuncFromPath(path)
			// For all matrix queries we want to ensure that we have (end-start) + range selected
			// this way we have `range` data before the start time
			params.Start = params.Start - durationMilliseconds(n.Range)
			if n.Offset > 0 {
				offsetMilliseconds := durationMilliseconds(n.Offset)
				params.Start = params.Start - offsetMilliseconds
				params.End = params.End - offsetMilliseconds
			}

			set, wrn, err = querier.Select(params, n.LabelMatchers...)
			warnings = append(warnings, wrn...)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error selecting series set", "err", err)
				return err
			}
			n.unexpandedSeriesSet = set
		}
		return nil
	})
	return querier, warnings, err
}

// extractFuncFromPath walks up the path and searches for the first instance of
// a function or aggregation.
func extractFuncFromPath(p []Node) string {
	if len(p) == 0 {
		return ""
	}
	switch n := p[len(p)-1].(type) {
	case *AggregateExpr:
		return n.Op.String()
	case *Call:
		return n.Func.Name
	case *BinaryExpr:
		// If we hit a binary expression we terminate since we only care about functions
		// or aggregations over a single metric.
		return ""
	}
	return extractFuncFromPath(p[:len(p)-1])
}

func checkForSeriesSetExpansion(ctx context.Context, expr Expr) {
	switch e := expr.(type) {
	case *MatrixSelector:
		if e.series == nil {
			series, err := expandSeriesSet(ctx, e.unexpandedSeriesSet)
			if err != nil {
				panic(err)
			} else {
				e.series = series
			}
		}
	case *VectorSelector:
		if e.series == nil {
			series, err := expandSeriesSet(ctx, e.unexpandedSeriesSet)
			if err != nil {
				panic(err)
			} else {
				e.series = series
			}
		}
	}
}

func expandSeriesSet(ctx context.Context, it storage.SeriesSet) (res []storage.Series, err error) {
	for it.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		res = append(res, it.At())
	}
	return res, it.Err()
}

// An evaluator evaluates given expressions over given fixed timestamps. It
// is attached to an engine through which it connects to a querier and reports
// errors. On timeout or cancellation of its context it terminates.
type evaluator struct {
	ctx context.Context

	startTimestamp int64 // Start time in milliseconds.
	endTimestamp   int64 // End time in milliseconds.
	interval       int64 // Interval in milliseconds.

	maxSamples          int
	currentSamples      int
	defaultEvalInterval int64
	logger              log.Logger
}

// errorf causes a panic with the input formatted into an error.
func (ev *evaluator) errorf(format string, args ...interface{}) {
	ev.error(errors.Errorf(format, args...))
}

// error causes a panic with the given error.
func (ev *evaluator) error(err error) {
	panic(err)
}

// recover is the handler that turns panics into returns from the top level of evaluation.
func (ev *evaluator) recover(errp *error) {
	e := recover()
	if e == nil {
		return
	}
	if err, ok := e.(runtime.Error); ok {
		// Print the stack trace but do not inhibit the running application.
		buf := make([]byte, 64<<10)
		buf = buf[:runtime.Stack(buf, false)]

		level.Error(ev.logger).Log("msg", "runtime panic in parser", "err", e, "stacktrace", string(buf))
		*errp = errors.Wrap(err, "unexpected error")
	} else {
		*errp = e.(error)
	}
}

func (ev *evaluator) Eval(expr Expr) (v Value, err error) {
	defer ev.recover(&err)
	return ev.eval(expr), nil
}

// EvalNodeHelper stores extra information and caches for evaluating a single node across steps.
type EvalNodeHelper struct {
	// Evaluation timestamp.
	ts int64
	// Vector that can be used for output.
	out Vector

	// Caches.
	// dropMetricName and label_*.
	dmn map[uint64]labels.Labels
	// signatureFunc.
	sigf map[uint64]uint64
	// funcHistogramQuantile.
	signatureToMetricWithBuckets map[uint64]*metricWithBuckets
	// label_replace.
	regex *regexp.Regexp

	// For binary vector matching.
	rightSigs    map[uint64]Sample
	matchedSigs  map[uint64]map[uint64]struct{}
	resultMetric map[uint64]labels.Labels
}

// dropMetricName is a cached version of dropMetricName.
func (enh *EvalNodeHelper) dropMetricName(l labels.Labels) labels.Labels {
	if enh.dmn == nil {
		enh.dmn = make(map[uint64]labels.Labels, len(enh.out))
	}
	h := l.Hash()
	ret, ok := enh.dmn[h]
	if ok {
		return ret
	}
	ret = dropMetricName(l)
	enh.dmn[h] = ret
	return ret
}

// signatureFunc is a cached version of signatureFunc.
func (enh *EvalNodeHelper) signatureFunc(on bool, names ...string) func(labels.Labels) uint64 {
	if enh.sigf == nil {
		enh.sigf = make(map[uint64]uint64, len(enh.out))
	}
	f := signatureFunc(on, names...)
	return func(l labels.Labels) uint64 {
		h := l.Hash()
		ret, ok := enh.sigf[h]
		if ok {
			return ret
		}
		ret = f(l)
		enh.sigf[h] = ret
		return ret
	}
}

// rangeEval evaluates the given expressions, and then for each step calls
// the given function with the values computed for each expression at that
// step.  The return value is the combination into time series of all the
// function call results.
func (ev *evaluator) rangeEval(f func([]Value, *EvalNodeHelper) Vector, exprs ...Expr) Matrix {
	numSteps := int((ev.endTimestamp-ev.startTimestamp)/ev.interval) + 1
	matrixes := make([]Matrix, len(exprs))
	origMatrixes := make([]Matrix, len(exprs))
	originalNumSamples := ev.currentSamples

	for i, e := range exprs {
		// Functions will take string arguments from the expressions, not the values.
		if e != nil && e.Type() != ValueTypeString {
			// ev.currentSamples will be updated to the correct value within the ev.eval call.
			matrixes[i] = ev.eval(e).(Matrix)

			// Keep a copy of the original point slices so that they
			// can be returned to the pool.
			origMatrixes[i] = make(Matrix, len(matrixes[i]))
			copy(origMatrixes[i], matrixes[i])
		}
	}

	vectors := make([]Vector, len(exprs)) // Input vectors for the function.
	args := make([]Value, len(exprs))     // Argument to function.
	// Create an output vector that is as big as the input matrix with
	// the most time series.
	biggestLen := 1
	for i := range exprs {
		vectors[i] = make(Vector, 0, len(matrixes[i]))
		if len(matrixes[i]) > biggestLen {
			biggestLen = len(matrixes[i])
		}
	}
	enh := &EvalNodeHelper{out: make(Vector, 0, biggestLen)}
	seriess := make(map[uint64]Series, biggestLen) // Output series by series hash.
	tempNumSamples := ev.currentSamples
	for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
		if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}
		// Reset number of samples in memory after each timestamp.
		ev.currentSamples = tempNumSamples
		// Gather input vectors for this timestamp.
		for i := range exprs {
			vectors[i] = vectors[i][:0]
			for si, series := range matrixes[i] {
				for _, point := range series.Points {
					if point.T == ts {
						if ev.currentSamples < ev.maxSamples {
							vectors[i] = append(vectors[i], Sample{Metric: series.Metric, Point: point})
							// Move input vectors forward so we don't have to re-scan the same
							// past points at the next step.
							matrixes[i][si].Points = series.Points[1:]
							ev.currentSamples++
						} else {
							ev.error(ErrTooManySamples(env))
						}
					}
					break
				}
			}
			args[i] = vectors[i]
		}
		// Make the function call.
		enh.ts = ts
		result := f(args, enh)
		if result.ContainsSameLabelset() {
			ev.errorf("vector cannot contain metrics with the same labelset")
		}
		enh.out = result[:0] // Reuse result vector.

		ev.currentSamples += len(result)
		// When we reset currentSamples to tempNumSamples during the next iteration of the loop it also
		// needs to include the samples from the result here, as they're still in memory.
		tempNumSamples += len(result)

		if ev.currentSamples > ev.maxSamples {
			ev.error(ErrTooManySamples(env))
		}

		// If this could be an instant query, shortcut so as not to change sort order.
		if ev.endTimestamp == ev.startTimestamp {
			mat := make(Matrix, len(result))
			for i, s := range result {
				s.Point.T = ts
				mat[i] = Series{Metric: s.Metric, Points: []Point{s.Point}}
			}
			ev.currentSamples = originalNumSamples + mat.TotalSamples()
			return mat
		}

		// Add samples in output vector to output series.
		for _, sample := range result {
			h := sample.Metric.Hash()
			ss, ok := seriess[h]
			if !ok {
				ss = Series{
					Metric: sample.Metric,
					Points: getPointSlice(numSteps),
				}
			}
			sample.Point.T = ts
			ss.Points = append(ss.Points, sample.Point)
			seriess[h] = ss

		}
	}

	// Reuse the original point slices.
	for _, m := range origMatrixes {
		for _, s := range m {
			putPointSlice(s.Points)
		}
	}
	// Assemble the output matrix. By the time we get here we know we don't have too many samples.
	mat := make(Matrix, 0, len(seriess))
	for _, ss := range seriess {
		mat = append(mat, ss)
	}
	ev.currentSamples = originalNumSamples + mat.TotalSamples()
	return mat
}

// evalSubquery evaluates given SubqueryExpr and returns an equivalent
// evaluated MatrixSelector in its place. Note that the Name and LabelMatchers are not set.
func (ev *evaluator) evalSubquery(subq *SubqueryExpr) *MatrixSelector {
	val := ev.eval(subq).(Matrix)
	ms := &MatrixSelector{
		Range:  subq.Range,
		Offset: subq.Offset,
		series: make([]storage.Series, 0, len(val)),
	}
	for _, s := range val {
		ms.series = append(ms.series, NewStorageSeries(s))
	}
	return ms
}

// eval evaluates the given expression as the given AST expression node requires.
func (ev *evaluator) eval(expr Expr) Value {
	// This is the top-level evaluation method.
	// Thus, we check for timeout/cancellation here.
	if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
		ev.error(err)
	}
	numSteps := int((ev.endTimestamp-ev.startTimestamp)/ev.interval) + 1

	switch e := expr.(type) {
	case *AggregateExpr:
		if s, ok := e.Param.(*StringLiteral); ok {
			return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
				return ev.aggregation(e.Op, e.Grouping, e.Without, s.Val, v[0].(Vector), enh)
			}, e.Expr)
		}
		return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
			var param float64
			if e.Param != nil {
				param = v[0].(Vector)[0].V
			}
			return ev.aggregation(e.Op, e.Grouping, e.Without, param, v[1].(Vector), enh)
		}, e.Param, e.Expr)

	case *Call:
		if e.Func.Name == "timestamp" {
			// Matrix evaluation always returns the evaluation time,
			// so this function needs special handling when given
			// a vector selector.
			vs, ok := e.Args[0].(*VectorSelector)
			if ok {
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return e.Func.Call([]Value{ev.vectorSelector(vs, enh.ts)}, e.Args, enh)
				})
			}
		}

		// Check if the function has a matrix argument.
		var matrixArgIndex int
		var matrixArg bool
		for i, a := range e.Args {
			if _, ok := a.(*MatrixSelector); ok {
				matrixArgIndex = i
				matrixArg = true
				break
			}
			// SubqueryExpr can be used in place of MatrixSelector.
			if subq, ok := a.(*SubqueryExpr); ok {
				matrixArgIndex = i
				matrixArg = true
				// Replacing SubqueryExpr with MatrixSelector.
				e.Args[i] = ev.evalSubquery(subq)
				break
			}
		}
		if !matrixArg {
			// Does not have a matrix argument.
			return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
				return e.Func.Call(v, e.Args, enh)
			}, e.Args...)
		}

		inArgs := make([]Value, len(e.Args))
		// Evaluate any non-matrix arguments.
		otherArgs := make([]Matrix, len(e.Args))
		otherInArgs := make([]Vector, len(e.Args))
		for i, e := range e.Args {
			if i != matrixArgIndex {
				otherArgs[i] = ev.eval(e).(Matrix)
				otherInArgs[i] = Vector{Sample{}}
				inArgs[i] = otherInArgs[i]
			}
		}

		sel := e.Args[matrixArgIndex].(*MatrixSelector)
		checkForSeriesSetExpansion(ev.ctx, sel)
		mat := make(Matrix, 0, len(sel.series)) // Output matrix.
		offset := durationMilliseconds(sel.Offset)
		selRange := durationMilliseconds(sel.Range)
		stepRange := selRange
		if stepRange > ev.interval {
			stepRange = ev.interval
		}
		// Reuse objects across steps to save memory allocations.
		points := getPointSlice(16)
		inMatrix := make(Matrix, 1)
		inArgs[matrixArgIndex] = inMatrix
		enh := &EvalNodeHelper{out: make(Vector, 0, 1)}
		// Process all the calls for one time series at a time.
		it := storage.NewBuffer(selRange)
		for i, s := range sel.series {
			points = points[:0]
			it.Reset(s.Iterator())
			ss := Series{
				// For all range vector functions, the only change to the
				// output labels is dropping the metric name so just do
				// it once here.
				Metric: dropMetricName(sel.series[i].Labels()),
				Points: getPointSlice(numSteps),
			}
			inMatrix[0].Metric = sel.series[i].Labels()
			for ts, step := ev.startTimestamp, -1; ts <= ev.endTimestamp; ts += ev.interval {
				step++
				// Set the non-matrix arguments.
				// They are scalar, so it is safe to use the step number
				// when looking up the argument, as there will be no gaps.
				for j := range e.Args {
					if j != matrixArgIndex {
						otherInArgs[j][0].V = otherArgs[j][0].Points[step].V
					}
				}
				maxt := ts - offset
				mint := maxt - selRange
				// Evaluate the matrix selector for this series for this step.
				points = ev.matrixIterSlice(it, mint, maxt, points)
				if len(points) == 0 {
					continue
				}
				inMatrix[0].Points = points
				enh.ts = ts
				// Make the function call.
				outVec := e.Func.Call(inArgs, e.Args, enh)
				enh.out = outVec[:0]
				if len(outVec) > 0 {
					ss.Points = append(ss.Points, Point{V: outVec[0].Point.V, T: ts})
				}
				// Only buffer stepRange milliseconds from the second step on.
				it.ReduceDelta(stepRange)
			}
			if len(ss.Points) > 0 {
				if ev.currentSamples < ev.maxSamples {
					mat = append(mat, ss)
					ev.currentSamples += len(ss.Points)
				} else {
					ev.error(ErrTooManySamples(env))
				}
			}
		}
		if mat.ContainsSameLabelset() {
			ev.errorf("vector cannot contain metrics with the same labelset")
		}

		putPointSlice(points)
		return mat

	case *ParenExpr:
		return ev.eval(e.Expr)

	case *UnaryExpr:
		mat := ev.eval(e.Expr).(Matrix)
		if e.Op == ItemSUB {
			for i := range mat {
				mat[i].Metric = dropMetricName(mat[i].Metric)
				for j := range mat[i].Points {
					mat[i].Points[j].V = -mat[i].Points[j].V
				}
			}
			if mat.ContainsSameLabelset() {
				ev.errorf("vector cannot contain metrics with the same labelset")
			}
		}
		return mat

	case *BinaryExpr:
		switch lt, rt := e.LHS.Type(), e.RHS.Type(); {
		case lt == ValueTypeScalar && rt == ValueTypeScalar:
			return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
				val := scalarBinop(e.Op, v[0].(Vector)[0].Point.V, v[1].(Vector)[0].Point.V)
				return append(enh.out, Sample{Point: Point{V: val}})
			}, e.LHS, e.RHS)
		case lt == ValueTypeVector && rt == ValueTypeVector:
			switch e.Op {
			case ItemLAND:
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return ev.VectorAnd(v[0].(Vector), v[1].(Vector), e.VectorMatching, enh)
				}, e.LHS, e.RHS)
			case ItemLOR:
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return ev.VectorOr(v[0].(Vector), v[1].(Vector), e.VectorMatching, enh)
				}, e.LHS, e.RHS)
			case ItemLUnless:
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return ev.VectorUnless(v[0].(Vector), v[1].(Vector), e.VectorMatching, enh)
				}, e.LHS, e.RHS)
			default:
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return ev.VectorBinop(e.Op, v[0].(Vector), v[1].(Vector), e.VectorMatching, e.ReturnBool, enh)
				}, e.LHS, e.RHS)
			}

		case lt == ValueTypeVector && rt == ValueTypeScalar:
			return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
				return ev.VectorscalarBinop(e.Op, v[0].(Vector), Scalar{V: v[1].(Vector)[0].Point.V}, false, e.ReturnBool, enh)
			}, e.LHS, e.RHS)

		case lt == ValueTypeScalar && rt == ValueTypeVector:
			return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
				return ev.VectorscalarBinop(e.Op, v[1].(Vector), Scalar{V: v[0].(Vector)[0].Point.V}, true, e.ReturnBool, enh)
			}, e.LHS, e.RHS)
		}

	case *NumberLiteral:
		return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
			return append(enh.out, Sample{Point: Point{V: e.Val}})
		})

	case *VectorSelector:
		checkForSeriesSetExpansion(ev.ctx, e)
		mat := make(Matrix, 0, len(e.series))
		it := storage.NewBuffer(durationMilliseconds(LookbackDelta))
		for i, s := range e.series {
			it.Reset(s.Iterator())
			ss := Series{
				Metric: e.series[i].Labels(),
				Points: getPointSlice(numSteps),
			}

			for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
				_, v, ok := ev.vectorSelectorSingle(it, e, ts)
				if ok {
					if ev.currentSamples < ev.maxSamples {
						ss.Points = append(ss.Points, Point{V: v, T: ts})
						ev.currentSamples++
					} else {
						ev.error(ErrTooManySamples(env))
					}
				}
			}

			if len(ss.Points) > 0 {
				mat = append(mat, ss)
			}

		}
		return mat

	case *MatrixSelector:
		if ev.startTimestamp != ev.endTimestamp {
			panic(errors.New("cannot do range evaluation of matrix selector"))
		}
		return ev.matrixSelector(e)

	case *SubqueryExpr:
		offsetMillis := durationToInt64Millis(e.Offset)
		rangeMillis := durationToInt64Millis(e.Range)
		newEv := &evaluator{
			endTimestamp:        ev.endTimestamp - offsetMillis,
			interval:            ev.defaultEvalInterval,
			ctx:                 ev.ctx,
			currentSamples:      ev.currentSamples,
			maxSamples:          ev.maxSamples,
			defaultEvalInterval: ev.defaultEvalInterval,
			logger:              ev.logger,
		}

		if e.Step != 0 {
			newEv.interval = durationToInt64Millis(e.Step)
		}

		// Start with the first timestamp after (ev.startTimestamp - offset - range)
		// that is aligned with the step (multiple of 'newEv.interval').
		newEv.startTimestamp = newEv.interval * ((ev.startTimestamp - offsetMillis - rangeMillis) / newEv.interval)
		if newEv.startTimestamp < (ev.startTimestamp - offsetMillis - rangeMillis) {
			newEv.startTimestamp += newEv.interval
		}

		res := newEv.eval(e.Expr)
		ev.currentSamples = newEv.currentSamples
		return res
	}

	panic(errors.Errorf("unhandled expression of type: %T", expr))
}

func durationToInt64Millis(d time.Duration) int64 {
	return int64(d / time.Millisecond)
}

// vectorSelector evaluates a *VectorSelector expression.
func (ev *evaluator) vectorSelector(node *VectorSelector, ts int64) Vector {
	checkForSeriesSetExpansion(ev.ctx, node)

	var (
		vec = make(Vector, 0, len(node.series))
	)

	it := storage.NewBuffer(durationMilliseconds(LookbackDelta))
	for i, s := range node.series {
		it.Reset(s.Iterator())

		t, v, ok := ev.vectorSelectorSingle(it, node, ts)
		if ok {
			vec = append(vec, Sample{
				Metric: node.series[i].Labels(),
				Point:  Point{V: v, T: t},
			})
			ev.currentSamples++
		}

		if ev.currentSamples >= ev.maxSamples {
			ev.error(ErrTooManySamples(env))
		}
	}
	return vec
}

// vectorSelectorSingle evaluates a instant vector for the iterator of one time series.
func (ev *evaluator) vectorSelectorSingle(it *storage.BufferedSeriesIterator, node *VectorSelector, ts int64) (int64, float64, bool) {
	refTime := ts - durationMilliseconds(node.Offset)
	var t int64
	var v float64

	ok := it.Seek(refTime)
	if !ok {
		if it.Err() != nil {
			ev.error(it.Err())
		}
	}

	if ok {
		t, v = it.Values()
	}

	if !ok || t > refTime {
		t, v, ok = it.PeekBack(1)
		if !ok || t < refTime-durationMilliseconds(LookbackDelta) {
			return 0, 0, false
		}
	}
	if value.IsStaleNaN(v) {
		return 0, 0, false
	}
	return t, v, true
}

var pointPool = sync.Pool{}

func getPointSlice(sz int) []Point {
	p := pointPool.Get()
	if p != nil {
		return p.([]Point)
	}
	return make([]Point, 0, sz)
}

func putPointSlice(p []Point) {
	//lint:ignore SA6002 relax staticcheck verification.
	pointPool.Put(p[:0])
}

// matrixSelector evaluates a *MatrixSelector expression.
func (ev *evaluator) matrixSelector(node *MatrixSelector) Matrix {
	checkForSeriesSetExpansion(ev.ctx, node)

	var (
		offset = durationMilliseconds(node.Offset)
		maxt   = ev.startTimestamp - offset
		mint   = maxt - durationMilliseconds(node.Range)
		matrix = make(Matrix, 0, len(node.series))
	)

	it := storage.NewBuffer(durationMilliseconds(node.Range))
	for i, s := range node.series {
		if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}
		it.Reset(s.Iterator())
		ss := Series{
			Metric: node.series[i].Labels(),
		}

		ss.Points = ev.matrixIterSlice(it, mint, maxt, getPointSlice(16))

		if len(ss.Points) > 0 {
			matrix = append(matrix, ss)
		} else {
			putPointSlice(ss.Points)
		}
	}
	return matrix
}

// matrixIterSlice populates a matrix vector covering the requested range for a
// single time series, with points retrieved from an iterator.
//
// As an optimization, the matrix vector may already contain points of the same
// time series from the evaluation of an earlier step (with lower mint and maxt
// values). Any such points falling before mint are discarded; points that fall
// into the [mint, maxt] range are retained; only points with later timestamps
// are populated from the iterator.
func (ev *evaluator) matrixIterSlice(it *storage.BufferedSeriesIterator, mint, maxt int64, out []Point) []Point {
	if len(out) > 0 && out[len(out)-1].T >= mint {
		// There is an overlap between previous and current ranges, retain common
		// points. In most such cases:
		//   (a) the overlap is significantly larger than the eval step; and/or
		//   (b) the number of samples is relatively small.
		// so a linear search will be as fast as a binary search.
		var drop int
		for drop = 0; out[drop].T < mint; drop++ {
		}
		copy(out, out[drop:])
		out = out[:len(out)-drop]
		// Only append points with timestamps after the last timestamp we have.
		mint = out[len(out)-1].T + 1
	} else {
		out = out[:0]
	}

	ok := it.Seek(maxt)
	if !ok {
		if it.Err() != nil {
			ev.error(it.Err())
		}
	}

	buf := it.Buffer()
	for buf.Next() {
		t, v := buf.At()
		if value.IsStaleNaN(v) {
			continue
		}
		// Values in the buffer are guaranteed to be smaller than maxt.
		if t >= mint {
			if ev.currentSamples >= ev.maxSamples {
				ev.error(ErrTooManySamples(env))
			}
			out = append(out, Point{T: t, V: v})
			ev.currentSamples++
		}
	}
	// The seeked sample might also be in the range.
	if ok {
		t, v := it.Values()
		if t == maxt && !value.IsStaleNaN(v) {
			if ev.currentSamples >= ev.maxSamples {
				ev.error(ErrTooManySamples(env))
			}
			out = append(out, Point{T: t, V: v})
			ev.currentSamples++
		}
	}
	return out
}

func (ev *evaluator) VectorAnd(lhs, rhs Vector, matching *VectorMatching, enh *EvalNodeHelper) Vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := enh.signatureFunc(matching.On, matching.MatchingLabels...)

	// The set of signatures for the right-hand side Vector.
	rightSigs := map[uint64]struct{}{}
	// Add all rhs samples to a map so we can easily find matches later.
	for _, rs := range rhs {
		rightSigs[sigf(rs.Metric)] = struct{}{}
	}

	for _, ls := range lhs {
		// If there's a matching entry in the right-hand side Vector, add the sample.
		if _, ok := rightSigs[sigf(ls.Metric)]; ok {
			enh.out = append(enh.out, ls)
		}
	}
	return enh.out
}

func (ev *evaluator) VectorOr(lhs, rhs Vector, matching *VectorMatching, enh *EvalNodeHelper) Vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := enh.signatureFunc(matching.On, matching.MatchingLabels...)

	leftSigs := map[uint64]struct{}{}
	// Add everything from the left-hand-side Vector.
	for _, ls := range lhs {
		leftSigs[sigf(ls.Metric)] = struct{}{}
		enh.out = append(enh.out, ls)
	}
	// Add all right-hand side elements which have not been added from the left-hand side.
	for _, rs := range rhs {
		if _, ok := leftSigs[sigf(rs.Metric)]; !ok {
			enh.out = append(enh.out, rs)
		}
	}
	return enh.out
}

func (ev *evaluator) VectorUnless(lhs, rhs Vector, matching *VectorMatching, enh *EvalNodeHelper) Vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := enh.signatureFunc(matching.On, matching.MatchingLabels...)

	rightSigs := map[uint64]struct{}{}
	for _, rs := range rhs {
		rightSigs[sigf(rs.Metric)] = struct{}{}
	}

	for _, ls := range lhs {
		if _, ok := rightSigs[sigf(ls.Metric)]; !ok {
			enh.out = append(enh.out, ls)
		}
	}
	return enh.out
}

// VectorBinop evaluates a binary operation between two Vectors, excluding set operators.
func (ev *evaluator) VectorBinop(op ItemType, lhs, rhs Vector, matching *VectorMatching, returnBool bool, enh *EvalNodeHelper) Vector {
	if matching.Card == CardManyToMany {
		panic("many-to-many only allowed for set operators")
	}
	sigf := enh.signatureFunc(matching.On, matching.MatchingLabels...)

	// The control flow below handles one-to-one or many-to-one matching.
	// For one-to-many, swap sidedness and account for the swap when calculating
	// values.
	if matching.Card == CardOneToMany {
		lhs, rhs = rhs, lhs
	}

	// All samples from the rhs hashed by the matching label/values.
	if enh.rightSigs == nil {
		enh.rightSigs = make(map[uint64]Sample, len(enh.out))
	} else {
		for k := range enh.rightSigs {
			delete(enh.rightSigs, k)
		}
	}
	rightSigs := enh.rightSigs

	// Add all rhs samples to a map so we can easily find matches later.
	for _, rs := range rhs {
		sig := sigf(rs.Metric)
		// The rhs is guaranteed to be the 'one' side. Having multiple samples
		// with the same signature means that the matching is many-to-many.
		if duplSample, found := rightSigs[sig]; found {
			// oneSide represents which side of the vector represents the 'one' in the many-to-one relationship.
			oneSide := "right"
			if matching.Card == CardOneToMany {
				oneSide = "left"
			}
			matchedLabels := rs.Metric.MatchLabels(matching.On, matching.MatchingLabels...)
			// Many-to-many matching not allowed.
			ev.errorf("found duplicate series for the match group %s on the %s hand-side of the operation: [%s, %s]"+
				";many-to-many matching not allowed: matching labels must be unique on one side", matchedLabels.String(), oneSide, rs.Metric.String(), duplSample.Metric.String())
		}
		rightSigs[sig] = rs
	}

	// Tracks the match-signature. For one-to-one operations the value is nil. For many-to-one
	// the value is a set of signatures to detect duplicated result elements.
	if enh.matchedSigs == nil {
		enh.matchedSigs = make(map[uint64]map[uint64]struct{}, len(rightSigs))
	} else {
		for k := range enh.matchedSigs {
			delete(enh.matchedSigs, k)
		}
	}
	matchedSigs := enh.matchedSigs

	// For all lhs samples find a respective rhs sample and perform
	// the binary operation.
	for _, ls := range lhs {
		sig := sigf(ls.Metric)

		rs, found := rightSigs[sig] // Look for a match in the rhs Vector.
		if !found {
			continue
		}

		// Account for potentially swapped sidedness.
		vl, vr := ls.V, rs.V
		if matching.Card == CardOneToMany {
			vl, vr = vr, vl
		}
		value, keep := vectorElemBinop(op, vl, vr)
		if returnBool {
			if keep {
				value = 1.0
			} else {
				value = 0.0
			}
		} else if !keep {
			continue
		}
		metric := resultMetric(ls.Metric, rs.Metric, op, matching, enh)

		insertedSigs, exists := matchedSigs[sig]
		if matching.Card == CardOneToOne {
			if exists {
				ev.errorf("multiple matches for labels: many-to-one matching must be explicit (group_left/group_right)")
			}
			matchedSigs[sig] = nil // Set existence to true.
		} else {
			// In many-to-one matching the grouping labels have to ensure a unique metric
			// for the result Vector. Check whether those labels have already been added for
			// the same matching labels.
			insertSig := metric.Hash()

			if !exists {
				insertedSigs = map[uint64]struct{}{}
				matchedSigs[sig] = insertedSigs
			} else if _, duplicate := insertedSigs[insertSig]; duplicate {
				ev.errorf("multiple matches for labels: grouping labels must ensure unique matches")
			}
			insertedSigs[insertSig] = struct{}{}
		}

		enh.out = append(enh.out, Sample{
			Metric: metric,
			Point:  Point{V: value},
		})
	}
	return enh.out
}

// signatureFunc returns a function that calculates the signature for a metric
// ignoring the provided labels. If on, then the given labels are only used instead.
func signatureFunc(on bool, names ...string) func(labels.Labels) uint64 {
	sort.Strings(names)
	if on {
		return func(lset labels.Labels) uint64 {
			h, _ := lset.HashForLabels(make([]byte, 0, 1024), names...)
			return h
		}
	}
	return func(lset labels.Labels) uint64 {
		h, _ := lset.HashWithoutLabels(make([]byte, 0, 1024), names...)
		return h
	}
}

// resultMetric returns the metric for the given sample(s) based on the Vector
// binary operation and the matching options.
func resultMetric(lhs, rhs labels.Labels, op ItemType, matching *VectorMatching, enh *EvalNodeHelper) labels.Labels {
	if enh.resultMetric == nil {
		enh.resultMetric = make(map[uint64]labels.Labels, len(enh.out))
	}
	// op and matching are always the same for a given node, so
	// there's no need to include them in the hash key.
	// If the lhs and rhs are the same then the xor would be 0,
	// so add in one side to protect against that.
	lh := lhs.Hash()
	h := (lh ^ rhs.Hash()) + lh
	if ret, ok := enh.resultMetric[h]; ok {
		return ret
	}

	lb := labels.NewBuilder(lhs)

	if shouldDropMetricName(op) {
		lb.Del(labels.MetricName)
	}

	if matching.Card == CardOneToOne {
		if matching.On {
		Outer:
			for _, l := range lhs {
				for _, n := range matching.MatchingLabels {
					if l.Name == n {
						continue Outer
					}
				}
				lb.Del(l.Name)
			}
		} else {
			lb.Del(matching.MatchingLabels...)
		}
	}
	for _, ln := range matching.Include {
		// Included labels from the `group_x` modifier are taken from the "one"-side.
		if v := rhs.Get(ln); v != "" {
			lb.Set(ln, v)
		} else {
			lb.Del(ln)
		}
	}

	ret := lb.Labels()
	enh.resultMetric[h] = ret
	return ret
}

// VectorscalarBinop evaluates a binary operation between a Vector and a Scalar.
func (ev *evaluator) VectorscalarBinop(op ItemType, lhs Vector, rhs Scalar, swap, returnBool bool, enh *EvalNodeHelper) Vector {
	for _, lhsSample := range lhs {
		lv, rv := lhsSample.V, rhs.V
		// lhs always contains the Vector. If the original position was different
		// swap for calculating the value.
		if swap {
			lv, rv = rv, lv
		}
		value, keep := vectorElemBinop(op, lv, rv)
		// Catch cases where the scalar is the LHS in a scalar-vector comparison operation.
		// We want to always keep the vector element value as the output value, even if it's on the RHS.
		if op.isComparisonOperator() && swap {
			value = rv
		}
		if returnBool {
			if keep {
				value = 1.0
			} else {
				value = 0.0
			}
			keep = true
		}
		if keep {
			lhsSample.V = value
			if shouldDropMetricName(op) || returnBool {
				lhsSample.Metric = enh.dropMetricName(lhsSample.Metric)
			}
			enh.out = append(enh.out, lhsSample)
		}
	}
	return enh.out
}

func dropMetricName(l labels.Labels) labels.Labels {
	return labels.NewBuilder(l).Del(labels.MetricName).Labels()
}

// scalarBinop evaluates a binary operation between two Scalars.
func scalarBinop(op ItemType, lhs, rhs float64) float64 {
	switch op {
	case ItemADD:
		return lhs + rhs
	case ItemSUB:
		return lhs - rhs
	case ItemMUL:
		return lhs * rhs
	case ItemDIV:
		return lhs / rhs
	case ItemPOW:
		return math.Pow(lhs, rhs)
	case ItemMOD:
		return math.Mod(lhs, rhs)
	case ItemEQL:
		return btos(lhs == rhs)
	case ItemNEQ:
		return btos(lhs != rhs)
	case ItemGTR:
		return btos(lhs > rhs)
	case ItemLSS:
		return btos(lhs < rhs)
	case ItemGTE:
		return btos(lhs >= rhs)
	case ItemLTE:
		return btos(lhs <= rhs)
	}
	panic(errors.Errorf("operator %q not allowed for Scalar operations", op))
}

// vectorElemBinop evaluates a binary operation between two Vector elements.
func vectorElemBinop(op ItemType, lhs, rhs float64) (float64, bool) {
	switch op {
	case ItemADD:
		return lhs + rhs, true
	case ItemSUB:
		return lhs - rhs, true
	case ItemMUL:
		return lhs * rhs, true
	case ItemDIV:
		return lhs / rhs, true
	case ItemPOW:
		return math.Pow(lhs, rhs), true
	case ItemMOD:
		return math.Mod(lhs, rhs), true
	case ItemEQL:
		return lhs, lhs == rhs
	case ItemNEQ:
		return lhs, lhs != rhs
	case ItemGTR:
		return lhs, lhs > rhs
	case ItemLSS:
		return lhs, lhs < rhs
	case ItemGTE:
		return lhs, lhs >= rhs
	case ItemLTE:
		return lhs, lhs <= rhs
	}
	panic(errors.Errorf("operator %q not allowed for operations between Vectors", op))
}

type groupedAggregation struct {
	labels      labels.Labels
	value       float64
	mean        float64
	groupCount  int
	heap        vectorByValueHeap
	reverseHeap vectorByReverseValueHeap
}

// aggregation evaluates an aggregation operation on a Vector.
func (ev *evaluator) aggregation(op ItemType, grouping []string, without bool, param interface{}, vec Vector, enh *EvalNodeHelper) Vector {

	result := map[uint64]*groupedAggregation{}
	var k int64
	if op == ItemTopK || op == ItemBottomK {
		f := param.(float64)
		if !convertibleToInt64(f) {
			ev.errorf("Scalar value %v overflows int64", f)
		}
		k = int64(f)
		if k < 1 {
			return Vector{}
		}
	}
	var q float64
	if op == ItemQuantile {
		q = param.(float64)
	}
	var valueLabel string
	if op == ItemCountValues {
		valueLabel = param.(string)
		if !model.LabelName(valueLabel).IsValid() {
			ev.errorf("invalid label name %q", valueLabel)
		}
		if !without {
			grouping = append(grouping, valueLabel)
		}
	}

	sort.Strings(grouping)
	lb := labels.NewBuilder(nil)
	buf := make([]byte, 0, 1024)
	for _, s := range vec {
		metric := s.Metric

		if op == ItemCountValues {
			lb.Reset(metric)
			lb.Set(valueLabel, strconv.FormatFloat(s.V, 'f', -1, 64))
			metric = lb.Labels()
		}

		var (
			groupingKey uint64
		)
		if without {
			groupingKey, buf = metric.HashWithoutLabels(buf, grouping...)
		} else {
			groupingKey, buf = metric.HashForLabels(buf, grouping...)
		}

		group, ok := result[groupingKey]
		// Add a new group if it doesn't exist.
		if !ok {
			var m labels.Labels

			if without {
				lb.Reset(metric)
				lb.Del(grouping...)
				lb.Del(labels.MetricName)
				m = lb.Labels()
			} else {
				m = make(labels.Labels, 0, len(grouping))
				for _, l := range metric {
					for _, n := range grouping {
						if l.Name == n {
							m = append(m, l)
							break
						}
					}
				}
				sort.Sort(m)
			}
			result[groupingKey] = &groupedAggregation{
				labels:     m,
				value:      s.V,
				mean:       s.V,
				groupCount: 1,
			}
			inputVecLen := int64(len(vec))
			resultSize := k
			if k > inputVecLen {
				resultSize = inputVecLen
			}
			if op == ItemStdvar || op == ItemStddev {
				result[groupingKey].value = 0.0
			} else if op == ItemTopK || op == ItemQuantile {
				result[groupingKey].heap = make(vectorByValueHeap, 0, resultSize)
				heap.Push(&result[groupingKey].heap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			} else if op == ItemBottomK {
				result[groupingKey].reverseHeap = make(vectorByReverseValueHeap, 0, resultSize)
				heap.Push(&result[groupingKey].reverseHeap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}
			continue
		}

		switch op {
		case ItemSum:
			group.value += s.V

		case ItemAvg:
			group.groupCount++
			group.mean += (s.V - group.mean) / float64(group.groupCount)

		case ItemMax:
			if group.value < s.V || math.IsNaN(group.value) {
				group.value = s.V
			}

		case ItemMin:
			if group.value > s.V || math.IsNaN(group.value) {
				group.value = s.V
			}

		case ItemCount, ItemCountValues:
			group.groupCount++

		case ItemStdvar, ItemStddev:
			group.groupCount++
			delta := s.V - group.mean
			group.mean += delta / float64(group.groupCount)
			group.value += delta * (s.V - group.mean)

		case ItemTopK:
			if int64(len(group.heap)) < k || group.heap[0].V < s.V || math.IsNaN(group.heap[0].V) {
				if int64(len(group.heap)) == k {
					heap.Pop(&group.heap)
				}
				heap.Push(&group.heap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}

		case ItemBottomK:
			if int64(len(group.reverseHeap)) < k || group.reverseHeap[0].V > s.V || math.IsNaN(group.reverseHeap[0].V) {
				if int64(len(group.reverseHeap)) == k {
					heap.Pop(&group.reverseHeap)
				}
				heap.Push(&group.reverseHeap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}

		case ItemQuantile:
			group.heap = append(group.heap, s)

		default:
			panic(errors.Errorf("expected aggregation operator but got %q", op))
		}
	}

	// Construct the result Vector from the aggregated groups.
	for _, aggr := range result {
		switch op {
		case ItemAvg:
			aggr.value = aggr.mean

		case ItemCount, ItemCountValues:
			aggr.value = float64(aggr.groupCount)

		case ItemStdvar:
			aggr.value = aggr.value / float64(aggr.groupCount)

		case ItemStddev:
			aggr.value = math.Sqrt(aggr.value / float64(aggr.groupCount))

		case ItemTopK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.heap))
			for _, v := range aggr.heap {
				enh.out = append(enh.out, Sample{
					Metric: v.Metric,
					Point:  Point{V: v.V},
				})
			}
			continue // Bypass default append.

		case ItemBottomK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.reverseHeap))
			for _, v := range aggr.reverseHeap {
				enh.out = append(enh.out, Sample{
					Metric: v.Metric,
					Point:  Point{V: v.V},
				})
			}
			continue // Bypass default append.

		case ItemQuantile:
			aggr.value = quantile(q, aggr.heap)

		default:
			// For other aggregations, we already have the right value.
		}

		enh.out = append(enh.out, Sample{
			Metric: aggr.labels,
			Point:  Point{V: aggr.value},
		})
	}
	return enh.out
}

// btos returns 1 if b is true, 0 otherwise.
func btos(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// shouldDropMetricName returns whether the metric name should be dropped in the
// result of the op operation.
func shouldDropMetricName(op ItemType) bool {
	switch op {
	case ItemADD, ItemSUB, ItemDIV, ItemMUL, ItemPOW, ItemMOD:
		return true
	default:
		return false
	}
}

// documentedType returns the internal type to the equivalent
// user facing terminology as defined in the documentation.
func documentedType(t ValueType) string {
	switch t {
	case "vector":
		return "instant vector"
	case "matrix":
		return "range vector"
	default:
		return string(t)
	}
}
