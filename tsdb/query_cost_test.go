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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

func TestDB_CostSamplingQueryable(t *testing.T) {
	t.Run("layouts", func(t *testing.T) {
		for _, tc := range []struct {
			name                   string
			blocks                 [][2]int64
			head, ooo, unsupported bool
		}{
			{name: "empty"},
			{name: "in-order head", head: true},
			{name: "single block", blocks: [][2]int64{{0, 100}}},
			{name: "adjacent blocks", blocks: [][2]int64{{0, 50}, {50, 100}}, unsupported: true},
			{name: "overlapping blocks", blocks: [][2]int64{{0, 80}, {20, 100}}, unsupported: true},
			{name: "head and block", blocks: [][2]int64{{0, 50}}, head: true, unsupported: true},
			{name: "OOO head", head: true, ooo: true, unsupported: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				for _, window := range tc.blocks {
					createBlock(t, dir, genSeries(1, 1, window[0], window[1]))
				}
				opts := DefaultOptions()
				opts.OutOfOrderTimeWindow = 1000
				db := newTestDB(t, withDir(dir), withOpts(opts))
				db.DisableCompactions()
				if tc.head {
					app := db.Appender(t.Context())
					_, err := app.Append(0, labels.FromStrings(defaultLabelName, "0"), 80, 1)
					require.NoError(t, err)
					if tc.ooo {
						_, err = app.Append(0, labels.FromStrings(defaultLabelName, "0"), 40, 2)
						require.NoError(t, err)
					}
					require.NoError(t, app.Commit())
				}
				source, supported := db.CostSamplingQueryable()
				require.True(t, supported)
				require.NotNil(t, source)
				for _, chunkQuery := range []bool{false, true} {
					t.Run(map[bool]string{false: "point", true: "chunk"}[chunkQuery], func(t *testing.T) {
						var q storage.LabelQuerier
						var err error
						if chunkQuery {
							q, err = source.ChunkQuerier(25, 90)
						} else {
							q, err = source.Querier(25, 90)
						}
						if tc.unsupported {
							require.ErrorIs(t, err, storage.ErrCostSamplingUnsupported)
							require.Nil(t, q)
						} else {
							require.NoError(t, err)
							require.NoError(t, q.Close())
						}
					})
				}
			})
		}
	})

	t.Run("layout_changes", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = 1000
		db := newTestDB(t, withOpts(opts))
		app := db.Appender(t.Context())
		metric := labels.FromStrings(defaultLabelName, "0")
		_, err := app.Append(0, metric, 20, 1)
		require.NoError(t, err)
		_, err = app.Append(0, metric, 80, 2)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		source, supported := db.CostSamplingQueryable()
		require.True(t, supported)
		pinned, err := source.ChunkQuerier(25, 75)
		require.NoError(t, err)
		defer func() { require.NoError(t, pinned.Close()) }()

		app = db.Appender(t.Context())
		_, err = app.Append(0, metric, 40, 3)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		// The already-pinned reader retains its in-order path; later openings must
		// reject the newly introduced OOO iterable before reading any chunks.
		set := pinned.Select(t.Context(), false, nil, labels.MustNewMatcher(labels.MatchEqual, defaultLabelName, "0"))
		for set.Next() {
			it := set.At().Iterator(nil)
			for it.Next() {
			}
			require.NoError(t, it.Err())
		}
		require.NoError(t, set.Err())
		q, err := source.ChunkQuerier(25, 75)
		require.Nil(t, q)
		require.ErrorIs(t, err, storage.ErrCostSamplingUnsupported)
		plain, err := source.Querier(25, 75)
		require.Nil(t, plain)
		require.ErrorIs(t, err, storage.ErrCostSamplingUnsupported)
	})

	t.Run("reader_cleanup", func(t *testing.T) {
		for _, failure := range []string{"unsupported", "close failure", "opening failure"} {
			for _, chunkQuery := range []bool{false, true} {
				t.Run(failure+map[bool]string{false: "/point", true: "/chunk"}[chunkQuery], func(t *testing.T) {
					dir := t.TempDir()
					createBlock(t, dir, genSeries(1, 1, 0, 50))
					createBlock(t, dir, genSeries(1, 1, 50, 100))
					db := newTestDB(t, withDir(dir))
					db.DisableCompactions()
					opens, closes := 0, 0
					want := errors.New("reader failure")
					db.blockChunkQuerierFunc = func(b BlockReader, mint, maxt int64) (storage.ChunkQuerier, error) {
						opens++
						if failure == "opening failure" && opens == 2 {
							return nil, want
						}
						q, err := NewBlockChunkQuerier(b, mint, maxt)
						if err != nil {
							return nil, err
						}
						var closeErr error
						if failure == "close failure" {
							closeErr = want
						}
						return costSamplingCloseQuerier{ChunkQuerier: q, closes: &closes, closeErr: closeErr}, nil
					}
					source, _ := db.CostSamplingQueryable()
					var q storage.LabelQuerier
					var err error
					if chunkQuery {
						q, err = source.ChunkQuerier(25, 75)
					} else {
						q, err = source.Querier(25, 75)
					}
					require.Nil(t, q)
					if failure == "unsupported" {
						require.ErrorIs(t, err, storage.ErrCostSamplingUnsupported)
						require.Equal(t, 2, closes)
					} else {
						require.ErrorIs(t, err, want)
						require.NotErrorIs(t, err, storage.ErrCostSamplingUnsupported, "reader failures must never trigger advisory fallback")
						if failure == "opening failure" {
							require.Equal(t, 1, closes)
						} else {
							require.Equal(t, 2, closes)
						}
					}
					require.Equal(t, 2, opens)
				})
			}
		}
	})
}

type costSamplingCloseQuerier struct {
	storage.ChunkQuerier
	closes   *int
	closeErr error
}

func (q costSamplingCloseQuerier) Close() error {
	*q.closes++
	return errors.Join(q.ChunkQuerier.Close(), q.closeErr)
}
