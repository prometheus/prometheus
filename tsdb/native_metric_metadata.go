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
	"context"
	"errors"
	"slices"
	"sync"
	"unique"

	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

const (
	nativeMetricMetadataStripes       = 256
	maxNativeMetricMetadataVersions   = 32
	maxNativeMetricMetadataValues     = 128 // Limits raw metadata retained by a transaction.
	maxNativeMetricMetadataBatch      = 256 // Limits series merged under one stripe lock.
	nativeMetricMetadataDirectRefMask = uint32(1 << 31)
)

// ErrNativeMetadataDisabled is returned when native metadata storage is not enabled.
var ErrNativeMetadataDisabled = errors.New("native metadata is disabled; enable with --enable-feature=native-metadata")

// NativeMetricMetadataVersion is a metric metadata change point in milliseconds.
type NativeMetricMetadataVersion struct {
	EffectiveFrom int64
	Metadata      metadata.Metadata
}

// NativeMetricMetadataSeries contains native metric metadata for one series.
type NativeMetricMetadataSeries struct {
	Labels    labels.Labels
	Versions  []NativeMetricMetadataVersion
	Truncated bool
}

type nativeMetricMetadataPoint struct {
	effectiveFrom int64
	metadata      unique.Handle[metadata.Metadata]
}

type nativeMetricMetadataHistory struct {
	versions  []nativeMetricMetadataPoint
	truncated bool
}

type nativeMetricMetadataStripe struct {
	mtx       sync.RWMutex
	histories map[chunks.HeadSeriesRef]nativeMetricMetadataHistory
}

type nativeMetricMetadataValue struct {
	metadata metadata.Metadata
	handle   unique.Handle[metadata.Metadata]
	resolved bool
}

type nativeMetricMetadataObservation struct {
	ref           chunks.HeadSeriesRef
	effectiveFrom int64
	metadataRef   uint32
	next          uint32
}

type nativeMetricMetadataAppender struct {
	observations    []nativeMetricMetadataObservation
	values          []nativeMetricMetadataValue
	valueRefs       map[metadata.Metadata]uint32
	directHandles   []unique.Handle[metadata.Metadata]
	sorted          []uint32
	points          []nativeMetricMetadataPoint
	groups          []nativeMetricMetadataGroup
	touched         []uint8
	stripeFirst     [nativeMetricMetadataStripes]uint32
	stripeLast      [nativeMetricMetadataStripes]uint32
	lastMetadata    metadata.Metadata
	lastObservation uint32
	haveLast        bool
}

func newNativeMetricMetadataAppender() *nativeMetricMetadataAppender {
	return &nativeMetricMetadataAppender{
		observations: make([]nativeMetricMetadataObservation, 0, nativeMetricMetadataStripes),
		values:       make([]nativeMetricMetadataValue, 0, maxNativeMetricMetadataValues),
		valueRefs:    make(map[metadata.Metadata]uint32, maxNativeMetricMetadataValues),
		groups:       make([]nativeMetricMetadataGroup, 0, maxNativeMetricMetadataBatch),
		touched:      make([]uint8, 0, nativeMetricMetadataStripes),
	}
}

func (a *nativeMetricMetadataAppender) metadataValue(ref uint32) metadata.Metadata {
	if ref&nativeMetricMetadataDirectRefMask != 0 {
		return a.directHandles[ref&^nativeMetricMetadataDirectRefMask].Value()
	}
	return a.values[ref-1].metadata
}

func (a *nativeMetricMetadataAppender) metadataHandle(ref uint32) unique.Handle[metadata.Metadata] {
	if ref&nativeMetricMetadataDirectRefMask != 0 {
		return a.directHandles[ref&^nativeMetricMetadataDirectRefMask]
	}
	value := &a.values[ref-1]
	if !value.resolved {
		value.handle = unique.Make(value.metadata)
		value.resolved = true
	}
	return value.handle
}

func (a *nativeMetricMetadataAppender) metadataReference(store *nativeMetricMetadataStore, ref chunks.HeadSeriesRef, m metadata.Metadata) uint32 {
	if valueRef, ok := a.valueRefs[m]; ok {
		return valueRef
	}
	if len(a.values) < maxNativeMetricMetadataValues {
		a.values = append(a.values, nativeMetricMetadataValue{metadata: m})
		valueRef := uint32(len(a.values))
		a.valueRefs[m] = valueRef
		return valueRef
	}

	// Reuse the committed handle for stable high-cardinality metadata. This
	// bounds raw transaction state without paying the interning cost again.
	stripe := store.stripe(ref)
	stripe.mtx.RLock()
	history, ok := stripe.histories[ref]
	if ok && len(history.versions) > 0 {
		handle := history.versions[len(history.versions)-1].metadata
		if handle.Value() == m {
			stripe.mtx.RUnlock()
			return a.appendDirectHandle(handle)
		}
	}
	stripe.mtx.RUnlock()
	return a.appendDirectHandle(unique.Make(m))
}

func (a *nativeMetricMetadataAppender) appendDirectHandle(handle unique.Handle[metadata.Metadata]) uint32 {
	ref := nativeMetricMetadataDirectRefMask | uint32(len(a.directHandles))
	if len(a.directHandles) == cap(a.directHandles) {
		newCapacity := max(16, 2*cap(a.directHandles))
		handles := make([]unique.Handle[metadata.Metadata], len(a.directHandles), newCapacity)
		copy(handles, a.directHandles)
		a.directHandles = handles
	}
	a.directHandles = append(a.directHandles, handle)
	return ref
}

func (a *nativeMetricMetadataAppender) appendObservation(ref chunks.HeadSeriesRef, effectiveFrom int64, metadataRef uint32) uint32 {
	stripe := uint8(uint64(ref) & (nativeMetricMetadataStripes - 1))
	observationRef := uint32(len(a.observations) + 1)
	if len(a.observations) == cap(a.observations) {
		observations := make([]nativeMetricMetadataObservation, len(a.observations), 2*cap(a.observations))
		copy(observations, a.observations)
		a.observations = observations
	}
	a.observations = append(a.observations, nativeMetricMetadataObservation{
		ref:           ref,
		effectiveFrom: effectiveFrom,
		metadataRef:   metadataRef,
	})
	if a.stripeFirst[stripe] == 0 {
		a.stripeFirst[stripe] = observationRef
		a.touched = append(a.touched, stripe)
	} else {
		a.observations[a.stripeLast[stripe]-1].next = observationRef
	}
	a.stripeLast[stripe] = observationRef
	return observationRef
}

func (a *nativeMetricMetadataAppender) observe(store *nativeMetricMetadataStore, ref chunks.HeadSeriesRef, effectiveFrom int64, m metadata.Metadata) {
	var metadataRef uint32
	if a.haveLast && a.lastMetadata == m {
		metadataRef = a.observations[a.lastObservation-1].metadataRef
	} else {
		metadataRef = a.metadataReference(store, ref, m)
	}

	observationRef := a.appendObservation(ref, effectiveFrom, metadataRef)
	a.lastMetadata = m
	a.lastObservation = observationRef
	a.haveLast = true
}

type nativeMetricMetadataStore struct {
	stripes      [nativeMetricMetadataStripes]nativeMetricMetadataStripe
	appenderPool sync.Pool
	series       atomic.Int64
	versions     atomic.Int64
	evictions    atomic.Uint64
}

func newNativeMetricMetadataStore() *nativeMetricMetadataStore {
	s := &nativeMetricMetadataStore{}
	for i := range s.stripes {
		s.stripes[i].histories = make(map[chunks.HeadSeriesRef]nativeMetricMetadataHistory)
	}
	return s
}

func canonicalMetricMetadata(m metadata.Metadata) metadata.Metadata {
	if m.Type == "" {
		m.Type = model.MetricTypeUnknown
	}
	return m
}

func (s *nativeMetricMetadataStore) stripe(ref chunks.HeadSeriesRef) *nativeMetricMetadataStripe {
	return &s.stripes[uint64(ref)&(nativeMetricMetadataStripes-1)]
}

func (s *nativeMetricMetadataStore) getAppender() *nativeMetricMetadataAppender {
	if appender := s.appenderPool.Get(); appender != nil {
		return appender.(*nativeMetricMetadataAppender)
	}
	return newNativeMetricMetadataAppender()
}

func (s *nativeMetricMetadataStore) putAppender(appender *nativeMetricMetadataAppender) {
	for _, stripe := range appender.touched {
		appender.stripeFirst[stripe] = 0
		appender.stripeLast[stripe] = 0
	}
	appender.observations = appender.observations[:0]
	clear(appender.values)
	appender.values = appender.values[:0]
	clear(appender.valueRefs)
	clear(appender.directHandles)
	appender.directHandles = appender.directHandles[:0]
	appender.sorted = appender.sorted[:0]
	clear(appender.points[:cap(appender.points)])
	appender.points = appender.points[:0]
	appender.groups = appender.groups[:0]
	appender.touched = appender.touched[:0]
	appender.lastMetadata = metadata.Metadata{}
	appender.lastObservation = 0
	appender.haveLast = false
	s.appenderPool.Put(appender)
}

func nativeMetricMetadataObservationStable(history nativeMetricMetadataHistory, appender *nativeMetricMetadataAppender, observation nativeMetricMetadataObservation) bool {
	if len(history.versions) == 0 {
		return false
	}
	last := history.versions[len(history.versions)-1]
	return observation.effectiveFrom >= last.effectiveFrom && appender.metadataValue(observation.metadataRef) == last.metadata.Value()
}

func nativeMetricMetadataGroupStable(stripe *nativeMetricMetadataStripe, appender *nativeMetricMetadataAppender, observationRefs []uint32) bool {
	first := appender.observations[observationRefs[0]-1]
	history, ok := stripe.histories[first.ref]
	if !ok {
		return false
	}
	for _, observationRef := range observationRefs {
		if !nativeMetricMetadataObservationStable(history, appender, appender.observations[observationRef-1]) {
			return false
		}
	}
	return true
}

func nativeMetricMetadataGroupStableResolved(stripe *nativeMetricMetadataStripe, appender *nativeMetricMetadataAppender, observationRefs []uint32) bool {
	first := appender.observations[observationRefs[0]-1]
	history, ok := stripe.histories[first.ref]
	if !ok || len(history.versions) == 0 {
		return false
	}
	last := history.versions[len(history.versions)-1]
	for _, observationRef := range observationRefs {
		observation := appender.observations[observationRef-1]
		if observation.effectiveFrom < last.effectiveFrom || appender.metadataHandle(observation.metadataRef) != last.metadata {
			return false
		}
	}
	return true
}

func nativeMetricMetadataStripeStable(stripe *nativeMetricMetadataStripe, appender *nativeMetricMetadataAppender, first uint32) bool {
	stripe.mtx.RLock()
	defer stripe.mtx.RUnlock()
	for observationRef := first; observationRef != 0; {
		observation := appender.observations[observationRef-1]
		history, ok := stripe.histories[observation.ref]
		if !ok || !nativeMetricMetadataObservationStable(history, appender, observation) {
			return false
		}
		observationRef = observation.next
	}
	return true
}

type nativeMetricMetadataGroup struct {
	start int
	end   int
}

func compareNativeMetricMetadataObservationRefs(appender *nativeMetricMetadataAppender, a, b uint32) int {
	left := appender.observations[a-1]
	right := appender.observations[b-1]
	switch {
	case left.ref < right.ref:
		return -1
	case left.ref > right.ref:
		return 1
	case left.effectiveFrom < right.effectiveFrom:
		return -1
	case left.effectiveFrom > right.effectiveFrom:
		return 1
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func (s *nativeMetricMetadataStore) commitAppender(appender *nativeMetricMetadataAppender) {
	for _, stripeIndex := range appender.touched {
		s.commitAppenderStripe(stripeIndex, appender)
	}
}

func (s *nativeMetricMetadataStore) commitAppenderStripe(stripeIndex uint8, appender *nativeMetricMetadataAppender) {
	stripe := &s.stripes[stripeIndex]
	first := appender.stripeFirst[stripeIndex]
	if nativeMetricMetadataStripeStable(stripe, appender, first) {
		return
	}

	appender.sorted = appender.sorted[:0]
	for observationRef := first; observationRef != 0; observationRef = appender.observations[observationRef-1].next {
		appender.sorted = append(appender.sorted, observationRef)
	}
	compare := func(a, b uint32) int {
		return compareNativeMetricMetadataObservationRefs(appender, a, b)
	}
	if !slices.IsSortedFunc(appender.sorted, compare) {
		slices.SortFunc(appender.sorted, compare)
	}

	for position := 0; position < len(appender.sorted); {
		appender.groups = appender.groups[:0]
		examinedGroups := 0
		stripe.mtx.RLock()
		for position < len(appender.sorted) && examinedGroups < maxNativeMetricMetadataBatch {
			end := position + 1
			ref := appender.observations[appender.sorted[position]-1].ref
			for end < len(appender.sorted) && appender.observations[appender.sorted[end]-1].ref == ref {
				end++
			}
			if !nativeMetricMetadataGroupStable(stripe, appender, appender.sorted[position:end]) {
				appender.groups = append(appender.groups, nativeMetricMetadataGroup{start: position, end: end})
			}
			examinedGroups++
			position = end
		}
		stripe.mtx.RUnlock()
		if len(appender.groups) == 0 {
			continue
		}

		for _, group := range appender.groups {
			for _, observationRef := range appender.sorted[group.start:group.end] {
				appender.metadataHandle(appender.observations[observationRef-1].metadataRef)
			}
		}

		stripe.mtx.Lock()
		for _, group := range appender.groups {
			observationRefs := appender.sorted[group.start:group.end]
			if nativeMetricMetadataGroupStableResolved(stripe, appender, observationRefs) {
				continue
			}
			appender.points = appender.points[:0]
			for _, observationRef := range observationRefs {
				observation := appender.observations[observationRef-1]
				point := nativeMetricMetadataPoint{
					effectiveFrom: observation.effectiveFrom,
					metadata:      appender.metadataHandle(observation.metadataRef),
				}
				if len(appender.points) > 0 && appender.points[len(appender.points)-1].effectiveFrom == point.effectiveFrom {
					appender.points[len(appender.points)-1] = point
					continue
				}
				appender.points = append(appender.points, point)
			}
			ref := appender.observations[observationRefs[0]-1].ref
			history, exists := stripe.histories[ref]
			s.mergeLocked(stripe, ref, history, exists, appender.points)
		}
		stripe.mtx.Unlock()
	}
}

func compareNativeMetricMetadataPoints(a, b nativeMetricMetadataPoint) int {
	switch {
	case a.effectiveFrom < b.effectiveFrom:
		return -1
	case a.effectiveFrom > b.effectiveFrom:
		return 1
	default:
		return 0
	}
}

func sortAndCompactNativeMetricMetadataObservations(observations []nativeMetricMetadataPoint) []nativeMetricMetadataPoint {
	if !slices.IsSortedFunc(observations, compareNativeMetricMetadataPoints) {
		slices.SortStableFunc(observations, compareNativeMetricMetadataPoints)
	}

	compacted := observations[:0]
	for _, observation := range observations {
		if len(compacted) > 0 && compacted[len(compacted)-1].effectiveFrom == observation.effectiveFrom {
			compacted[len(compacted)-1] = observation
			continue
		}
		compacted = append(compacted, observation)
	}
	clear(observations[len(compacted):])
	return compacted
}

func appendNativeMetricMetadataPoint(versions []nativeMetricMetadataPoint, point nativeMetricMetadataPoint) ([]nativeMetricMetadataPoint, bool) {
	if len(versions) == maxNativeMetricMetadataVersions {
		copy(versions, versions[1:])
		versions[len(versions)-1] = point
		return versions, true
	}

	if len(versions) == cap(versions) {
		newCap := 1
		if cap(versions) > 0 {
			newCap = min(2*cap(versions), maxNativeMetricMetadataVersions)
		}
		grown := make([]nativeMetricMetadataPoint, len(versions), newCap)
		copy(grown, versions)
		versions = grown
	}
	return append(versions, point), false
}

func mergeChronologicalNativeMetricMetadata(versions, observations []nativeMetricMetadataPoint) ([]nativeMetricMetadataPoint, int) {
	evictions := 0
	for _, observation := range observations {
		if len(versions) == 0 {
			versions, _ = appendNativeMetricMetadataPoint(versions, observation)
			continue
		}

		last := len(versions) - 1
		if observation.effectiveFrom == versions[last].effectiveFrom {
			versions[last] = observation
			if last > 0 && versions[last-1].metadata == observation.metadata {
				versions[last] = nativeMetricMetadataPoint{}
				versions = versions[:last]
			}
			continue
		}
		if versions[last].metadata == observation.metadata {
			continue
		}

		var evicted bool
		versions, evicted = appendNativeMetricMetadataPoint(versions, observation)
		if evicted {
			evictions++
		}
	}
	return versions, evictions
}

func mergeOverlappingNativeMetricMetadata(existing, observations []nativeMetricMetadataPoint) ([]nativeMetricMetadataPoint, int) {
	var retained [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
	start, count, evictions := 0, 0, 0
	var lastMetadata unique.Handle[metadata.Metadata]
	haveLastMetadata := false

	appendPoint := func(point nativeMetricMetadataPoint) {
		if haveLastMetadata && lastMetadata == point.metadata {
			return
		}
		haveLastMetadata = true
		lastMetadata = point.metadata

		if count < len(retained) {
			retained[(start+count)%len(retained)] = point
			count++
			return
		}
		retained[start] = point
		start = (start + 1) % len(retained)
		evictions++
	}

	for i, j := 0, 0; i < len(existing) || j < len(observations); {
		switch {
		case i == len(existing):
			appendPoint(observations[j])
			j++
		case j == len(observations):
			appendPoint(existing[i])
			i++
		case existing[i].effectiveFrom < observations[j].effectiveFrom:
			appendPoint(existing[i])
			i++
		case existing[i].effectiveFrom > observations[j].effectiveFrom:
			appendPoint(observations[j])
			j++
		default:
			appendPoint(observations[j])
			i++
			j++
		}
	}

	versions := make([]nativeMetricMetadataPoint, count)
	for i := range count {
		versions[i] = retained[(start+i)%len(retained)]
	}
	return versions, evictions
}

// merge applies observations in append order when timestamps are equal.
func (s *nativeMetricMetadataStore) merge(ref chunks.HeadSeriesRef, observations []nativeMetricMetadataPoint) {
	if len(observations) == 0 {
		return
	}
	observations = sortAndCompactNativeMetricMetadataObservations(observations)

	stripe := s.stripe(ref)
	stripe.mtx.Lock()
	history, exists := stripe.histories[ref]
	s.mergeLocked(stripe, ref, history, exists, observations)
	stripe.mtx.Unlock()
}

func (s *nativeMetricMetadataStore) mergeLocked(stripe *nativeMetricMetadataStripe, ref chunks.HeadSeriesRef, history nativeMetricMetadataHistory, exists bool, observations []nativeMetricMetadataPoint) {
	oldLen := len(history.versions)
	var evictions int
	if len(history.versions) == 0 || observations[0].effectiveFrom >= history.versions[len(history.versions)-1].effectiveFrom {
		history.versions, evictions = mergeChronologicalNativeMetricMetadata(history.versions, observations)
	} else {
		history.versions, evictions = mergeOverlappingNativeMetricMetadata(history.versions, observations)
	}
	if evictions > 0 {
		history.truncated = true
		s.evictions.Add(uint64(evictions))
	}

	stripe.histories[ref] = history
	if !exists {
		s.series.Add(1)
	}
	s.versions.Add(int64(len(history.versions) - oldLen))
}

func (s *nativeMetricMetadataStore) mergeOne(ref chunks.HeadSeriesRef, observation nativeMetricMetadataPoint) {
	stripe := s.stripe(ref)
	stripe.mtx.Lock()
	history, exists := stripe.histories[ref]
	if exists && len(history.versions) > 0 {
		last := history.versions[len(history.versions)-1]
		if observation.effectiveFrom >= last.effectiveFrom && observation.metadata == last.metadata {
			stripe.mtx.Unlock()
			return
		}
	}

	observations := [1]nativeMetricMetadataPoint{observation}
	s.mergeLocked(stripe, ref, history, exists, observations[:])
	stripe.mtx.Unlock()
}

func (s *nativeMetricMetadataStore) get(ref chunks.HeadSeriesRef) ([]NativeMetricMetadataVersion, bool, bool) {
	stripe := s.stripe(ref)
	stripe.mtx.RLock()
	history, ok := stripe.histories[ref]
	if !ok {
		stripe.mtx.RUnlock()
		return nil, false, false
	}
	versions := make([]NativeMetricMetadataVersion, len(history.versions))
	for i, version := range history.versions {
		versions[i] = NativeMetricMetadataVersion{
			EffectiveFrom: version.effectiveFrom,
			Metadata:      version.metadata.Value(),
		}
	}
	truncated := history.truncated
	stripe.mtx.RUnlock()
	return versions, truncated, true
}

func (s *nativeMetricMetadataStore) has(ref chunks.HeadSeriesRef) bool {
	stripe := s.stripe(ref)
	stripe.mtx.RLock()
	_, ok := stripe.histories[ref]
	stripe.mtx.RUnlock()
	return ok
}

func (s *nativeMetricMetadataStore) delete(refs map[storage.SeriesRef]struct{}) {
	var byStripe [nativeMetricMetadataStripes][]chunks.HeadSeriesRef
	for ref := range refs {
		headRef := chunks.HeadSeriesRef(ref)
		stripe := uint64(headRef) & (nativeMetricMetadataStripes - 1)
		byStripe[stripe] = append(byStripe[stripe], headRef)
	}
	for i := range nativeMetricMetadataStripes {
		stripeRefs := byStripe[i]
		if len(stripeRefs) == 0 {
			continue
		}
		stripe := &s.stripes[i]
		stripe.mtx.Lock()
		for _, ref := range stripeRefs {
			if history, ok := stripe.histories[ref]; ok {
				delete(stripe.histories, ref)
				s.series.Add(-1)
				s.versions.Add(-int64(len(history.versions)))
			}
		}
		stripe.mtx.Unlock()
	}
}

func (s *nativeMetricMetadataStore) reset() {
	for i := range s.stripes {
		stripe := &s.stripes[i]
		stripe.mtx.Lock()
		stripe.histories = make(map[chunks.HeadSeriesRef]nativeMetricMetadataHistory)
		stripe.mtx.Unlock()
	}
	s.series.Store(0)
	s.versions.Store(0)
}

type nativeMetricMetadataPostings struct {
	index.Postings
	store *nativeMetricMetadataStore
}

func (p *nativeMetricMetadataPostings) Next() bool {
	for p.Postings.Next() {
		if p.store.has(chunks.HeadSeriesRef(p.At())) {
			return true
		}
	}
	return false
}

func (p *nativeMetricMetadataPostings) Seek(ref storage.SeriesRef) bool {
	if !p.Postings.Seek(ref) {
		return false
	}
	if p.store.has(chunks.HeadSeriesRef(p.At())) {
		return true
	}
	return p.Next()
}

func (h *Head) nativeMetricMetadataForMatchers(ctx context.Context, matcherSets [][]*labels.Matcher, limit int) ([]NativeMetricMetadataSeries, bool, error) {
	if h.nativeMetricMetadata == nil {
		return nil, false, ErrNativeMetadataDisabled
	}

	reader, err := h.Index()
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()

	postings := make([]index.Postings, 0, len(matcherSets))
	for _, matchers := range matcherSets {
		p, err := PostingsForMatchers(ctx, reader, matchers...)
		if err != nil {
			return nil, false, err
		}
		postings = append(postings, p)
	}
	p := reader.SortedPostings(&nativeMetricMetadataPostings{
		Postings: index.Merge(ctx, postings...),
		store:    h.nativeMetricMetadata,
	})

	result := make([]NativeMetricMetadataSeries, 0)
	builder := labels.NewScratchBuilder(0)
	for p.Next() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		versions, historyTruncated, ok := h.nativeMetricMetadata.get(chunks.HeadSeriesRef(p.At()))
		if !ok {
			continue
		}
		if err := reader.Series(p.At(), &builder, nil); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, false, err
		}
		if limit > 0 && len(result) == limit {
			return result, true, nil
		}
		result = append(result, NativeMetricMetadataSeries{
			Labels:    builder.Labels(),
			Versions:  versions,
			Truncated: historyTruncated,
		})
	}
	if err := p.Err(); err != nil {
		return nil, false, err
	}
	return result, false, nil
}

// NativeMetricMetadata returns native metric metadata from the Head.
func (db *DB) NativeMetricMetadata(ctx context.Context, matcherSets [][]*labels.Matcher, limit int) ([]NativeMetricMetadataSeries, bool, error) {
	return db.head.nativeMetricMetadataForMatchers(ctx, matcherSets, limit)
}
