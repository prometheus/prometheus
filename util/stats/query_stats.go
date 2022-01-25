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
	"encoding/json"
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
	TotalSamplesPerTime TotalSamplesPerTime `json:"totalSamplesPerTime"`
	TotalSamples        int                 `json:"totalSamples"`
}

// QueryStats currently only holding query timings.
type QueryStats struct {
	Timings queryTimings `json:"timings,omitempty"`
	Samples querySamples `json:"samples"`
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

	samples = querySamples{
		TotalSamples:        sp.TotalSamples,
		TotalSamplesPerTime: sp.TotalSamplesPerTime,
	}

	qs := QueryStats{Timings: qt, Samples: samples}
	return &qs
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

type TotalSamplesPerTime map[int64]int

func (p *TotalSamplesPerTime) MarshalJSON() ([]byte, error) {
	toMs := map[int64]int{}

	for k, v := range *p {
		toMs[k/1000] = v
	}

	return json.Marshal(toMs)
}

type QuerySamples struct {
	// TotalSamplesPerTime represents the total number of samples scanned
	// while evaluating a query.
	TotalSamples        int
	TotalSamplesPerTime TotalSamplesPerTime
}

type Stats struct {
	TimerStats  *QueryTimers
	SampleStats *QuerySamples
}

// IncrementSamples increments the total samples count.
func (qs *QuerySamples) IncrementSamples(t int64, samples int) {
	if qs == nil {
		return
	}
	qs.TotalSamplesPerTime[t] += samples
	qs.TotalSamples += samples
}

func NewQueryTimers() *QueryTimers {
	return &QueryTimers{NewTimerGroup()}
}

func NewQuerySamples() *QuerySamples {
	return &QuerySamples{TotalSamplesPerTime: TotalSamplesPerTime{}}
}

func (qs *QueryTimers) GetSpanTimer(ctx context.Context, qt QueryTiming, observers ...prometheus.Observer) (*SpanTimer, context.Context) {
	return NewSpanTimer(ctx, qt.SpanOperation(), qs.TimerGroup.GetTimer(qt), observers...)
}
