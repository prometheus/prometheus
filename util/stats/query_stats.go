// Copyright The Prometheus Authors
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

package stats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// QueryTiming identifies the code area or functionality in which time is spent
// during a query.
type QueryTiming int

// Query timings.
const (
	EvalTotalTime QueryTiming = iota
	ResultSortTime
	QueryPreparationTime
	InnerEvalTime
	ExecQueueTime
	ExecTotalTime
)

// Return a string representation of a QueryTiming identifier.
func (s QueryTiming) String() string {
	switch s {
	case EvalTotalTime:
		return "Eval total time"
	case ResultSortTime:
		return "Result sorting time"
	case QueryPreparationTime:
		return "Query preparation time"
	case InnerEvalTime:
		return "Inner eval time"
	case ExecQueueTime:
		return "Exec queue wait time"
	case ExecTotalTime:
		return "Exec total time"
	default:
		return "Unknown query timing"
	}
}

// SpanOperation returns a string representation of a QueryTiming span operation.
func (s QueryTiming) SpanOperation() string {
	switch s {
	case EvalTotalTime:
		return "promqlEval"
	case ResultSortTime:
		return "promqlSort"
	case QueryPreparationTime:
		return "promqlPrepare"
	case InnerEvalTime:
		return "promqlInnerEval"
	case ExecQueueTime:
		return "promqlExecQueue"
	case ExecTotalTime:
		return "promqlExec"
	default:
		return "Unknown query timing"
	}
}

// stepStat represents a single statistic for a given step timestamp.
type stepStat struct {
	T int64
	V int64
}

func (s stepStat) String() string {
	return fmt.Sprintf("%v @[%v]", s.V, s.T)
}

// MarshalJSON implements json.Marshaler.
func (s stepStat) MarshalJSON() ([]byte, error) {
	return json.Marshal([...]any{float64(s.T) / 1000, s.V})
}

// queryTimings with all query timers mapped to durations.
type queryTimings struct {
	EvalTotalTime        float64 `json:"evalTotalTime"`
	ResultSortTime       float64 `json:"resultSortTime"`
	QueryPreparationTime float64 `json:"queryPreparationTime"`
	InnerEvalTime        float64 `json:"innerEvalTime"`
	ExecQueueTime        float64 `json:"execQueueTime"`
	ExecTotalTime        float64 `json:"execTotalTime"`
}

type querySamples struct {
	TotalQueryableSamplesPerStep []stepStat `json:"totalQueryableSamplesPerStep,omitempty"`
	TotalQueryableSamples        int64      `json:"totalQueryableSamples"`
	SamplesReadPerStep           []stepStat `json:"samplesReadPerStep,omitempty"`
	SamplesRead                  int64      `json:"samplesRead"`
	SeriesTouched                int64      `json:"seriesTouched"`
	PeakSamples                  int        `json:"peakSamples"`
}

// BuiltinStats holds the statistics that Prometheus's core gathers.
type BuiltinStats struct {
	Timings queryTimings  `json:"timings,omitempty"`
	Samples *querySamples `json:"samples,omitempty"`
}

// QueryStats holds BuiltinStats and any other stats the particular
// implementation wants to collect.
type QueryStats interface {
	Builtin() BuiltinStats
}

func (s *BuiltinStats) Builtin() BuiltinStats {
	return *s
}

// NewQueryStats makes a QueryStats struct with all QueryTimings found in the
// given TimerGroup.
func NewQueryStats(s *Statistics) QueryStats {
	var (
		qt      queryTimings
		samples *querySamples
		tg      = s.Timers
		sp      = s.Samples
	)

	for s, timer := range tg.timers {
		switch s {
		case EvalTotalTime:
			qt.EvalTotalTime = timer.Duration()
		case ResultSortTime:
			qt.ResultSortTime = timer.Duration()
		case QueryPreparationTime:
			qt.QueryPreparationTime = timer.Duration()
		case InnerEvalTime:
			qt.InnerEvalTime = timer.Duration()
		case ExecQueueTime:
			qt.ExecQueueTime = timer.Duration()
		case ExecTotalTime:
			qt.ExecTotalTime = timer.Duration()
		}
	}

	if sp != nil {
		samples = &querySamples{
			TotalQueryableSamples: sp.TotalSamples,
			SamplesRead:           sp.SamplesRead,
			SeriesTouched:         sp.TotalSeries,
			PeakSamples:           sp.PeakSamples,
		}
		samples.TotalQueryableSamplesPerStep = sp.totalSamplesPerStepPoints()
		samples.SamplesReadPerStep = sp.samplesReadPerStepPoints()
	}

	qs := BuiltinStats{Timings: qt, Samples: samples}
	return &qs
}

func (qs *QuerySamples) TotalSamplesPerStepMap() *TotalSamplesPerStep {
	if !qs.EnablePerStepStats {
		return nil
	}

	ts := TotalSamplesPerStep{}
	for _, s := range qs.totalSamplesPerStepPoints() {
		ts[s.T] = int(s.V)
	}
	return &ts
}

// SamplesReadPerStepMap returns the per-step samples read as a map
// (timestamp -> count), or nil if per-step stats are disabled.
func (qs *QuerySamples) SamplesReadPerStepMap() *TotalSamplesPerStep {
	if !qs.EnablePerStepStats || qs.SamplesReadPerStep == nil {
		return nil
	}

	ts := TotalSamplesPerStep{}
	for _, s := range qs.samplesReadPerStepPoints() {
		ts[s.T] = int(s.V)
	}
	return &ts
}

func (qs *QuerySamples) totalSamplesPerStepPoints() []stepStat {
	if !qs.EnablePerStepStats {
		return nil
	}

	ts := make([]stepStat, len(qs.TotalSamplesPerStep))
	for i, c := range qs.TotalSamplesPerStep {
		ts[i] = stepStat{T: qs.StartTimestamp + int64(i)*qs.Interval, V: c}
	}
	return ts
}

func (qs *QuerySamples) samplesReadPerStepPoints() []stepStat {
	if !qs.EnablePerStepStats || qs.SamplesReadPerStep == nil {
		return nil
	}

	ts := make([]stepStat, len(qs.SamplesReadPerStep))
	for i, c := range qs.SamplesReadPerStep {
		ts[i] = stepStat{T: qs.StartTimestamp + int64(i)*qs.Interval, V: c}
	}
	return ts
}

// SpanTimer unifies tracing and timing, to reduce repetition.
type SpanTimer struct {
	timer     *Timer
	observers []prometheus.Observer

	span trace.Span
}

func NewSpanTimer(ctx context.Context, operation string, timer *Timer, observers ...prometheus.Observer) (*SpanTimer, context.Context) {
	ctx, span := otel.Tracer("").Start(ctx, operation)
	timer.Start()

	return &SpanTimer{
		timer:     timer,
		observers: observers,

		span: span,
	}, ctx
}

func (s *SpanTimer) Finish() {
	s.timer.Stop()
	s.span.End()

	for _, obs := range s.observers {
		obs.Observe(s.timer.ElapsedTime().Seconds())
	}
}

type Statistics struct {
	Timers  *QueryTimers
	Samples *QuerySamples
}

type QueryTimers struct {
	*TimerGroup
}

type TotalSamplesPerStep map[int64]int

type QuerySamples struct {
	// PeakSamples represent the highest count of samples considered
	// while evaluating a query. It corresponds to the peak value of
	// currentSamples, which is in turn compared against the MaxSamples
	// configured in the engine.
	PeakSamples int

	// TotalSamples represents logical underlying sample consumption while
	// evaluating a query, using the existing histogram size weighting.
	// Subqueries contribute their underlying consumption, not their materialized
	// results. Overlapping evaluation windows may count the same samples repeatedly;
	// this is not a count of unique samples or physical storage reads.
	TotalSamples int64

	// TotalSamplesPerStep represents logical underlying sample consumption
	// per step while evaluating a query. Each step should be identical to the
	// TotalSamples when a step is run as an instant query, which means
	// we intentionally do not account for optimizations that happen inside the
	// range query engine that reduce the actual work that happens.
	// For range-vector functions, each step counts the full window at that step.
	TotalSamplesPerStep []int64

	// SamplesRead counts evaluator input, reusing reads across overlapping
	// range-vector windows. Subquery reads are attributed to consumed windows,
	// unless query-cost is enabled, which includes all evaluated child steps.
	// This is not a count of physical storage I/O.
	SamplesRead int64

	// SamplesReadPerStep is the number of samples read per step. For
	// range-vector functions, step 0 counts the full window and later
	// steps count only the points not already covered by the previous
	// step's window.
	SamplesReadPerStep []int64

	// TotalSeries is the number of series touched while evaluating a query.
	// It is computed as a per-selector SUM: the sum of len(series) across all
	// selectors expanded during evaluation. Series shared between selectors may
	// be counted more than once, so this value is an upper bound on the number
	// of distinct series touched.
	TotalSeries int64

	EnablePerStepStats bool
	StartTimestamp     int64
	Interval           int64
}

type Stats struct {
	TimerStats  *QueryTimers
	SampleStats *QuerySamples
}

func (qs *QuerySamples) InitStepTracking(start, end, interval int64) {
	if qs == nil {
		return
	}
	if !qs.EnablePerStepStats {
		return
	}

	numSteps := int((end-start)/interval) + 1
	qs.TotalSamplesPerStep = make([]int64, numSteps)
	qs.SamplesReadPerStep = make([]int64, numSteps)
	qs.StartTimestamp = start
	qs.Interval = interval
}

func (qs *QuerySamples) StepTrackingEnabled() bool {
	if qs == nil {
		return false
	}
	return qs.EnablePerStepStats
}

// IncrementSamplesAtStep increments the total samples count. Use this if you know the step index.
func (qs *QuerySamples) IncrementSamplesAtStep(i int, samples int64) {
	if qs == nil {
		return
	}
	qs.TotalSamples += samples

	if qs.TotalSamplesPerStep != nil {
		qs.TotalSamplesPerStep[i] += samples
	}
}

// IncrementSamplesAtTimestamp increments the total samples count. Use this if you only have the corresponding step
// timestamp.
func (qs *QuerySamples) IncrementSamplesAtTimestamp(t, samples int64) {
	if qs == nil {
		return
	}
	qs.TotalSamples += samples

	if qs.TotalSamplesPerStep != nil {
		i := int((t - qs.StartTimestamp) / qs.Interval)
		qs.TotalSamplesPerStep[i] += samples
	}
}

// IncrementSamplesReadAtStep increments the samples-read count.
// Use this when you know the step index.
func (qs *QuerySamples) IncrementSamplesReadAtStep(i int, n int64) {
	if qs == nil {
		return
	}
	qs.SamplesRead += n
	if qs.SamplesReadPerStep != nil {
		qs.SamplesReadPerStep[i] += n
	}
}

// IncrementSamplesReadAtTimestamp increments the samples-read count.
// Use this when you only have the step timestamp.
func (qs *QuerySamples) IncrementSamplesReadAtTimestamp(t, n int64) {
	if qs == nil {
		return
	}
	qs.SamplesRead += n
	if qs.SamplesReadPerStep != nil {
		i := int((t - qs.StartTimestamp) / qs.Interval)
		qs.SamplesReadPerStep[i] += n
	}
}

// IncrementSeries increments the count of series touched while evaluating a
// query. The total is a per-selector SUM and is therefore an upper bound on the
// number of distinct series touched.
func (qs *QuerySamples) IncrementSeries(n int64) {
	if qs == nil {
		return
	}
	qs.TotalSeries += n
}

// UpdatePeak updates the peak number of samples considered in
// the evaluation of a query as used with the MaxSamples limit.
func (qs *QuerySamples) UpdatePeak(samples int) {
	if qs == nil {
		return
	}
	if samples > qs.PeakSamples {
		qs.PeakSamples = samples
	}
}

// UpdatePeakFromSubquery updates the peak number of samples considered
// in a query from its evaluation of a subquery.
func (qs *QuerySamples) UpdatePeakFromSubquery(other *QuerySamples) {
	if qs == nil || other == nil {
		return
	}
	if other.PeakSamples > qs.PeakSamples {
		qs.PeakSamples = other.PeakSamples
	}
}

// MergeSeriesFromSubquery adds the series touched while evaluating a subquery to
// the parent query's total. As with TotalSeries itself, the result is a
// per-selector SUM and is an upper bound on the number of distinct series.
func (qs *QuerySamples) MergeSeriesFromSubquery(child *QuerySamples) {
	if qs == nil || child == nil {
		return
	}
	qs.TotalSeries += child.TotalSeries
}

func NewQueryTimers() *QueryTimers {
	return &QueryTimers{NewTimerGroup()}
}

func NewQuerySamples(enablePerStepStats bool) *QuerySamples {
	qs := QuerySamples{EnablePerStepStats: enablePerStepStats}
	return &qs
}

func (*QuerySamples) NewChild() *QuerySamples {
	return NewQuerySamples(false)
}

// NewChildWithStepTracking creates a child QuerySamples with per-step tracking
// enabled and initializes its per-step arrays via InitStepTracking.
func NewChildWithStepTracking(start, end, interval int64) *QuerySamples {
	qs := NewQuerySamples(true)
	qs.InitStepTracking(start, end, interval)
	return qs
}

// SubqueryConsumer describes how a parent evaluator consumes a subquery's
// output, so the subquery's sample stats can be attributed to parent steps.
// A parent step at parentTs consumes the child steps whose timestamps, shifted
// by Offset, fall in (parentTs-Range, parentTs]. Child steps outside every
// parent window are not counted.
type SubqueryConsumer struct {
	// Start, Interval and NumSteps describe the parent's step grid. NumSteps <= 1
	// folds the child's totals into the parent's first step.
	Start, Interval int64
	NumSteps        int
	// Offset is the subquery's original offset.
	Offset int64
	// Range is the window width consumed at each parent step. Range <= 0 disables
	// window filtering and attributes each child step to the earliest parent step
	// at or after it, clamped to the parent's grid.
	Range int64
	// AtTimestamp is the subquery's @ timestamp, if any. Every parent step then
	// consumes the fixed window (*AtTimestamp-Range, *AtTimestamp].
	AtTimestamp *int64
}

func (c SubqueryConsumer) stepTimestamp(step int) int64 {
	return c.Start + int64(step)*c.Interval
}

// MergeTotalSamplesFromSubquery merges the child's TotalSamples into the parent.
// Like a range-vector function, each parent step counts every child step in its
// window, so overlapping windows count a child step more than once. The child
// must be created with NewChildWithStepTracking.
func (qs *QuerySamples) MergeTotalSamplesFromSubquery(child *QuerySamples, c SubqueryConsumer) {
	if qs == nil || child == nil {
		return
	}
	switch {
	case c.NumSteps <= 1:
		qs.IncrementSamplesAtStep(0, child.TotalSamples)
	case c.Range <= 0:
		qs.mergeStepsOnce(child, child.TotalSamplesPerStep, c, qs.IncrementSamplesAtStep)
	case c.AtTimestamp != nil:
		n := child.sumInWindow(child.TotalSamplesPerStep, c.Offset, *c.AtTimestamp-c.Range, *c.AtTimestamp)
		for step := range c.NumSteps {
			qs.IncrementSamplesAtStep(step, n)
		}
	default:
		// Parent windows only move forward, so slide over the child steps.
		perStep := child.TotalSamplesPerStep
		var sum int64
		lo, hi := 0, 0
		for step := range c.NumSteps {
			maxt := c.stepTimestamp(step)
			for hi < len(perStep) && child.stepTimestamp(hi)+c.Offset <= maxt {
				sum += perStep[hi]
				hi++
			}
			for lo < hi && child.stepTimestamp(lo)+c.Offset <= maxt-c.Range {
				sum -= perStep[lo]
				lo++
			}
			qs.IncrementSamplesAtStep(step, sum)
		}
	}
}

// MergeSamplesReadFromSubquery merges the child's SamplesRead into the parent.
// Each consumed child step is counted once, at the earliest parent step that
// consumes it. The child must be created with NewChildWithStepTracking.
func (qs *QuerySamples) MergeSamplesReadFromSubquery(child *QuerySamples, c SubqueryConsumer) {
	if qs == nil || child == nil {
		return
	}
	switch {
	case c.NumSteps <= 1:
		qs.IncrementSamplesReadAtStep(0, child.SamplesRead)
	case c.AtTimestamp != nil && c.Range > 0:
		qs.IncrementSamplesReadAtStep(0, child.sumInWindow(child.SamplesReadPerStep, c.Offset, *c.AtTimestamp-c.Range, *c.AtTimestamp))
	default:
		qs.mergeStepsOnce(child, child.SamplesReadPerStep, c, qs.IncrementSamplesReadAtStep)
	}
}

// mergeStepsOnce increments the earliest parent step consuming each child step.
func (*QuerySamples) mergeStepsOnce(child *QuerySamples, perStep []int64, c SubqueryConsumer, increment func(int, int64)) {
	for k, n := range perStep {
		if n == 0 {
			continue
		}
		if step, ok := c.consumingStep(child.stepTimestamp(k) + c.Offset); ok {
			increment(step, n)
		}
	}
}

// consumingStep returns the earliest parent step whose window contains tk.
func (c SubqueryConsumer) consumingStep(tk int64) (int, bool) {
	step := 0
	if tk > c.Start {
		step = int((tk - c.Start + c.Interval - 1) / c.Interval)
	}
	if c.Range <= 0 {
		return min(step, c.NumSteps-1), true
	}
	if step >= c.NumSteps || tk <= c.stepTimestamp(step)-c.Range {
		return 0, false
	}
	return step, true
}

// sumInWindow sums perStep over steps whose timestamp plus offset is in (mint, maxt].
func (qs *QuerySamples) sumInWindow(perStep []int64, offset, mint, maxt int64) int64 {
	var sum int64
	for k, n := range perStep {
		if tk := qs.stepTimestamp(k) + offset; tk > mint && tk <= maxt {
			sum += n
		}
	}
	return sum
}

func (qs *QuerySamples) stepTimestamp(step int) int64 {
	return qs.StartTimestamp + int64(step)*qs.Interval
}

func (qs *QueryTimers) GetSpanTimer(ctx context.Context, qt QueryTiming, observers ...prometheus.Observer) (*SpanTimer, context.Context) {
	return NewSpanTimer(ctx, qt.SpanOperation(), qs.GetTimer(qt), observers...)
}
