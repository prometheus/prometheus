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

import "sync"

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

// Delete removes all metadata for the series.
func (m *MemStore[V]) Delete(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.byHash, labelsHash)
}

// Iter calls the function for each series' current version.
func (m *MemStore[V]) Iter(f func(labelsHash uint64, version V) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		current, ok := entry.versioned.CurrentVersion()
		if ok {
			if err := f(hash, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterVersioned calls the function for each series' versioned data.
func (m *MemStore[V]) IterVersioned(f func(labelsHash uint64, versioned *Versioned[V]) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		if err := f(hash, entry.versioned); err != nil {
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
