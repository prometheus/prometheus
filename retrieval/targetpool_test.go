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
	"github.com/prometheus/prometheus/retrieval/format"
	"github.com/prometheus/prometheus/utility/test"
	"sort"
	"testing"
	"time"
)

func testTargetPool(t test.Tester) {
	type expectation struct {
		size int
	}

	type input struct {
		address      string
		scheduledFor time.Time
	}

	type output struct {
		address string
	}

	var scenarios = []struct {
		name    string
		inputs  []input
		outputs []output
	}{
		{
			name:    "empty",
			inputs:  []input{},
			outputs: []output{},
		},
		{
			name: "single element",
			inputs: []input{
				{
					address: "http://single.com",
				},
			},
			outputs: []output{
				{
					address: "http://single.com",
				},
			},
		},
		{
			name: "plural descending schedules",
			inputs: []input{
				{
					address:      "http://plural-descending.com",
					scheduledFor: time.Date(2013, 1, 4, 12, 0, 0, 0, time.UTC),
				},
				{
					address:      "http://plural-descending.net",
					scheduledFor: time.Date(2013, 1, 4, 11, 0, 0, 0, time.UTC),
				},
			},
			outputs: []output{
				{
					address: "http://plural-descending.net",
				},
				{
					address: "http://plural-descending.com",
				},
			},
		},
		{
			name: "plural ascending schedules",
			inputs: []input{
				{
					address:      "http://plural-ascending.net",
					scheduledFor: time.Date(2013, 1, 4, 11, 0, 0, 0, time.UTC),
				},
				{
					address:      "http://plural-ascending.com",
					scheduledFor: time.Date(2013, 1, 4, 12, 0, 0, 0, time.UTC),
				},
			},
			outputs: []output{
				{
					address: "http://plural-ascending.net",
				},
				{
					address: "http://plural-ascending.com",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		pool := TargetPool{}

		for _, input := range scenario.inputs {
			target := target{
				address:   input.address,
				scheduler: literalScheduler(input.scheduledFor),
			}

			pool.addTarget(&target)
		}
		sort.Sort(pool)

		if pool.Len() != len(scenario.outputs) {
			t.Errorf("%s %d. expected TargetPool size to be %d but was %d", scenario.name, i, len(scenario.outputs), pool.Len())
		} else {
			for j, output := range scenario.outputs {
				target := pool.targets[j]

				if target.Address() != output.address {
					t.Errorf("%s %d.%d. expected Target address to be %s but was %s", scenario.name, i, j, output.address, target.Address())

				}
			}

			if pool.Len() != len(scenario.outputs) {
				t.Errorf("%s %d. expected to repopulated with %d elements, got %d", scenario.name, i, len(scenario.outputs), pool.Len())
			}
		}
	}
}

func TestTargetPool(t *testing.T) {
	testTargetPool(t)
}

func TestTargetPoolIterationWithUnhealthyTargetsFinishes(t *testing.T) {
	pool := TargetPool{}
	target := &target{
		address:   "http://example.com/metrics.json",
		scheduler: literalScheduler(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	pool.addTarget(target)

	done := make(chan bool)
	go func() {
		pool.runIteration(make(chan format.Result), time.Duration(0))
		done <- true
	}()

	select {
	case <-done:
		break
	case <-time.After(time.Duration(1) * time.Second):
		t.Fatalf("Targetpool iteration is stuck")
	}
}

func TestTargetPoolReplaceTargets(t *testing.T) {
	pool := TargetPool{}
	oldTarget1 := &target{
		address:   "http://example1.com/metrics.json",
		scheduler: literalScheduler(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)),
		state:     UNREACHABLE,
	}
	oldTarget2 := &target{
		address:   "http://example2.com/metrics.json",
		scheduler: literalScheduler(time.Date(7500, 1, 1, 0, 0, 0, 0, time.UTC)),
		state:     UNREACHABLE,
	}
	newTarget1 := &target{
		address:   "http://example1.com/metrics.json",
		scheduler: literalScheduler(time.Date(5000, 1, 1, 0, 0, 0, 0, time.UTC)),
		state:     ALIVE,
	}
	newTarget2 := &target{
		address:   "http://example3.com/metrics.json",
		scheduler: literalScheduler(time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)),
		state:     ALIVE,
	}

	pool.addTarget(oldTarget1)
	pool.addTarget(oldTarget2)

	pool.replaceTargets([]Target{newTarget1, newTarget2})
	sort.Sort(pool)

	if pool.Len() != 2 {
		t.Errorf("Expected 2 elements in pool, had %d", pool.Len())
	}

	target1 := pool.targets[0].(*target)
	if target1.state != newTarget1.state {
		t.Errorf("Wrong first target returned from pool, expected %v, got %v", newTarget2, target1)
	}
	target2 := pool.targets[1].(*target)
	if target2.state != oldTarget1.state {
		t.Errorf("Wrong second target returned from pool, expected %v, got %v", oldTarget1, target2)
	}
}

func BenchmarkTargetPool(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTargetPool(b)
	}
}
