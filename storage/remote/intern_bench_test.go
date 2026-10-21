// Copyright The Prometheus Authors
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

package remote

import (
	"strconv"
	"sync"
	"testing"
)

// BenchmarkIntern_Contention simulates the concurrent intern scenario that
// occurs when WAL replay or multiple remote write queues process series labels.
// Different goroutines intern distinct strings so the pool map grows large and
// the global lock becomes the contended resource.
func BenchmarkIntern_Contention(b *testing.B) {
	pool := newPool()

	// Pre-populate with a large number of unique strings to simulate a
	// well-populated pool (mirrors steady-state remote write processing).
	const prePopulate = 10000
	for i := range prePopulate {
		pool.intern("series_" + strconv.Itoa(i))
	}

	b.ResetTimer()
	b.SetParallelism(8)

	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			// Each goroutine interns a unique string so it always hits
			// the lock-contended path (new entry creation or shared entry).
			s := "metric_" + strconv.Itoa(counter) + "_" + strconv.Itoa(b.N)
			pool.intern(s)
			counter++
		}
	})
}

// BenchmarkIntern_SingleString is the low-contention case where all
// goroutines intern the same string (common label key).
func BenchmarkIntern_SingleString(b *testing.B) {
	pool := newPool()
	const s = "common_label_key"
	pool.intern(s)

	b.ResetTimer()
	b.SetParallelism(8)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.intern(s)
		}
	})
}

// BenchmarkRelease_Contention measures concurrent release on a populated pool.
func BenchmarkRelease_Contention(b *testing.B) {
	pool := newPool()

	const prePopulate = 10000
	strs := make([]string, prePopulate)
	for i := range prePopulate {
		strs[i] = "series_" + strconv.Itoa(i)
		pool.intern(strs[i])
	}

	b.ResetTimer()
	b.SetParallelism(8)

	var mu sync.Mutex
	idx := 0
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			i := idx % prePopulate
			idx++
			mu.Unlock()
			pool.intern(strs[i]) // Add ref so release doesn't immediately delete
			pool.release(strs[i])
		}
	})
}
