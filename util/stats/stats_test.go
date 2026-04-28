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
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestTimerGroupNewTimer(t *testing.T) {
	tg := NewTimerGroup()
	timer := tg.GetTimer(ExecTotalTime)
	duration := timer.Duration()
	require.Equal(t, 0.0, duration, "Expected duration equal 0")
	minimum := 2 * time.Millisecond
	timer.Start()
	time.Sleep(minimum)
	timer.Stop()
	duration = timer.Duration()
	require.Greater(t, duration, 0.0, "Expected duration greater than 0")
	elapsed := timer.ElapsedTime()
	require.GreaterOrEqual(t, elapsed, minimum,
		"Expected elapsed time to be greater than time slept.")
}

func TestQueryStatsWithTimersAndSamples(t *testing.T) {
	qt := NewQueryTimers()
	qs := NewQuerySamples(true)
	qs.InitStepTracking(20001000, 25001000, 1000000)
	timer := qt.GetTimer(ExecTotalTime)
	timer.Start()
	time.Sleep(2 * time.Millisecond)
	timer.Stop()
	qs.IncrementSamplesAtTimestamp(20001000, 5)
	qs.IncrementSamplesAtTimestamp(25001000, 5)
	qs.IncrementSamplesReadAtTimestamp(20001000, 5)
	qs.IncrementSamplesReadAtTimestamp(25001000, 5)

	qstats := NewQueryStats(&Statistics{Timers: qt, Samples: qs})
	actual, err := json.Marshal(qstats)
	require.NoError(t, err, "unexpected error during serialization")
	// Timing value is one of multiple fields, unit is seconds (float).
	match, err := regexp.MatchString(`[,{]"execTotalTime":\d+\.\d+[,}]`, string(actual))
	require.NoError(t, err, "unexpected error while matching string")
	require.True(t, match, "Expected timings with one non-zero entry.")

	require.Regexpf(t, `[,{]"totalQueryableSamples":10[,}]`, string(actual), "expected totalQueryableSamples")
	require.Regexpf(t, `[,{]"totalQueryableSamplesPerStep":\[\[20001,5\],\[21001,0\],\[22001,0\],\[23001,0\],\[24001,0\],\[25001,5\]\]`, string(actual), "expected totalQueryableSamplesPerStep")
	require.Regexpf(t, `[,{]"samplesRead":10[,}]`, string(actual), "expected samplesRead")
	require.Regexpf(t, `[,{]"samplesReadPerStep":\[\[20001,5\],\[21001,0\],\[22001,0\],\[23001,0\],\[24001,0\],\[25001,5\]\]`, string(actual), "expected samplesReadPerStep")
}

func TestQueryStatsWithSpanTimers(t *testing.T) {
	qt := NewQueryTimers()
	qs := NewQuerySamples(false)
	ctx := &testutil.MockContext{DoneCh: make(chan struct{})}
	qst, _ := qt.GetSpanTimer(ctx, ExecQueueTime, prometheus.NewSummary(prometheus.SummaryOpts{}))
	time.Sleep(5 * time.Millisecond)
	qst.Finish()
	qstats := NewQueryStats(&Statistics{Timers: qt, Samples: qs})
	actual, err := json.Marshal(qstats)
	require.NoError(t, err, "unexpected error during serialization")
	// Timing value is one of multiple fields, unit is seconds (float).
	match, err := regexp.MatchString(`[,{]"execQueueTime":\d+\.\d+[,}]`, string(actual))
	require.NoError(t, err, "unexpected error while matching string")
	require.True(t, match, "Expected timings with one non-zero entry.")
}

func TestMergeSamplesReadFromSubquery(t *testing.T) {
	type stepGrid struct {
		start, end, interval int64
		perStep              []int64
	}
	cases := []struct {
		name            string
		parent          stepGrid
		child           stepGrid
		wantPerStep     []int64
		wantSamplesRead int64
	}{
		{
			name:            "alignedChildGridAddsToParentSteps",
			parent:          stepGrid{1000, 3000, 1000, []int64{1, 2, 3}},
			child:           stepGrid{1000, 3000, 1000, []int64{10, 20, 30}},
			wantPerStep:     []int64{11, 22, 33},
			wantSamplesRead: 66,
		},
		{
			name:            "offsetChildGridAttributesByTimestamp",
			parent:          stepGrid{1000, 3000, 1000, []int64{0, 5, 0}},
			child:           stepGrid{2000, 4000, 1000, []int64{0, 100, 0}}, // tk=3000 -> parent step 2.
			wantPerStep:     []int64{0, 5, 100},
			wantSamplesRead: 105,
		},
		{
			// Child spans 1000..9000 (9 steps). Steps with tk <= parentStart=5000
			// (k=0..4) clamp to parent step 0; tk=6000 -> step 1; tk=7000 -> step 2;
			// tk=8000,9000 clamp to the last parent step (also 2).
			name:            "childBeforeAndAfterParentWindowClampsToEndpoints",
			parent:          stepGrid{5000, 7000, 1000, []int64{0, 0, 0}},
			child:           stepGrid{1000, 9000, 1000, []int64{1, 1, 1, 1, 1, 1, 1, 1, 1}},
			wantPerStep:     []int64{5, 1, 3},
			wantSamplesRead: 9,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := NewQuerySamples(true)
			parent.InitStepTracking(tc.parent.start, tc.parent.end, tc.parent.interval)
			copy(parent.SamplesReadPerStep, tc.parent.perStep)
			for _, v := range tc.parent.perStep {
				parent.SamplesRead += v
			}

			child := NewChildWithStepTracking(true, tc.child.start, tc.child.end, tc.child.interval)
			copy(child.SamplesReadPerStep, tc.child.perStep)
			for _, v := range tc.child.perStep {
				child.SamplesRead += v
			}

			parent.MergeSamplesReadFromSubquery(child)

			require.Equal(t, tc.wantSamplesRead, parent.SamplesRead)
			require.Equal(t, tc.wantPerStep, parent.SamplesReadPerStep)
		})
	}
}

func TestTimerGroup(t *testing.T) {
	tg := NewTimerGroup()
	require.Equal(t, "Exec total time: 0s", tg.GetTimer(ExecTotalTime).String())

	require.Equal(t, "Exec queue wait time: 0s", tg.GetTimer(ExecQueueTime).String())

	require.Equal(t, "Inner eval time: 0s", tg.GetTimer(InnerEvalTime).String())

	require.Equal(t, "Query preparation time: 0s", tg.GetTimer(QueryPreparationTime).String())

	require.Equal(t, "Result sorting time: 0s", tg.GetTimer(ResultSortTime).String())

	require.Equal(t, "Eval total time: 0s", tg.GetTimer(EvalTotalTime).String())

	actual := tg.String()
	expected := "Exec total time: 0s\nExec queue wait time: 0s\nInner eval time: 0s\nQuery preparation time: 0s\nResult sorting time: 0s\nEval total time: 0s\n"
	require.Equal(t, expected, actual)
}
