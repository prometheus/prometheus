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
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// nativeMetadataRecovery is what replaying a recovery input must restore,
// derived from the input alone, with declarations tracked independently of
// replay: a label set's current incarnation is the ref of its latest Series
// record or handoff in WAL order, and duplicates remap onto its first ref.
type nativeMetadataRecovery struct {
	// entries counts the input's native metadata entries.
	entries int
	labels  map[string]labels.Labels
	// surviving is each label set's first ref.
	surviving map[string]chunks.HeadSeriesRef
	current   map[string]chunks.HeadSeriesRef
	// refs counts the distinct refs of each label set's Series records.
	refs map[string]int
	// declared holds the refs with Series records.
	declared map[chunks.HeadSeriesRef]bool
	states   map[chunks.HeadSeriesRef]nativemetadata.State
	// undeclared counts handoffs for refs without a Series record before them.
	undeclared int
	// untrusted is set if the input ends untrusted, so that replay must write
	// a boundary.
	untrusted bool
	// present holds the label sets replay must keep a series for: those with
	// samples after their last Series record, which resets a series' chunks at
	// replay, or with out-of-order samples, which replay applies last. Replay
	// drops the others' series, or may.
	present map[string]bool
}

// readNativeMetadataRecovery reads a WAL directory as replay would: its last
// checkpoint, then the segments after it, then the out-of-order WAL beside it,
// if any.
func readNativeMetadataRecovery(t testing.TB, dir string) nativeMetadataRecovery {
	t.Helper()
	r := nativeMetadataRecovery{
		labels:    map[string]labels.Labels{},
		surviving: map[string]chunks.HeadSeriesRef{},
		current:   map[string]chunks.HeadSeriesRef{},
		refs:      map[string]int{},
		declared:  map[chunks.HeadSeriesRef]bool{},
		states:    map[chunks.HeadSeriesRef]nativemetadata.State{},
		present:   map[string]bool{},
	}
	keys := map[chunks.HeadSeriesRef]string{}
	dec := record.NewDecoder(nil, nil)
	_, _, err := wlog.LastCheckpoint(dir)
	r.untrusted = err == nil
	for _, rec := range readWALRecords(t, dir) {
		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			for _, s := range series {
				key := s.Labels.String()
				if _, ok := keys[s.Ref]; !ok {
					r.refs[key]++
				}
				r.declared[s.Ref] = true
				keys[s.Ref] = key
				r.labels[key] = s.Labels
				if _, ok := r.surviving[key]; !ok {
					r.surviving[key] = s.Ref
				}
				r.current[key] = s.Ref
				r.present[key] = false
			}
		case record.Samples, record.SamplesV2, record.HistogramSamples, record.HistogramSamplesV2, record.CustomBucketsHistogramSamples,
			record.FloatHistogramSamples, record.FloatHistogramSamplesV2, record.CustomBucketsFloatHistogramSamples:
			// Replay drops samples of refs whose Series record it hasn't seen.
			for _, ref := range sampleRecordRefs(t, &dec, rec) {
				if key, ok := keys[ref]; ok {
					r.present[key] = true
				}
			}
		case record.Metadata, record.NativeMetadataCompact:
			entries := decodeNativeMetadataRecordForTest(t, rec)
			r.entries += len(entries)
			r.untrusted = reduceNativeMetadataEntries(r.states, entries, r.untrusted)
			for _, e := range entries {
				if !e.Handoff {
					continue
				}
				if key, ok := keys[e.Ref]; ok {
					r.current[key] = e.Ref
				} else {
					r.undeclared++
				}
			}
		case record.Tombstones:
			stones, err := dec.Tombstones(rec, nil)
			require.NoError(t, err)
			for _, s := range stones {
				// Replay unlinks fully deleted series, which this model doesn't.
				require.False(t, len(s.Intervals) == 1 && s.Intervals[0] == tombstones.Interval{Mint: math.MinInt64, Maxt: math.MaxInt64}, "full deletion of ref %d", s.Ref)
			}
		}
	}
	if wbl := filepath.Join(filepath.Dir(dir), wlog.WblDirName); fileExists(t, wbl) {
		for _, rec := range readWALRecords(t, wbl) {
			for _, ref := range sampleRecordRefs(t, &dec, rec) {
				if key, ok := keys[ref]; ok {
					r.present[key] = true
				}
			}
		}
	}
	return r
}

// sampleRecordRefs returns the refs of a record's samples, if it is a sample
// record of any kind.
func sampleRecordRefs(t testing.TB, dec *record.Decoder, rec []byte) []chunks.HeadSeriesRef {
	t.Helper()
	var refs []chunks.HeadSeriesRef
	switch dec.Type(rec) {
	case record.Samples, record.SamplesV2:
		samples, err := dec.Samples(rec, nil)
		require.NoError(t, err)
		for _, s := range samples {
			refs = append(refs, s.Ref)
		}
	case record.HistogramSamples, record.HistogramSamplesV2, record.CustomBucketsHistogramSamples:
		samples, err := dec.HistogramSamples(rec, nil)
		require.NoError(t, err)
		for _, s := range samples {
			refs = append(refs, s.Ref)
		}
	case record.FloatHistogramSamples, record.FloatHistogramSamplesV2, record.CustomBucketsFloatHistogramSamples:
		samples, err := dec.FloatHistogramSamples(rec, nil)
		require.NoError(t, err)
		for _, s := range samples {
			refs = append(refs, s.Ref)
		}
	}
	return refs
}

func fileExists(t testing.TB, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

// state returns the history a label set's replayed series must hold.
func (r nativeMetadataRecovery) state(key string) nativemetadata.State {
	return r.states[r.current[key]]
}

// startup returns the entries replay must write: a boundary if the input ends
// untrusted, then the startup handoffs in ref order, one for each label set
// with duplicates, carrying its current incarnation's state.
func (r nativeMetadataRecovery) startup() []nativeMetadataEntryForTest {
	var out []nativeMetadataEntryForTest
	for key, n := range r.refs {
		if n < 2 {
			continue
		}
		state := r.state(key)
		e := nativeMetadataEntryForTest{Ref: r.surviving[key], Kind: record.NativeMetadataOverride, Handoff: true, Truncated: state.Truncated}
		for _, p := range state.AppendPoints(nil) {
			e.Points = appendNativeMetadataPointForTest(e.Points, p)
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b nativeMetadataEntryForTest) int { return cmp.Compare(a.Ref, b.Ref) })
	if r.untrusted {
		out = append([]nativeMetadataEntryForTest{{Kind: record.NativeMetadataBoundary}}, out...)
	}
	return out
}

// readWALRecords returns copies of a WAL directory's records in replay order.
func readWALRecords(t testing.TB, dir string) [][]byte {
	t.Helper()
	var ranges []wlog.SegmentRange
	first := 0
	checkpoint, index, err := wlog.LastCheckpoint(dir)
	if err == nil {
		ranges = append(ranges, wlog.SegmentRange{Dir: checkpoint, Last: math.MaxInt32})
		first = index + 1
	} else {
		require.ErrorIs(t, err, record.ErrNotFound)
	}
	ranges = append(ranges, wlog.SegmentRange{Dir: dir, First: first, Last: math.MaxInt32})
	sr, err := wlog.NewSegmentsRangeReader(ranges...)
	require.NoError(t, err)
	defer sr.Close()
	var out [][]byte
	r := wlog.NewReader(sr)
	for r.Next() {
		out = append(out, slices.Clone(r.Record()))
	}
	require.NoError(t, r.Err())
	return out
}

// requireNativeMetadataRecovered requires h, replayed from the input want was
// read from, to hold each label set's expected history under its first ref,
// and to have written exactly the expected startup entries, after which the
// sender's reduction of every surviving ref must equal native's. A label set
// may lack a series only where want predicts no samples for it; it returns how
// many do.
func requireNativeMetadataRecovered(t testing.TB, h *Head, want nativeMetadataRecovery) (absent int) {
	t.Helper()
	wal := readNativeMetadataWAL(t, h.wal.Dir())
	require.GreaterOrEqual(t, len(wal.entries), want.entries)
	require.Equal(t, want.startup(), normalizeNativeMetadataEntries(wal.entries[want.entries:]), "startup writes exactly the expected entries")
	sender := wal.reduce()
	for key, lset := range want.labels {
		s := h.series.getByHash(lset.Hash(), lset)
		if s == nil {
			require.False(t, want.present[key], "replay dropped %s, which the recovery input holds samples for", key)
			absent++
			continue
		}
		require.Equal(t, want.surviving[key], s.ref, key)
		s.Lock()
		var native nativemetadata.State
		if n := s.nativeMetadataLocked(); n != nil {
			native.Replace(n.AppendPoints(nil))
			native.Truncated = n.truncated
		}
		s.Unlock()
		requireNativeMetadataEqual(t, want.state(key), native, "native %s", key)
		requireNativeMetadataEqual(t, want.state(key), sender[s.ref], "sender %s", key)
	}
	return absent
}

// failureRecorder is a testing.TB that records a failure instead of failing.
// FailNow ends the calling goroutine.
type failureRecorder struct {
	testing.TB
	failed bool
}

func (r *failureRecorder) Errorf(string, ...any) { r.failed = true }

func (r *failureRecorder) FailNow() {
	r.failed = true
	runtime.Goexit()
}

// failsCheck reports whether check fails, run on its own goroutine.
func failsCheck(t testing.TB, check func(testing.TB)) bool {
	r := &failureRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		check(r)
	}()
	<-done
	return r.failed
}

func normalizeNativeMetadataEntries(entries []nativeMetadataEntryForTest) []nativeMetadataEntryForTest {
	var out []nativeMetadataEntryForTest
	for _, e := range entries {
		if len(e.Points) == 0 {
			e.Points = nil
		}
		out = append(out, e)
	}
	return out
}

// checkpointNativeMetadataWAL checkpoints every closed segment of h's WAL with
// keep and mint, and truncates the segments it covers, as WAL truncation does.
func checkpointNativeMetadataWAL(t testing.TB, h *Head, keep func(chunks.HeadSeriesRef) bool, mint int64) {
	t.Helper()
	_, err := h.wal.NextSegmentSync()
	require.NoError(t, err)
	first, last, err := wlog.Segments(h.wal.Dir())
	require.NoError(t, err)
	_, err = wlog.Checkpoint(promslog.NewNopLogger(), h.wal, first, last-1, keep, mint, false, true)
	require.NoError(t, err)
	require.NoError(t, h.wal.Truncate(last))
	require.NoError(t, wlog.DeleteCheckpoints(h.wal.Dir(), last-1))
}

func TestNativeMetricMetadataReplay_Handoffs(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	point := func(from int64, help string) nativeMetadataPointForTest {
		return nativeMetadataPointForTest{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}
	}
	group := func(ref chunks.HeadSeriesRef, from int64, help string) []nativeMetadataEntryForTest {
		return []nativeMetadataEntryForTest{{Ref: ref, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(from, help)}}}
	}
	l := labels.FromStrings(labels.MetricName, "metric")
	opts := func() *Options {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		opts.OutOfOrderTimeWindow = math.MaxInt32
		return opts
	}
	commit := func(t *testing.T, db *DB, ts int64, md metadata.Metadata) chunks.HeadSeriesRef {
		app := db.AppenderV2(t.Context())
		ref, err := app.Append(0, l, 0, ts, 1, nil, nil, storage.AOptions{Metadata: md})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		return chunks.HeadSeriesRef(ref)
	}
	retire := func(t *testing.T, db *DB, ref chunks.HeadSeriesRef) {
		db.head.gcSeries([]storage.SeriesRef{storage.SeriesRef(ref)}, math.MaxInt64, func(*memSeries) bool { return true })
		require.Nil(t, db.head.series.getByID(ref))
	}
	// restart closes db and reopens it, checking replay against the recovery
	// input as it was before the reopen.
	restart := func(t *testing.T, db *DB) (*DB, nativeMetadataRecovery) {
		dir := db.Dir()
		require.NoError(t, db.Close())
		want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
		db = newTestDB(t, withDir(dir), withOpts(opts()))
		requireNativeMetadataRecovered(t, db.head, want)
		return db, want
	}
	versions := func(t *testing.T, db *DB) []NativeMetricMetadataVersion {
		return nativeMetadataSnapshot(t, db)[l.String()].Versions
	}
	keepAll := func(chunks.HeadSeriesRef) bool { return true }
	// requireSenderEqual requires the sender's reduction of the WAL to equal
	// native's history for the live series of l.
	requireSenderEqual := func(t *testing.T, db *DB) {
		ref := db.head.series.getByHash(l.Hash(), l).ref
		wal := readNativeMetadataWAL(t, db.head.wal.Dir())
		require.Equal(t, versions(t, db), nativeMetadataVersions(wal.reduce()[ref]))
	}

	// replaySynthetic writes recs as a WAL and opens a DB on it.
	replaySynthetic := func(t *testing.T, recs []any) (*DB, nativeMetadataRecovery) {
		dir := t.TempDir()
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		populateTestWL(t, w, recs, nil, false)
		require.NoError(t, w.Close())
		want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
		db := newTestDB(t, withDir(dir), withOpts(opts()))
		requireNativeMetadataRecovered(t, db.head, want)
		return db, want
	}

	t.Run("a remapped ref", func(t *testing.T) {
		db := newTestDB(t, withOpts(opts()))
		r1 := commit(t, db, 100, m("a"))
		retire(t, db, r1)
		r2 := commit(t, db, 200, m("b"))
		require.NotEqual(t, r1, r2)
		db, want := restart(t, db)
		require.Len(t, want.startup(), 1)
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("b")}}, versions(t, db))
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.nativeMetadataHandoffsWritten.WithLabelValues(nativeMetadataHandoffStartup)))
		// Later changes under the surviving ref extend the handed-off history.
		commit(t, db, 300, m("c"))
		requireSenderEqual(t, db)
		// WAL truncation keeps r2, whose samples last until 200, so its
		// checkpoint keeps r1 current with a handoff, which the Head counts.
		for range 3 {
			_, err := db.head.wal.NextSegmentSync()
			require.NoError(t, err)
		}
		require.NoError(t, db.head.truncateWAL(1))
		_, _, err := wlog.LastCheckpoint(db.head.wal.Dir())
		require.NoError(t, err)
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.nativeMetadataHandoffsWritten.WithLabelValues(nativeMetadataHandoffCheckpoint)))
		db, _ = restart(t, db)
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("b")}, {EffectiveFrom: 300, Metadata: m("c")}}, versions(t, db))
	})

	t.Run("a current incarnation without native state", func(t *testing.T) {
		db := newTestDB(t, withOpts(opts()))
		r1 := commit(t, db, 100, m("a"))
		retire(t, db, r1)
		app := db.AppenderV2(t.Context())
		_, err := app.Append(0, l, 0, 200, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		db, want := restart(t, db)
		// The handoff is empty, and clears the sender's history.
		require.Equal(t, []nativeMetadataEntryForTest{{Ref: r1, Kind: record.NativeMetadataOverride, Handoff: true}}, want.startup())
		require.Empty(t, versions(t, db))
		// A checkpoint keeps the empty winner current.
		checkpointNativeMetadataWAL(t, db.head, keepAll, math.MinInt64)
		db, _ = restart(t, db)
		require.Empty(t, versions(t, db))
	})

	t.Run("repeated restarts", func(t *testing.T) {
		// Checkpoints keeping every series, at each point between the steps.
		const (
			beforeFirstRestart = iota
			afterFirstRestart
			beforeSecondRestart
			afterSecondRestart
		)
		for _, c := range []struct {
			name        string
			checkpoints []int
		}{
			{name: "without checkpoints"},
			{name: "checkpoint before the first restart", checkpoints: []int{beforeFirstRestart}},
			{name: "checkpoint after the first restart", checkpoints: []int{afterFirstRestart}},
			{name: "checkpoint before the second restart", checkpoints: []int{beforeSecondRestart}},
			{name: "checkpoint after the second restart", checkpoints: []int{afterSecondRestart}},
			{name: "checkpoints at every step", checkpoints: []int{beforeFirstRestart, afterFirstRestart, beforeSecondRestart, afterSecondRestart}},
		} {
			t.Run(c.name, func(t *testing.T) {
				checkpointAt := func(db *DB, step int) {
					if slices.Contains(c.checkpoints, step) {
						checkpointNativeMetadataWAL(t, db.head, keepAll, math.MinInt64)
					}
				}
				want := []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("b")}, {EffectiveFrom: 300, Metadata: m("c")}}
				db := newTestDB(t, withOpts(opts()))
				r1 := commit(t, db, 100, m("a"))
				retire(t, db, r1)
				commit(t, db, 200, m("b"))
				checkpointAt(db, beforeFirstRestart)
				db, _ = restart(t, db)
				checkpointAt(db, afterFirstRestart)
				require.Equal(t, r1, commit(t, db, 300, m("c")), "the surviving ref is the first")
				checkpointAt(db, beforeSecondRestart)
				db, _ = restart(t, db)
				require.Equal(t, want, versions(t, db))
				checkpointAt(db, afterSecondRestart)
				db, _ = restart(t, db)
				require.Equal(t, want, versions(t, db))
			})
		}
	})

	t.Run("duplicate expiry", func(t *testing.T) {
		// Ref 2 is a duplicate whose last sample is at 500, and whose metadata
		// starts later. Metadata doesn't extend its WAL expiry.
		db, _ := replaySynthetic(t, []any{
			[]record.RefSeries{{Ref: 1, Labels: l}},
			group(1, 100, "a"),
			[]record.RefSeries{{Ref: 2, Labels: l}},
			[]record.RefSample{{Ref: 2, T: 500, V: 1}},
			group(2, 900, "b"),
		})
		require.True(t, db.head.keepSeriesInWALCheckpointFn(500)(2), "kept at its last sample")
		require.False(t, db.head.keepSeriesInWALCheckpointFn(501)(2), "dropped after it")
		// A later sample keeps the series itself after the checkpoint.
		require.Equal(t, chunks.HeadSeriesRef(1), commit(t, db, 1000, m("b")))
		want := nativeMetadataSnapshot(t, db)
		// Without ref 2, the startup handoff keeps ref 1's history current.
		checkpointNativeMetadataWAL(t, db.head, db.head.keepSeriesInWALCheckpointFn(501), 501)
		db, recovery := restart(t, db)
		require.Equal(t, 1, recovery.refs[l.String()])
		require.Equal(t, want, nativeMetadataSnapshot(t, db))
		// Kept, ref 2 makes the checkpoint write a handoff for ref 1.
		db2, _ := replaySynthetic(t, []any{
			[]record.RefSeries{{Ref: 1, Labels: l}},
			group(1, 100, "a"),
			[]record.RefSeries{{Ref: 2, Labels: l}},
			[]record.RefSample{{Ref: 2, T: 500, V: 1}},
			group(2, 900, "b"),
		})
		commit(t, db2, 1000, m("b"))
		checkpointNativeMetadataWAL(t, db2.head, db2.head.keepSeriesInWALCheckpointFn(500), 500)
		db2, recovery = restart(t, db2)
		require.Equal(t, 2, recovery.refs[l.String()])
		require.Equal(t, chunks.HeadSeriesRef(1), recovery.current[l.String()])
		require.Equal(t, want, nativeMetadataSnapshot(t, db2))
	})

	t.Run("a later series record supersedes a handoff", func(t *testing.T) {
		db := newTestDB(t, withOpts(opts()))
		r1 := commit(t, db, 100, m("a"))
		retire(t, db, r1)
		commit(t, db, 200, m("b"))
		db, _ = restart(t, db)
		retire(t, db, r1)
		r3 := commit(t, db, 400, m("d"))
		require.Greater(t, r3, r1)
		db, want := restart(t, db)
		require.Equal(t, r3, want.current[l.String()])
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 400, Metadata: m("d")}}, versions(t, db))
	})

	t.Run("the allocation race", func(t *testing.T) {
		// A creator reserved ref 1 and paused before publishing it, while ref
		// 2 was created, committed and retired. Ref 1 then logged its Series
		// record after ref 2's.
		db, want := replaySynthetic(t, []any{
			[]record.RefSeries{{Ref: 2, Labels: l}},
			group(2, 100, "b"),
			[]record.RefSample{{Ref: 2, T: 100, V: 1}},
			[]record.RefSeries{{Ref: 1, Labels: l}},
			group(1, 200, "c"),
			[]record.RefSample{{Ref: 1, T: 200, V: 1}},
		})
		require.Equal(t, chunks.HeadSeriesRef(1), want.current[l.String()])
		require.Equal(t, chunks.HeadSeriesRef(2), want.surviving[l.String()])
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("c")}}, versions(t, db))
		require.Equal(t, chunks.HeadSeriesRef(2), commit(t, db, 300, m("d")))
		db, _ = restart(t, db)
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("c")}, {EffectiveFrom: 300, Metadata: m("d")}}, versions(t, db))
	})

	t.Run("a repeated series record", func(t *testing.T) {
		// A failed commit's rollback logs its created series' records again.
		// A ref that repeats its own Series record isn't remapped.
		_, want := replaySynthetic(t, []any{
			[]record.RefSeries{{Ref: 1, Labels: l}},
			group(1, 100, "a"),
			[]record.RefSeries{{Ref: 1, Labels: l}},
			[]record.RefSample{{Ref: 1, T: 100, V: 1}},
		})
		require.Empty(t, want.startup())
	})

	t.Run("the oracle requires every series the input holds samples for", func(t *testing.T) {
		other := labels.FromStrings(labels.MetricName, "other")
		db, want := replaySynthetic(t, []any{
			[]record.RefSeries{{Ref: 1, Labels: l}},
			group(1, 100, "a"),
			[]record.RefSample{{Ref: 1, T: 100, V: 1}},
			// Ref 2's samples precede its repeated Series record, so replay
			// drops it: a permitted absence.
			[]record.RefSeries{{Ref: 2, Labels: other}},
			group(2, 100, "o"),
			[]record.RefSample{{Ref: 2, T: 100, V: 1}},
			[]record.RefSeries{{Ref: 2, Labels: other}},
		})
		require.True(t, want.present[l.String()])
		require.False(t, want.present[other.String()])
		require.Nil(t, db.head.series.getByHash(other.Hash(), other))
		require.Equal(t, 1, requireNativeMetadataRecovered(t, db.head, want), "one permitted absence")
		// Without the series the input holds samples for, the check fails.
		db.head.deleteSeriesByID([]chunks.HeadSeriesRef{1})
		require.True(t, failsCheck(t, func(t testing.TB) { requireNativeMetadataRecovered(t, db.head, want) }))
	})

	t.Run("a failed handoff fails Open", func(t *testing.T) {
		db := newTestDB(t, withOpts(opts()))
		r1 := commit(t, db, 100, m("a"))
		retire(t, db, r1)
		commit(t, db, 200, m("b"))
		dir := db.Dir()
		require.NoError(t, db.Close())

		// Open initializes its Head through initHead. A WAL that can't take
		// metadata fails the startup handoff, and with it the open, without
		// a repair, and without counting a WAL corruption.
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		var mtx sync.Mutex
		fail := true
		failed := map[record.Type]int{}
		w.SetFlushFault(func(rec []byte, at wlog.FlushPoint) error {
			mtx.Lock()
			defer mtx.Unlock()
			if fail && at != wlog.FlushWithinRecord && len(rec) > 0 && nativeMetadataRecords(rec) {
				failed[record.Type(rec[0])]++
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
		require.NotContains(t, err.Error(), "repair")
		require.Zero(t, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
		mtx.Lock()
		fail = false
		require.Positive(t, failed[record.NativeMetadataCompact], "the compact handoff's write failed")
		require.Zero(t, failed[record.Metadata])
		mtx.Unlock()
		require.NoError(t, h.Close())

		// The handoff the failure retained landed when the WAL closed, and
		// the next open converges.
		db, _ = restart(t, newTestDB(t, withDir(dir), withOpts(opts())))
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("b")}}, versions(t, db))
	})

	t.Run("a corrupted WAL fails Open", func(t *testing.T) {
		db := newTestDB(t, withOpts(opts()))
		commit(t, db, 100, m("a"))
		dir := db.Dir()
		require.NoError(t, db.Close())
		// Garbage after the last record is a corruption that replay reports.
		_, last, err := wlog.Segments(filepath.Join(dir, "wal"))
		require.NoError(t, err)
		segment := wlog.SegmentName(filepath.Join(dir, "wal"), last)
		f, err := os.OpenFile(segment, os.O_APPEND|os.O_WRONLY, 0o666)
		require.NoError(t, err)
		_, err = f.Write(bytes.Repeat([]byte{0xff}, 64))
		require.NoError(t, err)
		require.NoError(t, f.Close())
		before, err := os.ReadFile(segment)
		require.NoError(t, err)

		// With native metadata, Open fails with the corruption instead of
		// repairing the WAL and continuing without seeding or handoffs.
		_, err = Open(dir, nil, nil, opts(), nil)
		_, corrupted := errors.AsType[*wlog.CorruptionErr](err)
		require.True(t, corrupted, "Open: %v", err)
		after, err := os.ReadFile(segment)
		require.NoError(t, err)
		require.Equal(t, before, after, "nothing repaired the corrupted segment")

		// Without native metadata, Open repairs it, as before.
		legacy := DefaultOptions()
		db, err = Open(dir, nil, nil, legacy, nil)
		require.NoError(t, err)
		require.NoError(t, db.Close())
	})

	t.Run("a failed native Init releases what Open loaded", func(t *testing.T) {
		dir := t.TempDir()
		createBlock(t, dir, genSeries(2, 1, 0, 100))
		// A WAL whose replay fails, after Open has loaded the block.
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		populateTestWL(t, w, []any{
			[]record.RefSeries{{Ref: 1, Labels: l}},
			[]record.RefSample{{Ref: 1, T: 200, V: 1}},
		}, nil, false)
		require.NoError(t, w.Close())
		_, last, err := wlog.Segments(filepath.Join(dir, "wal"))
		require.NoError(t, err)
		f, err := os.OpenFile(wlog.SegmentName(filepath.Join(dir, "wal"), last), os.O_APPEND|os.O_WRONLY, 0o666)
		require.NoError(t, err)
		_, err = f.Write(bytes.Repeat([]byte{0xff}, 64))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		o := opts()
		var compactorDone <-chan struct{}
		o.NewCompactorFunc = func(ctx context.Context, r prometheus.Registerer, l *slog.Logger, ranges []int64, pool chunkenc.Pool, _ *Options) (Compactor, error) {
			compactorDone = ctx.Done()
			return NewLeveledCompactor(ctx, r, l, ranges, pool, nil)
		}
		var loaded []*Block
		o.BlocksToDelete = func(blocks []*Block) map[ulid.ULID]struct{} {
			loaded = append(loaded, blocks...)
			return nil
		}
		// Only goroutines that Open starts count.
		running := goleak.IgnoreCurrent()
		_, err = Open(dir, nil, nil, o, nil)
		_, corrupted := errors.AsType[*wlog.CorruptionErr](err)
		require.True(t, corrupted, "Open: %v", err)

		require.Len(t, loaded, 1, "Open loaded the block before Init failed")
		_, err = loaded[0].Index()
		require.ErrorIs(t, err, ErrClosing, "the block is closed")
		select {
		case <-compactorDone:
		default:
			t.Fatal("the compactor's context isn't cancelled")
		}
		// The Head's WAL and WBL workers, among others, have stopped.
		goleak.VerifyNone(t, running)
		lock, _, err := fileutil.Flock(filepath.Join(dir, "lock"))
		require.NoError(t, err, "the directory lock is released")
		require.NoError(t, lock.Release())
	})

	t.Run("handoffs without a series record", func(t *testing.T) {
		other := labels.FromStrings(labels.MetricName, "other")
		handoff := func(ref chunks.HeadSeriesRef, from int64, help string) []nativeMetadataEntryForTest {
			return []nativeMetadataEntryForTest{{Ref: ref, Kind: record.NativeMetadataOverride, Handoff: true, Points: []nativeMetadataPointForTest{point(from, help)}}}
		}
		db, want := replaySynthetic(t, []any{
			[]record.RefSeries{{Ref: 1, Labels: l}},
			group(1, 100, "a"),
			// No Series record ever declares ref 5.
			handoff(5, 50, "z"),
			// Ref 2's handoff precedes its Series record, which declares it.
			handoff(2, 60, "y"),
			[]record.RefSeries{{Ref: 2, Labels: l}},
			// Replay keeps only samples after a label set's last Series record.
			[]record.RefSample{{Ref: 2, T: 200, V: 1}},
			[]record.RefSeries{{Ref: 3, Labels: other}},
			group(3, 100, "o"),
			[]record.RefSample{{Ref: 3, T: 100, V: 1}},
		})
		require.Equal(t, 2, want.undeclared)
		require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.nativeMetadataUndeclaredHandoffs))
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 60, Metadata: m("y")}}, versions(t, db))
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: m("o")}}, nativeMetadataSnapshot(t, db)[other.String()].Versions)
		// They still apply as overrides for the sender.
		wal := readNativeMetadataWAL(t, db.head.wal.Dir())
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 50, Metadata: m("z")}}, nativeMetadataVersions(wal.reduce()[5]))
	})

	t.Run("partial startup handoffs", func(t *testing.T) {
		// More remapped label sets than one handoff record holds.
		const n = nativeMetadataRepairBatch + 44
		dir := t.TempDir()
		walDir := filepath.Join(dir, "wal")
		w, err := wlog.New(nil, nil, walDir, compression.None)
		require.NoError(t, err)
		var recs []any
		for i := range n {
			lset := labels.FromStrings(labels.MetricName, "metric", "id", strconv.Itoa(i))
			first, second := chunks.HeadSeriesRef(i+1), chunks.HeadSeriesRef(n+i+1)
			recs = append(recs,
				[]record.RefSeries{{Ref: first, Labels: lset}}, group(first, 100, "a"),
				[]record.RefSeries{{Ref: second, Labels: lset}}, group(second, 200, "b"+strconv.Itoa(i)),
				[]record.RefSample{{Ref: second, T: 200, V: 1}})
		}
		populateTestWL(t, w, recs, nil, false)
		require.NoError(t, w.Close())
		want := readNativeMetadataRecovery(t, walDir)
		require.Len(t, want.startup(), n)

		open := func(t *testing.T, fault func([]byte, wlog.FlushPoint) error) (*Head, error) {
			w, err := wlog.New(nil, nil, walDir, compression.None)
			require.NoError(t, err)
			w.SetFlushFault(fault)
			opts := newTestHeadDefaultOptions(1000, false)
			opts.ChunkDirRoot = dir
			opts.EnableNativeMetadata = true
			opts.EnableNativeMetadataPersistence = true
			h, err := NewHead(nil, nil, w, nil, opts, nil)
			require.NoError(t, err)
			return h, h.Init(math.MinInt64)
		}
		// Fail the second handoff record's final flush, which retains it.
		var mtx sync.Mutex
		records := 0
		var failed record.Type
		h, err := open(t, func(rec []byte, at wlog.FlushPoint) error {
			mtx.Lock()
			defer mtx.Unlock()
			if at == wlog.FlushAfterRecord && len(rec) > 0 && nativeMetadataRecords(rec) {
				records++
				if records == 2 {
					failed = record.Type(rec[0])
					return errNativeMetadataFault
				}
			}
			return nil
		})
		require.ErrorIs(t, err, errNativeMetadataFault, "a failed handoff fails the start")
		mtx.Lock()
		require.Equal(t, 2, records, "the second handoff record failed")
		require.Equal(t, record.NativeMetadataCompact, failed)
		mtx.Unlock()
		h.wal.SetFlushFault(nil)
		require.NoError(t, h.Close())
		// The first record was written, and the retained second one landed
		// when the WAL closed.
		landed := readNativeMetadataWAL(t, walDir).entries[want.entries:]
		require.Equal(t, want.startup(), normalizeNativeMetadataEntries(landed))

		// The next start converges: it writes every handoff again.
		want = readNativeMetadataRecovery(t, walDir)
		h, err = open(t, nil)
		require.NoError(t, err)
		requireNativeMetadataRecovered(t, h, want)
		require.NoError(t, h.Close())
	})
}

// nativeMetadataFreezeManifestsEnv names a directory in which crash-like tests
// keep the manifests of their frozen recovery inputs, if set.
const nativeMetadataFreezeManifestsEnv = "NATIVE_METADATA_FREEZE_MANIFESTS"

// frozenRecoveryInput is an independent copy of a data directory, as a process
// crash would leave it: whatever reached its files, and nothing held only in
// memory. Its manifest lists each file's path, size and SHA-256 digest.
type frozenRecoveryInput struct {
	dir      string
	manifest []string
}

// freezeRecoveryInput copies db's directory byte for byte, flushing nothing,
// once nothing can change its files. The caller has paused every appender and
// any repair write; freezeRecoveryInput asserts the rest: compactions, and with
// them GC, checkpoints and WAL truncation, are disabled, and the chunk write
// queue and checkpoint directories are idle.
func freezeRecoveryInput(t *testing.T, db *DB) frozenRecoveryInput {
	t.Helper()
	db.autoCompactMtx.Lock()
	compacting := db.autoCompact
	db.autoCompactMtx.Unlock()
	require.False(t, compacting, "compactions must be disabled")
	require.True(t, db.head.chunkDiskMapper.IsQueueEmpty(), "the chunk write queue must be idle")

	f := frozenRecoveryInput{dir: t.TempDir()}
	require.NoError(t, filepath.WalkDir(db.Dir(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(db.Dir(), path)
		if err != nil {
			return err
		}
		to := filepath.Join(f.dir, rel)
		if d.IsDir() {
			require.False(t, strings.HasSuffix(rel, wlog.CheckpointTempFileSuffix), "a checkpoint is in progress")
			return os.MkdirAll(to, 0o777)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, b, 0o666); err != nil {
			return err
		}
		from, err := os.Stat(path)
		if err != nil {
			return err
		}
		copied, err := os.Stat(to)
		if err != nil {
			return err
		}
		require.False(t, os.SameFile(from, copied), "%s is linked to the original", rel)
		return nil
	}))
	f.manifest = f.currentManifest(t)
	if dir := os.Getenv(nativeMetadataFreezeManifestsEnv); dir != "" {
		name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + ".txt"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(f.manifest, "\n")+"\n"), 0o666))
	}
	return f
}

func (f frozenRecoveryInput) currentManifest(t *testing.T) []string {
	t.Helper()
	var manifest []string
	require.NoError(t, filepath.WalkDir(f.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(f.dir, path)
		if err != nil {
			return err
		}
		manifest = append(manifest, fmt.Sprintf("%s %d %x", filepath.ToSlash(rel), len(b), sha256.Sum256(b)))
		return nil
	}))
	return manifest
}

// open requires the copy to be unchanged since it was taken, derives the
// expected recovery from it, and opens it, requiring replay to restore that
// recovery. It returns how many label sets replay was permitted to drop.
func (f frozenRecoveryInput) open(t *testing.T, opts *Options) (*DB, int) {
	t.Helper()
	require.Equal(t, f.manifest, f.currentManifest(t), "the frozen input changed")
	want := readNativeMetadataRecovery(t, filepath.Join(f.dir, "wal"))
	db := newTestDB(t, withDir(f.dir), withOpts(opts))
	return db, requireNativeMetadataRecovered(t, db.head, want)
}

// TestNativeMetricMetadataReplay_Restarts classifies restarts: G-exact, native
// history equal to the history before the restart, holds only for a recovery
// input produced at a quiescent boundary. Otherwise G-prefix holds: native, the
// sender and checkpoints equal the reduction of the recovered WAL prefix, which
// can hold in-flight work, or the failure-repair residual.
func TestNativeMetricMetadataReplay_Restarts(t *testing.T) {
	l := labels.FromStrings(labels.MetricName, "a")
	other := labels.FromStrings(labels.MetricName, "b")
	opts := func() *Options {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		opts.OutOfOrderTimeWindow = math.MaxInt32
		return opts
	}
	newHarness := func(t *testing.T) *repairHarness {
		x := newRepairHarness(t)
		x.db.DisableCompactions()
		return x
	}
	versions := func(t *testing.T, db *DB, lset labels.Labels) []NativeMetricMetadataVersion {
		return nativeMetadataSnapshot(t, db)[lset.String()].Versions
	}
	version := func(ts int64, help string) NativeMetricMetadataVersion {
		return NativeMetricMetadataVersion{EffectiveFrom: ts, Metadata: repairMetadata(help).Metadata}
	}
	// history commits a history with a remapped ref, and a failed commit whose
	// repair has been written and discharged.
	history := func(t *testing.T, x *repairHarness) {
		r1 := x.mustCommit(l, 100, "a")
		x.h.gcSeries([]storage.SeriesRef{storage.SeriesRef(r1)}, math.MaxInt64, func(*memSeries) bool { return true })
		r2 := x.mustCommit(l, 200, "b")
		x.mustCommit(other, 100, "o")
		x.faults.arm(recordsOf(record.Samples), wlog.FlushAfterRecord)
		_, err := x.commit(l, 300, "phantom")
		require.ErrorIs(t, err, errNativeMetadataFault)
		require.Zero(t, x.faults.armed())
		require.Equal(t, []record.Type{record.NativeMetadataCompact}, walRecordsWithPoint(t, x.h.wal.Dir(), 300, "phantom"), "the group landed")
		x.check(1, []chunks.HeadSeriesRef{r2})
		require.True(t, x.pass())
		x.check(1, nil)
	}

	t.Run("a quiescent clean restart", func(t *testing.T) {
		x := newHarness(t)
		history(t, x)
		before := nativeMetadataSnapshot(t, x.db)
		dir := x.db.Dir()
		require.NoError(t, x.db.Close())
		require.Zero(t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))
		want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
		db := newTestDB(t, withDir(dir), withOpts(opts()))
		requireNativeMetadataRecovered(t, db.head, want)
		require.Equal(t, before, nativeMetadataSnapshot(t, db), "G-exact")
	})

	t.Run("a quiescent crash-like restart", func(t *testing.T) {
		x := newHarness(t)
		history(t, x)
		before := nativeMetadataSnapshot(t, x.db)
		frozen := freezeRecoveryInput(t, x.db)
		// Neither closing the original nor removing it changes the copy.
		require.NoError(t, x.db.Close())
		require.NoError(t, os.RemoveAll(x.db.Dir()))
		db, absent := frozen.open(t, opts())
		require.Zero(t, absent)
		require.Equal(t, before, nativeMetadataSnapshot(t, db), "G-exact")
	})

	t.Run("a commit logged but not merged", func(t *testing.T) {
		x := newHarness(t)
		x.mustCommit(l, 100, "a")
		paused, resume := make(chan struct{}), make(chan struct{})
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			close(paused)
			<-resume
		})
		done := make(chan error)
		go func() {
			_, err := x.commit(l, 200, "b")
			done <- err
		}()
		<-paused
		// The group is in the WAL, but not yet merged, and no repair is owed.
		before := versions(t, x.db, l)
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a")}, before)
		x.check(0, nil)
		frozen := freezeRecoveryInput(t, x.db)
		close(resume)
		require.NoError(t, <-done)
		require.NoError(t, x.db.Close())

		// The recovered prefix holds the group: in-flight work, which recovery
		// completes, not a repair residual.
		db, absent := frozen.open(t, opts())
		require.Zero(t, absent)
		got := versions(t, db, l)
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a"), version(200, "b")}, got)
		require.NotEqual(t, before, got)
	})

	t.Run("a failed commit with its repair outstanding", func(t *testing.T) {
		x := newHarness(t)
		x.mustCommit(l, 100, "a")
		x.faults.arm(recordsOf(record.Samples), wlog.FlushAfterRecord)
		ref, err := x.commit(l, 200, "phantom")
		require.ErrorIs(t, err, errNativeMetadataFault)
		require.Zero(t, x.faults.armed())
		require.Equal(t, []record.Type{record.NativeMetadataCompact}, walRecordsWithPoint(t, x.h.wal.Dir(), 200, "phantom"), "the group landed")
		// Observed before the crash: the repair is owed, and native never
		// merged the group, which the WAL holds.
		x.check(1, []chunks.HeadSeriesRef{ref})
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a")}, versions(t, x.db, l))
		frozen := freezeRecoveryInput(t, x.db)
		require.True(t, x.pass())
		require.NoError(t, x.db.Close())

		// The residual: everything agrees on the phantom.
		db, absent := frozen.open(t, opts())
		require.Zero(t, absent)
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a"), version(200, "phantom")}, versions(t, db, l))
	})

	t.Run("Commit's override loop interrupted", func(t *testing.T) {
		x := newHarness(t)
		ref := x.mustCommit(l, 100, "a")
		paused, resume := make(chan struct{}), make(chan struct{})
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			close(paused)
			<-resume
		})
		done := make(chan error)
		go func() {
			_, err := x.commit(l, 300, "c")
			done <- err
		}()
		<-paused
		// A later transaction merges first, so the paused one overrides.
		x.mustCommit(l, 200, "b")
		written, release := make(chan struct{}), make(chan struct{})
		x.hooks.overrideAfterWrite = inlineOnce(func(err error) {
			require.NoError(t, err)
			close(written)
			<-release
		})
		close(resume)
		<-written
		// The override is in the WAL; its re-read hasn't happened.
		entries := x.entriesFor(ref)
		require.Equal(t, record.NativeMetadataOverride, entries[len(entries)-1].Kind)
		before := versions(t, x.db, l)
		frozen := freezeRecoveryInput(t, x.db)
		close(release)
		require.NoError(t, <-done)
		require.NoError(t, x.db.Close())

		db, absent := frozen.open(t, opts())
		require.Zero(t, absent)
		require.Equal(t, before, versions(t, db, l), "the override carries the merged history")
	})

	t.Run("a shutdown drop", func(t *testing.T) {
		const timeout = 50 * time.Millisecond
		x := newHarness(t)
		x.h.nativeMetadataRepairs.closeTimeout = timeout
		x.mustCommit(l, 100, "a")
		// The phantom's group is retained, and lands when the WAL closes.
		x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
		ref, err := x.commit(l, 200, "phantom")
		require.ErrorIs(t, err, errNativeMetadataFault)
		require.Zero(t, x.faults.armed())
		require.Equal(t, 1, x.faults.failedRecords(record.NativeMetadataCompact), "the retained group is compact")
		x.check(1, []chunks.HeadSeriesRef{ref})
		// The final pass's preparation crosses Close's deadline, so its
		// repair is never written.
		x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) { time.Sleep(2 * timeout) }
		var admitted atomic.Int64
		x.hooks.repairAfterWrite = func(error) { admitted.Inc() }
		before := versions(t, x.db, l)
		dir := x.db.Dir()
		require.NoError(t, x.db.Close())
		require.Zero(t, admitted.Load())
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))

		want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
		db := newTestDB(t, withDir(dir), withOpts(opts()))
		requireNativeMetadataRecovered(t, db.head, want)
		got := versions(t, db, l)
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a"), version(200, "phantom")}, got, "the phantom landed when the WAL closed")
		require.NotEqual(t, before, got)
	})
}

// TestNativeMetricMetadataReplay_Combined combines repairs, restarts and
// checkpoints, with every native metadata record compact. Each stage checks
// native state, the sender's state, the WAL's reduction, against checkpoints
// of it and replay.
func TestNativeMetricMetadataReplay_Combined(t *testing.T) {
	l := labels.FromStrings(labels.MetricName, "a")
	other := labels.FromStrings(labels.MetricName, "b")
	opts := func() *Options {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		opts.OutOfOrderTimeWindow = math.MaxInt32
		return opts
	}
	newHarness := func(t *testing.T) *repairHarness {
		x := newRepairHarness(t)
		x.db.DisableCompactions()
		return x
	}
	versions := func(t *testing.T, db *DB, lset labels.Labels) []NativeMetricMetadataVersion {
		return nativeMetadataSnapshot(t, db)[lset.String()].Versions
	}
	version := func(ts int64, help string) NativeMetricMetadataVersion {
		return NativeMetricMetadataVersion{EffectiveFrom: ts, Metadata: repairMetadata(help).Metadata}
	}
	// sender returns the sender's history of l's current ref, reduced from
	// the WAL, which holds no native entry outside compact records.
	sender := func(t *testing.T, h *Head, lset labels.Labels) []NativeMetricMetadataVersion {
		t.Helper()
		wal := readNativeMetadataWAL(t, h.wal.Dir())
		require.NotContains(t, wal.types, record.Metadata)
		return nativeMetadataVersions(wal.reduce()[h.series.getByHash(lset.Hash(), lset).ref])
	}
	// checkpoint checkpoints every closed segment, keeping every series, and
	// requires the sender's history to be unchanged.
	checkpoint := func(t *testing.T, h *Head, lset labels.Labels) {
		t.Helper()
		before := sender(t, h, lset)
		checkpointNativeMetadataWAL(t, h, func(chunks.HeadSeriesRef) bool { return true }, math.MinInt64)
		require.Equal(t, before, sender(t, h, lset), "a checkpoint keeps the history")
	}

	t.Run("a retained group, its repair, a checkpoint and crash-like restarts", func(t *testing.T) {
		x := newHarness(t)
		x.mustCommit(l, 100, "a")
		x.failCommit(l, 200, "phantom")
		// The next write flushes the retained group: it lands, a phantom.
		x.mustCommit(other, 100, "o")
		require.Equal(t, []record.Type{record.NativeMetadataCompact}, walRecordsWithPoint(t, x.h.wal.Dir(), 200, "phantom"))
		owed := freezeRecoveryInput(t, x.db)
		require.True(t, x.pass())
		require.Equal(t, versions(t, x.db, l), sender(t, x.h, l), "the repair converges the sender")
		checkpoint(t, x.h, l)
		x.mustCommit(l, 300, "c")
		settled := freezeRecoveryInput(t, x.db)
		want := versions(t, x.db, l)
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a"), version(300, "c")}, want)
		require.Equal(t, want, sender(t, x.h, l))
		require.NoError(t, x.db.Close())

		// G-prefix: the input taken with the repair owed holds the phantom,
		// and replay restores it, as the sender reduces it.
		db, absent := owed.open(t, opts())
		require.Zero(t, absent)
		require.Equal(t, []NativeMetricMetadataVersion{version(100, "a"), version(200, "phantom")}, versions(t, db, l))
		require.Equal(t, versions(t, db, l), sender(t, db.head, l))
		// Convergence: the input taken after the repair and the checkpoint.
		db, absent = settled.open(t, opts())
		require.Zero(t, absent)
		require.Equal(t, want, versions(t, db, l))
		require.Equal(t, want, sender(t, db.head, l))
	})

	t.Run("an interrupted merge-order override, recreation, handoffs and a checkpoint", func(t *testing.T) {
		x := newHarness(t)
		r1 := x.mustCommit(l, 100, "a")
		paused, resume := make(chan struct{}), make(chan struct{})
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			close(paused)
			<-resume
		})
		done := make(chan error)
		go func() {
			_, err := x.commit(l, 300, "c")
			done <- err
		}()
		<-paused
		// A later transaction merges first, so the paused one overrides.
		x.mustCommit(l, 200, "b")
		written, release := make(chan struct{}), make(chan struct{})
		x.hooks.overrideAfterWrite = inlineOnce(func(err error) {
			require.NoError(t, err)
			close(written)
			<-release
		})
		close(resume)
		<-written
		entries := x.entriesFor(r1)
		require.Equal(t, record.NativeMetadataOverride, entries[len(entries)-1].Kind)
		interrupted := freezeRecoveryInput(t, x.db)
		merged := versions(t, x.db, l)
		close(release)
		require.NoError(t, <-done)
		require.Equal(t, merged, sender(t, x.h, l))

		// The series is retired, and recreated under a new ref.
		x.h.gcSeries([]storage.SeriesRef{storage.SeriesRef(r1)}, math.MaxInt64, func(*memSeries) bool { return true })
		r2 := x.mustCommit(l, 400, "d")
		require.NotEqual(t, r1, r2)
		require.NoError(t, x.db.Close())

		// The interrupted override carries the merged history.
		db, absent := interrupted.open(t, opts())
		require.Zero(t, absent)
		require.Equal(t, merged, versions(t, db, l))

		// A restart remaps r2 onto r1, and hands r2's history off to r1.
		dir := x.db.Dir()
		db = newTestDB(t, withDir(dir), withOpts(opts()))
		require.Equal(t, r1, db.head.series.getByHash(l.Hash(), l).ref)
		require.Equal(t, []NativeMetricMetadataVersion{version(400, "d")}, versions(t, db, l))
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.nativeMetadataHandoffsWritten.WithLabelValues(nativeMetadataHandoffStartup)))
		require.Equal(t, versions(t, db, l), sender(t, db.head, l))
		// A checkpoint that keeps the duplicate's Series record declares r1
		// by a compact handoff.
		db.DisableCompactions()
		checkpoint(t, db.head, l)
		cp, _, err := wlog.LastCheckpoint(db.head.wal.Dir())
		require.NoError(t, err)
		var handoffs []chunks.HeadSeriesRef
		for _, rec := range readWALRecords(t, cp) {
			if record.Type(rec[0]) == record.NativeMetadataCompact {
				for _, e := range decodeNativeMetadataRecordForTest(t, rec) {
					if e.Handoff {
						handoffs = append(handoffs, e.Ref)
					}
				}
			}
		}
		require.Equal(t, []chunks.HeadSeriesRef{r1}, handoffs)
		// A second restart reproduces the decision from the checkpoint.
		require.NoError(t, db.Close())
		want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
		require.Equal(t, r1, want.current[l.String()])
		db = newTestDB(t, withDir(dir), withOpts(opts()))
		requireNativeMetadataRecovered(t, db.head, want)
		require.Equal(t, []NativeMetricMetadataVersion{version(400, "d")}, versions(t, db, l))
		require.Equal(t, versions(t, db, l), sender(t, db.head, l))
	})

	t.Run("a shutdown drop, a restart and a checkpoint", func(t *testing.T) {
		const timeout = 50 * time.Millisecond
		x := newHarness(t)
		x.h.nativeMetadataRepairs.closeTimeout = timeout
		x.mustCommit(l, 100, "a")
		// The phantom's compact group is retained, and lands when the WAL
		// closes; its repair is dropped.
		x.failCommit(l, 200, "phantom")
		x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) { time.Sleep(2 * timeout) }
		dir := x.db.Dir()
		require.NoError(t, x.db.Close())
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))

		phantom := []NativeMetricMetadataVersion{version(100, "a"), version(200, "phantom")}
		want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
		db := newTestDB(t, withDir(dir), withOpts(opts()))
		db.DisableCompactions()
		requireNativeMetadataRecovered(t, db.head, want)
		require.Equal(t, phantom, versions(t, db, l))
		require.Equal(t, phantom, sender(t, db.head, l))
		// The residual survives a checkpoint, and another restart.
		checkpoint(t, db.head, l)
		require.NoError(t, db.Close())
		db = newTestDB(t, withDir(dir), withOpts(opts()))
		require.Equal(t, phantom, versions(t, db, l))
		require.Equal(t, phantom, sender(t, db.head, l))
	})
}
