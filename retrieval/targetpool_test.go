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
	"testing"
	"time"
)

func testTargetPool(t testing.TB) {
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
			name: "plural schedules",
			inputs: []input{
				{
					address: "http://plural.net",
				},
				{
					address: "http://plural.com",
				},
			},
			outputs: []output{
				{
					address: "http://plural.net",
				},
				{
					address: "http://plural.com",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		pool := NewTargetPool(nil, nil, nil, time.Duration(1))

		for _, input := range scenario.inputs {
			target := target{
				address: input.address,
			}

			target.StopScraper()
			pool.addTarget(&target)
		}

		if len(pool.targets) != len(scenario.outputs) {
			t.Errorf("%s %d. expected TargetPool size to be %d but was %d", scenario.name, i, len(scenario.outputs), len(pool.targets))
		} else {
			for j, output := range scenario.outputs {
				target := pool.Targets()[j]

				if target.Address() != output.address {
					t.Errorf("%s %d.%d. expected Target address to be %s but was %s", scenario.name, i, j, output.address, target.Address())

				}
			}

			if len(pool.targets) != len(scenario.outputs) {
				t.Errorf("%s %d. expected to repopulated with %d elements, got %d", scenario.name, i, len(scenario.outputs), len(pool.targets))
			}
		}
	}
}

func TestTargetPool(t *testing.T) {
	testTargetPool(t)
}

func TestTargetPoolReplaceTargets(t *testing.T) {
	pool := NewTargetPool(nil, nil, nil, time.Duration(1))
	oldTarget1 := &target{
		address: "http://example1.com/metrics.json",
		state:   UNREACHABLE,
	}
	oldTarget1.StopScraper()
	oldTarget2 := &target{
		address: "http://example2.com/metrics.json",
		state:   UNREACHABLE,
	}
	oldTarget2.StopScraper()
	newTarget1 := &target{
		address: "http://example1.com/metrics.json",
		state:   ALIVE,
	}
	newTarget1.StopScraper()
	newTarget2 := &target{
		address: "http://example3.com/metrics.json",
		state:   ALIVE,
	}
	newTarget2.StopScraper()

	pool.addTarget(oldTarget1)
	pool.addTarget(oldTarget2)

	pool.replaceTargets([]Target{newTarget1, newTarget2})

	if len(pool.targets) != 2 {
		t.Errorf("Expected 2 elements in pool, had %d", len(pool.targets))
	}

	target1 := pool.Targets()[0].(*target)
	if target1.state != oldTarget1.state {
		t.Errorf("Wrong first target returned from pool, expected %v, got %v", oldTarget1, target1)
	}
	target2 := pool.Targets()[1].(*target)
	if target2.state != newTarget2.state {
		t.Errorf("Wrong second target returned from pool, expected %v, got %v", newTarget2, target2)
	}
}

func BenchmarkTargetPool(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTargetPool(b)
	}
}
