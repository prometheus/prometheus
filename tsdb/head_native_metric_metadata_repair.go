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
	"cmp"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

const (
	// nativeMetadataRepairBatch bounds the overrides in one repair record.
	nativeMetadataRepairBatch = 256
	// nativeMetadataRepairBatchBytes is the encoded size a repair record stays
	// within, unless one override alone exceeds it, as a full history of long
	// values can.
	nativeMetadataRepairBatchBytes = 32 << 10
	// nativeMetadataRepairKeepBytes bounds the encoding buffer kept between
	// repair records.
	nativeMetadataRepairKeepBytes = nativeMetadataRepairBatchBytes

	nativeMetadataRepairMinBackoff = time.Second
	nativeMetadataRepairMaxBackoff = 30 * time.Second
	// nativeMetadataRepairCloseTimeout is how long after Close begins repair
	// writes are still admitted.
	nativeMetadataRepairCloseTimeout = 5 * time.Second
)

// Causes of native metadata repair registrations.
const (
	nativeMetadataRepairFailedCommit      = "failed_commit"
	nativeMetadataRepairFailedOverride    = "failed_override"
	nativeMetadataRepairUnsettledOverride = "unsettled_override"
	nativeMetadataRepairFailedRepair      = "failed_repair"
)

// nativeMetadataOverrideWrites bounds the overrides a committing transaction
// writes for a series before leaving it to the repair worker.
const nativeMetadataOverrideWrites = 3

// Causes of runtime native metadata overrides.
const (
	nativeMetadataOverrideRepair     = "repair"
	nativeMetadataOverrideMergeOrder = "merge_order"
)

// Causes of native metadata handoffs.
const (
	nativeMetadataHandoffStartup    = "startup"
	nativeMetadataHandoffCheckpoint = "checkpoint"
)

// nativeMetadataRepairs tracks series whose WAL may hold native metadata that
// their native history doesn't, until an override carrying that history is
// written after it. Each pending series holds one pending-commit reservation,
// so GC can't retire it before its override is written.
//
// One worker writes the overrides. A pending series is discharged only if its
// epoch hasn't changed since the worker read the history it wrote, so every
// record that can land after that override has a writer that acts after the
// discharge: a merge that compares its own epoch, or a new registration.
type nativeMetadataRepairs struct {
	// mtx guards the fields below. It is a leaf lock: it may be taken while a
	// series lock is held, and nothing is locked, waited for or written while
	// it is held.
	mtx repairMutex
	// pending maps each pending series' ref to the series, as its registrant
	// held it. It is never looked up by ref: a creator publishes a series by
	// labels before it adds it to the ref index, so a pending series can be
	// missing from the index while it is live. A pending series is pinned, so
	// the set keeps no retired series alive. A map never releases storage, so
	// it is replaced whenever it empties.
	pending map[chunks.HeadSeriesRef]*memSeries
	// started is set once the worker runs, and closing once Close has begun
	// dropping what remains; later registrations are dropped.
	started, closing bool
	// deadline is set when Close begins. Repair writes are admitted only
	// before it, in the pass in progress and in the final pass alike.
	deadline time.Time

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once

	minBackoff, maxBackoff, closeTimeout time.Duration

	// Scratch owned by the worker, or by Close once the worker has stopped.
	selected []*memSeries
	batch    []nativeMetadataRepair
	record   nativeMetadataOverrideRecord
	buf      []byte
}

// nativeMetadataRepair is an override copied from a pending series.
type nativeMetadataRepair struct {
	series    *memSeries
	epoch     uint32
	truncated bool
	count     int
	points    [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
}

func newNativeMetadataRepairs() *nativeMetadataRepairs {
	return &nativeMetadataRepairs{
		pending:      map[chunks.HeadSeriesRef]*memSeries{},
		wake:         make(chan struct{}, 1),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		minBackoff:   nativeMetadataRepairMinBackoff,
		maxBackoff:   nativeMetadataRepairMaxBackoff,
		closeTimeout: nativeMetadataRepairCloseTimeout,
		selected:     make([]*memSeries, 0, nativeMetadataRepairBatch),
		batch:        make([]nativeMetadataRepair, 0, nativeMetadataRepairBatch),
		// A batch defines at most one value per point.
		record: nativeMetadataOverrideRecord{
			values:  make([]record.NativeMetadataValue, 0, nativeMetadataRepairBatch*maxNativeMetricMetadataVersions),
			indices: make(map[*metadata.Metadata]uint32, nativeMetadataRepairBatch*maxNativeMetricMetadataVersions),
			entries: make([]record.RefCompactNativeMetadata, 0, nativeMetadataRepairBatch),
			points:  make([]record.CompactNativeMetadataPoint, 0, nativeMetadataRepairBatch*maxNativeMetricMetadataVersions),
		},
	}
}

// nativeMetadataOverrideRecord builds a compact record of overrides from
// native histories. Its dictionary is keyed by value pointer, since native
// histories hold immutable values: no string is hashed, and equal values under
// different pointers are defined once each. Its contents are valid until it is
// reset.
type nativeMetadataOverrideRecord struct {
	values  []record.NativeMetadataValue
	indices map[*metadata.Metadata]uint32
	entries []record.RefCompactNativeMetadata
	points  []record.CompactNativeMetadataPoint
}

// add adds an override of ref with points, a native history.
func (r *nativeMetadataOverrideRecord) add(ref chunks.HeadSeriesRef, truncated, handoff bool, points []nativeMetricMetadataPoint) {
	if r.indices == nil {
		r.indices = map[*metadata.Metadata]uint32{}
	}
	start := len(r.points)
	for _, p := range points {
		index, ok := r.indices[p.Metadata]
		if !ok {
			index = uint32(len(r.values))
			r.indices[p.Metadata] = index
			r.values = append(r.values, record.NativeMetadataValue{Type: record.GetMetricType(p.Metadata.Type), Unit: p.Metadata.Unit, Help: p.Metadata.Help})
		}
		r.points = append(r.points, record.CompactNativeMetadataPoint{EffectiveFrom: p.EffectiveFrom, Value: index})
	}
	r.entries = append(r.entries, record.RefCompactNativeMetadata{
		Ref: ref, Kind: record.NativeMetadataOverride, Truncated: truncated, Handoff: handoff,
		Points: r.points[start:len(r.points):len(r.points)],
	})
}

// len returns the number of overrides added since the last reset.
func (r *nativeMetadataOverrideRecord) len() int { return len(r.entries) }

// encode appends the record to b.
func (r *nativeMetadataOverrideRecord) encode(b []byte) []byte {
	var enc record.Encoder
	return enc.CompactNativeMetadata(r.values, r.entries, b)
}

// reset empties r, keeping its capacity but no values.
func (r *nativeMetadataOverrideRecord) reset() {
	clear(r.values)
	clear(r.indices)
	clear(r.entries)
	r.values, r.entries, r.points = r.values[:0], r.entries[:0], r.points[:0]
}

// reset forgets every pending repair, along with the series replaced by a Head
// reset. The caller must exclude concurrent appenders and repairs.
func (r *nativeMetadataRepairs) reset() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.pending = map[chunks.HeadSeriesRef]*memSeries{}
}

// nativeMetadataTestHooks pause or observe native metadata commits and repairs
// at fixed points. Production heads leave them unset.
type nativeMetadataTestHooks struct {
	// manualRepairs keeps the worker from starting; tests run passes instead.
	manualRepairs bool
	// commitAfterLog runs in Commit after a successful log, before native
	// metadata merges.
	commitAfterLog func(*nativeMetricMetadataAppender)
	// overrideBeforeWrite runs in Commit before each override write, and
	// overrideAfterWrite after it, before the series are locked again.
	overrideBeforeWrite func(refs []chunks.HeadSeriesRef)
	overrideAfterWrite  func(err error)
	// rollbackAfterRelease runs in Rollback once sample reservations are
	// released, before it logs.
	rollbackAfterRelease func()
	// repairBeforeWrite runs once a repair batch is prepared, before its
	// write is admitted.
	repairBeforeWrite func(refs []chunks.HeadSeriesRef)
	// repairAfterWrite runs after a repair batch's write, before its series
	// are locked again.
	repairAfterWrite func(err error)
	// repairBeforeLock runs after a repair batch's write, before each of its
	// series is locked again.
	repairBeforeLock func(ref chunks.HeadSeriesRef)
	// tornTailTruncate and tornTailSync, if set, run instead of truncating
	// and syncing the segment of a torn final record at startup.
	tornTailTruncate func(f *os.File, offset int64) error
	tornTailSync     func(f *os.File) error
}

// writeNativeMetadataOverrides writes overrides with the native histories of
// series that a's commit may have merged out of WAL order. The transaction
// still reserves them. Each write is followed, under each series' lock, by an
// epoch change; a series whose epoch changed since its history was read is
// written again. After nativeMetadataOverrideWrites writes, or a failed one,
// the series left are registered for the repair worker.
func (h *Head) writeNativeMetadataOverrides(a *nativeMetricMetadataAppender) {
	hooks := h.nativeMetadataTestHooks
	buf := h.getBytesBuffer()
	defer func() { h.putBytesBuffer(buf) }()
	series := a.overrides
	rec := &a.overrideRecord
	defer rec.reset()
	for writes := 0; len(series) > 0; writes++ {
		if writes == nativeMetadataOverrideWrites {
			h.registerNativeMetadataRepairs(series, nativeMetadataRepairUnsettledOverride)
			return
		}
		// The pre-log pass's epochs are no longer needed; reuse their storage.
		epochs := a.walEpochs[:0]
		rec.reset()
		var history [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
		for _, s := range series {
			s.Lock()
			epochs = append(epochs, s.nativeMetadataEpochLocked())
			var points []nativeMetricMetadataPoint
			truncated := false
			if native := s.nativeMetadataLocked(); native != nil {
				points = native.AppendPoints(history[:0])
				truncated = native.truncated
			}
			rec.add(s.ref, truncated, false, points)
			s.Unlock()
		}
		a.walEpochs = epochs
		buf = rec.encode(buf[:0])
		if hooks != nil && hooks.overrideBeforeWrite != nil {
			refs := make([]chunks.HeadSeriesRef, len(series))
			for i, s := range series {
				refs[i] = s.ref
			}
			hooks.overrideBeforeWrite(refs)
		}
		err := h.wal.Log(buf)
		if hooks != nil && hooks.overrideAfterWrite != nil {
			hooks.overrideAfterWrite(err)
		}
		if err != nil {
			h.logger.Warn("Writing native metadata overrides failed; retrying in the background", "overrides", len(series), "err", err)
			h.registerNativeMetadataRepairs(series, nativeMetadataRepairFailedOverride)
			return
		}
		h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideMergeOrder).Add(float64(len(series)))
		unsettled := series[:0]
		for i, s := range series {
			s.Lock()
			if s.nativeMetadataEpochLocked() != epochs[i] {
				unsettled = append(unsettled, s)
			}
			s.bumpNativeMetadataEpochLocked()
			s.Unlock()
		}
		series = unsettled
	}
}

// registerNativeMetadataRepairs registers repairs for series, which the caller
// reserves.
func (h *Head) registerNativeMetadataRepairs(series []*memSeries, cause string) {
	for _, s := range series {
		s.Lock()
		h.registerNativeMetadataRepairLocked(s, cause)
		s.Unlock()
	}
}

func (h *Head) nativeMetadataViolation(what string, ref chunks.HeadSeriesRef) {
	h.metrics.nativeMetadataRepairViolations.Inc()
	h.logger.Error("Native metadata repair invariant violated", "violation", what, "series", ref)
}

// registerNativeMetadataRepairLocked records that s's WAL may hold native
// metadata that its native history doesn't. It changes s's epoch, so that no
// writer that read it earlier settles s, and pins s unless it is pending
// already. The caller holds s's lock, and a reservation on s.
func (h *Head) registerNativeMetadataRepairLocked(s *memSeries, cause string) {
	s.bumpNativeMetadataEpochLocked()
	h.metrics.nativeMetadataRepairRegistrations.WithLabelValues(cause).Inc()
	if s.isGCed() {
		h.nativeMetadataViolation("registration of a retired series", s.ref)
		return
	}
	r := h.nativeMetadataRepairs
	r.mtx.Lock()
	_, pending := r.pending[s.ref]
	closing := r.closing
	start := false
	if !pending && !closing {
		r.pending[s.ref] = s
		start = !r.started && (h.nativeMetadataTestHooks == nil || !h.nativeMetadataTestHooks.manualRepairs)
		r.started = r.started || start
	}
	r.mtx.Unlock()
	switch {
	case closing:
		h.metrics.nativeMetadataRepairsDropped.Inc()
		h.logger.Warn("Dropped a native metadata repair registered during shutdown", "series", s.ref)
		return
	case !pending:
		s.markPendingCommit()
		h.metrics.nativeMetadataRepairsPending.Inc()
	}
	if start {
		go h.runNativeMetadataRepairs()
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// dischargeNativeMetadataRepairLocked removes s's pending repair and releases
// its pin, in one hold of s's lock.
func (h *Head) dischargeNativeMetadataRepairLocked(s *memSeries) {
	r := h.nativeMetadataRepairs
	r.mtx.Lock()
	_, pending := r.pending[s.ref]
	delete(r.pending, s.ref)
	if len(r.pending) == 0 {
		r.pending = map[chunks.HeadSeriesRef]*memSeries{}
	}
	r.mtx.Unlock()
	if !pending {
		h.nativeMetadataViolation("discharge of a series not pending", s.ref)
		return
	}
	h.metrics.nativeMetadataRepairsPending.Dec()
	if !s.unmarkPendingCommit() {
		h.metrics.pendingCommitUnderflow.Inc()
		h.logger.Error("Tried to release a native metadata repair pin that was not held", "series", s.ref)
	}
}

// forgetNativeMetadataRepair removes a pending ref whose series was retired,
// after an invariant violation.
func (h *Head) forgetNativeMetadataRepair(ref chunks.HeadSeriesRef) {
	r := h.nativeMetadataRepairs
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if _, ok := r.pending[ref]; ok {
		delete(r.pending, ref)
		h.metrics.nativeMetadataRepairsPending.Dec()
	}
	if len(r.pending) == 0 {
		r.pending = map[chunks.HeadSeriesRef]*memSeries{}
	}
}

// lockPendingNativeMetadataSeries locks a pending series. A pending series is
// pinned, so a retired one violates an invariant; it is then forgotten, and
// false returned. beforeLock, if set, runs first.
func (h *Head) lockPendingNativeMetadataSeries(s *memSeries, beforeLock func(chunks.HeadSeriesRef)) bool {
	if beforeLock != nil {
		beforeLock(s.ref)
	}
	s.Lock()
	if s.isGCed() {
		s.Unlock()
		h.nativeMetadataViolation("a pending series was retired", s.ref)
		h.forgetNativeMetadataRepair(s.ref)
		return false
	}
	return true
}

// runNativeMetadataRepairs is the repair worker. It writes pending repairs when
// woken, retries with exponential backoff after a failed write, and makes a
// final pass when stopped.
func (h *Head) runNativeMetadataRepairs() {
	r := h.nativeMetadataRepairs
	defer close(r.done)
	var timer *time.Timer
	for {
		select {
		case <-r.stop:
			h.finishNativeMetadataRepairs()
			return
		case <-r.wake:
		}
		for backoff := r.minBackoff; !h.nativeMetadataRepairPass(); backoff = min(2*backoff, r.maxBackoff) {
			if timer == nil {
				timer = time.NewTimer(backoff)
			} else {
				timer.Reset(backoff)
			}
			select {
			case <-r.stop:
				timer.Stop()
				h.finishNativeMetadataRepairs()
				return
			case <-timer.C:
			}
		}
	}
}

// stopNativeMetadataRepairs starts the deadline for repair writes, lets the
// worker finish its pass and make the final one, then drops what remains and
// stops the worker, before the WAL closes. Appenders must be closed.
func (h *Head) stopNativeMetadataRepairs() {
	r := h.nativeMetadataRepairs
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.mtx.Lock()
		started := r.started
		r.started = true // No worker starts from here on.
		r.deadline = time.Now().Add(r.closeTimeout)
		r.mtx.Unlock()
		if !started {
			h.finishNativeMetadataRepairs()
			return
		}
		close(r.stop)
		<-r.done
	})
}

// finishNativeMetadataRepairs makes one final pass, under the deadline Close
// started, then drops and unpins what remains. It runs on the worker, or on
// Close if the worker never started.
func (h *Head) finishNativeMetadataRepairs() {
	r := h.nativeMetadataRepairs
	h.nativeMetadataRepairPass()
	r.mtx.Lock()
	r.closing = true
	series := make([]*memSeries, 0, len(r.pending))
	for _, s := range r.pending {
		series = append(series, s)
	}
	r.mtx.Unlock()
	for _, s := range series {
		if h.lockPendingNativeMetadataSeries(s, nil) {
			h.dischargeNativeMetadataRepairLocked(s)
			s.Unlock()
		}
	}
	if len(series) > 0 {
		h.metrics.nativeMetadataRepairsDropped.Add(float64(len(series)))
		h.logger.Warn("Dropped native metadata repairs at shutdown; senders and replay may disagree with native metadata", "repairs", len(series))
	}
}

// nativeMetadataRepairPass writes overrides for pending series in batches until
// none is pending, or a write fails or isn't admitted. It reports whether none
// is pending.
func (h *Head) nativeMetadataRepairPass() bool {
	for {
		if !h.collectNativeMetadataRepairs() {
			return true
		}
		if !h.writeNativeMetadataRepairs() {
			return false
		}
	}
}

// collectNativeMetadataRepairs copies a batch of pending series' histories and
// epochs. It reports whether it copied any; if not, none is pending.
func (h *Head) collectNativeMetadataRepairs() bool {
	r := h.nativeMetadataRepairs
	for len(r.batch) == 0 {
		r.mtx.Lock()
		for _, s := range r.pending {
			if len(r.selected) == cap(r.selected) {
				break
			}
			r.selected = append(r.selected, s)
		}
		r.mtx.Unlock()
		if len(r.selected) == 0 {
			return false
		}
		// Ref order makes batches deterministic when all pending series fit.
		slices.SortFunc(r.selected, func(a, b *memSeries) int { return cmp.Compare(a.ref, b.ref) })
		size := 0
		for _, s := range r.selected {
			// A series that can't be locked is forgotten, so the next
			// collection makes progress.
			if !h.lockPendingNativeMetadataSeries(s, nil) {
				continue
			}
			repair := nativeMetadataRepair{series: s, epoch: s.nativeMetadataEpochLocked()}
			if native := s.nativeMetadataLocked(); native != nil {
				repair.count = copy(repair.points[:], native.Older)
				repair.points[repair.count] = nativeMetricMetadataPoint{EffectiveFrom: native.EffectiveFrom, Metadata: native.Metadata}
				repair.count++
				repair.truncated = native.truncated
			}
			s.Unlock()
			entrySize := repair.encodedSizeEstimate()
			if len(r.batch) > 0 && size+entrySize > nativeMetadataRepairBatchBytes {
				break
			}
			size += entrySize
			r.batch = append(r.batch, repair)
		}
		clear(r.selected)
		r.selected = r.selected[:0]
	}
	return true
}

// encodedSizeEstimate bounds the override's encoded size from above.
func (r *nativeMetadataRepair) encodedSizeEstimate() int {
	return nativeMetadataEntrySizeEstimate(r.points[:r.count])
}

// nativeMetadataEntrySizeEstimate bounds the encoded size of an override with
// points, of at most five, from above. A compact record of fewer than 2^14
// such overrides is no larger than its entries' estimates together, framing
// included, whatever the sizes of their strings, refs and starts.
func nativeMetadataEntrySizeEstimate(points []nativeMetricMetadataPoint) int {
	size := 32
	for _, p := range points {
		size += 32 + len(p.Metadata.Unit) + len(p.Metadata.Help)
	}
	return size
}

// writeNativeMetadataRepairs writes the collected batch as one record, if the
// write is admitted, then discharges each series whose epoch is unchanged since
// it was copied. Every series' epoch changes after a write. It reports whether
// the write was admitted and succeeded.
func (h *Head) writeNativeMetadataRepairs() bool {
	r := h.nativeMetadataRepairs
	hooks := h.nativeMetadataTestHooks
	defer func() {
		clear(r.batch)
		r.batch = r.batch[:0]
		r.record.reset()
	}()
	for i := range r.batch {
		repair := &r.batch[i]
		r.record.add(repair.series.ref, repair.truncated, false, repair.points[:repair.count])
	}
	rec := r.record.encode(r.buf[:0])
	if hooks != nil && hooks.repairBeforeWrite != nil {
		refs := make([]chunks.HeadSeriesRef, len(r.batch))
		for i := range r.batch {
			refs[i] = r.batch[i].series.ref
		}
		hooks.repairBeforeWrite(refs)
	}
	// A larger buffer was needed for an oversized batch only.
	if cap(rec) <= nativeMetadataRepairKeepBytes {
		r.buf = rec[:0]
	} else {
		r.buf = nil
	}
	// The write is admitted here, once its batch is prepared, and only before
	// the deadline Close started, if any. A write admitted in time may
	// finish after it.
	r.mtx.Lock()
	deadline := r.deadline
	r.mtx.Unlock()
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return false
	}
	err := h.wal.Log(rec)
	if hooks != nil && hooks.repairAfterWrite != nil {
		hooks.repairAfterWrite(err)
	}
	var beforeLock func(chunks.HeadSeriesRef)
	if hooks != nil {
		beforeLock = hooks.repairBeforeLock
	}
	if err != nil {
		h.logger.Warn("Writing native metadata repairs failed; retrying", "repairs", len(r.batch), "err", err)
		for i := range r.batch {
			if s := r.batch[i].series; h.lockPendingNativeMetadataSeries(s, beforeLock) {
				h.registerNativeMetadataRepairLocked(s, nativeMetadataRepairFailedRepair)
				s.Unlock()
			}
		}
		return false
	}
	h.metrics.nativeMetadataOverridesWritten.WithLabelValues(nativeMetadataOverrideRepair).Add(float64(len(r.batch)))
	discharged := 0
	for i := range r.batch {
		repair := &r.batch[i]
		s := repair.series
		if !h.lockPendingNativeMetadataSeries(s, beforeLock) {
			continue
		}
		if s.nativeMetadataEpochLocked() == repair.epoch {
			h.dischargeNativeMetadataRepairLocked(s)
			discharged++
		}
		s.bumpNativeMetadataEpochLocked()
		s.Unlock()
	}
	h.metrics.nativeMetadataRepairsDischarged.Add(float64(discharged))
	return true
}
