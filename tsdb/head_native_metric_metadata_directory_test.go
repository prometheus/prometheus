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
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestNativeMetricMetadataDirectory(t *testing.T) {
	t.Run("bitmap adds only its inline words", func(t *testing.T) {
		want := uintptr(80)
		if strconv.IntSize == 32 {
			want = 56
		}
		require.Equal(t, want, unsafe.Sizeof(nativeMetricMetadataPage{}))
	})
	t.Run("patterned full width references at every capacity", func(t *testing.T) {
		for _, capacity := range []int{4, 8, 16, 32, 64, 128, 256} {
			for _, key := range []uint64{0, 1, 1 << 24, math.MaxUint64 >> nativeMetadataPageBits} {
				for _, stride := range []int{1, 4, 16, 64} {
					t.Run(fmt.Sprintf("capacity=%d/page=%d/stride=%d", capacity, key, stride), func(t *testing.T) {
						page := newNativeMetricMetadataPage(capacity)
						want := map[chunks.HeadSeriesRef]*memSeries{}
						for residue := range stride {
							for offset := residue; offset < nativeMetadataPageSize && len(want) < capacity*3/4; offset += stride {
								ref := chunks.HeadSeriesRef(key<<nativeMetadataPageBits | uint64(offset))
								want[ref] = &memSeries{ref: ref}
								page.insert(want[ref])
							}
						}
						for offset := range nativeMetadataPageSize {
							ref := chunks.HeadSeriesRef(key<<nativeMetadataPageBits | uint64(offset))
							require.Same(t, want[ref], page.lookup(ref))
							if !page.dense {
								require.Equal(t, want[ref] != nil, page.contains(ref))
							}
						}
					})
				}
			}
		}
	})
	t.Run("bitmap gates sparse pointers during membership transitions", func(t *testing.T) {
		for _, ref := range []chunks.HeadSeriesRef{0, 31, 32, 63, 64, 127, 128, 223, 224, 255, 1 << 32, math.MaxUint64} {
			page := newNativeMetricMetadataPage(4)
			series := &memSeries{ref: ref}
			slot := page.slot(ref)
			page.slots[slot].Store(series)
			require.Nil(t, page.lookup(ref), "a pointer alone does not publish membership")
			offset := uint8(ref)
			word := &page.present[offset/32]
			word.Store(uint32(1) << (offset % 32))
			require.Same(t, series, page.lookup(ref))
			word.Store(0)
			require.Nil(t, page.lookup(ref), "clearing membership hides the retained pointer")
		}
	})
	t.Run("concurrent writers preserve neighboring bitmap bits", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		var writers sync.WaitGroup
		for offset := range 32 {
			writers.Go(func() { commitNativeMetricMetadata(store, chunks.HeadSeriesRef(offset), point) })
		}
		writers.Wait()
		value, _ := store.directory.Load(uint64(0))
		page := value.(*nativeMetricMetadataPage)
		require.False(t, page.dense)
		require.Equal(t, uint32(math.MaxUint32), page.present[0].Load())
		for offset := 0; offset < 32; offset += 2 {
			writers.Go(func() {
				ref := chunks.HeadSeriesRef(offset)
				series := store.indexedSeries(ref)
				setGCedForTest(series)
				store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(ref): {}})
			})
		}
		writers.Wait()
		for offset := range 32 {
			require.Equal(t, offset%2 != 0, store.indexedSeries(chunks.HeadSeriesRef(offset)) != nil)
		}
	})
	t.Run("representation growth and detached readers", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		var captured any
		for ref := range nativeMetadataPageSize {
			before, _ := store.directory.Load(uint64(0))
			commitNativeMetricMetadata(store, chunks.HeadSeriesRef(ref), point)
			value, ok := store.directory.Load(uint64(0))
			require.True(t, ok)
			if ref == 0 {
				require.IsType(t, &memSeries{}, value)
				continue
			}
			page := value.(*nativeMetricMetadataPage)
			capacity := 4
			for (ref+1)*4 > capacity*3 && capacity < nativeMetadataPageSize {
				capacity *= 2
			}
			require.Len(t, page.slots, capacity)
			require.Equal(t, capacity == nativeMetadataPageSize, page.dense)
			require.Equal(t, ref+1, page.live)
			if previous, ok := before.(*nativeMetricMetadataPage); ok && len(previous.slots) == capacity {
				require.Same(t, previous, page, "ordinary insertion must not copy a sparse page")
			}
			if ref == 10 {
				captured = page
			}
		}
		deleted := map[storage.SeriesRef]struct{}{}
		for ref := range nativeMetadataPageSize {
			setGCedForTest(store.indexedSeries(chunks.HeadSeriesRef(ref)))
			deleted[storage.SeriesRef(ref)] = struct{}{}
		}
		store.delete(deleted)
		_, exists := store.directory.Load(uint64(0))
		require.False(t, exists)
		require.Zero(t, store.series.Load())
		// This descriptor was detached by growth. Later deletion affects newer
		// pages only, so history reached through it remains unchanged.
		series := nativeMetadataDirectorySeries(captured, 0)
		require.NotNil(t, series)
		require.Equal(t, "test", series.metadata.native.metadata.Help)
		require.Zero(t, store.versions.Load())
	})

	t.Run("bulk and staged deletion retain membership through regrowth", func(t *testing.T) {
		// Each sequence deletes the page's highest references down to each target
		// in turn. The dense page demotes at 32, and the sparse page again at 16.
		sequences := [][]int{{32, 16}}
		for _, live := range []int{0, 1, 2, 31, 32, 33, 64, 80, 81, 96, 97, 256} {
			sequences = append(sequences, []int{live})
			if live < 64 {
				sequences = append(sequences, []int{64, live})
			}
		}
		for _, targets := range sequences {
			live := targets[len(targets)-1]
			t.Run(fmt.Sprintf("targets=%v", targets), func(t *testing.T) {
				store := newNativeMetricMetadataStore()
				base := chunks.HeadSeriesRef(math.MaxUint64 & ^uint64(nativeMetadataPageSize-1))
				point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "initial"})
				for offset := range nativeMetadataPageSize {
					commitNativeMetricMetadata(store, base+chunks.HeadSeriesRef(offset), point)
				}
				captured, _ := store.directory.Load(uint64(base) >> nativeMetadataPageBits)
				postings := &nativeMetricMetadataPostings{store: store}
				require.True(t, postings.has(base))
				remaining, detachedFrom := nativeMetadataPageSize, 0
				for _, target := range targets {
					if current, _ := store.directory.Load(uint64(base) >> nativeMetadataPageBits); current != captured && detachedFrom == 0 {
						detachedFrom = remaining
					}
					deleted := map[storage.SeriesRef]struct{}{}
					for offset := target; offset < remaining; offset++ {
						ref := base + chunks.HeadSeriesRef(offset)
						series := store.indexedSeries(ref)
						setGCedForTest(series)
						deleted[storage.SeriesRef(ref)] = struct{}{}
					}
					store.delete(deleted)
					remaining = target
				}
				value, exists := store.directory.Load(uint64(base) >> nativeMetadataPageBits)
				switch live {
				case 0:
					require.False(t, exists)
				case 1:
					require.IsType(t, &memSeries{}, value)
				default:
					page := value.(*nativeMetricMetadataPage)
					capacity := nativeMetadataPageSize
					if live <= 32 {
						capacity = 4
						for live*4 > capacity*3 {
							capacity *= 2
						}
					}
					require.Len(t, page.slots, capacity)
					require.Equal(t, live, page.live)
				}
				require.Equal(t, int64(live), store.series.Load())
				require.Equal(t, int64(live), store.versions.Load())
				for offset := range nativeMetadataPageSize {
					ref := base + chunks.HeadSeriesRef(offset)
					require.Equal(t, offset < live, postings.has(ref))
					require.Equal(t, offset < live, store.indexedSeries(ref) != nil)
					if offset >= live {
						// A page detached by an earlier stage keeps the series retired after it.
						require.Equal(t, offset < detachedFrom, nativeMetadataDirectorySeries(captured, ref) != nil, "offset %d", offset)
					}
				}
				// Re-publication must invalidate singleton and sparse cached pages.
				for offset := live; offset < nativeMetadataPageSize; offset++ {
					ref := base + chunks.HeadSeriesRef(offset)
					commitNativeMetricMetadata(store, ref, point)
					require.True(t, postings.has(ref))
				}
				require.Equal(t, int64(nativeMetadataPageSize), store.series.Load())
				require.Equal(t, int64(nativeMetadataPageSize), store.versions.Load())
				value, _ = store.directory.Load(uint64(base) >> nativeMetadataPageBits)
				require.True(t, value.(*nativeMetricMetadataPage).dense)
			})
		}
	})

	t.Run("probe chains tombstones and bounded misses", func(t *testing.T) {
		page := newNativeMetricMetadataPage(8)
		var refs []chunks.HeadSeriesRef
		for ref := chunks.HeadSeriesRef(0); len(refs) < 6; ref++ {
			if (uint64(ref)*nativeMetadataDirectoryHashMultiplier)>>page.hashShift == 0 {
				refs = append(refs, ref)
				page.insert(&memSeries{ref: ref})
			}
		}
		first := page.slot(refs[0])
		offset := uint8(refs[0])
		word := &page.present[offset/32]
		word.Store(word.Load() & ^(uint32(1) << (offset % 32)))
		page.slots[first].Store(nativeMetadataDirectoryTombstone)
		page.live--
		for _, ref := range refs[1:] {
			require.Equal(t, ref, page.lookup(ref).ref)
		}
		page.insert(&memSeries{ref: refs[0]})
		require.Equal(t, 6, page.live)
		require.Equal(t, 6, page.used, "reusing a tombstone does not increase used occupancy")
		require.Equal(t, first, page.slot(refs[0]))
		require.Nil(t, page.lookup(1<<32))
		for i := range page.slots {
			page.slots[i].Store(nativeMetadataDirectoryTombstone)
		}
		// Deliberately retain a positive bit to exercise a bounded failed probe.
		page.present[0].Store(page.present[0].Load() | 1)
		require.Nil(t, page.lookup(0), "even an all-tombstone table must terminate")
		require.GreaterOrEqual(t, page.slot(0), 0)
	})

	t.Run("readers survive concurrent page replacement", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		done := make(chan struct{})
		var readers sync.WaitGroup
		for range 4 {
			readers.Go(func() {
				postings := &nativeMetricMetadataPostings{store: store}
				for {
					select {
					case <-done:
						return
					default:
					}
					for ref := range nativeMetadataPageSize {
						postings.has(chunks.HeadSeriesRef(ref))
						if series := store.indexedSeries(chunks.HeadSeriesRef(ref)); series != nil && series.ref != chunks.HeadSeriesRef(ref) {
							t.Errorf("wrong reference: %d for %d", series.ref, ref)
							return
						}
					}
				}
			})
		}
		for range 10 {
			refs := make(map[storage.SeriesRef]struct{})
			for ref := range nativeMetadataPageSize {
				commitNativeMetricMetadata(store, chunks.HeadSeriesRef(ref), point)
				refs[storage.SeriesRef(ref)] = struct{}{}
			}
			for ref := range refs {
				series := store.indexedSeries(chunks.HeadSeriesRef(ref))
				setGCedForTest(series)
			}
			store.delete(refs)
			require.Zero(t, store.series.Load())
		}
		close(done)
		readers.Wait()
	})
	t.Run("random membership matches full width map", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		want := map[chunks.HeadSeriesRef]*memSeries{}
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
				series := store.indexedSeries(ref)
				require.NotNil(t, series)
				if previous := want[ref]; previous != nil {
					require.Same(t, previous, series, "duplicate publication changes no membership")
				}
				want[ref] = series
			} else {
				if series := want[ref]; series != nil {
					setGCedForTest(series)
				}
				store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(ref): {}})
				delete(want, ref)
			}
			// Assert through testify only on a mismatch: millions of calls
			// dominate this test's run time under -race.
			for _, ref := range refs {
				if series := store.indexedSeries(ref); series != want[ref] || store.has(ref) != (want[ref] != nil) {
					require.Equal(t, want[ref], series, "ref %d", ref)
					require.Equal(t, want[ref] != nil, store.has(ref), "ref %d", ref)
				}
			}
			require.Equal(t, int64(len(want)), store.series.Load())
			require.Equal(t, int64(len(want)), store.versions.Load())
		}
	})

	t.Run("last deletion races with first insertion in same page", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		for round := range 200 {
			first := chunks.HeadSeriesRef(round*256 + 1)
			second := first + 1
			commitNativeMetricMetadata(store, first, point)
			retired := store.indexedSeries(first)
			setGCedForTest(retired)
			var wg sync.WaitGroup
			wg.Go(func() { store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(first): {}}) })
			wg.Go(func() { commitNativeMetricMetadata(store, second, point) })
			wg.Wait()
			require.Nil(t, store.indexedSeries(first))
			require.NotNil(t, store.indexedSeries(second))
			require.Equal(t, int64(1), store.series.Load())
			store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(second): {}})
			require.Zero(t, store.series.Load())
		}
	})
}
