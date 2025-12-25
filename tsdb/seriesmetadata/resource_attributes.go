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

// ResourceAttributes represents OTel resource attributes for a time series.
type ResourceAttributes struct {
	// ServiceName is the service.name identifying attribute.
	ServiceName string
	// ServiceNamespace is the service.namespace identifying attribute.
	ServiceNamespace string
	// ServiceInstanceID is the service.instance.id identifying attribute.
	ServiceInstanceID string

	// Attributes contains all resource attributes as key-value pairs.
	// This includes the identifying attributes above for completeness.
	Attributes map[string]string

	// MinTime is the minimum timestamp (in milliseconds) when these attributes were observed.
	MinTime int64
	// MaxTime is the maximum timestamp (in milliseconds) when these attributes were observed.
	MaxTime int64
}

// NewResourceAttributes creates a new ResourceAttributes from a map of attributes.
// It automatically extracts and sets the identifying attributes.
func NewResourceAttributes(attrs map[string]string, minTime, maxTime int64) *ResourceAttributes {
	ra := &ResourceAttributes{
		Attributes: make(map[string]string, len(attrs)),
		MinTime:    minTime,
		MaxTime:    maxTime,
	}

	for k, v := range attrs {
		ra.Attributes[k] = v
		switch k {
		case AttrServiceName:
			ra.ServiceName = v
		case AttrServiceNamespace:
			ra.ServiceNamespace = v
		case AttrServiceInstanceID:
			ra.ServiceInstanceID = v
		}
	}

	return ra
}

// IdentifyingAttributes returns only the identifying attributes.
func (ra *ResourceAttributes) IdentifyingAttributes() map[string]string {
	result := make(map[string]string)
	if ra.ServiceName != "" {
		result[AttrServiceName] = ra.ServiceName
	}
	if ra.ServiceNamespace != "" {
		result[AttrServiceNamespace] = ra.ServiceNamespace
	}
	if ra.ServiceInstanceID != "" {
		result[AttrServiceInstanceID] = ra.ServiceInstanceID
	}
	return result
}

// NonIdentifyingAttributes returns attributes excluding the identifying ones.
func (ra *ResourceAttributes) NonIdentifyingAttributes() map[string]string {
	result := make(map[string]string)
	for k, v := range ra.Attributes {
		switch k {
		case AttrServiceName, AttrServiceNamespace, AttrServiceInstanceID:
			continue
		default:
			result[k] = v
		}
	}
	return result
}

// IsIdentifyingAttribute returns true if the given key is an identifying attribute.
func IsIdentifyingAttribute(key string) bool {
	switch key {
	case AttrServiceName, AttrServiceNamespace, AttrServiceInstanceID:
		return true
	default:
		return false
	}
}

// VersionedResourceAttributes holds multiple versions of resource attributes
// for a single time series. Each version represents a period when specific
// attribute values were active.
type VersionedResourceAttributes struct {
	// Versions ordered by MinTime ascending. Most recent version is last.
	Versions []*ResourceAttributes
}

// NewVersionedResourceAttributes creates a new VersionedResourceAttributes with a single version.
func NewVersionedResourceAttributes(attrs map[string]string, timestamp int64) *VersionedResourceAttributes {
	return &VersionedResourceAttributes{
		Versions: []*ResourceAttributes{
			NewResourceAttributes(attrs, timestamp, timestamp),
		},
	}
}

// CurrentVersion returns the most recent version (last in the list).
// Returns nil if no versions exist.
func (v *VersionedResourceAttributes) CurrentVersion() *ResourceAttributes {
	if len(v.Versions) == 0 {
		return nil
	}
	return v.Versions[len(v.Versions)-1]
}

// VersionAt returns the version that was active at the given timestamp.
// Returns nil if no version covers that timestamp.
func (v *VersionedResourceAttributes) VersionAt(timestamp int64) *ResourceAttributes {
	// Binary search for the version covering this timestamp
	// A version covers a timestamp if MinTime <= timestamp <= MaxTime
	for i := len(v.Versions) - 1; i >= 0; i-- {
		ver := v.Versions[i]
		if timestamp >= ver.MinTime && timestamp <= ver.MaxTime {
			return ver
		}
		// If timestamp is after this version's MaxTime but before next version's MinTime,
		// return this version (it was the active one)
		if timestamp > ver.MaxTime {
			return ver
		}
	}
	// Timestamp is before any version
	if len(v.Versions) > 0 {
		return v.Versions[0]
	}
	return nil
}

// AddOrExtend adds a new version if attributes changed, or extends the current version's
// time range if attributes are the same.
func (v *VersionedResourceAttributes) AddOrExtend(attrs map[string]string, timestamp int64) {
	if len(v.Versions) == 0 {
		v.Versions = []*ResourceAttributes{
			NewResourceAttributes(attrs, timestamp, timestamp),
		}
		return
	}

	current := v.Versions[len(v.Versions)-1]
	if AttributesEqual(current.Attributes, attrs) {
		// Same attributes: extend time range
		current.UpdateTimeRange(timestamp, timestamp)
	} else {
		// Different attributes: create new version
		v.Versions = append(v.Versions, NewResourceAttributes(attrs, timestamp, timestamp))
	}
}

// AttributesEqual compares two attribute maps for equality.
func AttributesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// MergeVersionedAttributes merges two VersionedResourceAttributes for the same series.
// It combines all versions, sorts by MinTime, and merges adjacent versions with identical attributes.
func MergeVersionedAttributes(a, b *VersionedResourceAttributes) *VersionedResourceAttributes {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	// Combine all versions
	all := make([]*ResourceAttributes, 0, len(a.Versions)+len(b.Versions))
	all = append(all, a.Versions...)
	all = append(all, b.Versions...)

	// Sort by MinTime
	sort.Slice(all, func(i, j int) bool {
		return all[i].MinTime < all[j].MinTime
	})

	// Merge adjacent/overlapping versions with identical attributes
	merged := make([]*ResourceAttributes, 0, len(all))
	for _, ver := range all {
		if len(merged) == 0 {
			// Deep copy the version
			merged = append(merged, copyResourceAttributes(ver))
			continue
		}

		last := merged[len(merged)-1]

		// Merge if: same attributes AND (overlapping OR adjacent time ranges)
		// Adjacent means the gap is at most 1ms
		if AttributesEqual(last.Attributes, ver.Attributes) && ver.MinTime <= last.MaxTime+1 {
			// Extend the time range
			if ver.MaxTime > last.MaxTime {
				last.MaxTime = ver.MaxTime
			}
			if ver.MinTime < last.MinTime {
				last.MinTime = ver.MinTime
			}
		} else {
			// Different attributes or non-adjacent: keep as separate version
			merged = append(merged, copyResourceAttributes(ver))
		}
	}

	return &VersionedResourceAttributes{Versions: merged}
}

// copyResourceAttributes creates a deep copy of ResourceAttributes.
func copyResourceAttributes(ra *ResourceAttributes) *ResourceAttributes {
	attrs := make(map[string]string, len(ra.Attributes))
	maps.Copy(attrs, ra.Attributes)
	return &ResourceAttributes{
		ServiceName:       ra.ServiceName,
		ServiceNamespace:  ra.ServiceNamespace,
		ServiceInstanceID: ra.ServiceInstanceID,
		Attributes:        attrs,
		MinTime:           ra.MinTime,
		MaxTime:           ra.MaxTime,
	}
}

// UpdateTimeRange extends the time range to include the given timestamps.
func (ra *ResourceAttributes) UpdateTimeRange(minTime, maxTime int64) {
	if minTime < ra.MinTime || ra.MinTime == 0 {
		ra.MinTime = minTime
	}
	if maxTime > ra.MaxTime {
		ra.MaxTime = maxTime
	}
}

// VersionedResourceAttributesReader provides read access to versioned resource attributes.
type VersionedResourceAttributesReader interface {
	// GetResourceAttributes returns the current (latest) resource attributes for the series.
	// Returns nil and false if not found.
	GetResourceAttributes(labelsHash uint64) (*ResourceAttributes, bool)

	// IterResourceAttributes calls the given function for each series' current resource attributes.
	// Iteration stops early if f returns an error.
	IterResourceAttributes(f func(labelsHash uint64, attrs *ResourceAttributes) error) error

	// TotalResourceAttributes returns the total count of series with resource attributes.
	TotalResourceAttributes() uint64

	// GetVersionedResourceAttributes returns all versions of resource attributes for the series.
	// Returns nil and false if not found.
	GetVersionedResourceAttributes(labelsHash uint64) (*VersionedResourceAttributes, bool)

	// GetResourceAttributesAt returns the resource attributes active at the given timestamp.
	// Returns nil and false if not found or no version covers the timestamp.
	GetResourceAttributesAt(labelsHash uint64, timestamp int64) (*ResourceAttributes, bool)

	// IterVersionedResourceAttributes calls the given function for each series' versioned attributes.
	// Iteration stops early if f returns an error.
	IterVersionedResourceAttributes(f func(labelsHash uint64, attrs *VersionedResourceAttributes) error) error

	// TotalResourceAttributeVersions returns the total count of all versions across all series.
	TotalResourceAttributeVersions() uint64
}

// versionedResourceAttributesEntry stores versioned resource attributes with labels hash for indexing.
type versionedResourceAttributesEntry struct {
	labelsHash uint64
	attrs      *VersionedResourceAttributes
}

// MemResourceAttributes is an in-memory implementation of versioned resource attributes storage.
type MemResourceAttributes struct {
	byHash map[uint64]*versionedResourceAttributesEntry
	mtx    sync.RWMutex
}

// NewMemResourceAttributes creates a new in-memory resource attributes store.
func NewMemResourceAttributes() *MemResourceAttributes {
	return &MemResourceAttributes{
		byHash: make(map[uint64]*versionedResourceAttributesEntry),
	}
}

// GetResourceAttributes returns the current (latest) resource attributes for the series.
func (m *MemResourceAttributes) GetResourceAttributes(labelsHash uint64) (*ResourceAttributes, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok || len(entry.attrs.Versions) == 0 {
		return nil, false
	}
	return entry.attrs.CurrentVersion(), true
}

// GetVersionedResourceAttributes returns all versions of resource attributes for the series.
func (m *MemResourceAttributes) GetVersionedResourceAttributes(labelsHash uint64) (*VersionedResourceAttributes, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.attrs, true
}

// GetResourceAttributesAt returns the resource attributes active at the given timestamp.
func (m *MemResourceAttributes) GetResourceAttributesAt(labelsHash uint64, timestamp int64) (*ResourceAttributes, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	ver := entry.attrs.VersionAt(timestamp)
	if ver == nil {
		return nil, false
	}
	return ver, true
}

// SetResourceAttributes stores resource attributes for the given labels hash.
// If attributes already exist, a new version is created if attrs differ,
// or the existing version's time range is extended if attrs are the same.
// For bulk loading pre-computed versions (e.g., from Parquet), use SetVersionedResourceAttributes.
func (m *MemResourceAttributes) SetResourceAttributes(labelsHash uint64, attrs *ResourceAttributes) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		// Use AddOrExtend to handle versioning
		existing.attrs.AddOrExtend(attrs.Attributes, attrs.MinTime)
		// Also extend with MaxTime if different
		if attrs.MaxTime != attrs.MinTime {
			current := existing.attrs.CurrentVersion()
			if current != nil && AttributesEqual(current.Attributes, attrs.Attributes) {
				current.UpdateTimeRange(attrs.MaxTime, attrs.MaxTime)
			}
		}
		return
	}

	m.byHash[labelsHash] = &versionedResourceAttributesEntry{
		labelsHash: labelsHash,
		attrs: &VersionedResourceAttributes{
			Versions: []*ResourceAttributes{attrs},
		},
	}
}

// SetVersionedResourceAttributes stores versioned resource attributes for the given labels hash.
// Used during compaction and loading from Parquet.
func (m *MemResourceAttributes) SetVersionedResourceAttributes(labelsHash uint64, attrs *VersionedResourceAttributes) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		// Merge versions from new into existing
		existing.attrs = MergeVersionedAttributes(existing.attrs, attrs)
		return
	}

	m.byHash[labelsHash] = &versionedResourceAttributesEntry{
		labelsHash: labelsHash,
		attrs:      attrs,
	}
}

// DeleteResourceAttributes removes resource attributes for the given labels hash.
func (m *MemResourceAttributes) DeleteResourceAttributes(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.byHash, labelsHash)
}

// IterResourceAttributes calls the given function for each series' current resource attributes.
func (m *MemResourceAttributes) IterResourceAttributes(f func(labelsHash uint64, attrs *ResourceAttributes) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		current := entry.attrs.CurrentVersion()
		if current != nil {
			if err := f(hash, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterVersionedResourceAttributes calls the given function for each series' versioned attributes.
func (m *MemResourceAttributes) IterVersionedResourceAttributes(f func(labelsHash uint64, attrs *VersionedResourceAttributes) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		if err := f(hash, entry.attrs); err != nil {
			return err
		}
	}
	return nil
}

// TotalResourceAttributes returns the total count of series with resource attributes.
func (m *MemResourceAttributes) TotalResourceAttributes() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byHash))
}

// TotalResourceAttributeVersions returns the total count of all versions across all series.
func (m *MemResourceAttributes) TotalResourceAttributeVersions() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	var total uint64
	for _, entry := range m.byHash {
		total += uint64(len(entry.attrs.Versions))
	}
	return total
}
