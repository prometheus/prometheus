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

package tsdb_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// TestOOOSamplesLostAfterRestart reproduces
// https://github.com/prometheus/prometheus/issues/20054: on a low-volume
// server, head chunk truncation after a restart deletes every chunk file.
// File numbering used to restart at 1, so new OOO chunks got refs below the
// OOO bookmarks, were skipped by queries and then dropped by head GC.
func TestOOOSamplesLostAfterRestart(t *testing.T) {
	dir := t.TempDir()
	opts := tsdb.DefaultOptions()
	opts.OutOfOrderTimeWindow = (6 * time.Hour).Milliseconds()
	open := func() *tsdb.DB {
		db, err := tsdb.Open(dir, nil, nil, opts, nil)
		require.NoError(t, err)
		db.DisableCompactions()
		return db
	}
	add := func(db *tsdb.DB, name string, ts ...time.Duration) {
		app := db.Appender(context.Background())
		for _, x := range ts {
			_, err := app.Append(0, labels.FromStrings("__name__", name), x.Milliseconds(), 1)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}
	// One scrape per minute, then what the DB run loop does every minute.
	run := func(db *tsdb.DB, from, to time.Duration) {
		for ts := from; ts < to; ts += time.Minute {
			add(db, "scraped", ts)
			db.ForceHeadMMap()
			require.NoError(t, db.Compact(context.Background()))
		}
	}
	// 40 late samples, 2.5h old.
	late := func(db *tsdb.DB, now time.Duration) {
		var ts []time.Duration
		for i := range 40 {
			ts = append(ts, now-150*time.Minute+time.Duration(i)*15*time.Second)
		}
		add(db, "late", ts...)
	}
	count := func(db *tsdb.DB) int {
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		defer q.Close()
		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "late"))
		n := 0
		for ss.Next() {
			it := ss.At().Iterator(nil)
			for it.Next() != chunkenc.ValNone {
				n++
			}
		}
		return n
	}

	db := open()
	run(db, 0, 3*time.Hour)
	late(db, 3*time.Hour)
	run(db, 3*time.Hour, 4*time.Hour+20*time.Minute)
	require.NoError(t, db.Close())

	db = open()
	run(db, 4*time.Hour+20*time.Minute, 5*time.Hour+5*time.Minute)
	late(db, 5*time.Hour+5*time.Minute)
	t.Log("after second late batch:", count(db))
	run(db, 5*time.Hour+5*time.Minute, 10*time.Hour)
	require.NoError(t, db.Close())

	db = open()
	defer db.Close()
	t.Log("after compactions and restart:", count(db))
	require.Equal(t, 80, count(db))
}
