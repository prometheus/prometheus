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
	"strconv"
	"testing"
	"unique"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/compression"
)

func makeNativeMetricMetadataPoint(timestamp int64, m metadata.Metadata) nativeMetricMetadataPoint {
	return nativeMetricMetadataPoint{
		effectiveFrom: timestamp,
		metadata:      unique.Make(canonicalMetricMetadata(m)),
	}
}

func TestNativeMetricMetadataHandleCache(t *testing.T) {
	t.Run("activates on reuse and resets in pool", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		nativeAppender := store.getAppender()
		app := headAppenderBase{
			head:                 &Head{nativeMetricMetadata: store},
			nativeMetricMetadata: nativeAppender,
		}
		m := canonicalMetricMetadata(metadata.Metadata{Help: "A"})

		first := nativeAppender.handle(m)
		require.Equal(t, first, nativeAppender.handle(m))
		require.True(t, nativeAppender.cacheActive)
		require.Len(t, nativeAppender.handles, 1)
		require.Empty(t, nativeAppender.seenHandles)
		app.clearNativeMetricMetadata()
		require.Nil(t, app.nativeMetricMetadata)

		nativeAppender = store.getAppender()
		require.Empty(t, nativeAppender.pending)
		require.Empty(t, nativeAppender.handles)
		require.Empty(t, nativeAppender.seenHandles)
		require.False(t, nativeAppender.cacheActive)
		require.False(t, nativeAppender.cacheDisabled)
		store.putAppender(nativeAppender)
	})

	t.Run("disables for high cardinality", func(t *testing.T) {
		nativeAppender := newNativeMetricMetadataAppender()
		for i := 0; i <= maxNativeMetricMetadataHandles; i++ {
			nativeAppender.handle(canonicalMetricMetadata(metadata.Metadata{Help: strconv.Itoa(i)}))
		}
		require.True(t, nativeAppender.cacheDisabled)
		require.False(t, nativeAppender.cacheActive)
		require.Empty(t, nativeAppender.handles)
		require.Empty(t, nativeAppender.seenHandles)
	})
}

func TestNativeMetricMetadataStoreVersioning(t *testing.T) {
	store := newNativeMetricMetadataStore()
	ref := chunks.HeadSeriesRef(1)
	a := metadata.Metadata{Type: model.MetricTypeGauge, Help: "A"}
	b := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}
	c := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "C"}

	store.merge(ref, []nativeMetricMetadataPoint{
		makeNativeMetricMetadataPoint(200, b),
		makeNativeMetricMetadataPoint(100, a),
		makeNativeMetricMetadataPoint(150, c),
		makeNativeMetricMetadataPoint(150, b), // Later in the transaction wins.
	})
	versions, truncated, ok := store.get(ref)
	require.True(t, ok)
	require.False(t, truncated)
	require.Equal(t, []NativeMetricMetadataVersion{
		{EffectiveFrom: 100, Metadata: a},
		{EffectiveFrom: 150, Metadata: b},
	}, versions)

	// Empty and unknown types are semantically identical and must intern to the
	// same handle so adjacent versions coalesce.
	store.merge(ref, []nativeMetricMetadataPoint{
		makeNativeMetricMetadataPoint(250, metadata.Metadata{Help: "C"}),
		makeNativeMetricMetadataPoint(300, c),
	})
	versions, _, _ = store.get(ref)
	require.Equal(t, []NativeMetricMetadataVersion{
		{EffectiveFrom: 100, Metadata: a},
		{EffectiveFrom: 150, Metadata: b},
		{EffectiveFrom: 250, Metadata: c},
	}, versions)

	// A later store application wins for an equal timestamp.
	store.mergeOne(ref, makeNativeMetricMetadataPoint(150, a))
	versions, _, _ = store.get(ref)
	require.Equal(t, []NativeMetricMetadataVersion{
		{EffectiveFrom: 100, Metadata: a},
		{EffectiveFrom: 250, Metadata: c},
	}, versions)

	// Equal incoming values must not coalesce across an existing change point.
	store.merge(ref, []nativeMetricMetadataPoint{
		makeNativeMetricMetadataPoint(50, a),
		makeNativeMetricMetadataPoint(300, a),
	})
	versions, _, _ = store.get(ref)
	require.Equal(t, []NativeMetricMetadataVersion{
		{EffectiveFrom: 50, Metadata: a},
		{EffectiveFrom: 250, Metadata: c},
		{EffectiveFrom: 300, Metadata: a},
	}, versions)
}

func TestNativeMetricMetadataStoreCapsVersions(t *testing.T) {
	store := newNativeMetricMetadataStore()
	ref := chunks.HeadSeriesRef(1)
	a := metadata.Metadata{Type: model.MetricTypeGauge, Help: "A"}
	b := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}

	const observations = 4096
	points := make([]nativeMetricMetadataPoint, observations)
	for i := range points {
		points[i] = makeNativeMetricMetadataPoint(int64(i), a)
	}
	store.merge(ref, points)

	stripe := store.stripe(ref)
	stripe.mtx.RLock()
	history := stripe.histories[ref]
	versionCount := len(history.versions)
	versionCapacity := cap(history.versions)
	backing := &history.versions[0]
	truncated := history.truncated
	stripe.mtx.RUnlock()
	require.Equal(t, 1, versionCount)
	require.LessOrEqual(t, versionCapacity, maxNativeMetricMetadataVersions)
	require.False(t, truncated)
	require.Zero(t, store.evictions.Load())

	store.mergeOne(ref, makeNativeMetricMetadataPoint(observations, a))
	store.mergeOne(ref, makeNativeMetricMetadataPoint(0, a))
	stripe.mtx.RLock()
	history = stripe.histories[ref]
	unchangedBacking := &history.versions[0]
	stripe.mtx.RUnlock()
	require.Same(t, backing, unchangedBacking)
	require.Equal(t, int64(1), store.versions.Load())

	points = make([]nativeMetricMetadataPoint, observations)
	for i := range points {
		m := a
		if i%2 != 0 {
			m = b
		}
		points[i] = makeNativeMetricMetadataPoint(int64(observations+1+i), m)
	}
	store.merge(ref, points)

	versions, truncated, ok := store.get(ref)
	require.True(t, ok)
	require.True(t, truncated)
	require.Len(t, versions, maxNativeMetricMetadataVersions)
	require.Equal(t, int64(2*observations+1-maxNativeMetricMetadataVersions), versions[0].EffectiveFrom)
	require.Equal(t, uint64(observations-maxNativeMetricMetadataVersions), store.evictions.Load())
	stripe.mtx.RLock()
	versionCapacity = cap(stripe.histories[ref].versions)
	stripe.mtx.RUnlock()
	require.LessOrEqual(t, versionCapacity, maxNativeMetricMetadataVersions)
	require.Equal(t, int64(maxNativeMetricMetadataVersions), store.versions.Load())

	store.mergeOne(ref, makeNativeMetricMetadataPoint(2*observations+1, a))
	versions, truncated, ok = store.get(ref)
	require.True(t, ok)
	require.True(t, truncated)
	require.Len(t, versions, maxNativeMetricMetadataVersions)
	require.Equal(t, int64(2*observations+2-maxNativeMetricMetadataVersions), versions[0].EffectiveFrom)
	require.Equal(t, uint64(observations-maxNativeMetricMetadataVersions+1), store.evictions.Load())
	require.Equal(t, int64(maxNativeMetricMetadataVersions), store.versions.Load())

	store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(ref): {}})
	_, _, ok = store.get(ref)
	require.False(t, ok)
	require.Zero(t, store.series.Load())
	require.Zero(t, store.versions.Load())
}

func TestNativeMetricMetadataPostings(t *testing.T) {
	t.Run("Next and Seek filter refs", func(t *testing.T) {
		store := newNativeMetricMetadataStore()
		store.merge(1, []nativeMetricMetadataPoint{makeNativeMetricMetadataPoint(1, metadata.Metadata{Help: "one"})})
		store.merge(3, []nativeMetricMetadataPoint{makeNativeMetricMetadataPoint(1, metadata.Metadata{Help: "three"})})

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

func TestHeadAppenderV2NativeMetricMetadataLifecycle(t *testing.T) {
	opts := newTestHeadDefaultOptions(1000, true)
	opts.EnableNativeMetadata = true
	opts.EnableMetadataWALRecords = true
	head, _ := newTestHeadWithOptions(t, compression.None, opts)
	ctx := context.Background()
	seriesLabels := labels.FromStrings(labels.MetricName, "requests_total", "job", "api")
	a := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "requests", Help: "A"}
	b := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "requests", Help: "B"}

	app := head.AppenderV2(ctx)
	ref, err := app.Append(0, seriesLabels, 0, 100, 1, nil, nil, storage.AOptions{
		NativeMetricMetadata: a,
	})
	require.NoError(t, err)
	series := head.series.getByID(chunks.HeadSeriesRef(ref))
	series.Lock()
	require.Equal(t, uint32(2), series.pendingCommitCount()) // Created series and sample.
	series.Unlock()
	require.NoError(t, app.Commit())
	series.Lock()
	require.Nil(t, series.meta)
	series.Unlock()

	// Existing metadata ingestion remains independent from native metadata.
	app = head.AppenderV2(ctx)
	_, err = app.Append(ref, seriesLabels, 0, 150, 1.5, nil, nil, storage.AOptions{Metadata: b})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	series.Lock()
	require.Equal(t, &b, series.meta)
	series.Unlock()

	app = head.AppenderV2(ctx)
	_, err = app.Append(ref, seriesLabels, 0, 200, 2, nil, nil, storage.AOptions{
		NativeMetricMetadata: b,
	})
	require.NoError(t, err)
	series.Lock()
	require.Equal(t, uint32(1), series.pendingCommitCount()) // Sample.
	series.Unlock()
	require.NoError(t, app.Rollback())

	nameMatcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "requests_total")
	jobMatcher := labels.MustNewMatcher(labels.MatchEqual, "job", "api")
	result, truncated, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{nameMatcher}, {jobMatcher}}, 0)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Equal(t, []NativeMetricMetadataSeries{{
		Labels:   seriesLabels,
		Versions: []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}},
	}}, result)

	app = head.AppenderV2(ctx)
	_, err = app.Append(ref, seriesLabels, 0, 300, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{
		NativeMetricMetadata: b,
	})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	result, _, err = head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{nameMatcher}}, 0)
	require.NoError(t, err)
	require.Equal(t, []NativeMetricMetadataVersion{
		{EffectiveFrom: 100, Metadata: a},
		{EffectiveFrom: 300, Metadata: b},
	}, result[0].Versions)

	head.gcSeries([]storage.SeriesRef{ref}, 301, func(*memSeries) bool { return true })
	result, _, err = head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{nameMatcher}}, 0)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestHeadAppenderV2NativeMetricMetadataTransactions(t *testing.T) {
	t.Run("multiple observations add no reservations", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(1000, true)
		opts.EnableNativeMetadata = true
		head, _ := newTestHeadWithOptions(t, compression.None, opts)
		seriesLabels := labels.FromStrings(labels.MetricName, "requests_total", "job", "api")
		a := metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"}
		b := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}
		c := metadata.Metadata{Type: model.MetricTypeCounter, Help: "C"}

		app := head.AppenderV2(context.Background())
		ref, err := app.Append(0, seriesLabels, 0, 100, 1, nil, nil, storage.AOptions{NativeMetricMetadata: a})
		require.NoError(t, err)
		_, err = app.Append(ref, seriesLabels, 0, 200, 2, nil, nil, storage.AOptions{NativeMetricMetadata: b})
		require.NoError(t, err)
		_, err = app.Append(ref, seriesLabels, 0, 200, 2, nil, nil, storage.AOptions{NativeMetricMetadata: c})
		require.NoError(t, err)

		series := head.series.getByID(chunks.HeadSeriesRef(ref))
		series.Lock()
		require.Equal(t, uint32(4), series.pendingCommitCount()) // Created series and three samples.
		series.Unlock()
		require.NoError(t, app.Commit())

		versions, truncated, ok := head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
		require.True(t, ok)
		require.False(t, truncated)
		require.Equal(t, []NativeMetricMetadataVersion{
			{EffectiveFrom: 100, Metadata: a},
			{EffectiveFrom: 200, Metadata: c},
		}, versions)
		series.Lock()
		require.Zero(t, series.pendingCommitCount())
		series.Unlock()
	})

	t.Run("outstanding unchanged metadata restores an intervening change", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(1000, true)
		opts.EnableNativeMetadata = true
		head, _ := newTestHeadWithOptions(t, compression.None, opts)
		seriesLabels := labels.FromStrings(labels.MetricName, "requests_total", "job", "api")
		a := metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"}
		b := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}

		seed := head.AppenderV2(context.Background())
		ref, err := seed.Append(0, seriesLabels, 0, 100, 1, nil, nil, storage.AOptions{NativeMetricMetadata: a})
		require.NoError(t, err)
		require.NoError(t, seed.Commit())

		later := head.AppenderV2(context.Background())
		_, err = later.Append(ref, seriesLabels, 0, 200, 2, nil, nil, storage.AOptions{NativeMetricMetadata: a})
		require.NoError(t, err)

		intervening := head.AppenderV2(context.Background())
		_, err = intervening.Append(ref, seriesLabels, 0, 150, 1.5, nil, nil, storage.AOptions{NativeMetricMetadata: b})
		require.NoError(t, err)
		require.NoError(t, intervening.Commit())
		require.NoError(t, later.Commit())

		versions, truncated, ok := head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
		require.True(t, ok)
		require.False(t, truncated)
		require.Equal(t, []NativeMetricMetadataVersion{
			{EffectiveFrom: 100, Metadata: a},
			{EffectiveFrom: 150, Metadata: b},
			{EffectiveFrom: 200, Metadata: a},
		}, versions)
	})

	for _, tc := range []struct {
		name     string
		rollback bool
	}{
		{name: "sample reservation protects metadata through commit"},
		{name: "rollback releases the sample reservation", rollback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := newTestHeadDefaultOptions(1000, true)
			opts.EnableNativeMetadata = true
			head, _ := newTestHeadWithOptions(t, compression.None, opts)
			seriesLabels := labels.FromStrings(labels.MetricName, "requests_total", "job", "api")

			seed := head.AppenderV2(context.Background())
			ref, err := seed.Append(0, seriesLabels, 0, 50, 0.5, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, seed.Commit())

			app := head.AppenderV2(context.Background())
			meta := metadata.Metadata{Type: model.MetricTypeCounter, Help: "requests"}
			_, err = app.Append(ref, seriesLabels, 0, 100, 1, nil, nil, storage.AOptions{NativeMetricMetadata: meta})
			require.NoError(t, err)

			series := head.series.getByID(chunks.HeadSeriesRef(ref))
			series.Lock()
			require.Equal(t, uint32(1), series.pendingCommitCount())
			series.Unlock()
			require.Empty(t, head.gcSeries([]storage.SeriesRef{ref}, math.MaxInt64, func(*memSeries) bool { return true }))
			_, _, ok := head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
			require.False(t, ok)

			if tc.rollback {
				require.NoError(t, app.Rollback())
				series.Lock()
				require.Zero(t, series.pendingCommitCount())
				series.Unlock()
				require.Contains(t, head.gcSeries([]storage.SeriesRef{ref}, math.MaxInt64, func(*memSeries) bool { return true }), ref)
				_, _, ok = head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
				require.False(t, ok)
				return
			}

			require.NoError(t, app.Commit())
			series.Lock()
			require.Zero(t, series.pendingCommitCount())
			series.Unlock()
			versions, truncated, ok := head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
			require.True(t, ok)
			require.False(t, truncated)
			require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: meta}}, versions)
		})
	}
}

func TestHeadNativeMetricMetadataWALReplayDeletion(t *testing.T) {
	opts := newTestHeadDefaultOptions(1000, false)
	opts.EnableNativeMetadata = true
	head, _ := newTestHeadWithOptions(t, compression.None, opts)

	app := head.AppenderV2(context.Background())
	ref, err := app.Append(0, labels.FromStrings(labels.MetricName, "requests_total"), 0, 100, 1, nil, nil, storage.AOptions{
		NativeMetricMetadata: metadata.Metadata{Type: model.MetricTypeCounter},
	})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	_, _, ok := head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
	require.True(t, ok)
	require.Equal(t, int64(1), head.nativeMetricMetadata.series.Load())
	require.Equal(t, int64(1), head.nativeMetricMetadata.versions.Load())
	require.Equal(t, uint64(1), head.NumSeries())

	// Native metadata is not reconstructed during WAL replay, so call the
	// replay-only deletion path directly to verify it cleans a populated store.
	head.deleteSeriesByID([]chunks.HeadSeriesRef{chunks.HeadSeriesRef(ref)})

	_, _, ok = head.nativeMetricMetadata.get(chunks.HeadSeriesRef(ref))
	require.False(t, ok)
	require.Zero(t, head.nativeMetricMetadata.series.Load())
	require.Zero(t, head.nativeMetricMetadata.versions.Load())
	require.Zero(t, head.NumSeries())
}

func TestHeadNativeMetricMetadataIsFeatureGatedAndNonPersistent(t *testing.T) {
	ctx := context.Background()
	matcher := labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+")

	t.Run("disabled", func(t *testing.T) {
		head, _ := newTestHead(t, 1000, compression.None, false)
		_, _, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{matcher}}, 0)
		require.ErrorIs(t, err, ErrNativeMetadataDisabled)
	})

	t.Run("reset clears in-memory metadata", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(1000, false)
		opts.EnableNativeMetadata = true
		head, _ := newTestHeadWithOptions(t, compression.None, opts)
		app := head.AppenderV2(ctx)
		_, err := app.Append(0, labels.FromStrings(labels.MetricName, "metric"), 0, 100, 1, nil, nil, storage.AOptions{
			NativeMetricMetadata: metadata.Metadata{Type: model.MetricTypeGauge},
		})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.NoError(t, head.resetInMemoryState())
		result, _, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{matcher}}, 0)
		require.NoError(t, err)
		require.Empty(t, result)
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
			NativeMetricMetadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: name},
		})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	// Supplying metadata without the RW2 native-storage intent does not
	// populate the native store.
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
			limit:     2,
			wantNames: []string{"a", "b"},
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
}
