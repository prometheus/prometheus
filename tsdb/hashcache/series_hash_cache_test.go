package hashcache

import (
	"crypto/rand"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
)

func TestSeriesHashCache(t *testing.T) {
	// Set the max cache size to store at most 1 entry per generation,
	// so that we test the GC logic too.
	c := NewSeriesHashCache(numGenerations * approxBytesPerEntry)

	block1 := c.GetBlockCache("1")
	assertFetch(t, block1, 1, 0, false)
	block1.Store(1, 100)
	assertFetch(t, block1, 1, 100, true)

	block2 := c.GetBlockCache("2")
	assertFetch(t, block2, 1, 0, false)
	block2.Store(1, 1000)
	assertFetch(t, block2, 1, 1000, true)

	block3 := c.GetBlockCache("3")
	assertFetch(t, block1, 1, 100, true)
	assertFetch(t, block2, 1, 1000, true)
	assertFetch(t, block3, 1, 0, false)

	// Get again the block caches.
	block1 = c.GetBlockCache("1")
	block2 = c.GetBlockCache("2")
	block3 = c.GetBlockCache("3")

	assertFetch(t, block1, 1, 100, true)
	assertFetch(t, block2, 1, 1000, true)
	assertFetch(t, block3, 1, 0, false)
}

func TestSeriesHashCache_MeasureApproximateSizePerEntry(t *testing.T) {
	// This test measures the approximate size (in bytes) per cache entry.
	// We only take in account the memory used by the map, which is the largest amount.
	const numEntries = 100000
	c := NewSeriesHashCache(1024 * 1024 * 1024)
	b := c.GetBlockCache(ulid.MustNew(0, rand.Reader).String())

	before := runtime.MemStats{}
	runtime.ReadMemStats(&before)

	// Preallocate the map in order to not account for re-allocations
	// since we want to measure the heap utilization and not allocations.
	b.generations[0].hashes = make(map[storage.SeriesRef]uint64, numEntries)

	for i := uint64(0); i < numEntries; i++ {
		b.Store(storage.SeriesRef(i), i)
	}

	after := runtime.MemStats{}
	runtime.ReadMemStats(&after)

	t.Logf("approximate size per entry: %d bytes", (after.TotalAlloc-before.TotalAlloc)/numEntries)
	require.Equal(t, uint64(approxBytesPerEntry), (after.TotalAlloc-before.TotalAlloc)/numEntries, "approxBytesPerEntry constant is out date")
}

func TestSeriesHashCache_Concurrency(t *testing.T) {
	const (
		concurrency   = 100
		numIterations = 10000
		numBlocks     = 10
	)

	// Set the max cache size to store at most 10 entries per generation,
	// so that we stress test the GC too.
	c := NewSeriesHashCache(10 * numGenerations * approxBytesPerEntry)

	wg := sync.WaitGroup{}
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()

			for n := 0; n < numIterations; n++ {
				blockID := strconv.Itoa(n % numBlocks)

				blockCache := c.GetBlockCache(blockID)
				blockCache.Store(storage.SeriesRef(n), uint64(n))
				actual, ok := blockCache.Fetch(storage.SeriesRef(n))

				require.True(t, ok)
				require.Equal(t, uint64(n), actual)
			}
		}()
	}

	wg.Wait()
}

func BenchmarkSeriesHashCache_StoreAndFetch(b *testing.B) {
	for _, numBlocks := range []int{1, 10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("blocks=%d", numBlocks), func(b *testing.B) {
			c := NewSeriesHashCache(1024 * 1024)

			// In this benchmark we assume the usage pattern is calling Fetch() and Store() will be
			// orders of magnitude more frequent than GetBlockCache(), so we call GetBlockCache() just
			// once per block.
			blockCaches := make([]*BlockSeriesHashCache, numBlocks)
			for idx := 0; idx < numBlocks; idx++ {
				blockCaches[idx] = c.GetBlockCache(strconv.Itoa(idx))
			}

			// In this benchmark we assume the ratio between Store() and Fetch() is 1:10.
			storeOps := (b.N / 10) + 1

			for n := 0; n < b.N; n++ {
				if n < storeOps {
					blockCaches[n%numBlocks].Store(storage.SeriesRef(n), uint64(n))
				} else {
					blockCaches[n%numBlocks].Fetch(storage.SeriesRef(n % storeOps))
				}
			}
		})
	}
}

func assertFetch(t *testing.T, c *BlockSeriesHashCache, seriesID storage.SeriesRef, expectedValue uint64, expectedOk bool) {
	actualValue, actualOk := c.Fetch(seriesID)
	require.Equal(t, expectedValue, actualValue)
	require.Equal(t, expectedOk, actualOk)
}
