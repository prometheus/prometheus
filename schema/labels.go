// Copyright 2025 The Prometheus Authors
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

package schema

import (
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	// Special label names and selectors for schema.Metadata fields.
	// They are currently private to ensure __name__, __type__ and __unit__ are used
	// together and remain extensible in Prometheus. See NewMetadataFromLabels and Metadata
	// methods for the interactions with the labels package structs.
	metricName = "__name__"
	metricType = "__type__"
	metricUnit = "__unit__"
)

// IsMetadataLabel returns true if the given label name is a special
// schema Metadata label.
func IsMetadataLabel(name string) bool {
	return name == metricName || name == metricType || name == metricUnit
}

// Metadata represents the core metric schema/metadata elements that:
// * are describing and identifying the metric schema/shape (e.g. name, type and unit).
// * are contributing to the general metric/series identity.
// * with the type-and-unit feature, are stored as Prometheus labels.
//
// Historically, similar information was encoded in the labels.MetricName (suffixes)
// and in the separate metadata.Metadata structures. However, with the
// type-and-unit-label feature (PROM-39), this information can be now stored directly
// in the special schema metadata labels, which offers better reliability (e.g. atomicity),
// compatibility and, in many cases, efficiency.
//
// NOTE: Metadata in the current form is generally similar (yet different) to:
//   - The MetricFamily definition in OpenMetrics (https://prometheus.io/docs/specs/om/open_metrics_spec/#metricfamily).
//     However, there is a small and important distinction around the metric name semantics
//     for the "classic" representation of complex metrics like histograms. The
//     Metadata.Name follows the __name__ semantics. See Name for details.
//   - Original metadata.Metadata entries. However, not all fields in that metadata
//     are "identifiable", notably the help field, plus metadata does not contain Name.
type Metadata struct {
	// Name represents the final metric name for a Prometheus series.
	// NOTE(bwplotka): Prometheus scrape formats (e.g. OpenMetrics) define
	// the "metric family name". The Metadata.Name (so __name__ label) is not
	// always the same as the MetricFamily.Name e.g.:
	// * OpenMetrics metric family name on scrape: "acme_http_router_request_seconds"
	// * Resulting Prometheus metric name: "acme_http_router_request_seconds_sum"
	//
	// Empty string means nameless metric (e.g. result of the PromQL function).
	Name string
	// Type represents the metric type. Empty value ("") is equivalent to
	// model.UnknownMetricType.
	Type model.MetricType
	// Unit represents the metric unit. Empty string means an unitless metric (e.g.
	// result of the PromQL function).
	//
	// NOTE: Currently unit value is not strictly defined other than OpenMetrics
	// recommendations: https://prometheus.io/docs/specs/om/open_metrics_spec/#units-and-base-units
	// TODO(bwplotka): Consider a stricter validation and rules e.g. lowercase only or UCUM standard.
	// Read more in https://github.com/prometheus/proposals/blob/main/proposals/2024-09-25_metadata-labels.md#more-strict-unit-and-type-value-definition
	Unit string
}

// NewMetadataFromLabels returns the schema metadata from the labels.
func NewMetadataFromLabels(ls labels.Labels) Metadata {
	typ := model.MetricTypeUnknown
	if got := ls.Get(metricType); got != "" {
		typ = model.MetricType(got)
	}
	return Metadata{
		Name: ls.Get(metricName),
		Type: typ,
		Unit: ls.Get(metricUnit),
	}
}

// IsTypeEmpty returns true if the metric type is empty (not set).
func (m Metadata) IsTypeEmpty() bool {
	return m.Type == "" || m.Type == model.MetricTypeUnknown
}

// IsEmptyFor returns true if the Metadata field, represented by the given labelName
// is empty (not set). If the labelName in not representing any Metadata field,
// IsEmptyFor returns true.
func (m Metadata) IsEmptyFor(labelName string) bool {
	switch labelName {
	case metricName:
		return m.Name == ""
	case metricType:
		return m.IsTypeEmpty()
	case metricUnit:
		return m.Unit == ""
	default:
		return true
	}
}

// AddToLabels adds metric schema metadata as labels into the labels.ScratchBuilder.
// Empty Metadata fields will be ignored (not added).
func (m Metadata) AddToLabels(b *labels.ScratchBuilder) {
	if m.Name != "" {
		b.Add(metricName, m.Name)
	}
	if !m.IsTypeEmpty() {
		b.Add(metricType, string(m.Type))
	}
	if m.Unit != "" {
		b.Add(metricUnit, m.Unit)
	}
}

// SetToLabels injects metric schema metadata as labels into the labels.Builder.
// It follows the labels.Builder.Set semantics, so empty Metadata fields will
// remove the corresponding existing labels if they were previously set.
func (m Metadata) SetToLabels(b *labels.Builder) {
	b.Set(metricName, m.Name)
	if m.Type == model.MetricTypeUnknown {
		// Unknown equals empty semantically, so remove the label on unknown too as per
		// method signature comment.
		b.Set(metricType, "")
	} else {
		b.Set(metricType, string(m.Type))
	}
	b.Set(metricUnit, m.Unit)
}

// NewIgnoreOverriddenMetadataLabelScratchBuilder creates IgnoreOverriddenMetadataLabelScratchBuilder.
func (m Metadata) NewIgnoreOverriddenMetadataLabelScratchBuilder(b *labels.ScratchBuilder) *IgnoreOverriddenMetadataLabelScratchBuilder {
	return &IgnoreOverriddenMetadataLabelScratchBuilder{ScratchBuilder: b, overwrite: m}
}

// IgnoreOverriddenMetadataLabelScratchBuilder is a wrapper over labels.ScratchBuilder
// that ignores label additions that would collide with non-empty Overwrite Metadata fields.
type IgnoreOverriddenMetadataLabelScratchBuilder struct {
	*labels.ScratchBuilder
	overwrite Metadata
}

// Add a name/value pair, unless it would collide with the non-empty Overwrite Metadata
// field. Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b IgnoreOverriddenMetadataLabelScratchBuilder) Add(name, value string) {
	if !b.overwrite.IsEmptyFor(name) {
		return
	}
	b.ScratchBuilder.Add(name, value)
}
