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
	"github.com/prometheus/prometheus/util/compression"
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
			opts := newTestHeadDefaultOptions(1000, false)
			opts.EnableNativeMetadata = true
			head, _ := newTestHeadWithOptions(t, compression.None, opts)
			var refs []storage.SeriesRef
			for _, name := range []string{"a", "b"} {
				app := head.AppenderV2(t.Context())
				ref, err := app.Append(0, labels.FromStrings(labels.MetricName, name), 0, 100, 1, nil, nil, storage.AOptions{Metadata: metadata.Metadata{Help: name}})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				refs = append(refs, ref)
			}
			old := head.series.getByID(chunks.HeadSeriesRef(refs[1]))
			postings := &nativeMetadataGCPostings{Postings: index.NewListPostings(refs), next: func() {
				deleted := head.gcSeries(refs[1:], math.MaxInt64, func(*memSeries) bool { return true })
				require.Len(t, deleted, 1)
				if recreate {
					app := head.AppenderV2(t.Context())
					ref, err := app.Append(0, labels.FromStrings(labels.MetricName, "b"), 0, 200, 2, nil, nil, storage.AOptions{Metadata: metadata.Metadata{Help: "new"}})
					require.NoError(t, err)
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
	t.Run("Next and Seek filter refs", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		commitNativeMetricMetadata(store, 1, makeNativeMetricMetadataPoint(1, metadata.Metadata{Help: "one"}))
		commitNativeMetricMetadata(store, 3, makeNativeMetricMetadataPoint(1, metadata.Metadata{Help: "three"}))

		p := &nativeMetricMetadataPostings{
			Postings: index.NewListPostings([]storage.SeriesRef{1, 2, 3, 4}),
			store:    store,
		}
		require.True(t, p.Next())
		require.Equal(t, storage.SeriesRef(1), p.At())
		require.True(t, p.Seek(2))
		require.Equal(t, storage.SeriesRef(3), p.At())
		require.False(t, p.Seek(4))
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
	opts := newTestHeadDefaultOptions(1000, false)
	opts.EnableNativeMetadata = true
	head, _ := newTestHeadWithOptions(t, compression.None, opts)
	ctx := context.Background()

	for i, name := range []string{"b", "a"} {
		app := head.AppenderV2(ctx)
		_, err := app.Append(0, labels.FromStrings(labels.MetricName, name, "job", "api"), 0, int64(100+i), float64(i), nil, nil, storage.AOptions{
			Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name},
		})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	app := head.AppenderV2(ctx)
	_, err := app.Append(0, labels.FromStrings(labels.MetricName, "c", "job", "api"), 0, 102, 2, nil, nil, storage.AOptions{
		Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: "c"},
	})
	require.NoError(t, err)
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
		opts := newTestHeadDefaultOptions(1000, false)
		opts.EnableNativeMetadata = true
		head, _ := newTestHeadWithOptions(t, compression.None, opts)

		// "z" sorts last, so the query reaches it only after the limit is
		// already satisfied.
		for _, name := range []string{"a", "b", "z"} {
			app := head.AppenderV2(ctx)
			_, err := app.Append(0, labels.FromStrings(labels.MetricName, name, "job", "api"), 0, 100, 1, nil, nil, storage.AOptions{
				Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name},
			})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
		}
		var zRef storage.SeriesRef
		for _, name := range []string{"a", "b"} {
			app := head.AppenderV2(ctx)
			_, err := app.Append(0, labels.FromStrings(labels.MetricName, name, "job", "api"), 0, 900, 1, nil, nil, storage.AOptions{
				Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name},
			})
			require.NoError(t, err)
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
}
