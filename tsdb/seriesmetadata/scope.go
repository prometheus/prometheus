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
	"maps"
	"sort"
	"sync"
)

// ScopeVersion represents a snapshot of OTel InstrumentationScope data at a point in time.
type ScopeVersion struct {
	Name      string
	Version   string
	SchemaURL string
	Attrs     map[string]string
	MinTime   int64
	MaxTime   int64
}

// NewScopeVersion creates a new ScopeVersion with deep-copied attributes.
func NewScopeVersion(name, version, schemaURL string, attrs map[string]string, minTime, maxTime int64) *ScopeVersion {
	attrsCopy := make(map[string]string, len(attrs))
	maps.Copy(attrsCopy, attrs)
	return &ScopeVersion{
		Name:      name,
		Version:   version,
		SchemaURL: schemaURL,
		Attrs:     attrsCopy,
		MinTime:   minTime,
		MaxTime:   maxTime,
	}
}

// CopyScopeVersion creates a deep copy of a ScopeVersion.
func CopyScopeVersion(sv *ScopeVersion) *ScopeVersion {
	var attrsCopy map[string]string
	if sv.Attrs != nil {
		attrsCopy = make(map[string]string, len(sv.Attrs))
		maps.Copy(attrsCopy, sv.Attrs)
	}
	return &ScopeVersion{
		Name:      sv.Name,
		Version:   sv.Version,
		SchemaURL: sv.SchemaURL,
		Attrs:     attrsCopy,
		MinTime:   sv.MinTime,
		MaxTime:   sv.MaxTime,
	}
}

// ScopeVersionsEqual compares two ScopeVersions for equality (ignoring time range).
func ScopeVersionsEqual(a, b *ScopeVersion) bool {
	if a.Name != b.Name {
		return false
	}
	if a.Version != b.Version {
		return false
	}
	if a.SchemaURL != b.SchemaURL {
		return false
	}
	return AttributesEqual(a.Attrs, b.Attrs)
}

// VersionedScope holds multiple versions of scope data for a single series.
type VersionedScope struct {
	// Versions ordered by MinTime ascending. Most recent version is last.
	Versions []*ScopeVersion
}

// Copy creates a deep copy of the VersionedScope.
func (vs *VersionedScope) Copy() *VersionedScope {
	versions := make([]*ScopeVersion, len(vs.Versions))
	for i, v := range vs.Versions {
		versions[i] = CopyScopeVersion(v)
	}
	return &VersionedScope{Versions: versions}
}

// NewVersionedScope creates a new VersionedScope with a single version.
func NewVersionedScope(version *ScopeVersion) *VersionedScope {
	return &VersionedScope{
		Versions: []*ScopeVersion{CopyScopeVersion(version)},
	}
}

// CurrentVersion returns the most recent version (last in the list).
func (vs *VersionedScope) CurrentVersion() *ScopeVersion {
	if len(vs.Versions) == 0 {
		return nil
	}
	return vs.Versions[len(vs.Versions)-1]
}

// VersionAt returns the version that was active at the given timestamp.
func (vs *VersionedScope) VersionAt(timestamp int64) *ScopeVersion {
	for i := len(vs.Versions) - 1; i >= 0; i-- {
		ver := vs.Versions[i]
		if timestamp >= ver.MinTime && timestamp <= ver.MaxTime {
			return ver
		}
		if timestamp > ver.MaxTime {
			return ver
		}
	}
	return nil
}

// AddOrExtend adds a new version if the scope changed, or extends the current version's
// time range if the scope is the same.
func (vs *VersionedScope) AddOrExtend(version *ScopeVersion) {
	if len(vs.Versions) == 0 {
		vs.Versions = []*ScopeVersion{CopyScopeVersion(version)}
		return
	}

	current := vs.Versions[len(vs.Versions)-1]
	if ScopeVersionsEqual(current, version) {
		if version.MinTime < current.MinTime {
			current.MinTime = version.MinTime
		}
		if version.MaxTime > current.MaxTime {
			current.MaxTime = version.MaxTime
		}
	} else {
		vs.Versions = append(vs.Versions, CopyScopeVersion(version))
	}
}

// MergeVersionedScopes merges two VersionedScope instances for the same series.
func MergeVersionedScopes(a, b *VersionedScope) *VersionedScope {
	if a == nil {
		return b.Copy()
	}
	if b == nil {
		return a.Copy()
	}

	all := make([]*ScopeVersion, 0, len(a.Versions)+len(b.Versions))
	all = append(all, a.Versions...)
	all = append(all, b.Versions...)

	sort.Slice(all, func(i, j int) bool {
		return all[i].MinTime < all[j].MinTime
	})

	merged := make([]*ScopeVersion, 0, len(all))
	for _, ver := range all {
		if len(merged) == 0 {
			merged = append(merged, CopyScopeVersion(ver))
			continue
		}

		last := merged[len(merged)-1]
		if ScopeVersionsEqual(last, ver) && ver.MinTime <= last.MaxTime+1 {
			if ver.MaxTime > last.MaxTime {
				last.MaxTime = ver.MaxTime
			}
			if ver.MinTime < last.MinTime {
				last.MinTime = ver.MinTime
			}
		} else {
			merged = append(merged, CopyScopeVersion(ver))
		}
	}

	return &VersionedScope{Versions: merged}
}

// VersionedScopeReader provides read access to versioned scopes.
type VersionedScopeReader interface {
	GetVersionedScope(labelsHash uint64) (*VersionedScope, bool)
	IterVersionedScopes(f func(labelsHash uint64, scopes *VersionedScope) error) error
	TotalScopes() uint64
	TotalScopeVersions() uint64
}

// versionedScopeEntry stores versioned scopes with labels hash for indexing.
type versionedScopeEntry struct {
	labelsHash uint64
	scopes     *VersionedScope
}

// MemScopeStore is an in-memory implementation of scope storage.
type MemScopeStore struct {
	byHash map[uint64]*versionedScopeEntry
	mtx    sync.RWMutex
}

// NewMemScopeStore creates a new in-memory scope store.
func NewMemScopeStore() *MemScopeStore {
	return &MemScopeStore{
		byHash: make(map[uint64]*versionedScopeEntry),
	}
}

// Len returns the number of unique series with scope data.
func (m *MemScopeStore) Len() int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return len(m.byHash)
}

// GetVersionedScope returns all versions of the scope for the series.
func (m *MemScopeStore) GetVersionedScope(labelsHash uint64) (*VersionedScope, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.scopes, true
}

// SetScope stores a scope version for the series.
func (m *MemScopeStore) SetScope(labelsHash uint64, scope *ScopeVersion) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.scopes.AddOrExtend(scope)
		return
	}

	m.byHash[labelsHash] = &versionedScopeEntry{
		labelsHash: labelsHash,
		scopes:     NewVersionedScope(scope),
	}
}

// SetVersionedScope stores versioned scopes for the series.
func (m *MemScopeStore) SetVersionedScope(labelsHash uint64, scopes *VersionedScope) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.scopes = MergeVersionedScopes(existing.scopes, scopes)
		return
	}

	m.byHash[labelsHash] = &versionedScopeEntry{
		labelsHash: labelsHash,
		scopes:     scopes.Copy(),
	}
}

// DeleteScope removes all scope data for the series.
func (m *MemScopeStore) DeleteScope(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.byHash, labelsHash)
}

// IterVersionedScopes calls the function for each series' versioned scopes.
func (m *MemScopeStore) IterVersionedScopes(f func(labelsHash uint64, scopes *VersionedScope) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		if err := f(hash, entry.scopes); err != nil {
			return err
		}
	}
	return nil
}

// TotalScopes returns the count of series with scopes.
func (m *MemScopeStore) TotalScopes() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byHash))
}

// TotalScopeVersions returns the total count of all scope versions.
func (m *MemScopeStore) TotalScopeVersions() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	var total uint64
	for _, entry := range m.byHash {
		total += uint64(len(entry.scopes.Versions))
	}
	return total
}
