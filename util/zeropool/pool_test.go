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

package zeropool_test

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/util/zeropool"
)

func TestPool(t *testing.T) {
	t.Run("provides correct values", func(t *testing.T) {
		pool := zeropool.New(func() []byte { return make([]byte, 1024) })
		item1 := pool.Get()
		require.Len(t, item1, 1024)

		item2 := pool.Get()
		require.Len(t, item2, 1024)

		pool.Put(item1)
		pool.Put(item2)

		item1 = pool.Get()
		require.Len(t, item1, 1024)

		item2 = pool.Get()
		require.Len(t, item2, 1024)
	})

	t.Run("is not racy", func(t *testing.T) {
		pool := zeropool.New(func() []byte { return make([]byte, 1024) })

		const iterations = 1e6
		const concurrency = math.MaxUint8
		var counter atomic.Int64

		do := make(chan struct{}, 1e6)
		for range int(iterations) {
			do <- struct{}{}
		}
		close(do)

		run := make(chan struct{})
		done := sync.WaitGroup{}
		done.Add(concurrency)
		for i := range concurrency {
			go func(worker int) {
				<-run
				for range do {
					item := pool.Get()
					item[0] = byte(worker)
					counter.Add(1) // Counts and also adds some delay to add raciness.
					if item[0] != byte(worker) {
						panic("wrong value")
					}
					pool.Put(item)
				}
				done.Done()
			}(i)
		}
		close(run)
		done.Wait()
		t.Logf("Done %d iterations", counter.Load())
	})

	t.Run("does not allocate", func(t *testing.T) {
		pool := zeropool.New(func() []byte { return make([]byte, 1024) })
		// Warm up, this will allocate one slice.
		slice := pool.Get()
		pool.Put(slice)

		allocs := testing.AllocsPerRun(1000, func() {
			slice := pool.Get()
			pool.Put(slice)
		})
		// Don't compare to 0, as when passing all the tests the GC could flush the pools during this test and we would allocate.
		// Just check that it's less than 1 on average, which is mostly the same thing.
		require.Less(t, allocs, 1., "Should not allocate.")
	})

	t.Run("zero value is valid", func(t *testing.T) {
		var pool zeropool.Pool[[]byte]
		slice := pool.Get()
		pool.Put(slice)

		allocs := testing.AllocsPerRun(1000, func() {
			slice := pool.Get()
			pool.Put(slice)
		})
		// Don't compare to 0, as when passing all the tests the GC could flush the pools during this test and we would allocate.
		// Just check that it's less than 1 on average, which is mostly the same thing.
		require.Less(t, allocs, 1., "Should not allocate.")
	})
}

func BenchmarkZeropoolPool(b *testing.B) {
	pool := zeropool.New(func() []byte { return make([]byte, 1024) })

	// Warmup
	item := pool.Get()
	pool.Put(item)

	for b.Loop() {
		item := pool.Get()
		pool.Put(item)
	}
}

// BenchmarkSyncPoolValue uses sync.Pool to store values, which makes an allocation on each Put call.
func BenchmarkSyncPoolValue(b *testing.B) {
	pool := sync.Pool{New: func() any {
		return make([]byte, 1024)
	}}

	// Warmup
	item := pool.Get().([]byte)
	pool.Put(item) //nolint:staticcheck // This allocates.

	for b.Loop() {
		item := pool.Get().([]byte)
		pool.Put(item) //nolint:staticcheck // This allocates.
	}
}

// BenchmarkSyncPoolNewPointer uses sync.Pool to store pointers, but it calls Put with a new pointer every time.
func BenchmarkSyncPoolNewPointer(b *testing.B) {
	pool := sync.Pool{New: func() any {
		v := make([]byte, 1024)
		return &v
	}}

	// Warmup
	item := pool.Get().(*[]byte)
	pool.Put(item)

	for b.Loop() {
		item := pool.Get().(*[]byte)
		buf := *item
		pool.Put(&buf)
	}
}

// BenchmarkSyncPoolPointer illustrates the optimal usage of sync.Pool, not always possible.
func BenchmarkSyncPoolPointer(b *testing.B) {
	pool := sync.Pool{New: func() any {
		v := make([]byte, 1024)
		return &v
	}}

	// Warmup
	item := pool.Get().(*[]byte)
	pool.Put(item)

	for b.Loop() {
		item := pool.Get().(*[]byte)
		pool.Put(item)
	}
}
