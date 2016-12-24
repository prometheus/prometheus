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
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fabxc/tsdb"
	"github.com/fabxc/tsdb/labels"
	"github.com/prometheus/common/log"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/util/stats"
)

const (
	// The largest SampleValue that can be converted to an int64 without overflow.
	maxInt64 = 9223372036854774784
	// The smallest SampleValue that can be converted to an int64 without underflow.
	minInt64 = -9223372036854775808

	// MetricNameLabel is the name of the label containing the metric name.
	MetricNameLabel = "__name__"
)

// convertibleToInt64 returns true if v does not over-/underflow an int64.
func convertibleToInt64(v float64) bool {
	return v <= maxInt64 && v >= minInt64
}

// Value is a generic interface for values resulting from a query evaluation.
type Value interface {
	Type() ValueType
	String() string
}

func (Matrix) Type() ValueType    { return ValueTypeMatrix }
func (Vector) Type() ValueType    { return ValueTypeVector }
func (Scalar) Type() ValueType    { return ValueTypeScalar }
func (stringVal) Type() ValueType { return ValueTypeString }

// ValueType describes a type of a value.
type ValueType string

// The valid value types.
const (
	ValueTypeNone   = "none"
	ValueTypeVector = "Vector"
	ValueTypeScalar = "Scalar"
	ValueTypeMatrix = "Matrix"
	ValueTypeString = "string"
)

type stringVal struct {
	s string
	t int64
}

func (s stringVal) String() string {
	return s.s
}

// Scalar is a data point that's explicitly not associated with a metric.
type Scalar struct {
	T int64
	V float64
}

func (s Scalar) String() string {
	return ""
}

// sampleStream is a stream of Values belonging to an attached COWMetric.
type sampleStream struct {
	Metric labels.Labels
	Values []Point
}

func (s sampleStream) String() string {
	return ""
}

// Point represents a single data point for a given timestamp.
type Point struct {
	T int64
	V float64
}

func (s Point) String() string {
	return ""
}

// sample is a single sample belonging to a COWMetric.
type sample struct {
	Point

	Metric labels.Labels
}

func (s sample) String() string {
	return ""
}

// Vector is basically only an alias for model.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type Vector []sample

func (vec Vector) String() string {
	entries := make([]string, len(vec))
	for i, s := range vec {
		entries[i] = s.String()
	}
	return strings.Join(entries, "\n")
}

// Matrix is a slice of SampleStreams that implements sort.Interface and
// has a String method.
type Matrix []sampleStream

func (m Matrix) String() string {
	// TODO(fabxc): sort, or can we rely on order from the querier?
	strs := make([]string, len(m))

	for i, ss := range m {
		strs[i] = ss.String()
	}

	return strings.Join(strs, "\n")
}

// Result holds the resulting value of an execution or an error
// if any occurred.
type Result struct {
	Err   error
	Value Value
}

// Vector returns a Vector if the result value is one. An error is returned if
// the result was an error or the result value is not a Vector.
func (r *Result) Vector() (Vector, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Vector)
	if !ok {
		return nil, fmt.Errorf("query result is not a Vector")
	}
	return v, nil
}

// Matrix returns a Matrix. An error is returned if
// the result was an error or the result value is not a Matrix.
func (r *Result) Matrix() (Matrix, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Matrix)
	if !ok {
		return nil, fmt.Errorf("query result is not a range Vector")
	}
	return v, nil
}

// Scalar returns a Scalar value. An error is returned if
// the result was an error or the result value is not a Scalar.
func (r *Result) Scalar() (Scalar, error) {
	if r.Err != nil {
		return Scalar{}, r.Err
	}
	v, ok := r.Value.(Scalar)
	if !ok {
		return Scalar{}, fmt.Errorf("query result is not a Scalar")
	}
	return v, nil
}

func (r *Result) String() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	if r.Value == nil {
		return ""
	}
	return r.Value.String()
}

type (
	// ErrQueryTimeout is returned if a query timed out during processing.
	ErrQueryTimeout string
	// ErrQueryCanceled is returned if a query was canceled during processing.
	ErrQueryCanceled string
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
	// The original query string.
	q string
	// Statement of the parsed query.
	stmt Statement
	// Timer stats for the query execution.
	stats *stats.TimerGroup
	// Cancelation function for the query.
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
	// A Querier constructor against an underlying storage.
	queryable Queryable
	// The gate limiting the maximum number of concurrent and waiting queries.
	gate    *queryGate
	options *EngineOptions
}

// Queryable allows opening a storage querier.
type Queryable interface {
	Querier(mint, maxt int64) (tsdb.Querier, error)
}

// NewEngine returns a new engine.
func NewEngine(queryable Queryable, o *EngineOptions) *Engine {
	if o == nil {
		o = DefaultEngineOptions
	}
	return &Engine{
		queryable: queryable,
		gate:      newQueryGate(o.MaxConcurrentQueries),
		options:   o,
	}
}

// EngineOptions contains configuration parameters for an Engine.
type EngineOptions struct {
	MaxConcurrentQueries int
	Timeout              time.Duration
}

// DefaultEngineOptions are the default engine options.
var DefaultEngineOptions = &EngineOptions{
	MaxConcurrentQueries: 20,
	Timeout:              2 * time.Minute,
}

// NewInstantQuery returns an evaluation query for the given expression at the given time.
func (ng *Engine) NewInstantQuery(qs string, ts time.Time) (Query, error) {
	expr, err := ParseExpr(qs)
	if err != nil {
		return nil, err
	}
	qry := ng.newQuery(expr, ts, ts, 0)
	qry.q = qs

	return qry, nil
}

// NewRangeQuery returns an evaluation query for the given time range and with
// the resolution set by the interval.
func (ng *Engine) NewRangeQuery(qs string, start, end time.Time, interval time.Duration) (Query, error) {
	expr, err := ParseExpr(qs)
	if err != nil {
		return nil, err
	}
	if expr.Type() != ValueTypeVector && expr.Type() != ValueTypeScalar {
		return nil, fmt.Errorf("invalid expression type %q for range query, must be Scalar or instant Vector", documentedType(expr.Type()))
	}
	qry := ng.newQuery(expr, start, end, interval)
	qry.q = qs

	return qry, nil
}

func (ng *Engine) newQuery(expr Expr, start, end time.Time, interval time.Duration) *query {
	es := &EvalStmt{
		Expr:     expr,
		Start:    start,
		End:      end,
		Interval: interval,
	}
	qry := &query{
		stmt:  es,
		ng:    ng,
		stats: stats.NewTimerGroup(),
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
	ctx, cancel := context.WithTimeout(ctx, ng.options.Timeout)
	q.cancel = cancel

	queueTimer := q.stats.GetTimer(stats.ExecQueueTime).Start()

	if err := ng.gate.Start(ctx); err != nil {
		return nil, err
	}
	defer ng.gate.Done()

	queueTimer.Stop()

	// Cancel when execution is done or an error was raised.
	defer q.cancel()

	const env = "query execution"

	evalTimer := q.stats.GetTimer(stats.TotalEvalTime).Start()
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
	querier, err := ng.populateIterators(ctx, s)
	prepareTimer.Stop()
	if err != nil {
		return nil, err
	}
	defer querier.Close()

	evalTimer := query.stats.GetTimer(stats.InnerEvalTime).Start()
	// Instant evaluation.
	if s.Start == s.End && s.Interval == 0 {
		evaluator := &evaluator{
			Timestamp: timeMilliseconds(s.Start),
			ctx:       ctx,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		evalTimer.Stop()
		return val, nil
	}
	numSteps := int(s.End.Sub(s.Start) / s.Interval)

	// Range evaluation.
	sampleStreams := map[uint64]sampleStream{}
	for ts := s.Start; !ts.After(s.End); ts = ts.Add(s.Interval) {

		if err := contextDone(ctx, "range evaluation"); err != nil {
			return nil, err
		}

		evaluator := &evaluator{
			Timestamp: timeMilliseconds(ts),
			ctx:       ctx,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		switch v := val.(type) {
		case Scalar:
			// As the expression type does not change we can safely default to 0
			// as the fingerprint for Scalar expressions.
			ss, ok := sampleStreams[0]
			if !ok {
				ss = sampleStream{Values: make([]Point, 0, numSteps)}
				sampleStreams[0] = ss
			}
			ss.Values = append(ss.Values, Point(v))
		case Vector:
			for _, sample := range v {
				h := sample.Metric.Hash()
				ss, ok := sampleStreams[h]
				if !ok {
					ss = sampleStream{
						Metric: sample.Metric,
						Values: make([]Point, 0, numSteps),
					}
					sampleStreams[h] = ss
				}
				ss.Values = append(ss.Values, sample.Point)
			}
		default:
			panic(fmt.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
		}
	}
	evalTimer.Stop()

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	appendTimer := query.stats.GetTimer(stats.ResultAppendTime).Start()
	mat := Matrix{}
	for _, ss := range sampleStreams {
		mat = append(mat, ss)
	}
	appendTimer.Stop()

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	// Turn Matrix type with protected metric into model.Matrix.
	resMatrix := mat

	// TODO(fabxc): order ensured by storage?
	// sortTimer := query.stats.GetTimer(stats.ResultSortTime).Start()
	// sort.Sort(resMatrix)
	// sortTimer.Stop()

	return resMatrix, nil
}

func (ng *Engine) populateIterators(ctx context.Context, s *EvalStmt) (tsdb.Querier, error) {
	var maxOffset time.Duration

	Inspect(s.Expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			if n.Offset > maxOffset {
				maxOffset = n.Offset + StalenessDelta
			}
		case *MatrixSelector:
			if n.Offset > maxOffset {
				maxOffset = n.Offset + n.Range
			}
		}
		return true
	})

	mint := s.Start.Add(-maxOffset)

	querier, err := ng.queryable.Querier(timeMilliseconds(mint), timeMilliseconds(s.End))
	if err != nil {
		return nil, err
	}

	Inspect(s.Expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			sel := make(labels.Selector, 0, len(n.LabelMatchers))
			for _, m := range n.LabelMatchers {
				sel = append(sel, m.matcher())
			}

			n.series, err = expandSeriesSet(querier.Select(sel...))
			if err != nil {
				return false
			}
			for _, s := range n.series {
				it := tsdb.NewBuffer(s.Iterator(), durationMilliseconds(StalenessDelta))
				n.iterators = append(n.iterators, it)
			}
		case *MatrixSelector:
			sel := make(labels.Selector, 0, len(n.LabelMatchers))
			for _, m := range n.LabelMatchers {
				sel = append(sel, m.matcher())
			}

			n.series, err = expandSeriesSet(querier.Select(sel...))
			if err != nil {
				return false
			}
			for _, s := range n.series {
				it := tsdb.NewBuffer(s.Iterator(), durationMilliseconds(n.Range))
				n.iterators = append(n.iterators, it)
			}
		}
		return true
	})
	return querier, err
}

func expandSeriesSet(it tsdb.SeriesSet) (res []tsdb.Series, err error) {
	for it.Next() {
		res = append(res, it.Series())
	}
	return res, it.Err()
}

// An evaluator evaluates given expressions at a fixed timestamp. It is attached to an
// engine through which it connects to a querier and reports errors. On timeout or
// cancellation of its context it terminates.
type evaluator struct {
	ctx context.Context

	Timestamp int64 // time in milliseconds
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
	if e != nil {
		if _, ok := e.(runtime.Error); ok {
			// Print the stack trace but do not inhibit the running application.
			buf := make([]byte, 64<<10)
			buf = buf[:runtime.Stack(buf, false)]

			log.Errorf("parser panic: %v\n%s", e, buf)
			*errp = fmt.Errorf("unexpected error")
		} else {
			*errp = e.(error)
		}
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
func (ev *evaluator) evalString(e Expr) stringVal {
	val := ev.eval(e)
	sv, ok := val.(stringVal)
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
	return ev.eval(expr), nil
}

// eval evaluates the given expression as the given AST expression node requires.
func (ev *evaluator) eval(expr Expr) Value {
	// This is the top-level evaluation method.
	// Thus, we check for timeout/cancelation here.
	if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
		ev.error(err)
	}

	switch e := expr.(type) {
	case *AggregateExpr:
		Vector := ev.evalVector(e.Expr)
		return ev.aggregation(e.Op, e.Grouping, e.Without, e.KeepCommonLabels, e.Param, Vector)

	case *BinaryExpr:
		lhs := ev.evalOneOf(e.LHS, ValueTypeScalar, ValueTypeVector)
		rhs := ev.evalOneOf(e.RHS, ValueTypeScalar, ValueTypeVector)

		switch lt, rt := lhs.Type(), rhs.Type(); {
		case lt == ValueTypeScalar && rt == ValueTypeScalar:
			return Scalar{
				V: ScalarBinop(e.Op, lhs.(Scalar).V, rhs.(Scalar).V),
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
			return ev.VectorScalarBinop(e.Op, lhs.(Vector), rhs.(Scalar), false, e.ReturnBool)

		case lt == ValueTypeScalar && rt == ValueTypeVector:
			return ev.VectorScalarBinop(e.Op, rhs.(Vector), lhs.(Scalar), true, e.ReturnBool)
		}

	case *Call:
		return e.Func.Call(ev, e.Args)

	case *MatrixSelector:
		return ev.MatrixSelector(e)

	case *NumberLiteral:
		return Scalar{V: e.Val, T: ev.Timestamp}

	case *ParenExpr:
		return ev.eval(e.Expr)

	case *StringLiteral:
		return stringVal{s: e.Val, t: ev.Timestamp}

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
		return ev.VectorSelector(e)
	}
	panic(fmt.Errorf("unhandled expression of type: %T", expr))
}

// VectorSelector evaluates a *VectorSelector expression.
func (ev *evaluator) VectorSelector(node *VectorSelector) Vector {
	var (
		ok      bool
		vec     = make(Vector, 0, len(node.series))
		refTime = ev.Timestamp - durationMilliseconds(node.Offset)
	)

	for i, it := range node.iterators {
		if !it.Seek(refTime) {
			if it.Err() != nil {
				ev.error(it.Err())
			}
			continue
		}
		t, v := it.Values()

		if t > refTime {
			t, v, ok = it.PeekBack()
			if !ok || t < refTime-durationMilliseconds(StalenessDelta) {
				continue
			}
		}

		vec = append(vec, sample{
			Metric: node.series[i].Labels(),
			Point:  Point{V: v, T: ev.Timestamp},
		})
	}
	return vec
}

// MatrixSelector evaluates a *MatrixSelector expression.
func (ev *evaluator) MatrixSelector(node *MatrixSelector) Matrix {
	var (
		offset = durationMilliseconds(node.Offset)
		maxt   = ev.Timestamp - offset
		mint   = maxt - durationMilliseconds(node.Range)
		Matrix = make(Matrix, 0, len(node.series))
	)

	for i, it := range node.iterators {
		ss := sampleStream{
			Metric: node.series[i].Labels(),
			Values: make([]Point, 0, 16),
		}

		if !it.Seek(maxt) {
			if it.Err() != nil {
				ev.error(it.Err())
			}
			continue
		}

		buf := it.Buffer()
		for buf.Next() {
			t, v := buf.Values()
			// Values in the buffer are guaranteed to be smaller than maxt.
			if t >= mint {
				ss.Values = append(ss.Values, Point{T: t + offset, V: v})
			}
		}
		// The seeked sample might also be in the range.
		t, v := it.Values()
		if t == maxt {
			ss.Values = append(ss.Values, Point{T: t + offset, V: v})
		}

		Matrix = append(Matrix, ss)
	}
	return Matrix
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
func (ev *evaluator) VectorBinop(op itemType, lhs, rhs Vector, matching *VectorMatching, returnBool bool) Vector {
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
	rightSigs := map[uint64]sample{}

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
		value, keep := VectorElemBinop(op, vl, vr)
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

		result = append(result, sample{
			Metric: metric,
			Point:  Point{V: value, T: ev.Timestamp},
		})
	}
	return result
}

func hashWithoutLabels(lset labels.Labels, names ...string) uint64 {
	cm := make(labels.Labels, 0, len(lset)-len(names)-1)

Outer:
	for _, l := range lset {
		for _, n := range names {
			if n == l.Name {
				continue Outer
			}
		}
		if l.Name == MetricNameLabel {
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
func resultMetric(lhs, rhs labels.Labels, op itemType, matching *VectorMatching) labels.Labels {
	// del and add hold modifications to the LHS input metric.
	// Deletions are applied first.
	del := make([]string, 0, 16)
	add := make([]labels.Label, 0, 16)

	if shouldDropMetricName(op) {
		del = append(del, MetricNameLabel)
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
				del = append(del, l.Name)
			}
		} else {
			del = append(del, matching.MatchingLabels...)
		}
	}
	for _, ln := range matching.Include {
		// We always want to delete the include label on the LHS
		// before adding an included one or not.
		del = append(del, ln)
		// Included labels from the `group_x` modifier are taken from the "one"-side.
		if v := rhs.Get(ln); v != "" {
			add = append(add, labels.Label{Name: ln, Value: v})
		}
	}

	return modifiedLabels(lhs, del, add)
}

func modifiedLabels(lhs labels.Labels, del []string, add []labels.Label) labels.Labels {
	res := make(labels.Labels, 0, len(lhs)+len(add)-len(del))
Outer:
	for _, l := range lhs {
		for _, n := range del {
			if l.Name == n {
				continue Outer
			}
		}
		res = append(res, l)
	}
	res = append(res, add...)
	sort.Sort(res)

	return res
}

// VectorScalarBinop evaluates a binary operation between a Vector and a Scalar.
func (ev *evaluator) VectorScalarBinop(op itemType, lhs Vector, rhs Scalar, swap, returnBool bool) Vector {
	vec := make(Vector, 0, len(lhs))

	for _, lhsSample := range lhs {
		lv, rv := lhsSample.V, rhs.V
		// lhs always contains the Vector. If the original position was different
		// swap for calculating the value.
		if swap {
			lv, rv = rv, lv
		}
		value, keep := VectorElemBinop(op, lv, rv)
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
			lhsSample.Metric = copyLabels(lhsSample.Metric, shouldDropMetricName(op))

			vec = append(vec, lhsSample)
		}
	}
	return vec
}

func copyLabels(metric labels.Labels, withName bool) labels.Labels {
	if withName {
		cm := make(labels.Labels, len(metric))
		copy(cm, metric)
		return cm
	}
	cm := make(labels.Labels, 0, len(metric)-1)
	for _, l := range metric {
		if l.Name != MetricNameLabel {
			cm = append(cm, l)
		}
	}
	return cm
}

// ScalarBinop evaluates a binary operation between two Scalars.
func ScalarBinop(op itemType, lhs, rhs float64) float64 {
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

// VectorElemBinop evaluates a binary operation between two Vector elements.
func VectorElemBinop(op itemType, lhs, rhs float64) (float64, bool) {
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
	heap             VectorByValueHeap
	reverseHeap      VectorByReverseValueHeap
}

// aggregation evaluates an aggregation operation on a Vector.
func (ev *evaluator) aggregation(op itemType, grouping []string, without bool, keepCommon bool, param Expr, vec Vector) Vector {

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
		valueLabel = ev.evalString(param).s
		if !without {
			grouping = append(grouping, valueLabel)
		}
	}

	for _, s := range vec {
		var (
			del []string
			add []labels.Label
		)
		if without {
			del = append(grouping, MetricNameLabel)
		}
		if op == itemCountValues {
			del = append(del, valueLabel)
			add = append(add, labels.Label{Name: valueLabel, Value: fmt.Sprintf("%f", s.V)})
		}

		var (
			metric      = modifiedLabels(s.Metric, del, add)
			groupingKey = metric.Hash()
		)
		group, ok := result[groupingKey]
		// Add a new group if it doesn't exist.
		if !ok {
			var m labels.Labels
			if keepCommon {
				m = copyLabels(metric, false)
			} else if without {
				m = metric
			} else {
				m = make(labels.Labels, 0, len(grouping))
				for _, l := range s.Metric {
					for _, n := range grouping {
						if l.Name == n {
							m = append(m, labels.Label{Name: n, Value: l.Value})
							break
						}
					}
				}
			}
			result[groupingKey] = &groupedAggregation{
				labels:           m,
				value:            s.V,
				valuesSquaredSum: s.V * s.V,
				groupCount:       1,
			}
			if op == itemTopK || op == itemQuantile {
				result[groupingKey].heap = make(VectorByValueHeap, 0, k)
				heap.Push(&result[groupingKey].heap, &sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			} else if op == itemBottomK {
				result[groupingKey].reverseHeap = make(VectorByReverseValueHeap, 0, k)
				heap.Push(&result[groupingKey].reverseHeap, &sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}
			continue
		}
		// Add the sample to the existing group.
		if keepCommon {
			group.labels = intersection(group.labels, s.Metric)
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
				heap.Push(&group.heap, &sample{
					Point:  Point{V: s.V},
					Metric: s.Metric,
				})
			}

		case itemBottomK:
			if int64(len(group.reverseHeap)) < k || group.reverseHeap[0].V > s.V || math.IsNaN(float64(group.reverseHeap[0].V)) {
				if int64(len(group.reverseHeap)) == k {
					heap.Pop(&group.reverseHeap)
				}
				heap.Push(&group.reverseHeap, &sample{
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
				resultVector = append(resultVector, sample{
					Metric: v.Metric,
					Point:  Point{V: v.V, T: ev.Timestamp},
				})
			}
			continue // Bypass default append.

		case itemBottomK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.reverseHeap))
			for _, v := range aggr.reverseHeap {
				resultVector = append(resultVector, sample{
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

		resultVector = append(resultVector, sample{
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
func shouldDropMetricName(op itemType) bool {
	switch op {
	case itemADD, itemSUB, itemDIV, itemMUL, itemMOD:
		return true
	default:
		return false
	}
}

// StalenessDelta determines the time since the last sample after which a time
// series is considered stale.
var StalenessDelta = 5 * time.Minute

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
	case "Vector":
		return "instant Vector"
	case "Matrix":
		return "range Vector"
	default:
		return string(t)
	}
}
