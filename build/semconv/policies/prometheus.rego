# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Prometheus Annotation Validation Policy
# Validates that histogram metrics have proper annotations for Prometheus code generation.

package before_resolution

import rego.v1

# =============================================================================
# Rule 1: Histogram metrics MUST have annotations.prometheus.histogram_type
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' has instrument=histogram but is missing annotations.prometheus.histogram_type. Please add one of: classic_histogram, native_histogram, mixed_histogram, or summary", [group.metric_name]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    not has_histogram_type(group)
}

# =============================================================================
# Rule 2: histogram_type must be a valid value
# Valid values: classic_histogram, native_histogram, mixed_histogram, summary
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' has invalid histogram_type. Must be one of: classic_histogram, native_histogram, mixed_histogram, summary", [group.metric_name]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    has_histogram_type(group)
    not valid_histogram_type(group.annotations.prometheus.histogram_type)
}

# =============================================================================
# Rule 3: classic_histogram and mixed_histogram MUST have buckets
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' with histogram_type=%s requires annotations.prometheus.buckets", [group.metric_name, group.annotations.prometheus.histogram_type]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    requires_buckets(group.annotations.prometheus.histogram_type)
    not group.annotations.prometheus.buckets
}

# =============================================================================
# Rule 4: native_histogram and mixed_histogram MUST have bucket_factor
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' with histogram_type=%s requires annotations.prometheus.bucket_factor", [group.metric_name, group.annotations.prometheus.histogram_type]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    requires_native_opts(group.annotations.prometheus.histogram_type)
    not group.annotations.prometheus.bucket_factor
}

# =============================================================================
# Rule 5: native_histogram and mixed_histogram MUST have max_bucket_number
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' with histogram_type=%s requires annotations.prometheus.max_bucket_number", [group.metric_name, group.annotations.prometheus.histogram_type]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    requires_native_opts(group.annotations.prometheus.histogram_type)
    not group.annotations.prometheus.max_bucket_number
}

# =============================================================================
# Rule 6: native_histogram and mixed_histogram MUST have min_reset_duration
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' with histogram_type=%s requires annotations.prometheus.min_reset_duration", [group.metric_name, group.annotations.prometheus.histogram_type]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    requires_native_opts(group.annotations.prometheus.histogram_type)
    not group.annotations.prometheus.min_reset_duration
}

# =============================================================================
# Rule 7: summary MUST have objectives
# =============================================================================
deny contains metric_violation(
    sprintf("Metric '%s' with histogram_type=summary requires annotations.prometheus.objectives", [group.metric_name]),
    group.id,
    group.metric_name
) if {
    group := input.groups[_]
    group.type == "metric"
    group.instrument == "histogram"
    group.annotations.prometheus.histogram_type == "summary"
    not group.annotations.prometheus.objectives
}

# =============================================================================
# Helper Functions
# =============================================================================

# Check if group has histogram_type annotation
has_histogram_type(group) if {
    group.annotations.prometheus.histogram_type
}

# Valid histogram_type values
valid_histogram_type(t) if t == "classic_histogram"
valid_histogram_type(t) if t == "native_histogram"
valid_histogram_type(t) if t == "mixed_histogram"
valid_histogram_type(t) if t == "summary"

# Types that require buckets (classic and mixed)
requires_buckets(t) if t == "classic_histogram"
requires_buckets(t) if t == "mixed_histogram"

# Types that require native histogram options (native and mixed)
requires_native_opts(t) if t == "native_histogram"
requires_native_opts(t) if t == "mixed_histogram"

# Build a metric violation object matching Weaver's expected format
# Using the same structure as attr_violation in otel_policies.rego
metric_violation(description, group_id, metric_name) := violation if {
    violation := {
        "id": description,
        "type": "semconv_attribute",
        "category": "prometheus_annotation",
        "group": group_id,
        "attr": metric_name,
    }
}
