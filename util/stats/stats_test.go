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
	const start, end, interval int64 = 1000, 3000, 1000

	parent := NewQuerySamples(true)
	parent.InitStepTracking(start, end, interval)
	parent.SamplesReadPerStep[0] = 1
	parent.SamplesReadPerStep[1] = 2
	parent.SamplesReadPerStep[2] = 3
	parent.SamplesRead = 6

	child := NewChildWithStepTracking(true, start, end, interval)
	child.SamplesReadPerStep[0] = 10
	child.SamplesReadPerStep[1] = 20
	child.SamplesReadPerStep[2] = 30
	child.SamplesRead = 60

	parent.MergeSamplesReadFromSubquery(child)

	require.Equal(t, int64(66), parent.SamplesRead)
	require.Equal(t, []int64{11, 22, 33}, parent.SamplesReadPerStep)
}

func TestMergeSamplesReadFromSubquery_offsetsChildGrid(t *testing.T) {
	parent := NewQuerySamples(true)
	parent.InitStepTracking(1000, 3000, 1000)
	parent.SamplesReadPerStep[1] = 5
	parent.SamplesRead = 5

	child := NewQuerySamples(true)
	child.InitStepTracking(2000, 4000, 1000)
	child.SamplesReadPerStep[1] = 100 // tk=3000 -> parent step 2
	child.SamplesRead = 100

	parent.MergeSamplesReadFromSubquery(child)

	require.Equal(t, int64(105), parent.SamplesRead)
	require.Equal(t, []int64{0, 5, 100}, parent.SamplesReadPerStep)
}

func TestMergeSamplesReadFromSubquery_childBeforeAndAfterParentWindow(t *testing.T) {
	parent := NewQuerySamples(true)
	parent.InitStepTracking(5000, 7000, 1000)
	parent.SamplesRead = 0

	child := NewQuerySamples(true)
	child.InitStepTracking(1000, 9000, 1000)
	for i := range child.SamplesReadPerStep {
		child.SamplesReadPerStep[i] = 1
	}
	child.SamplesRead = int64(len(child.SamplesReadPerStep))

	parent.MergeSamplesReadFromSubquery(child)

	require.Equal(t, child.SamplesRead, parent.SamplesRead)
	var sum int64
	for _, v := range parent.SamplesReadPerStep {
		sum += v
	}
	require.Equal(t, parent.SamplesRead, sum, "per-step sum should match merged SamplesRead")
	// tk 1000..5000 (k=0..4) -> step 0; tk 6000 -> step 1; tk 7000 -> step 2; tk 8000,9000 -> last step.
	require.Equal(t, []int64{5, 1, 3}, parent.SamplesReadPerStep)
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
