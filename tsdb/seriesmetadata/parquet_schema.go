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
	// NamespaceResourceTable indicates a content-addressed resource table row.
	// Contains the full resource content (identifying/descriptive attrs)
	// keyed by ContentHash. No SeriesRef or time range.
	NamespaceResourceTable = "resource_table"
	// NamespaceResourceMapping maps a series (SeriesRef) to a resource (ContentHash)
	// at a specific time range (MinTime/MaxTime).
	NamespaceResourceMapping = "resource_mapping"

	// NamespaceResourceAttrIndex stores inverted index entries mapping
	// resource attribute key:value pairs to series refs. Each row represents
	// one (key, value, seriesRef) tuple. The key and value are stored in
	// IdentifyingAttrs[0]; ContentHash is xxhash("key\x00value") for bloom
	// filter skipability.
	NamespaceResourceAttrIndex = "resource_attr_index"
)

// Identifying attribute keys per OTel semantic conventions.
const (
	AttrServiceName       = "service.name"
	AttrServiceNamespace  = "service.namespace"
	AttrServiceInstanceID = "service.instance.id"
)

// AttrEntry represents a single attribute key-value pair in the Parquet schema.
type AttrEntry struct {
	Key   string `parquet:"key"`
	Value string `parquet:"value"`
}

// metadataRow is the unified Parquet schema for OTel resources.
// The Namespace field discriminates the logical row type.
type metadataRow struct {
	// Namespace discriminates the row type: "resource_table" or "resource_mapping".
	Namespace string `parquet:"namespace"`

	// SeriesRef identifies the series for mapping rows.
	SeriesRef uint64 `parquet:"series_ref"`

	// MinTime is the minimum timestamp (in milliseconds) when this data was active.
	MinTime int64 `parquet:"mint,optional"`

	// MaxTime is the maximum timestamp (in milliseconds) when this data was active.
	MaxTime int64 `parquet:"maxt,optional"`

	// ContentHash is the xxhash of content for content-addressed rows.
	ContentHash uint64 `parquet:"content_hash,optional"`

	// --- Resource fields ---

	// IdentifyingAttrs contains the resource-level identifying attributes.
	IdentifyingAttrs []AttrEntry `parquet:"identifying_attrs,list,optional"`

	// DescriptiveAttrs contains the resource-level descriptive attributes.
	DescriptiveAttrs []AttrEntry `parquet:"descriptive_attrs,list,optional"`

	// --- Resource attribute index fields (namespace="resource_attr_index") ---

	// AttrKey is the attribute name for resource_attr_index rows.
	AttrKey string `parquet:"attr_key,optional"`

	// AttrValue is the attribute value for resource_attr_index rows.
	AttrValue string `parquet:"attr_value,optional"`
}
