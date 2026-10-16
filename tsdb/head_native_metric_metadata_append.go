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
// indexes into nativeMetricMetadataAppender.sorted.
type nativeMetricMetadataGroup struct {
	start int
	end   int
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
	// Observation references in commit order.
	sorted []nativeMetricMetadataObservationRef
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
		retained[0] = native.metadata
		count = 1
		for _, point := range native.older {
			retained[count] = point.metadata
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

// selectBatch selects changing series groups and returns the next position.
// Stable groups count toward the limit; each comparison holds only its series lock.
func (a *nativeMetricMetadataAppender) selectBatch(position int) int {
	a.groups = a.groups[:0]
	for examined := 0; position < len(a.sorted) && examined < maxNativeMetricMetadataBatch; examined++ {
		end := position + 1
		series := a.observations[a.sorted[position]-1].series
		for end < len(a.sorted) && a.observations[a.sorted[end]-1].series.ref == series.ref {
			end++
		}
		series.Lock()
		stable := nativeMetricMetadataGroupStable(series.nativeMetadataLocked(), a, a.sorted[position:end])
		series.Unlock()
		if !stable {
			a.groups = append(a.groups, nativeMetricMetadataGroup{start: position, end: end})
		}
		position = end
	}
	return position
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
	clear(appender.points[:cap(appender.points)])
	appender.points = appender.points[:0]
	appender.groups = appender.groups[:0]
	appender.lastMetadata = metadata.Metadata{}
	appender.lastObservation = 0
	s.appenderPool.Put(appender)
}

func (s *nativeMetricMetadataStore) commitAppender(appender *nativeMetricMetadataAppender) {
	appender.sorted = appender.sorted[:0]
	for observationRef := range nativeMetricMetadataObservationRef(len(appender.observations)) {
		appender.sorted = append(appender.sorted, observationRef+1)
	}
	// Order by series and timestamp, then by append order so the last
	// observation wins when equal timestamps are collapsed during merging.
	compare := func(a, b nativeMetricMetadataObservationRef) int {
		left := appender.observations[a-1]
		right := appender.observations[b-1]
		if c := cmp.Compare(left.series.ref, right.series.ref); c != 0 {
			return c
		}
		if c := cmp.Compare(left.effectiveFrom, right.effectiveFrom); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	}
	if !slices.IsSortedFunc(appender.sorted, compare) {
		slices.SortFunc(appender.sorted, compare)
	}

	for position := 0; position < len(appender.sorted); {
		position = appender.selectBatch(position)
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
				effectiveFrom: observation.effectiveFrom,
				metadata:      appender.metadataPointer(series, observation.metadataRef),
			}
			if last := len(appender.points) - 1; last >= 0 && appender.points[last].effectiveFrom == point.effectiveFrom {
				appender.points[last] = point
			} else {
				appender.points = append(appender.points, point)
			}
		}

		series.Lock()
		native := series.nativeMetadataLocked()
		if series.isGCed() || nativeMetricMetadataGroupStableResolved(native, appender.points) {
			series.Unlock()
			continue
		}
		first := native == nil
		if first {
			native = series.initNativeMetadataLocked()
			addedSeries++
		}
		delta, evictions := native.mergeLocked(appender.points)
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
		if observation.effectiveFrom < native.effectiveFrom || appender.metadataValue(observation.metadataRef) != *native.metadata {
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
		if point.effectiveFrom < native.effectiveFrom || !equalNativeMetricMetadata(point.metadata, native.metadata) {
			return false
		}
	}
	return true
}
