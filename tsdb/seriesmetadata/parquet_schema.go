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

// Namespace constants for discriminating row types in the Parquet file.
const (
	// NamespaceResource indicates a row contains a unified resource with attributes and entities.
	NamespaceResource = "resource"
	// NamespaceScope indicates a row contains OTel InstrumentationScope data.
	NamespaceScope = "scope"

	// NamespaceResourceTable indicates a content-addressed resource table row.
	// Contains the full resource content (identifying/descriptive attrs, entities)
	// keyed by ContentHash. No SeriesRef or time range.
	NamespaceResourceTable = "resource_table"
	// NamespaceResourceMapping maps a series (SeriesRef) to a resource (ContentHash)
	// at a specific time range (MinTime/MaxTime).
	NamespaceResourceMapping = "resource_mapping"
	// NamespaceScopeTable indicates a content-addressed scope table row.
	// Contains the full scope content keyed by ContentHash.
	NamespaceScopeTable = "scope_table"
	// NamespaceScopeMapping maps a series (SeriesRef) to a scope (ContentHash)
	// at a specific time range (MinTime/MaxTime).
	NamespaceScopeMapping = "scope_mapping"
)

// Identifying attribute keys per OTel semantic conventions.
const (
	AttrServiceName       = "service.name"
	AttrServiceNamespace  = "service.namespace"
	AttrServiceInstanceID = "service.instance.id"
)

// AttributeEntry represents a single resource attribute in the Parquet schema.
// Used as a nested list within metadataRow for backward compatibility.
type AttributeEntry struct {
	// Key is the attribute name (e.g., "deployment.environment").
	Key string `parquet:"key"`
	// Value is the attribute value as a string.
	Value string `parquet:"value"`
	// IsIdentifying indicates if this attribute is an identifying attribute
	// (service.name, service.namespace, service.instance.id).
	IsIdentifying bool `parquet:"is_identifying"`
}

// EntityAttributeEntry represents a single entity attribute in the Parquet schema.
// Used for the entity namespace with separate identifying and descriptive attribute lists.
type EntityAttributeEntry struct {
	// Key is the attribute name.
	Key string `parquet:"key"`
	// Value is the attribute value as a string.
	Value string `parquet:"value"`
}

// EntityRow represents a single entity within a resource version in the Parquet schema.
type EntityRow struct {
	// Type is the entity type (e.g., "service", "host", "container", "resource").
	Type string `parquet:"type"`
	// ID contains identifying attributes that uniquely identify the entity.
	ID []EntityAttributeEntry `parquet:"id,list"`
	// Description contains descriptive (non-identifying) attributes.
	Description []EntityAttributeEntry `parquet:"description,list"`
}

// metadataRow is the unified Parquet schema for OTel resources and scopes.
// The Namespace field discriminates the logical row type.
type metadataRow struct {
	// Namespace discriminates the row type: "resource_table", "resource_mapping",
	// "scope_table", or "scope_mapping".
	Namespace string `parquet:"namespace"`

	// SeriesRef identifies the series for mapping rows. In block Parquet files
	// this is the block-level series reference; the read path uses a RefResolver
	// to convert back to labelsHash for in-memory lookups. When no RefResolver
	// is provided (e.g. head/test writes), labelsHash is stored directly.
	SeriesRef uint64 `parquet:"series_ref"`

	// MinTime is the minimum timestamp (in milliseconds) when this data was active.
	MinTime int64 `parquet:"mint,optional"`

	// MaxTime is the maximum timestamp (in milliseconds) when this data was active.
	MaxTime int64 `parquet:"maxt,optional"`

	// ContentHash is the xxhash of content for content-addressed rows.
	// Used for resource_table/resource_mapping and scope_table/scope_mapping rows.
	ContentHash uint64 `parquet:"content_hash,optional"`

	// --- Resource fields (namespace="resource") ---

	// IdentifyingAttrs contains the resource-level identifying attributes.
	IdentifyingAttrs []EntityAttributeEntry `parquet:"identifying_attrs,list,optional"`

	// DescriptiveAttrs contains the resource-level descriptive attributes.
	DescriptiveAttrs []EntityAttributeEntry `parquet:"descriptive_attrs,list,optional"`

	// Entities contains typed entities associated with this resource version.
	Entities []EntityRow `parquet:"entities,list,optional"`

	// --- Scope fields (namespace="scope") ---

	// ScopeName is the InstrumentationScope name.
	ScopeName string `parquet:"scope_name,optional"`

	// ScopeVersionStr is the InstrumentationScope version.
	// Named "scope_version_str" to avoid clash with Parquet reserved words.
	ScopeVersionStr string `parquet:"scope_version_str,optional"`

	// SchemaURL is the InstrumentationScope schema URL.
	SchemaURL string `parquet:"schema_url,optional"`

	// ScopeAttrs contains InstrumentationScope attributes.
	ScopeAttrs []EntityAttributeEntry `parquet:"scope_attrs,list,optional"`
}
