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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// nativeMetadataMode is a row of the native metadata mode matrix, as the TSDB
// sees it: forwarding changes only the remote write sender.
type nativeMetadataMode string

const (
	nativeMetadataOff       nativeMetadataMode = "off"
	nativeMetadataInMemory  nativeMetadataMode = "in-memory"
	nativeMetadataPersisted nativeMetadataMode = "persisted"
)

func (m nativeMetadataMode) options() *Options {
	opts := DefaultOptions()
	opts.EnableNativeMetadata = m != nativeMetadataOff
	opts.EnableNativeMetadataPersistence = m == nativeMetadataPersisted
	opts.OutOfOrderTimeWindow = math.MaxInt32
	return opts
}

// walPageSizeForTest is the WAL's page size, at which records are fragmented.
const walPageSizeForTest = 32 << 10

// walFilesForTest returns the contents of the files under dir, by path
// relative to it.
func walFilesForTest(t testing.TB, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files[rel] = b
		return err
	}))
	return files
}

// requireWALFilesUnchanged requires every file of before to be unchanged in
// dir, which may only have gained empty files: each open starts a segment.
func requireWALFilesUnchanged(t testing.TB, before map[string][]byte, dir string) {
	t.Helper()
	after := walFilesForTest(t, dir)
	for path, b := range before {
		require.Contains(t, after, path)
		require.True(t, bytes.Equal(b, after[path]), "%s is unchanged", path)
	}
	for path, b := range after {
		if _, ok := before[path]; !ok {
			require.Empty(t, b, "%s is new, and empty", path)
		}
	}
}

// writeWALForTest writes recs to new WAL segments in dir, after any there.
func writeWALForTest(t testing.TB, dir string, recs ...any) {
	t.Helper()
	w, err := wlog.New(nil, nil, dir, compression.None)
	require.NoError(t, err)
	populateTestWL(t, w, recs, nil, false)
	require.NoError(t, w.Close())
}

// checkpointWALForTest checkpoints the segments of a closed WAL directory
// through last, keeping the series keep keeps, and truncates them, as a
// Head with or without native metadata persistence does.
func checkpointWALForTest(t testing.TB, dir string, last int, persisted bool, keep func(chunks.HeadSeriesRef) bool) {
	t.Helper()
	w, err := wlog.New(nil, nil, dir, compression.None)
	require.NoError(t, err)
	first, _, err := wlog.Segments(dir)
	require.NoError(t, err)
	if keep == nil {
		keep = func(chunks.HeadSeriesRef) bool { return true }
	}
	_, err = wlog.Checkpoint(promslog.NewNopLogger(), w, first, last, keep, math.MinInt64, false, persisted)
	require.NoError(t, err)
	require.NoError(t, w.Truncate(last+1))
	require.NoError(t, w.Close())
}

// nativeMetadataMarkersForTest returns whether a WAL directory's last
// checkpoint is marked, with a provenance marker before its compact entries,
// and how many boundaries its segments hold.
func nativeMetadataMarkersForTest(t testing.TB, dir string) (marked bool, boundaries int) {
	t.Helper()
	checkpoint, index, err := wlog.LastCheckpoint(dir)
	first := 0
	if err == nil {
		var dec record.Decoder
		for _, rec := range readWALRecords(t, checkpoint) {
			if dec.Type(rec) == record.NativeMetadataCompact {
				entries := decodeNativeMetadataRecordForTest(t, rec)
				marked = len(entries) == 1 && entries[0].Kind == record.NativeMetadataProvenance
				break
			}
		}
		first = index + 1
	} else {
		require.ErrorIs(t, err, record.ErrNotFound)
	}
	sr, err := wlog.NewSegmentsRangeReader(wlog.SegmentRange{Dir: dir, First: first, Last: math.MaxInt32})
	require.NoError(t, err)
	defer sr.Close()
	var dec record.Decoder
	r := wlog.NewReader(sr)
	for r.Next() {
		if dec.Type(r.Record()) != record.NativeMetadataCompact {
			continue
		}
		for _, e := range decodeNativeMetadataRecordForTest(t, r.Record()) {
			if e.Kind == record.NativeMetadataBoundary {
				boundaries++
			}
		}
	}
	require.NoError(t, r.Err())
	return marked, boundaries
}

// lastSegmentForTest returns the last segment of a WAL directory.
func lastSegmentForTest(t testing.TB, dir string) int {
	t.Helper()
	_, last, err := wlog.Segments(dir)
	require.NoError(t, err)
	return last
}

// undecodableNativeMetadataRecordsForTest returns compact records that don't
// decode, by name: unsupported ones, of an unknown format, entry kind and
// entry flag, and a malformed one.
func undecodableNativeMetadataRecordsForTest(t testing.TB) map[string][]byte {
	t.Helper()
	group := compactNativeMetadataForTest(nativeMetadataEntryForTest{Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{
		{EffectiveFrom: 100, Type: uint8(record.Counter), Unit: "seconds", Help: "a"},
		{EffectiveFrom: 200, Type: uint8(record.Counter), Unit: "seconds", Help: "b"},
	}})
	var enc record.Encoder
	valid := enc.CompactNativeMetadata(group.Values, group.Entries, nil)
	// The entry's kind byte follows the type and format bytes, the base and
	// the entry count.
	kindAt := 2 + len(binary.AppendVarint(nil, 100)) + len(binary.AppendUvarint(nil, 1))
	format, kind, flag := slices.Clone(valid), slices.Clone(valid), slices.Clone(valid)
	format[1] = 9
	kind[kindAt] = 6
	flag[kindAt] = 1 << 5
	recs := map[string][]byte{
		"an unknown format":     format,
		"an unknown entry kind": kind,
		"an unknown entry flag": flag,
		"a malformed record":    valid[:len(valid)-1],
	}
	var dec record.Decoder
	var compact record.CompactNativeMetadata
	for name, rec := range recs {
		err := dec.CompactNativeMetadata(rec, &compact)
		require.Error(t, err, name)
		require.Equal(t, name != "a malformed record", errors.Is(err, record.ErrUnsupportedNativeMetadata), name)
	}
	return recs
}

func TestNativeMetricMetadataReplay_Modes(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	l := labels.FromStrings(labels.MetricName, "metric")
	modes := []nativeMetadataMode{nativeMetadataOff, nativeMetadataInMemory, nativeMetadataPersisted}
	commitForMode := func(t *testing.T, db *DB, ts int64, md metadata.Metadata) {
		app := db.AppenderV2(t.Context())
		_, err := app.Append(0, l, 0, ts, float64(ts), nil, nil, storage.AOptions{Metadata: md})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	samples := func(t *testing.T, db *DB) []chunks.Sample {
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		defer q.Close()
		return query(t, q, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric"))[l.String()]
	}

	t.Run("a fresh database", func(t *testing.T) {
		// Only persistence writes compact records.
		for _, mode := range modes {
			t.Run(string(mode), func(t *testing.T) {
				db := newTestDB(t, withOpts(mode.options()))
				commitForMode(t, db, 100, m("a"))
				commitForMode(t, db, 200, m("b"))
				types := readNativeMetadataWAL(t, db.Head().wal.Dir()).types
				require.Equal(t, mode == nativeMetadataPersisted, slices.Contains(types, record.NativeMetadataCompact))
				require.NotContains(t, types, record.Metadata)
			})
		}
	})

	t.Run("a database holding compact records", func(t *testing.T) {
		written := newTestDB(t, withOpts(nativeMetadataPersisted.options()))
		commitForMode(t, written, 100, m("a"))
		commitForMode(t, written, 200, m("b"))
		dir := written.Dir()
		require.NoError(t, written.Close())
		for _, mode := range modes {
			t.Run(string(mode), func(t *testing.T) {
				copied := t.TempDir()
				require.NoError(t, os.CopyFS(copied, os.DirFS(dir)))
				walDir := filepath.Join(copied, "wal")
				before := walFilesForTest(t, walDir)
				db := newTestDB(t, withDir(copied), withOpts(mode.options()))
				require.Equal(t, []chunks.Sample{sample{t: 100, f: 100}, sample{t: 200, f: 200}}, samples(t, db))
				s := db.head.series.getByHash(l.Hash(), l)
				require.NotNil(t, s)
				require.Nil(t, legacyMetadataForTest(s))
				switch mode {
				case nativeMetadataPersisted:
					require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: m("a")}, {EffectiveFrom: 200, Metadata: m("b")}}, nativeMetadataSnapshot(t, db)[l.String()].Versions)
				case nativeMetadataInMemory:
					require.Empty(t, nativeMetadataSnapshot(t, db))
				}
				// Startup wrote nothing beside the new segment.
				requireWALFilesUnchanged(t, before, walDir)

				// Without persistence, a checkpoint drops compact records;
				// with it, the checkpoint is marked and holds the history.
				for range 3 {
					_, err := db.head.wal.NextSegmentSync()
					require.NoError(t, err)
				}
				require.NoError(t, db.head.truncateWAL(1))
				checkpoint, _, err := wlog.LastCheckpoint(walDir)
				require.NoError(t, err)
				types := readNativeMetadataWAL(t, checkpoint).types
				require.Equal(t, mode == nativeMetadataPersisted, slices.Contains(types, record.NativeMetadataCompact), "%v", types)
				marked, _ := nativeMetadataMarkersForTest(t, walDir)
				require.Equal(t, mode == nativeMetadataPersisted, marked)
			})
		}
	})
}

func TestNativeMetricMetadataReplay_UndecodableRecords(t *testing.T) {
	l := labels.FromStrings(labels.MetricName, "metric")
	samples := func(t *testing.T, db *DB) []chunks.Sample {
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		defer q.Close()
		return query(t, q, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric"))[l.String()]
	}

	for name, rec := range undecodableNativeMetadataRecordsForTest(t) {
		unsupported := name != "a malformed record"
		for _, place := range []string{"a segment", "the checkpoint"} {
			t.Run(name+" in "+place, func(t *testing.T) {
				dir := t.TempDir()
				walDir := filepath.Join(dir, "wal")
				series := []record.RefSeries{{Ref: 1, Labels: l}}
				first := []record.RefSample{{Ref: 1, T: 100, V: 1}}
				after := []record.RefSample{{Ref: 1, T: 200, V: 2}}
				if place == "the checkpoint" {
					// Checkpoint 0 holds the record, and segment 1 the
					// samples after it.
					writeWALForTest(t, wlog.CheckpointDir(walDir, 0), series, rec)
					seg, err := wlog.CreateSegment(walDir, 0)
					require.NoError(t, err)
					require.NoError(t, seg.Close())
					writeWALForTest(t, walDir, first, after)
					require.NoError(t, os.Remove(wlog.SegmentName(walDir, 0)))
				} else {
					writeWALForTest(t, walDir, series, first, rec, after)
				}
				before := walFilesForTest(t, walDir)

				// With persistence, Open fails with the record's class, and
				// repairs nothing.
				_, err := Open(dir, nil, nil, nativeMetadataPersisted.options(), nil)
				require.Error(t, err)
				require.Equal(t, unsupported, errors.Is(err, record.ErrUnsupportedNativeMetadata), "%v", err)
				_, corruption := errors.AsType[*wlog.CorruptionErr](err)
				require.Equal(t, !unsupported, corruption, "%v", err)
				require.NotContains(t, err.Error(), "repair")
				requireWALFilesUnchanged(t, before, walDir)

				// Without it, the record is opaque: everything after it is
				// replayed, and nothing is repaired.
				for _, mode := range []nativeMetadataMode{nativeMetadataOff, nativeMetadataInMemory} {
					db, err := Open(dir, nil, nil, mode.options(), nil)
					require.NoError(t, err, mode)
					require.Equal(t, []chunks.Sample{sample{t: 100, f: 1}, sample{t: 200, f: 2}}, samples(t, db), mode)
					require.Zero(t, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal), mode)
					require.NoError(t, db.Close())
					requireWALFilesUnchanged(t, before, walDir)
				}
			})
		}
	}

	t.Run("a checkpoint with persistence", func(t *testing.T) {
		// A record that can't be decoded fails the checkpoint, which then
		// truncates nothing; ingestion continues.
		for name, rec := range undecodableNativeMetadataRecordsForTest(t) {
			t.Run(name, func(t *testing.T) {
				db := newTestDB(t, withOpts(nativeMetadataPersisted.options()))
				app := db.AppenderV2(t.Context())
				_, err := app.Append(0, l, 0, 100, 1, nil, nil, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: "a"}})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				require.NoError(t, db.head.wal.Log(rec))
				for range 3 {
					_, err := db.head.wal.NextSegmentSync()
					require.NoError(t, err)
				}
				err = db.head.truncateWAL(1)
				require.Error(t, err)
				require.Equal(t, name != "a malformed record", errors.Is(err, record.ErrUnsupportedNativeMetadata), "%v", err)
				require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationFail))
				first, _, err := wlog.Segments(db.head.wal.Dir())
				require.NoError(t, err)
				require.Zero(t, first, "no segment is deleted")
				_, _, err = wlog.LastCheckpoint(db.head.wal.Dir())
				require.ErrorIs(t, err, record.ErrNotFound)
				app = db.AppenderV2(t.Context())
				_, err = app.Append(0, l, 0, 200, 2, nil, nil, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: "b"}})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
			})
		}
	})
}

// TestNativeMetricMetadataReplay_TrustBoundary checks the trust rule across
// restarts and intervals without persistence. Each step's expected history is
// written out by hand; the sender's reduction of the WAL must agree with it.
func TestNativeMetricMetadataReplay_TrustBoundary(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	v := func(from int64, help string) NativeMetricMetadataVersion {
		return NativeMetricMetadataVersion{EffectiveFrom: from, Metadata: m(help)}
	}
	l := labels.FromStrings(labels.MetricName, "metric")
	open := func(t *testing.T, dir string, mode nativeMetadataMode) *DB {
		t.Helper()
		db, err := Open(dir, nil, nil, mode.options(), nil)
		require.NoError(t, err)
		return db
	}
	commit := func(t *testing.T, db *DB, ts int64, md metadata.Metadata) {
		t.Helper()
		app := db.AppenderV2(t.Context())
		_, err := app.Append(0, l, 0, ts, 1, nil, nil, storage.AOptions{Metadata: md})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	// start opens dir with persistence, and requires its native history, the
	// sender's reduction of the WAL, and the number of boundaries in its
	// segments.
	start := func(t *testing.T, dir string, want []NativeMetricMetadataVersion, boundaries int) *DB {
		t.Helper()
		db := open(t, dir, nativeMetadataPersisted)
		require.Equal(t, want, nativeMetadataSnapshot(t, db)[l.String()].Versions, "native")
		s := db.head.series.getByHash(l.Hash(), l)
		require.NotNil(t, s)
		require.Equal(t, want, nativeMetadataVersions(readNativeMetadataWAL(t, db.head.wal.Dir()).reduce()[s.ref]), "sender")
		_, got := nativeMetadataMarkersForTest(t, db.head.wal.Dir())
		require.Equal(t, boundaries, got, "boundaries")
		return db
	}
	// interval runs dir without native metadata, committing a change that
	// isn't persisted.
	interval := func(t *testing.T, dir string, ts int64) {
		t.Helper()
		db := open(t, dir, nativeMetadataOff)
		commit(t, db, ts, m("not persisted"))
		require.NoError(t, db.Close())
	}

	t.Run("the boundary is stable across restarts", func(t *testing.T) {
		dir := t.TempDir()
		walDir := filepath.Join(dir, "wal")
		db := open(t, dir, nativeMetadataPersisted)
		commit(t, db, 100, m("a"))
		require.NoError(t, db.Close())
		// An interval without persistence checkpoints everything, dropping
		// the compact records, and leaves the checkpoint unmarked.
		interval(t, dir, 150)
		checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, nil)
		marked, _ := nativeMetadataMarkersForTest(t, walDir)
		require.False(t, marked)

		// The first start trusts nothing before its boundary.
		db = start(t, dir, nil, 1)
		boundarySegment := lastSegmentForTest(t, walDir)
		commit(t, db, 300, m("b"))
		require.NoError(t, db.Close())
		// Later starts find the boundary, trust what follows it, and write
		// none of their own.
		for range 2 {
			db = start(t, dir, []NativeMetricMetadataVersion{v(300, "b")}, 1)
			require.NoError(t, db.Close())
		}

		// A checkpoint that doesn't cover the boundary stays unmarked, and
		// holds no native metadata: there was no legacy-derived state.
		checkpointWALForTest(t, walDir, boundarySegment-1, true, nil)
		marked, boundaries := nativeMetadataMarkersForTest(t, walDir)
		require.False(t, marked)
		require.Equal(t, 1, boundaries)
		checkpoint, _, err := wlog.LastCheckpoint(walDir)
		require.NoError(t, err)
		require.NotContains(t, readNativeMetadataWAL(t, checkpoint).types, record.NativeMetadataCompact)
		db = start(t, dir, []NativeMetricMetadataVersion{v(300, "b")}, 1)
		require.NoError(t, db.Close())

		// One that covers it is marked, and holds the history after it.
		checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), true, nil)
		marked, boundaries = nativeMetadataMarkersForTest(t, walDir)
		require.True(t, marked)
		require.Zero(t, boundaries)
		db = start(t, dir, []NativeMetricMetadataVersion{v(300, "b")}, 0)
		require.NoError(t, db.Close())
	})

	t.Run("a first upgrade from latest-value metadata", func(t *testing.T) {
		for _, checkpointed := range []bool{false, true} {
			dir := t.TempDir()
			walDir := filepath.Join(dir, "wal")
			opts := nativeMetadataOff.options()
			opts.EnableMetadataWALRecords = true
			db, err := Open(dir, nil, nil, opts, nil)
			require.NoError(t, err)
			commit(t, db, 100, m("legacy"))
			require.NoError(t, db.Close())
			boundaries := 0
			if checkpointed {
				checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, nil)
				boundaries = 1
			}
			// Latest-value entries apply in either trust state.
			db = start(t, dir, []NativeMetricMetadataVersion{v(math.MinInt64, "legacy")}, boundaries)
			require.NoError(t, db.Close())
		}
	})

	t.Run("a failed boundary write", func(t *testing.T) {
		dir := t.TempDir()
		walDir := filepath.Join(dir, "wal")
		writeWALForTest(t, walDir, []record.RefSeries{{Ref: 1, Labels: l}}, []record.RefSample{{Ref: 1, T: 100, V: 1}})
		checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, nil)

		w, err := wlog.New(nil, nil, walDir, compression.None)
		require.NoError(t, err)
		var mtx sync.Mutex
		failed := 0
		w.SetFlushFault(func(rec []byte, at wlog.FlushPoint) error {
			mtx.Lock()
			defer mtx.Unlock()
			if at != wlog.FlushWithinRecord && len(rec) > 0 && record.Type(rec[0]) == record.NativeMetadataCompact && failed == 0 {
				failed++
				return errNativeMetadataFault
			}
			return nil
		})
		headOpts := newTestHeadDefaultOptions(1000, false)
		headOpts.ChunkDirRoot = dir
		headOpts.EnableNativeMetadata = true
		headOpts.EnableNativeMetadataPersistence = true
		h, err := NewHead(nil, nil, w, nil, headOpts, nil)
		require.NoError(t, err)
		err = initHead(h, math.MinInt64, w, nil, promslog.NewNopLogger())
		require.ErrorIs(t, err, errNativeMetadataFault)
		require.Equal(t, 1, failed)
		require.NoError(t, h.Close())
		// The failed write may land when the WAL closes; the next start then
		// finds it and writes none, or writes its own.
		_, boundaries := nativeMetadataMarkersForTest(t, walDir)
		require.LessOrEqual(t, boundaries, 1)
		db := open(t, dir, nativeMetadataPersisted)
		_, boundaries = nativeMetadataMarkersForTest(t, walDir)
		require.Equal(t, 1, boundaries)
		commit(t, db, 200, m("a"))
		require.NoError(t, db.Close())
		db = start(t, dir, []NativeMetricMetadataVersion{v(200, "a")}, 1)
		require.NoError(t, db.Close())
	})

	t.Run("a crash after the boundary", func(t *testing.T) {
		for _, ingested := range []bool{false, true} {
			dir := t.TempDir()
			walDir := filepath.Join(dir, "wal")
			writeWALForTest(t, walDir, []record.RefSeries{{Ref: 1, Labels: l}}, []record.RefSample{{Ref: 1, T: 100, V: 1}})
			checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, nil)
			db := open(t, dir, nativeMetadataPersisted)
			var want []NativeMetricMetadataVersion
			if ingested {
				commit(t, db, 200, m("a"))
				want = []NativeMetricMetadataVersion{v(200, "a")}
			}
			// The crashed process's files, as it left them.
			crashed := t.TempDir()
			require.NoError(t, os.CopyFS(crashed, os.DirFS(dir)))
			require.NoError(t, db.Close())
			require.NoError(t, os.Remove(filepath.Join(crashed, "lock")))
			db = start(t, crashed, want, 1)
			require.NoError(t, db.Close())
		}
	})

	t.Run("intervals without persistence", func(t *testing.T) {
		t.Run("without a checkpoint", func(t *testing.T) {
			// The history is carried over, without the intervals' changes.
			dir := t.TempDir()
			db := open(t, dir, nativeMetadataPersisted)
			commit(t, db, 100, m("a"))
			require.NoError(t, db.Close())
			want := []NativeMetricMetadataVersion{v(100, "a")}
			for round := range int64(3) {
				interval(t, dir, 1000*round+500)
				db = start(t, dir, want, 0)
				commit(t, db, 1000*round+1000, m(string(rune('b'+round))))
				want = append(want, v(1000*round+1000, string(rune('b'+round))))
				require.NoError(t, db.Close())
			}
			db = start(t, dir, want, 0)
			require.NoError(t, db.Close())
		})

		t.Run("with a checkpoint covering the boundary", func(t *testing.T) {
			// Each interval's checkpoint drops everything compact, the last
			// boundary included, so each start writes a new boundary and
			// keeps nothing from before it.
			dir := t.TempDir()
			walDir := filepath.Join(dir, "wal")
			db := open(t, dir, nativeMetadataPersisted)
			commit(t, db, 100, m("a"))
			require.NoError(t, db.Close())
			for round := range int64(3) {
				interval(t, dir, 1000*round+500)
				checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, nil)
				db = start(t, dir, nil, 1)
				commit(t, db, 1000*round+1000, m("kept until the next interval"))
				require.NoError(t, db.Close())
			}
		})

		t.Run("with a checkpoint not covering the boundary", func(t *testing.T) {
			// The first boundary after the unmarked checkpoint stays, and
			// everything after it is kept, without the intervals' changes.
			dir := t.TempDir()
			walDir := filepath.Join(dir, "wal")
			db := open(t, dir, nativeMetadataPersisted)
			commit(t, db, 100, m("a"))
			require.NoError(t, db.Close())
			interval(t, dir, 500)
			checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, nil)
			db = start(t, dir, nil, 1)
			boundarySegment := lastSegmentForTest(t, walDir)
			commit(t, db, 1000, m("b"))
			require.NoError(t, db.Close())
			want := []NativeMetricMetadataVersion{v(1000, "b")}
			for round := range int64(3) {
				interval(t, dir, 1000*round+1500)
				// The interval's checkpoint covers what precedes the boundary.
				first, _, err := wlog.Segments(walDir)
				require.NoError(t, err)
				if first < boundarySegment {
					checkpointWALForTest(t, walDir, boundarySegment-1, false, nil)
				}
				db = start(t, dir, want, 1)
				commit(t, db, 1000*round+2000, m(string(rune('c'+round))))
				want = append(want, v(1000*round+2000, string(rune('c'+round))))
				require.NoError(t, db.Close())
			}
		})

		t.Run("a partial drop", func(t *testing.T) {
			// [A@100, B@200]; a backfilled group {A@150} coalesces into A@100
			// and leaves native unchanged. An interval's checkpoint drops the
			// records of A@100 and B@200, but not {A@150}'s. Applied to the
			// lost base, {A@150} would restore [A@150], so that a sample at
			// 250 would be labelled A instead of B. Untrusted, it is skipped.
			dir := t.TempDir()
			walDir := filepath.Join(dir, "wal")
			db := open(t, dir, nativeMetadataPersisted)
			commit(t, db, 100, m("A"))
			commit(t, db, 200, m("B"))
			require.NoError(t, db.Close())
			base := lastSegmentForTest(t, walDir)
			db = open(t, dir, nativeMetadataPersisted)
			commit(t, db, 150, m("A"))
			require.Equal(t, []NativeMetricMetadataVersion{v(100, "A"), v(200, "B")}, nativeMetadataSnapshot(t, db)[l.String()].Versions)
			require.NoError(t, db.Close())
			interval(t, dir, 300)
			checkpointWALForTest(t, walDir, base, false, nil)
			db = start(t, dir, nil, 1)
			require.NoError(t, db.Close())
		})

		t.Run("a stale incarnation", func(t *testing.T) {
			// Ref 1 holds A and is retired, and its labels return as ref 2,
			// with B. An interval's replay remaps ref 2 onto ref 1 without a
			// handoff, and its checkpoint drops ref 2. A replay that trusted
			// what remains would seed ref 1's A.
			dir := t.TempDir()
			walDir := filepath.Join(dir, "wal")
			group := func(ref chunks.HeadSeriesRef, from int64, help string) []nativeMetadataEntryForTest {
				return []nativeMetadataEntryForTest{{Ref: ref, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}}}}
			}
			writeWALForTest(t, walDir,
				[]record.RefSeries{{Ref: 1, Labels: l}}, group(1, 100, "A"), []record.RefSample{{Ref: 1, T: 100, V: 1}},
				[]record.RefSeries{{Ref: 2, Labels: l}}, group(2, 300, "B"), []record.RefSample{{Ref: 2, T: 300, V: 1}},
			)
			db := start(t, dir, []NativeMetricMetadataVersion{v(300, "B")}, 0)
			require.NoError(t, db.Close())
			interval(t, dir, 400)
			checkpointWALForTest(t, walDir, lastSegmentForTest(t, walDir), false, func(ref chunks.HeadSeriesRef) bool { return ref != 2 })
			db = start(t, dir, nil, 1)
			require.Equal(t, chunks.HeadSeriesRef(1), db.head.series.getByHash(l.Hash(), l).ref)
			require.NoError(t, db.Close())
		})
	})
}

// tornWALForTest is a WAL whose last non-empty segment ends in a torn record.
type tornWALForTest struct {
	dir, walDir string
	segment     int
	// start is where the torn record starts in its segment.
	start int64
	// marked is set if the checkpoint is marked; otherwise a start writes a
	// boundary.
	marked bool
}

// newTornWALForTest writes a WAL: a checkpoint, marked or not, of a series with
// native metadata and a sample, then a segment of samples whose last record is
// torn after its first fragments fragments, then empty empty segments.
func newTornWALForTest(t *testing.T, l labels.Labels, fragments, empty int, marked bool) tornWALForTest {
	t.Helper()
	w := tornWALForTest{dir: t.TempDir(), marked: marked}
	w.walDir = filepath.Join(w.dir, "wal")
	writeWALForTest(t, w.walDir,
		[]record.RefSeries{{Ref: 1, Labels: l}},
		[]nativeMetadataEntryForTest{{Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{{EffectiveFrom: 100, Type: uint8(record.Counter), Unit: "seconds", Help: "a"}}}},
		[]record.RefSample{{Ref: 1, T: 100, V: 1}},
	)
	checkpointWALForTest(t, w.walDir, lastSegmentForTest(t, w.walDir), marked, nil)

	wl, err := wlog.New(nil, nil, w.walDir, compression.None)
	require.NoError(t, err)
	w.segment = lastSegmentForTest(t, w.walDir)
	populateTestWL(t, wl, []any{[]record.RefSample{{Ref: 1, T: 200, V: 2}}}, nil, false)
	info, err := os.Stat(wlog.SegmentName(w.walDir, w.segment))
	require.NoError(t, err)
	w.start = info.Size()
	// A record of more than fragments pages.
	var samples []record.RefSample
	for i := range fragments * walPageSizeForTest / 8 {
		samples = append(samples, record.RefSample{Ref: 1, T: int64(1000 + i), V: float64(i)})
	}
	populateTestWL(t, wl, []any{samples}, nil, false)
	require.NoError(t, wl.Close())

	// The record's first fragment fills the rest of its page, unless too
	// little is left for a fragment's header.
	page := w.start / walPageSizeForTest
	if walPageSizeForTest-w.start%walPageSizeForTest < 8 {
		page++
	}
	cut := (page + int64(fragments)) * walPageSizeForTest
	name := wlog.SegmentName(w.walDir, w.segment)
	info, err = os.Stat(name)
	require.NoError(t, err)
	require.Less(t, cut, info.Size(), "the record has more than %d fragments", fragments)
	require.NoError(t, os.Truncate(name, cut))
	for i := 1; i <= empty; i++ {
		seg, err := wlog.CreateSegment(w.walDir, w.segment+i)
		require.NoError(t, err)
		require.NoError(t, seg.Close())
	}
	return w
}

// TestNativeMetricMetadataReplay_TornTail checks the one exception to failing
// Open on a WAL error with persistence: an incomplete final record, in the
// last non-empty segment, which replay ends the segment at and startup
// removes before writing anything.
func TestNativeMetricMetadataReplay_TornTail(t *testing.T) {
	l := labels.FromStrings(labels.MetricName, "metric")
	type state struct {
		samples []chunks.Sample
		native  []NativeMetricMetadataVersion
	}
	read := func(t *testing.T, db *DB) state {
		t.Helper()
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		defer q.Close()
		return state{
			samples: query(t, q, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric"))[l.String()],
			native:  nativeMetadataSnapshot(t, db)[l.String()].Versions,
		}
	}
	// expected opens a copy of w's directory truncated independently to the
	// record's start, without the torn record.
	expected := func(t *testing.T, w tornWALForTest) state {
		t.Helper()
		copied := t.TempDir()
		require.NoError(t, os.CopyFS(copied, os.DirFS(w.dir)))
		require.NoError(t, os.Truncate(wlog.SegmentName(filepath.Join(copied, "wal"), w.segment), w.start))
		db, err := Open(copied, nil, nil, nativeMetadataPersisted.options(), nil)
		require.NoError(t, err)
		defer db.Close()
		got := read(t, db)
		require.Equal(t, []chunks.Sample{sample{t: 100, f: 1}, sample{t: 200, f: 2}}, got.samples)
		var native []NativeMetricMetadataVersion
		if w.marked {
			native = []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: "a"}}}
		}
		require.Equal(t, native, got.native)
		return got
	}
	// requireRecovered opens w with persistence, and requires the torn
	// record alone to have been removed.
	requireRecovered := func(t *testing.T, w tornWALForTest) {
		t.Helper()
		want := expected(t, w)
		before := walFilesForTest(t, w.walDir)
		db, err := Open(w.dir, nil, nil, nativeMetadataPersisted.options(), nil)
		require.NoError(t, err)
		require.Equal(t, want, read(t, db))
		require.NoError(t, db.Close())
		// Only the torn record is removed.
		segment := filepath.Base(wlog.SegmentName(w.walDir, w.segment))
		before[segment] = before[segment][:w.start]
		requireWALFilesUnchanged(t, before, w.walDir)
	}
	// requireOpenFails requires Open with persistence to fail, leaving every
	// file unchanged.
	requireOpenFails := func(t *testing.T, dir string) error {
		t.Helper()
		walDir := filepath.Join(dir, "wal")
		before := walFilesForTest(t, walDir)
		_, err := Open(dir, nil, nil, nativeMetadataPersisted.options(), nil)
		require.Error(t, err)
		requireWALFilesUnchanged(t, before, walDir)
		return err
	}

	t.Run("eligible", func(t *testing.T) {
		for _, fragments := range []int{1, 2, 3} {
			for _, empty := range []int{0, 1, 3} {
				t.Run(fmt.Sprintf("%d fragments, %d empty segments", fragments, empty), func(t *testing.T) {
					requireRecovered(t, newTornWALForTest(t, l, fragments, empty, true))
				})
			}
		}
	})

	t.Run("ineligible", func(t *testing.T) {
		t.Run("a checksum failure", func(t *testing.T) {
			w := newTornWALForTest(t, l, 1, 0, true)
			name := wlog.SegmentName(w.walDir, w.segment)
			require.NoError(t, os.Truncate(name, w.start))
			b, err := os.ReadFile(name)
			require.NoError(t, err)
			b[len(b)-1] ^= 0xff
			require.NoError(t, os.WriteFile(name, b, 0o666))
			_, corruption := errors.AsType[*wlog.CorruptionErr](requireOpenFails(t, w.dir))
			require.True(t, corruption)

			// Without persistence, upstream's repair applies.
			db, err := Open(w.dir, nil, nil, nativeMetadataOff.options(), nil)
			require.NoError(t, err)
			require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
			require.NoError(t, db.Close())
		})
		t.Run("a partial fragment", func(t *testing.T) {
			// A crash or a failed write during the write of a record's only
			// fragment. Readers pad a short segment with zeros, so the cut
			// removes non-zero bytes: the sample value's.
			w := newTornWALForTest(t, l, 1, 0, true)
			require.NoError(t, os.Truncate(wlog.SegmentName(w.walDir, w.segment), w.start-10))
			requireOpenFails(t, w.dir)

			// Starting once without persistence repairs it by truncation,
			// after which persistence starts again.
			db, err := Open(w.dir, nil, nil, nativeMetadataOff.options(), nil)
			require.NoError(t, err)
			require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
			require.NoError(t, db.Close())
			db, err = Open(w.dir, nil, nil, nativeMetadataPersisted.options(), nil)
			require.NoError(t, err)
			require.Equal(t, []chunks.Sample{sample{t: 100, f: 1}}, read(t, db).samples)
			require.NoError(t, db.Close())
		})
		t.Run("a later segment with records", func(t *testing.T) {
			w := newTornWALForTest(t, l, 1, 1, true)
			writeWALForTest(t, w.walDir, []record.RefSample{{Ref: 1, T: 300, V: 3}})
			requireOpenFails(t, w.dir)
		})
		t.Run("a torn record in a checkpoint", func(t *testing.T) {
			// The torn segment becomes the checkpoint's.
			w := newTornWALForTest(t, l, 1, 0, true)
			checkpoint, _, err := wlog.LastCheckpoint(w.walDir)
			require.NoError(t, err)
			torn, err := os.ReadFile(wlog.SegmentName(w.walDir, w.segment))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(wlog.SegmentName(checkpoint, 0), torn, 0o666))
			require.NoError(t, os.Remove(wlog.SegmentName(w.walDir, w.segment)))
			seg, err := wlog.CreateSegment(w.walDir, w.segment)
			require.NoError(t, err)
			require.NoError(t, seg.Close())
			requireOpenFails(t, w.dir)
		})
		for name, rec := range undecodableNativeMetadataRecordsForTest(t) {
			t.Run("a final record of "+name, func(t *testing.T) {
				w := newTornWALForTest(t, l, 1, 0, true)
				require.NoError(t, os.Truncate(wlog.SegmentName(w.walDir, w.segment), w.start))
				writeWALForTest(t, w.walDir, rec)
				requireOpenFails(t, w.dir)
			})
		}
	})

	t.Run("a segment that can't be opened for writing", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Getuid() == 0 {
			t.Skip("a read-only file can be opened for writing")
		}
		// Opening the segment to truncate it fails, before the truncation.
		// Each failed start leaves the case eligible, with one more empty
		// segment after it.
		w := newTornWALForTest(t, l, 2, 0, true)
		name := wlog.SegmentName(w.walDir, w.segment)
		require.NoError(t, os.Chmod(name, 0o444))
		for range 3 {
			err := requireOpenFails(t, w.dir)
			require.ErrorContains(t, err, "open WAL segment")
		}
		require.NoError(t, os.Chmod(name, 0o666))
		requireRecovered(t, w)
	})

	t.Run("a clean prefix", func(t *testing.T) {
		// A truncation that reached the disk, whether or not its sync did,
		// leaves a segment ending in a complete record, which opens like any
		// other.
		w := newTornWALForTest(t, l, 2, 1, true)
		want := expected(t, w)
		require.NoError(t, os.Truncate(wlog.SegmentName(w.walDir, w.segment), w.start))
		db, err := Open(w.dir, nil, nil, nativeMetadataPersisted.options(), nil)
		require.NoError(t, err)
		require.Equal(t, want, read(t, db))
		require.NoError(t, db.Close())
	})

	t.Run("failed truncations and syncs", func(t *testing.T) {
		// The checkpoint is unmarked, so a start that recovers writes a
		// boundary: a start that fails must not.
		errFault := errors.New("injected fault")
		// start initializes a Head on w's WAL, as Open does, with hooks.
		start := func(t *testing.T, w tornWALForTest, hooks *nativeMetadataTestHooks) error {
			t.Helper()
			wl, err := wlog.New(nil, nil, w.walDir, compression.None)
			require.NoError(t, err)
			opts := newTestHeadDefaultOptions(1000, false)
			opts.ChunkDirRoot = w.dir
			opts.EnableNativeMetadata = true
			opts.EnableNativeMetadataPersistence = true
			h, err := NewHead(nil, nil, wl, nil, opts, nil)
			require.NoError(t, err)
			h.nativeMetadataTestHooks = hooks
			err = initHead(h, math.MinInt64, wl, nil, promslog.NewNopLogger())
			require.NoError(t, h.Close())
			return err
		}
		// requireRecoveredOnce requires a start without faults to recover,
		// writing one boundary, after the torn segment.
		requireRecoveredOnce := func(t *testing.T, w tornWALForTest) {
			t.Helper()
			want := expected(t, w)
			require.NoError(t, start(t, w, nil))
			info, err := os.Stat(wlog.SegmentName(w.walDir, w.segment))
			require.NoError(t, err)
			require.Equal(t, w.start, info.Size())
			_, boundaries := nativeMetadataMarkersForTest(t, w.walDir)
			require.Equal(t, 1, boundaries)
			db, err := Open(w.dir, nil, nil, nativeMetadataPersisted.options(), nil)
			require.NoError(t, err)
			require.Equal(t, want, read(t, db))
			require.NoError(t, db.Close())
		}

		t.Run("truncation", func(t *testing.T) {
			// A failed truncation leaves every file as it was, and the case
			// eligible, however often it fails.
			w := newTornWALForTest(t, l, 2, 0, false)
			truncations := 0
			hooks := &nativeMetadataTestHooks{tornTailTruncate: func(*os.File, int64) error {
				truncations++
				return errFault
			}}
			for range 3 {
				before := walFilesForTest(t, w.walDir)
				require.ErrorIs(t, start(t, w, hooks), errFault)
				requireWALFilesUnchanged(t, before, w.walDir)
			}
			require.Equal(t, 3, truncations)
			requireRecoveredOnce(t, w)
		})

		for _, c := range []struct {
			name string
			// restore undoes the truncation, as a sync that never reached
			// the disk may.
			restore bool
		}{
			{name: "sync, leaving a clean prefix"},
			{name: "sync, leaving the eligible tail", restore: true},
		} {
			t.Run(c.name, func(t *testing.T) {
				// A failed sync follows a completed truncation, and fails the
				// start, which writes nothing more.
				w := newTornWALForTest(t, l, 2, 0, false)
				segment := filepath.Base(wlog.SegmentName(w.walDir, w.segment))
				before := walFilesForTest(t, w.walDir)
				syncs := 0
				hooks := &nativeMetadataTestHooks{tornTailSync: func(*os.File) error {
					syncs++
					return errFault
				}}
				require.ErrorIs(t, start(t, w, hooks), errFault)
				require.Equal(t, 1, syncs)
				truncated := maps.Clone(before)
				truncated[segment] = before[segment][:w.start]
				requireWALFilesUnchanged(t, truncated, w.walDir)
				if c.restore {
					require.NoError(t, os.WriteFile(wlog.SegmentName(w.walDir, w.segment), before[segment], 0o666))
				}
				requireRecoveredOnce(t, w)
			})
		}
	})

	t.Run("WBL corruption is repaired in every mode", func(t *testing.T) {
		for _, mode := range []nativeMetadataMode{nativeMetadataOff, nativeMetadataInMemory, nativeMetadataPersisted} {
			t.Run(string(mode), func(t *testing.T) {
				db := newTestDB(t, withOpts(mode.options()))
				for _, ts := range []int64{200, 100} {
					app := db.AppenderV2(t.Context())
					_, err := app.Append(0, l, 0, ts, 1, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					require.NoError(t, app.Commit())
				}
				dir := db.Dir()
				require.NoError(t, db.Close())
				wblDir := filepath.Join(dir, wlog.WblDirName)
				name := wlog.SegmentName(wblDir, lastSegmentForTest(t, wblDir))
				f, err := os.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0o666)
				require.NoError(t, err)
				_, err = f.Write([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
				require.NoError(t, err)
				require.NoError(t, f.Close())
				db, err = Open(dir, nil, nil, mode.options(), nil)
				require.NoError(t, err)
				require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
				require.NoError(t, db.Close())
			})
		}
	})
}
