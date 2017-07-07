// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, softwar
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import "testing"

// A Benchmark holds context for running a unit test as a benchmark.
type Benchmark struct {
	b         *testing.B
	t         *Test
	iterCount int
}

// NewBenchmark returns an initialized empty Benchmark.
func NewBenchmark(b *testing.B, input string) *Benchmark {
	t, err := NewTest(b, input)
	if err != nil {
		b.Fatalf("Unable to run benchmark: %s", err)
	}
	return &Benchmark{
		b: b,
		t: t,
	}
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	b.b.ReportAllocs()
	b.b.ResetTimer()
	for i := 0; i < b.b.N; i++ {
		if err := b.t.RunAsBenchmark(b); err != nil {
			b.b.Error(err)
		}
		b.iterCount++
	}
}
