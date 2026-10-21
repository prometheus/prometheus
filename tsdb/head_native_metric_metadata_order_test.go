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
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// orderPoint is one sample at ts, with metadata help.
type orderPoint struct {
	ts   int64
	help string
}

// orderPause is where a transaction of a merge-order case waits.
type orderPause int

const (
	orderNoPause orderPause = iota
	// orderAfterLog waits after the transaction's WAL write, before it merges.
	orderAfterLog
	// orderAfterSelect waits after its batch's selection, before its merge.
	orderAfterSelect
)

type orderTxn struct {
	points []orderPoint
	pause  orderPause
	// partner also appends to a series sharing the target's stripe.
	partner bool
}

// mergePointEvent is one merge point that testMergePoint observed.
type mergePointEvent struct {
	site     nativeMetadataMergeSite
	ref      chunks.HeadSeriesRef
	mismatch bool
}

// orderFixture is a target series and, in its stripe, a partner series.
type orderFixture struct {
	x       *repairHarness
	target  labels.Labels
	partner labels.Labels
	ref     chunks.HeadSeriesRef

	mtx    sync.Mutex
	events []mergePointEvent
}

func newOrderFixture(t *testing.T, initial []orderPoint) *orderFixture {
	f := &orderFixture{
		x:       newRepairHarness(t),
		target:  labels.FromStrings(labels.MetricName, "target"),
		partner: labels.FromStrings(labels.MetricName, "partner"),
	}
	app := f.x.db.AppenderV2(t.Context())
	for _, p := range initial {
		ref, err := app.Append(storage.SeriesRef(f.ref), f.target, 0, p.ts, 1, nil, nil, repairMetadata(p.help))
		require.NoError(t, err)
		f.ref = chunks.HeadSeriesRef(ref)
	}
	// Fill the stripes in between, so that the partner shares the target's.
	for i := 1; i < nativeMetricMetadataStripes; i++ {
		_, err := app.Append(0, labels.FromStrings(labels.MetricName, "filler", "id", strconv.Itoa(i)), 0, 1, 1, nil, nil, repairMetadata(""))
		require.NoError(t, err)
	}
	ref, err := app.Append(0, f.partner, 0, 1, 1, nil, nil, repairMetadata(""))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(f.ref)%nativeMetricMetadataStripes, uint64(ref)%nativeMetricMetadataStripes)
	f.x.h.nativeMetricMetadata.testMergePoint = func(site nativeMetadataMergeSite, ref chunks.HeadSeriesRef, mismatch bool) {
		f.mtx.Lock()
		defer f.mtx.Unlock()
		f.events = append(f.events, mergePointEvent{site: site, ref: ref, mismatch: mismatch})
	}
	return f
}

// run starts txns in log order, each once the previous one waits or is done,
// then releases the waiting ones in release order.
func (f *orderFixture) run(t *testing.T, txns []orderTxn, release []int) {
	byFirst := map[int64]int{}
	reached := make([]chan struct{}, len(txns))
	resume := make([]chan struct{}, len(txns))
	once := make([]sync.Once, len(txns))
	for i, txn := range txns {
		byFirst[txn.points[0].ts] = i
		reached[i], resume[i] = make(chan struct{}), make(chan struct{})
	}
	wait := func(pause orderPause) func(*nativeMetricMetadataAppender) {
		return func(a *nativeMetricMetadataAppender) {
			i, ok := byFirst[a.observations[0].effectiveFrom]
			if !ok || txns[i].pause != pause {
				return
			}
			once[i].Do(func() {
				close(reached[i])
				<-resume[i]
			})
		}
	}
	f.x.hooks.commitAfterLog = wait(orderAfterLog)
	f.x.h.nativeMetricMetadata.testAfterSelectBatch = wait(orderAfterSelect)
	defer func() {
		f.x.hooks.commitAfterLog = nil
		f.x.h.nativeMetricMetadata.testAfterSelectBatch = nil
	}()

	done := make([]chan error, len(txns))
	for i, txn := range txns {
		done[i] = make(chan error, 1)
		go func() {
			app := f.x.db.AppenderV2(t.Context())
			for _, p := range txn.points {
				if _, err := app.Append(storage.SeriesRef(f.ref), f.target, 0, p.ts, 1, nil, nil, repairMetadata(p.help)); err != nil {
					done[i] <- err
					return
				}
				if txn.partner {
					if _, err := app.Append(0, f.partner, 0, p.ts, 1, nil, nil, repairMetadata("partner")); err != nil {
						done[i] <- err
						return
					}
				}
			}
			done[i] <- app.Commit()
		}()
		if txn.pause == orderNoPause {
			require.NoError(t, receive(t, done[i], "a commit"))
		} else {
			receive(t, reached[i], "a pause")
		}
	}
	for _, i := range release {
		close(resume[i])
		require.NoError(t, receive(t, done[i], "a released commit"))
	}
}

// mismatchAt reports whether the target's merge point at site found a mismatch.
func (f *orderFixture) mismatchAt(site nativeMetadataMergeSite) bool {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	for _, e := range f.events {
		if e.site == site && e.ref == f.ref && e.mismatch {
			return true
		}
	}
	return false
}

func TestNativeMetricMetadataMergeOrder(t *testing.T) {
	points := func(ts []int64, helps ...string) []orderPoint {
		out := make([]orderPoint, len(ts))
		for i := range ts {
			out[i] = orderPoint{ts: ts[i], help: helps[i]}
		}
		return out
	}
	// A skipped group's site depends on where the skip happens: alone in its
	// stripe, the stripe is skipped; with a partner, selection filters the
	// group; paused after selection, the merge loop skips it.
	skipSites := []struct {
		site    nativeMetadataMergeSite
		pause   orderPause
		partner bool
	}{
		{site: nativeMetadataMergeSiteSelect, pause: orderAfterLog, partner: true},
		{site: nativeMetadataMergeSiteCommit, pause: orderAfterSelect},
	}

	t.Run("a merge that leaves native unchanged", func(t *testing.T) {
		// [A@100, B@200]; T1 = {C@120} logs first, but T2 = {A@150} merges
		// first, coalescing into A@100. Applied in WAL order, A@150 stays.
		f := newOrderFixture(t, points([]int64{100, 200}, "A", "B"))
		f.run(t, []orderTxn{
			{points: points([]int64{120}, "C"), pause: orderAfterLog},
			{points: points([]int64{150}, "A")},
		}, []int{0})
		f.requireOverridden(t)
		require.True(t, f.mismatchAt(nativeMetadataMergeSiteMerge))
	})

	for _, c := range []struct {
		name    string
		initial []orderPoint
		// first logs first, and is skipped as stable after second merges.
		first, second []orderPoint
	}{
		{
			// The full history [a1@10 … a5@50]: V@50 replaces a5@50 without
			// eviction, then V@300 is stable. In WAL order, V@300 evicts a1.
			name:    "an overtaken group at the version cap",
			initial: points([]int64{10, 20, 30, 40, 50}, "a1", "a2", "a3", "a4", "a5"),
			first:   points([]int64{300}, "V"),
			second:  points([]int64{50}, "V"),
		},
		{
			// [A@10 … E@50]: D@50 replaces E@50 and coalesces into D@40, then
			// D@60 is stable. In WAL order, D@60 evicts A.
			name:    "an overtaken group that coalesces",
			initial: points([]int64{10, 20, 30, 40, 50}, "A", "B", "C", "D", "E"),
			first:   points([]int64{60}, "D"),
			second:  points([]int64{50}, "D"),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, s := range skipSites {
				t.Run(string(s.site), func(t *testing.T) {
					f := newOrderFixture(t, c.initial)
					f.run(t, []orderTxn{
						{points: c.first, pause: s.pause, partner: s.partner},
						{points: c.second},
					}, []int{0})
					f.requireOverridden(t)
					require.True(t, f.mismatchAt(s.site))
				})
			}
		})
	}

	t.Run("a group merged without being logged", func(t *testing.T) {
		// T1 observes B@200 on [A@100]. Before it logs, B@150 commits, so
		// its pre-log pass finds B@200 stable and logs nothing. Before it
		// merges, C@175 commits, so B@200 merges after all, and only an
		// override puts it in the WAL.
		x := newRepairHarness(t)
		la := labels.FromStrings(labels.MetricName, "a")
		ref := x.mustCommit(la, 100, "A")
		app := x.db.AppenderV2(t.Context())
		_, err := app.Append(storage.SeriesRef(ref), la, 0, 200, 1, nil, nil, repairMetadata("B"))
		require.NoError(t, err)
		x.mustCommit(la, 150, "B")
		x.hooks.commitAfterLog = inlineOnce(func(a *nativeMetricMetadataAppender) {
			require.Empty(t, a.walEntries)
			x.mustCommit(la, 175, "C")
		})
		var mismatch bool
		x.h.nativeMetricMetadata.testMergePoint = func(site nativeMetadataMergeSite, got chunks.HeadSeriesRef, m bool) {
			mismatch = mismatch || site == nativeMetadataMergeSiteMerge && got == ref && m
		}
		require.NoError(t, app.Commit())
		requireNativeMetadataConverged(t, x.db, nil)
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder)))
		require.True(t, mismatch)
	})

	// In the next two cases, the target, the lower ref, and the partner share
	// a stripe. T observes B@200 on the target's [A@100], but B@150 commits
	// before T logs, so its pre-log pass finds B@200 stable and logs only the
	// partner's group. Each group must be matched with its own ref's entry.
	partnerLoggedOnly := func(t *testing.T, f *orderFixture, partnerHelp string, afterLog func(*nativeMetricMetadataAppender)) chunks.HeadSeriesRef {
		partner := f.x.h.series.getByHash(f.partner.Hash(), f.partner).ref
		app := f.x.db.AppenderV2(t.Context())
		_, err := app.Append(storage.SeriesRef(f.ref), f.target, 0, 200, 1, nil, nil, repairMetadata("B"))
		require.NoError(t, err)
		_, err = app.Append(storage.SeriesRef(partner), f.partner, 0, 200, 1, nil, nil, repairMetadata(partnerHelp))
		require.NoError(t, err)
		f.x.mustCommit(f.target, 150, "B")
		f.mtx.Lock()
		f.events = nil
		f.mtx.Unlock()
		f.x.hooks.commitAfterLog = inlineOnce(func(a *nativeMetricMetadataAppender) {
			require.Len(t, a.walEntries, 1)
			require.Equal(t, partner, a.walEntries[0].Ref)
			afterLog(a)
		})
		require.NoError(t, app.Commit())
		requireNativeMetadataConverged(t, f.x.db, nil)
		require.Empty(t, pendingNativeMetadataRepairs(f.x.h))
		return partner
	}

	t.Run("a stable group before a logged group of its stripe", func(t *testing.T) {
		// B@200 is still stable when T merges: it has no merge point, and
		// nothing is overridden.
		f := newOrderFixture(t, points([]int64{100}, "A"))
		partner := partnerLoggedOnly(t, f, "P", func(*nativeMetricMetadataAppender) {})
		require.Zero(t, prom_testutil.ToFloat64(f.x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder)))
		require.Equal(t, []mergePointEvent{{site: nativeMetadataMergeSiteMerge, ref: partner}}, f.events)
	})

	t.Run("a group merged without being logged before a logged group of its stripe", func(t *testing.T) {
		// Before T merges, C@175 commits, so B@200 merges after all, and the
		// target's epoch then equals the one T recorded for the partner's
		// group. Only the target's override puts B@200 in the WAL.
		f := newOrderFixture(t, points([]int64{100}, "A"))
		for i, help := range []string{"P1", "P2", "P3"} {
			f.x.mustCommit(f.partner, int64(10*(i+1)), help)
		}
		partnerLoggedOnly(t, f, "P4", func(a *nativeMetricMetadataAppender) {
			f.x.mustCommit(f.target, 175, "C")
			require.Equal(t, a.walEpochs[0], f.x.epoch(f.ref), "the target's epoch equals the partner's recorded one")
		})
		require.Equal(t, 1.0, prom_testutil.ToFloat64(f.x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder)))
		require.True(t, f.mismatchAt(nativeMetadataMergeSiteMerge))
	})

	t.Run("a group skipped between groups merged out of order", func(t *testing.T) {
		// [A@100]; logged W@350, V@360, V@300, merged in reverse. V@360 is
		// then stable and skipped, so native never retains it, while WAL
		// order does.
		for _, s := range skipSites {
			t.Run(string(s.site), func(t *testing.T) {
				f := newOrderFixture(t, points([]int64{100}, "A"))
				f.run(t, []orderTxn{
					{points: points([]int64{350}, "W"), pause: orderAfterLog},
					{points: points([]int64{360}, "V"), pause: s.pause, partner: s.partner},
					{points: points([]int64{300}, "V")},
				}, []int{1, 0})
				f.requireOverridden(t)
				require.True(t, f.mismatchAt(s.site))
				require.True(t, f.mismatchAt(nativeMetadataMergeSiteMerge))
			})
		}
	})
}

// requireOverridden requires the WAL to have converged through overrides that
// the committing transactions wrote.
func (f *orderFixture) requireOverridden(t *testing.T) {
	t.Helper()
	requireNativeMetadataConverged(t, f.x.db, nil)
	require.Positive(t, prom_testutil.ToFloat64(f.x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder)))
	require.Empty(t, pendingNativeMetadataRepairs(f.x.h))
}

func TestNativeMetricMetadataOverrideLoop(t *testing.T) {
	la := labels.FromStrings(labels.MetricName, "a")
	mergeOrderWrites := func(x *repairHarness) float64 {
		return prom_testutil.ToFloat64(x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder))
	}
	// overriding commits T1 = {B@200} on [A@100], after T0 = {C@300} merges
	// between T1's log and its merge, so T1 takes the override path. Every
	// transaction runs on the test goroutine: hooks run the others inline.
	overriding := func(x *repairHarness) func() {
		x.mustCommit(la, 100, "A")
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			x.mustCommit(la, 300, "C")
		})
		return func() {
			_, err := x.commit(la, 200, "B")
			require.NoError(t, err)
		}
	}

	t.Run("a bump after every write", func(t *testing.T) {
		// T4 merges between T1's history read and its first write, and T3
		// between its second read and write. Each write's post-write lock
		// finds the epoch changed, so T1 writes a third time, after both. A
		// loop that stopped after its second write would leave T3's group
		// out of the WAL's final history.
		x := newRepairHarness(t)
		commitT1 := overriding(x)
		writes := 0
		x.hooks.overrideBeforeWrite = func([]chunks.HeadSeriesRef) {
			writes++
			switch writes {
			case 1:
				x.mustCommit(la, 400, "D")
			case 2:
				x.mustCommit(la, 500, "E")
			}
		}
		// Each write holds the history its preceding read found.
		require.Equal(t, 3, requireOverridesOfHistory(t, x, commitT1))
		require.Equal(t, 3, writes)
		require.Equal(t, 3.0, mergeOrderWrites(x))
		require.Empty(t, pendingNativeMetadataRepairs(x.h))
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("an exhausted loop", func(t *testing.T) {
		// Another merge lands before each post-write lock, so after its
		// last write T1 leaves the series to the worker.
		x := newRepairHarness(t)
		commitT1 := overriding(x)
		ts := int64(400)
		x.hooks.overrideAfterWrite = func(error) {
			x.mustCommit(la, ts, strconv.FormatInt(ts, 10))
			ts += 100
		}
		commitT1()
		x.hooks.overrideAfterWrite = nil
		require.Equal(t, float64(nativeMetadataOverrideWrites), mergeOrderWrites(x))
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairRegistrations.WithLabelValues(nativeMetadataRepairUnsettledOverride)))
		x.check(1, pendingNativeMetadataRepairs(x.h))
		require.Len(t, pendingNativeMetadataRepairs(x.h), 1)
		require.True(t, x.pass())
		x.check(1, nil)
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("a failed override write", func(t *testing.T) {
		// The write fails, but wlog retains it; the series is pending, with
		// one pin, and the worker's override lands after the retained one.
		x := newRepairHarness(t)
		commitT1 := overriding(x)
		x.hooks.overrideBeforeWrite = inlineOnce(func([]chunks.HeadSeriesRef) {
			x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
		})
		commitT1()
		require.Zero(t, x.faults.armed())
		require.Equal(t, 1, x.faults.failedRecords(record.NativeMetadataCompact), "the failed override is compact")
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairRegistrations.WithLabelValues(nativeMetadataRepairFailedOverride)))
		ref := x.h.series.getByHash(la.Hash(), la).ref
		x.check(1, []chunks.HeadSeriesRef{ref})
		require.True(t, x.pass())
		x.check(1, nil, ref)
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("a registration's bump", func(t *testing.T) {
		// A registration on a series already pending changes its epoch, so
		// a transaction logged before it overrides at its merge.
		x := newRepairHarness(t)
		ref := x.mustCommit(la, 100, "A")
		x.failCommit(la, 150, "phantom 1")
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			x.failCommit(la, 160, "phantom 2")
		})
		var mismatches int
		x.h.nativeMetricMetadata.testMergePoint = func(_ nativeMetadataMergeSite, got chunks.HeadSeriesRef, mismatch bool) {
			if got == ref && mismatch {
				mismatches++
			}
		}
		_, err := x.commit(la, 200, "B")
		require.NoError(t, err)
		require.Equal(t, 1, mismatches)
		require.Equal(t, 1.0, mergeOrderWrites(x))
		x.check(1, []chunks.HeadSeriesRef{ref})
		require.True(t, x.pass())
		x.check(1, nil, ref)
		requireNativeMetadataConverged(t, x.db, nil)
	})
}

// receive waits for ch, failing the test instead of hanging if what never
// happens.
func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

// tryCommit is repairHarness.commit for goroutines other than the test's.
func (x *repairHarness) tryCommit(lset labels.Labels, ts int64, help string) error {
	app := x.db.AppenderV2(x.t.Context())
	if _, err := app.Append(0, lset, 0, ts, 1, nil, nil, repairMetadata(help)); err != nil {
		_ = app.Rollback()
		return err
	}
	return app.Commit()
}

// sequencer lets events of concurrent actors happen in a chosen order. An
// actor waits for its event's turn, and reports the event finished.
type sequencer struct {
	grant, done map[string]chan struct{}
	finished    map[string]*sync.Once
}

func newSequencer(events ...string) *sequencer {
	s := &sequencer{grant: map[string]chan struct{}{}, done: map[string]chan struct{}{}, finished: map[string]*sync.Once{}}
	for _, e := range events {
		s.grant[e], s.done[e], s.finished[e] = make(chan struct{}), make(chan struct{}), &sync.Once{}
	}
	return s
}

func (s *sequencer) wait(e string)   { <-s.grant[e] }
func (s *sequencer) finish(e string) { s.finished[e].Do(func() { close(s.done[e]) }) }

func (s *sequencer) run(t *testing.T, order []string) {
	for _, e := range order {
		close(s.grant[e])
		receive(t, s.done[e], e)
	}
}

func TestNativeMetricMetadataRepair_AgainstOverrides(t *testing.T) {
	la := labels.FromStrings(labels.MetricName, "a")
	// setup leaves the series pending after a failed commit, and arranges for
	// T = {B@200} to take the override path: C@300 merges between T's log and
	// its merge.
	setup := func(t *testing.T) (*repairHarness, chunks.HeadSeriesRef) {
		x := newRepairHarness(t)
		ref := x.mustCommit(la, 100, "A")
		x.failCommit(la, 150, "phantom")
		x.check(1, []chunks.HeadSeriesRef{ref})
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			x.mustCommit(la, 300, "C")
		})
		return x, ref
	}

	t.Run("an override retained after the worker's write", func(t *testing.T) {
		// 1. T reads H1 and waits before its write. 2. U merges with a
		// matching epoch: native is H2. 3. The worker copies H2 and writes
		// it. 4. T's write of H1 fails, retained, and T registers the
		// series. Its registration changes the epoch, so 5. the worker keeps
		// the series if it hasn't discharged it yet, and otherwise the series
		// re-enters the set. 6. H1 lands, and a later pass writes H2 after it.
		// Discharging by unchanged history, or registering without a bump,
		// would leave H1 last.
		for _, beforeLock := range []bool{true, false} {
			t.Run(fmt.Sprintf("registration before the worker's post-write lock %t", beforeLock), func(t *testing.T) {
				x, ref := setup(t)
				reached, resume := make(chan struct{}), make(chan struct{})
				x.hooks.overrideBeforeWrite = inlineOnce(func([]chunks.HeadSeriesRef) {
					close(reached)
					<-resume
					x.faults.arm(nativeMetadataRecords, wlog.FlushAfterRecord)
				})
				done := make(chan error, 1)
				go func() { done <- x.tryCommit(la, 200, "B") }()
				receive(t, reached, "T's override write")
				x.mustCommit(la, 400, "D")
				if beforeLock {
					x.hooks.repairAfterWrite = inlineOnce(func(err error) {
						require.NoError(t, err)
						close(resume)
						require.NoError(t, receive(t, done, "T's commit"))
					})
					require.True(t, x.pass())
					// The first write didn't discharge; the second did.
					require.Equal(t, 2.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideRepair)))
				} else {
					require.True(t, x.pass())
					// Discharged; T's queued sample still reserves the series.
					require.Empty(t, pendingNativeMetadataRepairs(x.h))
					require.Equal(t, 1, x.reservations(ref))
					close(resume)
					require.NoError(t, receive(t, done, "T's commit"))
					x.check(1, []chunks.HeadSeriesRef{ref})
					require.True(t, x.pass())
				}
				require.Zero(t, x.faults.armed())
				require.Positive(t, x.faults.failedRecords(record.NativeMetadataCompact), "the failed override is compact")
				require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataRepairRegistrations.WithLabelValues(nativeMetadataRepairFailedOverride)))
				x.check(1, nil, ref)
				requireNativeMetadataConverged(t, x.db, nil)
			})
		}
	})

	t.Run("a group logged before the worker's write, merged after its discharge", func(t *testing.T) {
		// The worker copies H, then T3 logs G3 and waits before merging. The
		// worker's override of H lands after G3, and with its epoch unchanged
		// the worker discharges. Its post-write bump then makes T3's merge
		// find a mismatch, so T3 overrides with H+G3; without the bump, the
		// WAL would end at H.
		x := newRepairHarness(t)
		ref := x.mustCommit(la, 100, "A")
		x.failCommit(la, 150, "phantom")
		x.check(1, []chunks.HeadSeriesRef{ref})
		logged, resume := make(chan struct{}), make(chan struct{})
		x.hooks.commitAfterLog = inlineOnce(func(*nativeMetricMetadataAppender) {
			close(logged)
			<-resume
		})
		done := make(chan error, 1)
		x.hooks.repairBeforeWrite = inlineOnce(func([]chunks.HeadSeriesRef) {
			go func() { done <- x.tryCommit(la, 300, "C") }()
			receive(t, logged, "T3's log")
		})
		require.True(t, x.pass())
		require.Empty(t, pendingNativeMetadataRepairs(x.h))
		close(resume)
		require.NoError(t, receive(t, done, "T3's commit"))
		require.Equal(t, 1.0, prom_testutil.ToFloat64(x.h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder)))
		x.check(1, nil, ref)
		requireNativeMetadataConverged(t, x.db, nil)
	})

	t.Run("the worker against Commit's loop", func(t *testing.T) {
		// Both have read the same history when they write it. Tw and Ww are
		// their writes, Tp and Wp their post-write locks, in every order. The
		// worker discharges only if its epoch is unchanged at Wp, that is,
		// only if T's post-write lock, which bumps, hasn't come first.
		for _, order := range [][]string{
			{"Tw", "Tp", "Ww", "Wp"},
			{"Tw", "Ww", "Tp", "Wp"},
			{"Tw", "Ww", "Wp", "Tp"},
			{"Ww", "Tw", "Tp", "Wp"},
			{"Ww", "Tw", "Wp", "Tp"},
			{"Ww", "Wp", "Tw", "Tp"},
		} {
			t.Run(strings.Join(order, " "), func(t *testing.T) {
				x, ref := setup(t)
				seq := newSequencer("Tw", "Tp", "Ww", "Wp")
				tReady, wReady := make(chan struct{}), make(chan struct{})
				var tWrites, wWrites atomic.Int32
				x.hooks.overrideBeforeWrite = func([]chunks.HeadSeriesRef) {
					switch tWrites.Add(1) {
					case 1:
						close(tReady)
						seq.wait("Tw")
					case 2:
						seq.finish("Tp")
					}
				}
				x.hooks.overrideAfterWrite = func(error) {
					if tWrites.Load() == 1 {
						seq.finish("Tw")
						seq.wait("Tp")
					}
				}
				var kept atomic.Bool
				x.hooks.repairBeforeWrite = func([]chunks.HeadSeriesRef) {
					switch wWrites.Add(1) {
					case 1:
						close(wReady)
						seq.wait("Ww")
					case 2:
						kept.Store(true)
						seq.finish("Wp")
					}
				}
				x.hooks.repairAfterWrite = func(error) {
					if wWrites.Load() == 1 {
						seq.finish("Ww")
						seq.wait("Wp")
					}
				}
				tDone, wDone := make(chan error, 1), make(chan bool, 1)
				go func() {
					err := x.tryCommit(la, 200, "B")
					seq.finish("Tp")
					tDone <- err
				}()
				receive(t, tReady, "T's first override write")
				go func() {
					ok := x.pass()
					seq.finish("Wp")
					wDone <- ok
				}()
				receive(t, wReady, "the worker's first write")
				seq.run(t, order)
				require.NoError(t, receive(t, tDone, "T's commit"))
				require.True(t, receive(t, wDone, "the worker's pass"))
				require.Equal(t, slices.Index(order, "Tp") < slices.Index(order, "Wp"), kept.Load())

				x.hooks.overrideBeforeWrite, x.hooks.overrideAfterWrite = nil, nil
				x.hooks.repairBeforeWrite, x.hooks.repairAfterWrite = nil, nil
				require.True(t, x.pass())
				x.check(1, nil, ref)
				requireNativeMetadataConverged(t, x.db, nil)
			})
		}
	})
}
