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
	"context"
	"errors"
	"math"
	"runtime"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

func TestNativeMetricMetadataSnapshotOwnership(t *testing.T) {
	store := newNativeMetricMetadataStore()
	m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "initial"}
	commitNativeMetricMetadata(store, 1, makeNativeMetricMetadataPoint(100, m))
	var snapshot nativeMetricMetadataSnapshot
	ok := store.snapshot(1, &snapshot)
	require.True(t, ok)
	for i := range 10 {
		commitNativeMetricMetadata(store, 1, makeNativeMetricMetadataPoint(int64(100+i), metadata.Metadata{Help: strconv.Itoa(i)}))
	}
	store.delete(map[storage.SeriesRef]struct{}{1: {}})
	runtime.GC()
	require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: m}}, snapshot.expand())
	require.False(t, snapshot.truncated)
}

type nativeMetadataGCPostings struct {
	index.Postings
	next func()
}

func (p *nativeMetadataGCPostings) Next() bool {
	if p.next != nil {
		p.next()
		p.next = nil
	}
	return p.Postings.Next()
}

func TestHeadNativeMetricMetadataQueryConcurrentDeletion(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(strconv.FormatBool(recreate), func(t *testing.T) {
			head := newNativeMetadataTestHead(t, withoutOOO)
			var refs []storage.SeriesRef
			for _, name := range []string{"a", "b"} {
				app := head.AppenderV2(t.Context())
				ref := mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, name), 100, 1, storage.AOptions{Metadata: metadata.Metadata{Help: name}})
				require.NoError(t, app.Commit())
				refs = append(refs, ref)
			}
			old := head.series.getByID(chunks.HeadSeriesRef(refs[1]))
			postings := &nativeMetadataGCPostings{Postings: index.NewListPostings(refs), next: func() {
				deleted := head.gcSeries(refs[1:], math.MaxInt64, func(*memSeries) bool { return true })
				require.Len(t, deleted, 1)
				if recreate {
					app := head.AppenderV2(t.Context())
					ref := mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, "b"), 200, 2, storage.AOptions{Metadata: metadata.Metadata{Help: "new"}})
					require.NotEqual(t, refs[1], ref)
					require.NoError(t, app.Commit())
				}
			}}
			result, truncated, err := head.nativeMetricMetadataForPostings(t.Context(), postings, 1)
			require.NoError(t, err)
			require.False(t, truncated)
			require.Len(t, result, 1)
			require.Equal(t, "a", result[0].Labels.Get(labels.MetricName))
			require.True(t, old.isGCed())
			require.False(t, head.nativeMetricMetadata.has(old.ref))
			want := int64(1)
			if recreate {
				want++
			}
			require.Equal(t, want, head.nativeMetricMetadata.series.Load())
			require.Equal(t, want, head.nativeMetricMetadata.versions.Load())
		})
	}
}

func TestNativeMetricMetadataPostings(t *testing.T) {
	t.Run("cached pages follow completed mutations", func(t *testing.T) {
		span := func(lo, hi int) []int {
			offsets := make([]int, 0, hi-lo)
			for offset := lo; offset < hi; offset++ {
				offsets = append(offsets, offset)
			}
			return offsets
		}
		for _, tc := range []struct {
			name                    string
			seed                    int
			add, remove             []int
			reset, update           bool
			target                  int
			want, generationChanged bool
		}{
			{name: "absent page published", add: []int{0}, want: true, generationChanged: true},
			{name: "singleton grows", seed: 1, add: []int{1}, target: 1, want: true, generationChanged: true},
			{name: "sparse slot inserted", seed: 2, add: []int{2}, target: 2, want: true},
			{name: "next bitmap word inserted", seed: 2, add: []int{32}, target: 32, want: true},
			{name: "last bitmap word inserted", seed: 2, add: []int{224}, target: 224, want: true},
			{name: "sparse page replaced", seed: 3, add: []int{3}, target: 3, want: true, generationChanged: true},
			{name: "dense slot inserted", seed: 97, add: []int{97}, target: 97, want: true},
			{name: "sparse slot removed", seed: 3, remove: []int{2}, target: 2},
			{name: "dense slot removed", seed: 100, remove: []int{99}, target: 99},
			{name: "half-tombstoned sparse page kept", seed: 48, remove: span(24, 48), target: 0, want: true},
			{name: "mostly tombstoned sparse page rebuilt", seed: 48, remove: span(23, 48), target: 0, want: true, generationChanged: true},
			{name: "dense page retained", seed: 97, remove: span(80, 97), target: 79, want: true},
			{name: "page becomes singleton", seed: 2, remove: []int{1}, want: true, generationChanged: true},
			{name: "last member removed", seed: 1, remove: []int{0}, generationChanged: true},
			{name: "reset", seed: 2, reset: true, generationChanged: true},
			{name: "reset and recreate", seed: 2, reset: true, add: []int{0}, want: true, generationChanged: true},
			{name: "history update", seed: 3, update: true, want: true},
			{name: "last full width reference", seed: 1, add: []int{255}, target: 255, want: true, generationChanged: true},
		} {
			for _, seek := range []bool{false, true} {
				t.Run(tc.name+"/seek="+strconv.FormatBool(seek), func(t *testing.T) {
					store := newNativeMetricMetadataStore()
					base := chunks.HeadSeriesRef(math.MaxUint64 & ^uint64(nativeMetadataPageSize-1))
					for offset := range tc.seed {
						commitNativeMetricMetadata(store, base+chunks.HeadSeriesRef(offset), makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "initial"}))
					}
					p := &nativeMetricMetadataPostings{store: store}
					require.Equal(t, tc.seed > 0, p.has(base))
					before := store.directoryGeneration.Load()
					if tc.reset {
						store.reset()
					}
					if len(tc.remove) > 0 {
						refs := map[storage.SeriesRef]struct{}{}
						for _, offset := range tc.remove {
							ref := base + chunks.HeadSeriesRef(offset)
							series := store.indexedSeries(ref)
							setGCedForTest(series)
							refs[storage.SeriesRef(ref)] = struct{}{}
						}
						store.delete(refs)
					}
					for _, offset := range tc.add {
						commitNativeMetricMetadata(store, base+chunks.HeadSeriesRef(offset), makeNativeMetricMetadataPoint(200, metadata.Metadata{Help: "new"}))
					}
					if tc.update {
						commitNativeMetricMetadata(store, base, makeNativeMetricMetadataPoint(200, metadata.Metadata{Help: "changed"}))
					}
					require.Equal(t, tc.generationChanged, store.directoryGeneration.Load() != before)
					ref := base + chunks.HeadSeriesRef(tc.target)
					p.Postings = index.NewListPostings([]storage.SeriesRef{storage.SeriesRef(ref)})
					var got bool
					if seek {
						got = p.Seek(storage.SeriesRef(ref))
					} else {
						got = p.Next()
					}
					require.Equal(t, tc.want, got)
					if got {
						require.Equal(t, storage.SeriesRef(ref), p.At())
						require.Same(t, store.indexedSeries(ref), nativeMetadataDirectorySeries(p.page, ref))
					}
				})
			}
		}
	})
	t.Run("capture racing replacement retains the earlier generation", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "initial"})
		commitNativeMetricMetadata(store, 0, point)
		generation := store.directoryGeneration.Load()
		page, _ := store.directory.Load(uint64(0))
		commitNativeMetricMetadata(store, 1, point)
		p := &nativeMetricMetadataPostings{store: store, page: page, pageKey: 0, pageGeneration: generation, pageCached: true}
		require.True(t, p.has(1))
		require.Equal(t, store.directoryGeneration.Load(), p.pageGeneration)
		require.False(t, p.has(256), "a different page must not reuse the cached value")
		require.True(t, p.has(0), "returning to page zero must reload it")
	})
	t.Run("bitmap offsets do not alias between full page keys", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		point := makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "test"})
		for _, ref := range []chunks.HeadSeriesRef{1, 3, 1<<32 + 2, 1<<32 + 4} {
			commitNativeMetricMetadata(store, ref, point)
		}
		p := &nativeMetricMetadataPostings{
			Postings: index.NewListPostings([]storage.SeriesRef{1, 2, 3, 257, 259, 1<<32 + 1, 1<<32 + 2, 1<<32 + 3, 1<<32 + 4}),
			store:    store,
		}
		require.True(t, p.Next())
		require.Equal(t, storage.SeriesRef(1), p.At())
		require.True(t, p.Seek(257))
		require.Equal(t, storage.SeriesRef(1<<32+2), p.At())
		require.True(t, p.Next())
		require.Equal(t, storage.SeriesRef(1<<32+4), p.At())
		require.False(t, p.Next())
		require.NoError(t, p.Err())
	})

	t.Run("underlying error is propagated", func(t *testing.T) {
		expectedErr := errors.New("postings failed")
		p := &nativeMetricMetadataPostings{
			Postings: index.ErrPostings(expectedErr),
			store:    newNativeMetricMetadataStore(),
		}
		require.False(t, p.Next())
		require.ErrorIs(t, p.Err(), expectedErr)
	})
}

func TestHeadNativeMetricMetadataMatchersAndLimit(t *testing.T) {
	head := newNativeMetadataTestHead(t, withoutOOO)
	ctx := context.Background()

	for i, name := range []string{"b", "a"} {
		app := head.AppenderV2(ctx)
		mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, name, "job", "api"), int64(100+i), float64(i), storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name}})
		require.NoError(t, app.Commit())
	}

	app := head.AppenderV2(ctx)
	mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, "c", "job", "api"), 102, 2, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: "c"}})
	require.NoError(t, app.Commit())

	jobMatcher := labels.MustNewMatcher(labels.MatchEqual, "job", "api")
	nameMatcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "a")
	for _, tc := range []struct {
		name          string
		limit         int
		wantNames     []string
		wantTruncated bool
	}{
		{
			name:          "limit one reports truncation",
			limit:         1,
			wantNames:     []string{"a"},
			wantTruncated: true,
		},
		{
			name:      "limit equal to result count",
			limit:     3,
			wantNames: []string{"a", "b", "c"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, truncated, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{jobMatcher}, {nameMatcher}}, tc.limit)
			require.NoError(t, err)
			require.Equal(t, tc.wantTruncated, truncated)
			require.Len(t, result, len(tc.wantNames))
			for i, name := range tc.wantNames {
				require.Equal(t, name, result[i].Labels.Get(labels.MetricName))
			}
		})
	}

	t.Run("metadata outliving its series does not leak into results", func(t *testing.T) {
		head := newNativeMetadataTestHead(t, withoutOOO)

		// "z" sorts last, so the query reaches it only after the limit is
		// already satisfied.
		for _, name := range []string{"a", "b", "z"} {
			app := head.AppenderV2(ctx)
			mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, name, "job", "api"), 100, 1, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name}})
			require.NoError(t, app.Commit())
		}
		var zRef storage.SeriesRef
		for _, name := range []string{"a", "b"} {
			app := head.AppenderV2(ctx)
			mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, name, "job", "api"), 900, 1, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name}})
			require.NoError(t, app.Commit())
		}

		// Reproduce the window inside Head.gc between dropping expired series
		// and removing them from the postings and the metadata store: "z" is
		// gone from head.series while its postings entry and its metadata both
		// survive. SortedPostings drops it, so it must not appear in the result
		// and must not be mistaken for a further result behind the limit.
		//
		// TestHeadNativeMetricMetadataQueryConcurrentDeletion covers deletion
		// after references have been selected, while consuming the postings.
		deleted, _, _, _, _, _, _, _, _ := head.series.gc(500, 0)
		require.Len(t, deleted, 1, "gc must expire exactly the trailing series")
		for ref := range deleted {
			zRef = ref
		}
		require.Nil(t, head.series.getByID(chunks.HeadSeriesRef(zRef)))
		require.True(t, head.nativeMetricMetadata.has(chunks.HeadSeriesRef(zRef)),
			"the removed series must keep its metadata, or the query never reaches it")

		result, truncated, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{jobMatcher}}, 2)
		require.NoError(t, err)
		require.False(t, truncated, "every matching series was returned, so nothing was truncated")
		require.Len(t, result, 2)
		require.Equal(t, "a", result[0].Labels.Get(labels.MetricName))
		require.Equal(t, "b", result[1].Labels.Get(labels.MetricName))
	})

	t.Run("series marked garbage-collected after lookup is skipped", func(t *testing.T) {
		head := newNativeMetadataTestHead(t, withoutOOO)
		var refs []storage.SeriesRef
		for _, name := range []string{"a", "b"} {
			app := head.AppenderV2(ctx)
			refs = append(refs, mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, name, "job", "api"), 100, 1, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name}}))
			require.NoError(t, app.Commit())
		}
		// GC marks a series under its lock before removing it from the Head,
		// so a query can still look it up. Nothing may append to it afterwards.
		series := head.series.getByID(chunks.HeadSeriesRef(refs[1]))
		setGCedForTest(series)

		result, truncated, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{jobMatcher}}, 0)
		require.NoError(t, err)
		require.False(t, truncated)
		require.Len(t, result, 1)
		require.Equal(t, "a", result[0].Labels.Get(labels.MetricName))
	})

	t.Run("results report per-series history truncation", func(t *testing.T) {
		head := newNativeMetadataTestHead(t, withoutOOO)
		for i := range maxNativeMetricMetadataVersions + 1 {
			app := head.AppenderV2(ctx)
			mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, "a", "job", "api"), int64(100+i), 1, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: strconv.Itoa(i)}})
			require.NoError(t, app.Commit())
		}
		app := head.AppenderV2(ctx)
		mustAppendV2(t, app, 0, labels.FromStrings(labels.MetricName, "b", "job", "api"), 100, 1, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: "b"}})
		require.NoError(t, app.Commit())

		result, truncated, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{jobMatcher}}, 0)
		require.NoError(t, err)
		require.False(t, truncated)
		require.Len(t, result, 2)
		require.True(t, result[0].Truncated)
		require.Len(t, result[0].Versions, maxNativeMetricMetadataVersions)
		require.False(t, result[1].Truncated)
	})
}
