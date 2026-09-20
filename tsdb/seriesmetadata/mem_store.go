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

package seriesmetadata

import (
	"context"
	"math"
	"sync"
)

// numMemStoreStripes is the number of shards in a MemStore.
// Must be a power of two for fast modulo via bitmask.
const numMemStoreStripes = 256

// versionedEntry stores versioned metadata for a single series.
//
// For single-version entries (>99% of series), the version is stored inline:
// canonical + contentHash hold the shared content, and minTime/maxTime hold
// the per-series time range. multi is nil. This avoids 3 heap allocations
// per series (Versioned, []V backing, ThinCopy) that the old layout required.
//
// For multi-version entries (<1% of series), multi is non-nil and holds the
// full Versioned with interned ThinCopies.
//
// Stored by value in the map to avoid one heap allocation per series.
// All mutations use read-modify-write: read from map, mutate local copy,
// write back to map.
type versionedEntry[V VersionConstraint] struct {
	labelsHash  uint64
	contentHash uint64        // cached content hash (single-version fast path)
	seriesRef   uint64        // series ref for LabelsForHash resolution
	canonical   V             // shared canonical content pointer (from contentStripe)
	minTime     int64         // inline time range (single-version only)
	maxTime     int64         // inline time range (single-version only)
	multi       *Versioned[V] // non-nil only when >1 version or dedup disabled
}

// memStoreStripe is a single shard of a MemStore. Each stripe has its own
// mutex and map to reduce lock contention across concurrent goroutines.
// The map stores versionedEntry by value (not pointer) to avoid one heap
// allocation per series — mutations use read-modify-write.
type memStoreStripe[V VersionConstraint] struct {
	mtx    sync.RWMutex
	byHash map[uint64]versionedEntry[V]
	_      [40]byte // cache-line padding to prevent false sharing
}

// contentStripe is a single shard of the content-addressed dedup table.
type contentStripe[V VersionConstraint] struct {
	mtx    sync.RWMutex
	byHash map[uint64]V
	_      [40]byte // cache-line padding to prevent false sharing
}

// MemStore is a generic in-memory store for versioned metadata keyed by labels hash.
// It uses 256-way sharding (matching the stripeSeries pattern in tsdb/head.go)
// to reduce lock contention under high concurrency.
//
// When the KindOps also implements ContentDedupOps, MemStore maintains a
// content-addressed table so that versions with identical content share
// map/slice pointers from a single canonical entry. Single-version entries
// (>99% of series) are stored inline without allocating Versioned/ThinCopy
// objects, reducing per-series memory from ~200B to ~0B of sub-allocations.
//
// It is safe for concurrent use.
type MemStore[V VersionConstraint] struct {
	stripes        [numMemStoreStripes]memStoreStripe[V]
	contentStripes [numMemStoreStripes]contentStripe[V]
	ops            KindOps[V]
	dedupOps       ContentDedupOps[V] // nil when ops doesn't implement ContentDedupOps
}

// NewMemStore creates a new generic in-memory store.
func NewMemStore[V VersionConstraint](ops KindOps[V]) *MemStore[V] {
	m := &MemStore[V]{ops: ops}
	for i := range m.stripes {
		m.stripes[i].byHash = make(map[uint64]versionedEntry[V])
	}
	if dedupOps, ok := any(ops).(ContentDedupOps[V]); ok {
		m.dedupOps = dedupOps
		for i := range m.contentStripes {
			m.contentStripes[i].byHash = make(map[uint64]V)
		}
	}
	return m
}

func (m *MemStore[V]) stripe(labelsHash uint64) *memStoreStripe[V] {
	return &m.stripes[labelsHash&uint64(numMemStoreStripes-1)]
}

// getOrCreateCanonical returns the canonical version for the given content hash.
// If no canonical exists yet, it deep-copies v via ops.Copy and stores it.
// When owned is true and a new canonical is created, v is stored directly
// (caller guarantees exclusive ownership), skipping the deep copy.
// Uses double-checked locking: RLock first, then Lock only on miss.
func (m *MemStore[V]) getOrCreateCanonical(hash uint64, v V, owned bool) V {
	cs := &m.contentStripes[hash&uint64(numMemStoreStripes-1)]

	cs.mtx.RLock()
	if canonical, ok := cs.byHash[hash]; ok {
		cs.mtx.RUnlock()
		return canonical
	}
	cs.mtx.RUnlock()

	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	if canonical, ok := cs.byHash[hash]; ok {
		return canonical
	}
	if owned {
		cs.byHash[hash] = v
		return v
	}
	canonical := m.ops.Copy(v)
	cs.byHash[hash] = canonical
	return canonical
}

// getCanonical returns the canonical version for the given content hash, if it exists.
func (m *MemStore[V]) getCanonical(hash uint64) (V, bool) {
	cs := &m.contentStripes[hash&uint64(numMemStoreStripes-1)]
	cs.mtx.RLock()
	canonical, ok := cs.byHash[hash]
	cs.mtx.RUnlock()
	return canonical, ok
}

// internVersions replaces deep-copied versions with thin copies sharing canonical
// map/slice pointers. No-op when dedupOps is nil.
func (m *MemStore[V]) internVersions(vs *Versioned[V]) {
	if m.dedupOps == nil {
		return
	}
	for i, v := range vs.Versions {
		hash := m.dedupOps.ContentHash(v)
		canonical := m.getOrCreateCanonical(hash, v, false)
		if !m.dedupOps.IsInterned(canonical, v) {
			vs.Versions[i] = m.dedupOps.ThinCopy(canonical, v)
		}
	}
}

// mergeVersionedInterned merges two Versioned instances and interns the result
// in a single pass, avoiding the intermediate deep copies that MergeVersioned +
// internVersions would create. Versions from input a (the existing multi) are
// reused directly since they are already-interned ThinCopies, skipping
// ContentHash + getOrCreateCanonical + ThinCopy for the common case.
// Only versions from input b (the new insertion) are interned.
// Falls back to MergeVersioned + internVersions when dedupOps is nil.
func (m *MemStore[V]) mergeVersionedInterned(a, b *Versioned[V]) *Versioned[V] {
	if m.dedupOps == nil {
		result := MergeVersioned(m.ops, a, b)
		m.internVersions(result)
		return result
	}
	if a == nil {
		result := b.Copy(m.ops)
		m.internVersions(result)
		return result
	}
	if b == nil {
		result := a.Copy(m.ops)
		m.internVersions(result)
		return result
	}

	merged := make([]V, 0, len(a.Versions)+len(b.Versions))
	lastIsReused := false

	// Two-pointer merge (both slices are sorted by MinTime).
	i, j := 0, 0
	for i < len(a.Versions) || j < len(b.Versions) {
		var ver V
		fromExisting := false
		switch {
		case i >= len(a.Versions):
			ver = b.Versions[j]
			j++
		case j >= len(b.Versions):
			ver = a.Versions[i]
			fromExisting = true
			i++
		case a.Versions[i].GetMinTime() <= b.Versions[j].GetMinTime():
			ver = a.Versions[i]
			fromExisting = true
			i++
		default:
			ver = b.Versions[j]
			j++
		}

		if len(merged) > 0 {
			last := merged[len(merged)-1]
			if m.ops.Equal(last, ver) && (last.GetMaxTime() == math.MaxInt64 || ver.GetMinTime() <= last.GetMaxTime()+1) {
				if lastIsReused {
					// Coalescing would mutate a reused input version.
					// Replace with a fresh ThinCopy before mutating.
					hash := m.dedupOps.ContentHash(last)
					canonical := m.getOrCreateCanonical(hash, last, false)
					fresh := m.dedupOps.ThinCopy(canonical, last)
					merged[len(merged)-1] = fresh
					lastIsReused = false
					last = fresh
				}
				if ver.GetMaxTime() > last.GetMaxTime() {
					last.SetMaxTime(ver.GetMaxTime())
				}
				if ver.GetMinTime() < last.GetMinTime() {
					last.SetMinTime(ver.GetMinTime())
				}
				continue
			}
		}

		if fromExisting {
			// Already-interned ThinCopy from input a — reuse directly,
			// skipping ContentHash + getOrCreateCanonical + ThinCopy.
			merged = append(merged, ver)
			lastIsReused = true
		} else {
			// Intern directly: get canonical and produce a ThinCopy, avoiding the
			// intermediate deep copy that ops.Copy would create.
			hash := m.dedupOps.ContentHash(ver)
			canonical := m.getOrCreateCanonical(hash, ver, false)
			if m.dedupOps.IsInterned(canonical, ver) {
				merged = append(merged, ver)
			} else {
				merged = append(merged, m.dedupOps.ThinCopy(canonical, ver))
			}
			lastIsReused = false
		}
	}

	return &Versioned[V]{Versions: merged}
}

// internLastVersion replaces only the last version with a thin copy.
// Returns the content hash of the last version (0 when dedup is disabled).
// Used after AddOrExtend appends a new version.
func (m *MemStore[V]) internLastVersion(vs *Versioned[V]) uint64 {
	if m.dedupOps == nil || len(vs.Versions) == 0 {
		return 0
	}
	last := len(vs.Versions) - 1
	v := vs.Versions[last]
	hash := m.dedupOps.ContentHash(v)
	canonical := m.getOrCreateCanonical(hash, v, false)
	if !m.dedupOps.IsInterned(canonical, v) {
		vs.Versions[last] = m.dedupOps.ThinCopy(canonical, v)
	}
	return hash
}

// materializeEntry creates a *Versioned[V] from an entry's current state.
// For multi-version entries, returns multi directly.
// For single-version entries, creates a ThinCopy with the inline time range.
func (m *MemStore[V]) materializeEntry(e versionedEntry[V]) *Versioned[V] {
	if e.multi != nil {
		return e.multi
	}
	thin := m.dedupOps.ThinCopy(e.canonical, e.canonical)
	thin.SetMinTime(e.minTime)
	thin.SetMaxTime(e.maxTime)
	return &Versioned[V]{Versions: []V{thin}}
}

// TotalCanonical returns the number of unique canonical entries in the content table.
func (m *MemStore[V]) TotalCanonical() int {
	if m.dedupOps == nil {
		return 0
	}
	var total int
	for i := range m.contentStripes {
		cs := &m.contentStripes[i]
		cs.mtx.RLock()
		total += len(cs.byHash)
		cs.mtx.RUnlock()
	}
	return total
}

// Len returns the number of unique series with metadata.
func (m *MemStore[V]) Len() int {
	var total int
	for i := range m.stripes {
		s := &m.stripes[i]
		s.mtx.RLock()
		total += len(s.byHash)
		s.mtx.RUnlock()
	}
	return total
}

// Get returns the current (latest) version for the series.
func (m *MemStore[V]) Get(labelsHash uint64) (V, bool) {
	s := m.stripe(labelsHash)
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	entry, ok := s.byHash[labelsHash]
	if !ok {
		var zero V
		return zero, false
	}
	if entry.multi != nil {
		if len(entry.multi.Versions) == 0 {
			var zero V
			return zero, false
		}
		return entry.multi.CurrentVersion()
	}
	// Single-version inline: materialize a ThinCopy.
	thin := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
	thin.SetMinTime(entry.minTime)
	thin.SetMaxTime(entry.maxTime)
	return thin, true
}

// GetVersioned returns all versions for the series.
func (m *MemStore[V]) GetVersioned(labelsHash uint64) (*Versioned[V], bool) {
	s := m.stripe(labelsHash)
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	entry, ok := s.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	return m.materializeEntry(entry), true
}

// GetAt returns the version active at the given timestamp.
func (m *MemStore[V]) GetAt(labelsHash uint64, timestamp int64) (V, bool) {
	s := m.stripe(labelsHash)
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	entry, ok := s.byHash[labelsHash]
	if !ok {
		var zero V
		return zero, false
	}
	if entry.multi != nil {
		return entry.multi.VersionAt(timestamp)
	}
	// Single-version inline: return if timestamp is in range or after maxTime.
	if timestamp >= entry.minTime {
		thin := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
		thin.SetMinTime(entry.minTime)
		thin.SetMaxTime(entry.maxTime)
		return thin, true
	}
	var zero V
	return zero, false
}

// Set stores a single version for the series.
// If data already exists, a new version is created if it differs,
// or the existing version's time range is extended if identical.
func (m *MemStore[V]) Set(labelsHash uint64, version V) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if entry, ok := s.byHash[labelsHash]; ok {
		if entry.multi != nil {
			prevLen := len(entry.multi.Versions)
			entry.multi.AddOrExtend(m.ops, version)
			if len(entry.multi.Versions) > prevLen {
				entry.contentHash = m.internLastVersion(entry.multi)
				s.byHash[labelsHash] = entry
			}
			return
		}
		// Single-version inline: compare content with canonical.
		if m.ops.Equal(entry.canonical, version) {
			if version.GetMinTime() < entry.minTime {
				entry.minTime = version.GetMinTime()
			}
			if version.GetMaxTime() > entry.maxTime {
				entry.maxTime = version.GetMaxTime()
			}
			s.byHash[labelsHash] = entry
			return
		}
		// Content differs: promote to multi.
		thin1 := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
		thin1.SetMinTime(entry.minTime)
		thin1.SetMaxTime(entry.maxTime)
		multi := &Versioned[V]{Versions: []V{thin1, m.ops.Copy(version)}}
		entry.contentHash = m.internLastVersion(multi)
		entry.multi = multi
		s.byHash[labelsHash] = entry
		return
	}

	if m.dedupOps != nil {
		// Dedup enabled: store inline.
		hash := m.dedupOps.ContentHash(version)
		canonical := m.getOrCreateCanonical(hash, version, false)
		s.byHash[labelsHash] = versionedEntry[V]{
			labelsHash:  labelsHash,
			contentHash: hash,
			canonical:   canonical,
			minTime:     version.GetMinTime(),
			maxTime:     version.GetMaxTime(),
		}
		return
	}

	s.byHash[labelsHash] = versionedEntry[V]{
		labelsHash: labelsHash,
		multi:      &Versioned[V]{Versions: []V{m.ops.Copy(version)}},
	}
}

// SetVersioned stores versioned data for the series.
// Used during compaction and loading from Parquet.
func (m *MemStore[V]) SetVersioned(labelsHash uint64, versioned *Versioned[V]) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if existing, ok := s.byHash[labelsHash]; ok {
		existingVersioned := m.materializeEntry(existing)
		merged := m.mergeVersionedInterned(existingVersioned, versioned)
		entry := existing
		m.setFromVersioned(&entry, merged)
		s.byHash[labelsHash] = entry
		return
	}

	entry := versionedEntry[V]{
		labelsHash: labelsHash,
	}
	copied := versioned.Copy(m.ops)
	m.internVersions(copied)
	m.setFromVersioned(&entry, copied)
	s.byHash[labelsHash] = entry
}

// setFromVersioned updates an entry from a Versioned. If the Versioned has
// exactly one version and dedup is enabled, stores inline. Otherwise stores as multi.
func (m *MemStore[V]) setFromVersioned(entry *versionedEntry[V], vs *Versioned[V]) {
	if m.dedupOps != nil && len(vs.Versions) == 1 {
		v := vs.Versions[0]
		hash := m.dedupOps.ContentHash(v)
		canonical := m.getOrCreateCanonical(hash, v, false)
		entry.contentHash = hash
		entry.canonical = canonical
		entry.minTime = v.GetMinTime()
		entry.maxTime = v.GetMaxTime()
		entry.multi = nil
		return
	}
	entry.multi = vs
	entry.contentHash = m.latestContentHash(vs)
}

// latestContentHash returns the content hash of the latest version in a Versioned.
func (m *MemStore[V]) latestContentHash(vs *Versioned[V]) uint64 {
	if m.dedupOps == nil || len(vs.Versions) == 0 {
		return 0
	}
	return m.dedupOps.ContentHash(vs.Versions[len(vs.Versions)-1])
}

// SetVersionedWithDiff atomically stores versioned data and returns the
// versioned state before and after the operation in a single lock acquisition.
// Returns (nil, new) for first insert, (old, merged) for merge.
func (m *MemStore[V]) SetVersionedWithDiff(labelsHash uint64, versioned *Versioned[V]) (old, cur *Versioned[V]) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if existing, ok := s.byHash[labelsHash]; ok {
		old = m.materializeEntry(existing)

		if existing.multi != nil {
			// Multi-version: fast path for single incoming that matches current.
			if len(versioned.Versions) == 1 && len(existing.multi.Versions) > 0 {
				current := existing.multi.Versions[len(existing.multi.Versions)-1]
				incoming := versioned.Versions[0]
				if m.ops.Equal(current, incoming) {
					current.UpdateTimeRange(incoming.GetMinTime(), incoming.GetMaxTime())
					return old, existing.multi
				}
			}
			existing.multi = m.mergeVersionedInterned(existing.multi, versioned)
			existing.contentHash = m.latestContentHash(existing.multi)
			s.byHash[labelsHash] = existing
			return old, existing.multi
		}

		// Single-version inline: check if incoming matches.
		if len(versioned.Versions) == 1 {
			incoming := versioned.Versions[0]
			if m.ops.Equal(existing.canonical, incoming) {
				if incoming.GetMinTime() < existing.minTime {
					existing.minTime = incoming.GetMinTime()
				}
				if incoming.GetMaxTime() > existing.maxTime {
					existing.maxTime = incoming.GetMaxTime()
				}
				s.byHash[labelsHash] = existing
				return old, m.materializeEntry(existing)
			}
		}

		// Content differs: promote to multi.
		existingVersioned := m.materializeEntry(existing)
		merged := m.mergeVersionedInterned(existingVersioned, versioned)
		entry := existing
		m.setFromVersioned(&entry, merged)
		s.byHash[labelsHash] = entry
		return old, m.materializeEntry(entry)
	}

	entry := versionedEntry[V]{
		labelsHash: labelsHash,
	}
	copied := versioned.Copy(m.ops)
	m.internVersions(copied)
	m.setFromVersioned(&entry, copied)
	s.byHash[labelsHash] = entry
	return nil, m.materializeEntry(entry)
}

// HasContentHash reports whether the series at labelsHash currently has a
// version with the given contentHash. Uses a read lock only — no allocations,
// no time-range extension. Designed for the ingester hot path to skip
// entriesToMap when content hasn't changed.
func (m *MemStore[V]) HasContentHash(labelsHash, contentHash uint64) bool {
	if m.dedupOps == nil {
		return false
	}
	s := m.stripe(labelsHash)
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	entry, ok := s.byHash[labelsHash]
	if !ok {
		return false
	}
	return entry.contentHash == contentHash
}

// InsertVersion inserts a version for labelsHash, using the content dedup table
// to avoid unnecessary deep copies. The buildFull callback is only invoked if
// no canonical with contentHash exists yet (first time seeing this content).
//
// Returns contentChanged=false when the existing entry's content matches and only
// the time range was extended (the >99% hot path — zero allocations).
// Returns contentChanged=true with old/cur materialized when content actually changed.
// For first insert: old is nil, cur is the new versioned state.
// For content change: old and cur are both materialized.
func (m *MemStore[V]) InsertVersion(
	labelsHash, contentHash uint64,
	minTime, maxTime int64,
	buildFull func() V,
	owned bool,
) (contentChanged bool, old, cur *Versioned[V]) {
	if m.dedupOps == nil {
		// No dedup — fall back to building the full version and using SetVersionedWithDiff.
		v := buildFull()
		old, cur = m.SetVersionedWithDiff(labelsHash, &Versioned[V]{Versions: []V{v}})
		contentChanged = old == nil || len(old.Versions) != len(cur.Versions)
		return contentChanged, old, cur
	}

	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if entry, ok := s.byHash[labelsHash]; ok {
		if entry.multi != nil {
			// Multi-version entry: check cached content hash.
			if len(entry.multi.Versions) > 0 {
				if entry.contentHash == contentHash {
					current := entry.multi.Versions[len(entry.multi.Versions)-1]
					current.UpdateTimeRange(minTime, maxTime)
					return false, nil, nil
				}
			}
			// Content differs in multi-version entry.
			old = entry.multi
			v := buildFull()
			incoming := &Versioned[V]{Versions: []V{v}}
			entry.multi = m.mergeVersionedInterned(entry.multi, incoming)
			entry.contentHash = m.latestContentHash(entry.multi)
			s.byHash[labelsHash] = entry
			return true, old, entry.multi
		}

		// Single-version inline: compare cached contentHash directly (zero-alloc fast path).
		if entry.contentHash == contentHash {
			if minTime < entry.minTime {
				entry.minTime = minTime
			}
			if maxTime > entry.maxTime {
				entry.maxTime = maxTime
			}
			s.byHash[labelsHash] = entry
			return false, nil, nil
		}

		// Content changed: promote from inline to multi.
		old = m.materializeEntry(entry)
		thin1 := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
		thin1.SetMinTime(entry.minTime)
		thin1.SetMaxTime(entry.maxTime)

		v := buildFull()
		canonical := m.getOrCreateCanonical(contentHash, v, owned)
		thin2 := m.dedupOps.ThinCopy(canonical, v)

		entry.multi = &Versioned[V]{Versions: []V{thin1, thin2}}
		entry.contentHash = contentHash
		s.byHash[labelsHash] = entry
		return true, old, entry.multi
	}

	// First insert: store inline.
	canonical, hasCanonical := m.getCanonical(contentHash)
	if !hasCanonical {
		// No canonical — must build full version and register it.
		full := buildFull()
		canonical = m.getOrCreateCanonical(contentHash, full, owned)
	}

	s.byHash[labelsHash] = versionedEntry[V]{
		labelsHash:  labelsHash,
		contentHash: contentHash,
		canonical:   canonical,
		minTime:     minTime,
		maxTime:     maxTime,
	}

	// Return cur wrapping the canonical directly — no ThinCopy needed since
	// callers only read content fields (not time ranges) from the returned version.
	cur = &Versioned[V]{Versions: []V{canonical}}
	return true, nil, cur
}

// InsertVersionWithRef is like InsertVersion but also sets the seriesRef on the
// entry in the same critical section, avoiding a separate SetSeriesRef call
// (which would require re-acquiring the stripe lock and re-looking-up the entry).
func (m *MemStore[V]) InsertVersionWithRef(
	labelsHash, contentHash uint64,
	minTime, maxTime int64,
	seriesRef uint64,
	buildFull func() V,
	owned bool,
) (contentChanged bool, old, cur *Versioned[V]) {
	if m.dedupOps == nil {
		v := buildFull()
		old, cur = m.SetVersionedWithDiff(labelsHash, &Versioned[V]{Versions: []V{v}})
		contentChanged = old == nil || len(old.Versions) != len(cur.Versions)
		// SetSeriesRef separately since SetVersionedWithDiff doesn't accept seriesRef.
		m.SetSeriesRef(labelsHash, seriesRef)
		return contentChanged, old, cur
	}

	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if entry, ok := s.byHash[labelsHash]; ok {
		entry.seriesRef = seriesRef

		if entry.multi != nil {
			if len(entry.multi.Versions) > 0 {
				if entry.contentHash == contentHash {
					current := entry.multi.Versions[len(entry.multi.Versions)-1]
					current.UpdateTimeRange(minTime, maxTime)
					s.byHash[labelsHash] = entry
					return false, nil, nil
				}
			}
			old = entry.multi
			v := buildFull()
			incoming := &Versioned[V]{Versions: []V{v}}
			entry.multi = m.mergeVersionedInterned(entry.multi, incoming)
			entry.contentHash = m.latestContentHash(entry.multi)
			s.byHash[labelsHash] = entry
			return true, old, entry.multi
		}

		if entry.contentHash == contentHash {
			if minTime < entry.minTime {
				entry.minTime = minTime
			}
			if maxTime > entry.maxTime {
				entry.maxTime = maxTime
			}
			s.byHash[labelsHash] = entry
			return false, nil, nil
		}

		old = m.materializeEntry(entry)
		thin1 := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
		thin1.SetMinTime(entry.minTime)
		thin1.SetMaxTime(entry.maxTime)

		v := buildFull()
		canonical := m.getOrCreateCanonical(contentHash, v, owned)
		thin2 := m.dedupOps.ThinCopy(canonical, v)

		entry.multi = &Versioned[V]{Versions: []V{thin1, thin2}}
		entry.contentHash = contentHash
		s.byHash[labelsHash] = entry
		return true, old, entry.multi
	}

	// First insert: store inline.
	canonical, hasCanonical := m.getCanonical(contentHash)
	if !hasCanonical {
		full := buildFull()
		canonical = m.getOrCreateCanonical(contentHash, full, owned)
	}

	s.byHash[labelsHash] = versionedEntry[V]{
		labelsHash:  labelsHash,
		contentHash: contentHash,
		canonical:   canonical,
		minTime:     minTime,
		maxTime:     maxTime,
		seriesRef:   seriesRef,
	}

	cur = &Versioned[V]{Versions: []V{canonical}}
	return true, nil, cur
}

// Delete removes all metadata for the series.
func (m *MemStore[V]) Delete(labelsHash uint64) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.byHash, labelsHash)
}

// SetSeriesRef stores the series ref for a given labels hash, enabling
// hash→ref lookup for LabelsForHash resolution without external stripe maps.
func (m *MemStore[V]) SetSeriesRef(labelsHash, ref uint64) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if entry, ok := s.byHash[labelsHash]; ok {
		entry.seriesRef = ref
		s.byHash[labelsHash] = entry
	}
}

// GetSeriesRef returns the series ref for a given labels hash.
func (m *MemStore[V]) GetSeriesRef(labelsHash uint64) (uint64, bool) {
	s := m.stripe(labelsHash)
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	entry, ok := s.byHash[labelsHash]
	if !ok {
		return 0, false
	}
	return entry.seriesRef, entry.seriesRef != 0
}

// checkContextEveryNIterations controls how often ctx.Err() is checked during iteration.
const checkContextEveryNIterations = 100

// snapshotEntries returns a copy of all entries across all stripes.
// Each stripe's read lock is held briefly while copying its entries, then released
// before moving to the next stripe. This avoids holding all locks simultaneously
// and prevents writer starvation on large stores.
// With value-type entries (56 bytes each), the snapshot is slightly larger than
// the old pointer-based approach but avoids heap allocation per entry.
func (m *MemStore[V]) snapshotEntries() []versionedEntry[V] {
	// Pre-count total entries to allocate once.
	var total int
	for i := range m.stripes {
		s := &m.stripes[i]
		s.mtx.RLock()
		total += len(s.byHash)
		s.mtx.RUnlock()
	}

	entries := make([]versionedEntry[V], 0, total)
	for i := range m.stripes {
		s := &m.stripes[i]
		s.mtx.RLock()
		for _, entry := range s.byHash {
			entries = append(entries, entry)
		}
		s.mtx.RUnlock()
	}
	return entries
}

// IterVersionedFlatInline is the primary iteration method. It avoids ThinCopy
// allocations for single-version entries (>99% of entries). For single-version
// entries, isInline is true and the caller should use inlineMinTime/inlineMaxTime
// instead of v.GetMinTime()/v.GetMaxTime(). The canonical is passed directly
// in the versions slice without materializing a ThinCopy.
// The versions slice passed to f must not be retained by the caller.
func (m *MemStore[V]) IterVersionedFlatInline(ctx context.Context, f func(labelsHash uint64, versions []V, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	snapshot := m.snapshotEntries()
	buf := make([]V, 1)
	for i, entry := range snapshot {
		if i%checkContextEveryNIterations == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if entry.multi != nil {
			if err := f(entry.labelsHash, entry.multi.Versions, 0, 0, false); err != nil {
				return err
			}
		} else {
			buf[0] = entry.canonical
			if err := f(entry.labelsHash, buf, entry.minTime, entry.maxTime, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterVersionedFlatInlineWithContentHash is like IterVersionedFlatInline but
// also passes the cached contentHash for single-version inline entries.
// For multi-version entries, contentHash is 0 (caller must recompute).
func (m *MemStore[V]) IterVersionedFlatInlineWithContentHash(ctx context.Context, f func(labelsHash uint64, versions []V, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error {
	snapshot := m.snapshotEntries()
	buf := make([]V, 1)
	for i, entry := range snapshot {
		if i%checkContextEveryNIterations == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if entry.multi != nil {
			if err := f(entry.labelsHash, entry.multi.Versions, 0, 0, false, entry.contentHash); err != nil {
				return err
			}
		} else {
			buf[0] = entry.canonical
			if err := f(entry.labelsHash, buf, entry.minTime, entry.maxTime, true, entry.contentHash); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterHashes calls the function for each series' labelsHash, without
// materializing versions. This is cheaper than iteration when only
// the hash is needed (e.g., building the needsResolve set in compaction).
func (m *MemStore[V]) IterHashes(ctx context.Context, f func(labelsHash uint64) error) error {
	snapshot := m.snapshotEntries()
	for i, entry := range snapshot {
		if i%checkContextEveryNIterations == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if err := f(entry.labelsHash); err != nil {
			return err
		}
	}
	return nil
}

// TotalEntries returns the count of series with metadata.
func (m *MemStore[V]) TotalEntries() uint64 {
	var total uint64
	for i := range m.stripes {
		s := &m.stripes[i]
		s.mtx.RLock()
		total += uint64(len(s.byHash))
		s.mtx.RUnlock()
	}
	return total
}

// TotalVersions returns the total count of all versions across all series.
func (m *MemStore[V]) TotalVersions() uint64 {
	var total uint64
	for i := range m.stripes {
		s := &m.stripes[i]
		s.mtx.RLock()
		for _, entry := range s.byHash {
			if entry.multi != nil {
				total += uint64(len(entry.multi.Versions))
			} else {
				total++
			}
		}
		s.mtx.RUnlock()
	}
	return total
}
