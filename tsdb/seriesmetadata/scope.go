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
	"maps"
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

// GetMinTime returns the minimum timestamp.
func (sv *ScopeVersion) GetMinTime() int64 { return sv.MinTime }

// GetMaxTime returns the maximum timestamp.
func (sv *ScopeVersion) GetMaxTime() int64 { return sv.MaxTime }

// SetMinTime sets the minimum timestamp.
func (sv *ScopeVersion) SetMinTime(t int64) { sv.MinTime = t }

// SetMaxTime sets the maximum timestamp.
func (sv *ScopeVersion) SetMaxTime(t int64) { sv.MaxTime = t }

// UpdateTimeRange extends the time range to include the given timestamps.
func (sv *ScopeVersion) UpdateTimeRange(minTime, maxTime int64) {
	if minTime < sv.MinTime {
		sv.MinTime = minTime
	}
	if maxTime > sv.MaxTime {
		sv.MaxTime = maxTime
	}
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

// VersionedScope is a type alias for the generic Versioned container for scopes.
type VersionedScope = Versioned[*ScopeVersion]

// NewVersionedScope creates a new VersionedScope with a single version.
func NewVersionedScope(version *ScopeVersion) *VersionedScope {
	return &VersionedScope{
		Versions: []*ScopeVersion{CopyScopeVersion(version)},
	}
}

// MergeVersionedScopes merges two VersionedScope instances for the same series.
func MergeVersionedScopes(a, b *VersionedScope) *VersionedScope {
	return MergeVersioned(ScopeOps, a, b)
}

// VersionedScopeReader provides read access to versioned scopes.
type VersionedScopeReader interface {
	GetVersionedScope(labelsHash uint64) (*VersionedScope, bool)
	IterVersionedScopes(ctx context.Context, f func(labelsHash uint64, scopes *VersionedScope) error) error
	TotalScopes() uint64
	TotalScopeVersions() uint64
}

// MemScopeStore is a type alias for the generic MemStore for scopes.
type MemScopeStore = MemStore[*ScopeVersion]

// NewMemScopeStore creates a new in-memory scope store.
func NewMemScopeStore() *MemScopeStore {
	return NewMemStore[*ScopeVersion](ScopeOps)
}
