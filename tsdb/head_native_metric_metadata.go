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
	"sync"
	"unique"

	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

const (
	nativeMetricMetadataStripes     = 256
	maxNativeMetricMetadataVersions = 5
)

// ErrNativeMetadataDisabled is returned when native metadata storage is not enabled.
var ErrNativeMetadataDisabled = errors.New("native metadata is disabled; enable with --enable-feature=native-metadata")

// NativeMetricMetadataVersion is a metric metadata change point in milliseconds.
type NativeMetricMetadataVersion struct {
	EffectiveFrom int64
	Metadata      metadata.Metadata
}

// NativeMetricMetadataSeries contains native metric metadata for one series.
// Labels are immutable; callers must not modify them.
type NativeMetricMetadataSeries struct {
	Labels labels.Labels
	// Versions contains change points ordered by increasing EffectiveFrom.
	Versions []NativeMetricMetadataVersion
	// Truncated reports whether the per-series cap has ever evicted a version.
	Truncated bool
}

type nativeMetricMetadataPoint struct {
	effectiveFrom int64
	metadata      unique.Handle[metadata.Metadata]
}

// nativeMetricMetadataHistory retains change points with strictly increasing
// timestamps and distinct adjacent metadata values, capped at
// maxNativeMetricMetadataVersions. Once set by a cap eviction, truncated stays set.
type nativeMetricMetadataHistory struct {
	versions  []nativeMetricMetadataPoint
	truncated bool
}

// nativeMetricMetadataStripe holds committed metadata histories for a subset
// of series. Its mutex protects both the histories map and its contents.
type nativeMetricMetadataStripe struct {
	mtx       sync.RWMutex
	histories map[chunks.HeadSeriesRef]nativeMetricMetadataHistory
}

// nativeSeriesMetadata caches committed metadata for append-time comparisons.
// It can lag the history store until publication. The series lock protects the
// cache; its immutable pointee is shared within the publishing transaction.
type nativeSeriesMetadata struct {
	metadata      *metadata.Metadata
	effectiveFrom int64
}

// nativeMetricMetadataStore holds a Head's committed, in-memory metadata
// histories, keyed by series reference. It is shared across appenders and
// queries; each stripe's lock protects its histories.
type nativeMetricMetadataStore struct {
	// Keep the 64-bit atomics first for alignment on 32-bit platforms.
	// Stores are allocated individually by newNativeMetricMetadataStore.
	series    atomic.Int64
	versions  atomic.Int64
	evictions atomic.Uint64

	stripes      [nativeMetricMetadataStripes]nativeMetricMetadataStripe
	appenderPool sync.Pool
}

func newNativeMetricMetadataStore() *nativeMetricMetadataStore {
	s := &nativeMetricMetadataStore{}
	for i := range s.stripes {
		s.stripes[i].histories = make(map[chunks.HeadSeriesRef]nativeMetricMetadataHistory)
	}
	return s
}

func (s *nativeMetricMetadataStore) stripe(ref chunks.HeadSeriesRef) *nativeMetricMetadataStripe {
	return &s.stripes[uint64(ref)%nativeMetricMetadataStripes]
}

// snapshot takes the stripe read lock and copies ref's history into snapshot.
// It reports whether the history exists, leaving snapshot unchanged on a miss.
func (s *nativeMetricMetadataStore) snapshot(ref chunks.HeadSeriesRef, snapshot *nativeMetricMetadataSnapshot) bool {
	stripe := s.stripe(ref)
	stripe.mtx.RLock()
	history, ok := stripe.histories[ref]
	if ok {
		snapshot.count = copy(snapshot.points[:], history.versions)
		snapshot.truncated = history.truncated
	}
	stripe.mtx.RUnlock()
	return ok
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
		stripe := uint64(headRef) % nativeMetricMetadataStripes
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

// reset clears histories and current series/version counts when rebuilding Head
// state, while preserving cumulative evictions. The caller must exclude
// concurrent store mutations.
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

// mergeChronologicalNativeMetricMetadata merges strictly timestamp-ordered inputs.
// Observations must be at or after the newest existing version, if any. Incoming
// values win timestamp ties, and adjacent equal metadata values coalesce.
// It may mutate versions' backing array and returns retained points and the
// number evicted by the version cap.
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

// mergeOverlappingNativeMetricMetadata merges strictly timestamp-ordered inputs,
// preferring observations at equal timestamps and coalescing adjacent equal values.
// It leaves both inputs unchanged and returns a separate slice of the newest
// retained points and the number evicted by the version cap.
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

// mergeNativeMetricMetadataLocked merges observations into the stored history
// for ref. It returns the newest retained point (which may come from history),
// the net change in retained versions, and the count evicted by the version cap.
//
// Observations must be non-empty and ordered by strictly increasing effectiveFrom.
// The caller supplies history from its lookup of stripe.histories[ref] to avoid
// a second map lookup. It must hold the stripe write lock from that lookup
// until the returned accounting deltas have been applied.
func mergeNativeMetricMetadataLocked(stripe *nativeMetricMetadataStripe, ref chunks.HeadSeriesRef, history nativeMetricMetadataHistory, observations []nativeMetricMetadataPoint) (newest nativeMetricMetadataPoint, versionDelta, evictions int) {
	oldLen := len(history.versions)
	if len(history.versions) == 0 || observations[0].effectiveFrom >= history.versions[len(history.versions)-1].effectiveFrom {
		history.versions, evictions = mergeChronologicalNativeMetricMetadata(history.versions, observations)
	} else {
		history.versions, evictions = mergeOverlappingNativeMetricMetadata(history.versions, observations)
	}
	if evictions > 0 {
		history.truncated = true
	}

	stripe.histories[ref] = history
	return history.versions[len(history.versions)-1], len(history.versions) - oldLen, evictions
}

// nativeMetricMetadataSnapshot owns its points independently of the store lock.
type nativeMetricMetadataSnapshot struct {
	points    [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
	count     int
	truncated bool
}

func (s *nativeMetricMetadataSnapshot) expand() []NativeMetricMetadataVersion {
	versions := make([]NativeMetricMetadataVersion, s.count)
	for i, point := range s.points[:s.count] {
		versions[i] = NativeMetricMetadataVersion{EffectiveFrom: point.effectiveFrom, Metadata: point.metadata.Value()}
	}
	return versions
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
	// Sorting materialises every matching series before iteration, so limit
	// bounds the response and the per-series decode below but not this. That is
	// deliberate: bounding before the sort would return an arbitrary subset
	// rather than the first by label order.
	p := reader.SortedPostings(&nativeMetricMetadataPostings{
		Postings: index.Merge(ctx, postings...),
		store:    h.nativeMetricMetadata,
	})

	return h.nativeMetricMetadataForPostings(ctx, p, limit)
}

// nativeMetricMetadataForPostings consumes label-sorted references, not retained
// series pointers.
func (h *Head) nativeMetricMetadataForPostings(ctx context.Context, p index.Postings, limit int) ([]NativeMetricMetadataSeries, bool, error) {
	result := make([]NativeMetricMetadataSeries, 0)
	for p.Next() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		ref := chunks.HeadSeriesRef(p.At())
		var snapshot nativeMetricMetadataSnapshot
		if !h.nativeMetricMetadata.snapshot(ref, &snapshot) {
			continue
		}
		// Revalidate Head membership after reading metadata. labels takes the
		// series lock only in builds whose label representation can change.
		series := h.series.getByID(ref)
		if series == nil {
			continue
		}
		lset := series.labels()
		// Only a live additional row proves truncation. Do not expand its metadata.
		if limit > 0 && len(result) == limit {
			return result, true, nil
		}
		result = append(result, NativeMetricMetadataSeries{
			Labels:    lset,
			Versions:  snapshot.expand(),
			Truncated: snapshot.truncated,
		})
	}
	if err := p.Err(); err != nil {
		return nil, false, err
	}
	return result, false, nil
}

// NativeMetricMetadata returns native metric metadata from the Head in label order.
// Matcher sets are ORed, with matchers within each set ANDed. Empty sets contribute
// no matches; no sets yields no results. Matcher slices may be reordered.
//
// A non-positive limit is unlimited. The boolean reports result truncation by
// the series limit, independently of per-series history truncation.
func (db *DB) NativeMetricMetadata(ctx context.Context, matcherSets [][]*labels.Matcher, limit int) ([]NativeMetricMetadataSeries, bool, error) {
	return db.head.nativeMetricMetadataForMatchers(ctx, matcherSets, limit)
}
