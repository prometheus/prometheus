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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/stats"
)

const (
	namespace = "prometheus"
	subsystem = "engine"

	// The largest SampleValue that can be converted to an int64 without overflow.
	maxInt64 model.SampleValue = 9223372036854774784
	// The smallest SampleValue that can be converted to an int64 without underflow.
	minInt64 model.SampleValue = -9223372036854775808
)

var (
	currentQueries = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "queries",
		Help:      "The current number of queries being executed or waiting.",
	})
	maxConcurrentQueries = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "queries_concurrent_max",
		Help:      "The max number of concurrent queries.",
	})
	queryPrepareTime = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "prepare_time"},
		},
	)
	queryInnerEval = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "inner_eval"},
		},
	)
	queryResultAppend = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "result_append"},
		},
	)
	queryResultSort = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "query_duration_seconds",
			Help:        "Query timings",
			ConstLabels: prometheus.Labels{"slice": "result_sort"},
		},
	)
)

func init() {
	prometheus.MustRegister(currentQueries)
	prometheus.MustRegister(maxConcurrentQueries)
	prometheus.MustRegister(queryPrepareTime)
	prometheus.MustRegister(queryInnerEval)
	prometheus.MustRegister(queryResultAppend)
	prometheus.MustRegister(queryResultSort)
}

// convertibleToInt64 returns true if v does not over-/underflow an int64.
func convertibleToInt64(v model.SampleValue) bool {
	return v <= maxInt64 && v >= minInt64
}

// sampleStream is a stream of Values belonging to an attached COWMetric.
type sampleStream struct {
	Metric metric.Metric
	Values []model.SamplePair
}

// sample is a single sample belonging to a COWMetric.
type sample struct {
	Metric    metric.Metric
	Value     model.SampleValue
	Timestamp model.Time
}

// vector is basically only an alias for model.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type vector []*sample

func (vector) Type() model.ValueType { return model.ValVector }
func (vec vector) String() string    { return vec.value().String() }

func (vec vector) value() model.Vector {
	val := make(model.Vector, len(vec))
	for i, s := range vec {
		val[i] = &model.Sample{
			Metric:    s.Metric.Copy().Metric,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return val
}

// matrix is a slice of SampleStreams that implements sort.Interface and
// has a String method.
type matrix []*sampleStream

func (matrix) Type() model.ValueType { return model.ValMatrix }
func (mat matrix) String() string    { return mat.value().String() }

func (mat matrix) value() model.Matrix {
	val := make(model.Matrix, len(mat))
	for i, ss := range mat {
		val[i] = &model.SampleStream{
			Metric: ss.Metric.Copy().Metric,
			Values: ss.Values,
		}
	}
	return val
}

// Result holds the resulting value of an execution or an error
// if any occurred.
type Result struct {
	Err   error
	Value model.Value
}

// Vector returns a vector if the result value is one. An error is returned if
// the result was an error or the result value is not a vector.
func (r *Result) Vector() (model.Vector, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("query result is not a vector")
	}
	return v, nil
}

// Matrix returns a matrix. An error is returned if
// the result was an error or the result value is not a matrix.
func (r *Result) Matrix() (model.Matrix, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("query result is not a range vector")
	}
	return v, nil
}

// Scalar returns a scalar value. An error is returned if
// the result was an error or the result value is not a scalar.
func (r *Result) Scalar() (*model.Scalar, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(*model.Scalar)
	if !ok {
		return nil, fmt.Errorf("query result is not a scalar")
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
	Querier() (local.Querier, error)
}

// NewEngine returns a new engine.
func NewEngine(queryable Queryable, o *EngineOptions) *Engine {
	if o == nil {
		o = DefaultEngineOptions
	}
	maxConcurrentQueries.Set(float64(o.MaxConcurrentQueries))
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
func (ng *Engine) NewInstantQuery(qs string, ts model.Time) (Query, error) {
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
func (ng *Engine) NewRangeQuery(qs string, start, end model.Time, interval time.Duration) (Query, error) {
	expr, err := ParseExpr(qs)
	if err != nil {
		return nil, err
	}
	if expr.Type() != model.ValVector && expr.Type() != model.ValScalar {
		return nil, fmt.Errorf("invalid expression type %q for range query, must be scalar or instant vector", documentedType(expr.Type()))
	}
	qry := ng.newQuery(expr, start, end, interval)
	qry.q = qs

	return qry, nil
}

func (ng *Engine) newQuery(expr Expr, start, end model.Time, interval time.Duration) *query {
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
func (ng *Engine) exec(ctx context.Context, q *query) (model.Value, error) {
	currentQueries.Inc()
	defer currentQueries.Dec()
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

// execEvalStmt evaluates the expression of an evaluation statement for the given time range.
func (ng *Engine) execEvalStmt(ctx context.Context, query *query, s *EvalStmt) (model.Value, error) {
	querier, err := ng.queryable.Querier()
	if err != nil {
		return nil, err
	}
	defer querier.Close()

	prepareTimer := query.stats.GetTimer(stats.QueryPreparationTime).Start()
	err = ng.populateIterators(ctx, querier, s)
	prepareTimer.Stop()
	queryPrepareTime.Observe(prepareTimer.ElapsedTime().Seconds())

	if err != nil {
		return nil, err
	}
	defer ng.closeIterators(s)

	evalTimer := query.stats.GetTimer(stats.InnerEvalTime).Start()
	// Instant evaluation.
	if s.Start == s.End && s.Interval == 0 {
		evaluator := &evaluator{
			Timestamp: s.Start,
			ctx:       ctx,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		// Turn matrix and vector types with protected metrics into
		// model.* types.
		switch v := val.(type) {
		case vector:
			val = v.value()
		case matrix:
			val = v.value()
		}

		evalTimer.Stop()
		queryInnerEval.Observe(evalTimer.ElapsedTime().Seconds())

		return val, nil
	}
	numSteps := int(s.End.Sub(s.Start) / s.Interval)

	// Range evaluation.
	sampleStreams := map[model.Fingerprint]*sampleStream{}
	for ts := s.Start; !ts.After(s.End); ts = ts.Add(s.Interval) {

		if err := contextDone(ctx, "range evaluation"); err != nil {
			return nil, err
		}

		evaluator := &evaluator{
			Timestamp: ts,
			ctx:       ctx,
		}
		val, err := evaluator.Eval(s.Expr)
		if err != nil {
			return nil, err
		}

		switch v := val.(type) {
		case *model.Scalar:
			// As the expression type does not change we can safely default to 0
			// as the fingerprint for scalar expressions.
			ss := sampleStreams[0]
			if ss == nil {
				ss = &sampleStream{Values: make([]model.SamplePair, 0, numSteps)}
				sampleStreams[0] = ss
			}
			ss.Values = append(ss.Values, model.SamplePair{
				Value:     v.Value,
				Timestamp: v.Timestamp,
			})
		case vector:
			for _, sample := range v {
				fp := sample.Metric.Metric.Fingerprint()
				ss := sampleStreams[fp]
				if ss == nil {
					ss = &sampleStream{
						Metric: sample.Metric,
						Values: make([]model.SamplePair, 0, numSteps),
					}
					sampleStreams[fp] = ss
				}
				ss.Values = append(ss.Values, model.SamplePair{
					Value:     sample.Value,
					Timestamp: sample.Timestamp,
				})
			}
		default:
			panic(fmt.Errorf("promql.Engine.exec: invalid expression type %q", val.Type()))
		}
	}
	evalTimer.Stop()
	queryInnerEval.Observe(evalTimer.ElapsedTime().Seconds())

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	appendTimer := query.stats.GetTimer(stats.ResultAppendTime).Start()
	mat := matrix{}
	for _, ss := range sampleStreams {
		mat = append(mat, ss)
	}
	appendTimer.Stop()
	queryResultAppend.Observe(appendTimer.ElapsedTime().Seconds())

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	// Turn matrix type with protected metric into model.Matrix.
	resMatrix := mat.value()

	sortTimer := query.stats.GetTimer(stats.ResultSortTime).Start()
	sort.Sort(resMatrix)
	sortTimer.Stop()
	queryResultSort.Observe(sortTimer.ElapsedTime().Seconds())
	return resMatrix, nil
}

func (ng *Engine) populateIterators(ctx context.Context, querier local.Querier, s *EvalStmt) error {
	var queryErr error
	Inspect(s.Expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			if s.Start.Equal(s.End) {
				n.iterators, queryErr = querier.QueryInstant(
					ctx,
					s.Start.Add(-n.Offset),
					StalenessDelta,
					n.LabelMatchers...,
				)
			} else {
				n.iterators, queryErr = querier.QueryRange(
					ctx,
					s.Start.Add(-n.Offset-StalenessDelta),
					s.End.Add(-n.Offset),
					n.LabelMatchers...,
				)
			}
			if queryErr != nil {
				return false
			}
		case *MatrixSelector:
			n.iterators, queryErr = querier.QueryRange(
				ctx,
				s.Start.Add(-n.Offset-n.Range),
				s.End.Add(-n.Offset),
				n.LabelMatchers...,
			)
			if queryErr != nil {
				return false
			}
		}
		return true
	})
	return queryErr
}

func (ng *Engine) closeIterators(s *EvalStmt) {
	Inspect(s.Expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			for _, it := range n.iterators {
				it.Close()
			}
		case *MatrixSelector:
			for _, it := range n.iterators {
				it.Close()
			}
		}
		return true
	})
}

// An evaluator evaluates given expressions at a fixed timestamp. It is attached to an
// engine through which it connects to a querier and reports errors. On timeout or
// cancellation of its context it terminates.
type evaluator struct {
	ctx context.Context

	Timestamp model.Time
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

// evalScalar attempts to evaluate e to a scalar value and errors otherwise.
func (ev *evaluator) evalScalar(e Expr) *model.Scalar {
	val := ev.eval(e)
	sv, ok := val.(*model.Scalar)
	if !ok {
		ev.errorf("expected scalar but got %s", documentedType(val.Type()))
	}
	return sv
}

// evalVector attempts to evaluate e to a vector value and errors otherwise.
func (ev *evaluator) evalVector(e Expr) vector {
	val := ev.eval(e)
	vec, ok := val.(vector)
	if !ok {
		ev.errorf("expected instant vector but got %s", documentedType(val.Type()))
	}
	return vec
}

// evalInt attempts to evaluate e into an integer and errors otherwise.
func (ev *evaluator) evalInt(e Expr) int64 {
	sc := ev.evalScalar(e)
	if !convertibleToInt64(sc.Value) {
		ev.errorf("scalar value %v overflows int64", sc.Value)
	}
	return int64(sc.Value)
}

// evalFloat attempts to evaluate e into a float and errors otherwise.
func (ev *evaluator) evalFloat(e Expr) float64 {
	sc := ev.evalScalar(e)
	return float64(sc.Value)
}

// evalMatrix attempts to evaluate e into a matrix and errors otherwise.
// The error message uses the term "range vector" to match the user facing
// documentation.
func (ev *evaluator) evalMatrix(e Expr) matrix {
	val := ev.eval(e)
	mat, ok := val.(matrix)
	if !ok {
		ev.errorf("expected range vector but got %s", documentedType(val.Type()))
	}
	return mat
}

// evalString attempts to evaluate e to a string value and errors otherwise.
func (ev *evaluator) evalString(e Expr) *model.String {
	val := ev.eval(e)
	sv, ok := val.(*model.String)
	if !ok {
		ev.errorf("expected string but got %s", documentedType(val.Type()))
	}
	return sv
}

// evalOneOf evaluates e and errors unless the result is of one of the given types.
func (ev *evaluator) evalOneOf(e Expr, t1, t2 model.ValueType) model.Value {
	val := ev.eval(e)
	if val.Type() != t1 && val.Type() != t2 {
		ev.errorf("expected %s or %s but got %s", documentedType(t1), documentedType(t2), documentedType(val.Type()))
	}
	return val
}

func (ev *evaluator) Eval(expr Expr) (v model.Value, err error) {
	defer ev.recover(&err)
	return ev.eval(expr), nil
}

// eval evaluates the given expression as the given AST expression node requires.
func (ev *evaluator) eval(expr Expr) model.Value {
	// This is the top-level evaluation method.
	// Thus, we check for timeout/cancelation here.
	if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
		ev.error(err)
	}

	switch e := expr.(type) {
	case *AggregateExpr:
		vector := ev.evalVector(e.Expr)
		return ev.aggregation(e.Op, e.Grouping, e.Without, e.KeepCommonLabels, e.Param, vector)

	case *BinaryExpr:
		lhs := ev.evalOneOf(e.LHS, model.ValScalar, model.ValVector)
		rhs := ev.evalOneOf(e.RHS, model.ValScalar, model.ValVector)

		switch lt, rt := lhs.Type(), rhs.Type(); {
		case lt == model.ValScalar && rt == model.ValScalar:
			return &model.Scalar{
				Value:     scalarBinop(e.Op, lhs.(*model.Scalar).Value, rhs.(*model.Scalar).Value),
				Timestamp: ev.Timestamp,
			}

		case lt == model.ValVector && rt == model.ValVector:
			switch e.Op {
			case itemLAND:
				return ev.vectorAnd(lhs.(vector), rhs.(vector), e.VectorMatching)
			case itemLOR:
				return ev.vectorOr(lhs.(vector), rhs.(vector), e.VectorMatching)
			case itemLUnless:
				return ev.vectorUnless(lhs.(vector), rhs.(vector), e.VectorMatching)
			default:
				return ev.vectorBinop(e.Op, lhs.(vector), rhs.(vector), e.VectorMatching, e.ReturnBool)
			}
		case lt == model.ValVector && rt == model.ValScalar:
			return ev.vectorScalarBinop(e.Op, lhs.(vector), rhs.(*model.Scalar), false, e.ReturnBool)

		case lt == model.ValScalar && rt == model.ValVector:
			return ev.vectorScalarBinop(e.Op, rhs.(vector), lhs.(*model.Scalar), true, e.ReturnBool)
		}

	case *Call:
		return e.Func.Call(ev, e.Args)

	case *MatrixSelector:
		return ev.matrixSelector(e)

	case *NumberLiteral:
		return &model.Scalar{Value: e.Val, Timestamp: ev.Timestamp}

	case *ParenExpr:
		return ev.eval(e.Expr)

	case *StringLiteral:
		return &model.String{Value: e.Val, Timestamp: ev.Timestamp}

	case *UnaryExpr:
		se := ev.evalOneOf(e.Expr, model.ValScalar, model.ValVector)
		// Only + and - are possible operators.
		if e.Op == itemSUB {
			switch v := se.(type) {
			case *model.Scalar:
				v.Value = -v.Value
			case vector:
				for i, sv := range v {
					v[i].Value = -sv.Value
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
func (ev *evaluator) vectorSelector(node *VectorSelector) vector {
	vec := vector{}
	for _, it := range node.iterators {
		refTime := ev.Timestamp.Add(-node.Offset)
		samplePair := it.ValueAtOrBeforeTime(refTime)
		if samplePair.Timestamp.Before(refTime.Add(-StalenessDelta)) {
			continue // Sample outside of staleness policy window.
		}
		vec = append(vec, &sample{
			Metric:    it.Metric(),
			Value:     samplePair.Value,
			Timestamp: ev.Timestamp,
		})
	}
	return vec
}

// matrixSelector evaluates a *MatrixSelector expression.
func (ev *evaluator) matrixSelector(node *MatrixSelector) matrix {
	interval := metric.Interval{
		OldestInclusive: ev.Timestamp.Add(-node.Range - node.Offset),
		NewestInclusive: ev.Timestamp.Add(-node.Offset),
	}

	sampleStreams := make([]*sampleStream, 0, len(node.iterators))
	for _, it := range node.iterators {
		samplePairs := it.RangeValues(interval)
		if len(samplePairs) == 0 {
			continue
		}

		if node.Offset != 0 {
			for _, sp := range samplePairs {
				sp.Timestamp = sp.Timestamp.Add(node.Offset)
			}
		}

		sampleStream := &sampleStream{
			Metric: it.Metric(),
			Values: samplePairs,
		}
		sampleStreams = append(sampleStreams, sampleStream)
	}
	return matrix(sampleStreams)
}

func (ev *evaluator) vectorAnd(lhs, rhs vector, matching *VectorMatching) vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := signatureFunc(matching.On, matching.MatchingLabels...)

	var result vector
	// The set of signatures for the right-hand side vector.
	rightSigs := map[uint64]struct{}{}
	// Add all rhs samples to a map so we can easily find matches later.
	for _, rs := range rhs {
		rightSigs[sigf(rs.Metric)] = struct{}{}
	}

	for _, ls := range lhs {
		// If there's a matching entry in the right-hand side vector, add the sample.
		if _, ok := rightSigs[sigf(ls.Metric)]; ok {
			result = append(result, ls)
		}
	}
	return result
}

func (ev *evaluator) vectorOr(lhs, rhs vector, matching *VectorMatching) vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := signatureFunc(matching.On, matching.MatchingLabels...)

	var result vector
	leftSigs := map[uint64]struct{}{}
	// Add everything from the left-hand-side vector.
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

func (ev *evaluator) vectorUnless(lhs, rhs vector, matching *VectorMatching) vector {
	if matching.Card != CardManyToMany {
		panic("set operations must only use many-to-many matching")
	}
	sigf := signatureFunc(matching.On, matching.MatchingLabels...)

	rightSigs := map[uint64]struct{}{}
	for _, rs := range rhs {
		rightSigs[sigf(rs.Metric)] = struct{}{}
	}

	var result vector
	for _, ls := range lhs {
		if _, ok := rightSigs[sigf(ls.Metric)]; !ok {
			result = append(result, ls)
		}
	}
	return result
}

// vectorBinop evaluates a binary operation between two vectors, excluding set operators.
func (ev *evaluator) vectorBinop(op itemType, lhs, rhs vector, matching *VectorMatching, returnBool bool) vector {
	if matching.Card == CardManyToMany {
		panic("many-to-many only allowed for set operators")
	}
	var (
		result = vector{}
		sigf   = signatureFunc(matching.On, matching.MatchingLabels...)
	)

	// The control flow below handles one-to-one or many-to-one matching.
	// For one-to-many, swap sidedness and account for the swap when calculating
	// values.
	if matching.Card == CardOneToMany {
		lhs, rhs = rhs, lhs
	}

	// All samples from the rhs hashed by the matching label/values.
	rightSigs := map[uint64]*sample{}

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

		rs, found := rightSigs[sig] // Look for a match in the rhs vector.
		if !found {
			continue
		}

		// Account for potentially swapped sidedness.
		vl, vr := ls.Value, rs.Value
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
			// for the result vector. Check whether those labels have already been added for
			// the same matching labels.
			insertSig := uint64(metric.Metric.Fingerprint())
			if !exists {
				insertedSigs = map[uint64]struct{}{}
				matchedSigs[sig] = insertedSigs
			} else if _, duplicate := insertedSigs[insertSig]; duplicate {
				ev.errorf("multiple matches for labels: grouping labels must ensure unique matches")
			}
			insertedSigs[insertSig] = struct{}{}
		}

		result = append(result, &sample{
			Metric:    metric,
			Value:     value,
			Timestamp: ev.Timestamp,
		})
	}
	return result
}

// signatureFunc returns a function that calculates the signature for a metric
// ignoring the provided labels. If on, then the given labels are only used instead.
func signatureFunc(on bool, labels ...model.LabelName) func(m metric.Metric) uint64 {
	if !on {
		return func(m metric.Metric) uint64 {
			tmp := m.Metric.Clone()
			for _, l := range labels {
				delete(tmp, l)
			}
			delete(tmp, model.MetricNameLabel)
			return uint64(tmp.Fingerprint())
		}
	}
	return func(m metric.Metric) uint64 {
		return model.SignatureForLabels(m.Metric, labels...)
	}
}

// resultMetric returns the metric for the given sample(s) based on the vector
// binary operation and the matching options.
func resultMetric(lhs, rhs metric.Metric, op itemType, matching *VectorMatching) metric.Metric {
	if shouldDropMetricName(op) {
		lhs.Del(model.MetricNameLabel)
	}
	if !matching.On {
		if matching.Card == CardOneToOne {
			for _, l := range matching.MatchingLabels {
				lhs.Del(l)
			}
		}
		for _, ln := range matching.Include {
			// Included labels from the `group_x` modifier are taken from the "one"-side.
			value := rhs.Metric[ln]
			if value != "" {
				lhs.Set(ln, rhs.Metric[ln])
			} else {
				lhs.Del(ln)
			}
		}
		return lhs
	}
	// As we definitely write, creating a new metric is the easiest solution.
	m := model.Metric{}
	if matching.Card == CardOneToOne {
		for _, ln := range matching.MatchingLabels {
			if v, ok := lhs.Metric[ln]; ok {
				m[ln] = v
			}
		}
	} else {
		for k, v := range lhs.Metric {
			m[k] = v
		}
	}
	for _, ln := range matching.Include {
		// Included labels from the `group_x` modifier are taken from the "one"-side .
		if v, ok := rhs.Metric[ln]; ok {
			m[ln] = v
		} else {
			delete(m, ln)
		}
	}
	return metric.Metric{Metric: m, Copied: false}
}

// vectorScalarBinop evaluates a binary operation between a vector and a scalar.
func (ev *evaluator) vectorScalarBinop(op itemType, lhs vector, rhs *model.Scalar, swap, returnBool bool) vector {
	vec := make(vector, 0, len(lhs))

	for _, lhsSample := range lhs {
		lv, rv := lhsSample.Value, rhs.Value
		// lhs always contains the vector. If the original position was different
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
			lhsSample.Value = value
			if shouldDropMetricName(op) {
				lhsSample.Metric.Del(model.MetricNameLabel)
			}
			vec = append(vec, lhsSample)
		}
	}
	return vec
}

// scalarBinop evaluates a binary operation between two scalars.
func scalarBinop(op itemType, lhs, rhs model.SampleValue) model.SampleValue {
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
		return model.SampleValue(math.Pow(float64(lhs), float64(rhs)))
	case itemMOD:
		return model.SampleValue(math.Mod(float64(lhs), float64(rhs)))
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
	panic(fmt.Errorf("operator %q not allowed for scalar operations", op))
}

// vectorElemBinop evaluates a binary operation between two vector elements.
func vectorElemBinop(op itemType, lhs, rhs model.SampleValue) (model.SampleValue, bool) {
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
		return model.SampleValue(math.Pow(float64(lhs), float64(rhs))), true
	case itemMOD:
		return model.SampleValue(math.Mod(float64(lhs), float64(rhs))), true
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
	panic(fmt.Errorf("operator %q not allowed for operations between vectors", op))
}

// labelIntersection returns the metric of common label/value pairs of two input metrics.
func labelIntersection(metric1, metric2 metric.Metric) metric.Metric {
	for label, value := range metric1.Metric {
		if metric2.Metric[label] != value {
			metric1.Del(label)
		}
	}
	return metric1
}

type groupedAggregation struct {
	labels           metric.Metric
	value            model.SampleValue
	valuesSquaredSum model.SampleValue
	groupCount       int
	heap             vectorByValueHeap
	reverseHeap      vectorByReverseValueHeap
}

// aggregation evaluates an aggregation operation on a vector.
func (ev *evaluator) aggregation(op itemType, grouping model.LabelNames, without bool, keepCommon bool, param Expr, vec vector) vector {

	result := map[uint64]*groupedAggregation{}
	var k int64
	if op == itemTopK || op == itemBottomK {
		k = ev.evalInt(param)
		if k < 1 {
			return vector{}
		}
	}
	var q float64
	if op == itemQuantile {
		q = ev.evalFloat(param)
	}
	var valueLabel model.LabelName
	if op == itemCountValues {
		valueLabel = model.LabelName(ev.evalString(param).Value)
		if !without {
			grouping = append(grouping, valueLabel)
		}
	}

	for _, s := range vec {
		withoutMetric := s.Metric
		if without {
			for _, l := range grouping {
				withoutMetric.Del(l)
			}
			withoutMetric.Del(model.MetricNameLabel)
			if op == itemCountValues {
				withoutMetric.Set(valueLabel, model.LabelValue(s.Value.String()))
			}
		} else {
			if op == itemCountValues {
				s.Metric.Set(valueLabel, model.LabelValue(s.Value.String()))
			}
		}

		var groupingKey uint64
		if without {
			groupingKey = uint64(withoutMetric.Metric.Fingerprint())
		} else {
			groupingKey = model.SignatureForLabels(s.Metric.Metric, grouping...)
		}

		groupedResult, ok := result[groupingKey]
		// Add a new group if it doesn't exist.
		if !ok {
			var m metric.Metric
			if keepCommon {
				m = s.Metric
				m.Del(model.MetricNameLabel)
			} else if without {
				m = withoutMetric
			} else {
				m = metric.Metric{
					Metric: model.Metric{},
					Copied: true,
				}
				for _, l := range grouping {
					if v, ok := s.Metric.Metric[l]; ok {
						m.Set(l, v)
					}
				}
			}
			result[groupingKey] = &groupedAggregation{
				labels:           m,
				value:            s.Value,
				valuesSquaredSum: s.Value * s.Value,
				groupCount:       1,
			}
			if op == itemTopK || op == itemQuantile {
				result[groupingKey].heap = make(vectorByValueHeap, 0, k)
				heap.Push(&result[groupingKey].heap, &sample{Value: s.Value, Metric: s.Metric})
			} else if op == itemBottomK {
				result[groupingKey].reverseHeap = make(vectorByReverseValueHeap, 0, k)
				heap.Push(&result[groupingKey].reverseHeap, &sample{Value: s.Value, Metric: s.Metric})
			}
			continue
		}
		// Add the sample to the existing group.
		if keepCommon {
			groupedResult.labels = labelIntersection(groupedResult.labels, s.Metric)
		}

		switch op {
		case itemSum:
			groupedResult.value += s.Value
		case itemAvg:
			groupedResult.value += s.Value
			groupedResult.groupCount++
		case itemMax:
			if groupedResult.value < s.Value || math.IsNaN(float64(groupedResult.value)) {
				groupedResult.value = s.Value
			}
		case itemMin:
			if groupedResult.value > s.Value || math.IsNaN(float64(groupedResult.value)) {
				groupedResult.value = s.Value
			}
		case itemCount, itemCountValues:
			groupedResult.groupCount++
		case itemStdvar, itemStddev:
			groupedResult.value += s.Value
			groupedResult.valuesSquaredSum += s.Value * s.Value
			groupedResult.groupCount++
		case itemTopK:
			if int64(len(groupedResult.heap)) < k || groupedResult.heap[0].Value < s.Value || math.IsNaN(float64(groupedResult.heap[0].Value)) {
				if int64(len(groupedResult.heap)) == k {
					heap.Pop(&groupedResult.heap)
				}
				heap.Push(&groupedResult.heap, &sample{Value: s.Value, Metric: s.Metric})
			}
		case itemBottomK:
			if int64(len(groupedResult.reverseHeap)) < k || groupedResult.reverseHeap[0].Value > s.Value || math.IsNaN(float64(groupedResult.reverseHeap[0].Value)) {
				if int64(len(groupedResult.reverseHeap)) == k {
					heap.Pop(&groupedResult.reverseHeap)
				}
				heap.Push(&groupedResult.reverseHeap, &sample{Value: s.Value, Metric: s.Metric})
			}
		case itemQuantile:
			groupedResult.heap = append(groupedResult.heap, s)
		default:
			panic(fmt.Errorf("expected aggregation operator but got %q", op))
		}
	}

	// Construct the result vector from the aggregated groups.
	resultVector := make(vector, 0, len(result))

	for _, aggr := range result {
		switch op {
		case itemAvg:
			aggr.value = aggr.value / model.SampleValue(aggr.groupCount)
		case itemCount, itemCountValues:
			aggr.value = model.SampleValue(aggr.groupCount)
		case itemStdvar:
			avg := float64(aggr.value) / float64(aggr.groupCount)
			aggr.value = model.SampleValue(float64(aggr.valuesSquaredSum)/float64(aggr.groupCount) - avg*avg)
		case itemStddev:
			avg := float64(aggr.value) / float64(aggr.groupCount)
			aggr.value = model.SampleValue(math.Sqrt(float64(aggr.valuesSquaredSum)/float64(aggr.groupCount) - avg*avg))
		case itemTopK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.heap))
			for _, v := range aggr.heap {
				resultVector = append(resultVector, &sample{
					Metric:    v.Metric,
					Value:     v.Value,
					Timestamp: ev.Timestamp,
				})
			}
			continue // Bypass default append.
		case itemBottomK:
			// The heap keeps the lowest value on top, so reverse it.
			sort.Sort(sort.Reverse(aggr.reverseHeap))
			for _, v := range aggr.reverseHeap {
				resultVector = append(resultVector, &sample{
					Metric:    v.Metric,
					Value:     v.Value,
					Timestamp: ev.Timestamp,
				})
			}
			continue // Bypass default append.
		case itemQuantile:
			aggr.value = model.SampleValue(quantile(q, aggr.heap))
		default:
			// For other aggregations, we already have the right value.
		}
		sample := &sample{
			Metric:    aggr.labels,
			Value:     aggr.value,
			Timestamp: ev.Timestamp,
		}
		resultVector = append(resultVector, sample)
	}
	return resultVector
}

// btos returns 1 if b is true, 0 otherwise.
func btos(b bool) model.SampleValue {
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
func documentedType(t model.ValueType) string {
	switch t.String() {
	case "vector":
		return "instant vector"
	case "matrix":
		return "range vector"
	default:
		return t.String()
	}
}
