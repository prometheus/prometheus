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
	// maxNativeMetricMetadataBytes bounds the combined type, unit and help text
	// of one metadata value.
	maxNativeMetricMetadataBytes = 8 << 10
)

// ErrNativeMetadataDisabled is returned when native metadata storage is not enabled.
var ErrNativeMetadataDisabled = errors.New("native metadata is disabled")

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
	metadata      *metadata.Metadata
}

// nativeMetricMetadataStore indexes series-owned native histories. Index entries
// may outlive their series until cleanup; they are not a substitute for
// revalidating membership in the Head.
type nativeMetricMetadataStore struct {
	// Keep the 64-bit atomics first for alignment on 32-bit platforms.
	// Stores are allocated individually by newNativeMetricMetadataStore.
	series    atomic.Int64
	versions  atomic.Int64
	evictions atomic.Uint64
	// oversized counts appended samples whose metadata exceeded the size limit.
	oversized atomic.Uint64
	// Directory identity changes invalidate query-local page caches.
	directoryGeneration atomic.Uint64

	// directory maps full-width page keys to singleton series or typed pages.
	// Membership implies initialized native state.
	directory      sync.Map
	directoryLocks [nativeMetadataDirectoryLocks]sync.Mutex
	appenderPool   sync.Pool
	values         *nativeMetricMetadataValueCache
}

func newNativeMetricMetadataStore() *nativeMetricMetadataStore {
	return &nativeMetricMetadataStore{
		values: newNativeMetricMetadataValueCache(),
	}
}

// fits reports whether m is small enough to retain.
func fits(m *metadata.Metadata) bool {
	// Compare against the remaining budget so that the sizes cannot overflow.
	return len(m.Type) <= maxNativeMetricMetadataBytes &&
		len(m.Unit) <= maxNativeMetricMetadataBytes-len(m.Type) &&
		len(m.Help) <= maxNativeMetricMetadataBytes-len(m.Type)-len(m.Unit)
}

// reset clears presence and current series/version counts after replacing Head
// series, while preserving cumulative evictions. The caller must exclude
// concurrent store mutations.
func (s *nativeMetricMetadataStore) reset() {
	s.directory.Clear()
	s.directoryGeneration.Inc()
	s.series.Store(0)
	s.versions.Store(0)
	s.values = newNativeMetricMetadataValueCache()
}

// nativeSeriesMetadata owns a series' committed history, with its newest point
// inline and up to four chronological older points. Adjacent values differ;
// Truncation remains set after an eviction, even if the history later collapses.
// The series lock protects updates.
type nativeSeriesMetadata struct {
	metadata      *metadata.Metadata
	effectiveFrom int64
	older         []nativeMetricMetadataPoint
	truncated     bool
}

// mergeLocked merges non-empty, strictly timestamp-ordered observations. It
// returns the change in retained versions and the number evicted. The caller
// holds the series lock. All input values must be immutable and independently owned.
func (n *nativeSeriesMetadata) mergeLocked(observations []nativeMetricMetadataPoint) (versionDelta, evictions int) {
	oldCount := 0
	if n.metadata != nil {
		oldCount = len(n.older) + 1
	}
	if oldCount == 0 || observations[0].effectiveFrom >= n.effectiveFrom {
		for _, observation := range observations {
			switch {
			case n.metadata == nil:
				n.effectiveFrom, n.metadata = observation.effectiveFrom, observation.metadata
			case observation.effectiveFrom == n.effectiveFrom:
				n.metadata = observation.metadata
				if last := len(n.older) - 1; last >= 0 && equalNativeMetricMetadata(n.older[last].metadata, n.metadata) {
					n.effectiveFrom = n.older[last].effectiveFrom
					n.older[last] = nativeMetricMetadataPoint{}
					n.older = n.older[:last]
				}
			case equalNativeMetricMetadata(observation.metadata, n.metadata):
				// A repeated value does not advance the change point.
			default:
				previous := nativeMetricMetadataPoint{effectiveFrom: n.effectiveFrom, metadata: n.metadata}
				if len(n.older) == maxNativeMetricMetadataVersions-1 {
					copy(n.older, n.older[1:])
					n.older[len(n.older)-1] = previous
					evictions++
				} else {
					n.older = append(ensureOlderCapacity(n.older, len(n.older)+1), previous)
				}
				n.effectiveFrom, n.metadata = observation.effectiveFrom, observation.metadata
			}
		}
	} else {
		var existing, retained [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
		count := copy(existing[:], n.older)
		existing[count] = nativeMetricMetadataPoint{effectiveFrom: n.effectiveFrom, metadata: n.metadata}
		var versions []nativeMetricMetadataPoint
		versions, evictions = mergeOverlappingNativeMetricMetadata(existing[:count+1], observations, retained[:0])
		olderCount := len(versions) - 1
		if len(n.older) > olderCount {
			clear(n.older[olderCount:])
			n.older = n.older[:olderCount]
		}
		n.older = ensureOlderCapacity(n.older, olderCount)[:olderCount]
		copy(n.older, versions[:olderCount])
		newest := versions[olderCount]
		n.effectiveFrom, n.metadata = newest.effectiveFrom, newest.metadata
	}
	if len(n.older) == 0 {
		n.older = nil
	}
	if evictions > 0 {
		n.truncated = true
	}
	return len(n.older) + 1 - oldCount, evictions
}

// ensureOlderCapacity returns older with room for at least need points, growing
// its capacity in powers of two so that short histories stay small.
func ensureOlderCapacity(older []nativeMetricMetadataPoint, need int) []nativeMetricMetadataPoint {
	if cap(older) >= need {
		return older
	}
	capacity := 1
	for capacity < need {
		capacity *= 2
	}
	grown := make([]nativeMetricMetadataPoint, len(older), capacity)
	copy(grown, older)
	return grown
}

// mergeOverlappingNativeMetricMetadata merges strictly timestamp-ordered inputs,
// preferring observations at equal timestamps and coalescing adjacent equal values.
// It leaves both inputs unchanged and appends retained points to versions,
// returning that slice and the number evicted by the version cap.
func mergeOverlappingNativeMetricMetadata(existing, observations, versions []nativeMetricMetadataPoint) ([]nativeMetricMetadataPoint, int) {
	var retained [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
	start, count, evictions := 0, 0, 0
	var lastMetadata *metadata.Metadata

	appendPoint := func(point nativeMetricMetadataPoint) {
		if equalNativeMetricMetadata(lastMetadata, point.metadata) {
			return
		}
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

	for i := range count {
		versions = append(versions, retained[(start+i)%len(retained)])
	}
	return versions, evictions
}

// nativeMetricMetadataSnapshot owns its copied points independently of the series.
type nativeMetricMetadataSnapshot struct {
	points    [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
	count     int
	truncated bool
}

// snapshotLocked copies the history. The caller holds the series lock.
func (n *nativeSeriesMetadata) snapshotLocked() nativeMetricMetadataSnapshot {
	snapshot := nativeMetricMetadataSnapshot{truncated: n.truncated}
	snapshot.count = copy(snapshot.points[:], n.older)
	snapshot.points[snapshot.count] = nativeMetricMetadataPoint{effectiveFrom: n.effectiveFrom, metadata: n.metadata}
	snapshot.count++
	return snapshot
}

func (s *nativeMetricMetadataSnapshot) expand() []NativeMetricMetadataVersion {
	versions := make([]NativeMetricMetadataVersion, s.count)
	for i, point := range s.points[:s.count] {
		versions[i] = NativeMetricMetadataVersion{EffectiveFrom: point.effectiveFrom, Metadata: *point.metadata}
	}
	return versions
}

type nativeMetricMetadataPostings struct {
	index.Postings
	store          *nativeMetricMetadataStore
	page           any
	pageKey        uint64
	pageGeneration uint64
	pageCached     bool
}

func (p *nativeMetricMetadataPostings) has(ref chunks.HeadSeriesRef) bool {
	key := uint64(ref) >> nativeMetadataPageBits
	// Capture before Load: a replacement racing with Load must invalidate the
	// captured value, not let it inherit the replacement's newer generation.
	generation := p.store.directoryGeneration.Load()
	if !p.pageCached || key != p.pageKey || generation != p.pageGeneration {
		p.page, _ = p.store.directory.Load(key)
		p.pageKey, p.pageGeneration, p.pageCached = key, generation, true
	}
	if page, ok := p.page.(*nativeMetricMetadataPage); ok && !page.dense {
		// The full page key is resolved above. Membership is only a prefilter;
		// nativeMetricMetadataForPostings revalidates the series in the Head.
		return page.contains(ref)
	}
	return nativeMetadataDirectorySeries(p.page, ref) != nil
}

func (p *nativeMetricMetadataPostings) Next() bool {
	for p.Postings.Next() {
		if p.has(chunks.HeadSeriesRef(p.At())) {
			return true
		}
	}
	return false
}

func (p *nativeMetricMetadataPostings) Seek(ref storage.SeriesRef) bool {
	if !p.Postings.Seek(ref) {
		return false
	}
	if p.has(chunks.HeadSeriesRef(p.At())) {
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
		series := h.series.getByID(ref)
		if series == nil {
			continue
		}
		// Head lookup releases its index lock before we take the series lock.
		// GC marks a deleted series under that lock, so this check revalidates
		// liveness while copying history and labels (including dedupelabels).
		series.Lock()
		native := series.nativeMetadataLocked()
		if native == nil || series.isGCed() {
			series.Unlock()
			continue
		}
		snapshot := native.snapshotLocked()
		lset := series.lset
		series.Unlock()
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
