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
	"container/heap"
	"github.com/prometheus/prometheus/retrieval/format"
	"github.com/prometheus/prometheus/utility/test"
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

			heap.Push(&pool, &target)
		}

		targets := []Target{}

		if pool.Len() != len(scenario.outputs) {
			t.Errorf("%s %d. expected TargetPool size to be %d but was %d", scenario.name, i, len(scenario.outputs), pool.Len())
		} else {
			for j, output := range scenario.outputs {
				target := heap.Pop(&pool).(Target)

				if target.Address() != output.address {
					t.Errorf("%s %d.%d. expected Target address to be %s but was %s", scenario.name, i, j, output.address, target.Address())

				}
				targets = append(targets, target)
			}

			if pool.Len() != 0 {
				t.Errorf("%s %d. expected pool to be empty, had %d", scenario.name, i, pool.Len())
			}

			if len(targets) != len(scenario.outputs) {
				t.Errorf("%s %d. expected to receive %d elements, got %d", scenario.name, i, len(scenario.outputs), len(targets))
			}

			for _, target := range targets {
				heap.Push(&pool, target)
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
	pool.Push(target)

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

func BenchmarkTargetPool(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTargetPool(b)
	}
}
