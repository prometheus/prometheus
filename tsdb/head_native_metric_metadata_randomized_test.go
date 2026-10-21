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
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// randomFailure classifies an injected failure by the record whose flush
// failed, its position, and what the record's transaction logged before it.
type randomFailure int

const (
	randomFailureOther randomFailure = iota
	// randomFailureRetained fails a native metadata record's final flush,
	// which keeps the whole record in the page buffer.
	randomFailureRetained
	// randomFailureNotCopied fails a native metadata record's leading flush,
	// before any of it is copied.
	randomFailureNotCopied
	// randomFailureLanded fails a Samples record after its transaction's
	// merge groups landed.
	randomFailureLanded
	// randomFailureBeforeMetadata fails a Series record, which a
	// transaction logs before any metadata.
	randomFailureBeforeMetadata
	randomFailures
)

var randomFailureNames = [randomFailures]string{"other", "retained metadata", "metadata not copied", "landed metadata", "before metadata"}

// randomWALFaults fails WAL flushes at random, at a rate that changes over
// time, but never between two fragments of a record, which would tear it.
type randomWALFaults struct {
	mtx  sync.Mutex
	rng  *rand.Rand
	rate float64
	// failed counts injected failures. A plain field under mtx: a 64-bit
	// atomic here would be misaligned on 32-bit platforms.
	failed int
	// classes counts the failures by class, and nativeTypes the failed
	// native metadata records by type.
	classes     [randomFailures]int
	nativeTypes map[record.Type]int
	// groupsLanded holds the goroutines whose transaction's merge-group
	// record landed, until their transaction's Samples record is flushed.
	groupsLanded map[uint64]bool
}

func (f *randomWALFaults) setRate(rate float64) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.rate = rate
}

func (f *randomWALFaults) fault(rec []byte, at wlog.FlushPoint) error {
	checkNoLockHeld("a WAL write")
	f.mtx.Lock()
	defer f.mtx.Unlock()
	fail := at != wlog.FlushWithinRecord && f.rate != 0 && f.rng.Float64() < f.rate
	if len(rec) > 0 {
		f.classifyLocked(rec, at, fail)
	}
	if !fail {
		return nil
	}
	f.failed++
	return errNativeMetadataFault
}

// classifyLocked tracks what each goroutine's transaction logged, and
// classifies the flush of rec at at if it fails. A transaction logs its
// Series records, then its merge groups, then its samples, from the
// goroutine that commits it.
func (f *randomWALFaults) classifyLocked(rec []byte, at wlog.FlushPoint, fail bool) {
	if f.groupsLanded == nil {
		f.groupsLanded, f.nativeTypes = map[uint64]bool{}, map[record.Type]int{}
	}
	switch typ := record.Type(rec[0]); typ {
	case record.Series:
		if fail {
			f.classes[randomFailureBeforeMetadata]++
		}
	case record.Samples, record.SamplesV2:
		id := faultGoroutineID()
		if fail {
			if f.groupsLanded[id] {
				f.classes[randomFailureLanded]++
			} else {
				f.classes[randomFailureOther]++
			}
		}
		if fail || at == wlog.FlushAfterRecord {
			delete(f.groupsLanded, id)
		}
	case record.Metadata, record.NativeMetadataCompact:
		switch {
		case fail && at == wlog.FlushAfterRecord:
			f.nativeTypes[typ]++
			f.classes[randomFailureRetained]++
		case fail:
			f.nativeTypes[typ]++
			f.classes[randomFailureNotCopied]++
		case at == wlog.FlushAfterRecord && nativeMetadataGroupsRecord(rec):
			f.groupsLanded[faultGoroutineID()] = true
		}
	default:
		if fail {
			f.classes[randomFailureOther]++
		}
	}
}

// nativeMetadataGroupsRecord reports whether rec is a record of merge groups,
// as Commit logs before its samples.
func nativeMetadataGroupsRecord(rec []byte) bool {
	var dec record.Decoder
	if dec.Type(rec) != record.NativeMetadataCompact {
		return false
	}
	var compact record.CompactNativeMetadata
	if dec.CompactNativeMetadata(rec, &compact) != nil {
		return false
	}
	for _, e := range compact.Entries {
		if e.Kind != record.NativeMetadataGroup {
			return false
		}
	}
	return len(compact.Entries) > 0
}

// faultGoroutineID returns the calling goroutine's ID.
func faultGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// The trace starts "goroutine <id> [".
	fields := bytes.Fields(buf[:n])
	id, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("parsing goroutine ID from %q: %v", buf[:n], err))
	}
	return id
}

func (f *randomWALFaults) failures() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.failed
}

// randomizedCoverage accumulates what randomized rounds exercised.
type randomizedCoverage struct {
	handoffs      float64
	failures      [randomFailures]int
	nativeTypes   map[record.Type]int
	registrations map[string]float64
	// Overrides written by Commit's override loop and by the repair worker.
	mergeOrderOverrides, repairOverrides float64
}

// add adds a round's faults and its Head's repair registrations.
func (c *randomizedCoverage) add(faults *randomWALFaults, m *headMetrics) {
	faults.mtx.Lock()
	defer faults.mtx.Unlock()
	if c.nativeTypes == nil {
		c.nativeTypes, c.registrations = map[record.Type]int{}, map[string]float64{}
	}
	for i, n := range faults.classes {
		c.failures[i] += n
	}
	for typ, n := range faults.nativeTypes {
		c.nativeTypes[typ] += n
	}
	for _, cause := range nativeMetadataRepairCauses {
		c.registrations[cause] += prom_testutil.ToFloat64(m.nativeMetadataRepairRegistrations.WithLabelValues(cause))
	}
	c.mergeOrderOverrides += prom_testutil.ToFloat64(m.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder))
	c.repairOverrides += prom_testutil.ToFloat64(m.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideRepair))
}

func (c *randomizedCoverage) String() string {
	out := fmt.Sprintf("%.0f startup handoffs, %.0f merge-order and %.0f repair overrides; failures:", c.handoffs, c.mergeOrderOverrides, c.repairOverrides)
	for i, n := range c.failures {
		out += fmt.Sprintf(" %s %d,", randomFailureNames[i], n)
	}
	out += fmt.Sprintf(" native metadata records by type %v; registrations by cause %v", c.nativeTypes, c.registrations)
	return out
}

var nativeMetadataRepairCauses = []string{nativeMetadataRepairFailedCommit, nativeMetadataRepairFailedOverride, nativeMetadataRepairUnsettledOverride, nativeMetadataRepairFailedRepair}

// TestNativeMetricMetadataRepair_Randomized runs concurrent writers with
// out-of-order and equal timestamps, equal values and rollbacks, against random
// WAL failures, some of whose records land later, and both GC paths. Commit's
// override loop and the repair worker write the same series. The invariants are
// checked throughout. Once writes stop failing and repairs settle, every
// series' history reduced from the WAL, as senders, checkpoints and replay
// reduce it, must equal native's, with no exception: retired series' at
// their retirement.
//
// Rounds of this are separated by restarts, whose GC and recreation churn
// remaps refs. A clean restart closes the settled DB, and must restore every
// series' history exactly. A crash-like one freezes a copy of the DB's files
// once the round's writers, GC and fault changes stop, with outstanding
// repairs and retained records as the round left them, and opens the copy:
// it must restore the reduction of the copy's WAL.
func TestNativeMetricMetadataRepair_Randomized(t *testing.T) {
	seeds := 24
	if testing.Short() {
		seeds = 4
	}
	var coverage randomizedCoverage
	for seed := range seeds {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			runRandomizedNativeMetadataRepairs(t, uint64(seed), &coverage)
		})
	}
	t.Logf("coverage: %v", &coverage)
	// Every writer's compact records occur: no native record is of another
	// type, as each round checks.
	require.Positive(t, coverage.handoffs, "restarts remap refs")
	require.Positive(t, coverage.mergeOrderOverrides, "Commit overrides")
	require.Positive(t, coverage.repairOverrides, "the worker repairs")
	// Metadata-targeting failures fail compact records only.
	require.Zero(t, coverage.nativeTypes[record.Metadata])
	require.Positive(t, coverage.nativeTypes[record.NativeMetadataCompact])
	// The seeds exercise every class of failure, and every repair cause but
	// an exhausted override loop, which needs three overtaken writes in a
	// row; a merge-order test covers it.
	for i, n := range coverage.failures[randomFailureRetained:] {
		require.Positive(t, n, "failures: %s", randomFailureNames[randomFailureRetained+randomFailure(i)])
	}
	for _, cause := range nativeMetadataRepairCauses {
		if cause != nativeMetadataRepairUnsettledOverride {
			require.Positive(t, coverage.registrations[cause], "registrations: %s", cause)
		}
	}
}

// registrations sums repair registrations over their causes.
func registrations(m *headMetrics) float64 {
	var sum float64
	for _, cause := range nativeMetadataRepairCauses {
		sum += prom_testutil.ToFloat64(m.nativeMetadataRepairRegistrations.WithLabelValues(cause))
	}
	return sum
}

// runRandomizedNativeMetadataRepairs adds what its rounds exercised to
// coverage.
func runRandomizedNativeMetadataRepairs(t *testing.T, seed uint64, coverage *randomizedCoverage) {
	const rounds = 3
	// Registered first, the check runs after the DBs' teardown.
	resetLockOrderViolations()
	t.Cleanup(func() {
		requireNoLockOrderViolations(t)
		logLockOrderChecks(t)
	})
	opts := DefaultOptions()
	opts.EnableNativeMetadata = true
	opts.EnableNativeMetadataPersistence = true
	opts.OutOfOrderTimeWindow = math.MaxInt32
	// Small segments rotate often, so that some flushes precede records.
	opts.WALSegmentSize = 32 << 10
	db := newTestDB(t, withOpts(opts))
	rng := rand.New(rand.NewPCG(seed, 200))
	crashes := 0
	handoffs := 0.0
	for round := range rounds {
		crash := round < rounds-1 && rng.IntN(2) == 0
		if crash {
			crashes++
		}
		db = runRandomizedNativeMetadataRound(t, seed, uint64(round), db, opts, crash, coverage)
		handoffs += prom_testutil.ToFloat64(db.head.metrics.nativeMetadataHandoffsWritten.WithLabelValues(nativeMetadataHandoffStartup))
	}
	t.Logf("seed %d: %d crash-like and %d clean restarts, %.0f startup handoffs", seed, crashes, rounds-crashes, handoffs)
	coverage.handoffs += handoffs
}

// runRandomizedNativeMetadataRound runs one round against db, then restarts
// it, and returns the restarted DB.
func runRandomizedNativeMetadataRound(t *testing.T, seed, round uint64, db *DB, opts *Options, crash bool, coverage *randomizedCoverage) *DB {
	const (
		writers         = 4
		transactions    = 50
		seriesCount     = 6
		timestampsRange = 300
	)
	db.DisableCompactions()
	h := db.Head()
	// retired holds the terminal history of every ref that no round's write
	// can change: those not live as the round starts, and those GC retires.
	retired := nonLiveNativeMetadata(t, db)
	r := h.nativeMetadataRepairs
	r.minBackoff, r.maxBackoff = 100*time.Microsecond, 2*time.Millisecond
	faults := &randomWALFaults{rng: rand.New(rand.NewPCG(seed, 1+round*1000))}
	h.wal.SetFlushFault(faults.fault)
	// A freeze waits for repair writes in flight and keeps new ones out.
	var freeze sync.RWMutex
	h.nativeMetadataTestHooks = &nativeMetadataTestHooks{
		repairBeforeWrite: func([]chunks.HeadSeriesRef) { freeze.RLock() },
		repairAfterWrite:  func(error) { freeze.RUnlock() },
	}

	lsets := make([]labels.Labels, seriesCount)
	for i := range lsets {
		lsets[i] = labels.FromStrings(labels.MetricName, "series", "id", strconv.Itoa(i))
	}
	// One help is long, so that some native metadata records, small as
	// compact records are, are the ones that start a new segment, whose
	// leading flush then fails before any of the record is copied.
	helps := []string{"a", "b", "c", strings.Repeat("d", 4<<10)}

	// seen holds every series live as the round starts, and every series a
	// writer appended to, taken while its transaction reserved it, so no
	// series can leave Head unrecorded.
	// gced holds each series' history retired in this round, captured once a
	// GC has retired it. missed counts the retirements that a snapshot of live
	// series, taken before each GC, would have missed.
	var seenMtx sync.Mutex
	seen := map[chunks.HeadSeriesRef]*memSeries{}
	gced := map[chunks.HeadSeriesRef]nativemetadata.State{}
	missed := 0
	captureRetired := func(before map[chunks.HeadSeriesRef]struct{}) {
		seenMtx.Lock()
		defer seenMtx.Unlock()
		for ref, s := range seen {
			if _, ok := gced[ref]; ok {
				continue
			}
			s.Lock()
			isGCed := s.isGCed()
			s.Unlock()
			if !isGCed {
				continue
			}
			gced[ref] = terminalNativeMetadata(t, s)
			if _, ok := before[ref]; !ok {
				missed++
			}
		}
	}
	liveSeries := func() []*memSeries {
		var out []*memSeries
		for i := range h.series.series {
			h.series.locks[i].RLock()
			for _, s := range h.series.series[i] {
				out = append(out, s)
			}
			h.series.locks[i].RUnlock()
		}
		return out
	}
	// checkInvariants checks the repair invariants as they hold at any
	// moment: every pending series is unretired and pinned, and at most one
	// per label set is pending.
	// A restart can leave series no writer appends to, which GC may retire.
	for _, s := range liveSeries() {
		seen[s.ref] = s
	}
	var violations atomic.Int64
	checkInvariants := func() {
		r.mtx.Lock()
		pending := make([]*memSeries, 0, len(r.pending))
		for _, s := range r.pending {
			pending = append(pending, s)
		}
		r.mtx.Unlock()
		if len(pending) > seriesCount {
			violations.Inc()
			t.Errorf("seed %d: %d pending repairs exceed the bound of %d live label sets", seed, len(pending), seriesCount)
		}
		for _, s := range pending {
			s.Lock()
			r.mtx.Lock()
			still := r.pending[s.ref] == s
			r.mtx.Unlock()
			if still && (s.isGCed() || s.pendingCommitCount() == 0) {
				violations.Inc()
				t.Errorf("seed %d: pending series %d is retired or unpinned", seed, s.ref)
			}
			s.Unlock()
		}
	}

	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			rng := rand.New(rand.NewPCG(seed, round*1000+uint64(w)+2))
			for range transactions {
				app := db.AppenderV2(t.Context())
				for range 1 + rng.IntN(3) {
					lset := lsets[rng.IntN(seriesCount)]
					for range 1 + rng.IntN(3) {
						// Equal samples at equal timestamps are accepted.
						ref, err := app.Append(0, lset, 0, int64(rng.IntN(timestampsRange)), 1, nil, nil, repairMetadata(helps[rng.IntN(len(helps))]))
						if err != nil {
							continue
						}
						// The open transaction reserves the series, so it is live.
						// Its creator may not have added it to the ref index yet,
						// so it is looked up by labels.
						s := h.series.getByHash(lset.Hash(), lset)
						if s == nil || s.ref != chunks.HeadSeriesRef(ref) {
							t.Errorf("seed %d: appended series %d isn't in Head", seed, ref)
							continue
						}
						seenMtx.Lock()
						seen[s.ref] = s
						seenMtx.Unlock()
					}
				}
				if rng.IntN(10) == 0 {
					_ = app.Rollback()
				} else {
					_ = app.Commit()
				}
				// Occasional idle gaps let GC find series nothing reserves;
				// back-to-back commits race on the same series.
				if rng.IntN(2) == 0 {
					time.Sleep(time.Duration(rng.IntN(500)) * time.Microsecond)
				}
			}
		})
	}
	done := make(chan struct{})
	var background sync.WaitGroup
	background.Go(func() {
		// Failure episodes of random rates and lengths.
		rng := rand.New(rand.NewPCG(seed, round*1000+100))
		rates := []float64{0, 0, 0.1, 0.5, 1}
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Duration(rng.IntN(3000)) * time.Microsecond):
			}
			faults.setRate(rates[rng.IntN(len(rates))])
		}
	})
	background.Go(func() {
		// Both GC paths, throughout. Truncation's GC retires series without
		// chunks, the targeted GC series with them.
		rng := rand.New(rand.NewPCG(seed, round*1000+101))
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Duration(rng.IntN(2000)) * time.Microsecond):
			}
			series := liveSeries()
			before := make(map[chunks.HeadSeriesRef]struct{}, len(series))
			for _, s := range series {
				before[s.ref] = struct{}{}
			}
			if rng.IntN(2) == 0 {
				h.gc()
			} else {
				// About half the series; reserved and pinned ones are kept.
				var refs []storage.SeriesRef
				for _, s := range series {
					if rng.IntN(2) == 0 {
						refs = append(refs, storage.SeriesRef(s.ref))
					}
				}
				slices.Sort(refs)
				h.gcSeries(refs, math.MaxInt64, func(*memSeries) bool { return true })
			}
			captureRetired(before)
		}
	})
	background.Go(func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(200 * time.Microsecond):
			}
			checkInvariants()
		}
	})
	wg.Wait()
	close(done)
	background.Wait()

	// Retire about half the series, unless pinned, and recreate them, so that
	// the restart remaps refs whatever GC managed during the round.
	churn := rand.New(rand.NewPCG(seed, round*1000+102))
	series := liveSeries()
	live := make(map[chunks.HeadSeriesRef]struct{}, len(series))
	var churned []storage.SeriesRef
	for _, s := range series {
		live[s.ref] = struct{}{}
		if churn.IntN(2) == 0 {
			churned = append(churned, storage.SeriesRef(s.ref))
		}
	}
	slices.Sort(churned)
	h.gcSeries(churned, math.MaxInt64, func(*memSeries) bool { return true })
	captureRetired(live)
	app := db.AppenderV2(t.Context())
	for _, lset := range lsets {
		if _, err := app.Append(0, lset, 0, int64(churn.IntN(timestampsRange)), 1, nil, nil, repairMetadata(helps[churn.IntN(len(helps))])); err == nil {
			s := h.series.getByHash(lset.Hash(), lset)
			seenMtx.Lock()
			seen[s.ref] = s
			seenMtx.Unlock()
		}
	}
	_ = app.Commit()

	if crash {
		// The round's work has stopped; repairs may be outstanding, and
		// failed flushes may have left records in the page buffer.
		freeze.Lock()
		frozen := freezeRecoveryInput(t, db)
		pending := len(pendingNativeMetadataRepairs(h))
		freeze.Unlock()
		faults.setRate(0)
		settleNativeMetadataRepairs(t, db)
		require.NoError(t, db.Close())
		coverage.add(faults, h.metrics)
		restarted, absent := frozen.open(t, opts)
		t.Logf("seed %d, round %d: crash-like restart with %d repairs outstanding, after %d injected failures; replay dropped %d series without samples after their last Series records", seed, round, pending, faults.failures(), absent)
		return restarted
	}

	// Settle: writes recover, and the worker writes every repair.
	faults.setRate(0)
	settleNativeMetadataRepairs(t, db)
	checkInvariants()
	require.Zero(t, violations.Load())
	m := h.metrics
	require.Zero(t, prom_testutil.ToFloat64(m.nativeMetadataRepairViolations))
	require.Zero(t, prom_testutil.ToFloat64(m.nativeMetadataRepairsDropped))
	require.Zero(t, prom_testutil.ToFloat64(m.pendingCommitUnderflow))
	require.Zero(t, prom_testutil.ToFloat64(m.nativeMetadataRepairsPending))

	// Every series a writer touched is accounted for: live ones converge with
	// native, and retired ones with their history at retirement, which
	// retirement froze. Any other ref with WAL entries fails the check.
	captureRetired(nil)
	seenMtx.Lock()
	for ref, s := range seen {
		if want, ok := gced[ref]; ok {
			requireNativeMetadataEqual(t, want, terminalNativeMetadata(t, s), "seed %d: retired ref %d changed after its retirement", seed, ref)
			continue
		}
		require.Same(t, s, h.series.getByID(ref), "seed %d: ref %d", seed, ref)
	}
	seenMtx.Unlock()
	for ref, state := range gced {
		_, ok := retired[ref]
		require.False(t, ok, "seed %d: ref %d was retired as the round started", seed, ref)
		retired[ref] = state
	}
	requireNativeMetadataConverged(t, db, retired)

	// A checkpoint of every closed segment reduces to the same histories. The
	// rotation must succeed and leave only an empty segment after the
	// checkpoint, so that the check reads the checkpoint's entries alone.
	wal := readNativeMetadataWAL(t, h.wal.Dir())
	// Every writer writes native metadata in compact records.
	require.NotContains(t, wal.types, record.Metadata, "seed %d", seed)
	states := wal.reduce()
	_, err := h.wal.NextSegmentSync()
	require.NoError(t, err)
	first, last, err := wlog.Segments(h.wal.Dir())
	require.NoError(t, err)
	require.Greater(t, last, first, "seed %d: no closed segment to checkpoint", seed)
	_, err = wlog.Checkpoint(promslog.NewNopLogger(), h.wal, first, last-1, func(chunks.HeadSeriesRef) bool { return true }, math.MinInt64, false, true)
	require.NoError(t, err)
	_, index, err := wlog.LastCheckpoint(h.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, last-1, index, "seed %d", seed)
	info, err := os.Stat(wlog.SegmentName(h.wal.Dir(), last))
	require.NoError(t, err)
	require.Zero(t, info.Size(), "seed %d: the segment after the checkpoint holds records", seed)
	checkpointed := readNativeMetadataWAL(t, h.wal.Dir())
	require.NotContains(t, checkpointed.types, record.Metadata, "seed %d", seed)
	reduced := checkpointed.reduce()
	for ref := range reduced {
		_, ok := states[ref]
		require.True(t, ok, "seed %d: the checkpoint holds ref %d, which the segments don't", seed, ref)
	}
	for ref, state := range states {
		requireNativeMetadataEqual(t, state, reduced[ref], "seed %d: checkpointed ref %d", seed, ref)
	}

	// Every pin is released: no transaction is open, so no series holds a
	// reservation, and the next GC retires series without samples.
	for _, s := range liveSeries() {
		s.Lock()
		require.Zero(t, s.pendingCommitCount(), "seed %d: ref %d", seed, s.ref)
		s.Unlock()
	}
	t.Logf("seed %d, round %d: %d injected failures, %.0f registrations, %.0f merge-order and %.0f repair overrides, %d retired series, %d of them created after the snapshot that preceded their GC",
		seed, round, faults.failures(),
		registrations(m),
		prom_testutil.ToFloat64(m.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder)),
		prom_testutil.ToFloat64(m.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideRepair)),
		len(gced), missed)

	// A clean restart restores every live series' history exactly, if its
	// Series record reached the WAL: a failed commit's write and its
	// rollback's can both drop it. Replay also restores retired series whose
	// samples the WAL still holds, with their terminal histories, which
	// G-prefix covers.
	before := nativeMetadataSnapshot(t, db)
	liveRefs := map[string]chunks.HeadSeriesRef{}
	for key, series := range before {
		liveRefs[key] = h.series.getByHash(series.Labels.Hash(), series.Labels).ref
	}
	dir := db.Dir()
	require.NoError(t, db.Close())
	coverage.add(faults, m)
	require.Zero(t, prom_testutil.ToFloat64(m.nativeMetadataRepairsDropped))
	want := readNativeMetadataRecovery(t, filepath.Join(dir, "wal"))
	restarted := newTestDB(t, withDir(dir), withOpts(opts))
	if absent := requireNativeMetadataRecovered(t, restarted.head, want); absent > 0 {
		t.Logf("seed %d, round %d: replay dropped %d series without samples after their last Series records", seed, round, absent)
	}
	after := nativeMetadataSnapshot(t, restarted)
	undeclared := 0
	for key, series := range before {
		if !want.declared[liveRefs[key]] {
			undeclared++
			continue
		}
		require.True(t, labels.Equal(series.Labels, after[key].Labels), "seed %d, round %d: labels for %s", seed, round, key)
		require.Equal(t, series.Versions, after[key].Versions, "seed %d, round %d: G-exact for %s", seed, round, key)
		require.Equal(t, series.Truncated, after[key].Truncated, "seed %d, round %d: truncation for %s", seed, round, key)
	}
	if undeclared > 0 {
		t.Logf("seed %d, round %d: %d live series without a Series record in the WAL", seed, round, undeclared)
	}
	return restarted
}

// nonLiveNativeMetadata returns the WAL history of every ref that isn't live in
// db, such as a retired series or a duplicate a restart remapped.
func nonLiveNativeMetadata(t *testing.T, db *DB) map[chunks.HeadSeriesRef]nativemetadata.State {
	t.Helper()
	out := map[chunks.HeadSeriesRef]nativemetadata.State{}
	for ref, state := range readNativeMetadataWAL(t, db.head.wal.Dir()).reduce() {
		if db.head.series.getByID(ref) == nil {
			out[ref] = state
		}
	}
	return out
}
