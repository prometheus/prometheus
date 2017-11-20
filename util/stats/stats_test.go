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
)

func TestTimerGroupNewTimer(t *testing.T) {
	tg := NewTimerGroup()
	timer := tg.GetTimer(ExecTotalTime)
	if duration := timer.Duration(); duration != 0 {
		t.Fatalf("Expected duration of 0, but it was %f instead.", duration)
	}
	minimum := 2 * time.Millisecond
	timer.Start()
	time.Sleep(minimum)
	timer.Stop()
	if duration := timer.Duration(); duration == 0 {
		t.Fatalf("Expected duration greater than 0, but it was %f instead.", duration)
	}
	if elapsed := timer.ElapsedTime(); elapsed < minimum {
		t.Fatalf("Expected elapsed time to be greater than time slept, elapsed was %d, and time slept was %d.", elapsed.Nanoseconds(), minimum)
	}
}

func TestQueryStatsWithTimers(t *testing.T) {
	tg := NewTimerGroup()
	timer := tg.GetTimer(ExecTotalTime)
	timer.Start()
	time.Sleep(2 * time.Millisecond)
	timer.Stop()

	var qs *QueryStats
	qs = NewQueryStats(tg)
	actual, err := json.Marshal(qs)
	if err != nil {
		t.Fatalf("Unexpected error during serialization: %v", err)
	}
	// Timing value is one of multiple fields, unit is seconds (float).
	match, err := regexp.MatchString(`[,{]"execTotalTime":\d+\.\d+[,}]`, string(actual))
	if err != nil {
		t.Fatalf("Unexpected error while matching string: %v", err)
	}
	if !match {
		t.Fatalf("Expected timings with one non-zero entry, but got %s.", actual)
	}
}
