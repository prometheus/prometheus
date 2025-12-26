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
	"regexp"
	"sort"
	"sync"
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
type Entity struct {
	// Type defines the entity type (e.g., "service", "host", "container", "resource").
	// MUST NOT be empty. Default is "resource" for backward compatibility.
	Type string

	// Id contains identifying attributes that uniquely identify the entity.
	// These attributes MUST NOT change during the entity's lifetime.
	Id map[string]string

	// Description contains descriptive (non-identifying) attributes.
	// These attributes MAY change over the entity's lifetime.
	Description map[string]string

	// MinTime is the minimum timestamp (in milliseconds) when this entity version was observed.
	MinTime int64
	// MaxTime is the maximum timestamp (in milliseconds) when this entity version was observed.
	MaxTime int64
}

// NewEntity creates a new Entity with the given type, identifying and descriptive attributes.
func NewEntity(entityType string, id, description map[string]string, minTime, maxTime int64) *Entity {
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
		Id:          idCopy,
		Description: descCopy,
		MinTime:     minTime,
		MaxTime:     maxTime,
	}
}

// NewResourceEntity creates a new Entity with the default "resource" type.
// It uses the hardcoded identifying attributes (service.name, service.namespace, service.instance.id)
// to separate identifying from descriptive attributes.
func NewResourceEntity(attrs map[string]string, minTime, maxTime int64) *Entity {
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
		Id:          id,
		Description: description,
		MinTime:     minTime,
		MaxTime:     maxTime,
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
	result := make(map[string]string, len(e.Id)+len(e.Description))
	maps.Copy(result, e.Id)
	maps.Copy(result, e.Description)
	return result
}

// Identity returns the EntityIdentity for this entity.
func (e *Entity) Identity() EntityIdentity {
	idCopy := make(map[string]string, len(e.Id))
	maps.Copy(idCopy, e.Id)
	return EntityIdentity{
		Type: e.Type,
		Id:   idCopy,
	}
}

// UpdateTimeRange extends the time range to include the given timestamps.
func (e *Entity) UpdateTimeRange(minTime, maxTime int64) {
	if minTime < e.MinTime || e.MinTime == 0 {
		e.MinTime = minTime
	}
	if maxTime > e.MaxTime {
		e.MaxTime = maxTime
	}
}

// EntityIdentity defines the minimal identifying information for an entity.
// Used for deduplication and lookup.
type EntityIdentity struct {
	Type string
	Id   map[string]string
}

// Equals checks if two entity identities are equal.
func (ei EntityIdentity) Equals(other EntityIdentity) bool {
	if ei.Type != other.Type {
		return false
	}
	return AttributesEqual(ei.Id, other.Id)
}

// VersionedEntity holds multiple versions of an entity for a single series.
// Each version represents a period when specific attribute values were active.
type VersionedEntity struct {
	// Versions ordered by MinTime ascending. Most recent version is last.
	Versions []*Entity
}

// NewVersionedEntity creates a new VersionedEntity with a single version.
func NewVersionedEntity(entity *Entity) *VersionedEntity {
	return &VersionedEntity{
		Versions: []*Entity{entity},
	}
}

// CurrentVersion returns the most recent version (last in the list).
// Returns nil if no versions exist.
func (v *VersionedEntity) CurrentVersion() *Entity {
	if len(v.Versions) == 0 {
		return nil
	}
	return v.Versions[len(v.Versions)-1]
}

// VersionAt returns the version that was active at the given timestamp.
// Returns nil if no version covers that timestamp.
func (v *VersionedEntity) VersionAt(timestamp int64) *Entity {
	for i := len(v.Versions) - 1; i >= 0; i-- {
		ver := v.Versions[i]
		if timestamp >= ver.MinTime && timestamp <= ver.MaxTime {
			return ver
		}
		if timestamp > ver.MaxTime {
			return ver
		}
	}
	if len(v.Versions) > 0 {
		return v.Versions[0]
	}
	return nil
}

// AddOrExtend adds a new version if the entity changed, or extends the current version's
// time range if the entity is the same.
func (v *VersionedEntity) AddOrExtend(entity *Entity) {
	if len(v.Versions) == 0 {
		v.Versions = []*Entity{copyEntity(entity)}
		return
	}

	current := v.Versions[len(v.Versions)-1]
	if EntitiesEqual(current, entity) {
		// Same entity: extend time range
		current.UpdateTimeRange(entity.MinTime, entity.MaxTime)
	} else {
		// Different entity: create new version
		v.Versions = append(v.Versions, copyEntity(entity))
	}
}

// EntitiesEqual compares two entities for equality (type, id, and description).
func EntitiesEqual(a, b *Entity) bool {
	if a.Type != b.Type {
		return false
	}
	if !AttributesEqual(a.Id, b.Id) {
		return false
	}
	return AttributesEqual(a.Description, b.Description)
}

// MergeVersionedEntities merges two VersionedEntity instances for the same series.
// It combines all versions, sorts by MinTime, and merges adjacent versions with identical attributes.
func MergeVersionedEntities(a, b *VersionedEntity) *VersionedEntity {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	// Combine all versions
	all := make([]*Entity, 0, len(a.Versions)+len(b.Versions))
	all = append(all, a.Versions...)
	all = append(all, b.Versions...)

	// Sort by MinTime
	sort.Slice(all, func(i, j int) bool {
		return all[i].MinTime < all[j].MinTime
	})

	// Merge adjacent/overlapping versions with identical attributes
	merged := make([]*Entity, 0, len(all))
	for _, ver := range all {
		if len(merged) == 0 {
			merged = append(merged, copyEntity(ver))
			continue
		}

		last := merged[len(merged)-1]

		// Merge if: same entity AND (overlapping OR adjacent time ranges)
		if EntitiesEqual(last, ver) && ver.MinTime <= last.MaxTime+1 {
			if ver.MaxTime > last.MaxTime {
				last.MaxTime = ver.MaxTime
			}
			if ver.MinTime < last.MinTime {
				last.MinTime = ver.MinTime
			}
		} else {
			merged = append(merged, copyEntity(ver))
		}
	}

	return &VersionedEntity{Versions: merged}
}

// copyEntity creates a deep copy of an Entity.
func copyEntity(e *Entity) *Entity {
	idCopy := make(map[string]string, len(e.Id))
	maps.Copy(idCopy, e.Id)

	descCopy := make(map[string]string, len(e.Description))
	maps.Copy(descCopy, e.Description)

	return &Entity{
		Type:        e.Type,
		Id:          idCopy,
		Description: descCopy,
		MinTime:     e.MinTime,
		MaxTime:     e.MaxTime,
	}
}

// VersionedEntityReader provides read access to versioned entities.
type VersionedEntityReader interface {
	// GetEntity returns the current (latest) entity for the series.
	GetEntity(labelsHash uint64) (*Entity, bool)

	// GetVersionedEntity returns all versions of the entity for the series.
	GetVersionedEntity(labelsHash uint64) (*VersionedEntity, bool)

	// GetEntityAt returns the entity version active at the given timestamp.
	GetEntityAt(labelsHash uint64, timestamp int64) (*Entity, bool)

	// IterEntities calls the function for each series' current entity.
	IterEntities(f func(labelsHash uint64, entity *Entity) error) error

	// IterVersionedEntities calls the function for each series' versioned entities.
	IterVersionedEntities(f func(labelsHash uint64, entities *VersionedEntity) error) error

	// TotalEntities returns the count of series with entities.
	TotalEntities() uint64

	// TotalEntityVersions returns the total count of all entity versions.
	TotalEntityVersions() uint64
}

// versionedEntityEntry stores versioned entities with labels hash for indexing.
type versionedEntityEntry struct {
	labelsHash uint64
	entities   *VersionedEntity
}

// MemEntityStore is an in-memory implementation of entity storage.
type MemEntityStore struct {
	byHash map[uint64]*versionedEntityEntry
	mtx    sync.RWMutex
}

// NewMemEntityStore creates a new in-memory entity store.
func NewMemEntityStore() *MemEntityStore {
	return &MemEntityStore{
		byHash: make(map[uint64]*versionedEntityEntry),
	}
}

// GetEntity returns the current (latest) entity for the series.
func (m *MemEntityStore) GetEntity(labelsHash uint64) (*Entity, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok || len(entry.entities.Versions) == 0 {
		return nil, false
	}
	return entry.entities.CurrentVersion(), true
}

// GetVersionedEntity returns all versions of the entity for the series.
func (m *MemEntityStore) GetVersionedEntity(labelsHash uint64) (*VersionedEntity, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.entities, true
}

// GetEntityAt returns the entity version active at the given timestamp.
func (m *MemEntityStore) GetEntityAt(labelsHash uint64, timestamp int64) (*Entity, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return nil, false
	}
	ver := entry.entities.VersionAt(timestamp)
	if ver == nil {
		return nil, false
	}
	return ver, true
}

// SetEntity stores an entity for the series.
// If an entity already exists, a new version is created if it differs,
// or the existing version's time range is extended if it's the same.
func (m *MemEntityStore) SetEntity(labelsHash uint64, entity *Entity) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.entities.AddOrExtend(entity)
		return
	}

	m.byHash[labelsHash] = &versionedEntityEntry{
		labelsHash: labelsHash,
		entities:   NewVersionedEntity(entity),
	}
}

// SetVersionedEntity stores versioned entities for the series.
// Used during compaction and loading from Parquet.
func (m *MemEntityStore) SetVersionedEntity(labelsHash uint64, entities *VersionedEntity) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHash[labelsHash]; ok {
		existing.entities = MergeVersionedEntities(existing.entities, entities)
		return
	}

	m.byHash[labelsHash] = &versionedEntityEntry{
		labelsHash: labelsHash,
		entities:   entities,
	}
}

// DeleteEntity removes all entity data for the series.
func (m *MemEntityStore) DeleteEntity(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.byHash, labelsHash)
}

// IterEntities calls the function for each series' current entity.
func (m *MemEntityStore) IterEntities(f func(labelsHash uint64, entity *Entity) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		current := entry.entities.CurrentVersion()
		if current != nil {
			if err := f(hash, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// IterVersionedEntities calls the function for each series' versioned entities.
func (m *MemEntityStore) IterVersionedEntities(f func(labelsHash uint64, entities *VersionedEntity) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		if err := f(hash, entry.entities); err != nil {
			return err
		}
	}
	return nil
}

// TotalEntities returns the count of series with entities.
func (m *MemEntityStore) TotalEntities() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byHash))
}

// TotalEntityVersions returns the total count of all entity versions.
func (m *MemEntityStore) TotalEntityVersions() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	var total uint64
	for _, entry := range m.byHash {
		total += uint64(len(entry.entities.Versions))
	}
	return total
}
