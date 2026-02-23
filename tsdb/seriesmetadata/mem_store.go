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

// versionedEntry stores versioned metadata with a labels hash for indexing.
type versionedEntry[V VersionConstraint] struct {
	labelsHash uint64
	versioned  *Versioned[V]
}

// memStoreStripe is a single shard of a MemStore. Each stripe has its own
// mutex and map to reduce lock contention across concurrent goroutines.
type memStoreStripe[V VersionConstraint] struct {
	mtx    sync.RWMutex
	byHash map[uint64]*versionedEntry[V]
	_      [40]byte // cache-line padding to prevent false sharing
}

// MemStore is a generic in-memory store for versioned metadata keyed by labels hash.
// It uses 256-way sharding (matching the stripeSeries pattern in tsdb/head.go)
// to reduce lock contention under high concurrency.
// It is safe for concurrent use.
type MemStore[V VersionConstraint] struct {
	stripes [numMemStoreStripes]memStoreStripe[V]
	ops     KindOps[V]
}

// NewMemStore creates a new generic in-memory store.
func NewMemStore[V VersionConstraint](ops KindOps[V]) *MemStore[V] {
	m := &MemStore[V]{ops: ops}
	for i := range m.stripes {
		m.stripes[i].byHash = make(map[uint64]*versionedEntry[V])
	}
	return m
}

func (m *MemStore[V]) stripe(labelsHash uint64) *memStoreStripe[V] {
	return &m.stripes[labelsHash&uint64(numMemStoreStripes-1)]
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
	if !ok || len(entry.versioned.Versions) == 0 {
		var zero V
		return zero, false
	}
	return entry.versioned.CurrentVersion()
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
	return entry.versioned, true
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
	return entry.versioned.VersionAt(timestamp)
}

// Set stores a single version for the series.
// If data already exists, a new version is created if it differs,
// or the existing version's time range is extended if identical.
func (m *MemStore[V]) Set(labelsHash uint64, version V) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if existing, ok := s.byHash[labelsHash]; ok {
		existing.versioned.AddOrExtend(m.ops, version)
		return
	}

	s.byHash[labelsHash] = &versionedEntry[V]{
		labelsHash: labelsHash,
		versioned:  &Versioned[V]{Versions: []V{m.ops.Copy(version)}},
	}
}

// SetVersioned stores versioned data for the series.
// Used during compaction and loading from Parquet.
func (m *MemStore[V]) SetVersioned(labelsHash uint64, versioned *Versioned[V]) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if existing, ok := s.byHash[labelsHash]; ok {
		existing.versioned = MergeVersioned(m.ops, existing.versioned, versioned)
		return
	}

	s.byHash[labelsHash] = &versionedEntry[V]{
		labelsHash: labelsHash,
		versioned:  versioned.Copy(m.ops),
	}
}

// SetVersionedWithDiff atomically stores versioned data and returns the
// versioned state before and after the operation in a single lock acquisition.
// Returns (nil, new) for first insert, (old, merged) for merge.
func (m *MemStore[V]) SetVersionedWithDiff(labelsHash uint64, versioned *Versioned[V]) (old, cur *Versioned[V]) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if existing, ok := s.byHash[labelsHash]; ok {
		old = existing.versioned

		// Fast path: single incoming version that matches current — just extend time range.
		// This is the common case (~90% of commits) where resource/scope content hasn't changed.
		// Mirrors AddOrExtend (versioned.go) — zero allocations vs ~12 from MergeVersioned.
		if len(versioned.Versions) == 1 && len(existing.versioned.Versions) > 0 {
			current := existing.versioned.Versions[len(existing.versioned.Versions)-1]
			incoming := versioned.Versions[0]
			if m.ops.Equal(current, incoming) {
				current.UpdateTimeRange(incoming.GetMinTime(), incoming.GetMaxTime())
				return old, existing.versioned
			}
		}

		existing.versioned = MergeVersioned(m.ops, existing.versioned, versioned)
		return old, existing.versioned
	}

	entry := &versionedEntry[V]{
		labelsHash: labelsHash,
		versioned:  versioned.Copy(m.ops),
	}
	s.byHash[labelsHash] = entry
	return nil, entry.versioned
}

// Delete removes all metadata for the series.
func (m *MemStore[V]) Delete(labelsHash uint64) {
	s := m.stripe(labelsHash)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.byHash, labelsHash)
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
		current, ok := entry.versioned.CurrentVersion()
		if ok {
			if err := f(entry.labelsHash, current); err != nil {
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
		if err := f(entry.labelsHash, entry.versioned); err != nil {
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
			total += uint64(len(entry.versioned.Versions))
		}
		s.mtx.RUnlock()
	}
	return total
}
