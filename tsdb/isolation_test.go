// Copyright 2020 The Prometheus Authors
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

package tsdb

import (
	"sync"
	"testing"
)

func TestIsolation(t *testing.T) {
}

func BenchmarkIsolation_10(b *testing.B) {
	benchmarkIsolation(b, 10)
}

func BenchmarkIsolation_100(b *testing.B) {
	benchmarkIsolation(b, 100)
}

func BenchmarkIsolation_1000(b *testing.B) {
	benchmarkIsolation(b, 1000)
}

func BenchmarkIsolation_10000(b *testing.B) {
	benchmarkIsolation(b, 10000)
}

func benchmarkIsolation(b *testing.B, goroutines int) {
	iso := newIsolation()

	wg := sync.WaitGroup{}
	start := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			for i := 0; i < b.N; i++ {
				appendID := iso.newAppendID()
				_ = iso.lowWatermark()

				iso.closeAppend(appendID)
			}
		}()
	}

	b.ResetTimer()
	close(start)
	wg.Wait()
}
