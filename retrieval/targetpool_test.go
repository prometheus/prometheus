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

// import (
// 	"container/heap"
// 	"testing"
// 	"time"
// )

type literalScheduler time.Time

func (s literalScheduler) ScheduledFor() time.Time {
	return time.Time(s)
}

func (s literalScheduler) Reschedule(earliest time.Time, future TargetState) {
}

func TestTargetPool(t *testing.T) {
	type expectation struct {
		size int
	}

// 	type input struct {
// 		address      string
// 		scheduledFor time.Time
// 	}

// 	type output struct {
// 		address string
// 	}

// 	var scenarios = []struct {
// 		name    string
// 		outputs []output
// 		inputs  []input
// 	}{
// 		{
// 			name:    "empty",
// 			inputs:  []input{},
// 			outputs: []output{},
// 		},
// 		{
// 			name: "single element",
// 			inputs: []input{
// 				{
// 					address: "http://single.com",
// 				},
// 			},
// 			outputs: []output{
// 				{
// 					address: "http://single.com",
// 				},
// 			},
// 		},
// 		{
// 			name: "plural descending schedules",
// 			inputs: []input{
// 				{
// 					address:      "http://plural-descending.com",
// 					scheduledFor: time.Date(2013, 1, 4, 12, 0, 0, 0, time.UTC),
// 				},
// 				{
// 					address:      "http://plural-descending.net",
// 					scheduledFor: time.Date(2013, 1, 4, 11, 0, 0, 0, time.UTC),
// 				},
// 			},
// 			outputs: []output{
// 				{
// 					address: "http://plural-descending.net",
// 				},
// 				{
// 					address: "http://plural-descending.com",
// 				},
// 			},
// 		},
// 		{
// 			name: "plural ascending schedules",
// 			inputs: []input{
// 				{
// 					address:      "http://plural-ascending.net",
// 					scheduledFor: time.Date(2013, 1, 4, 11, 0, 0, 0, time.UTC),
// 				},
// 				{
// 					address:      "http://plural-ascending.com",
// 					scheduledFor: time.Date(2013, 1, 4, 12, 0, 0, 0, time.UTC),
// 				},
// 			},
// 			outputs: []output{
// 				{
// 					address: "http://plural-ascending.net",
// 				},
// 				{
// 					address: "http://plural-ascending.com",
// 				},
// 			},
// 		},
// 	}

// 	for i, scenario := range scenarios {
// 		pool := TargetPool{}

		for _, input := range scenario.inputs {
			target := Target{
				Address:   input.address,
				scheduler: literalScheduler(input.scheduledFor),
			}

// 			heap.Push(&pool, &target)
// 		}

// 		if pool.Len() != len(scenario.outputs) {
// 			t.Errorf("%s %d. expected TargetPool size to be %d but was %d", scenario.name, i, len(scenario.outputs), pool.Len())
// 		} else {
// 			for j, output := range scenario.outputs {
// 				target := heap.Pop(&pool).(*Target)

// 				if target.Address != output.address {
// 					t.Errorf("%s %d.%d. expected Target address to be %s but was %s", scenario.name, i, j, output.address, target.Address)

// 				}
// 			}
// 		}
// 	}
// }
