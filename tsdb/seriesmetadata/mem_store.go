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

// versionedEntry stores versioned metadata with a labels hash for indexing.
type versionedEntry[V VersionConstraint] struct {
	labelsHash uint64
	versioned  *Versioned[V]
}

// MemStore is a generic in-memory store for versioned metadata keyed by labels hash.
// It is safe for concurrent use.
type MemStore[V VersionConstraint] struct {
	byHash map[uint64]*versionedEntry[V]
	mtx    sync.RWMutex
	ops    KindOps[V]
}

// NewMemStore creates a new generic in-memory store.
func NewMemStore[V VersionConstraint](ops KindOps[V]) *MemStore[V] {
	return &MemStore[V]{
		byHash: make(map[uint64]*versionedEntry[V]),
		ops:    ops,
	}
}

// Len returns the number of unique series with metadata.
func (m *MemStore[V]) Len() int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return len(m.byHash)
}

// Get returns the current (latest) version for the series.
func (m *MemStore[V]) Get(labelsHash uint64) (V, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok || len(entry.versioned.Versions) == 0 {
		var zero V
		return zero, false
	}
	return entry.versioned.CurrentVersion()
}

// GetVersioned returns all versions for the series.
func (m *MemStore[V]) GetVersioned(labelsHash uint64) (*Versioned[V], bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.versioned, true
}

// GetAt returns the version active at the given timestamp.
func (m *MemStore[V]) GetAt(labelsHash uint64, timestamp int64) (V, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
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
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.versioned.AddOrExtend(m.ops, version)
		return
	}

	m.byHash[labelsHash] = &versionedEntry[V]{
		labelsHash: labelsHash,
		versioned:  &Versioned[V]{Versions: []V{m.ops.Copy(version)}},
	}
}

// SetVersioned stores versioned data for the series.
// Used during compaction and loading from Parquet.
func (m *MemStore[V]) SetVersioned(labelsHash uint64, versioned *Versioned[V]) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.versioned = MergeVersioned(m.ops, existing.versioned, versioned)
		return
	}

	m.byHash[labelsHash] = &versionedEntry[V]{
		labelsHash: labelsHash,
		versioned:  versioned.Copy(m.ops),
	}
}

// SetVersionedWithDiff atomically stores versioned data and returns the
// versioned state before and after the operation in a single lock acquisition.
// Returns (nil, new) for first insert, (old, merged) for merge.
func (m *MemStore[V]) SetVersionedWithDiff(labelsHash uint64, versioned *Versioned[V]) (old, cur *Versioned[V]) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		old = existing.versioned
		existing.versioned = MergeVersioned(m.ops, existing.versioned, versioned)
		return old, existing.versioned
	}

	entry := &versionedEntry[V]{
		labelsHash: labelsHash,
		versioned:  versioned.Copy(m.ops),
	}
	m.byHash[labelsHash] = entry
	return nil, entry.versioned
}

// Delete removes all metadata for the series.
func (m *MemStore[V]) Delete(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.byHash, labelsHash)
}

// checkContextEveryNIterations controls how often ctx.Err() is checked during iteration.
const checkContextEveryNIterations = 100

// snapshotEntries returns a shallow copy of all entries. The slice of
// pointers is taken under the read lock so iteration can proceed without
// holding the lock, avoiding writer starvation on large stores.
// The *versionedEntry pointers themselves are stable (not deleted, only
// replaced), and the Versioned inside is append-only, so a shallow
// snapshot is safe for read-only iteration.
func (m *MemStore[V]) snapshotEntries() []*versionedEntry[V] {
	m.mtx.RLock()
	entries := make([]*versionedEntry[V], 0, len(m.byHash))
	for _, entry := range m.byHash {
		entries = append(entries, entry)
	}
	m.mtx.RUnlock()
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
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byHash))
}

// TotalVersions returns the total count of all versions across all series.
func (m *MemStore[V]) TotalVersions() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	var total uint64
	for _, entry := range m.byHash {
		total += uint64(len(entry.versioned.Versions))
	}
	return total
}
