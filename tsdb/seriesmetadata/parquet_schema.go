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
	// NamespaceMetric indicates a row contains metric metadata (type, unit, help).
	NamespaceMetric = "metric"
	// NamespaceResourceAttrs indicates a row contains OTel resource attributes.
	NamespaceResourceAttrs = "resource_attrs"
)

// Identifying attribute keys per OTel semantic conventions.
const (
	AttrServiceName       = "service.name"
	AttrServiceNamespace  = "service.namespace"
	AttrServiceInstanceID = "service.instance.id"
)

// AttributeEntry represents a single resource attribute in the Parquet schema.
// Used as a nested list within metadataRow.
type AttributeEntry struct {
	// Key is the attribute name (e.g., "deployment.environment").
	Key string `parquet:"key"`
	// Value is the attribute value as a string.
	Value string `parquet:"value"`
	// IsIdentifying indicates if this attribute is an identifying attribute
	// (service.name, service.namespace, service.instance.id).
	IsIdentifying bool `parquet:"is_identifying"`
}

// metadataRow is the unified Parquet schema for both metric metadata and resource attributes.
// The Namespace field discriminates between the two types of data.
type metadataRow struct {
	// Namespace discriminates the row type: "metric" or "resource_attrs".
	Namespace string `parquet:"namespace"`

	// LabelsHash is the stable hash of the series labels (using labels.StableHash).
	// For metric metadata, this is used for deduplication during compaction.
	// For resource attributes, this identifies the specific series.
	LabelsHash uint64 `parquet:"labels_hash"`

	// MinTime is the minimum timestamp (in milliseconds) when this metadata was active.
	// Optional for metric metadata, required for resource attributes.
	MinTime int64 `parquet:"mint,optional"`

	// MaxTime is the maximum timestamp (in milliseconds) when this metadata was active.
	// Optional for metric metadata, required for resource attributes.
	MaxTime int64 `parquet:"maxt,optional"`

	// --- Metric metadata fields (namespace="metric") ---

	// MetricName is the __name__ label value for the metric.
	MetricName string `parquet:"metric_name,optional"`

	// Type is the metric type as a string (counter, gauge, histogram, etc.)
	Type string `parquet:"type,optional"`

	// Unit is the metric unit (e.g., "bytes", "seconds").
	Unit string `parquet:"unit,optional"`

	// Help is the metric help text.
	Help string `parquet:"help,optional"`

	// --- Resource attributes fields (namespace="resource_attrs") ---

	// ServiceName is the service.name identifying attribute (explicit column for efficient querying).
	ServiceName string `parquet:"service_name,optional"`

	// ServiceNamespace is the service.namespace identifying attribute.
	ServiceNamespace string `parquet:"service_namespace,optional"`

	// ServiceInstanceID is the service.instance.id identifying attribute.
	ServiceInstanceID string `parquet:"service_instance_id,optional"`

	// Attributes is the list of all resource attributes (including identifying ones).
	Attributes []AttributeEntry `parquet:"attributes,list,optional"`
}
