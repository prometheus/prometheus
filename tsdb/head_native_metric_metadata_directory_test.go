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

package tsdb

import (
	"math"
	"math/rand/v2"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestNativeMetricMetadataDirectory(t *testing.T) {
	t.Run("page contains only bitmap words", func(t *testing.T) {
		require.Equal(t, uintptr(32), unsafe.Sizeof(nativeMetricMetadataPage{}))
	})
	t.Run("full width keys and word boundaries", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		for _, key := range []uint64{0, 1, 1 << 24, math.MaxUint64 >> nativeMetadataPageBits} {
			for offset := range nativeMetadataPageSize {
				ref := chunks.HeadSeriesRef(key<<nativeMetadataPageBits | uint64(offset))
				commitNativeMetricMetadata(store, ref, point)
			}
			before, _ := store.directory.Load(key)
			refs := map[storage.SeriesRef]struct{}{}
			for offset := 0; offset < nativeMetadataPageSize; offset += 3 {
				refs[storage.SeriesRef(key<<nativeMetadataPageBits|uint64(offset))] = struct{}{}
			}
			store.delete(refs)
			after, _ := store.directory.Load(key)
			require.Same(t, before, after, "partial deletion retains page identity")
			for offset := range nativeMetadataPageSize {
				ref := chunks.HeadSeriesRef(key<<nativeMetadataPageBits | uint64(offset))
				require.Equal(t, offset%3 != 0, store.has(ref))
			}
		}
	})
	t.Run("concurrent writers preserve neighboring bits", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		var writers sync.WaitGroup
		for offset := range 32 {
			writers.Go(func() { commitNativeMetricMetadata(store, chunks.HeadSeriesRef(offset), point) })
		}
		writers.Wait()
		value, _ := store.directory.Load(uint64(0))
		require.Equal(t, uint32(math.MaxUint32), value.(*nativeMetricMetadataPage).present[0].Load())
		for offset := 0; offset < 32; offset += 2 {
			writers.Go(func() { store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(offset): {}}) })
		}
		writers.Wait()
		for offset := range 32 {
			require.Equal(t, offset%2 != 0, store.has(chunks.HeadSeriesRef(offset)))
		}
		require.Equal(t, int64(16), store.series.Load())
		require.Equal(t, int64(16), store.versions.Load())
	})
	t.Run("random membership matches independent map", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		want := map[chunks.HeadSeriesRef]bool{}
		refs := []chunks.HeadSeriesRef{0, 1, 255, 256, 257, 1 << 32, 1<<32 + 255, math.MaxUint64}
		for ref := range 512 {
			refs = append(refs, chunks.HeadSeriesRef(ref))
		}
		rng := rand.New(rand.NewPCG(17, 29))
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		for range 5000 {
			ref := refs[rng.IntN(len(refs))]
			if rng.IntN(3) != 0 {
				commitNativeMetricMetadata(store, ref, point)
				want[ref] = true
			} else {
				store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(ref): {}})
				delete(want, ref)
			}
			for _, ref := range refs {
				if got := store.has(ref); got != want[ref] {
					require.Equal(t, want[ref], got, "ref %d", ref)
				}
			}
			require.Equal(t, int64(len(want)), store.series.Load())
			require.Equal(t, int64(len(want)), store.versions.Load())
		}
	})
	t.Run("readers survive concurrent removal and recreation", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		done := make(chan struct{})
		var readers sync.WaitGroup
		for range 4 {
			readers.Go(func() {
				postings := &nativeMetricMetadataPostings{store: store.nativeMetricMetadataStore}
				for {
					select {
					case <-done:
						return
					default:
					}
					for ref := range nativeMetadataPageSize {
						postings.has(chunks.HeadSeriesRef(ref))
					}
				}
			})
		}
		for range 10 {
			refs := map[storage.SeriesRef]struct{}{}
			for ref := range nativeMetadataPageSize {
				commitNativeMetricMetadata(store, chunks.HeadSeriesRef(ref), point)
				refs[storage.SeriesRef(ref)] = struct{}{}
			}
			store.delete(refs)
			require.Zero(t, store.series.Load())
			require.Zero(t, store.versions.Load())
		}
		close(done)
		readers.Wait()
	})
	t.Run("last deletion races with first insertion in same page", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		for round := range 200 {
			first := chunks.HeadSeriesRef(round*256 + 1)
			second := first + 1
			commitNativeMetricMetadata(store, first, point)
			var writers sync.WaitGroup
			writers.Go(func() { store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(first): {}}) })
			writers.Go(func() { commitNativeMetricMetadata(store, second, point) })
			writers.Wait()
			require.False(t, store.has(first))
			require.True(t, store.has(second))
			require.Equal(t, int64(1), store.series.Load())
			require.Equal(t, int64(1), store.versions.Load())
			store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(second): {}})
			require.Zero(t, store.series.Load())
		}
	})
}
