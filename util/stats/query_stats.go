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

// SamplesReadPerStepMap returns the per-step samples read as a map (timestamp -> count), or nil if per-step stats are disabled.
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

	// TotalSamples represents the total number of samples loaded while
	// evaluating a query. For range-vector functions, each step counts the
	// full window (points may be counted in multiple steps).
	TotalSamples int64

	// TotalSamplesPerStep represents the total number of samples scanned
	// per step while evaluating a query. Each step should be identical to the
	// TotalSamples when a step is run as an instant query, which means
	// we intentionally do not account for optimizations that happen inside the
	// range query engine that reduce the actual work that happens.
	// For range-vector functions, each step counts the full window at that step.
	TotalSamplesPerStep []int64

	// SamplesRead is the number of samples read (I/O). For range-vector functions
	// in range queries, only new points per step are counted; elsewhere it
	// equals TotalSamples.
	SamplesRead int64

	// SamplesReadPerStep is the number of samples read per step. For
	// range-vector functions, step 0 is the full window, later steps only new points.
	SamplesReadPerStep []int64

	EnablePerStepStats bool
	StartTimestamp     int64
	Interval           int64
	// numSteps is the number of evaluation steps. Always set by
	// NewChildWithStepTracking so that MergeSamplesReadFromSubquery
	// can compute parent windows even when per-step arrays are nil.
	numSteps int
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

// IncrementSamplesReadAtStep increments the samples-read count. Use this when you know the step index.
func (qs *QuerySamples) IncrementSamplesReadAtStep(i int, n int64) {
	if qs == nil {
		return
	}
	qs.SamplesRead += n
	if qs.SamplesReadPerStep != nil && i < len(qs.SamplesReadPerStep) {
		qs.SamplesReadPerStep[i] += n
	}
}

// IncrementSamplesReadAtTimestamp increments the samples-read count. Use this when you only have the step timestamp.
func (qs *QuerySamples) IncrementSamplesReadAtTimestamp(t, n int64) {
	if qs == nil {
		return
	}
	qs.SamplesRead += n
	if qs.SamplesReadPerStep != nil {
		i := int((t - qs.StartTimestamp) / qs.Interval)
		if i >= 0 && i < len(qs.SamplesReadPerStep) {
			qs.SamplesReadPerStep[i] += n
		}
	}
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

// NewChildWithStepTracking creates a child QuerySamples and, when
// enablePerStepStats is true, initializes per-step arrays. The step layout
// (start, interval, numSteps) is always stored so that
// MergeSamplesReadFromSubquery can compute parent windows for windowed
// attribution even when the parent does not track per-step arrays.
func NewChildWithStepTracking(enablePerStepStats bool, start, end, interval int64) *QuerySamples {
	qs := NewQuerySamples(enablePerStepStats)
	qs.StartTimestamp = start
	qs.Interval = interval
	if interval > 0 {
		qs.numSteps = int((end-start)/interval) + 1
	}
	if enablePerStepStats {
		qs.InitStepTracking(start, end, interval)
	}
	return qs
}

// MergeSamplesReadFromSubquery merges only SamplesRead and SamplesReadPerStep from
// the child (subquery) into the parent. TotalSamples and TotalSamplesPerStep are
// not merged, because the outer range-eval loop already counts those when it
// iterates over the pre-computed matrix.
//
// When subqRange > 0, range-windowed attribution is used: only child steps whose
// timestamp falls within a parent step's [parentTs-subqRange, parentTs] window are
// counted. Child steps in gaps between parent windows (when the parent interval
// exceeds subqRange) are not attributed, consistent with range vector selectors
// where only samples inside the selected range are counted.
//
// When subqRange is 0, all child steps are attributed to the nearest parent step.
//
// The parent must have StartTimestamp and Interval set (via NewChildWithStepTracking)
// for windowing to work. Per-step arrays on the parent are optional; when present
// they receive per-step attribution, otherwise only SamplesRead is updated.
func (qs *QuerySamples) MergeSamplesReadFromSubquery(child *QuerySamples, subqRange int64) {
	if qs == nil || child == nil {
		return
	}
	if child.SamplesReadPerStep == nil {
		qs.SamplesRead += child.SamplesRead
		return
	}
	if qs.Interval == 0 {
		qs.SamplesRead += child.SamplesRead
		return
	}

	parentStart := qs.StartTimestamp
	parentInterval := qs.Interval
	numParent := 0
	if qs.SamplesReadPerStep != nil {
		numParent = len(qs.SamplesReadPerStep)
	}
	numParentForWindow := qs.numSteps
	numParentForWindow = max(numParentForWindow, numParent)

	for k := range child.SamplesReadPerStep {
		n := child.SamplesReadPerStep[k]
		if n == 0 {
			continue
		}
		tk := child.StartTimestamp + int64(k)*child.Interval

		outerStep := 0
		if tk > parentStart {
			outerStep = int((tk - parentStart + parentInterval - 1) / parentInterval)
		}

		if subqRange > 0 {
			if outerStep >= numParentForWindow {
				continue
			}
			parentTs := parentStart + int64(outerStep)*parentInterval
			if tk <= parentTs-subqRange {
				continue
			}
		} else if numParent > 0 && outerStep >= numParent {
			outerStep = numParent - 1
		}

		qs.SamplesRead += n
		if numParent > 0 && outerStep < numParent {
			qs.SamplesReadPerStep[outerStep] += n
		}
	}
}

func (qs *QueryTimers) GetSpanTimer(ctx context.Context, qt QueryTiming, observers ...prometheus.Observer) (*SpanTimer, context.Context) {
	return NewSpanTimer(ctx, qt.SpanOperation(), qs.GetTimer(qt), observers...)
}
