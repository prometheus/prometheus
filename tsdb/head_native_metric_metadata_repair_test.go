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
	"maps"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

var errNativeMetadataFault = errors.New("injected WAL flush failure")

// recordSelector selects the records whose flushes a fault fails.
type recordSelector func(rec []byte) bool

// nativeMetadataRecords selects native metadata records, in either format.
func nativeMetadataRecords(rec []byte) bool {
	typ := record.Type(rec[0])
	return typ == record.Metadata || typ == record.NativeMetadataCompact
}

// recordsOf selects records of typ.
func recordsOf(typ record.Type) recordSelector {
	return func(rec []byte) bool { return record.Type(rec[0]) == typ }
}

// walFlushFaults fails chosen WAL page flushes. Each armed fault fails the
// next flush at its position of a record it selects, once.
type walFlushFaults struct {
	mtx    sync.Mutex
	faults []walFlushFault
	// failAll fails every flush that can't tear a record, until cleared.
	failAll bool
	// failRecords fails every such flush of a record it selects, if set.
	failRecords recordSelector
	// failed counts injected failures by position, and failedTypes those of
	// records by the failed record's type.
	failed      map[wlog.FlushPoint]int
	failedTypes map[record.Type]int
}

type walFlushFault struct {
	selects recordSelector
	at      wlog.FlushPoint
}

func newWALFlushFaults(w *wlog.WL) *walFlushFaults {
	f := &walFlushFaults{}
	w.SetFlushFault(f.fault)
	return f
}

func (f *walFlushFaults) arm(selects recordSelector, at wlog.FlushPoint) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.faults = append(f.faults, walFlushFault{selects: selects, at: at})
}

func (f *walFlushFaults) setFailAll(fail bool) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.failAll = fail
}

func (f *walFlushFaults) setFailRecords(selects recordSelector) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.failRecords = selects
}

func (f *walFlushFaults) armed() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return len(f.faults)
}

func (f *walFlushFaults) failedAt(at wlog.FlushPoint) int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.failed[at]
}

// failedRecords returns how many flushes of records of typ failed.
func (f *walFlushFaults) failedRecords(typ record.Type) int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.failedTypes[typ]
}

func (f *walFlushFaults) fault(rec []byte, at wlog.FlushPoint) error {
	checkNoLockHeld("a WAL write")
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.failed == nil {
		f.failed, f.failedTypes = map[wlog.FlushPoint]int{}, map[record.Type]int{}
	}
	fail := func() error {
		f.failed[at]++
		if len(rec) > 0 {
			f.failedTypes[record.Type(rec[0])]++
		}
		return errNativeMetadataFault
	}
	// Failing a flush between two fragments would tear a record.
	if f.failAll && at != wlog.FlushWithinRecord {
		return fail()
	}
	if len(rec) == 0 {
		return nil
	}
	if f.failRecords != nil && f.failRecords(rec) && at != wlog.FlushWithinRecord {
		return fail()
	}
	for i, fault := range f.faults {
		if fault.at == at && fault.selects(rec) {
			f.faults = append(f.faults[:i], f.faults[i+1:]...)
			return fail()
		}
	}
	return nil
}

// nativeMetadataByRef returns the retained native history of every live series
// with native state. The caller must exclude concurrent GC.
func nativeMetadataByRef(h *Head) map[chunks.HeadSeriesRef]nativemetadata.State {
	out := map[chunks.HeadSeriesRef]nativemetadata.State{}
	for i := range h.series.series {
		h.series.locks[i].RLock()
		for ref, s := range h.series.series[i] {
			s.Lock()
			if native := s.nativeMetadataLocked(); native != nil && !s.isGCed() {
				var state nativemetadata.State
				state.Replace(native.AppendPoints(nil))
				state.Truncated = native.truncated
				out[ref] = state
			}
			s.Unlock()
		}
		h.series.locks[i].RUnlock()
	}
	return out
}

// requireNativeMetadataConverged requires every series' history reduced from
// the WAL, as senders, checkpoints and replay reduce it, to equal native's,
// including the truncation flag: a live series' retained history, and a
// retired one's terminal history, from retired. A series without native state
// must reduce to no versions. Every ref with WAL entries must be live or in
// retired.
func requireNativeMetadataConverged(t testing.TB, db *DB, retired map[chunks.HeadSeriesRef]nativemetadata.State) {
	t.Helper()
	h := db.Head()
	// Land anything a failed flush left in the page buffer.
	_, err := h.wal.NextSegmentSync()
	require.NoError(t, err)
	states := readNativeMetadataWAL(t, h.wal.Dir()).reduce()
	native := nativeMetadataByRef(h)
	for ref, want := range native {
		got, ok := states[ref]
		require.True(t, ok, "ref %d has native metadata but no WAL entries", ref)
		requireNativeMetadataEqual(t, want, got, "ref %d", ref)
	}
	for ref, want := range retired {
		require.Nil(t, h.series.getByID(ref), "ref %d is live", ref)
		requireNativeMetadataEqual(t, want, states[ref], "retired ref %d", ref)
	}
	for ref, got := range states {
		if _, ok := native[ref]; ok {
			continue
		}
		if _, ok := retired[ref]; ok {
			continue
		}
		require.NotNil(t, h.series.getByID(ref), "ref %d has WAL entries but left the Head unaccounted for", ref)
		require.Empty(t, nativeMetadataVersions(got), "ref %d has no native metadata", ref)
	}
}

// requireNativeMetadataEqual requires got to hold want's versions and
// truncation flag.
func requireNativeMetadataEqual(t testing.TB, want, got nativemetadata.State, msgAndArgs ...any) {
	t.Helper()
	require.Equal(t, nativeMetadataVersions(want), nativeMetadataVersions(got), msgAndArgs...)
	require.Equal(t, want.Truncated, got.Truncated, msgAndArgs...)
}

// terminalNativeMetadata returns a retired series' native history. Retirement
// freezes it: no transaction reserves the series, so nothing merges into it.
func terminalNativeMetadata(t testing.TB, s *memSeries) nativemetadata.State {
	t.Helper()
	s.Lock()
	defer s.Unlock()
	require.True(t, s.isGCed(), "ref %d isn't retired", s.ref)
	var state nativemetadata.State
	if native := s.nativeMetadataLocked(); native != nil {
		state.Replace(native.AppendPoints(nil))
		state.Truncated = native.truncated
	}
	return state
}

// pendingNativeMetadataRepairs returns the refs of outstanding repairs.
func pendingNativeMetadataRepairs(h *Head) []chunks.HeadSeriesRef {
	r := h.nativeMetadataRepairs
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return slices.Sorted(maps.Keys(r.pending))
}

// settleNativeMetadataRepairs waits for outstanding repairs to be written.
func settleNativeMetadataRepairs(t testing.TB, db *DB) {
	t.Helper()
	require.Eventually(t, func() bool { return len(pendingNativeMetadataRepairs(db.Head())) == 0 }, 10*time.Second, time.Millisecond)
}

func TestNativeMetricMetadataRepair(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	newDB := func(t *testing.T) *DB {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		opts.OutOfOrderTimeWindow = math.MaxInt32
		return newTestDB(t, withOpts(opts))
	}
	a, b := labels.FromStrings(labels.MetricName, "a"), labels.FromStrings(labels.MetricName, "b")
	commit := func(t *testing.T, db *DB, lset labels.Labels, ts int64, md metadata.Metadata) error {
		app := db.AppenderV2(t.Context())
		_, err := app.Append(0, lset, 0, ts, 1, nil, nil, storage.AOptions{Metadata: md})
		require.NoError(t, err)
		return app.Commit()
	}

	for _, c := range []struct {
		name string
		// Whether a commit precedes the failing one, so it isn't a first commit.
		existing bool
		// The flush that fails in the failing commit.
		selects recordSelector
		at      wlog.FlushPoint
		// failed is the failed record's type. With a Samples record, the
		// transaction's compact record must have landed before it; with a
		// Series record, the transaction must have logged no metadata.
		failed record.Type
	}{
		{name: "a retained metadata record of an existing series", existing: true, selects: nativeMetadataRecords, at: wlog.FlushAfterRecord, failed: record.NativeMetadataCompact},
		{name: "a retained metadata record of a first commit", selects: nativeMetadataRecords, at: wlog.FlushAfterRecord, failed: record.NativeMetadataCompact},
		{name: "a landed metadata record of an existing series", existing: true, selects: recordsOf(record.Samples), at: wlog.FlushAfterRecord, failed: record.Samples},
		{name: "a landed metadata record of a first commit", selects: recordsOf(record.Samples), at: wlog.FlushAfterRecord, failed: record.Samples},
		{name: "a failed series record before any metadata", selects: recordsOf(record.Series), at: wlog.FlushAfterRecord, failed: record.Series},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := newDB(t)
			faults := newWALFlushFaults(db.Head().wal)
			if c.existing {
				require.NoError(t, commit(t, db, a, 100, m("a")))
			}
			faults.arm(c.selects, c.at)
			require.ErrorIs(t, commit(t, db, a, 200, m("b")), errNativeMetadataFault)
			require.Zero(t, faults.armed())
			require.Equal(t, 1, faults.failedRecords(c.failed))
			walDir := db.Head().wal.Dir()
			switch c.failed {
			case record.Samples:
				require.Equal(t, []record.Type{record.NativeMetadataCompact}, walRecordsWithPoint(t, walDir, 200, "b"), "the group landed")
			case record.Series:
				require.Empty(t, walRecordsWithPoint(t, walDir, 200, "b"), "no metadata was logged")
			}
			// Any later write flushes what the failed one left in the page.
			require.NoError(t, commit(t, db, b, 300, m("c")))
			settleNativeMetadataRepairs(t, db)
			requireNativeMetadataConverged(t, db, nil)
		})
	}
}

// inlineOnce returns a hook that runs f on the first call only, on the caller's
// goroutine. The caller holds no locks at the hook points it is used for. f may
// reach the hook again, through a nested commit, which then does nothing.
func inlineOnce[T any](f func(T)) func(T) {
	var called atomic.Bool
	return func(v T) {
		if called.CompareAndSwap(false, true) {
			f(v)
		}
	}
}

// repairHarness drives native metadata repairs by hand: the worker never
// starts, and tests run its passes on their own goroutine.
type repairHarness struct {
	t      *testing.T
	db     *DB
	h      *Head
	faults *walFlushFaults
	hooks  *nativeMetadataTestHooks
}

func newRepairHarness(t *testing.T, configure ...func(*Options)) *repairHarness {
	// Registered first, the check runs after the DB's teardown.
	resetLockOrderViolations()
	t.Cleanup(func() { requireNoLockOrderViolations(t) })
	opts := DefaultOptions()
	opts.EnableNativeMetadata = true
	opts.EnableNativeMetadataPersistence = true
	opts.OutOfOrderTimeWindow = math.MaxInt32
	for _, f := range configure {
		f(opts)
	}
	db := newTestDB(t, withOpts(opts))
	x := &repairHarness{t: t, db: db, h: db.Head(), hooks: &nativeMetadataTestHooks{manualRepairs: true}}
	x.h.nativeMetadataTestHooks = x.hooks
	x.faults = newWALFlushFaults(x.h.wal)
	// Every native metadata record is compact.
	t.Cleanup(func() { require.Zero(t, x.faults.failedRecords(record.Metadata), "a Metadata record's flush failed") })
	return x
}

// longNativeMetadataHelp returns help of the largest size native storage keeps
// with repairMetadata's type and unit, distinct by version below 10. An
// override of a full history of them exceeds a repair record's byte target.
func longNativeMetadataHelp(version int) string {
	return strings.Repeat(strconv.Itoa(version), maxNativeMetricMetadataBytes-len(model.MetricTypeCounter)-len("seconds"))
}

func repairMetadata(help string) storage.AOptions {
	if help == "" {
		return storage.AOptions{}
	}
	return storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}}
}

// commit appends one sample in its own transaction, with metadata unless help
// is empty.
func (x *repairHarness) commit(lset labels.Labels, ts int64, help string) (chunks.HeadSeriesRef, error) {
	app := x.db.AppenderV2(x.t.Context())
	ref, err := app.Append(0, lset, 0, ts, 1, nil, nil, repairMetadata(help))
	require.NoError(x.t, err)
	return chunks.HeadSeriesRef(ref), app.Commit()
}

func (x *repairHarness) mustCommit(lset labels.Labels, ts int64, help string) chunks.HeadSeriesRef {
	ref, err := x.commit(lset, ts, help)
	require.NoError(x.t, err)
	return ref
}

func (x *repairHarness) pass() bool { return x.h.nativeMetadataRepairPass() }

// reservations returns ref's pending-commit count, or -1 once it has left the
// Head.
func (x *repairHarness) reservations(ref chunks.HeadSeriesRef) int {
	s := x.h.series.getByID(ref)
	if s == nil {
		return -1
	}
	s.Lock()
	defer s.Unlock()
	return int(s.pendingCommitCount())
}

func (x *repairHarness) epoch(ref chunks.HeadSeriesRef) uint32 {
	s := x.h.series.getByID(ref)
	require.NotNil(x.t, s)
	s.Lock()
	defer s.Unlock()
	return s.nativeMetadataEpochLocked()
}

// check asserts, while no transaction is open, that exactly pending are
// pending, within bound; that each is live and holds exactly its one pin; that
// each of unpinned holds no reservation; and that nothing was dropped and no
// invariant violated.
func (x *repairHarness) check(bound int, pending []chunks.HeadSeriesRef, unpinned ...chunks.HeadSeriesRef) {
	x.t.Helper()
	got := pendingNativeMetadataRepairs(x.h)
	require.LessOrEqual(x.t, len(got), bound, "pending repairs exceed the test's bound")
	require.ElementsMatch(x.t, pending, got)
	require.Equal(x.t, float64(len(got)), prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsPending))
	for _, ref := range pending {
		require.Equal(x.t, 1, x.reservations(ref), "pending series %d must hold exactly its pin", ref)
	}
	for _, ref := range unpinned {
		require.LessOrEqual(x.t, x.reservations(ref), 0, "series %d must hold no reservation", ref)
	}
	require.Zero(x.t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))
	require.Zero(x.t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairViolations))
	require.Zero(x.t, prom_testutil.ToFloat64(x.h.metrics.pendingCommitUnderflow))
}

// entriesFor returns ref's native metadata WAL entries in WAL order.
func (x *repairHarness) entriesFor(ref chunks.HeadSeriesRef) []nativeMetadataEntryForTest {
	var out []nativeMetadataEntryForTest
	for _, e := range readNativeMetadataWAL(x.t, x.h.wal.Dir()).entries {
		if e.Ref == ref {
			out = append(out, e)
		}
	}
	return out
}

func TestNativeMetricMetadataRepair_Sequence(t *testing.T) {
	// One-page segments: a record that doesn't fit in the page's remainder
	// starts a new segment, flushing the page before any of it is copied.
	x := newRepairHarness(t, func(o *Options) { o.WALSegmentSize = 32 << 10 })
	la, lb, lc, le := labels.FromStrings(labels.MetricName, "a"), labels.FromStrings(labels.MetricName, "b"),
		labels.FromStrings(labels.MetricName, "c"), labels.FromStrings(labels.MetricName, "e")
	refA := x.mustCommit(la, 100, "a1")
	refC := x.mustCommit(lc, 100, "c1")
	refE := x.mustCommit(le, 100, "e1")
	// Two failure episodes: T1's two landed groups, then T2's retained group.
	// T3's record and the fillers add none.
	const bound = 3

	// 1. A landed record: T1 emits groups for A and the new B. Its native
	// record lands, its sample record's final flush fails, and it rolls back.
	x.faults.arm(recordsOf(record.Samples), wlog.FlushAfterRecord)
	app := x.db.AppenderV2(t.Context())
	_, err := app.Append(storage.SeriesRef(refA), la, 0, 200, 1, nil, nil, repairMetadata("a2"))
	require.NoError(t, err)
	ref, err := app.Append(0, lb, 0, 200, 1, nil, nil, repairMetadata("b1"))
	require.NoError(t, err)
	refB := chunks.HeadSeriesRef(ref)
	require.ErrorIs(t, app.Commit(), errNativeMetadataFault)
	require.Zero(t, x.faults.armed())
	require.Equal(t, []record.Type{record.NativeMetadataCompact}, walRecordsWithPoint(t, x.h.wal.Dir(), 200, "b1"), "T1's groups landed")
	x.check(bound, []chunks.HeadSeriesRef{refA, refB}, refC, refE)

	// 2. A failed repair: the pass stops at its first failed write.
	x.faults.setFailAll(true)
	require.False(t, x.pass())
	x.check(bound, []chunks.HeadSeriesRef{refA, refB}, refC, refE)

	// 3. T2's native record is retained, so C is pending.
	_, err = x.commit(lc, 200, "c2")
	require.ErrorIs(t, err, errNativeMetadataFault)
	x.check(bound, []chunks.HeadSeriesRef{refA, refB, refC}, refE)
	// Metadata-free records fill the page, until a flush fails before one.
	padding := strings.Repeat("x", 1024)
	for i := 0; x.faults.failedAt(wlog.FlushBeforeRecord) == 0; i++ {
		_, err := x.commit(labels.FromStrings(labels.MetricName, "filler", "id", strconv.Itoa(i), "padding", padding), 200, "")
		require.ErrorIs(t, err, errNativeMetadataFault)
	}
	// T3's native record, too large for the page's remainder, then fails
	// before any of it is copied, and adds nothing.
	before := x.faults.failedAt(wlog.FlushBeforeRecord)
	_, err = x.commit(le, 200, strings.Repeat("e", 4<<10))
	require.ErrorIs(t, err, errNativeMetadataFault)
	require.Equal(t, before+1, x.faults.failedAt(wlog.FlushBeforeRecord))
	x.check(bound, []chunks.HeadSeriesRef{refA, refB, refC}, refE)

	// 4. An incomplete pass: in batches of one, writes recover for A's batch
	// only. Batches are taken in no fixed order, so passes repeat until A's
	// batch comes first; each fails at its first other batch.
	x.h.nativeMetadataRepairs.selected = make([]*memSeries, 0, 1)
	failedBatches := 0
	x.hooks.repairBeforeWrite = func(refs []chunks.HeadSeriesRef) {
		fail := !slices.Equal(refs, []chunks.HeadSeriesRef{refA})
		if fail {
			failedBatches++
		}
		x.faults.setFailAll(fail)
	}
	for passes := 0; slices.Contains(pendingNativeMetadataRepairs(x.h), refA); passes++ {
		require.Less(t, passes, 100)
		require.False(t, x.pass())
		x.check(bound, pendingNativeMetadataRepairs(x.h), refE)
	}
	x.hooks.repairBeforeWrite = nil
	x.faults.setFailAll(true)
	x.check(bound, []chunks.HeadSeriesRef{refB, refC}, refA, refE)

	// 5. GC while pending: B, a failed first commit without samples, is pinned,
	// so both GC paths keep it.
	x.h.gc()
	require.Empty(t, x.h.gcSeries([]storage.SeriesRef{storage.SeriesRef(refB)}, math.MaxInt64, func(*memSeries) bool { return true }))
	x.check(bound, []chunks.HeadSeriesRef{refB, refC}, refA, refE)

	// 6. Recovery: the pass completes, and releases every pin.
	x.faults.setFailAll(false)
	require.True(t, x.pass())
	x.check(bound, nil, refA, refB, refC, refE)
	requireNativeMetadataConverged(t, x.db, nil)

	// B's last WAL entry is its empty override, and the next GC retires it.
	entries := x.entriesFor(refB)
	last := entries[len(entries)-1]
	require.Equal(t, record.NativeMetadataOverride, last.Kind)
	require.Empty(t, last.Points)
	require.False(t, last.Truncated)
	seriesB := x.h.series.getByID(refB)
	x.h.gc()
	require.Nil(t, x.h.series.getByID(refB))
	// No metadata record for B lands after its retirement.
	x.mustCommit(la, 300, "a3")
	require.Equal(t, entries, x.entriesFor(refB))
	requireNativeMetadataConverged(t, x.db, map[chunks.HeadSeriesRef]nativemetadata.State{refB: terminalNativeMetadata(t, seriesB)})

	m := x.h.metrics
	require.Equal(t, 3.0, prom_testutil.ToFloat64(m.nativeMetadataRepairRegistrations.WithLabelValues(nativeMetadataRepairFailedCommit)))
	// Step 2's batch held A and B; step 4's failed batches held one each.
	require.Equal(t, float64(2+failedBatches), prom_testutil.ToFloat64(m.nativeMetadataRepairRegistrations.WithLabelValues(nativeMetadataRepairFailedRepair)))
	require.Equal(t, 3.0, prom_testutil.ToFloat64(m.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideRepair)))
	require.Equal(t, 3.0, prom_testutil.ToFloat64(m.nativeMetadataRepairsDischarged))
}

// failCommit commits one sample with metadata whose native record is retained
// by a failed final flush, so the series becomes pending.
func (x *repairHarness) failCommit(lset labels.Labels, ts int64, help string) chunks.HeadSeriesRef {
	x.t.Helper()
	x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
	failed := x.faults.failedRecords(record.NativeMetadataCompact)
	ref, err := x.commit(lset, ts, help)
	require.ErrorIs(x.t, err, errNativeMetadataFault)
	require.Zero(x.t, x.faults.armed())
	require.Equal(x.t, failed+1, x.faults.failedRecords(record.NativeMetadataCompact), "the retained record is compact")
	return ref
}

func TestNativeMetricMetadataRepair_Discharge(t *testing.T) {
	la := labels.FromStrings(labels.MetricName, "a")
	writes := func(x *repairHarness) float64 {
		return prom_testutil.ToFloat64(x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideRepair))
	}
	discharges := func(x *repairHarness) float64 {
		return prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDischarged)
	}

	t.Run("a failed transaction's group landing after the repair", func(t *testing.T) {
		// T's native record is retained after the worker's override lands, so
		// it lands after that override. T's registration changes the epoch.
		t.Run("registration before the post-write lock", func(t *testing.T) {
			// The worker keeps the entry, and its next batch writes the
			// override again, after T's record.
			x := newRepairHarness(t)
			ref := x.mustCommit(la, 100, "a")
			x.failCommit(la, 200, "phantom 1")
			x.hooks.repairAfterWrite = func(err error) {
				require.NoError(t, err)
				x.hooks.repairAfterWrite = nil
				x.failCommit(la, 300, "phantom 2")
			}
			require.True(t, x.pass())
			require.Equal(t, 2.0, writes(x))
			require.Equal(t, 1.0, discharges(x))
			x.check(1, nil, ref)
			requireNativeMetadataConverged(t, x.db, nil)
		})
		t.Run("registration after the discharge", func(t *testing.T) {
			// The series re-enters the set, with a new pin.
			x := newRepairHarness(t)
			ref := x.mustCommit(la, 100, "a")
			x.failCommit(la, 200, "phantom 1")
			require.True(t, x.pass())
			x.check(1, nil, ref)
			x.failCommit(la, 300, "phantom 2")
			x.check(1, []chunks.HeadSeriesRef{ref})
			require.True(t, x.pass())
			require.Equal(t, 2.0, writes(x))
			require.Equal(t, 2.0, discharges(x))
			x.check(1, nil, ref)
			requireNativeMetadataConverged(t, x.db, nil)
		})
	})

	t.Run("a history that changes and changes back", func(t *testing.T) {
		// Between the worker's copy and its post-write lock, two merges replace
		// the history's only point and then restore it. The history compares
		// equal, but the epoch has moved, so the worker keeps the entry and
		// writes again.
		x := newRepairHarness(t)
		ref := x.mustCommit(la, 100, "a")
		x.failCommit(la, 200, "phantom")
		before := nativeMetadataByRef(x.h)[ref]
		x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) {
			x.hooks.repairBeforeWrite = nil
			x.mustCommit(la, 100, "b")
			x.mustCommit(la, 100, "a")
			require.Equal(t, nativeMetadataVersions(before), nativeMetadataVersions(nativeMetadataByRef(x.h)[ref]))
		}
		require.True(t, x.pass())
		require.Equal(t, 2.0, writes(x))
		require.Equal(t, 1.0, discharges(x))
		x.check(1, nil, ref)
		requireNativeMetadataConverged(t, x.db, nil)
	})
}

func (x *repairHarness) hasChunks(ref chunks.HeadSeriesRef) bool {
	s := x.h.series.getByID(ref)
	require.NotNil(x.t, s)
	s.Lock()
	defer s.Unlock()
	return s.headChunks != nil || len(s.mmappedChunks) > 0
}

func (x *repairHarness) gcSeries(ref chunks.HeadSeriesRef) bool {
	deleted := x.h.gcSeries([]storage.SeriesRef{storage.SeriesRef(ref)}, math.MaxInt64, func(*memSeries) bool { return true })
	_, ok := deleted[storage.SeriesRef(ref)]
	return ok
}

func TestNativeMetricMetadataRepair_Retirement(t *testing.T) {
	t.Run("GC between a repair's write and its lock", func(t *testing.T) {
		// A failed first commit leaves a series without samples, which GC
		// retires once nothing pins it.
		x := newRepairHarness(t)
		ref := x.failCommit(labels.FromStrings(labels.MetricName, "a"), 100, "phantom")
		x.hooks.repairBeforeLock = func(got chunks.HeadSeriesRef) {
			require.Equal(t, ref, got)
			x.h.gc()
			require.False(t, x.gcSeries(ref))
		}
		require.True(t, x.pass())
		x.hooks.repairBeforeLock = nil
		x.check(1, nil, ref)

		// Once discharged, the next GC retires the series, and no metadata
		// record for it lands afterwards.
		entries := x.entriesFor(ref)
		require.NotEmpty(t, entries)
		x.h.gc()
		require.Nil(t, x.h.series.getByID(ref))
		x.mustCommit(labels.FromStrings(labels.MetricName, "b"), 200, "b")
		require.Equal(t, entries, x.entriesFor(ref))
	})

	t.Run("GC between a batch's write and its locks", func(t *testing.T) {
		// One batch holds a failed first commit without samples, which
		// truncation's GC would retire, and a series with chunks, which the
		// targeted GC would. Both are pinned until their discharge.
		x := newRepairHarness(t)
		empty := x.failCommit(labels.FromStrings(labels.MetricName, "empty"), 100, "phantom")
		lc := labels.FromStrings(labels.MetricName, "chunks")
		withChunks := x.mustCommit(lc, 100, "a")
		x.failCommit(lc, 200, "phantom")
		x.hooks.repairAfterWrite = inlineOnce(func(err error) {
			require.NoError(t, err)
			x.h.gc()
			require.False(t, x.gcSeries(withChunks))
			require.NotNil(t, x.h.series.getByID(empty))
		})
		require.True(t, x.pass())
		x.check(2, nil, empty, withChunks)
		x.h.gc()
		require.Nil(t, x.h.series.getByID(empty))
		require.True(t, x.gcSeries(withChunks))
	})

	t.Run("registration before release", func(t *testing.T) {
		// Rollback releases sample reservations before it logs. By then the
		// series must already be pinned: only the pin keeps the targeted GC,
		// which retires a series with chunks, from retiring it.
		x := newRepairHarness(t)
		la := labels.FromStrings(labels.MetricName, "a")
		ref := x.mustCommit(la, 100, "a")
		ran := false
		x.hooks.rollbackAfterRelease = func() {
			ran = true
			require.Equal(t, 1, x.reservations(ref))
			require.False(t, x.gcSeries(ref))
			require.NoError(t, x.h.truncateMemory(1000))
			require.NotNil(t, x.h.series.getByID(ref))
		}
		x.failCommit(la, 200, "phantom")
		require.True(t, ran)
		x.hooks.rollbackAfterRelease = nil
		x.check(1, []chunks.HeadSeriesRef{ref})
		require.True(t, x.pass())
		x.check(1, nil, ref)
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("both GC paths' chunk behaviour", func(t *testing.T) {
		x := newRepairHarness(t)
		truncated := x.mustCommit(labels.FromStrings(labels.MetricName, "truncated"), 100, "a")
		// Its samples are above the truncation point below.
		targeted := x.mustCommit(labels.FromStrings(labels.MetricName, "targeted"), 2000, "a")
		x.failCommit(labels.FromStrings(labels.MetricName, "truncated"), 200, "phantom")
		x.failCommit(labels.FromStrings(labels.MetricName, "targeted"), 2100, "phantom")
		x.check(2, []chunks.HeadSeriesRef{truncated, targeted})

		// Truncation's GC truncates a pinned series' chunks, and keeps it.
		require.True(t, x.hasChunks(truncated))
		x.h.series.gc(1000, 0)
		require.False(t, x.hasChunks(truncated))
		// The targeted GC checks reservations first, so a pinned series keeps
		// its chunks.
		require.True(t, x.hasChunks(targeted))
		require.False(t, x.gcSeries(targeted))
		require.True(t, x.hasChunks(targeted))
		x.check(2, []chunks.HeadSeriesRef{truncated, targeted})

		require.True(t, x.pass())
		x.check(2, nil, truncated, targeted)
		// After discharge, each waits for an eligible GC: truncation's once no
		// chunk remains above its truncation point, the targeted one once it is
		// selected again.
		x.h.gc()
		require.Nil(t, x.h.series.getByID(truncated))
		require.NotNil(t, x.h.series.getByID(targeted))
		require.True(t, x.gcSeries(targeted))
		require.Nil(t, x.h.series.getByID(targeted))
	})

	t.Run("stale-series truncation of a pinned series", func(t *testing.T) {
		x := newRepairHarness(t)
		ls := labels.FromStrings(labels.MetricName, "stale")
		ref := x.mustCommit(ls, 100, "a")
		app := x.db.AppenderV2(t.Context())
		_, err := app.Append(storage.SeriesRef(ref), ls, 0, 200, math.Float64frombits(value.StaleNaN), nil, nil, repairMetadata("a"))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		x.failCommit(ls, 300, "phantom")

		samples := func() int {
			q, err := x.db.Querier(0, math.MaxInt64)
			require.NoError(t, err)
			defer q.Close()
			n := 0
			ss := q.Select(t.Context(), false, nil, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "stale"))
			for ss.Next() {
				it := ss.At().Iterator(nil)
				for it.Next() != chunkenc.ValNone {
					n++
				}
				require.NoError(t, it.Err())
			}
			require.NoError(t, ss.Err())
			return n
		}

		// The block holds the series, but the pinned series stays in the Head.
		require.NoError(t, x.db.CompactStaleHead())
		require.Len(t, x.db.Blocks(), 1)
		require.NotNil(t, x.h.series.getByID(ref))
		require.True(t, x.hasChunks(ref))
		require.Equal(t, 2, samples())
		x.check(1, []chunks.HeadSeriesRef{ref})

		// After discharge, the next stale-series truncation retires it.
		require.True(t, x.pass())
		x.check(1, nil, ref)
		require.NoError(t, x.db.CompactStaleHead())
		require.Nil(t, x.h.series.getByID(ref))
		require.Equal(t, 2, samples())
	})

	t.Run("a pending series not yet in the ref index", func(t *testing.T) {
		// A creator publishes a series by labels before it adds it to the ref
		// index (setUnlessAlreadySet). Meanwhile another transaction can find
		// it by labels, fail its commit and register it. The series is live
		// and pinned; the pass must neither count a violation nor drop the
		// repair, and the pin must go with the discharge.
		x := newRepairHarness(t)
		la := labels.FromStrings(labels.MetricName, "a")
		ref := x.mustCommit(la, 100, "a")
		s := x.h.series.getByID(ref)
		stripe := x.h.series.refStripe(ref)
		// The creator's window: in the hash index, not yet in the ref index.
		x.h.series.locks[stripe].Lock()
		delete(x.h.series.series[stripe], ref)
		x.h.series.locks[stripe].Unlock()
		require.Equal(t, ref, x.failCommit(la, 200, "phantom"))
		require.Equal(t, []chunks.HeadSeriesRef{ref}, pendingNativeMetadataRepairs(x.h))
		require.True(t, x.pass())
		// The creator completes its publication.
		x.h.series.locks[stripe].Lock()
		x.h.series.series[stripe][ref] = s
		x.h.series.locks[stripe].Unlock()
		require.Zero(t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairViolations), "a live series isn't a violation")
		require.Equal(t, 0, x.reservations(ref), "the pin goes with the discharge")
		requireNativeMetadataConverged(t, x.db, nil)
		x.check(1, nil, ref)
	})

	t.Run("a violated pin is asserted", func(t *testing.T) {
		// A fault retires a pinned series behind the pin's back. The next
		// pass counts an invariant violation, and forgets the repair.
		x := newRepairHarness(t)
		ref := x.failCommit(labels.FromStrings(labels.MetricName, "a"), 100, "phantom")
		s := x.h.series.getByID(ref)
		s.Lock()
		s.setGCed()
		s.Unlock()
		require.True(t, x.pass())
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairViolations))
		require.Empty(t, pendingNativeMetadataRepairs(x.h))
		require.Zero(t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsPending))
	})
}

func TestNativeMetricMetadataRepair_Pins(t *testing.T) {
	la := labels.FromStrings(labels.MetricName, "a")

	t.Run("repeated registration", func(t *testing.T) {
		// Two failed transactions and a failed repair register the series;
		// each registration changes the epoch, and the series holds one pin.
		x := newRepairHarness(t)
		ref := x.mustCommit(la, 100, "a")
		epoch := x.epoch(ref)
		x.failCommit(la, 200, "phantom 1")
		require.NotEqual(t, epoch, x.epoch(ref))
		epoch = x.epoch(ref)
		x.check(1, []chunks.HeadSeriesRef{ref})
		x.failCommit(la, 300, "phantom 2")
		require.NotEqual(t, epoch, x.epoch(ref))
		epoch = x.epoch(ref)
		x.check(1, []chunks.HeadSeriesRef{ref})
		x.faults.setFailAll(true)
		require.False(t, x.pass())
		require.NotEqual(t, epoch, x.epoch(ref))
		x.check(1, []chunks.HeadSeriesRef{ref})
		x.faults.setFailAll(false)
		require.True(t, x.pass())
		x.check(1, nil, ref)
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("pins while a transaction holds reservations", func(t *testing.T) {
		// The expected count is the open transaction's queued samples, plus
		// the series it created, plus one pin while the series is pending.
		x := newRepairHarness(t)
		ref := x.mustCommit(la, 100, "a")
		x.failCommit(la, 200, "phantom")
		app := x.db.AppenderV2(t.Context())
		for ts := int64(300); ts < 303; ts++ {
			_, err := app.Append(storage.SeriesRef(ref), la, 0, ts, 1, nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		fresh, err := app.Append(0, labels.FromStrings(labels.MetricName, "fresh"), 0, 300, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.Equal(t, 3+1, x.reservations(ref))
		// One sample, and the series' creation.
		require.Equal(t, 1+1, x.reservations(chunks.HeadSeriesRef(fresh)))
		require.True(t, x.pass())
		require.Equal(t, 3, x.reservations(ref))
		require.NoError(t, app.Commit())
		x.check(1, nil, ref, chunks.HeadSeriesRef(fresh))
	})

	t.Run("shutdown and reset cleanup", func(t *testing.T) {
		for _, c := range []struct {
			name   string
			worker bool
			fail   bool
		}{
			{name: "final pass by the worker", worker: true},
			{name: "final pass without a worker"},
			{name: "dropped by the worker", worker: true, fail: true},
			{name: "dropped without a worker", fail: true},
		} {
			t.Run(c.name, func(t *testing.T) {
				x := newRepairHarness(t)
				x.hooks.manualRepairs = !c.worker
				r := x.h.nativeMetadataRepairs
				r.minBackoff, r.maxBackoff = time.Hour, time.Hour
				refA := x.mustCommit(la, 100, "a")
				refB := x.mustCommit(labels.FromStrings(labels.MetricName, "b"), 100, "b")
				if c.fail {
					// Every repair write fails; the WAL itself closes.
					x.faults.setFailRecords(nativeMetadataRecords)
				}
				// One failed transaction registers both, before the worker
				// starts and could take an armed fault.
				if !c.fail {
					x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
				}
				app := x.db.AppenderV2(t.Context())
				for _, lset := range []labels.Labels{la, labels.FromStrings(labels.MetricName, "b")} {
					_, err := app.Append(0, lset, 0, 200, 1, nil, nil, repairMetadata("phantom"))
					require.NoError(t, err)
				}
				require.ErrorIs(t, app.Commit(), errNativeMetadataFault)
				require.Zero(t, x.faults.armed())
				require.Positive(t, x.faults.failedRecords(record.NativeMetadataCompact))
				if c.worker {
					// The worker has started; with failures it backs off.
					require.Eventually(t, func() bool {
						return prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairRegistrations.WithLabelValues(nativeMetadataRepairFailedRepair)) > 0 || len(pendingNativeMetadataRepairs(x.h)) == 0
					}, 10*time.Second, time.Millisecond)
				}
				writes := 0
				x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) { writes++ }
				want := nativeMetadataByRef(x.h)
				require.NoError(t, x.db.Close())
				select {
				case <-r.done:
					require.True(t, c.worker)
				default:
					require.False(t, c.worker, "the worker must have exited")
				}
				// Every pin is released, the set is empty, and later
				// registrations are dropped.
				require.Equal(t, 0, x.reservations(refA))
				require.Equal(t, 0, x.reservations(refB))
				require.Empty(t, pendingNativeMetadataRepairs(x.h))
				require.Zero(t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsPending))
				dropped := prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped)
				if c.fail {
					// The final pass stops at its first failed write: the
					// commit's record failed, and at least that write did.
					require.Equal(t, 1, writes)
					require.Equal(t, 2.0, dropped)
					require.GreaterOrEqual(t, x.faults.failedRecords(record.NativeMetadataCompact), 2)
					return
				}
				require.Zero(t, dropped)
				// The final pass wrote both repairs before the WAL closed.
				states := readNativeMetadataWAL(t, filepath.Join(x.db.Dir(), "wal")).reduce()
				for ref, state := range want {
					require.Equal(t, nativeMetadataVersions(state), nativeMetadataVersions(states[ref]))
				}
			})
		}

		t.Run("closing during a pass of several batches", func(t *testing.T) {
			// Close stops the worker while its ordinary pass is about to write
			// its first batch. The worker goes on with that pass, admitting
			// writes until the deadline Close started, makes the final pass
			// under the same deadline and cleans up, all before Close returns.
			const n = 2*nativeMetadataRepairBatch + 10
			for _, c := range []struct {
				name string
				// failFrom is the first write that fails, if any.
				failFrom int
				// writes are the sizes of the batches written, in order.
				writes  []int
				dropped int
			}{
				{name: "the pass completes", writes: []int{256, 256, 10}},
				{
					// The ordinary pass stops at its failed second write, and
					// the final pass at its first.
					name: "writes fail during the pass", failFrom: 2,
					writes: []int{256, 256, 256}, dropped: n - 256,
				},
			} {
				t.Run(c.name, func(t *testing.T) {
					x := newRepairHarness(t)
					r := x.h.nativeMetadataRepairs
					r.minBackoff, r.maxBackoff = time.Hour, time.Hour
					refs := burstNativeMetadataRepairs(x, n)
					want := nativeMetadataByRef(x.h)
					failedBefore := x.faults.failedRecords(record.NativeMetadataCompact)

					var (
						mtx      sync.Mutex
						batches  [][]chunks.HeadSeriesRef
						landed   []bool
						stopSeen bool
					)
					started := make(chan struct{})
					x.hooks.repairBeforeWrite = func(batch []chunks.HeadSeriesRef) {
						mtx.Lock()
						batches = append(batches, batch)
						first := len(batches) == 1
						fail := c.failFrom > 0 && len(batches) >= c.failFrom
						mtx.Unlock()
						if first {
							close(started)
							select {
							case <-r.stop:
								mtx.Lock()
								stopSeen = true
								mtx.Unlock()
							case <-time.After(10 * time.Second):
								t.Error("Close didn't stop the worker")
							}
						}
						if fail {
							// Only metadata writes fail, so the WAL itself closes.
							x.faults.setFailRecords(nativeMetadataRecords)
						}
					}
					x.hooks.repairAfterWrite = func(err error) {
						mtx.Lock()
						landed = append(landed, err == nil)
						mtx.Unlock()
					}
					startNativeMetadataRepairs(x)
					<-started
					require.NoError(t, x.db.Close())

					// Close returned once the worker had exited, and the
					// worker saw the stop during its ordinary pass.
					select {
					case <-r.done:
					default:
						t.Fatal("the worker must have exited")
					}
					mtx.Lock()
					defer mtx.Unlock()
					require.True(t, stopSeen)
					var sizes []int
					for _, batch := range batches {
						sizes = append(sizes, len(batch))
					}
					require.Equal(t, c.writes, sizes)
					// Every pin is released, and the set is empty.
					for _, ref := range refs {
						require.Equal(t, 0, x.reservations(ref), "ref %d", ref)
					}
					require.Empty(t, pendingNativeMetadataRepairs(x.h))
					require.Zero(t, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsPending))
					require.Equal(t, float64(c.dropped), prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))
					// The landed batches' series converge. A dropped series' override
					// may land too: its failed write kept the record in the page
					// buffer, and the WAL's close flushed it.
					states := readNativeMetadataWAL(t, filepath.Join(x.db.Dir(), "wal")).reduce()
					repaired := map[chunks.HeadSeriesRef]bool{}
					for i, ok := range landed {
						for _, ref := range batches[i] {
							repaired[ref] = repaired[ref] || ok
						}
					}
					dropped := 0
					for _, ref := range refs {
						if repaired[ref] {
							requireNativeMetadataEqual(t, want[ref], states[ref], "ref %d", ref)
							continue
						}
						dropped++
					}
					require.Equal(t, c.dropped, dropped)
					if c.failFrom > 0 {
						require.Greater(t, x.faults.failedRecords(record.NativeMetadataCompact), failedBefore, "compact repair writes failed")
					}
				})
			}
		})

		t.Run("a write in flight at the final pass's deadline", func(t *testing.T) {
			// The deadline only stops shutdown from admitting another write. A
			// write admitted before it completes, however long it takes, and
			// Close waits for it.
			const (
				n        = 2*nativeMetadataRepairBatch + 10
				timeout  = 50 * time.Millisecond
				slowness = 4 * timeout
			)
			x := newRepairHarness(t)
			r := x.h.nativeMetadataRepairs
			r.minBackoff, r.maxBackoff = time.Hour, time.Hour
			r.closeTimeout = timeout
			var slow atomic.Bool
			x.h.wal.SetFlushFault(func(rec []byte, at wlog.FlushPoint) error {
				if slow.Load() && at != wlog.FlushWithinRecord {
					time.Sleep(slowness)
				}
				return x.faults.fault(rec, at)
			})
			refs := burstNativeMetadataRepairs(x, n)
			var mtx sync.Mutex
			var batches [][]chunks.HeadSeriesRef
			var errs []error
			x.hooks.repairBeforeWrite = func(batch []chunks.HeadSeriesRef) {
				mtx.Lock()
				defer mtx.Unlock()
				batches = append(batches, batch)
				// The ordinary pass's first write fails; the final pass's
				// first write succeeds, slowly.
				x.faults.setFailAll(len(batches) == 1)
				slow.Store(len(batches) == 2)
			}
			x.hooks.repairAfterWrite = func(err error) {
				mtx.Lock()
				defer mtx.Unlock()
				errs = append(errs, err)
				slow.Store(false)
			}
			startNativeMetadataRepairs(x)
			require.Eventually(t, func() bool {
				mtx.Lock()
				defer mtx.Unlock()
				return len(errs) == 1
			}, 10*time.Second, time.Millisecond)
			start := time.Now()
			require.NoError(t, x.db.Close())
			require.GreaterOrEqual(t, time.Since(start), slowness, "Close waits for the write in flight")

			mtx.Lock()
			defer mtx.Unlock()
			require.Len(t, errs, 2, "no write is admitted after the deadline")
			require.ErrorIs(t, errs[0], errNativeMetadataFault)
			require.NoError(t, errs[1])
			require.Equal(t, float64(n-nativeMetadataRepairBatch), prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))
			for _, ref := range refs {
				require.Equal(t, 0, x.reservations(ref), "ref %d", ref)
			}
			require.Empty(t, pendingNativeMetadataRepairs(x.h))
		})

		t.Run("preparation crossing the deadline", func(t *testing.T) {
			// A write is admitted after its batch is prepared, immediately
			// before the write, and only before the deadline that Close
			// starts. A batch whose preparation crosses the deadline is never
			// written, in the ordinary pass or in the final one.
			const (
				n       = 2*nativeMetadataRepairBatch + 10
				timeout = 50 * time.Millisecond
			)
			for _, c := range []struct {
				name string
				// failFirst fails the ordinary pass's first write, before
				// Close, so that the crossing preparation is the final pass's.
				failFirst bool
				admitted  int
			}{
				{name: "in the ordinary pass", admitted: 0},
				{name: "in the final pass", failFirst: true, admitted: 1},
			} {
				t.Run(c.name, func(t *testing.T) {
					x := newRepairHarness(t)
					r := x.h.nativeMetadataRepairs
					r.minBackoff, r.maxBackoff = time.Hour, time.Hour
					r.closeTimeout = timeout
					refs := burstNativeMetadataRepairs(x, n)
					failedBefore := x.faults.failedRecords(record.NativeMetadataCompact)
					var mtx sync.Mutex
					prepared := 0
					var admitted []error
					crossing := make(chan struct{})
					crossingAt := 1
					if c.failFirst {
						crossingAt = 2
					}
					x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) {
						mtx.Lock()
						prepared++
						k := prepared
						mtx.Unlock()
						if c.failFirst && k == 1 {
							x.faults.setFailRecords(nativeMetadataRecords)
							return
						}
						x.faults.setFailRecords(nil)
						if k == crossingAt {
							close(crossing)
							select {
							case <-r.stop:
							case <-time.After(10 * time.Second):
								t.Error("Close didn't stop the worker")
							}
							// The preparation crosses the deadline.
							time.Sleep(2 * timeout)
						}
					}
					x.hooks.repairAfterWrite = func(err error) {
						mtx.Lock()
						defer mtx.Unlock()
						admitted = append(admitted, err)
					}
					startNativeMetadataRepairs(x)
					if c.failFirst {
						require.Eventually(t, func() bool {
							mtx.Lock()
							defer mtx.Unlock()
							return len(admitted) == 1
						}, 10*time.Second, time.Millisecond)
					} else {
						<-crossing
					}
					require.NoError(t, x.db.Close())

					mtx.Lock()
					defer mtx.Unlock()
					require.Len(t, admitted, c.admitted, "no write is admitted after the deadline")
					if c.failFirst {
						require.ErrorIs(t, admitted[0], errNativeMetadataFault)
						require.Greater(t, x.faults.failedRecords(record.NativeMetadataCompact), failedBefore, "the first, compact, repair write failed")
					}
					require.Equal(t, float64(n), prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))
					for _, ref := range refs {
						require.Equal(t, 0, x.reservations(ref), "ref %d", ref)
					}
					require.Empty(t, pendingNativeMetadataRepairs(x.h))
				})
			}
		})

		t.Run("one deadline for the ordinary and the final pass", func(t *testing.T) {
			// Close's deadline covers the rest of an ordinary pass and the
			// final pass together. A write admitted before it may finish after
			// it, but no write is admitted afterwards, and the final pass gets
			// no budget of its own.
			const (
				n        = 2*nativeMetadataRepairBatch + 10
				timeout  = 50 * time.Millisecond
				slowness = 4 * timeout
			)
			x := newRepairHarness(t)
			r := x.h.nativeMetadataRepairs
			r.minBackoff, r.maxBackoff = time.Hour, time.Hour
			r.closeTimeout = timeout
			var slow atomic.Bool
			x.h.wal.SetFlushFault(func(rec []byte, at wlog.FlushPoint) error {
				if slow.Load() && at != wlog.FlushWithinRecord {
					time.Sleep(slowness)
				}
				return x.faults.fault(rec, at)
			})
			refs := burstNativeMetadataRepairs(x, n)
			var mtx sync.Mutex
			prepared := 0
			var admitted []error
			started := make(chan struct{})
			x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) {
				mtx.Lock()
				prepared++
				first := prepared == 1
				mtx.Unlock()
				if first {
					close(started)
					select {
					case <-r.stop:
					case <-time.After(10 * time.Second):
						t.Error("Close didn't stop the worker")
					}
					// Admitted before the deadline, the write finishes after it.
					slow.Store(true)
				}
			}
			x.hooks.repairAfterWrite = func(err error) {
				slow.Store(false)
				mtx.Lock()
				defer mtx.Unlock()
				admitted = append(admitted, err)
			}
			startNativeMetadataRepairs(x)
			<-started
			start := time.Now()
			require.NoError(t, x.db.Close())
			require.GreaterOrEqual(t, time.Since(start), slowness, "Close waits for the admitted write")

			mtx.Lock()
			defer mtx.Unlock()
			require.Len(t, admitted, 1, "no write is admitted after the deadline")
			require.NoError(t, admitted[0])
			require.Equal(t, float64(n-nativeMetadataRepairBatch), prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairsDropped))
			for _, ref := range refs {
				require.Equal(t, 0, x.reservations(ref), "ref %d", ref)
			}
			require.Empty(t, pendingNativeMetadataRepairs(x.h))
		})

		t.Run("a Head reset", func(t *testing.T) {
			x := newRepairHarness(t)
			x.failCommit(la, 100, "phantom")
			r := x.h.nativeMetadataRepairs
			before := reflect.ValueOf(r.pending).UnsafePointer()
			require.NoError(t, x.h.resetInMemoryState())
			require.Empty(t, pendingNativeMetadataRepairs(x.h))
			require.NotEqual(t, before, reflect.ValueOf(r.pending).UnsafePointer())
		})
	})
}

// burstNativeMetadataRepairs registers repairs for n new series, in one failed
// transaction.
func burstNativeMetadataRepairs(x *repairHarness, n int) []chunks.HeadSeriesRef {
	x.t.Helper()
	x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
	app := x.db.AppenderV2(x.t.Context())
	refs := make([]chunks.HeadSeriesRef, n)
	for i := range refs {
		ref, err := app.Append(0, labels.FromStrings(labels.MetricName, "burst", "id", strconv.Itoa(i)), 0, 100, 1, nil, nil, repairMetadata("phantom"))
		require.NoError(x.t, err)
		refs[i] = chunks.HeadSeriesRef(ref)
	}
	require.ErrorIs(x.t, app.Commit(), errNativeMetadataFault)
	require.Zero(x.t, x.faults.armed())
	require.Positive(x.t, x.faults.failedRecords(record.NativeMetadataCompact), "the retained record is compact")
	require.Len(x.t, pendingNativeMetadataRepairs(x.h), n)
	return refs
}

// startNativeMetadataRepairs starts the worker of a harness whose repairs
// were registered by hand, as the first registration would have.
func startNativeMetadataRepairs(x *repairHarness) {
	r := x.h.nativeMetadataRepairs
	r.mtx.Lock()
	require.False(x.t, r.started)
	r.started = true
	r.mtx.Unlock()
	go x.h.runNativeMetadataRepairs()
	r.wake <- struct{}{}
}

func TestNativeMetricMetadataRepair_RetainedCapacity(t *testing.T) {
	burst := burstNativeMetadataRepairs
	mapPointer := func(x *repairHarness) unsafe.Pointer {
		r := x.h.nativeMetadataRepairs
		r.mtx.Lock()
		defer r.mtx.Unlock()
		return reflect.ValueOf(r.pending).UnsafePointer()
	}

	t.Run("the set's map after full discharge", func(t *testing.T) {
		x := newRepairHarness(t)
		burst(x, 1000)
		before := mapPointer(x)
		require.True(t, x.pass())
		require.NotEqual(t, before, mapPointer(x), "an emptied map is replaced")
	})

	t.Run("the set's map while one stays pending", func(t *testing.T) {
		// A map is replaced only once it empties, so one pending entry keeps
		// the storage of the whole burst.
		x := newRepairHarness(t)
		refs := burst(x, 1000)
		before := mapPointer(x)
		kept := refs[len(refs)/2]
		x.hooks.repairBeforeWrite = func(batch []chunks.HeadSeriesRef) {
			x.faults.setFailAll(slices.Contains(batch, kept))
		}
		x.h.nativeMetadataRepairs.selected = make([]*memSeries, 0, 1)
		for passes := 0; len(pendingNativeMetadataRepairs(x.h)) > 1; passes++ {
			require.Less(t, passes, 10000)
			x.pass()
		}
		require.Equal(t, []chunks.HeadSeriesRef{kept}, pendingNativeMetadataRepairs(x.h))
		require.Equal(t, before, mapPointer(x), "a non-empty map isn't replaced")
		x.faults.setFailAll(false)
		x.hooks.repairBeforeWrite = nil
		require.True(t, x.pass())
		require.NotEqual(t, before, mapPointer(x))
	})

	t.Run("the batch scratch", func(t *testing.T) {
		// Between batches, the scratch keeps its fixed capacities and no
		// copied value.
		x := newRepairHarness(t)
		burst(x, 3*nativeMetadataRepairBatch+1)
		r := x.h.nativeMetadataRepairs
		require.True(t, x.pass())
		require.Equal(t, nativeMetadataRepairBatch, cap(r.selected))
		require.Equal(t, nativeMetadataRepairBatch, cap(r.batch))
		require.Equal(t, nativeMetadataRepairBatch, cap(r.record.entries))
		require.Equal(t, nativeMetadataRepairBatch*maxNativeMetricMetadataVersions, cap(r.record.points))
		require.Equal(t, nativeMetadataRepairBatch*maxNativeMetricMetadataVersions, cap(r.record.values))
		require.Empty(t, r.record.indices)
		for _, repair := range r.batch[:cap(r.batch)] {
			require.Zero(t, repair)
		}
		for _, entry := range r.record.entries[:cap(r.record.entries)] {
			require.Zero(t, entry)
		}
		for _, value := range r.record.values[:cap(r.record.values)] {
			require.Zero(t, value)
		}
	})

	t.Run("an oversized entry", func(t *testing.T) {
		// An entry beyond the batch's byte target is written in a batch of
		// its own.
		x := newRepairHarness(t)
		for version := range maxNativeMetricMetadataVersions {
			x.mustCommit(labels.FromStrings(labels.MetricName, "huge"), int64(100*(version+1)), longNativeMetadataHelp(version))
		}
		x.failCommit(labels.FromStrings(labels.MetricName, "huge"), 1000, "phantom")
		x.failCommit(labels.FromStrings(labels.MetricName, "small"), 100, "phantom")
		var batches [][]chunks.HeadSeriesRef
		x.hooks.repairBeforeWrite = func(refs []chunks.HeadSeriesRef) { batches = append(batches, refs) }
		require.True(t, x.pass())
		require.Len(t, batches, 2)
		require.Len(t, batches[0], 1)
		require.Len(t, batches[1], 1)
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("the encoding buffer", func(t *testing.T) {
		x := newRepairHarness(t)
		r := x.h.nativeMetadataRepairs
		for version := range maxNativeMetricMetadataVersions {
			x.mustCommit(labels.FromStrings(labels.MetricName, "huge"), int64(100*(version+1)), longNativeMetadataHelp(version))
		}
		ref := x.failCommit(labels.FromStrings(labels.MetricName, "huge"), 1000, "phantom")
		require.True(t, x.pass())
		require.Zero(t, cap(r.buf), "a buffer grown past the limit isn't kept")
		x.check(1, nil, ref)
		x.failCommit(labels.FromStrings(labels.MetricName, "small"), 100, "phantom")
		require.True(t, x.pass())
		require.NotZero(t, cap(r.buf))
		require.LessOrEqual(t, cap(r.buf), nativeMetadataRepairKeepBytes)
	})
}

// TestNativeMetricMetadataRepair_OverrideRecord checks the dictionary of the
// override records that Commit's override loop, the repair worker and startup
// handoffs write: one definition per value pointer, so equal values that native
// histories share are defined once, and equal values under different pointers
// once each.
func TestNativeMetricMetadataRepair_OverrideRecord(t *testing.T) {
	a := &metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: "a"}
	b := &metadata.Metadata{Type: model.MetricTypeGauge, Help: "b"}
	equalA := &metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: "a"}
	var r nativeMetadataOverrideRecord
	r.add(1, false, false, []nativeMetricMetadataPoint{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 200, Metadata: b}, {EffectiveFrom: 300, Metadata: a}})
	r.add(2, true, true, []nativeMetricMetadataPoint{{EffectiveFrom: 50, Metadata: b}, {EffectiveFrom: 60, Metadata: equalA}})
	r.add(3, false, false, nil)
	require.Equal(t, 3, r.len())
	rec := r.encode(nil)
	var dec record.Decoder
	var decoded record.CompactNativeMetadata
	require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
	value := func(m *metadata.Metadata) record.NativeMetadataValue {
		return record.NativeMetadataValue{Type: record.GetMetricType(m.Type), Unit: m.Unit, Help: m.Help}
	}
	require.Equal(t, []record.NativeMetadataValue{value(a), value(b), value(equalA)}, decoded.Values)
	require.Equal(t, []record.RefCompactNativeMetadata{
		{Ref: 1, Kind: record.NativeMetadataOverride, Points: []record.CompactNativeMetadataPoint{{EffectiveFrom: 100, Value: 0}, {EffectiveFrom: 200, Value: 1}, {EffectiveFrom: 300, Value: 0}}},
		{Ref: 2, Kind: record.NativeMetadataOverride, Truncated: true, Handoff: true, Points: []record.CompactNativeMetadataPoint{{EffectiveFrom: 50, Value: 1}, {EffectiveFrom: 60, Value: 2}}},
		{Ref: 3, Kind: record.NativeMetadataOverride},
	}, normalizeCompactNativeMetadataForTest(decoded.Entries))
	// A reset keeps no value.
	r.reset()
	require.Zero(t, r.len())
	require.Empty(t, r.values)
	require.Empty(t, r.indices)
	require.Equal(t, []record.NativeMetadataValue{value(b)}, func() []record.NativeMetadataValue {
		r.add(4, false, false, []nativeMetricMetadataPoint{{EffectiveFrom: 1, Metadata: b}})
		require.NoError(t, dec.CompactNativeMetadata(r.encode(nil), &decoded))
		return decoded.Values
	}())
}

// normalizeCompactNativeMetadataForTest clears capacity differences, and
// empty point lists, for comparisons.
func normalizeCompactNativeMetadataForTest(entries []record.RefCompactNativeMetadata) []record.RefCompactNativeMetadata {
	out := make([]record.RefCompactNativeMetadata, 0, len(entries))
	for _, e := range entries {
		e.Points = slices.Clip(e.Points)
		if len(e.Points) == 0 {
			e.Points = nil
		}
		out = append(out, e)
	}
	return out
}

// TestNativeMetricMetadataRepair_RecordSizes checks the batch policy of the
// writers that bound their records, the repair worker and startup handoffs,
// for compact records: at most nativeMetadataRepairBatch entries, and at most
// nativeMetadataRepairBatchBytes by nativeMetadataEntrySizeEstimate, unless one
// entry alone exceeds it. A record must be no larger than its entries'
// estimates together, framing included.
func TestNativeMetricMetadataRepair_RecordSizes(t *testing.T) {
	t.Run("the estimate's allowances", func(t *testing.T) {
		// The grammar's largest encodings, whatever the values: a start
		// delta is a uvarint of up to 64 bits, and a string's length one of
		// up to this platform's largest int.
		uvarint := func(x uint64) int { return len(binary.AppendUvarint(nil, x)) }
		definition := uvarint(math.MaxUint64) + 1 + 1 + 2*uvarint(math.MaxInt)
		reference := uvarint(math.MaxUint64) + uvarint(math.MaxUint32)
		// An entry's kind byte, ref delta and point count, of at most five.
		entry := 1 + uvarint(math.MaxUint64) + uvarint(maxNativeMetricMetadataVersions)
		// A record's type, format, base and entry count, below 2^14.
		framing := 1 + 1 + uvarint(math.MaxUint64) + uvarint(1<<14-1)
		require.Equal(t, 14, framing)
		// The estimate allows 32 bytes per entry, and per point besides its
		// strings, which definitions take once and references not at all.
		empty := nativeMetadataEntrySizeEstimate(nil)
		perPoint := nativeMetadataEntrySizeEstimate([]nativeMetricMetadataPoint{{Metadata: &metadata.Metadata{}}}) - empty
		require.LessOrEqual(t, max(definition, reference), perPoint)
		// For n entries with p points in all, a record takes at most
		// framing + n*entry + p*definition bytes besides strings, and the
		// estimates n*empty + p*perPoint: 6 bytes or more to spare.
		require.GreaterOrEqual(t, (empty-entry)-framing, 6)
	})

	// Long values, distinct by series and version, make each history's
	// estimate several hundred bytes.
	value := func(id, version int) string {
		return fmt.Sprintf("%d.%d.%s", id, version, strings.Repeat("v", 100+id%50))
	}

	t.Run("repairs", func(t *testing.T) {
		const n = 3*nativeMetadataRepairBatch + 7
		x := newRepairHarness(t)
		lsets := make([]labels.Labels, n)
		for id := range lsets {
			lsets[id] = labels.FromStrings(labels.MetricName, "series", "id", strconv.Itoa(id))
			for version := range maxNativeMetricMetadataVersions {
				help := value(id, version)
				if id == n/2 {
					help = longNativeMetadataHelp(version)
				}
				x.mustCommit(lsets[id], int64(100*(version+1)), help)
			}
		}
		// Counted first: the failed record can span pages, and stay torn
		// until a later flush completes it.
		before := len(readWALRecords(t, x.h.wal.Dir()))
		// One failed transaction makes every series pending.
		x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
		app := x.db.AppenderV2(t.Context())
		for _, lset := range lsets {
			_, err := app.Append(0, lset, 0, 1000, 1, nil, nil, repairMetadata("phantom"))
			require.NoError(t, err)
		}
		require.ErrorIs(t, app.Commit(), errNativeMetadataFault)
		require.Zero(t, x.faults.armed())
		require.Len(t, pendingNativeMetadataRepairs(x.h), n)
		require.Positive(t, requireOverridesOfHistory(t, x, func() { require.True(t, x.pass()) }))
		// The failed commit's retained groups land with the first repair.
		var repairs [][]byte
		for _, rec := range readWALRecords(t, x.h.wal.Dir())[before:] {
			if entries := decodeNativeMetadataRecordForTest(t, rec); entries[0].Kind == record.NativeMetadataOverride {
				repairs = append(repairs, rec)
			}
		}
		written := requireBoundedOverrideRecords(t, repairs, false)
		require.Len(t, written, n)
		for ref, times := range written {
			require.Equal(t, 1, times, "ref %d", ref)
		}
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("startup handoffs", func(t *testing.T) {
		const n = 2*nativeMetadataRepairBatch + 9
		point := func(from int64, help string) nativeMetadataPointForTest {
			return nativeMetadataPointForTest{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}
		}
		var recs []any
		for id := range n {
			lset := labels.FromStrings(labels.MetricName, "series", "id", strconv.Itoa(id))
			first, second := chunks.HeadSeriesRef(id+1), chunks.HeadSeriesRef(n+id+1)
			group := nativeMetadataEntryForTest{Ref: second, Kind: record.NativeMetadataGroup}
			for version := range maxNativeMetricMetadataVersions {
				help := value(id, version)
				if id == n/2 {
					help = longNativeMetadataHelp(version)
				}
				group.Points = append(group.Points, point(int64(100*(version+1)), help))
			}
			recs = append(recs,
				[]record.RefSeries{{Ref: first, Labels: lset}},
				[]record.RefSeries{{Ref: second, Labels: lset}}, []nativeMetadataEntryForTest{group},
				[]record.RefSample{{Ref: second, T: 1000, V: 1}})
		}
		dir := t.TempDir()
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		populateTestWL(t, w, recs, nil, false)
		require.NoError(t, w.Close())
		before := len(readWALRecords(t, filepath.Join(dir, "wal")))
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		db := newTestDB(t, withDir(dir), withOpts(opts))
		landed := readWALRecords(t, db.head.wal.Dir())[before:]
		written := requireBoundedOverrideRecords(t, landed, true)
		require.Len(t, written, n)
		for ref, times := range written {
			require.Equal(t, 1, times, "ref %d", ref)
		}
		// Each handoff holds its survivor's seeded history, which nothing
		// has changed since.
		for _, rec := range landed {
			for _, e := range decodeNativeMetadataRecordForTest(t, rec) {
				s := db.head.series.getByID(e.Ref)
				require.NotNil(t, s)
				require.Equal(t, normalizeNativeMetadataEntries([]nativeMetadataEntryForTest{nativeMetadataEntryOfSeries(s, true)}), normalizeNativeMetadataEntries([]nativeMetadataEntryForTest{e}))
			}
		}
	})
}

// requireBoundedOverrideRecords checks the records a batching writer wrote,
// all compact records of overrides, handoffs if handoff is set: each has the
// size its grammar gives, no more than its entries' estimates together less 6,
// at most nativeMetadataRepairBatch entries, and at most
// nativeMetadataRepairBatchBytes unless it holds one entry, which must then
// exceed it. It returns how often each ref was written.
func requireBoundedOverrideRecords(t *testing.T, recs [][]byte, handoff bool) map[chunks.HeadSeriesRef]int {
	t.Helper()
	uvarint := func(x uint64) int { return len(binary.AppendUvarint(nil, x)) }
	varint := func(x int64) int { return len(binary.AppendVarint(nil, x)) }
	var dec record.Decoder
	var compact record.CompactNativeMetadata
	written := map[chunks.HeadSeriesRef]int{}
	oversized := 0
	for i, rec := range recs {
		require.Equal(t, record.NativeMetadataCompact, dec.Type(rec), "record %d", i)
		require.NoError(t, dec.CompactNativeMetadata(rec, &compact))
		require.NotEmpty(t, compact.Entries)
		require.LessOrEqual(t, len(compact.Entries), nativeMetadataRepairBatch, "record %d", i)
		var base int64
		havePoints := false
		for _, e := range compact.Entries {
			for _, p := range e.Points {
				if !havePoints || p.EffectiveFrom < base {
					base, havePoints = p.EffectiveFrom, true
				}
			}
		}
		size := 1 + 1 + varint(base) + uvarint(uint64(len(compact.Entries)))
		estimates := 0
		defined := uint32(0)
		var previous chunks.HeadSeriesRef
		for _, e := range compact.Entries {
			require.Equal(t, record.NativeMetadataOverride, e.Kind)
			require.Equal(t, handoff, e.Handoff)
			written[e.Ref]++
			size += 1 + varint(int64(e.Ref-previous)) + uvarint(uint64(len(e.Points)))
			previous = e.Ref
			estimate := 32
			for _, p := range e.Points {
				v := compact.Values[p.Value]
				estimate += 32 + len(v.Unit) + len(v.Help)
				size += uvarint(uint64(p.EffectiveFrom) - uint64(base))
				if p.Value < defined {
					size += uvarint(uint64(p.Value) + 1)
					continue
				}
				size += 1 + 1 + uvarint(uint64(len(v.Unit))) + len(v.Unit) + uvarint(uint64(len(v.Help))) + len(v.Help)
				defined++
			}
			estimates += estimate
		}
		require.Len(t, rec, size, "record %d has its grammar's size", i)
		require.LessOrEqual(t, len(rec), estimates-6, "record %d", i)
		if len(compact.Entries) == 1 && estimates > nativeMetadataRepairBatchBytes {
			oversized++
			continue
		}
		require.LessOrEqual(t, estimates, nativeMetadataRepairBatchBytes, "record %d", i)
		require.LessOrEqual(t, len(rec), nativeMetadataRepairBatchBytes, "record %d", i)
	}
	require.Equal(t, 1, oversized, "the oversized entry is written alone")
	return written
}

// nativeMetadataEntryOfSeries returns the override the pin's rules write for s:
// its native history and truncation flag, as read under its lock.
func nativeMetadataEntryOfSeries(s *memSeries, handoff bool) nativeMetadataEntryForTest {
	e := nativeMetadataEntryForTest{Ref: s.ref, Kind: record.NativeMetadataOverride, Handoff: handoff}
	s.Lock()
	defer s.Unlock()
	if native := s.nativeMetadataLocked(); native != nil {
		for _, p := range native.AppendPoints(nil) {
			e.Points = append(e.Points, nativeMetadataPointForTest{EffectiveFrom: p.EffectiveFrom, Type: record.GetMetricType(p.Metadata.Type), Unit: p.Metadata.Unit, Help: p.Metadata.Help})
		}
		e.Truncated = native.truncated
	}
	return e
}

// requireOverridesOfHistory runs f, and checks every override record that
// Commit's override loop or the repair worker writes meanwhile: its decoded
// entries must equal the entries the pin's rules give for the histories the
// writer read. Those are still the series' states when the writer's
// before-write hook starts, which runs before any hook f installed. It
// returns how many records it checked.
func requireOverridesOfHistory(t *testing.T, x *repairHarness, f func()) int {
	t.Helper()
	var (
		mtx      sync.Mutex
		expected [][]nativeMetadataEntryForTest
		last     []byte
		checked  int
	)
	snapshot := func(refs []chunks.HeadSeriesRef) {
		var entries []nativeMetadataEntryForTest
		for _, ref := range refs {
			s := x.h.series.getByID(ref)
			require.NotNil(t, s, "ref %d is live", ref)
			entries = append(entries, nativeMetadataEntryOfSeries(s, false))
		}
		mtx.Lock()
		expected = append(expected, entries)
		mtx.Unlock()
	}
	override, repair := x.hooks.overrideBeforeWrite, x.hooks.repairBeforeWrite
	x.hooks.overrideBeforeWrite = func(refs []chunks.HeadSeriesRef) {
		snapshot(refs)
		if override != nil {
			override(refs)
		}
	}
	x.hooks.repairBeforeWrite = func(refs []chunks.HeadSeriesRef) {
		snapshot(refs)
		if repair != nil {
			repair(refs)
		}
	}
	x.h.wal.SetFlushFault(func(rec []byte, at wlog.FlushPoint) error {
		if at == wlog.FlushAfterRecord && len(rec) > 0 && record.Type(rec[0]) == record.NativeMetadataCompact && !bytes.Equal(rec, last) {
			if entries := decodeNativeMetadataRecordForTest(t, rec); entries[0].Kind == record.NativeMetadataOverride {
				mtx.Lock()
				require.NotEmpty(t, expected, "an override record without a before-write hook")
				require.Equal(t, normalizeNativeMetadataEntries(expected[0]), normalizeNativeMetadataEntries(entries))
				expected = expected[1:]
				checked++
				mtx.Unlock()
				last = slices.Clone(rec)
			}
		}
		return x.faults.fault(rec, at)
	})
	f()
	x.hooks.overrideBeforeWrite, x.hooks.repairBeforeWrite = override, repair
	x.h.wal.SetFlushFault(x.faults.fault)
	mtx.Lock()
	defer mtx.Unlock()
	require.Empty(t, expected, "every prepared write was checked")
	return checked
}

// walRecordsWithPoint returns the types of the WAL's native metadata records,
// in order, that hold an entry with a point of help at from.
func walRecordsWithPoint(t *testing.T, dir string, from int64, help string) []record.Type {
	t.Helper()
	var dec record.Decoder
	var out []record.Type
	for _, rec := range readWALRecords(t, dir) {
		if typ := dec.Type(rec); typ == record.Metadata || typ == record.NativeMetadataCompact {
			for _, e := range decodeNativeMetadataRecordForTest(t, rec) {
				if slices.ContainsFunc(e.Points, func(p nativeMetadataPointForTest) bool { return p.EffectiveFrom == from && p.Help == help }) {
					out = append(out, typ)
					break
				}
			}
		}
	}
	return out
}
