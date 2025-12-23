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
	"errors"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/grafana/regexp"
)

// Entity type constants aligned with OTel semantic conventions.
const (
	// EntityTypeResource is the default entity type for generic OTel resources.
	EntityTypeResource = "resource"
	// EntityTypeService represents a service entity.
	EntityTypeService = "service"
	// EntityTypeHost represents a host entity.
	EntityTypeHost = "host"
	// EntityTypeContainer represents a container entity.
	EntityTypeContainer = "container"
	// EntityTypeK8sPod represents a Kubernetes pod entity.
	EntityTypeK8sPod = "k8s.pod"
	// EntityTypeK8sNode represents a Kubernetes node entity.
	EntityTypeK8sNode = "k8s.node"
	// EntityTypeProcess represents a process entity.
	EntityTypeProcess = "process"
)

// entityTypePattern validates entity type strings.
// Must start with a letter and contain only letters, numbers, dots, underscores, and hyphens.
var entityTypePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*$`)

// Entity represents an OTel entity with explicit type and structured attributes.
// Conforms to the OTel Entity Data Model specification.
// An Entity is part of a ResourceVersion and does not have its own time range.
type Entity struct {
	// Type defines the entity type (e.g., "service", "host", "container", "resource").
	// MUST NOT be empty. Default is "resource" for backward compatibility.
	Type string

	// ID contains identifying attributes that uniquely identify the entity.
	// These attributes MUST NOT change during the entity's lifetime.
	ID map[string]string

	// Description contains descriptive (non-identifying) attributes.
	// These attributes MAY change over the entity's lifetime.
	Description map[string]string
}

// NewEntity creates a new Entity with the given type, identifying and descriptive attributes.
func NewEntity(entityType string, id, description map[string]string) *Entity {
	if entityType == "" {
		entityType = EntityTypeResource
	}

	// Deep copy the maps
	idCopy := make(map[string]string, len(id))
	maps.Copy(idCopy, id)

	descCopy := make(map[string]string, len(description))
	maps.Copy(descCopy, description)

	return &Entity{
		Type:        entityType,
		ID:          idCopy,
		Description: descCopy,
	}
}

// NewDefaultEntity creates a new Entity with the default "resource" type.
// It uses the hardcoded identifying attributes (service.name, service.namespace, service.instance.id)
// to separate identifying from descriptive attributes.
func NewDefaultEntity(attrs map[string]string) *Entity {
	id := make(map[string]string)
	description := make(map[string]string)

	for k, v := range attrs {
		if IsIdentifyingAttribute(k) {
			id[k] = v
		} else {
			description[k] = v
		}
	}

	return &Entity{
		Type:        EntityTypeResource,
		ID:          id,
		Description: description,
	}
}

// Validate checks if the entity has valid structure.
func (e *Entity) Validate() error {
	if e.Type == "" {
		return errors.New("entity type must not be empty")
	}
	if !entityTypePattern.MatchString(e.Type) {
		return errors.New("entity type must match pattern [a-zA-Z][a-zA-Z0-9._-]*")
	}
	return nil
}

// AllAttributes returns all attributes (both identifying and descriptive) combined.
func (e *Entity) AllAttributes() map[string]string {
	result := make(map[string]string, len(e.ID)+len(e.Description))
	maps.Copy(result, e.ID)
	maps.Copy(result, e.Description)
	return result
}

// copyEntity creates a deep copy of an Entity.
func copyEntity(e *Entity) *Entity {
	var idCopy map[string]string
	if e.ID != nil {
		idCopy = make(map[string]string, len(e.ID))
		maps.Copy(idCopy, e.ID)
	}

	var descCopy map[string]string
	if e.Description != nil {
		descCopy = make(map[string]string, len(e.Description))
		maps.Copy(descCopy, e.Description)
	}

	return &Entity{
		Type:        e.Type,
		ID:          idCopy,
		Description: descCopy,
	}
}

// EntitiesEqual compares two entities for equality (type, id, and description).
func EntitiesEqual(a, b *Entity) bool {
	if a.Type != b.Type {
		return false
	}
	if !AttributesEqual(a.ID, b.ID) {
		return false
	}
	return AttributesEqual(a.Description, b.Description)
}

// ResourceVersion represents a snapshot of resource data at a point in time.
// Contains both resource-level attributes and typed entities.
type ResourceVersion struct {
	// Identifying contains resource-level identifying attributes.
	// These uniquely identify the resource (e.g., service.name, service.namespace).
	Identifying map[string]string

	// Descriptive contains resource-level descriptive attributes.
	// These provide additional context but don't identify the resource.
	Descriptive map[string]string

	// Entities contains typed entities for this resource version.
	// Each entity has a unique Type within the version.
	Entities []*Entity

	// MinTime is the minimum timestamp (in milliseconds) when this version was observed.
	MinTime int64
	// MaxTime is the maximum timestamp (in milliseconds) when this version was observed.
	MaxTime int64
}

// NewResourceVersion creates a new ResourceVersion with the given attributes, entities and time range.
func NewResourceVersion(identifying, descriptive map[string]string, entities []*Entity, minTime, maxTime int64) *ResourceVersion {
	// Deep copy identifying
	idCopy := make(map[string]string, len(identifying))
	maps.Copy(idCopy, identifying)

	// Deep copy descriptive
	descCopy := make(map[string]string, len(descriptive))
	maps.Copy(descCopy, descriptive)

	// Deep copy the entities
	copiedEntities := make([]*Entity, len(entities))
	for i, e := range entities {
		copiedEntities[i] = copyEntity(e)
	}
	// Sort by Type for consistent comparison
	sort.Slice(copiedEntities, func(i, j int) bool {
		return copiedEntities[i].Type < copiedEntities[j].Type
	})
	return &ResourceVersion{
		Identifying: idCopy,
		Descriptive: descCopy,
		Entities:    copiedEntities,
		MinTime:     minTime,
		MaxTime:     maxTime,
	}
}

// GetEntity returns the entity with the given type, or nil if not found.
func (rv *ResourceVersion) GetEntity(entityType string) *Entity {
	for _, e := range rv.Entities {
		if e.Type == entityType {
			return e
		}
	}
	return nil
}

// UpdateTimeRange extends the time range to include the given timestamps.
func (rv *ResourceVersion) UpdateTimeRange(minTime, maxTime int64) {
	if minTime < rv.MinTime {
		rv.MinTime = minTime
	}
	if maxTime > rv.MaxTime {
		rv.MaxTime = maxTime
	}
}

// ResourceVersionsEqual compares two resource versions for equality.
// Compares identifying, descriptive attributes and all entities.
func ResourceVersionsEqual(a, b *ResourceVersion) bool {
	if !AttributesEqual(a.Identifying, b.Identifying) {
		return false
	}
	if !AttributesEqual(a.Descriptive, b.Descriptive) {
		return false
	}
	if len(a.Entities) != len(b.Entities) {
		return false
	}
	// Entities are sorted by Type, so we can compare in order
	for i := range a.Entities {
		if !EntitiesEqual(a.Entities[i], b.Entities[i]) {
			return false
		}
	}
	return true
}

// copyResourceVersion creates a deep copy of a ResourceVersion.
func copyResourceVersion(rv *ResourceVersion) *ResourceVersion {
	var idCopy map[string]string
	if rv.Identifying != nil {
		idCopy = make(map[string]string, len(rv.Identifying))
		maps.Copy(idCopy, rv.Identifying)
	}

	var descCopy map[string]string
	if rv.Descriptive != nil {
		descCopy = make(map[string]string, len(rv.Descriptive))
		maps.Copy(descCopy, rv.Descriptive)
	}

	var copiedEntities []*Entity
	if rv.Entities != nil {
		copiedEntities = make([]*Entity, len(rv.Entities))
		for i, e := range rv.Entities {
			copiedEntities[i] = copyEntity(e)
		}
	}
	return &ResourceVersion{
		Identifying: idCopy,
		Descriptive: descCopy,
		Entities:    copiedEntities,
		MinTime:     rv.MinTime,
		MaxTime:     rv.MaxTime,
	}
}

// VersionedResource holds multiple versions of a resource for a single series.
// Each version represents a period when specific entity configurations were active.
type VersionedResource struct {
	// Versions ordered by MinTime ascending. Most recent version is last.
	Versions []*ResourceVersion
}

// Copy creates a deep copy of the VersionedResource.
func (vr *VersionedResource) Copy() *VersionedResource {
	versions := make([]*ResourceVersion, len(vr.Versions))
	for i, v := range vr.Versions {
		versions[i] = copyResourceVersion(v)
	}
	return &VersionedResource{Versions: versions}
}

// NewVersionedResource creates a new VersionedResource with a single version.
func NewVersionedResource(version *ResourceVersion) *VersionedResource {
	return &VersionedResource{
		Versions: []*ResourceVersion{copyResourceVersion(version)},
	}
}

// CurrentVersion returns the most recent version (last in the list).
// Returns nil if no versions exist.
func (vr *VersionedResource) CurrentVersion() *ResourceVersion {
	if len(vr.Versions) == 0 {
		return nil
	}
	return vr.Versions[len(vr.Versions)-1]
}

// VersionAt returns the version that was active at the given timestamp.
// Returns nil if no version covers that timestamp.
func (vr *VersionedResource) VersionAt(timestamp int64) *ResourceVersion {
	for i := len(vr.Versions) - 1; i >= 0; i-- {
		ver := vr.Versions[i]
		if timestamp >= ver.MinTime && timestamp <= ver.MaxTime {
			return ver
		}
		if timestamp > ver.MaxTime {
			return ver
		}
	}
	return nil
}

// AddOrExtend adds a new version if the resource changed, or extends the current version's
// time range if the resource is the same.
func (vr *VersionedResource) AddOrExtend(version *ResourceVersion) {
	if len(vr.Versions) == 0 {
		vr.Versions = []*ResourceVersion{copyResourceVersion(version)}
		return
	}

	current := vr.Versions[len(vr.Versions)-1]
	if ResourceVersionsEqual(current, version) {
		// Same resource version: extend time range
		current.UpdateTimeRange(version.MinTime, version.MaxTime)
	} else {
		// Different resource version: create new version
		vr.Versions = append(vr.Versions, copyResourceVersion(version))
	}
}

// MergeVersionedResources merges two VersionedResource instances for the same series.
// It combines all versions, sorts by MinTime, and merges adjacent versions with identical entities.
func MergeVersionedResources(a, b *VersionedResource) *VersionedResource {
	if a == nil {
		return b.Copy()
	}
	if b == nil {
		return a.Copy()
	}

	// Combine all versions
	all := make([]*ResourceVersion, 0, len(a.Versions)+len(b.Versions))
	all = append(all, a.Versions...)
	all = append(all, b.Versions...)

	// Sort by MinTime
	sort.Slice(all, func(i, j int) bool {
		return all[i].MinTime < all[j].MinTime
	})

	// Merge adjacent/overlapping versions with identical entities
	merged := make([]*ResourceVersion, 0, len(all))
	for _, ver := range all {
		if len(merged) == 0 {
			merged = append(merged, copyResourceVersion(ver))
			continue
		}

		last := merged[len(merged)-1]

		// Merge if: same resource version AND (overlapping OR adjacent time ranges)
		if ResourceVersionsEqual(last, ver) && ver.MinTime <= last.MaxTime+1 {
			if ver.MaxTime > last.MaxTime {
				last.MaxTime = ver.MaxTime
			}
			if ver.MinTime < last.MinTime {
				last.MinTime = ver.MinTime
			}
		} else {
			merged = append(merged, copyResourceVersion(ver))
		}
	}

	return &VersionedResource{Versions: merged}
}

// VersionedResourceReader provides read access to versioned resources.
type VersionedResourceReader interface {
	// GetResource returns the current (latest) resource version for the series.
	GetResource(labelsHash uint64) (*ResourceVersion, bool)

	// GetVersionedResource returns all versions of the resource for the series.
	GetVersionedResource(labelsHash uint64) (*VersionedResource, bool)

	// GetResourceAt returns the resource version active at the given timestamp.
	GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool)

	// IterResources calls the function for each series' current resource.
	IterResources(f func(labelsHash uint64, resource *ResourceVersion) error) error

	// IterVersionedResources calls the function for each series' versioned resources.
	IterVersionedResources(f func(labelsHash uint64, resources *VersionedResource) error) error

	// TotalResources returns the count of series with resources.
	TotalResources() uint64

	// TotalResourceVersions returns the total count of all resource versions.
	TotalResourceVersions() uint64
}

// versionedResourceEntry stores versioned resources with labels hash for indexing.
type versionedResourceEntry struct {
	labelsHash uint64
	resources  *VersionedResource
}

// MemResourceStore is an in-memory implementation of resource storage.
type MemResourceStore struct {
	byHash map[uint64]*versionedResourceEntry
	mtx    sync.RWMutex
}

// NewMemResourceStore creates a new in-memory resource store.
func NewMemResourceStore() *MemResourceStore {
	return &MemResourceStore{
		byHash: make(map[uint64]*versionedResourceEntry),
	}
}

// Len returns the number of unique series with resource data.
func (m *MemResourceStore) Len() int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return len(m.byHash)
}

// GetResource returns the current (latest) resource version for the series.
func (m *MemResourceStore) GetResource(labelsHash uint64) (*ResourceVersion, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok || len(entry.resources.Versions) == 0 {
		return nil, false
	}
	return entry.resources.CurrentVersion(), true
}

// GetVersionedResource returns all versions of the resource for the series.
func (m *MemResourceStore) GetVersionedResource(labelsHash uint64) (*VersionedResource, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.resources, true
}

// GetResourceAt returns the resource version active at the given timestamp.
func (m *MemResourceStore) GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	ver := entry.resources.VersionAt(timestamp)
	if ver == nil {
		return nil, false
	}
	return ver, true
}

// SetResource stores a resource version for the series.
// If a resource already exists, a new version is created if it differs,
// or the existing version's time range is extended if it's the same.
func (m *MemResourceStore) SetResource(labelsHash uint64, resource *ResourceVersion) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.resources.AddOrExtend(resource)
		return
	}

	m.byHash[labelsHash] = &versionedResourceEntry{
		labelsHash: labelsHash,
		resources:  NewVersionedResource(resource),
	}
}

// SetVersionedResource stores versioned resources for the series.
// Used during compaction and loading from Parquet.
func (m *MemResourceStore) SetVersionedResource(labelsHash uint64, resources *VersionedResource) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.resources = MergeVersionedResources(existing.resources, resources)
		return
	}

	m.byHash[labelsHash] = &versionedResourceEntry{
		labelsHash: labelsHash,
		resources:  resources.Copy(),
	}
}

// DeleteResource removes all resource data for the series.
func (m *MemResourceStore) DeleteResource(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.byHash, labelsHash)
}

// IterResources calls the function for each series' current resource.
func (m *MemResourceStore) IterResources(f func(labelsHash uint64, resource *ResourceVersion) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		current := entry.resources.CurrentVersion()
		if current != nil {
			if err := f(hash, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterVersionedResources calls the function for each series' versioned resources.
func (m *MemResourceStore) IterVersionedResources(f func(labelsHash uint64, resources *VersionedResource) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		if err := f(hash, entry.resources); err != nil {
			return err
		}
	}
	return nil
}

// TotalResources returns the count of series with resources.
func (m *MemResourceStore) TotalResources() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byHash))
}

// TotalResourceVersions returns the total count of all resource versions.
func (m *MemResourceStore) TotalResourceVersions() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	var total uint64
	for _, entry := range m.byHash {
		total += uint64(len(entry.resources.Versions))
	}
	return total
}

// Legacy compatibility types and functions below.
// These are kept to support existing code during transition.

// VersionedEntity is kept for backward compatibility.
//
// Deprecated: Use VersionedResource instead.
type VersionedEntity = VersionedResource

// MemEntityStore is kept for backward compatibility.
//
// Deprecated: Use MemResourceStore instead.
type MemEntityStore = MemResourceStore

// NewMemEntityStore is kept for backward compatibility.
//
// Deprecated: Use NewMemResourceStore instead.
func NewMemEntityStore() *MemResourceStore {
	return NewMemResourceStore()
}

// EntitiesFromResourceVersion extracts all entities from a resource version.
// This is useful for iterating over entities in a resource.
func EntitiesFromResourceVersion(rv *ResourceVersion) []*Entity {
	if rv == nil {
		return nil
	}
	return slices.Clone(rv.Entities)
}
