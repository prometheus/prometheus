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
	"github.com/matttproud/prometheus/retrieval/format"
	"github.com/matttproud/prometheus/utility/test"
	"testing"
	"time"
)

type fakeTarget struct {
	scrapeCount   int
	schedules     []time.Time
	interval      time.Duration
	scheduleIndex int
}

func (t fakeTarget) Address() string {
	return "fake"
}

func (t fakeTarget) Interval() time.Duration {
	return t.interval
}

func (t *fakeTarget) Scrape(e time.Time, r chan format.Result) error {
	t.scrapeCount++

	return nil
}

func (t fakeTarget) State() TargetState {
	return ALIVE
}

func (t *fakeTarget) scheduledFor() (time time.Time) {
	time = t.schedules[t.scheduleIndex]
	t.scheduleIndex++

	return
}

func testTargetManager(t test.Tester) {
	results := make(chan format.Result, 5)
	targetManager := NewTargetManager(results, 3)

	target1GroupA := &fakeTarget{
		schedules: []time.Time{time.Now()},
		interval:  time.Minute,
	}
	target2GroupA := &fakeTarget{
		schedules: []time.Time{time.Now()},
		interval:  time.Minute,
	}

	targetManager.Add(target1GroupA)
	targetManager.Add(target2GroupA)

	target1GroupB := &fakeTarget{
		schedules: []time.Time{time.Now()},
		interval:  time.Minute * 2,
	}

	targetManager.Add(target1GroupB)

}

func TestTargetManager(t *testing.T) {
	testTargetManager(t)
}

func BenchmarkTargetManager(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTargetManager(b)
	}
}
