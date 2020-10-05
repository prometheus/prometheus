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

package stats

import (
	"context"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
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
	TotalSamples int `json:"totalSamples"`
	PeakSamples  int `json:"peakSamples"`
}

// QueryStats currently only holding query timings.
type QueryStats struct {
	Timings queryTimings `json:"timings,omitempty"`
	Samples querySamples `json:"samples,omitempty"`
}

// NewQueryStats makes a QueryStats struct with all QueryTimings found in the
// given TimerGroup.
func NewQueryStats(s *Statistics) *QueryStats {
	var (
		qt      queryTimings
		samples querySamples
		tg      = s.Timers
		sp      = s.Samples
	)

	for s, timer := range tg.TimerGroup.timers {
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

	if sp != (QuerySamples{}) {
		samples = querySamples{
			PeakSamples:  sp.PeakSamples,
			TotalSamples: sp.TotalSamples,
		}
	}

	qs := QueryStats{Timings: qt, Samples: samples}
	return &qs
}

// SpanTimer unifies tracing and timing, to reduce repetition.
type SpanTimer struct {
	timer     *Timer
	observers []prometheus.Observer

	span opentracing.Span
}

func NewSpanTimer(ctx context.Context, operation string, timer *Timer, observers ...prometheus.Observer) (*SpanTimer, context.Context) {
	span, ctx := opentracing.StartSpanFromContext(ctx, operation)
	timer.Start()

	return &SpanTimer{
		timer:     timer,
		observers: observers,

		span: span,
	}, ctx
}

func (s *SpanTimer) Finish() {
	s.timer.Stop()
	s.span.Finish()

	for _, obs := range s.observers {
		obs.Observe(s.timer.ElapsedTime().Seconds())
	}
}

type Statistics struct {
	Timers  QueryTimers
	Samples QuerySamples
}

type QueryTimers struct {
	*TimerGroup
}

type QuerySamples struct {
	// PeakSamples represent the highest count of samples considered
	// while evaluating a query. It corresponds to the peak value of
	// currentSamples.
	PeakSamples int
	// TotalSamples represents the total number of samples scanned
	// while evaluating a query.
	TotalSamples int
}

type Stats struct {
	TimerStats  *QueryTimers
	SampleStats *QuerySamples
}

// Increment increments the total samples count.
func (qs *QuerySamples) Increment(samples int) {
	qs.TotalSamples += samples
}

// UpdatePeak updates the peak number of samples considered in
// evaluation of query if the input samples are in more in
// numbers than the peak.
func (qs *QuerySamples) UpdatePeak(samples int) {
	if samples > qs.PeakSamples {
		qs.PeakSamples = samples
	}
}

func NewQueryTimers() *QueryTimers {
	return &QueryTimers{NewTimerGroup()}
}

func NewQuerySamples() *QuerySamples {
	return &QuerySamples{}
}

func (qs *QueryTimers) GetSpanTimer(ctx context.Context, qt QueryTiming, observers ...prometheus.Observer) (*SpanTimer, context.Context) {
	return NewSpanTimer(ctx, qt.SpanOperation(), qs.TimerGroup.GetTimer(qt), observers...)
}
