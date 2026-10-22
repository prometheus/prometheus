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
	"slices"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

const (
	nativeMetricMetadataBitsPerWord   = 64
	maxNativeMetricMetadataValues     = 128 // Limits raw metadata retained by a transaction.
	maxNativeMetricMetadataBatch      = 256 // Bounds value preparation before merging a batch.
	nativeMetadataPooledStrings       = 256 // Larger string copiers are dropped rather than pooled.
	nativeMetricMetadataDirectRefMask = nativeMetricMetadataValueRef(1 << 31)
)

// nativeMetricMetadataValue retains raw metadata for comparisons without
// cloning. Once resolved, owned identifies an immutable copy of the same value.
type nativeMetricMetadataValue struct {
	metadata metadata.Metadata
	owned    *metadata.Metadata
}

// nativeMetricMetadataValueRef selects a one-based raw value, or a zero-based
// direct owned value when the high bit is set. Zero is not a value reference.
type nativeMetricMetadataValueRef uint32

// nativeMetricMetadataObservationRef is a one-based observation index.
// Zero denotes an empty stripe.
type nativeMetricMetadataObservationRef uint32

type nativeMetricMetadataObservation struct {
	series        *memSeries
	effectiveFrom int64
	metadataRef   nativeMetricMetadataValueRef
}

// nativeMetricMetadataGroup selects one series' observations with [start, end)
// indexes into nativeMetricMetadataAppender.sorted. emission indexes the
// group's WAL entry, or is -1 if the group wasn't logged.
type nativeMetricMetadataGroup struct {
	start    int
	end      int
	emission int
}

// nativeMetricMetadataAppender buffers metadata observations for one transaction
// until commit. It has a single owner and is not safe for concurrent use.
// Returning it through putAppender resets it for reuse. References into its
// mutable scratch storage must not survive its return to the pool.
type nativeMetricMetadataAppender struct {
	observations []nativeMetricMetadataObservation
	values       []nativeMetricMetadataValue
	valueRefs    map[metadata.Metadata]nativeMetricMetadataValueRef
	directValues []*metadata.Metadata
	valueCache   *nativeMetricMetadataValueCache
	// Copies of long metadata strings made by this transaction.
	strings nativeMetricMetadataStrings
	// Observation references in commit order, once sortedObservations has
	// run; the pre-log pass and commit share them.
	sorted       []nativeMetricMetadataObservationRef
	sortedLength int
	// Reusable merge input for the current series, with unique timestamps.
	points []nativeMetricMetadataPoint
	// Changing series selected for the current batch.
	groups []nativeMetricMetadataGroup
	// The first observation recorded for each stripe, or zero.
	stripeFirst [nativeMetricMetadataStripes]nativeMetricMetadataObservationRef
	// One bit per stripe, set when this transaction records observations for
	// multiple distinct series. Stripe sharing is expected.
	multiSeriesStripes [nativeMetricMetadataStripes / nativeMetricMetadataBitsPerWord]uint64
	lastMetadata       metadata.Metadata
	lastObservation    nativeMetricMetadataObservationRef
	// Reusable WAL record contents for the pre-log pass. Values retain the
	// strings of the transaction's values.
	walValues  []record.NativeMetadataValue
	walEntries []record.RefCompactNativeMetadata
	walPoints  []record.CompactNativeMetadataPoint
	// One-based indices into walValues by raw value reference and by direct
	// value index; zero before a value's first use in the record.
	walRawIndices    [maxNativeMetricMetadataValues + 1]uint32
	walDirectIndices []uint32
	// walSeries holds each WAL entry's series, and walEpochs its epoch as the
	// pre-log pass read it.
	walSeries []*memSeries
	walEpochs []uint32
	// walMayLand is set once the WAL entries' record was written, or failed
	// but stays whole in the WAL's page buffer, so it can still land.
	walMayLand bool
	// logged is set once the pre-log pass has run, so that commit compares
	// each merge point's order with the WAL's.
	logged bool
	// emitNext is the first WAL entry not yet matched with a group. Commit
	// visits groups in the pre-log pass's order.
	emitNext int
	// overrides holds series whose native merges may differ from WAL order.
	overrides []*memSeries
	// overrideRecord is the reusable record of their overrides.
	overrideRecord nativeMetadataOverrideRecord
}

func newNativeMetricMetadataAppender(cache *nativeMetricMetadataValueCache) *nativeMetricMetadataAppender {
	return &nativeMetricMetadataAppender{
		observations: make([]nativeMetricMetadataObservation, 0, nativeMetricMetadataStripes),
		values:       make([]nativeMetricMetadataValue, 0, maxNativeMetricMetadataValues),
		valueRefs:    make(map[metadata.Metadata]nativeMetricMetadataValueRef, maxNativeMetricMetadataValues),
		groups:       make([]nativeMetricMetadataGroup, 0, maxNativeMetricMetadataBatch),
		valueCache:   cache,
	}
}

func (a *nativeMetricMetadataAppender) metadataValue(ref nativeMetricMetadataValueRef) metadata.Metadata {
	if ref&nativeMetricMetadataDirectRefMask != 0 {
		return *a.directValues[ref&^nativeMetricMetadataDirectRefMask]
	}
	return a.values[ref-1].metadata
}

// metadataPointer lazily owns raw values. Commit resolves selected references
// before taking a series lock.
func (a *nativeMetricMetadataAppender) metadataPointer(series *memSeries, ref nativeMetricMetadataValueRef) *metadata.Metadata {
	if ref&nativeMetricMetadataDirectRefMask != 0 {
		return a.directValues[ref&^nativeMetricMetadataDirectRefMask]
	}
	value := &a.values[ref-1]
	if value.owned == nil {
		value.owned = a.resolveMetadata(series, value.metadata)
	}
	return value.owned
}

// metadataReference returns a transaction-local reference to m. It deduplicates
// a bounded set of raw values, deferring their cloning until needed. Values
// outside that set use direct owned pointers.
func (a *nativeMetricMetadataAppender) metadataReference(series *memSeries, m metadata.Metadata) nativeMetricMetadataValueRef {
	if valueRef, ok := a.valueRefs[m]; ok {
		return valueRef
	}
	if len(a.values) < maxNativeMetricMetadataValues {
		a.values = append(a.values, nativeMetricMetadataValue{metadata: m})
		valueRef := nativeMetricMetadataValueRef(len(a.values))
		a.valueRefs[m] = valueRef
		return valueRef
	}

	valueRef := nativeMetricMetadataDirectRefMask | nativeMetricMetadataValueRef(len(a.directValues))
	a.directValues = append(a.directValues, a.resolveMetadata(series, m))
	return valueRef
}

// resolveMetadata reuses immutable values still owned by the series before
// consulting the shared cache. The caller must not hold the series lock.
func (a *nativeMetricMetadataAppender) resolveMetadata(series *memSeries, m metadata.Metadata) *metadata.Metadata {
	var retained [maxNativeMetricMetadataVersions]*metadata.Metadata
	var count int
	series.Lock()
	if native := series.nativeMetadataLocked(); native != nil {
		retained[0] = native.Metadata
		count = 1
		for _, point := range native.Older {
			retained[count] = point.Metadata
			count++
		}
	}
	series.Unlock()

	// Pointers keep the immutable values alive across concurrent truncation or
	// deletion. Reusing a value does not reuse its observation's timestamp.
	for _, value := range retained[:count] {
		if *value == m {
			return value
		}
	}
	return a.valueCache.resolve(m, &a.strings)
}

// mayHaveObservedSeries reports whether ref may have a buffered observation
// in this transaction. False is definitive; true may be a false positive.
// With hundreds of distinct observed series most stripes are shared, so later
// unchanged samples are observed too. That costs time but never correctness.
// Tracking series exactly in a set was measured and is slower overall.
func (a *nativeMetricMetadataAppender) mayHaveObservedSeries(ref chunks.HeadSeriesRef) bool {
	stripe := uint8(uint64(ref) % nativeMetricMetadataStripes)
	first := a.stripeFirst[stripe]
	if first == 0 {
		return false
	}
	if a.observations[first-1].series.ref == ref {
		return true
	}
	// The first observed series differs from ref. If multiple series were
	// observed in this stripe, conservatively report a possible match rather
	// than walking its observation chain.
	return a.multiSeriesStripes[stripe/nativeMetricMetadataBitsPerWord]&(uint64(1)<<(stripe%nativeMetricMetadataBitsPerWord)) != 0
}

// observe buffers m for the series at effectiveFrom without updating committed
// metadata. Every call adds an observation, even when m is unchanged.
func (a *nativeMetricMetadataAppender) observe(s *memSeries, effectiveFrom int64, m metadata.Metadata) {
	var metadataRef nativeMetricMetadataValueRef
	if a.lastObservation != 0 && a.lastMetadata == m {
		metadataRef = a.observations[a.lastObservation-1].metadataRef
	} else {
		metadataRef = a.metadataReference(s, m)
	}

	stripe := uint8(uint64(s.ref) % nativeMetricMetadataStripes)
	observationRef := nativeMetricMetadataObservationRef(len(a.observations) + 1)
	a.observations = append(a.observations, nativeMetricMetadataObservation{
		series:        s,
		effectiveFrom: effectiveFrom,
		metadataRef:   metadataRef,
	})
	if a.stripeFirst[stripe] == 0 {
		a.stripeFirst[stripe] = observationRef
	} else if a.observations[a.stripeFirst[stripe]-1].series.ref != s.ref {
		a.multiSeriesStripes[stripe/nativeMetricMetadataBitsPerWord] |= uint64(1) << (stripe % nativeMetricMetadataBitsPerWord)
	}

	a.lastMetadata = m
	a.lastObservation = observationRef
}

// sortObservations orders the observation references by series and timestamp,
// then by append order so the last observation wins when equal timestamps are
// collapsed during merging. It sorts once per transaction.
func (a *nativeMetricMetadataAppender) sortObservations() {
	if a.sortedLength == len(a.observations) && len(a.sorted) == len(a.observations) {
		return
	}
	a.sorted = a.sorted[:0]
	for observationRef := range nativeMetricMetadataObservationRef(len(a.observations)) {
		a.sorted = append(a.sorted, observationRef+1)
	}
	compare := func(x, y nativeMetricMetadataObservationRef) int {
		left := a.observations[x-1]
		right := a.observations[y-1]
		if c := cmp.Compare(left.series.ref, right.series.ref); c != 0 {
			return c
		}
		if c := cmp.Compare(left.effectiveFrom, right.effectiveFrom); c != 0 {
			return c
		}
		return cmp.Compare(x, y)
	}
	if !slices.IsSortedFunc(a.sorted, compare) {
		slices.SortFunc(a.sorted, compare)
	}
	a.sortedLength = len(a.observations)
}

// groupEnd returns the end of the series group starting at position.
func (a *nativeMetricMetadataAppender) groupEnd(position int) int {
	end := position + 1
	ref := a.observations[a.sorted[position]-1].series.ref
	for end < len(a.sorted) && a.observations[a.sorted[end]-1].series.ref == ref {
		end++
	}
	return end
}

// appendWALRecord returns the contents of a compact record of one merge group
// entry per series whose observations the commit stability check finds
// unstable before logging, with the points commit merges: deduplicated per
// timestamp, the last observation winning. Points index the returned values,
// which hold each value reference used once, in order of first use: the
// caller's raw values, or owned values beyond the transaction's bounded set.
// The results are valid until the appender is reset. Without concurrent
// writers to a series, commit merges exactly these groups.
func (a *nativeMetricMetadataAppender) appendWALRecord() ([]record.NativeMetadataValue, []record.RefCompactNativeMetadata) {
	a.walValues, a.walEntries, a.walPoints, a.walSeries, a.walEpochs = a.walValues[:0], a.walEntries[:0], a.walPoints[:0], a.walSeries[:0], a.walEpochs[:0]
	clear(a.walRawIndices[:len(a.values)+1])
	a.walDirectIndices = slices.Grow(a.walDirectIndices[:0], len(a.directValues))[:len(a.directValues)]
	clear(a.walDirectIndices)
	a.logged = true
	a.sortObservations()
	for position := 0; position < len(a.sorted); {
		end := a.groupEnd(position)
		series := a.observations[a.sorted[position]-1].series
		series.Lock()
		stable := nativeMetricMetadataGroupStable(series.nativeMetadataLocked(), a, a.sorted[position:end])
		epoch := series.nativeMetadataEpochLocked()
		series.Unlock()
		if !stable {
			// Points hold value references until the group is reduced, so
			// that replaced points add no values.
			start := len(a.walPoints)
			for _, observationRef := range a.sorted[position:end] {
				observation := a.observations[observationRef-1]
				point := record.CompactNativeMetadataPoint{EffectiveFrom: observation.effectiveFrom, Value: uint32(observation.metadataRef)}
				if last := len(a.walPoints) - 1; last >= start && a.walPoints[last].EffectiveFrom == point.EffectiveFrom {
					a.walPoints[last] = point
				} else {
					a.walPoints = append(a.walPoints, point)
				}
			}
			for i := start; i < len(a.walPoints); i++ {
				a.walPoints[i].Value = a.walValueIndex(nativeMetricMetadataValueRef(a.walPoints[i].Value))
			}
			a.walEntries = append(a.walEntries, record.RefCompactNativeMetadata{
				Ref: series.ref, Kind: record.NativeMetadataGroup,
				Points: a.walPoints[start:len(a.walPoints):len(a.walPoints)],
			})
			a.walSeries = append(a.walSeries, series)
			a.walEpochs = append(a.walEpochs, epoch)
		}
		position = end
	}
	return a.walValues, a.walEntries
}

// walValueIndex returns the index of ref's value in the WAL record's values,
// adding the value on the reference's first use.
func (a *nativeMetricMetadataAppender) walValueIndex(ref nativeMetricMetadataValueRef) uint32 {
	var index *uint32
	if ref&nativeMetricMetadataDirectRefMask != 0 {
		index = &a.walDirectIndices[ref&^nativeMetricMetadataDirectRefMask]
	} else {
		index = &a.walRawIndices[ref]
	}
	if *index == 0 {
		m := a.metadataValue(ref)
		a.walValues = append(a.walValues, record.NativeMetadataValue{Type: record.GetMetricType(m.Type), Unit: m.Unit, Help: m.Help})
		*index = uint32(len(a.walValues))
	}
	return *index - 1
}

// selectBatch selects changing series groups and returns the next position.
// Stable groups count toward the limit; each comparison holds only its series
// lock. Skipping a stable group is its merge point.
func (a *nativeMetricMetadataAppender) selectBatch(store *nativeMetricMetadataStore, position int) int {
	a.groups = a.groups[:0]
	for examined := 0; position < len(a.sorted) && examined < maxNativeMetricMetadataBatch; examined++ {
		end := a.groupEnd(position)
		series := a.observations[a.sorted[position]-1].series
		emission := a.emission(series.ref)
		series.Lock()
		stable := nativeMetricMetadataGroupStable(series.nativeMetadataLocked(), a, a.sorted[position:end])
		if stable {
			a.mergePointLocked(store, nativeMetadataMergeSiteSelect, series, emission, false)
		}
		series.Unlock()
		if !stable {
			a.groups = append(a.groups, nativeMetricMetadataGroup{start: position, end: end, emission: emission})
		}
		position = end
	}
	return position
}

// emission returns the index of ref's WAL entry, or -1 if its group wasn't
// logged. Groups must be visited in ref order, once.
func (a *nativeMetricMetadataAppender) emission(ref chunks.HeadSeriesRef) int {
	if a.emitNext < len(a.walEntries) && a.walEntries[a.emitNext].Ref == ref {
		a.emitNext++
		return a.emitNext - 1
	}
	return -1
}

// mergePointLocked handles the merge point of series' group, emitted as WAL
// entry emission, or not if it is -1: its merge, or its skip as stable. The
// caller holds the series lock.
//
// An emitted group compares the epoch its pre-log pass read: if another merge,
// override or registration changed it since, this transaction's group may have
// merged out of WAL order, and its series needs an override. So does a group
// that merges without having been emitted. Emitted and merged groups then
// change the epoch, so that transactions merging later detect them in turn.
// Without the pre-log pass, nothing is persisted, and merge points change
// nothing.
func (a *nativeMetricMetadataAppender) mergePointLocked(store *nativeMetricMetadataStore, site nativeMetadataMergeSite, series *memSeries, emission int, merged bool) {
	if !a.logged {
		return
	}
	emitted := emission >= 0
	if !emitted && !merged {
		return
	}
	mismatch := !emitted || series.nativeMetadataEpochLocked() != a.walEpochs[emission]
	if mismatch {
		a.overrides = append(a.overrides, series)
	}
	series.bumpNativeMetadataEpochLocked()
	if hook := store.testMergePoint; hook != nil {
		hook(site, series.ref, mismatch)
	}
}

func (s *nativeMetricMetadataStore) getAppender() *nativeMetricMetadataAppender {
	if appender := s.appenderPool.Get(); appender != nil {
		a := appender.(*nativeMetricMetadataAppender)
		a.valueCache = s.values
		return a
	}
	return newNativeMetricMetadataAppender(s.values)
}

func (s *nativeMetricMetadataStore) putAppender(appender *nativeMetricMetadataAppender) {
	for _, observation := range appender.observations {
		appender.stripeFirst[uint64(observation.series.ref)%nativeMetricMetadataStripes] = 0
	}
	appender.multiSeriesStripes = [nativeMetricMetadataStripes / nativeMetricMetadataBitsPerWord]uint64{}
	clear(appender.observations)
	appender.observations = appender.observations[:0]
	clear(appender.values)
	appender.values = appender.values[:0]
	clear(appender.valueRefs)
	clear(appender.directValues)
	appender.directValues = appender.directValues[:0]
	appender.valueCache = nil
	if len(appender.strings) > nativeMetadataPooledStrings {
		appender.strings = nil
	} else {
		clear(appender.strings)
	}
	appender.sorted = appender.sorted[:0]
	appender.sortedLength = 0
	clear(appender.points[:cap(appender.points)])
	appender.points = appender.points[:0]
	appender.groups = appender.groups[:0]
	appender.lastMetadata = metadata.Metadata{}
	appender.lastObservation = 0
	clear(appender.walValues)
	appender.walValues = appender.walValues[:0]
	clear(appender.walEntries)
	appender.walEntries = appender.walEntries[:0]
	appender.walPoints = appender.walPoints[:0]
	clear(appender.walSeries)
	appender.walSeries = appender.walSeries[:0]
	appender.walEpochs = appender.walEpochs[:0]
	appender.walMayLand = false
	appender.logged = false
	appender.emitNext = 0
	clear(appender.overrides)
	appender.overrides = appender.overrides[:0]
	appender.overrideRecord.reset()
	s.appenderPool.Put(appender)
}

func (s *nativeMetricMetadataStore) commitAppender(appender *nativeMetricMetadataAppender) {
	appender.sortObservations()
	appender.emitNext = 0
	for position := 0; position < len(appender.sorted); {
		position = appender.selectBatch(s, position)
		if hook := s.testAfterSelectBatch; hook != nil {
			hook(appender)
		}
		s.commitBatch(appender)
	}
}

// commitBatch publishes complete per-series states. Head appenders retain their
// pending-sample reservations until after all batched accounting has finished.
// Immutable values are prepared without a series lock, then the group is
// rechecked and merged under that lock: concurrent commits may have changed the
// current point.
func (s *nativeMetricMetadataStore) commitBatch(appender *nativeMetricMetadataAppender) {
	var addedSeries, versionDelta int64
	var evicted uint64
	for _, group := range appender.groups {
		observationRefs := appender.sorted[group.start:group.end]
		series := appender.observations[observationRefs[0]-1].series
		appender.points = appender.points[:0]
		for _, observationRef := range observationRefs {
			observation := appender.observations[observationRef-1]
			point := nativeMetricMetadataPoint{
				EffectiveFrom: observation.effectiveFrom,
				Metadata:      appender.metadataPointer(series, observation.metadataRef),
			}
			if last := len(appender.points) - 1; last >= 0 && appender.points[last].EffectiveFrom == point.EffectiveFrom {
				appender.points[last] = point
			} else {
				appender.points = append(appender.points, point)
			}
		}

		series.Lock()
		native := series.nativeMetadataLocked()
		if series.isGCed() {
			series.Unlock()
			continue
		}
		if nativeMetricMetadataGroupStableResolved(native, appender.points) {
			appender.mergePointLocked(s, nativeMetadataMergeSiteCommit, series, group.emission, false)
			series.Unlock()
			continue
		}
		first := native == nil
		if first {
			native = series.initNativeMetadataLocked()
			addedSeries++
		}
		delta, evictions := native.mergeLocked(appender.points)
		appender.mergePointLocked(s, nativeMetadataMergeSiteMerge, series, group.emission, true)
		versionDelta += int64(delta)
		evicted += uint64(evictions)
		if first {
			s.publishLocked(series)
		}
		series.Unlock()
	}
	// Pending samples prevent deletion until these deltas are accounted for.
	// Avoid contended per-series atomic updates, especially at the version cap.
	if addedSeries != 0 {
		s.series.Add(addedSeries)
	}
	if versionDelta != 0 {
		s.versions.Add(versionDelta)
	}
	if evicted != 0 {
		s.evictions.Add(evicted)
	}
}

// nativeMetricMetadataGroupStable compares an entire series group against one
// committed state. The caller must hold the series lock throughout the check.
func nativeMetricMetadataGroupStable(native *nativeSeriesMetadata, appender *nativeMetricMetadataAppender, refs []nativeMetricMetadataObservationRef) bool {
	if native == nil {
		return false
	}
	for _, ref := range refs {
		observation := appender.observations[ref-1]
		if observation.effectiveFrom < native.EffectiveFrom || appender.metadataValue(observation.metadataRef) != *native.Metadata {
			return false
		}
	}
	return true
}

func nativeMetricMetadataGroupStableResolved(native *nativeSeriesMetadata, points []nativeMetricMetadataPoint) bool {
	if native == nil {
		return false
	}
	for _, point := range points {
		if point.EffectiveFrom < native.EffectiveFrom || !equalNativeMetricMetadata(point.Metadata, native.Metadata) {
			return false
		}
	}
	return true
}
