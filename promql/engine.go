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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
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

	// The largest SampleValue that can be converted to an int64 without overflow.
	maxInt64 = 9223372036854774784
	// The smallest SampleValue that can be converted to an int64 without underflow.
	minInt64 = -9223372036854775808
)

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
	// ErrStorage is returned if an error was encountered in the storage layer
	// during query handling.
	ErrStorage error
)

func (e ErrQueryTimeout) Error() string  { return fmt.Sprintf("query timed out in %s", string(e)) }
func (e ErrQueryCanceled) Error() string { return fmt.Sprintf("query was canceled in %s", string(e)) }

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
	Stats() *stats.TimerGroup
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
	stats *stats.TimerGroup
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
func (q *query) Stats() *stats.TimerGroup {
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

	res, err := q.ng.exec(ctx, q)
	return &Result{Err: err, Value: res}
}

// contextDone returns an error if the context was canceled or timed out.
func contextDone(ctx context.Context, env string) error {
	select {
	case <-ctx.Done():
		err := ctx.Err()
		switch err {
		case context.Canceled:
			return ErrQueryCanceled(env)
		case context.DeadlineExceeded:
			return ErrQueryTimeout(env)
		default:
			return err
		}
	default:
		return nil
	}
}

// Engine handles the lifetime of queries from beginning to end.
// It is connected to a querier.
type Engine struct {
	logger  log.Logger
	metrics *engineMetrics
	timeout time.Duration
	gate    *queryGate
}

// NewEngine returns a new engine.
func NewEngine(logger log.Logger, reg prometheus.Registerer, maxConcurrent int, timeout time.Duration) *Engine {
	if logger == nil {
		logger = log.NewNopLogger()
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
		}),
		queryPrepareTime: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "prepare_time"},
		}),
		queryInnerEval: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "inner_eval"},
		}),
		queryResultSort: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "result_sort"},
		}),
	}
	metrics.maxConcurrentQueries.Set(float64(maxConcurrent))

	if reg != nil {
		reg.MustRegister(
			metrics.currentQueries,
			metrics.maxConcurrentQueries,
			metrics.queryInnerEval,
			metrics.queryPrepareTime,
			metrics.queryResultSort,
		)
	}
	return &Engine{
		gate:    newQueryGate(maxConcurrent),
		timeout: timeout,
		logger:  logger,
		metrics: metrics,
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
		return nil, fmt.Errorf("invalid expression type %q for range query, must be Scalar or instant Vector", documentedType(expr.Type()))
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
		stats:     stats.NewTimerGroup(),
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
		stats: stats.NewTimerGroup(),
	}
	return qry
}

// exec executes the query.
//
// At this point per query only one EvalStmt is evaluated. Alert and record
// statements are not handled by the Engine.
func (ng *Engine) exec(ctx context.Context, q *query) (Value, error) {
	ng.metrics.currentQueries.Inc()
	defer ng.metrics.currentQueries.Dec()

	ctx, cancel := context.WithTimeout(ctx, ng.timeout)
	q.cancel = cancel

	execTimer := q.stats.GetTimer(stats.ExecTotalTime).Start()
	defer execTimer.Stop()
	queueTimer := q.stats.GetTimer(stats.ExecQueueTime).Start()

	if err := ng.gate.Start(ctx); err != nil {
		return nil, err
	}
	defer ng.gate.Done()

	queueTimer.Stop()
	ng.metrics.queryQueueTime.Observe(queueTimer.ElapsedTime().Seconds())

	// Cancel when execution is done or an error was raised.
	defer q.cancel()

	const env = "query execution"

	evalTimer := q.stats.GetTimer(stats.EvalTotalTime).Start()
	defer evalTimer.Stop()

	// The base context might already be canceled on the first iteration (e.g. during shutdown).
	if err := contextDone(ctx, env); err != nil {
		return nil, err
	}

	switch s := q.Statement().(type) {
	case *EvalStmt:
		return ng.execEvalStmt(ctx, q, s)
	case testStmt:
		return nil, s(ctx)
	}

	panic(fmt.Errorf("promql.Engine.exec: unhandled statement of type %T", q.Statement()))
}

func timeMilliseconds(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond/time.Nanosecond)
}

func durationMilliseconds(d time.Duration) int64 {
	return int64(d / (time.Millisecond / time.Nanosecond))
}

// execEvalStmt evaluates the expression of an evaluation statement for the given time range.
func (ng *Engine) execEvalStmt(ctx context.Context, query *query, s *EvalStmt) (Value, error) {
	prepareTimer := query.stats.GetTimer(stats.QueryPreparationTime).Start()
	querier, err := ng.populateSeries(ctx, query.queryable, s)
	prepareTimer.Stop()
	ng.metrics.queryPrepareTime.Observe(prepareTimer.ElapsedTime().Seconds())

	// XXX(fabxc): the querier returned by populateSeries might be instantiated
	// we must not return without closing irrespective of the error.
	// TODO: make this semantically saner.
	if querier != nil {
		defer querier.Close()
	}

	if err != nil {
		return nil, err
	}

	evalTimer := query.stats.GetTimer(stats.InnerEvalTime).Start()
	// Instant evaluation. This is executed as a range evaluation with one step.
	if s.Start == s.End && s.Interval == 0 {
		start := timeMilliseconds(s.Start)
		evaluator := &evaluator{
			startTimestamp: start,
			endTimestamp:   start,
			interval:       1,
			ctx:            ctx,
			logger:         ng.logger,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		evalTimer.Stop()
		ng.metrics.queryInnerEval.Observe(evalTimer.ElapsedTime().Seconds())

		mat, ok := val.(Matrix)
		if !ok {
			panic(fmt.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
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
			return vector, nil
		case ValueTypeScalar:
			return Scalar{V: mat[0].Points[0].V, T: start}, nil
		case ValueTypeMatrix:
			return mat, nil
		default:
			panic(fmt.Errorf("promql.Engine.exec: unexpected expression type %q", s.Expr.Type()))
		}

	}

	// Range evaluation.
	evaluator := &evaluator{
		startTimestamp: timeMilliseconds(s.Start),
		endTimestamp:   timeMilliseconds(s.End),
		interval:       durationMilliseconds(s.Interval),
		ctx:            ctx,
		logger:         ng.logger,
	}
	val, err := evaluator.Eval(s.Expr)
	if err != nil {
		return nil, err
	}
	evalTimer.Stop()
	ng.metrics.queryInnerEval.Observe(evalTimer.ElapsedTime().Seconds())

	mat, ok := val.(Matrix)
	if !ok {
		panic(fmt.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
	}
	query.matrix = mat

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	// TODO(fabxc): order ensured by storage?
	// TODO(fabxc): where to ensure metric labels are a copy from the storage internals.
	sortTimer := query.stats.GetTimer(stats.ResultSortTime).Start()
	sort.Sort(mat)
	sortTimer.Stop()

	ng.metrics.queryResultSort.Observe(sortTimer.ElapsedTime().Seconds())
	return mat, nil
}

func (ng *Engine) populateSeries(ctx context.Context, q storage.Queryable, s *EvalStmt) (storage.Querier, error) {
	var maxOffset time.Duration
	Inspect(s.Expr, func(node Node, _ []Node) error {
		switch n := node.(type) {
		case *VectorSelector:
			if maxOffset < LookbackDelta {
				maxOffset = LookbackDelta
			}
			if n.Offset+LookbackDelta > maxOffset {
				maxOffset = n.Offset + LookbackDelta
			}
		case *MatrixSelector:
			if maxOffset < n.Range {
				maxOffset = n.Range
			}
			if n.Offset+n.Range > maxOffset {
				maxOffset = n.Offset + n.Range
			}
		}
		return nil
	})

	mint := s.Start.Add(-maxOffset)

	querier, err := q.Querier(ctx, timestamp.FromTime(mint), timestamp.FromTime(s.End))
	if err != nil {
		return nil, err
	}

	Inspect(s.Expr, func(node Node, path []Node) error {
		var set storage.SeriesSet
		params := &storage.SelectParams{
			Step: int64(s.Interval / time.Millisecond),
		}

		switch n := node.(type) {
		case *VectorSelector:
			params.Func = extractFuncFromPath(path)

			set, err = querier.Select(params, n.LabelMatchers...)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error selecting series set", "err", err)
				return err
			}
			n.series, err = expandSeriesSet(set)
			if err != nil {
				// TODO(fabxc): use multi-error.
				level.Error(ng.logger).Log("msg", "error expanding series set", "err", err)
				return err
			}

		case *MatrixSelector:
			params.Func = extractFuncFromPath(path)

			set, err = querier.Select(params, n.LabelMatchers...)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error selecting series set", "err", err)
				return err
			}
			n.series, err = expandSeriesSet(set)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error expanding series set", "err", err)
				return err
			}
		}
		return nil
	})
	return querier, err
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

func expandSeriesSet(it storage.SeriesSet) (res []storage.Series, err error) {
	for it.Next() {
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

	endTimestamp int64 // End time in milliseconds.
	interval     int64 // Interval in milliseconds.

	logger log.Logger
}

// errorf causes a panic with the input formatted into an error.
func (ev *evaluator) errorf(format string, args ...interface{}) {
	ev.error(fmt.Errorf(format, args...))
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
		*errp = fmt.Errorf("unexpected error: %s", err)
	} else {
		*errp = e.(error)
	}
}

func (ev *evaluator) Eval(expr Expr) (v Value, err error) {
	defer ev.recover(&err)
	return ev.eval(expr), nil
}

// Extra information and caches for evaluating a single node across steps.
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
// step.  The return value is the combination into time series of  of all the
// function call results.
func (ev *evaluator) rangeEval(f func([]Value, *EvalNodeHelper) Vector, exprs ...Expr) Matrix {
	numSteps := int((ev.endTimestamp-ev.startTimestamp)/ev.interval) + 1
	matrixes := make([]Matrix, len(exprs))
	origMatrixes := make([]Matrix, len(exprs))
	for i, e := range exprs {
		// Functions will take string arguments from the expressions, not the values.
		if e != nil && e.Type() != ValueTypeString {
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
	for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
		// Gather input vectors for this timestamp.
		for i := range exprs {
			vectors[i] = vectors[i][:0]
			for si, series := range matrixes[i] {
				for _, point := range series.Points {
					if point.T == ts {
						vectors[i] = append(vectors[i], Sample{Metric: series.Metric, Point: point})
						// Move input vectors forward so we don't have to re-scan the same
						// past points at the next step.
						matrixes[i][si].Points = series.Points[1:]
					}
					break
				}
			}
			args[i] = vectors[i]
		}
		// Make the function call.
		enh.ts = ts
		result := f(args, enh)
		enh.out = result[:0] // Reuse result vector.
		// If this could be an instant query, shortcut so as not to change sort order.
		if ev.endTimestamp == ev.startTimestamp {
			mat := make(Matrix, len(result))
			for i, s := range result {
				s.Point.T = ts
				mat[i] = Series{Metric: s.Metric, Points: []Point{s.Point}}
			}
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
	// Assemble the output matrix.
	mat := make(Matrix, 0, len(seriess))
	for _, ss := range seriess {
		mat = append(mat, ss)
	}
	return mat
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
			_, ok := a.(*MatrixSelector)
			if ok {
				matrixArgIndex = i
				matrixArg = true
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
		mat := make(Matrix, 0, len(sel.series)) // Output matrix.
		offset := durationMilliseconds(sel.Offset)
		selRange := durationMilliseconds(sel.Range)
		// Reuse objects across steps to save memory allocations.
		points := getPointSlice(16)
		inMatrix := make(Matrix, 1)
		inArgs[matrixArgIndex] = inMatrix
		enh := &EvalNodeHelper{out: make(Vector, 0, 1)}
		// Process all the calls for one time series at a time.
		var it *storage.BufferedSeriesIterator
		for i, s := range sel.series {
			if it == nil {
				it = storage.NewBuffer(s.Iterator(), selRange)
			} else {
				it.Reset(s.Iterator())
			}
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
				points = ev.matrixIterSlice(it, mint, maxt, points[:0])
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
			}
			if len(ss.Points) > 0 {
				mat = append(mat, ss)
			}
		}
		putPointSlice(points)
		return mat

	case *ParenExpr:
		return ev.eval(e.Expr)

	case *UnaryExpr:
		mat := ev.eval(e.Expr).(Matrix)
		if e.Op == itemSUB {
			for i := range mat {
				mat[i].Metric = dropMetricName(mat[i].Metric)
				for j := range mat[i].Points {
					mat[i].Points[j].V = -mat[i].Points[j].V
				}
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
			case itemLAND:
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return ev.VectorAnd(v[0].(Vector), v[1].(Vector), e.VectorMatching, enh)
				}, e.LHS, e.RHS)
			case itemLOR:
				return ev.rangeEval(func(v []Value, enh *EvalNodeHelper) Vector {
					return ev.VectorOr(v[0].(Vector), v[1].(Vector), e.VectorMatching, enh)
				}, e.LHS, e.RHS)
			case itemLUnless:
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
		mat := make(Matrix, 0, len(e.series))
		var it *storage.BufferedSeriesIterator
		for i, s := range e.series {
			if it == nil {
				it = storage.NewBuffer(s.Iterator(), durationMilliseconds(LookbackDelta))
			} else {
				it.Reset(s.Iterator())
			}
			ss := Series{
				Metric: e.series[i].Labels(),
				Points: getPointSlice(numSteps),
			}

			for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
				_, v, ok := ev.vectorSelectorSingle(it, e, ts)
				if ok {
					ss.Points = append(ss.Points, Point{V: v, T: ts})
				}
			}

			if len(ss.Points) > 0 {
				mat = append(mat, ss)
			}
		}
		return mat

	case *MatrixSelector:
		if ev.startTimestamp != ev.endTimestamp {
			panic(fmt.Errorf("cannot do range evaluation of matrix selector"))
		}
		return ev.matrixSelector(e)
	}

	panic(fmt.Errorf("unhandled expression of type: %T", expr))
}

// vectorSelector evaluates a *VectorSelector expression.
func (ev *evaluator) vectorSelector(node *VectorSelector, ts int64) Vector {
	var (
		vec = make(Vector, 0, len(node.series))
	)

	var it *storage.BufferedSeriesIterator
	for i, s := range node.series {
		if it == nil {
			it = storage.NewBuffer(s.Iterator(), durationMilliseconds(LookbackDelta))
		} else {
			it.Reset(s.Iterator())
		}

		t, v, ok := ev.vectorSelectorSingle(it, node, ts)
		if ok {
			vec = append(vec, Sample{
				Metric: node.series[i].Labels(),
				Point:  Point{V: v, T: t},
			})
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
	pointPool.Put(p[:0])
}

// matrixSelector evaluates a *MatrixSelector expression.
func (ev *evaluator) matrixSelector(node *MatrixSelector) Matrix {
	var (
		offset = durationMilliseconds(node.Offset)
		maxt   = ev.startTimestamp - offset
		mint   = maxt - durationMilliseconds(node.Range)
		matrix = make(Matrix, 0, len(node.series))
	)

	var it *storage.BufferedSeriesIterator
	for i, s := range node.series {
		if it == nil {
			it = storage.NewBuffer(s.Iterator(), durationMilliseconds(node.Range))
		} else {
			it.Reset(s.Iterator())
		}
		ss := Series{
			Metric: node.series[i].Labels(),
		}

		ss.Points = ev.matrixIterSlice(it, mint, maxt, getPointSlice(16))

		if len(ss.Points) > 0 {
			matrix = append(matrix, ss)
		}
	}
	return matrix
}

// matrixIterSlice evaluates a matrix vector for the iterator of one time series.
func (ev *evaluator) matrixIterSlice(it *storage.BufferedSeriesIterator, mint, maxt int64, out []Point) []Point {
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
			out = append(out, Point{T: t, V: v})
		}
	}
	// The seeked sample might also be in the range.
	if ok {
		t, v := it.Values()
		if t == maxt && !value.IsStaleNaN(v) {
			out = append(out, Point{T: t, V: v})
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
		if _, found := rightSigs[sig]; found {
			// Many-to-many matching not allowed.
			ev.errorf("many-to-many matching not allowed: matching labels must be unique on one side")
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

func hashWithoutLabels(lset labels.Labels, names ...string) uint64 {
	cm := make(labels.Labels, 0, len(lset))

Outer:
	for _, l := range lset {
		for _, n := range names {
			if n == l.Name {
				continue Outer
			}
		}
		if l.Name == labels.MetricName {
			continue
		}
		cm = append(cm, l)
	}

	return cm.Hash()
}

func hashForLabels(lset labels.Labels, names ...string) uint64 {
	cm := make(labels.Labels, 0, len(names))

	for _, l := range lset {
		for _, n := range names {
			if l.Name == n {
				cm = append(cm, l)
				break
			}
		}
	}
	return cm.Hash()
}

// signatureFunc returns a function that calculates the signature for a metric
// ignoring the provided labels. If on, then the given labels are only used instead.
func signatureFunc(on bool, names ...string) func(labels.Labels) uint64 {
	// TODO(fabxc): ensure names are sorted and then use that and sortedness
	// of labels by names to speed up the operations below.
	// Alternatively, inline the hashing and don't build new label sets.
	if on {
		return func(lset labels.Labels) uint64 { return hashForLabels(lset, names...) }
	}
	return func(lset labels.Labels) uint64 { return hashWithoutLabels(lset, names...) }
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
	case itemADD:
		return lhs + rhs
	case itemSUB:
		return lhs - rhs
	case itemMUL:
		return lhs * rhs
	case itemDIV:
		return lhs / rhs
	case itemPOW:
		return math.Pow(lhs, rhs)
	case itemMOD:
		return math.Mod(lhs, rhs)
	case itemEQL:
		return btos(lhs == rhs)
	case itemNEQ:
		return btos(lhs != rhs)
	case itemGTR:
		return btos(lhs > rhs)
	case itemLSS:
		return btos(lhs < rhs)
	case itemGTE:
		return btos(lhs >= rhs)
	case itemLTE:
		return btos(lhs <= rhs)
	}
	panic(fmt.Errorf("operator %q not allowed for Scalar operations", op))
}

// vectorElemBinop evaluates a binary operation between two Vector elements.
func vectorElemBinop(op ItemType, lhs, rhs float64) (float64, bool) {
	switch op {
	case itemADD:
		return lhs + rhs, true
	case itemSUB:
		return lhs - rhs, true
	case itemMUL:
		return lhs * rhs, true
	case itemDIV:
		return lhs / rhs, true
	case itemPOW:
		return math.Pow(lhs, rhs), true
	case itemMOD:
		return math.Mod(lhs, rhs), true
	case itemEQL:
		return lhs, lhs == rhs
	case itemNEQ:
		return lhs, lhs != rhs
	case itemGTR:
		return lhs, lhs > rhs
	case itemLSS:
		return lhs, lhs < rhs
	case itemGTE:
		return lhs, lhs >= rhs
	case itemLTE:
		return lhs, lhs <= rhs
	}
	panic(fmt.Errorf("operator %q not allowed for operations between Vectors", op))
}

// intersection returns the metric of common label/value pairs of two input metrics.
func intersection(ls1, ls2 labels.Labels) labels.Labels {
	res := make(labels.Labels, 0, 5)

	for _, l1 := range ls1 {
		for _, l2 := range ls2 {
			if l1.Name == l2.Name && l1.Value == l2.Value {
				res = append(res, l1)
				continue
			}
		}
	}
	return res
}

type groupedAggregation struct {
	labels           labels.Labels
	value            float64
	valuesSquaredSum float64
	groupCount       int
	heap             vectorByValueHeap
	reverseHeap      vectorByReverseValueHeap
}

// aggregation evaluates an aggregation operation on a Vector.
func (ev *evaluator) aggregation(op ItemType, grouping []string, without bool, param interface{}, vec Vector, enh *EvalNodeHelper) Vector {

	result := map[uint64]*groupedAggregation{}
	var k int64
	if op == itemTopK || op == itemBottomK {
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
	if op == itemQuantile {
		q = param.(float64)
	}
	var valueLabel string
	if op == itemCountValues {
		valueLabel = param.(string)
		if !without {
			grouping = append(grouping, valueLabel)
		}
	}

	for _, s := range vec {
		lb := labels.NewBuilder(s.Metric)

		if without {
			lb.Del(grouping...)
			lb.Del(labels.MetricName)
		}
		if op == itemCountValues {
			lb.Set(valueLabel, strconv.FormatFloat(s.V, 'f', -1, 64))
		}

		var (
			groupingKey uint64
			metric      = lb.Labels()
		)
		if without {
			groupingKey = metric.Hash()
		} else {
			groupingKey = hashForLabels(metric, grouping...)
		}

		group, ok := result[groupingKey]
		// Add a new group if it doesn't exist.
		if !ok {
			var m labels.Labels

			if without {
				m = metric
			} else {
				m = make(labels.Labels, 0, len(grouping))
				for _, l := range metric {
					for _, n := range grouping {
						if l.Name == n {
							m = append(m, labels.Label{Name: n, Value: l.Value})
							break
						}
					}
				}
				sort.Sort(m)
			}
			result[groupingKey] = &groupedAggregation{
				labels:           m,
				value:            s.V,
				valuesSquaredSum: s.V * s.V,
				groupCount:       1,
			}
			input_vec_len := int64(len(vec))
			result_size := k
			if k > input_vec_len {
				result_size = input_vec_len
			}
			if op == itemTopK || op == itemQuantile {
				result[groupingKey].heap = make(vectorByValueHeap, 0, result_size)
				heap.Push(&result[groupingKey].heap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			} else if op == itemBottomK {
				result[groupingKey].reverseHeap = make(vectorByReverseValueHeap, 0, result_size)
				heap.Push(&result[groupingKey].reverseHeap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}
			continue
		}

		switch op {
		case itemSum:
			group.value += s.V

		case itemAvg:
			group.value += s.V
			group.groupCount++

		case itemMax:
			if group.value < s.V || math.IsNaN(group.value) {
				group.value = s.V
			}

		case itemMin:
			if group.value > s.V || math.IsNaN(group.value) {
				group.value = s.V
			}

		case itemCount, itemCountValues:
			group.groupCount++

		case itemStdvar, itemStddev:
			group.value += s.V
			group.valuesSquaredSum += s.V * s.V
			group.groupCount++

		case itemTopK:
			if int64(len(group.heap)) < k || group.heap[0].V < s.V || math.IsNaN(group.heap[0].V) {
				if int64(len(group.heap)) == k {
					heap.Pop(&group.heap)
				}
				heap.Push(&group.heap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}

		case itemBottomK:
			if int64(len(group.reverseHeap)) < k || group.reverseHeap[0].V > s.V || math.IsNaN(group.reverseHeap[0].V) {
				if int64(len(group.reverseHeap)) == k {
					heap.Pop(&group.reverseHeap)
				}
				heap.Push(&group.reverseHeap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}

		case itemQuantile:
			group.heap = append(group.heap, s)

		default:
			panic(fmt.Errorf("expected aggregation operator but got %q", op))
		}
	}

	// Construct the result Vector from the aggregated groups.
	for _, aggr := range result {
		switch op {
		case itemAvg:
			aggr.value = aggr.value / float64(aggr.groupCount)

		case itemCount, itemCountValues:
			aggr.value = float64(aggr.groupCount)

		case itemStdvar:
			avg := aggr.value / float64(aggr.groupCount)
			aggr.value = aggr.valuesSquaredSum/float64(aggr.groupCount) - avg*avg

		case itemStddev:
			avg := aggr.value / float64(aggr.groupCount)
			aggr.value = math.Sqrt(aggr.valuesSquaredSum/float64(aggr.groupCount) - avg*avg)

		case itemTopK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.heap))
			for _, v := range aggr.heap {
				enh.out = append(enh.out, Sample{
					Metric: v.Metric,
					Point:  Point{V: v.V},
				})
			}
			continue // Bypass default append.

		case itemBottomK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.reverseHeap))
			for _, v := range aggr.reverseHeap {
				enh.out = append(enh.out, Sample{
					Metric: v.Metric,
					Point:  Point{V: v.V},
				})
			}
			continue // Bypass default append.

		case itemQuantile:
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
	case itemADD, itemSUB, itemDIV, itemMUL, itemMOD:
		return true
	default:
		return false
	}
}

// LookbackDelta determines the time since the last sample after which a time
// series is considered stale.
var LookbackDelta = 5 * time.Minute

// A queryGate controls the maximum number of concurrently running and waiting queries.
type queryGate struct {
	ch chan struct{}
}

// newQueryGate returns a query gate that limits the number of queries
// being concurrently executed.
func newQueryGate(length int) *queryGate {
	return &queryGate{
		ch: make(chan struct{}, length),
	}
}

// Start blocks until the gate has a free spot or the context is done.
func (g *queryGate) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return contextDone(ctx, "query queue")
	case g.ch <- struct{}{}:
		return nil
	}
}

// Done releases a single spot in the gate.
func (g *queryGate) Done() {
	select {
	case <-g.ch:
	default:
		panic("engine.queryGate.Done: more operations done than started")
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
