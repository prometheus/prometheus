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
		pool := NewTargetPool(nil, nil, nopIngester{}, time.Duration(1))

		for _, input := range scenario.inputs {
			target := target{
				address: input.address,
			}

			pool.addTarget(&target)
		}

		if len(pool.targetsByAddress) != len(scenario.outputs) {
			t.Errorf("%s %d. expected TargetPool size to be %d but was %d", scenario.name, i, len(scenario.outputs), len(pool.targetsByAddress))
		} else {
			for j, output := range scenario.outputs {
				if target, ok := pool.targetsByAddress[output.address]; !ok {
					t.Errorf("%s %d.%d. expected Target address to be %s but was %s", scenario.name, i, j, output.address, target.Address())
				}
			}

			if len(pool.targetsByAddress) != len(scenario.outputs) {
				t.Errorf("%s %d. expected to repopulated with %d elements, got %d", scenario.name, i, len(scenario.outputs), len(pool.targetsByAddress))
			}
		}
	}
}

func TestTargetPool(t *testing.T) {
	testTargetPool(t)
}

func TestTargetPoolReplaceTargets(t *testing.T) {
	pool := NewTargetPool(nil, nil, nopIngester{}, time.Duration(1))
	oldTarget1 := &target{
		address:     "example1",
		state:       UNREACHABLE,
		stopScraper: make(chan bool, 1),
	}
	oldTarget2 := &target{
		address:     "example2",
		state:       UNREACHABLE,
		stopScraper: make(chan bool, 1),
	}
	newTarget1 := &target{
		address:     "example1",
		state:       ALIVE,
		stopScraper: make(chan bool, 1),
	}
	newTarget2 := &target{
		address:     "example3",
		state:       ALIVE,
		stopScraper: make(chan bool, 1),
	}

	oldTarget1.StopScraper()
	oldTarget2.StopScraper()
	newTarget2.StopScraper()

	pool.addTarget(oldTarget1)
	pool.addTarget(oldTarget2)

	pool.ReplaceTargets([]Target{newTarget1, newTarget2})

	if len(pool.targetsByAddress) != 2 {
		t.Errorf("Expected 2 elements in pool, had %d", len(pool.targetsByAddress))
	}

	if pool.targetsByAddress["example1"].State() != oldTarget1.State() {
		t.Errorf("target1 channel has changed")
	}
	if pool.targetsByAddress["example3"].State() == oldTarget2.State() {
		t.Errorf("newTarget2 channel same as oldTarget2's")
	}

}

func BenchmarkTargetPool(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTargetPool(b)
	}
}
