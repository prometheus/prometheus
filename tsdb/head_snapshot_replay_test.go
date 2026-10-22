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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

func TestRecoverSnapshotSeries(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		refs                          []uint64
		deleteRef                     uint64
		partial, lateDelete, failScan bool
		lastID                        uint64
		want                          map[chunks.HeadSeriesRef]chunks.HeadSeriesRef
	}{
		{name: "aliases", refs: []uint64{1, 2, 3}, want: map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{2: 1, 3: 1}},
		{name: "canonical definition absent", refs: []uint64{2, 3}, want: map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{2: 1, 3: 1}},
		{name: "partial tombstone", refs: []uint64{1, 2}, deleteRef: 2, partial: true, want: map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{2: 1}},
		{name: "delete by alias and recreate", refs: []uint64{1, 2}, deleteRef: 2, want: map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{3: 1}},
		{name: "late retired alias tombstone", refs: []uint64{1, 2}, deleteRef: 2, lateDelete: true, want: map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{3: 1}},
		{name: "failed scan commits no metadata", refs: []uint64{1, 2}, failScan: true},
		{name: "preserve higher startup ID", refs: []uint64{1, 2}, lastID: 100, want: map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{2: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, wal := newTestHead(t, 10000, compression.None, false)
			ls := labels.FromStrings("foo", "bar")
			ms, _, err := h.getOrCreateWithOptionalID(1, ls.Hash(), ls, false)
			require.NoError(t, err)
			h.lastSeriesID.Store(max(1, tc.lastID))
			var enc record.Encoder
			for _, ref := range tc.refs {
				require.NoError(t, wal.Log(enc.Series([]record.RefSeries{{Ref: chunks.HeadSeriesRef(ref), Labels: ls}}, nil)))
			}
			if tc.deleteRef != 0 {
				interval := tombstones.Interval{Mint: math.MinInt64, Maxt: math.MaxInt64}
				if tc.partial {
					interval = tombstones.Interval{Mint: 100, Maxt: 200}
				}
				require.NoError(t, wal.Log(enc.Tombstones([]tombstones.Stone{{Ref: storage.SeriesRef(tc.deleteRef), Intervals: tombstones.Intervals{interval}}}, nil)))
				if !tc.partial {
					require.NoError(t, wal.Log(enc.Series([]record.RefSeries{{Ref: 3, Labels: ls}}, nil)))
					if tc.lateDelete {
						require.NoError(t, wal.Log(enc.Tombstones([]tombstones.Stone{{Ref: 1, Intervals: tombstones.Intervals{interval}}}, nil)))
					}
				}
			}
			// Tombstone-only references must not be allocated again, even for partial deletions.
			require.NoError(t, wal.Log(enc.Tombstones([]tombstones.Stone{{Ref: 42, Intervals: tombstones.Intervals{{Mint: 0, Maxt: 10}}}}, nil)))
			if tc.failScan {
				require.NoError(t, wal.Log([]byte{byte(record.Tombstones), 255}))
			}
			idx, offset, err := wal.LastSegmentAndOffset()
			require.NoError(t, err)
			// A definition after the snapshot must not leak into recovered metadata.
			require.NoError(t, wal.Log(enc.Series([]record.RefSeries{{Ref: 99, Labels: ls}}, nil)))
			aliases, err := h.recoverSnapshotSeries("", -1, idx, offset, nil, nil, 0)
			if tc.failScan {
				var corruption *wlog.CorruptionErr
				require.ErrorAs(t, err, &corruption)
				require.Nil(t, aliases)
				require.Equal(t, uint64(1), h.lastSeriesID.Load())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, aliases)
			require.Same(t, ms, h.series.getByID(1))
			require.Equal(t, max(uint64(42), tc.lastID), h.lastSeriesID.Load())
			newLabels := labels.FromStrings("new", "series")
			next, _, err := h.getOrCreate(newLabels.Hash(), newLabels, false)
			require.NoError(t, err)
			require.Greater(t, uint64(next.ref), max(uint64(42), tc.lastID))
		})
	}
}

func TestSnapshotSeriesRecovery(t *testing.T) {
	for _, tc := range []struct {
		name                                                                             string
		alias, nonzeroOffset, fastStartup, disableOOO, mappedOOO, missingWBL, tailSeries bool
		failure                                                                          string
	}{
		{name: "zero offset", alias: true},
		{name: "nonzero offset", alias: true, nonzeroOffset: true},
		{name: "fast startup", alias: true, fastStartup: true},
		{name: "OOO disabled with retained WBL", alias: true, disableOOO: true},
		{name: "mapped OOO without WBL", alias: true, mappedOOO: true, missingWBL: true},
		{name: "prefix framing corruption", alias: true, failure: "corrupt"},
		{name: "fast startup prefix corruption", alias: true, fastStartup: true, failure: "corrupt"},
		{name: "WBL corruption uses normal repair", alias: true, failure: "wbl"},
		{name: "WAL tail corruption uses normal repair", alias: true, failure: "tail"},
		{name: "prefix decode corruption", alias: true, failure: "decode"},
		{name: "physically truncated prefix", alias: true, failure: "truncated"},
		{name: "prefix open failure", alias: true, failure: "missing"},
		{name: "canonical OOO ignores obsolete corruption", failure: "corrupt"},
		{name: "tail definition avoids prefix scan", tailSeries: true, failure: "corrupt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			opts := DefaultOptions()
			opts.WALSegmentSize = 32 * 1024
			opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
			opts.EnableFastStartup = tc.fastStartup
			openDB := func() *DB {
				db := newTestDB(t, withDir(dir), withOpts(opts))
				db.DisableCompactions()
				return db
			}
			ls := labels.FromStrings("foo", "bar")
			appendSample := func(db *DB, ts int64) storage.SeriesRef {
				app := db.Appender(t.Context())
				ref, err := app.Append(0, ls, ts, float64(ts))
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				return ref
			}
			db := openDB()
			if tc.alias {
				appendSample(db, 100*time.Minute.Milliseconds())
				require.NoError(t, db.CompactHead(NewRangeHead(db.head, 0, 100*time.Minute.Milliseconds())))
			}
			appendSample(db, 300*time.Minute.Milliseconds())
			if !tc.tailSeries {
				for i := range 33 {
					if !tc.mappedOOO && i > 0 {
						break
					}
					appendSample(db, 250*time.Minute.Milliseconds()+int64(i))
				}
			}
			require.NoError(t, db.Close())
			opts.EnableMemorySnapshotOnShutdown = true
			db = openDB()
			if tc.nonzeroOffset {
				appendSample(db, 400*time.Minute.Milliseconds())
			}
			require.NoError(t, db.Close())
			snapshotDir, snapshotIdx, offset, err := LastChunkSnapshot(dir)
			require.NoError(t, err)
			if tc.nonzeroOffset {
				require.Positive(t, offset)
			} else {
				require.Zero(t, offset)
			}
			if tc.missingWBL {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "wbl")))
			}
			if tc.disableOOO {
				opts.OutOfOrderTimeWindow = 0
			}
			// Write a real WAL tail definition while snapshots are disabled, leaving the old snapshot intact.
			if tc.tailSeries {
				opts.EnableMemorySnapshotOnShutdown = false
				db = openDB()
				ls = labels.FromStrings("foo", "tail")
				appendSample(db, 300*time.Minute.Milliseconds())
				appendSample(db, 250*time.Minute.Milliseconds())
				require.NoError(t, db.Close())
				opts.EnableMemorySnapshotOnShutdown = true
			}
			prefix := wlog.SegmentName(filepath.Join(dir, "wal"), 0)
			original, err := os.ReadFile(prefix)
			require.NoError(t, err)
			switch tc.failure {
			case "corrupt":
				bad := append([]byte(nil), original...)
				bad[0] = 255
				require.NoError(t, os.WriteFile(prefix, bad, 0o600))
			case "truncated":
				require.NoError(t, os.Truncate(prefix, int64(len(original)-1)))
			case "decode":
				wal, err := wlog.NewSize(nil, nil, t.TempDir(), 32*1024, compression.None)
				require.NoError(t, err)
				require.NoError(t, wal.Log([]byte{byte(record.Series), 255}))
				require.NoError(t, wal.Close())
				bad, err := os.ReadFile(wlog.SegmentName(wal.Dir(), 0))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(prefix, bad, 0o600))
			case "wbl", "tail":
				wal, err := wlog.NewSize(nil, nil, t.TempDir(), 32*1024, compression.None)
				require.NoError(t, err)
				if tc.failure == "wbl" {
					var enc record.Encoder
					require.NoError(t, wal.Log(enc.Samples([]record.RefSample{{Ref: 2, T: 250 * time.Minute.Milliseconds(), V: float64(250 * time.Minute.Milliseconds())}}, nil)))
				}
				badType := record.Series
				if tc.failure == "wbl" {
					badType = record.Samples
				}
				require.NoError(t, wal.Log([]byte{byte(badType), 255}))
				require.NoError(t, wal.Close())
				bad, err := os.ReadFile(wlog.SegmentName(wal.Dir(), 0))
				require.NoError(t, err)
				path := wlog.SegmentName(filepath.Join(dir, "wal"), snapshotIdx+1)
				if tc.failure == "wbl" {
					path = wlog.SegmentName(filepath.Join(dir, "wbl"), 0)
				}
				require.NoError(t, os.WriteFile(path, bad, 0o600))
			case "missing":
				// A missing prefix segment must not silently erase the alias definitions.
				require.Positive(t, snapshotIdx)
				require.NoError(t, os.Remove(prefix))
			}
			before := snapshotReplayFiles(t, dir)
			if tc.alias && (tc.failure == "corrupt" || tc.failure == "decode" || tc.failure == "missing" || tc.failure == "truncated") {
				failed, err := Open(dir, nil, nil, opts, nil)
				require.Nil(t, failed)
				var recoveryErr *errSnapshotSeriesRecovery
				require.ErrorAs(t, err, &recoveryErr)
				if tc.failure != "missing" {
					var corruption *wlog.CorruptionErr
					require.ErrorAs(t, err, &corruption)
					require.Equal(t, filepath.Join(dir, "wal"), corruption.Dir)
					require.Zero(t, corruption.Segment)
					require.Positive(t, corruption.Offset)
					if tc.failure == "truncated" {
						require.Equal(t, int64(len(original)-1), corruption.Offset)
					}
				}
				after := snapshotReplayFiles(t, dir)
				for path, content := range before {
					require.Equal(t, content, after[path], path)
				}
				for path, content := range after {
					if _, existed := before[path]; !existed {
						require.Empty(t, content, path)
						require.Contains(t, []string{filepath.Join(dir, "wal"), filepath.Join(dir, "wbl")}, filepath.Dir(path))
					}
				}
				require.True(t, opts.EnableMemorySnapshotOnShutdown)
				latest, _, _, err := LastChunkSnapshot(dir)
				require.NoError(t, err)
				require.Equal(t, snapshotDir, latest)
				// Restoring the prefix must permit a fresh open, including reacquiring the lock.
				require.NoError(t, os.WriteFile(prefix, original, 0o600))
			}
			for restart := range 2 {
				db = openDB()
				if (tc.failure == "wbl" || tc.failure == "tail") && restart == 0 {
					require.Equal(t, float64(1), prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
				}
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", ls.Get("foo")))
				want := []chunks.Sample{}
				if tc.alias {
					want = append(want, sample{t: 100 * time.Minute.Milliseconds(), f: float64(100 * time.Minute.Milliseconds())})
				}
				for i := range 33 {
					if tc.failure == "tail" && restart == 0 {
						break
					}
					if !tc.mappedOOO && i > 0 {
						break
					}
					if tc.missingWBL && i == 32 {
						break
					} // The final OOO head sample was only in the removed WBL.
					ts := 250*time.Minute.Milliseconds() + int64(i)
					want = append(want, sample{t: ts, f: float64(ts)})
				}
				want = append(want, sample{t: 300 * time.Minute.Milliseconds(), f: float64(300 * time.Minute.Milliseconds())})
				if tc.nonzeroOffset {
					want = append(want, sample{t: 400 * time.Minute.Milliseconds(), f: float64(400 * time.Minute.Milliseconds())})
				}
				requireEqualSeries(t, map[string][]chunks.Sample{ls.String(): want}, got, true)
				require.Zero(t, prom_testutil.ToFloat64(db.head.metrics.snapshotReplayErrorTotal))
				require.Zero(t, prom_testutil.ToFloat64(db.head.metrics.wblReplayUnknownRefsTotal.WithLabelValues("series")))
				require.NoError(t, db.Close())
			}
		})
	}
}

// snapshotReplayFiles captures persisted replay inputs, excluding the transient lock file.
func snapshotReplayFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "lock" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[path] = data
		}
		return err
	}))
	return files
}

func TestSnapshotSeriesRecoveryWithoutCanonicalDefinition(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.WALSegmentSize = 32 * 1024
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.EnableMemorySnapshotOnShutdown = true
	openDB := func() *DB {
		db := newTestDB(t, withDir(dir), withOpts(opts))
		db.DisableCompactions()
		return db
	}
	ls := labels.FromStrings("foo", "bar")
	appendSample := func(db *DB, ts int64) storage.SeriesRef {
		app := db.Appender(t.Context())
		ref, err := app.Append(0, ls, ts, float64(ts))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		return ref
	}
	db := openDB()
	canonical := appendSample(db, 100*time.Minute.Milliseconds())
	require.NoError(t, db.Close())
	opts.EnableMemorySnapshotOnShutdown = false
	db = openDB()
	require.NoError(t, db.CompactHead(NewRangeHead(db.head, 0, 100*time.Minute.Milliseconds())))
	alias := appendSample(db, 300*time.Minute.Milliseconds())
	require.NotEqual(t, canonical, alias)
	appendSample(db, 250*time.Minute.Milliseconds())
	app := db.Appender(t.Context())
	for i := range 32 {
		_, err := app.Append(0, labels.FromStrings("padding", strings.Repeat("x", 8192), "id", strconv.Itoa(i)), 300*time.Minute.Milliseconds(), 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.NoError(t, db.CompactHead(NewRangeHead(db.head, db.head.MinTime(), 300*time.Minute.Milliseconds())))
	cp, _, err := wlog.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err)
	r, err := wlog.NewSegmentsReader(cp)
	require.NoError(t, err)
	reader := wlog.NewReader(r)
	dec := record.NewDecoder(nil, db.logger)
	found := map[chunks.HeadSeriesRef]bool{}
	for reader.Next() {
		if dec.Type(reader.Record()) == record.Series {
			series, err := dec.Series(reader.Record(), nil)
			require.NoError(t, err)
			for _, s := range series {
				found[s.Ref] = true
			}
		}
	}
	require.NoError(t, reader.Err())
	require.NoError(t, r.Close())
	require.False(t, found[chunks.HeadSeriesRef(canonical)])
	require.True(t, found[chunks.HeadSeriesRef(alias)])
	require.NoError(t, db.Close())
	opts.EnableMemorySnapshotOnShutdown = true
	for range 3 {
		db = openDB()
		require.Equal(t, chunks.HeadSeriesRef(canonical), db.head.series.getByHash(ls.Hash(), ls).ref)
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		requireEqualSeries(t, map[string][]chunks.Sample{ls.String(): {
			sample{t: 100 * time.Minute.Milliseconds(), f: float64(100 * time.Minute.Milliseconds())},
			sample{t: 250 * time.Minute.Milliseconds(), f: float64(250 * time.Minute.Milliseconds())},
			sample{t: 300 * time.Minute.Milliseconds(), f: float64(300 * time.Minute.Milliseconds())},
		}}, query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")), true)
		require.NoError(t, db.Close())
	}
	// The retained checkpoint is older than the latest snapshot. Corruption here
	// must not be sent to WAL repair, which would truncate a different directory.
	first, _, err := wlog.Segments(cp)
	require.NoError(t, err)
	prefix := wlog.SegmentName(cp, first)
	original, err := os.ReadFile(prefix)
	require.NoError(t, err)
	bad := append([]byte(nil), original...)
	bad[0] = 255
	require.NoError(t, os.WriteFile(prefix, bad, 0o600))
	before := snapshotReplayFiles(t, dir)
	failed, err := Open(dir, nil, nil, opts, nil)
	require.Nil(t, failed)
	var recoveryErr *errSnapshotSeriesRecovery
	require.ErrorAs(t, err, &recoveryErr)
	var corruption *wlog.CorruptionErr
	require.ErrorAs(t, err, &corruption)
	require.Equal(t, cp, corruption.Dir)
	require.Equal(t, first, corruption.Segment)
	after := snapshotReplayFiles(t, dir)
	for path, content := range before {
		require.Equal(t, content, after[path], path)
	}
	require.NoError(t, os.WriteFile(prefix, original, 0o600))
	db = openDB()
	require.NoError(t, db.Close())
}

func TestSnapshotReplaySegmentBounds(t *testing.T) {
	h, wal := newTestHead(t, 10000, compression.None, false)
	var enc record.Encoder
	require.NoError(t, wal.Log(enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("foo", "bar")}}, nil)))
	idx, offset, err := wal.LastSegmentAndOffset()
	require.NoError(t, err)
	require.NoError(t, wal.Log(enc.Series([]record.RefSeries{{Ref: 2, Labels: labels.FromStrings("foo", "tail")}}, nil)))
	require.NoError(t, wal.Close())
	stat, err := os.Stat(wlog.SegmentName(wal.Dir(), idx))
	require.NoError(t, err)
	for _, tc := range []struct {
		name         string
		end, records int
		corrupt      bool
	}{
		{name: "zero offset", end: 0},
		{name: "record boundary", end: offset, records: 1},
		{name: "page padding", end: int(stat.Size()), records: 2},
		{name: "whole segment", end: -1, records: 2},
		{name: "torn record at boundary", end: offset - 1, corrupt: true},
		{name: "past physical EOF", end: int(stat.Size()) + 1, corrupt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			err := h.readSnapshotReplaySegment(wal.Dir(), idx, 0, tc.end, func(*wlog.Reader, *record.Decoder) error {
				count++
				return nil
			})
			if tc.corrupt {
				var corruption *wlog.CorruptionErr
				require.ErrorAs(t, err, &corruption)
				require.Equal(t, wal.Dir(), corruption.Dir)
				require.Equal(t, idx, corruption.Segment)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.records, count)
			}
		})
	}
}

func TestSnapshotReplayEmptyAndMissing(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		t.Run(strconv.FormatBool(snapshot), func(t *testing.T) {
			dir := t.TempDir()
			opts := DefaultOptions()
			opts.EnableMemorySnapshotOnShutdown = snapshot
			db := newTestDB(t, withDir(dir), withOpts(opts))
			idx, offset, refs, err := db.head.loadChunkSnapshot()
			require.NoError(t, err)
			require.Equal(t, -1, idx)
			require.Zero(t, offset)
			require.Empty(t, refs)
			require.NoError(t, db.Close())
			// An untouched initial WAL has position (0, 0), so its first close skips
			// snapshot creation. The next close writes a valid empty snapshot.
			if snapshot {
				db = newTestDB(t, withDir(dir), withOpts(opts))
				require.NoError(t, db.Close())
			}
			db = newTestDB(t, withDir(dir), withOpts(opts))
			require.Zero(t, db.head.NumSeries())
			require.Zero(t, prom_testutil.ToFloat64(db.head.metrics.snapshotReplayErrorTotal))
			if snapshot {
				_, idx, offset, err := LastChunkSnapshot(dir)
				require.NoError(t, err)
				require.Equal(t, 1, idx)
				require.Zero(t, offset)
			}
			require.NoError(t, db.Close())
		})
	}
}
