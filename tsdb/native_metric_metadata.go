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
	nativeMetricMetadataStripes     = 256
	maxNativeMetricMetadataVersions = 32
	maxNativeMetricMetadataHandles  = 128 // Limits transient memory for high-cardinality metadata.
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

type nativeMetricMetadataAppender struct {
	pending       map[chunks.HeadSeriesRef]nativeMetricMetadataPending
	handles       map[metadata.Metadata]unique.Handle[metadata.Metadata]
	seenHandles   map[unique.Handle[metadata.Metadata]]metadata.Metadata
	cacheActive   bool
	cacheDisabled bool
}

func newNativeMetricMetadataAppender() *nativeMetricMetadataAppender {
	return &nativeMetricMetadataAppender{
		pending:     make(map[chunks.HeadSeriesRef]nativeMetricMetadataPending),
		seenHandles: make(map[unique.Handle[metadata.Metadata]]metadata.Metadata),
	}
}

func (a *nativeMetricMetadataAppender) handle(m metadata.Metadata) unique.Handle[metadata.Metadata] {
	if a.cacheActive {
		if handle, ok := a.handles[m]; ok {
			return handle
		}
	}

	// Build the value-keyed cache only after a repeated handle proves that the
	// transaction contains reusable metadata.
	handle := unique.Make(m)
	if a.cacheDisabled {
		return handle
	}
	if a.cacheActive {
		if len(a.handles) == maxNativeMetricMetadataHandles {
			clear(a.handles)
			a.cacheActive = false
			a.cacheDisabled = true
		} else {
			a.handles[m] = handle
		}
		return handle
	}

	if _, ok := a.seenHandles[handle]; ok {
		if a.handles == nil {
			a.handles = make(map[metadata.Metadata]unique.Handle[metadata.Metadata])
		}
		for seenHandle, seenMetadata := range a.seenHandles {
			a.handles[seenMetadata] = seenHandle
		}
		clear(a.seenHandles)
		a.cacheActive = true
		return handle
	}
	if len(a.seenHandles) == maxNativeMetricMetadataHandles {
		clear(a.seenHandles)
		a.cacheDisabled = true
		return handle
	}
	a.seenHandles[handle] = m
	return handle
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
	clear(appender.pending)
	clear(appender.handles)
	clear(appender.seenHandles)
	appender.cacheActive = false
	appender.cacheDisabled = false
	s.appenderPool.Put(appender)
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
