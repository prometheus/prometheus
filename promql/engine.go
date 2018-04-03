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
	queryPrepareTime     prometheus.Summary
	queryInnerEval       prometheus.Summary
	queryResultAppend    prometheus.Summary
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
	// Exec processes the query and
	Exec(ctx context.Context) *Result
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
		queryResultAppend: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "result_append"},
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
			metrics.queryResultAppend,
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
	querier, err := ng.populateIterators(ctx, query.queryable, s)
	prepareTimer.Stop()
	ng.metrics.queryPrepareTime.Observe(prepareTimer.ElapsedTime().Seconds())

	// XXX(fabxc): the querier returned by populateIterators might be instantiated
	// we must not return without closing irrespective of the error.
	// TODO: make this semantically saner.
	if querier != nil {
		defer querier.Close()
	}

	if err != nil {
		return nil, err
	}

	evalTimer := query.stats.GetTimer(stats.InnerEvalTime).Start()
	// Instant evaluation.
	if s.Start == s.End && s.Interval == 0 {
		start := timeMilliseconds(s.Start)
		evaluator := &evaluator{
			Timestamp: start,
			ctx:       ctx,
			logger:    ng.logger,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		evalTimer.Stop()
		ng.metrics.queryInnerEval.Observe(evalTimer.ElapsedTime().Seconds())
		// Point might have a different timestamp, force it to the evaluation
		// timestamp as that is when we ran the evaluation.
		switch v := val.(type) {
		case Scalar:
			v.T = start
		case Vector:
			for i := range v {
				v[i].Point.T = start
			}
		}

		return val, nil
	}
	numSteps := int(s.End.Sub(s.Start) / s.Interval)

	// Range evaluation.
	Seriess := map[uint64]Series{}
	for ts := s.Start; !ts.After(s.End); ts = ts.Add(s.Interval) {

		if err := contextDone(ctx, "range evaluation"); err != nil {
			return nil, err
		}

		t := timeMilliseconds(ts)
		evaluator := &evaluator{
			Timestamp: t,
			ctx:       ctx,
			logger:    ng.logger,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		switch v := val.(type) {
		case Scalar:
			// As the expression type does not change we can safely default to 0
			// as the fingerprint for Scalar expressions.
			ss, ok := Seriess[0]
			if !ok {
				ss = Series{Points: make([]Point, 0, numSteps)}
				Seriess[0] = ss
			}
			ss.Points = append(ss.Points, Point{V: v.V, T: t})
			Seriess[0] = ss
		case Vector:
			for _, sample := range v {
				h := sample.Metric.Hash()
				ss, ok := Seriess[h]
				if !ok {
					ss = Series{
						Metric: sample.Metric,
						Points: make([]Point, 0, numSteps),
					}
					Seriess[h] = ss
				}
				sample.Point.T = t
				ss.Points = append(ss.Points, sample.Point)
				Seriess[h] = ss
			}
		default:
			panic(fmt.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
		}
	}
	evalTimer.Stop()
	ng.metrics.queryInnerEval.Observe(evalTimer.ElapsedTime().Seconds())

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	appendTimer := query.stats.GetTimer(stats.ResultAppendTime).Start()
	mat := Matrix{}
	for _, ss := range Seriess {
		mat = append(mat, ss)
	}
	appendTimer.Stop()
	ng.metrics.queryResultAppend.Observe(appendTimer.ElapsedTime().Seconds())

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

func (ng *Engine) populateIterators(ctx context.Context, q storage.Queryable, s *EvalStmt) (storage.Querier, error) {
	var maxOffset time.Duration
	Inspect(s.Expr, func(node Node, _ []Node) bool {
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
		return true
	})

	mint := s.Start.Add(-maxOffset)

	querier, err := q.Querier(ctx, timestamp.FromTime(mint), timestamp.FromTime(s.End))
	if err != nil {
		return nil, err
	}

	Inspect(s.Expr, func(node Node, path []Node) bool {
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
				return false
			}
			n.series, err = expandSeriesSet(set)
			if err != nil {
				// TODO(fabxc): use multi-error.
				level.Error(ng.logger).Log("msg", "error expanding series set", "err", err)
				return false
			}
			for _, s := range n.series {
				it := storage.NewBuffer(s.Iterator(), durationMilliseconds(LookbackDelta))
				n.iterators = append(n.iterators, it)
			}

		case *MatrixSelector:
			params.Func = extractFuncFromPath(path)

			set, err = querier.Select(params, n.LabelMatchers...)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error selecting series set", "err", err)
				return false
			}
			n.series, err = expandSeriesSet(set)
			if err != nil {
				level.Error(ng.logger).Log("msg", "error expanding series set", "err", err)
				return false
			}
			for _, s := range n.series {
				it := storage.NewBuffer(s.Iterator(), durationMilliseconds(n.Range))
				n.iterators = append(n.iterators, it)
			}
		}
		return true
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

// An evaluator evaluates given expressions at a fixed timestamp. It is attached to an
// engine through which it connects to a querier and reports errors. On timeout or
// cancellation of its context it terminates.
type evaluator struct {
	ctx context.Context

	Timestamp int64 // time in milliseconds

	finalizers []func()

	logger log.Logger
}

func (ev *evaluator) close() {
	for _, f := range ev.finalizers {
		f()
	}
}

// fatalf causes a panic with the input formatted into an error.
func (ev *evaluator) errorf(format string, args ...interface{}) {
	ev.error(fmt.Errorf(format, args...))
}

// fatal causes a panic with the given error.
func (ev *evaluator) error(err error) {
	panic(err)
}

// recover is the handler that turns panics into returns from the top level of evaluation.
func (ev *evaluator) recover(errp *error) {
	e := recover()
	if e == nil {
		return
	}
	if _, ok := e.(runtime.Error); ok {
		// Print the stack trace but do not inhibit the running application.
		buf := make([]byte, 64<<10)
		buf = buf[:runtime.Stack(buf, false)]

		level.Error(ev.logger).Log("msg", "runtime panic in parser", "err", e, "stacktrace", string(buf))
		*errp = fmt.Errorf("unexpected error")
	} else {
		*errp = e.(error)
	}
}

// evalScalar attempts to evaluate e to a Scalar value and errors otherwise.
func (ev *evaluator) evalScalar(e Expr) Scalar {
	val := ev.eval(e)
	sv, ok := val.(Scalar)
	if !ok {
		ev.errorf("expected Scalar but got %s", documentedType(val.Type()))
	}
	return sv
}

// evalVector attempts to evaluate e to a Vector value and errors otherwise.
func (ev *evaluator) evalVector(e Expr) Vector {
	val := ev.eval(e)
	vec, ok := val.(Vector)
	if !ok {
		ev.errorf("expected instant Vector but got %s", documentedType(val.Type()))
	}
	return vec
}

// evalInt attempts to evaluate e into an integer and errors otherwise.
func (ev *evaluator) evalInt(e Expr) int64 {
	sc := ev.evalScalar(e)
	if !convertibleToInt64(sc.V) {
		ev.errorf("Scalar value %v overflows int64", sc.V)
	}
	return int64(sc.V)
}

// evalFloat attempts to evaluate e into a float and errors otherwise.
func (ev *evaluator) evalFloat(e Expr) float64 {
	sc := ev.evalScalar(e)
	return float64(sc.V)
}

// evalMatrix attempts to evaluate e into a Matrix and errors otherwise.
// The error message uses the term "range Vector" to match the user facing
// documentation.
func (ev *evaluator) evalMatrix(e Expr) Matrix {
	val := ev.eval(e)
	mat, ok := val.(Matrix)
	if !ok {
		ev.errorf("expected range Vector but got %s", documentedType(val.Type()))
	}
	return mat
}

// evalString attempts to evaluate e to a string value and errors otherwise.
func (ev *evaluator) evalString(e Expr) String {
	val := ev.eval(e)
	sv, ok := val.(String)
	if !ok {
		ev.errorf("expected string but got %s", documentedType(val.Type()))
	}
	return sv
}

// evalOneOf evaluates e and errors unless the result is of one of the given types.
func (ev *evaluator) evalOneOf(e Expr, t1, t2 ValueType) Value {
	val := ev.eval(e)
	if val.Type() != t1 && val.Type() != t2 {
		ev.errorf("expected %s or %s but got %s", documentedType(t1), documentedType(t2), documentedType(val.Type()))
	}
	return val
}

func (ev *evaluator) Eval(expr Expr) (v Value, err error) {
	defer ev.recover(&err)
	defer ev.close()
	return ev.eval(expr), nil
}

// eval evaluates the given expression as the given AST expression node requires.
func (ev *evaluator) eval(expr Expr) Value {
	// This is the top-level evaluation method.
	// Thus, we check for timeout/cancellation here.
	if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
		ev.error(err)
	}

	switch e := expr.(type) {
	case *AggregateExpr:
		Vector := ev.evalVector(e.Expr)
		return ev.aggregation(e.Op, e.Grouping, e.Without, e.Param, Vector)

	case *BinaryExpr:
		lhs := ev.evalOneOf(e.LHS, ValueTypeScalar, ValueTypeVector)
		rhs := ev.evalOneOf(e.RHS, ValueTypeScalar, ValueTypeVector)

		switch lt, rt := lhs.Type(), rhs.Type(); {
		case lt == ValueTypeScalar && rt == ValueTypeScalar:
			return Scalar{
				V: scalarBinop(e.Op, lhs.(Scalar).V, rhs.(Scalar).V),
				T: ev.Timestamp,
			}

		case lt == ValueTypeVector && rt == ValueTypeVector:
			switch e.Op {
			case itemLAND:
				return ev.VectorAnd(lhs.(Vector), rhs.(Vector), e.VectorMatching)
			case itemLOR:
				return ev.VectorOr(lhs.(Vector), rhs.(Vector), e.VectorMatching)
			case itemLUnless:
				return ev.VectorUnless(lhs.(Vector), rhs.(Vector), e.VectorMatching)
			default:
				return ev.VectorBinop(e.Op, lhs.(Vector), rhs.(Vector), e.VectorMatching, e.ReturnBool)
			}
		case lt == ValueTypeVector && rt == ValueTypeScalar:
			return ev.VectorscalarBinop(e.Op, lhs.(Vector), rhs.(Scalar), false, e.ReturnBool)

		case lt == ValueTypeScalar && rt == ValueTypeVector:
			return ev.VectorscalarBinop(e.Op, rhs.(Vector), lhs.(Scalar), true, e.ReturnBool)
		}

	case *Call:
		return e.Func.Call(ev, e.Args)

	case *MatrixSelector:
		return ev.matrixSelector(e)

	case *NumberLiteral:
		return Scalar{V: e.Val, T: ev.Timestamp}

	case *ParenExpr:
		return ev.eval(e.Expr)

	case *StringLiteral:
		return String{V: e.Val, T: ev.Timestamp}

	case *UnaryExpr:
		se := ev.evalOneOf(e.Expr, ValueTypeScalar, ValueTypeVector)
		// Only + and - are possible operators.
		if e.Op == itemSUB {
			switch v := se.(type) {
			case Scalar:
				v.V = -v.V
			case Vector:
				for i, sv := range v {
					v[i].V = -sv.V
				}
			}
		}
		return se

	case *VectorSelector:
		return ev.vectorSelector(e)
	}
	panic(fmt.Errorf("unhandled expression of type: %T", expr))
}

// vectorSelector evaluates a *VectorSelector expression.
func (ev *evaluator) vectorSelector(node *VectorSelector) Vector {
	var (
		vec     = make(Vector, 0, len(node.series))
		refTime = ev.Timestamp - durationMilliseconds(node.Offset)
	)

	for i, it := range node.iterators {
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

		peek := 1
		if !ok || t > refTime {
			t, v, ok = it.PeekBack(peek)
			peek++
			if !ok || t < refTime-durationMilliseconds(LookbackDelta) {
				continue
			}
		}
		if value.IsStaleNaN(v) {
			continue
		}

		vec = append(vec, Sample{
			Metric: node.series[i].Labels(),
			Point:  Point{V: v, T: t},
		})
	}
	return vec
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

var matrixPool = sync.Pool{}

func getMatrix(sz int) Matrix {
	m := matrixPool.Get()
	if m != nil {
		return m.(Matrix)
	}
	return make(Matrix, 0, sz)
}

func putMatrix(m Matrix) {
	matrixPool.Put(m[:0])
}

// matrixSelector evaluates a *MatrixSelector expression.
func (ev *evaluator) matrixSelector(node *MatrixSelector) Matrix {
	var (
		offset = durationMilliseconds(node.Offset)
		maxt   = ev.Timestamp - offset
		mint   = maxt - durationMilliseconds(node.Range)
		matrix = getMatrix(len(node.series))
		// Write all points into a single slice to avoid lots of tiny allocations.
		allPoints = getPointSlice(5 * len(matrix))
	)

	ev.finalizers = append(ev.finalizers,
		func() { putPointSlice(allPoints) },
		func() { putMatrix(matrix) },
	)

	for i, it := range node.iterators {
		start := len(allPoints)

		ss := Series{
			Metric: node.series[i].Labels(),
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
				allPoints = append(allPoints, Point{T: t, V: v})
			}
		}
		// The seeked sample might also be in the range.
		if ok {
			t, v := it.Values()
			if t == maxt && !value.IsStaleNaN(v) {
				allPoints = append(allPoints, Point{T: t, V: v})
			}
		}

		ss.Points = allPoints[start:]

		if len(ss.Points) > 0 {
			matrix = append(matrix, ss)
		}
	}
	return matrix
}

func (ev *evaluator) VectorAnd(lhs, rhs Vector, matching *VectorMatching) Vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := signatureFunc(matching.On, matching.MatchingLabels...)

	var result Vector
	// The set of signatures for the right-hand side Vector.
	rightSigs := map[uint64]struct{}{}
	// Add all rhs samples to a map so we can easily find matches later.
	for _, rs := range rhs {
		rightSigs[sigf(rs.Metric)] = struct{}{}
	}

	for _, ls := range lhs {
		// If there's a matching entry in the right-hand side Vector, add the sample.
		if _, ok := rightSigs[sigf(ls.Metric)]; ok {
			result = append(result, ls)
		}
	}
	return result
}

func (ev *evaluator) VectorOr(lhs, rhs Vector, matching *VectorMatching) Vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := signatureFunc(matching.On, matching.MatchingLabels...)

	var result Vector
	leftSigs := map[uint64]struct{}{}
	// Add everything from the left-hand-side Vector.
	for _, ls := range lhs {
		leftSigs[sigf(ls.Metric)] = struct{}{}
		result = append(result, ls)
	}
	// Add all right-hand side elements which have not been added from the left-hand side.
	for _, rs := range rhs {
		if _, ok := leftSigs[sigf(rs.Metric)]; !ok {
			result = append(result, rs)
		}
	}
	return result
}

func (ev *evaluator) VectorUnless(lhs, rhs Vector, matching *VectorMatching) Vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := signatureFunc(matching.On, matching.MatchingLabels...)

	rightSigs := map[uint64]struct{}{}
	for _, rs := range rhs {
		rightSigs[sigf(rs.Metric)] = struct{}{}
	}

	var result Vector
	for _, ls := range lhs {
		if _, ok := rightSigs[sigf(ls.Metric)]; !ok {
			result = append(result, ls)
		}
	}
	return result
}

// VectorBinop evaluates a binary operation between two Vectors, excluding set operators.
func (ev *evaluator) VectorBinop(op ItemType, lhs, rhs Vector, matching *VectorMatching, returnBool bool) Vector {
	if matching.Card == CardManyToMany {
		panic("many-to-many only allowed for set operators")
	}
	var (
		result = Vector{}
		sigf   = signatureFunc(matching.On, matching.MatchingLabels...)
	)

	// The control flow below handles one-to-one or many-to-one matching.
	// For one-to-many, swap sidedness and account for the swap when calculating
	// values.
	if matching.Card == CardOneToMany {
		lhs, rhs = rhs, lhs
	}

	// All samples from the rhs hashed by the matching label/values.
	rightSigs := map[uint64]Sample{}

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
	matchedSigs := map[uint64]map[uint64]struct{}{}

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
		metric := resultMetric(ls.Metric, rs.Metric, op, matching)

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

		result = append(result, Sample{
			Metric: metric,
			Point:  Point{V: value, T: ev.Timestamp},
		})
	}
	return result
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
func resultMetric(lhs, rhs labels.Labels, op ItemType, matching *VectorMatching) labels.Labels {
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

	return lb.Labels()
}

// VectorscalarBinop evaluates a binary operation between a Vector and a Scalar.
func (ev *evaluator) VectorscalarBinop(op ItemType, lhs Vector, rhs Scalar, swap, returnBool bool) Vector {
	vec := make(Vector, 0, len(lhs))

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
				lhsSample.Metric = dropMetricName(lhsSample.Metric)
			}
			vec = append(vec, lhsSample)
		}
	}
	return vec
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
		return math.Pow(float64(lhs), float64(rhs))
	case itemMOD:
		return math.Mod(float64(lhs), float64(rhs))
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
		return math.Pow(float64(lhs), float64(rhs)), true
	case itemMOD:
		return math.Mod(float64(lhs), float64(rhs)), true
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
func (ev *evaluator) aggregation(op ItemType, grouping []string, without bool, param Expr, vec Vector) Vector {

	result := map[uint64]*groupedAggregation{}
	var k int64
	if op == itemTopK || op == itemBottomK {
		k = ev.evalInt(param)
		if k < 1 {
			return Vector{}
		}
	}
	var q float64
	if op == itemQuantile {
		q = ev.evalFloat(param)
	}
	var valueLabel string
	if op == itemCountValues {
		valueLabel = ev.evalString(param).V
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
			lb.Set(valueLabel, strconv.FormatFloat(float64(s.V), 'f', -1, 64))
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
			if op == itemTopK || op == itemQuantile {
				result[groupingKey].heap = make(vectorByValueHeap, 0, k)
				heap.Push(&result[groupingKey].heap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			} else if op == itemBottomK {
				result[groupingKey].reverseHeap = make(vectorByReverseValueHeap, 0, k)
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
			if group.value < s.V || math.IsNaN(float64(group.value)) {
				group.value = s.V
			}

		case itemMin:
			if group.value > s.V || math.IsNaN(float64(group.value)) {
				group.value = s.V
			}

		case itemCount, itemCountValues:
			group.groupCount++

		case itemStdvar, itemStddev:
			group.value += s.V
			group.valuesSquaredSum += s.V * s.V
			group.groupCount++

		case itemTopK:
			if int64(len(group.heap)) < k || group.heap[0].V < s.V || math.IsNaN(float64(group.heap[0].V)) {
				if int64(len(group.heap)) == k {
					heap.Pop(&group.heap)
				}
				heap.Push(&group.heap, &Sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}

		case itemBottomK:
			if int64(len(group.reverseHeap)) < k || group.reverseHeap[0].V > s.V || math.IsNaN(float64(group.reverseHeap[0].V)) {
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
	resultVector := make(Vector, 0, len(result))

	for _, aggr := range result {
		switch op {
		case itemAvg:
			aggr.value = aggr.value / float64(aggr.groupCount)

		case itemCount, itemCountValues:
			aggr.value = float64(aggr.groupCount)

		case itemStdvar:
			avg := float64(aggr.value) / float64(aggr.groupCount)
			aggr.value = float64(aggr.valuesSquaredSum)/float64(aggr.groupCount) - avg*avg

		case itemStddev:
			avg := float64(aggr.value) / float64(aggr.groupCount)
			aggr.value = math.Sqrt(float64(aggr.valuesSquaredSum)/float64(aggr.groupCount) - avg*avg)

		case itemTopK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.heap))
			for _, v := range aggr.heap {
				resultVector = append(resultVector, Sample{
					Metric: v.Metric,
					Point:  Point{V: v.V, T: ev.Timestamp},
				})
			}
			continue // Bypass default append.

		case itemBottomK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.reverseHeap))
			for _, v := range aggr.reverseHeap {
				resultVector = append(resultVector, Sample{
					Metric: v.Metric,
					Point:  Point{V: v.V, T: ev.Timestamp},
				})
			}
			continue // Bypass default append.

		case itemQuantile:
			aggr.value = quantile(q, aggr.heap)

		default:
			// For other aggregations, we already have the right value.
		}

		resultVector = append(resultVector, Sample{
			Metric: aggr.labels,
			Point:  Point{V: aggr.value, T: ev.Timestamp},
		})
	}
	return resultVector
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
