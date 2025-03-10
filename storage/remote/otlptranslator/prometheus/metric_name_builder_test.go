// Copyright 2024 The Prometheus Authors
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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheus/normalize_name_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheus

import (
	"testing"

	translator "github.com/ArthurSens/otlp-prometheus-translator"
	"github.com/stretchr/testify/require"
)

// Tests in this file don't test existing code, but rather the translator code.
// It's important to keep them up to ensure we don't get surprised by possible
// regressions in the translator code.

func TestBuildCompliantMetricNameWithSuffixes(t *testing.T) {
	require.Equal(t, "system_io_bytes_total", translator.BuildCompliantMetricName("system.io", "By", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "network_io_bytes_total", translator.BuildCompliantMetricName("network.io", "By", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "_3_14_digits", translator.BuildCompliantMetricName("3.14 digits", "", translator.MetricTypeGauge, true))
	require.Equal(t, "envoy_rule_engine_zlib_buf_error", translator.BuildCompliantMetricName("envoy__rule_engine_zlib_buf_error", "", translator.MetricTypeGauge, true))
	require.Equal(t, ":foo::bar", translator.BuildCompliantMetricName(":foo::bar", "", translator.MetricTypeGauge, true))
	require.Equal(t, ":foo::bar_total", translator.BuildCompliantMetricName(":foo::bar", "", translator.MetricTypeMonotonicCounter, true))
	// Gauges with unit 1 are considered ratios.
	require.Equal(t, "foo_bar_ratio", translator.BuildCompliantMetricName("foo.bar", "1", translator.MetricTypeGauge, true))
	// Slashes in units are converted.
	require.Equal(t, "system_io_foo_per_bar_total", translator.BuildCompliantMetricName("system.io", "foo/bar", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "metric_with_foreign_characters_total", translator.BuildCompliantMetricName("metric_with_字符_foreign_characters", "", translator.MetricTypeMonotonicCounter, true))
	// Removes non aplhanumerical characters from units, but leaves colons.
	require.Equal(t, "temperature_:C", translator.BuildCompliantMetricName("temperature", "%*()°:C", translator.MetricTypeGauge, true))
}

func TestBuildCompliantMetricNameWithoutSuffixes(t *testing.T) {
	require.Equal(t, "system_io", translator.BuildCompliantMetricName("system.io", "By", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "network_io", translator.BuildCompliantMetricName("network.io", "By", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "network_I_O", translator.BuildCompliantMetricName("network (I/O)", "By", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "_3_14_digits", translator.BuildCompliantMetricName("3.14 digits", "By", translator.MetricTypeGauge, false))
	require.Equal(t, "envoy__rule_engine_zlib_buf_error", translator.BuildCompliantMetricName("envoy__rule_engine_zlib_buf_error", "", translator.MetricTypeGauge, false))
	require.Equal(t, ":foo::bar", translator.BuildCompliantMetricName(":foo::bar", "", translator.MetricTypeGauge, false))
	require.Equal(t, ":foo::bar", translator.BuildCompliantMetricName(":foo::bar", "", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "foo_bar", translator.BuildCompliantMetricName("foo.bar", "1", translator.MetricTypeGauge, false))
	require.Equal(t, "system_io", translator.BuildCompliantMetricName("system.io", "foo/bar", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "metric_with___foreign_characters", translator.BuildCompliantMetricName("metric_with_字符_foreign_characters", "", translator.MetricTypeMonotonicCounter, false))
}

func TestBuildMetricNameWithSuffixes(t *testing.T) {
	require.Equal(t, "system.io_bytes_total", translator.BuildMetricName("system.io", "By", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "network.io_bytes_total", translator.BuildMetricName("network.io", "By", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "3.14 digits", translator.BuildMetricName("3.14 digits", "", translator.MetricTypeGauge, true))
	require.Equal(t, "envoy__rule_engine_zlib_buf_error", translator.BuildMetricName("envoy__rule_engine_zlib_buf_error", "", translator.MetricTypeGauge, true))
	require.Equal(t, ":foo::bar", translator.BuildMetricName(":foo::bar", "", translator.MetricTypeGauge, true))
	require.Equal(t, ":foo::bar_total", translator.BuildMetricName(":foo::bar", "", translator.MetricTypeMonotonicCounter, true))
	// Gauges with unit 1 are considered ratios.
	require.Equal(t, "foo.bar_ratio", translator.BuildMetricName("foo.bar", "1", translator.MetricTypeGauge, true))
	// Slashes in units are converted.
	require.Equal(t, "system.io_foo_per_bar_total", translator.BuildMetricName("system.io", "foo/bar", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "metric_with_字符_foreign_characters_total", translator.BuildMetricName("metric_with_字符_foreign_characters", "", translator.MetricTypeMonotonicCounter, true))
	require.Equal(t, "temperature_%*()°C", translator.BuildMetricName("temperature", "%*()°C", translator.MetricTypeGauge, true)) // Keeps the all characters in unit
	// Tests below show weird interactions that users can have with the metric names.
	// With BuildMetricName we don't check if units/type suffixes are already present in the metric name, we always add them.
	require.Equal(t, "system_io_seconds_seconds", translator.BuildMetricName("system_io_seconds", "s", translator.MetricTypeGauge, true))
	require.Equal(t, "system_io_total_total", translator.BuildMetricName("system_io_total", "", translator.MetricTypeMonotonicCounter, true))
}

func TestBuildMetricNameWithoutSuffixes(t *testing.T) {
	require.Equal(t, "system.io", translator.BuildMetricName("system.io", "By", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "3.14 digits", translator.BuildMetricName("3.14 digits", "", translator.MetricTypeGauge, false))
	require.Equal(t, "envoy__rule_engine_zlib_buf_error", translator.BuildMetricName("envoy__rule_engine_zlib_buf_error", "", translator.MetricTypeGauge, false))
	require.Equal(t, ":foo::bar", translator.BuildMetricName(":foo::bar", "", translator.MetricTypeGauge, false))
	require.Equal(t, ":foo::bar", translator.BuildMetricName(":foo::bar", "", translator.MetricTypeMonotonicCounter, false))
	// Gauges with unit 1 are considered ratios.
	require.Equal(t, "foo.bar", translator.BuildMetricName("foo.bar", "1", translator.MetricTypeGauge, false))
	require.Equal(t, "metric_with_字符_foreign_characters", translator.BuildMetricName("metric_with_字符_foreign_characters", "", translator.MetricTypeMonotonicCounter, false))
	require.Equal(t, "system_io_seconds", translator.BuildMetricName("system_io_seconds", "s", translator.MetricTypeGauge, false))
	require.Equal(t, "system_io_total", translator.BuildMetricName("system_io_total", "", translator.MetricTypeMonotonicCounter, false))
}
