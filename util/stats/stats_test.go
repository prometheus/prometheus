// Copyright 2017 The Prometheus Authors
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
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestTimerGroupNewTimer(t *testing.T) {
	tg := NewTimerGroup()
	timer := tg.GetTimer(ExecTotalTime)
	duration := timer.Duration()
	require.NotEqual(t, 0, duration, "Expected duration of 0, but it was %f instead.", duration)
	minimum := 2 * time.Millisecond
	timer.Start()
	time.Sleep(minimum)
	timer.Stop()
	duration = timer.Duration()
	require.Greater(t, duration, 0.0, "Expected duration greater than 0, but it was %f instead.", duration)
	elapsed := timer.ElapsedTime()
	require.Greater(t, elapsed, minimum,
		"Expected elapsed time to be greater than time slept, elapsed was %d, and time slept was %d.",
		elapsed.Nanoseconds(), minimum)
}

func TestQueryStatsWithTimers(t *testing.T) {
	qt := NewQueryTimers()
	timer := qt.GetTimer(ExecTotalTime)
	timer.Start()
	time.Sleep(2 * time.Millisecond)
	timer.Stop()

	qs := NewQueryStats(qt)
	actual, err := json.Marshal(qs)
	require.NoError(t, err, "Unexpected error during serialization: %v", err)
	// Timing value is one of multiple fields, unit is seconds (float).
	match, err := regexp.MatchString(`[,{]"execTotalTime":\d+\.\d+[,}]`, string(actual))
	require.NoError(t, err, "Unexpected error while matching string: %v", err)
	require.True(t, match, "Expected timings with one non-zero entry, but got %s.", actual)
}

func TestQueryStatsWithSpanTimers(t *testing.T) {
	qt := NewQueryTimers()
	ctx := &testutil.MockContext{DoneCh: make(chan struct{})}
	qst, _ := qt.GetSpanTimer(ctx, ExecQueueTime, prometheus.NewSummary(prometheus.SummaryOpts{}))
	time.Sleep(5 * time.Millisecond)
	qst.Finish()
	qs := NewQueryStats(qt)
	actual, err := json.Marshal(qs)
	require.NoError(t, err, "Unexpected error during serialization: %v", err)
	// Timing value is one of multiple fields, unit is seconds (float).
	match, err := regexp.MatchString(`[,{]"execQueueTime":\d+\.\d+[,}]`, string(actual))
	require.NoError(t, err, "Unexpected error while matching string: %v", err)
	require.True(t, match, "Expected timings with one non-zero entry, but got %s.", actual)
}

func TestTimerGroup(t *testing.T) {
	tg := NewTimerGroup()
	execTotalTimer := tg.GetTimer(ExecTotalTime)
	require.Equal(t, "Exec total time: 0s", tg.GetTimer(ExecTotalTime).String(), "Expected string %s, but got %s", "", execTotalTimer.String())

	execQueueTimer := tg.GetTimer(ExecQueueTime)
	require.Equal(t, "Exec queue wait time: 0s", tg.GetTimer(ExecQueueTime).String(), "Expected string %s, but got %s", "", execQueueTimer.String())

	innerEvalTimer := tg.GetTimer(InnerEvalTime)
	require.Equal(t, "Inner eval time: 0s", tg.GetTimer(InnerEvalTime).String(), "Expected string %s, but got %s", "", innerEvalTimer.String())

	queryPreparationTimer := tg.GetTimer(QueryPreparationTime)
	require.Equal(t, "Query preparation time: 0s", tg.GetTimer(QueryPreparationTime).String(), "Expected string %s, but got %s", "", queryPreparationTimer.String())

	resultSortTimer := tg.GetTimer(ResultSortTime)
	require.Equal(t, "Result sorting time: 0s", tg.GetTimer(ResultSortTime).String(), "Expected string %s, but got %s", "", resultSortTimer.String())

	evalTotalTimer := tg.GetTimer(EvalTotalTime)
	require.Equal(t, "Eval total time: 0s", tg.GetTimer(EvalTotalTime).String(), "Expected string %s, but got %s", "", evalTotalTimer.String())

	actual := tg.String()
	expected := "Exec total time: 0s\nExec queue wait time: 0s\nInner eval time: 0s\nQuery preparation time: 0s\nResult sorting time: 0s\nEval total time: 0s\n"
	require.Equal(t, expected, actual, "Expected timerGroup string %s, but got %s.", expected, actual)
}
