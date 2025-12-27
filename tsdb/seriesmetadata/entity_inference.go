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

// EntityInferenceStrategy defines how to infer entity type and separate
// identifying from descriptive attributes based on raw resource attributes.
type EntityInferenceStrategy interface {
	// InferEntity determines the entity type and separates identifying from
	// descriptive attributes based on OTel semantic conventions.
	InferEntity(attrs map[string]string, minTime, maxTime int64) *Entity
}

// SemanticConventionInferrer implements entity inference based on OTel semantic conventions.
// It uses pattern matching to determine entity type and which attributes are identifying.
type SemanticConventionInferrer struct {
	// identifyingAttributesByType maps entity types to their identifying attribute keys.
	identifyingAttributesByType map[string][]string
}

// NewSemanticConventionInferrer creates a new inferrer with default OTel semantic convention patterns.
func NewSemanticConventionInferrer() *SemanticConventionInferrer {
	return &SemanticConventionInferrer{
		identifyingAttributesByType: map[string][]string{
			EntityTypeService:   {AttrServiceName, AttrServiceNamespace},
			EntityTypeHost:      {"host.id"},
			EntityTypeContainer: {"container.id"},
			EntityTypeK8sPod:    {"k8s.pod.uid", "k8s.namespace.name"},
			EntityTypeK8sNode:   {"k8s.node.uid", "k8s.cluster.name"},
			EntityTypeProcess:   {"process.pid", "host.id"},
			// Default resource type uses hardcoded identifying attributes
			EntityTypeResource: {AttrServiceName, AttrServiceNamespace, AttrServiceInstanceID},
		},
	}
}

// InferEntity determines the entity type and separates attributes.
// Priority order for type inference:
// 1. Explicit otel.entity.type attribute
// 2. Pattern matching based on present attributes
// 3. Default to "resource" type
func (s *SemanticConventionInferrer) InferEntity(attrs map[string]string, minTime, maxTime int64) *Entity {
	// Check for explicit entity type
	entityType := EntityTypeResource
	if explicitType, ok := attrs["otel.entity.type"]; ok && explicitType != "" {
		entityType = explicitType
	} else {
		// Infer type from present attributes
		entityType = s.inferTypeFromAttributes(attrs)
	}

	// Get identifying attributes for this type
	identifyingKeys := s.getIdentifyingKeys(entityType)

	// Separate identifying from descriptive attributes
	id := make(map[string]string)
	description := make(map[string]string)

	for k, v := range attrs {
		// Skip the entity type marker attribute
		if k == "otel.entity.type" {
			continue
		}

		if s.isIdentifyingKey(k, identifyingKeys) {
			id[k] = v
		} else {
			description[k] = v
		}
	}

	return &Entity{
		Type:        entityType,
		Id:          id,
		Description: description,
		MinTime:     minTime,
		MaxTime:     maxTime,
	}
}

// inferTypeFromAttributes determines entity type based on which attributes are present.
func (s *SemanticConventionInferrer) inferTypeFromAttributes(attrs map[string]string) string {
	// Check for specific entity types in priority order
	// More specific types take precedence

	// K8s Pod (most specific k8s entity)
	if _, ok := attrs["k8s.pod.uid"]; ok {
		return EntityTypeK8sPod
	}

	// K8s Node
	if _, ok := attrs["k8s.node.uid"]; ok {
		return EntityTypeK8sNode
	}

	// Container
	if _, ok := attrs["container.id"]; ok {
		return EntityTypeContainer
	}

	// Process
	if _, hasPid := attrs["process.pid"]; hasPid {
		return EntityTypeProcess
	}

	// Host
	if _, ok := attrs["host.id"]; ok {
		return EntityTypeHost
	}

	// Service (if service.name is present without more specific identifiers)
	if _, ok := attrs[AttrServiceName]; ok {
		return EntityTypeService
	}

	// Default to resource
	return EntityTypeResource
}

// getIdentifyingKeys returns the identifying attribute keys for a given entity type.
func (s *SemanticConventionInferrer) getIdentifyingKeys(entityType string) []string {
	if keys, ok := s.identifyingAttributesByType[entityType]; ok {
		return keys
	}
	// For unknown types, fall back to default resource identifying attributes
	return s.identifyingAttributesByType[EntityTypeResource]
}

// isIdentifyingKey checks if an attribute key is an identifying attribute.
func (s *SemanticConventionInferrer) isIdentifyingKey(key string, identifyingKeys []string) bool {
	for _, k := range identifyingKeys {
		if k == key {
			return true
		}
	}
	return false
}

// DefaultEntityInferrer is the default entity inferrer using semantic conventions.
var DefaultEntityInferrer = NewSemanticConventionInferrer()

// InferEntityFromAttributes is a convenience function that uses the default inferrer.
func InferEntityFromAttributes(attrs map[string]string, minTime, maxTime int64) *Entity {
	return DefaultEntityInferrer.InferEntity(attrs, minTime, maxTime)
}
