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
	"net/http"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
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
					address: "single1",
				},
			},
			outputs: []output{
				{
					address: "single1",
				},
			},
		},
		{
			name: "plural schedules",
			inputs: []input{
				{
					address: "plural1",
				},
				{
					address: "plural2",
				},
			},
			outputs: []output{
				{
					address: "plural1",
				},
				{
					address: "plural2",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		pool := NewTargetPool(nil, nil, nopIngester{}, time.Duration(1))

		for _, input := range scenario.inputs {
			target := target{
				address:       input.address,
				newBaseLabels: make(chan clientmodel.LabelSet, 1),
				httpClient:    &http.Client{},
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
		address:       "example1",
		state:         UNREACHABLE,
		stopScraper:   make(chan struct{}),
		newBaseLabels: make(chan clientmodel.LabelSet, 1),
		httpClient:    &http.Client{},
	}
	oldTarget2 := &target{
		address:       "example2",
		state:         UNREACHABLE,
		stopScraper:   make(chan struct{}),
		newBaseLabels: make(chan clientmodel.LabelSet, 1),
		httpClient:    &http.Client{},
	}
	newTarget1 := &target{
		address:       "example1",
		state:         ALIVE,
		stopScraper:   make(chan struct{}),
		newBaseLabels: make(chan clientmodel.LabelSet, 1),
		httpClient:    &http.Client{},
	}
	newTarget2 := &target{
		address:       "example3",
		state:         ALIVE,
		stopScraper:   make(chan struct{}),
		newBaseLabels: make(chan clientmodel.LabelSet, 1),
		httpClient:    &http.Client{},
	}

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
