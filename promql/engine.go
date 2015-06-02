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
	"flag"
	"fmt"
	"math"
	"runtime"
	"sort"
	"time"

	"golang.org/x/net/context"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/stats"
)

var (
	stalenessDelta       = flag.Duration("query.staleness-delta", 300*time.Second, "Staleness delta allowance during expression evaluations.")
	defaultQueryTimeout  = flag.Duration("query.timeout", 2*time.Minute, "Maximum time a query may take before being aborted.")
	maxConcurrentQueries = flag.Int("query.max-concurrency", 20, "Maximum number of queries executed concurrently.")
)

// SampleStream is a stream of Values belonging to an attached COWMetric.
type SampleStream struct {
	Metric clientmodel.COWMetric `json:"metric"`
	Values metric.Values         `json:"values"`
}

// Sample is a single sample belonging to a COWMetric.
type Sample struct {
	Metric    clientmodel.COWMetric   `json:"metric"`
	Value     clientmodel.SampleValue `json:"value"`
	Timestamp clientmodel.Timestamp   `json:"timestamp"`
}

// Scalar is a scalar value evaluated at the set timestamp.
type Scalar struct {
	Value     clientmodel.SampleValue
	Timestamp clientmodel.Timestamp
}

func (s *Scalar) String() string {
	return fmt.Sprintf("scalar: %v @[%v]", s.Value, s.Timestamp)
}

// String is a string value evaluated at the set timestamp.
type String struct {
	Value     string
	Timestamp clientmodel.Timestamp
}

func (s *String) String() string {
	return s.Value
}

// Vector is basically only an alias for clientmodel.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type Vector []*Sample

// Matrix is a slice of SampleStreams that implements sort.Interface and
// has a String method.
type Matrix []*SampleStream

// Len implements sort.Interface.
func (matrix Matrix) Len() int {
	return len(matrix)
}

// Less implements sort.Interface.
func (matrix Matrix) Less(i, j int) bool {
	return matrix[i].Metric.String() < matrix[j].Metric.String()
}

// Swap implements sort.Interface.
func (matrix Matrix) Swap(i, j int) {
	matrix[i], matrix[j] = matrix[j], matrix[i]
}

// Value is a generic interface for values resulting from a query evaluation.
type Value interface {
	Type() ExprType
	String() string
}

func (Matrix) Type() ExprType  { return ExprMatrix }
func (Vector) Type() ExprType  { return ExprVector }
func (*Scalar) Type() ExprType { return ExprScalar }
func (*String) Type() ExprType { return ExprString }

// Result holds the resulting value of an execution or an error
// if any occurred.
type Result struct {
	Err   error
	Value Value
}

// Vector returns a vector if the result value is one. An error is returned if
// the result was an error or the result value is not a vector.
func (r *Result) Vector() (Vector, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Vector)
	if !ok {
		return nil, fmt.Errorf("query result is not a vector")
	}
	return v, nil
}

// Matrix returns a matrix. An error is returned if
// the result was an error or the result value is not a matrix.
func (r *Result) Matrix() (Matrix, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Matrix)
	if !ok {
		return nil, fmt.Errorf("query result is not a matrix")
	}
	return v, nil
}

// Scalar returns a scalar value. An error is returned if
// the result was an error or the result value is not a scalar.
func (r *Result) Scalar() (*Scalar, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(*Scalar)
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
)

func (e ErrQueryTimeout) Error() string  { return fmt.Sprintf("query timed out in %s", string(e)) }
func (e ErrQueryCanceled) Error() string { return fmt.Sprintf("query was canceled in %s", string(e)) }

// A Query is derived from an a raw query string and can be run against an engine
// it is associated with.
type Query interface {
	// Exec processes the query and
	Exec() *Result
	// Statements returns the parsed statements of the query.
	Statements() Statements
	// Stats returns statistics about the lifetime of the query.
	Stats() *stats.TimerGroup
	// Cancel signals that a running query execution should be aborted.
	Cancel()
}

// query implements the Query interface.
type query struct {
	// The original query string.
	q string
	// Statements of the parsed query.
	stmts Statements
	// Timer stats for the query execution.
	stats *stats.TimerGroup
	// Cancelation function for the query.
	cancel func()

	// The engine against which the query is executed.
	ng *Engine
}

// Statements implements the Query interface.
func (q *query) Statements() Statements {
	return q.stmts
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
func (q *query) Exec() *Result {
	res, err := q.ng.exec(q)
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

// Engine handles the liftetime of queries from beginning to end.
// It is connected to a storage.
type Engine struct {
	// The storage on which the engine operates.
	storage local.Storage

	// The base context for all queries and its cancellation function.
	baseCtx       context.Context
	cancelQueries func()
	// The gate limiting the maximum number of concurrent and waiting queries.
	gate *queryGate
}

// NewEngine returns a new engine.
func NewEngine(storage local.Storage) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		storage:       storage,
		baseCtx:       ctx,
		cancelQueries: cancel,
		gate:          newQueryGate(*maxConcurrentQueries),
	}
}

// Stop the engine and cancel all running queries.
func (ng *Engine) Stop() {
	ng.cancelQueries()
}

// NewInstantQuery returns an evaluation query for the given expression at the given time.
func (ng *Engine) NewInstantQuery(es string, ts clientmodel.Timestamp) (Query, error) {
	return ng.NewRangeQuery(es, ts, ts, 0)
}

// NewRangeQuery returns an evaluation query for the given time range and with
// the resolution set by the interval.
func (ng *Engine) NewRangeQuery(qs string, start, end clientmodel.Timestamp, interval time.Duration) (Query, error) {
	expr, err := ParseExpr(qs)
	if err != nil {
		return nil, err
	}
	qry := ng.newQuery(expr, start, end, interval)
	qry.q = qs

	return qry, nil
}

func (ng *Engine) newQuery(expr Expr, start, end clientmodel.Timestamp, interval time.Duration) *query {
	es := &EvalStmt{
		Expr:     expr,
		Start:    start,
		End:      end,
		Interval: interval,
	}
	qry := &query{
		stmts: Statements{es},
		ng:    ng,
		stats: stats.NewTimerGroup(),
	}
	return qry
}

// testStmt is an internal helper statement that allows execution
// of an arbitrary function during handling. It is used to test the Engine.
type testStmt func(context.Context) error

func (testStmt) String() string   { return "test statement" }
func (testStmt) DotGraph() string { return "test statement" }
func (testStmt) stmt()            {}

func (ng *Engine) newTestQuery(stmts ...Statement) Query {
	qry := &query{
		q:     "test statement",
		stmts: Statements(stmts),
		ng:    ng,
		stats: stats.NewTimerGroup(),
	}
	return qry
}

// exec executes the query.
//
// At this point per query only one EvalStmt is evaluated. Alert and record
// statements are not handled by the Engine.
func (ng *Engine) exec(q *query) (Value, error) {
	const env = "query execution"

	ctx, cancel := context.WithTimeout(q.ng.baseCtx, *defaultQueryTimeout)
	q.cancel = cancel

	queueTimer := q.stats.GetTimer(stats.ExecQueueTime).Start()

	if err := ng.gate.Start(ctx); err != nil {
		return nil, err
	}
	defer ng.gate.Done()

	queueTimer.Stop()

	// Cancel when execution is done or an error was raised.
	defer q.cancel()

	evalTimer := q.stats.GetTimer(stats.TotalEvalTime).Start()
	defer evalTimer.Stop()

	for _, stmt := range q.stmts {
		// The base context might already be canceled on the first iteration (e.g. during shutdown).
		if err := contextDone(ctx, env); err != nil {
			return nil, err
		}

		switch s := stmt.(type) {
		case *EvalStmt:
			// Currently, only one execution statement per query is allowed.
			return ng.execEvalStmt(ctx, q, s)

		case testStmt:
			if err := s(ctx); err != nil {
				return nil, err
			}

		default:
			panic(fmt.Errorf("promql.Engine.exec: unhandled statement of type %T", stmt))
		}
	}
	return nil, nil
}

// execEvalStmt evaluates the expression of an evaluation statement for the given time range.
func (ng *Engine) execEvalStmt(ctx context.Context, query *query, s *EvalStmt) (Value, error) {
	prepareTimer := query.stats.GetTimer(stats.TotalQueryPreparationTime).Start()
	analyzeTimer := query.stats.GetTimer(stats.QueryAnalysisTime).Start()

	// Only one execution statement per query is allowed.
	analyzer := &Analyzer{
		Storage: ng.storage,
		Expr:    s.Expr,
		Start:   s.Start,
		End:     s.End,
	}
	err := analyzer.Analyze(ctx)
	if err != nil {
		analyzeTimer.Stop()
		prepareTimer.Stop()
		return nil, err
	}
	analyzeTimer.Stop()

	preloadTimer := query.stats.GetTimer(stats.PreloadTime).Start()
	closer, err := analyzer.Prepare(ctx)
	if err != nil {
		preloadTimer.Stop()
		prepareTimer.Stop()
		return nil, err
	}
	defer closer.Close()

	preloadTimer.Stop()
	prepareTimer.Stop()

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

		evalTimer.Stop()
		return val, nil
	}

	// Range evaluation.
	sampleStreams := map[clientmodel.Fingerprint]*SampleStream{}
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
		vector, ok := val.(Vector)
		if !ok {
			return nil, fmt.Errorf("value for expression %q must be of type vector but is %s", s.Expr, val.Type())
		}

		for _, sample := range vector {
			samplePair := metric.SamplePair{
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			}
			fp := sample.Metric.Metric.Fingerprint()
			if sampleStreams[fp] == nil {
				sampleStreams[fp] = &SampleStream{
					Metric: sample.Metric,
					Values: metric.Values{samplePair},
				}
			} else {
				sampleStreams[fp].Values = append(sampleStreams[fp].Values, samplePair)
			}

		}
	}
	evalTimer.Stop()

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	appendTimer := query.stats.GetTimer(stats.ResultAppendTime).Start()
	matrix := Matrix{}
	for _, sampleStream := range sampleStreams {
		matrix = append(matrix, sampleStream)
	}
	appendTimer.Stop()

	if err := contextDone(ctx, "expression evaluation"); err != nil {
		return nil, err
	}

	sortTimer := query.stats.GetTimer(stats.ResultSortTime).Start()
	sort.Sort(matrix)
	sortTimer.Stop()

	return matrix, nil
}

// An evaluator evaluates given expressions at a fixed timestamp. It is attached to an
// engine through which it connects to a storage and reports errors. On timeout or
// cancellation of its context it terminates.
type evaluator struct {
	ctx context.Context

	Timestamp clientmodel.Timestamp
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
		// Do not recover from runtime errors.
		if _, ok := e.(runtime.Error); ok {
			panic(e)
		}
		*errp = e.(error)
	}
}

// evalScalar attempts to evaluate e to a scalar value and errors otherwise.
func (ev *evaluator) evalScalar(e Expr) *Scalar {
	val := ev.eval(e)
	sv, ok := val.(*Scalar)
	if !ok {
		ev.errorf("expected scalar but got %s", val.Type())
	}
	return sv
}

// evalVector attempts to evaluate e to a vector value and errors otherwise.
func (ev *evaluator) evalVector(e Expr) Vector {
	val := ev.eval(e)
	vec, ok := val.(Vector)
	if !ok {
		ev.errorf("expected vector but got %s", val.Type())
	}
	return vec
}

// evalInt attempts to evaluate e into an integer and errors otherwise.
func (ev *evaluator) evalInt(e Expr) int {
	sc := ev.evalScalar(e)
	return int(sc.Value)
}

// evalFloat attempts to evaluate e into a float and errors otherwise.
func (ev *evaluator) evalFloat(e Expr) float64 {
	sc := ev.evalScalar(e)
	return float64(sc.Value)
}

// evalMatrix attempts to evaluate e into a matrix and errors otherwise.
func (ev *evaluator) evalMatrix(e Expr) Matrix {
	val := ev.eval(e)
	mat, ok := val.(Matrix)
	if !ok {
		ev.errorf("expected matrix but got %s", val.Type())
	}
	return mat
}

// evalMatrixBounds attempts to evaluate e to matrix boundaries and errors otherwise.
func (ev *evaluator) evalMatrixBounds(e Expr) Matrix {
	ms, ok := e.(*MatrixSelector)
	if !ok {
		ev.errorf("matrix bounds can only be evaluated for matrix selectors, got %T", e)
	}
	return ev.matrixSelectorBounds(ms)
}

// evalOneOf evaluates e and errors unless the result is of one of the given types.
func (ev *evaluator) evalOneOf(e Expr, t1, t2 ExprType) Value {
	val := ev.eval(e)
	if val.Type() != t1 && val.Type() != t2 {
		ev.errorf("expected %s or %s but got %s", t1, t2, val.Type())
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
	// Thus, we check for timeout/cancellation here.
	if err := contextDone(ev.ctx, "expression evaluation"); err != nil {
		ev.error(err)
	}

	switch e := expr.(type) {
	case *AggregateExpr:
		vector := ev.evalVector(e.Expr)
		return ev.aggregation(e.Op, e.Grouping, e.KeepExtraLabels, vector)

	case *BinaryExpr:
		lhs := ev.evalOneOf(e.LHS, ExprScalar, ExprVector)
		rhs := ev.evalOneOf(e.RHS, ExprScalar, ExprVector)

		switch lt, rt := lhs.Type(), rhs.Type(); {
		case lt == ExprScalar && rt == ExprScalar:
			return &Scalar{
				Value:     scalarBinop(e.Op, lhs.(*Scalar).Value, rhs.(*Scalar).Value),
				Timestamp: ev.Timestamp,
			}

		case lt == ExprVector && rt == ExprVector:
			switch e.Op {
			case itemLAND:
				return ev.vectorAnd(lhs.(Vector), rhs.(Vector), e.VectorMatching)
			case itemLOR:
				return ev.vectorOr(lhs.(Vector), rhs.(Vector), e.VectorMatching)
			default:
				return ev.vectorBinop(e.Op, lhs.(Vector), rhs.(Vector), e.VectorMatching)
			}
		case lt == ExprVector && rt == ExprScalar:
			return ev.vectorScalarBinop(e.Op, lhs.(Vector), rhs.(*Scalar), false)

		case lt == ExprScalar && rt == ExprVector:
			return ev.vectorScalarBinop(e.Op, rhs.(Vector), lhs.(*Scalar), true)
		}

	case *Call:
		return e.Func.Call(ev, e.Args)

	case *MatrixSelector:
		return ev.matrixSelector(e)

	case *NumberLiteral:
		return &Scalar{Value: e.Val, Timestamp: ev.Timestamp}

	case *ParenExpr:
		return ev.eval(e.Expr)

	case *StringLiteral:
		return &String{Value: e.Val, Timestamp: ev.Timestamp}

	case *UnaryExpr:
		smpl := ev.evalScalar(e.Expr)
		if e.Op == itemSUB {
			smpl.Value = -smpl.Value
		}
		return smpl

	case *VectorSelector:
		return ev.vectorSelector(e)
	}
	panic(fmt.Errorf("unhandled expression of type: %T", expr))
}

// vectorSelector evaluates a *VectorSelector expression.
func (ev *evaluator) vectorSelector(node *VectorSelector) Vector {
	vec := Vector{}
	for fp, it := range node.iterators {
		sampleCandidates := it.ValueAtTime(ev.Timestamp.Add(-node.Offset))
		samplePair := chooseClosestSample(sampleCandidates, ev.Timestamp.Add(-node.Offset))
		if samplePair != nil {
			vec = append(vec, &Sample{
				Metric:    node.metrics[fp],
				Value:     samplePair.Value,
				Timestamp: ev.Timestamp,
			})
		}
	}
	return vec
}

// matrixSelector evaluates a *MatrixSelector expression.
func (ev *evaluator) matrixSelector(node *MatrixSelector) Matrix {
	interval := metric.Interval{
		OldestInclusive: ev.Timestamp.Add(-node.Range - node.Offset),
		NewestInclusive: ev.Timestamp.Add(-node.Offset),
	}

	sampleStreams := make([]*SampleStream, 0, len(node.iterators))
	for fp, it := range node.iterators {
		samplePairs := it.RangeValues(interval)
		if len(samplePairs) == 0 {
			continue
		}

		if node.Offset != 0 {
			for _, sp := range samplePairs {
				sp.Timestamp = sp.Timestamp.Add(node.Offset)
			}
		}

		sampleStream := &SampleStream{
			Metric: node.metrics[fp],
			Values: samplePairs,
		}
		sampleStreams = append(sampleStreams, sampleStream)
	}
	return Matrix(sampleStreams)
}

// matrixSelectorBounds evaluates the boundaries of a *MatrixSelector.
func (ev *evaluator) matrixSelectorBounds(node *MatrixSelector) Matrix {
	interval := metric.Interval{
		OldestInclusive: ev.Timestamp.Add(-node.Range - node.Offset),
		NewestInclusive: ev.Timestamp.Add(-node.Offset),
	}

	sampleStreams := make([]*SampleStream, 0, len(node.iterators))
	for fp, it := range node.iterators {
		samplePairs := it.BoundaryValues(interval)
		if len(samplePairs) == 0 {
			continue
		}

		sampleStream := &SampleStream{
			Metric: node.metrics[fp],
			Values: samplePairs,
		}
		sampleStreams = append(sampleStreams, sampleStream)
	}
	return Matrix(sampleStreams)
}

func (ev *evaluator) vectorAnd(lhs, rhs Vector, matching *VectorMatching) Vector {
	if matching.Card != CardManyToMany {
		panic("logical operations must always be many-to-many matching")
	}
	// If no matching labels are specified, match by all labels.
	sigf := signatureFunc(matching.On...)

	var result Vector
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

func (ev *evaluator) vectorOr(lhs, rhs Vector, matching *VectorMatching) Vector {
	if matching.Card != CardManyToMany {
		panic("logical operations must always be many-to-many matching")
	}
	sigf := signatureFunc(matching.On...)

	var result Vector
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

// vectorBinop evaluates a binary operation between two vector, excluding AND and OR.
func (ev *evaluator) vectorBinop(op itemType, lhs, rhs Vector, matching *VectorMatching) Vector {
	if matching.Card == CardManyToMany {
		panic("many-to-many only allowed for AND and OR")
	}
	var (
		result       = Vector{}
		sigf         = signatureFunc(matching.On...)
		resultLabels = append(matching.On, matching.Include...)
	)

	// The control flow below handles one-to-one or many-to-one matching.
	// For one-to-many, swap sidedness and account for the swap when calculating
	// values.
	if matching.Card == CardOneToMany {
		lhs, rhs = rhs, lhs
	}

	// All samples from the rhs hashed by the matching label/values.
	rightSigs := map[uint64]*Sample{}

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
		if !keep {
			continue
		}
		metric := resultMetric(ls.Metric, op, resultLabels...)

		insertedSigs, exists := matchedSigs[sig]
		if matching.Card == CardOneToOne {
			if exists {
				ev.errorf("multiple matches for labels: many-to-one matching must be explicit (group_left/group_right)")
			}
			matchedSigs[sig] = nil // Set existance to true.
		} else {
			// In many-to-one matching the grouping labels have to ensure a unique metric
			// for the result vector. Check whether those labels have already been added for
			// the same matching labels.
			insertSig := clientmodel.SignatureForLabels(metric.Metric, matching.Include)
			if !exists {
				insertedSigs = map[uint64]struct{}{}
				matchedSigs[sig] = insertedSigs
			} else if _, duplicate := insertedSigs[insertSig]; duplicate {
				ev.errorf("multiple matches for labels: grouping labels must ensure unique matches")
			}
			insertedSigs[insertSig] = struct{}{}
		}

		result = append(result, &Sample{
			Metric:    metric,
			Value:     value,
			Timestamp: ev.Timestamp,
		})
	}
	return result
}

// signatureFunc returns a function that calculates the signature for a metric
// based on the provided labels.
func signatureFunc(labels ...clientmodel.LabelName) func(m clientmodel.COWMetric) uint64 {
	if len(labels) == 0 {
		return func(m clientmodel.COWMetric) uint64 {
			m.Delete(clientmodel.MetricNameLabel)
			return uint64(m.Metric.Fingerprint())
		}
	}
	return func(m clientmodel.COWMetric) uint64 {
		return clientmodel.SignatureForLabels(m.Metric, labels)
	}
}

// resultMetric returns the metric for the given sample(s) based on the vector
// binary operation and the matching options.
func resultMetric(met clientmodel.COWMetric, op itemType, labels ...clientmodel.LabelName) clientmodel.COWMetric {
	if len(labels) == 0 {
		if shouldDropMetricName(op) {
			met.Delete(clientmodel.MetricNameLabel)
		}
		return met
	}
	// As we definitly write, creating a new metric is the easiest solution.
	m := clientmodel.Metric{}
	for _, ln := range labels {
		// Included labels from the `group_x` modifier are taken from the "many"-side.
		if v, ok := met.Metric[ln]; ok {
			m[ln] = v
		}
	}
	return clientmodel.COWMetric{Metric: m, Copied: false}
}

// vectorScalarBinop evaluates a binary operation between a vector and a scalar.
func (ev *evaluator) vectorScalarBinop(op itemType, lhs Vector, rhs *Scalar, swap bool) Vector {
	vector := make(Vector, 0, len(lhs))

	for _, lhsSample := range lhs {
		lv, rv := lhsSample.Value, rhs.Value
		// lhs always contains the vector. If the original position was different
		// swap for calculating the value.
		if swap {
			lv, rv = rv, lv
		}
		value, keep := vectorElemBinop(op, lv, rv)
		if keep {
			lhsSample.Value = value
			if shouldDropMetricName(op) {
				lhsSample.Metric.Delete(clientmodel.MetricNameLabel)
			}
			vector = append(vector, lhsSample)
		}
	}
	return vector
}

// scalarBinop evaluates a binary operation between two scalars.
func scalarBinop(op itemType, lhs, rhs clientmodel.SampleValue) clientmodel.SampleValue {
	switch op {
	case itemADD:
		return lhs + rhs
	case itemSUB:
		return lhs - rhs
	case itemMUL:
		return lhs * rhs
	case itemDIV:
		return lhs / rhs
	case itemMOD:
		if rhs != 0 {
			return clientmodel.SampleValue(int(lhs) % int(rhs))
		}
		return clientmodel.SampleValue(math.NaN())
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
func vectorElemBinop(op itemType, lhs, rhs clientmodel.SampleValue) (clientmodel.SampleValue, bool) {
	switch op {
	case itemADD:
		return lhs + rhs, true
	case itemSUB:
		return lhs - rhs, true
	case itemMUL:
		return lhs * rhs, true
	case itemDIV:
		return lhs / rhs, true
	case itemMOD:
		if rhs != 0 {
			return clientmodel.SampleValue(int(lhs) % int(rhs)), true
		}
		return clientmodel.SampleValue(math.NaN()), true
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
func labelIntersection(metric1, metric2 clientmodel.COWMetric) clientmodel.COWMetric {
	for label, value := range metric1.Metric {
		if metric2.Metric[label] != value {
			metric1.Delete(label)
		}
	}
	return metric1
}

type groupedAggregation struct {
	labels           clientmodel.COWMetric
	value            clientmodel.SampleValue
	valuesSquaredSum clientmodel.SampleValue
	groupCount       int
}

// aggregation evaluates an aggregation operation on a vector.
func (ev *evaluator) aggregation(op itemType, grouping clientmodel.LabelNames, keepExtra bool, vector Vector) Vector {

	result := map[uint64]*groupedAggregation{}

	for _, sample := range vector {
		groupingKey := clientmodel.SignatureForLabels(sample.Metric.Metric, grouping)

		groupedResult, ok := result[groupingKey]
		// Add a new group if it doesn't exist.
		if !ok {
			var m clientmodel.COWMetric
			if keepExtra {
				m = sample.Metric
				m.Delete(clientmodel.MetricNameLabel)
			} else {
				m = clientmodel.COWMetric{
					Metric: clientmodel.Metric{},
					Copied: true,
				}
				for _, l := range grouping {
					if v, ok := sample.Metric.Metric[l]; ok {
						m.Set(l, v)
					}
				}
			}
			result[groupingKey] = &groupedAggregation{
				labels:           m,
				value:            sample.Value,
				valuesSquaredSum: sample.Value * sample.Value,
				groupCount:       1,
			}
			continue
		}
		// Add the sample to the existing group.
		if keepExtra {
			groupedResult.labels = labelIntersection(groupedResult.labels, sample.Metric)
		}

		switch op {
		case itemSum:
			groupedResult.value += sample.Value
		case itemAvg:
			groupedResult.value += sample.Value
			groupedResult.groupCount++
		case itemMax:
			if groupedResult.value < sample.Value {
				groupedResult.value = sample.Value
			}
		case itemMin:
			if groupedResult.value > sample.Value {
				groupedResult.value = sample.Value
			}
		case itemCount:
			groupedResult.groupCount++
		case itemStdvar, itemStddev:
			groupedResult.value += sample.Value
			groupedResult.valuesSquaredSum += sample.Value * sample.Value
			groupedResult.groupCount++
		default:
			panic(fmt.Errorf("expected aggregation operator but got %q", op))
		}
	}

	// Construct the result vector from the aggregated groups.
	resultVector := make(Vector, 0, len(result))

	for _, aggr := range result {
		switch op {
		case itemAvg:
			aggr.value = aggr.value / clientmodel.SampleValue(aggr.groupCount)
		case itemCount:
			aggr.value = clientmodel.SampleValue(aggr.groupCount)
		case itemStdvar:
			avg := float64(aggr.value) / float64(aggr.groupCount)
			aggr.value = clientmodel.SampleValue(float64(aggr.valuesSquaredSum)/float64(aggr.groupCount) - avg*avg)
		case itemStddev:
			avg := float64(aggr.value) / float64(aggr.groupCount)
			aggr.value = clientmodel.SampleValue(math.Sqrt(float64(aggr.valuesSquaredSum)/float64(aggr.groupCount) - avg*avg))
		default:
			// For other aggregations, we already have the right value.
		}
		sample := &Sample{
			Metric:    aggr.labels,
			Value:     aggr.value,
			Timestamp: ev.Timestamp,
		}
		resultVector = append(resultVector, sample)
	}
	return resultVector
}

// btos returns 1 if b is true, 0 otherwise.
func btos(b bool) clientmodel.SampleValue {
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

// chooseClosestSample chooses the closest sample of a list of samples
// surrounding a given target time. If samples are found both before and after
// the target time, the sample value is interpolated between these. Otherwise,
// the single closest sample is returned verbatim.
func chooseClosestSample(samples metric.Values, timestamp clientmodel.Timestamp) *metric.SamplePair {
	var closestBefore *metric.SamplePair
	var closestAfter *metric.SamplePair
	for _, candidate := range samples {
		delta := candidate.Timestamp.Sub(timestamp)
		// Samples before target time.
		if delta < 0 {
			// Ignore samples outside of staleness policy window.
			if -delta > *stalenessDelta {
				continue
			}
			// Ignore samples that are farther away than what we've seen before.
			if closestBefore != nil && candidate.Timestamp.Before(closestBefore.Timestamp) {
				continue
			}
			sample := candidate
			closestBefore = &sample
		}

		// Samples after target time.
		if delta >= 0 {
			// Ignore samples outside of staleness policy window.
			if delta > *stalenessDelta {
				continue
			}
			// Ignore samples that are farther away than samples we've seen before.
			if closestAfter != nil && candidate.Timestamp.After(closestAfter.Timestamp) {
				continue
			}
			sample := candidate
			closestAfter = &sample
		}
	}

	switch {
	case closestBefore != nil && closestAfter != nil:
		return interpolateSamples(closestBefore, closestAfter, timestamp)
	case closestBefore != nil:
		return closestBefore
	default:
		return closestAfter
	}
}

// interpolateSamples interpolates a value at a target time between two
// provided sample pairs.
func interpolateSamples(first, second *metric.SamplePair, timestamp clientmodel.Timestamp) *metric.SamplePair {
	dv := second.Value - first.Value
	dt := second.Timestamp.Sub(first.Timestamp)

	dDt := dv / clientmodel.SampleValue(dt)
	offset := clientmodel.SampleValue(timestamp.Sub(first.Timestamp))

	return &metric.SamplePair{
		Value:     first.Value + (offset * dDt),
		Timestamp: timestamp,
	}
}

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
