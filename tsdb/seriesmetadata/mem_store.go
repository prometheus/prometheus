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
type memStoreStripe[V VersionConstraint] struct {
	mtx    sync.RWMutex
	byHash map[uint64]*versionedEntry[V]
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
		m.stripes[i].byHash = make(map[uint64]*versionedEntry[V])
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
// Uses double-checked locking: RLock first, then Lock only on miss.
func (m *MemStore[V]) getOrCreateCanonical(hash uint64, v V) V {
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
		canonical := m.getOrCreateCanonical(hash, v)
		vs.Versions[i] = m.dedupOps.ThinCopy(canonical, v)
	}
}

// internLastVersion replaces only the last version with a thin copy.
// Used after AddOrExtend appends a new version.
func (m *MemStore[V]) internLastVersion(vs *Versioned[V]) {
	if m.dedupOps == nil || len(vs.Versions) == 0 {
		return
	}
	last := len(vs.Versions) - 1
	v := vs.Versions[last]
	hash := m.dedupOps.ContentHash(v)
	canonical := m.getOrCreateCanonical(hash, v)
	vs.Versions[last] = m.dedupOps.ThinCopy(canonical, v)
}

// InternVersion returns a thin copy of v sharing map/slice pointers from the
// canonical entry in the content table. Used for per-series interning from
// the head commit and WAL replay paths.
// Returns v unchanged when dedup is not enabled.
func (m *MemStore[V]) InternVersion(v V) V {
	if m.dedupOps == nil {
		return v
	}
	hash := m.dedupOps.ContentHash(v)
	canonical := m.getOrCreateCanonical(hash, v)
	return m.dedupOps.ThinCopy(canonical, v)
}

// materializeEntry creates a *Versioned[V] from an entry's current state.
// For multi-version entries, returns multi directly.
// For single-version entries, creates a ThinCopy with the inline time range.
// Callers must hold the stripe read or write lock.
func (m *MemStore[V]) materializeEntry(e *versionedEntry[V]) *Versioned[V] {
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
	// Single-version inline: check time range.
	if timestamp >= entry.minTime && timestamp <= entry.maxTime {
		thin := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
		thin.SetMinTime(entry.minTime)
		thin.SetMaxTime(entry.maxTime)
		return thin, true
	}
	if timestamp > entry.maxTime {
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

	if existing, ok := s.byHash[labelsHash]; ok {
		if existing.multi != nil {
			prevLen := len(existing.multi.Versions)
			existing.multi.AddOrExtend(m.ops, version)
			if len(existing.multi.Versions) > prevLen {
				m.internLastVersion(existing.multi)
			}
			return
		}
		// Single-version inline: compare content with canonical.
		if m.ops.Equal(existing.canonical, version) {
			if version.GetMinTime() < existing.minTime {
				existing.minTime = version.GetMinTime()
			}
			if version.GetMaxTime() > existing.maxTime {
				existing.maxTime = version.GetMaxTime()
			}
			return
		}
		// Content differs: promote to multi.
		thin1 := m.dedupOps.ThinCopy(existing.canonical, existing.canonical)
		thin1.SetMinTime(existing.minTime)
		thin1.SetMaxTime(existing.maxTime)
		multi := &Versioned[V]{Versions: []V{thin1, m.ops.Copy(version)}}
		m.internLastVersion(multi)
		existing.multi = multi
		return
	}

	if m.dedupOps != nil {
		// Dedup enabled: store inline.
		hash := m.dedupOps.ContentHash(version)
		canonical := m.getOrCreateCanonical(hash, version)
		entry := &versionedEntry[V]{
			labelsHash:  labelsHash,
			contentHash: hash,
			canonical:   canonical,
			minTime:     version.GetMinTime(),
			maxTime:     version.GetMaxTime(),
		}
		s.byHash[labelsHash] = entry
		return
	}

	entry := &versionedEntry[V]{
		labelsHash: labelsHash,
		multi:      &Versioned[V]{Versions: []V{m.ops.Copy(version)}},
	}
	s.byHash[labelsHash] = entry
}

// SetVersioned stores versioned data for the series.
// Used during compaction and loading from Parquet.
func (m *MemStore[V]) SetVersioned(labelsHash uint64, versioned *Versioned[V]) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if existing, ok := s.byHash[labelsHash]; ok {
		existingVersioned := m.materializeEntry(existing)
		merged := MergeVersioned(m.ops, existingVersioned, versioned)
		m.internVersions(merged)
		m.setFromVersioned(existing, merged)
		return
	}

	entry := &versionedEntry[V]{
		labelsHash: labelsHash,
	}
	copied := versioned.Copy(m.ops)
	m.internVersions(copied)
	m.setFromVersioned(entry, copied)
	s.byHash[labelsHash] = entry
}

// setFromVersioned updates an entry from a Versioned. If the Versioned has
// exactly one version and dedup is enabled, stores inline. Otherwise stores as multi.
func (m *MemStore[V]) setFromVersioned(entry *versionedEntry[V], vs *Versioned[V]) {
	if m.dedupOps != nil && len(vs.Versions) == 1 {
		v := vs.Versions[0]
		hash := m.dedupOps.ContentHash(v)
		canonical := m.getOrCreateCanonical(hash, v)
		entry.contentHash = hash
		entry.canonical = canonical
		entry.minTime = v.GetMinTime()
		entry.maxTime = v.GetMaxTime()
		entry.multi = nil
		return
	}
	entry.multi = vs
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
			existing.multi = MergeVersioned(m.ops, existing.multi, versioned)
			m.internVersions(existing.multi)
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
				return old, m.materializeEntry(existing)
			}
		}

		// Content differs: promote to multi.
		existingVersioned := m.materializeEntry(existing)
		merged := MergeVersioned(m.ops, existingVersioned, versioned)
		m.internVersions(merged)
		m.setFromVersioned(existing, merged)
		return old, m.materializeEntry(existing)
	}

	entry := &versionedEntry[V]{
		labelsHash: labelsHash,
	}
	copied := versioned.Copy(m.ops)
	m.internVersions(copied)
	m.setFromVersioned(entry, copied)
	s.byHash[labelsHash] = entry
	return nil, m.materializeEntry(entry)
}

// ExtendTimeRangeIfContentMatch checks whether the current (latest) version
// for labelsHash has the given contentHash. If so, it extends the time range
// and returns (existing versioned, true). Otherwise returns (existing or nil, false).
// This avoids allocating a full version object + maps.Clone when content is unchanged.
func (m *MemStore[V]) ExtendTimeRangeIfContentMatch(labelsHash, contentHash uint64, minTime, maxTime int64) (*Versioned[V], bool) {
	if m.dedupOps == nil {
		return nil, false
	}
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	entry, ok := s.byHash[labelsHash]
	if !ok {
		return nil, false
	}

	if entry.multi != nil {
		if len(entry.multi.Versions) == 0 {
			return nil, false
		}
		current := entry.multi.Versions[len(entry.multi.Versions)-1]
		if m.dedupOps.ContentHash(current) == contentHash {
			current.UpdateTimeRange(minTime, maxTime)
			return entry.multi, true
		}
		return entry.multi, false
	}

	// Single-version inline: compare cached contentHash directly.
	if entry.contentHash == contentHash {
		if minTime < entry.minTime {
			entry.minTime = minTime
		}
		if maxTime > entry.maxTime {
			entry.maxTime = maxTime
		}
		return m.materializeEntry(entry), true
	}
	return m.materializeEntry(entry), false
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

	if existing, ok := s.byHash[labelsHash]; ok {
		if existing.multi != nil {
			// Multi-version entry: check latest version's content hash.
			if len(existing.multi.Versions) > 0 {
				current := existing.multi.Versions[len(existing.multi.Versions)-1]
				if m.dedupOps.ContentHash(current) == contentHash {
					current.UpdateTimeRange(minTime, maxTime)
					return false, nil, nil
				}
			}
			// Content differs in multi-version entry.
			old = existing.multi
			v := buildFull()
			incoming := &Versioned[V]{Versions: []V{v}}
			existing.multi = MergeVersioned[V](m.ops, existing.multi, incoming)
			m.internVersions(existing.multi)
			return true, old, existing.multi
		}

		// Single-version inline: compare cached contentHash directly (zero-alloc fast path).
		if existing.contentHash == contentHash {
			if minTime < existing.minTime {
				existing.minTime = minTime
			}
			if maxTime > existing.maxTime {
				existing.maxTime = maxTime
			}
			return false, nil, nil
		}

		// Content changed: promote from inline to multi.
		old = m.materializeEntry(existing)
		thin1 := m.dedupOps.ThinCopy(existing.canonical, existing.canonical)
		thin1.SetMinTime(existing.minTime)
		thin1.SetMaxTime(existing.maxTime)

		v := buildFull()
		canonical := m.getOrCreateCanonical(contentHash, v)
		thin2 := m.dedupOps.ThinCopy(canonical, v)

		existing.multi = &Versioned[V]{Versions: []V{thin1, thin2}}
		return true, old, existing.multi
	}

	// First insert: store inline.
	canonical, hasCanonical := m.getCanonical(contentHash)
	if !hasCanonical {
		// No canonical — must build full version and register it.
		full := buildFull()
		canonical = m.getOrCreateCanonical(contentHash, full)
	}

	entry := &versionedEntry[V]{
		labelsHash:  labelsHash,
		contentHash: contentHash,
		canonical:   canonical,
		minTime:     minTime,
		maxTime:     maxTime,
	}
	s.byHash[labelsHash] = entry

	// Materialize cur for the caller (first insert needs attr index update).
	cur = m.materializeEntry(entry)
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

// snapshotEntries returns a shallow copy of all entries across all stripes.
// Each stripe's read lock is held briefly while copying its entries, then released
// before moving to the next stripe. This avoids holding all locks simultaneously
// and prevents writer starvation on large stores.
// The *versionedEntry pointers themselves are stable (not deleted, only
// replaced), and the Versioned inside is append-only, so a shallow
// snapshot is safe for read-only iteration.
func (m *MemStore[V]) snapshotEntries() []*versionedEntry[V] {
	// Pre-count total entries to allocate once.
	var total int
	for i := range m.stripes {
		s := &m.stripes[i]
		s.mtx.RLock()
		total += len(s.byHash)
		s.mtx.RUnlock()
	}

	entries := make([]*versionedEntry[V], 0, total)
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

// Iter calls the function for each series' current version.
// A snapshot of entries is taken under the lock, then iteration proceeds
// without holding the lock to avoid blocking writers for large stores.
func (m *MemStore[V]) Iter(ctx context.Context, f func(labelsHash uint64, version V) error) error {
	snapshot := m.snapshotEntries()
	for i, entry := range snapshot {
		if i%checkContextEveryNIterations == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if entry.multi != nil {
			current, ok := entry.multi.CurrentVersion()
			if ok {
				if err := f(entry.labelsHash, current); err != nil {
					return err
				}
			}
		} else {
			// Single-version inline: materialize a ThinCopy.
			thin := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
			thin.SetMinTime(entry.minTime)
			thin.SetMaxTime(entry.maxTime)
			if err := f(entry.labelsHash, thin); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterVersioned calls the function for each series' versioned data.
// A snapshot of entries is taken under the lock, then iteration proceeds
// without holding the lock to avoid blocking writers for large stores.
func (m *MemStore[V]) IterVersioned(ctx context.Context, f func(labelsHash uint64, versioned *Versioned[V]) error) error {
	snapshot := m.snapshotEntries()
	for i, entry := range snapshot {
		if i%checkContextEveryNIterations == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		vs := m.materializeEntry(entry)
		if err := f(entry.labelsHash, vs); err != nil {
			return err
		}
	}
	return nil
}

// IterVersionedFlat calls the function for each series' versions as a flat slice,
// avoiding the *Versioned wrapper allocation. For single-version entries (>99%),
// a reusable buf is used instead of allocating a Versioned{Versions: []V{thin}}.
// The versions slice passed to f must not be retained by the caller.
func (m *MemStore[V]) IterVersionedFlat(ctx context.Context, f func(labelsHash uint64, versions []V) error) error {
	snapshot := m.snapshotEntries()
	buf := make([]V, 1)
	for i, entry := range snapshot {
		if i%checkContextEveryNIterations == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if entry.multi != nil {
			if err := f(entry.labelsHash, entry.multi.Versions); err != nil {
				return err
			}
		} else {
			thin := m.dedupOps.ThinCopy(entry.canonical, entry.canonical)
			thin.SetMinTime(entry.minTime)
			thin.SetMaxTime(entry.maxTime)
			buf[0] = thin
			if err := f(entry.labelsHash, buf); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterHashes calls the function for each series' labelsHash, without
// materializing versions. This is cheaper than IterVersioned when only
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
