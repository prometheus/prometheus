// Copyright 2013 Prometheus Team
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

package retrieval

import (
	"github.com/matttproud/prometheus/utility/test"
	"testing"
	"time"
)

type fakeHealthReporter struct {
	index      int
	stateQueue []TargetState
}

func (h fakeHealthReporter) State() (state TargetState) {
	state = h.stateQueue[h.index]

	h.index++

	return
}

type fakeTimeProvider struct {
	index     int
	timeQueue []time.Time
}

func (t *fakeTimeProvider) Now() (time time.Time) {
	time = t.timeQueue[t.index]

	t.index++

	return
}

func testHealthScheduler(t test.Tester) {
	now := time.Now()
	var scenarios = []struct {
		futureHealthState []TargetState
		preloadedTimes    []time.Time
		expectedSchedule  []time.Time
	}{
		// The behavior discussed in healthScheduler.Reschedule should be read
		// fully to understand the whys and wherefores.
		{
			futureHealthState: []TargetState{UNKNOWN, ALIVE, ALIVE},
			preloadedTimes:    []time.Time{now, now.Add(time.Minute), now.Add(time.Minute * 2)},
			expectedSchedule:  []time.Time{now, now.Add(time.Minute), now.Add(time.Minute * 2)},
		},
		{
			futureHealthState: []TargetState{UNKNOWN, UNREACHABLE, UNREACHABLE},
			preloadedTimes:    []time.Time{now, now.Add(time.Minute), now.Add(time.Minute * 2)},
			expectedSchedule:  []time.Time{now, now.Add(time.Second * 2), now.Add(time.Minute).Add(time.Second * 4)},
		},
		{
			futureHealthState: []TargetState{UNKNOWN, UNREACHABLE, ALIVE},
			preloadedTimes:    []time.Time{now, now.Add(time.Minute), now.Add(time.Minute * 2)},
			expectedSchedule:  []time.Time{now, now.Add(time.Second * 2), now.Add(time.Minute * 2)},
		},
		{
			futureHealthState: []TargetState{UNKNOWN, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE, UNREACHABLE},
			preloadedTimes:    []time.Time{now, now.Add(time.Minute), now.Add(time.Minute * 2), now.Add(time.Minute * 3), now.Add(time.Minute * 4), now.Add(time.Minute * 5), now.Add(time.Minute * 6), now.Add(time.Minute * 7), now.Add(time.Minute * 8), now.Add(time.Minute * 9), now.Add(time.Minute * 10), now.Add(time.Minute * 11), now.Add(time.Minute * 12)},
			expectedSchedule:  []time.Time{now, now.Add(time.Second * 2), now.Add(time.Minute * 1).Add(time.Second * 4), now.Add(time.Minute * 2).Add(time.Second * 8), now.Add(time.Minute * 3).Add(time.Second * 16), now.Add(time.Minute * 4).Add(time.Second * 32), now.Add(time.Minute * 5).Add(time.Second * 64), now.Add(time.Minute * 6).Add(time.Second * 128), now.Add(time.Minute * 7).Add(time.Second * 256), now.Add(time.Minute * 8).Add(time.Second * 512), now.Add(time.Minute * 9).Add(time.Second * 1024), now.Add(time.Minute * 10).Add(time.Minute * 30), now.Add(time.Minute * 11).Add(time.Minute * 30)},
		},
	}

	for i, scenario := range scenarios {
		provider := &fakeTimeProvider{}
		for _, time := range scenario.preloadedTimes {
			provider.timeQueue = append(provider.timeQueue, time)
		}

		reporter := fakeHealthReporter{}
		for _, state := range scenario.futureHealthState {
			reporter.stateQueue = append(reporter.stateQueue, state)
		}
		if len(scenario.preloadedTimes) != len(scenario.futureHealthState) || len(scenario.futureHealthState) != len(scenario.expectedSchedule) {
			t.Fatalf("%d. times and health reports and next time lengths were not equal.", i)
		}

		timer := timer{
			provider: provider,
		}

		scheduler := healthScheduler{
			timer:        timer,
			target:       reporter,
			scheduledFor: now,
		}

		for j := 0; j < len(scenario.preloadedTimes); j++ {
			futureState := scenario.futureHealthState[j]
			scheduler.Reschedule(scenario.preloadedTimes[j], futureState)
			nextSchedule := scheduler.ScheduledFor()
			if nextSchedule != scenario.expectedSchedule[j] {
				t.Errorf("%d.%d. Expected to be scheduled to %s, got %s", i, j, scenario.expectedSchedule[j], nextSchedule)
			}
		}
	}
}

func TestHealthScheduler(t *testing.T) {
	testHealthScheduler(t)
}

func BenchmarkHealthScheduler(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testHealthScheduler(b)
	}
}
