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
	"errors"
	"maps"
	"slices"

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
	slices.SortFunc(copiedEntities, func(a, b *Entity) int {
		if a.Type < b.Type {
			return -1
		}
		if a.Type > b.Type {
			return 1
		}
		return 0
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

// GetMinTime returns the minimum timestamp.
func (rv *ResourceVersion) GetMinTime() int64 { return rv.MinTime }

// GetMaxTime returns the maximum timestamp.
func (rv *ResourceVersion) GetMaxTime() int64 { return rv.MaxTime }

// SetMinTime sets the minimum timestamp.
func (rv *ResourceVersion) SetMinTime(t int64) { rv.MinTime = t }

// SetMaxTime sets the maximum timestamp.
func (rv *ResourceVersion) SetMaxTime(t int64) { rv.MaxTime = t }

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

// VersionedResource is a type alias for the generic Versioned container for resources.
type VersionedResource = Versioned[*ResourceVersion]

// NewVersionedResource creates a new VersionedResource with a single version.
func NewVersionedResource(version *ResourceVersion) *VersionedResource {
	return &VersionedResource{
		Versions: []*ResourceVersion{copyResourceVersion(version)},
	}
}

// MergeVersionedResources merges two VersionedResource instances for the same series.
func MergeVersionedResources(a, b *VersionedResource) *VersionedResource {
	return MergeVersioned(ResourceOps, a, b)
}

// VersionedResourceReader provides read access to versioned resources.
type VersionedResourceReader interface {
	GetResource(labelsHash uint64) (*ResourceVersion, bool)
	GetVersionedResource(labelsHash uint64) (*VersionedResource, bool)
	GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool)
	IterResources(ctx context.Context, f func(labelsHash uint64, resource *ResourceVersion) error) error
	IterVersionedResources(ctx context.Context, f func(labelsHash uint64, resources *VersionedResource) error) error
	TotalResources() uint64
	TotalResourceVersions() uint64
}

// MemResourceStore is a type alias for the generic MemStore for resources.
type MemResourceStore = MemStore[*ResourceVersion]

// NewMemResourceStore creates a new in-memory resource store.
func NewMemResourceStore() *MemResourceStore {
	return NewMemStore[*ResourceVersion](ResourceOps)
}

// EntitiesFromResourceVersion extracts all entities from a resource version.
func EntitiesFromResourceVersion(rv *ResourceVersion) []*Entity {
	if rv == nil {
		return nil
	}
	return slices.Clone(rv.Entities)
}
