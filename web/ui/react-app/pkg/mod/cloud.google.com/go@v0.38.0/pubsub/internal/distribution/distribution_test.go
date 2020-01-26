// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package distribution

import (
	"sync"
	"testing"
)

func TestDistribution(t *testing.T) {
	// These tests come from examples in https://en.wikipedia.org/wiki/Percentile#The_nearest-rank_method
	tests := []struct {
		// values in distribution
		vals []int

		// percentiles and expected percentile values
		pp []float64
		vv []int
	}{
		{
			vals: []int{15, 20, 35, 40, 50},
			pp:   []float64{0.05, 0.3, 0.4, 0.5, 1},
			vv:   []int{15, 20, 20, 35, 50},
		},
		{
			vals: []int{3, 6, 7, 8, 8, 10, 13, 15, 16, 20},
			pp:   []float64{0.25, 0.5, 0.75, 1},
			vv:   []int{7, 8, 15, 20},
		},
		{
			vals: []int{3, 6, 7, 8, 8, 9, 10, 13, 15, 16, 20},
			pp:   []float64{0.25, 0.5, 0.75, 1},
			vv:   []int{7, 9, 15, 20},
		},
	}

	maxVal := 0
	for _, tst := range tests {
		for _, v := range tst.vals {
			if maxVal < v {
				maxVal = v
			}
		}
	}

	for _, tst := range tests {
		d := New(maxVal + 1)
		for _, v := range tst.vals {
			d.Record(v)
		}
		for i, p := range tst.pp {
			got, want := d.Percentile(p), tst.vv[i]
			if got != want {
				t.Errorf("d=%v, d.Percentile(%f)=%d, want %d", d, p, got, want)
			}
		}
	}
}

func TestRace(t *testing.T) {
	const N int = 1e3
	const parallel = 2

	d := New(N)

	var wg sync.WaitGroup
	wg.Add(parallel)
	for i := 0; i < parallel; i++ {
		go func() {
			for i := 0; i < N; i++ {
				d.Record(i)
			}
			wg.Done()
		}()
	}

	for i := 0; i < N; i++ {
		if p := d.Percentile(0.5); p > N {
			t.Fatalf("d.Percentile(0.5)=%d, expected to be at most %d", p, N)
		}
	}
}
