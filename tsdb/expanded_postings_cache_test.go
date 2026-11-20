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
	"strconv"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/index"
)

func TestExpandedPostingsCache(t *testing.T) {
	ctx := context.Background()
	blockID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	cache := newExpandedPostingsCache(1024)
	require.NotNil(t, cache)

	ix := newMockIndex()
	require.NoError(t, ix.WritePostings("a", "1", index.NewListPostings([]storage.SeriesRef{1, 2, 3})))
	require.NoError(t, ix.WritePostings("b", "2", index.NewListPostings([]storage.SeriesRef{2, 3, 4})))

	a := labels.MustNewMatcher(labels.MatchEqual, "a", "1")
	b := labels.MustNewMatcher(labels.MatchEqual, "b", "2")

	p, err := cache.postingsForMatchers(ctx, blockID, ix, a, b)
	require.NoError(t, err)
	refs, err := index.ExpandPostings(p)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{2, 3}, refs)
	require.Len(t, cache.entries, 1)

	// Matcher order is not semantically significant. Passing a nil index proves
	// that this request is served from the existing cache entry.
	p, err = cache.postingsForMatchers(ctx, blockID, nil, b, a)
	require.NoError(t, err)
	refs, err = index.ExpandPostings(p)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{2, 3}, refs)
	require.Len(t, cache.entries, 1)
}

func TestExpandedPostingsCacheEviction(t *testing.T) {
	id1 := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	id2 := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAW")
	id3 := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAX")
	refs := []storage.SeriesRef{1, 2}

	key1 := expandedPostingsCacheKey{blockID: id1, matchers: "a"}
	key2 := expandedPostingsCacheKey{blockID: id2, matchers: "b"}
	key3 := expandedPostingsCacheKey{blockID: id3, matchers: "c"}
	entrySize := expandedPostingsCacheEntrySize(key1, refs)

	cache := newExpandedPostingsCache(2 * entrySize)
	cache.add(key1, refs)
	cache.add(key2, refs)

	// Touch key1 so key2 becomes least recently used.
	_, ok := cache.get(key1)
	require.True(t, ok)

	cache.add(key3, refs)
	_, ok = cache.get(key1)
	require.True(t, ok)
	_, ok = cache.get(key2)
	require.False(t, ok)
	_, ok = cache.get(key3)
	require.True(t, ok)
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
}

func TestExpandedPostingsCacheRemoveBlocks(t *testing.T) {
	id1 := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	id2 := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAW")
	cache := newExpandedPostingsCache(1024)

	key1 := expandedPostingsCacheKey{blockID: id1, matchers: "a"}
	key2 := expandedPostingsCacheKey{blockID: id2, matchers: "b"}
	cache.add(key1, []storage.SeriesRef{1})
	cache.add(key2, []storage.SeriesRef{2})

	cache.removeBlocks(map[ulid.ULID]struct{}{id2: {}})

	_, ok := cache.get(key1)
	require.False(t, ok)
	refs, ok := cache.get(key2)
	require.True(t, ok)
	require.Equal(t, []storage.SeriesRef{2}, refs)
}

func TestExpandedPostingsCacheSkipsOversizedEntry(t *testing.T) {
	id := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	cache := newExpandedPostingsCache(1)
	key := expandedPostingsCacheKey{blockID: id, matchers: "a"}

	cache.add(key, []storage.SeriesRef{1})

	_, ok := cache.get(key)
	require.False(t, ok)
	require.Zero(t, cache.bytes)
}

func TestDBExpandedPostingsCacheOnlyCachesImmutableBlocks(t *testing.T) {
	opts := DefaultOptions()
	opts.NoLockfile = true
	opts.MinBlockDuration = 100
	opts.MaxBlockDuration = 100
	opts.ExpandedPostingsCacheMaxBytes = 1 << 20

	db := newTestDB(t, withOpts(opts))
	require.NotNil(t, db.expandedPostingsCache)

	ctx := context.Background()
	app := db.Appender(ctx)
	for ts := range int64(100) {
		_, err := app.Append(0, labels.FromStrings("job", "api"), ts, float64(ts))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	matcher := labels.MustNewMatcher(labels.MatchEqual, "job", "api")

	// A Head-only query must not populate the immutable-block cache.
	q, err := db.Querier(0, 99)
	require.NoError(t, err)
	ss := q.Select(ctx, false, nil, matcher)
	for ss.Next() {
		it := ss.At().Iterator(nil)
		for it.Next() != 0 {
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, ss.Err())
	require.NoError(t, q.Close())
	require.Empty(t, db.expandedPostingsCache.entries)

	require.NoError(t, db.CompactHead(NewRangeHead(db.Head(), 0, 99)))
	require.NotEmpty(t, db.Blocks())

	// The same query now touches an immutable block and populates the cache.
	q, err = db.Querier(0, 99)
	require.NoError(t, err)
	ss = q.Select(ctx, false, nil, matcher)
	for ss.Next() {
		it := ss.At().Iterator(nil)
		for it.Next() != 0 {
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, ss.Err())
	require.NoError(t, q.Close())
	require.NotEmpty(t, db.expandedPostingsCache.entries)
}

func BenchmarkExpandedPostingsCache(b *testing.B) {
	ix := newMockIndex()
	const values = 10_000
	for i := range values {
		value := strconv.Itoa(i)
		require.NoError(b, ix.WritePostings("instance", value, index.NewListPostings([]storage.SeriesRef{storage.SeriesRef(i + 1)})))
	}

	matcher := labels.MustNewMatcher(labels.MatchRegexp, "instance", ".*1.*")
	ctx := context.Background()
	blockID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")

	b.Run("Uncached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			p, err := PostingsForMatchers(ctx, ix, matcher)
			if err != nil {
				b.Fatal(err)
			}
			for p.Next() {
			}
			if err := p.Err(); err != nil {
				b.Fatal(err)
			}
		}
	})

	cache := newExpandedPostingsCache(16 << 20)
	p, err := cache.postingsForMatchers(ctx, blockID, ix, matcher)
	require.NoError(b, err)
	_, err = index.ExpandPostings(p)
	require.NoError(b, err)

	b.Run("Cached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			p, err := cache.postingsForMatchers(ctx, blockID, ix, matcher)
			if err != nil {
				b.Fatal(err)
			}
			for p.Next() {
			}
			if err := p.Err(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
