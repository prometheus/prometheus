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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestByte(t *testing.T) {
	require.Equal(t, "system_filesystem_usage_bytes", normalizeName(createGauge("system.filesystem.usage", "By"), ""))
}

func TestByteCounter(t *testing.T) {
	require.Equal(t, "system_io_bytes_total", normalizeName(createCounter("system.io", "By"), ""))
	require.Equal(t, "network_transmitted_bytes_total", normalizeName(createCounter("network_transmitted_bytes_total", "By"), ""))
}

func TestWhiteSpaces(t *testing.T) {
	require.Equal(t, "system_filesystem_usage_bytes", normalizeName(createGauge("\t system.filesystem.usage       ", "  By\t"), ""))
}

func TestNonStandardUnit(t *testing.T) {
	require.Equal(t, "system_network_dropped", normalizeName(createGauge("system.network.dropped", "{packets}"), ""))
}

func TestNonStandardUnitCounter(t *testing.T) {
	require.Equal(t, "system_network_dropped_total", normalizeName(createCounter("system.network.dropped", "{packets}"), ""))
}

func TestBrokenUnit(t *testing.T) {
	require.Equal(t, "system_network_dropped_packets", normalizeName(createGauge("system.network.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets_dropped", normalizeName(createGauge("system.network.packets.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets", normalizeName(createGauge("system.network.packets", "packets"), ""))
}

func TestBrokenUnitCounter(t *testing.T) {
	require.Equal(t, "system_network_dropped_packets_total", normalizeName(createCounter("system.network.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets_dropped_total", normalizeName(createCounter("system.network.packets.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets_total", normalizeName(createCounter("system.network.packets", "packets"), ""))
}

func TestRatio(t *testing.T) {
	require.Equal(t, "hw_gpu_memory_utilization_ratio", normalizeName(createGauge("hw.gpu.memory.utilization", "1"), ""))
	require.Equal(t, "hw_fan_speed_ratio", normalizeName(createGauge("hw.fan.speed_ratio", "1"), ""))
	require.Equal(t, "objects_total", normalizeName(createCounter("objects", "1"), ""))
}

func TestHertz(t *testing.T) {
	require.Equal(t, "hw_cpu_speed_limit_hertz", normalizeName(createGauge("hw.cpu.speed_limit", "Hz"), ""))
}

func TestPer(t *testing.T) {
	require.Equal(t, "broken_metric_speed_km_per_hour", normalizeName(createGauge("broken.metric.speed", "km/h"), ""))
	require.Equal(t, "astro_light_speed_limit_meters_per_second", normalizeName(createGauge("astro.light.speed_limit", "m/s"), ""))
}

func TestPercent(t *testing.T) {
	require.Equal(t, "broken_metric_success_ratio_percent", normalizeName(createGauge("broken.metric.success_ratio", "%"), ""))
	require.Equal(t, "broken_metric_success_percent", normalizeName(createGauge("broken.metric.success_percent", "%"), ""))
}

func TestEmpty(t *testing.T) {
	require.Equal(t, "test_metric_no_unit", normalizeName(createGauge("test.metric.no_unit", ""), ""))
	require.Equal(t, "test_metric_spaces", normalizeName(createGauge("test.metric.spaces", "   \t  "), ""))
}

func TestOTelReceivers(t *testing.T) {
	require.Equal(t, "active_directory_ds_replication_network_io_bytes_total", normalizeName(createCounter("active_directory.ds.replication.network.io", "By"), ""))
	require.Equal(t, "active_directory_ds_replication_sync_object_pending_total", normalizeName(createCounter("active_directory.ds.replication.sync.object.pending", "{objects}"), ""))
	require.Equal(t, "active_directory_ds_replication_object_rate_per_second", normalizeName(createGauge("active_directory.ds.replication.object.rate", "{objects}/s"), ""))
	require.Equal(t, "active_directory_ds_name_cache_hit_rate_percent", normalizeName(createGauge("active_directory.ds.name_cache.hit_rate", "%"), ""))
	require.Equal(t, "active_directory_ds_ldap_bind_last_successful_time_milliseconds", normalizeName(createGauge("active_directory.ds.ldap.bind.last_successful.time", "ms"), ""))
	require.Equal(t, "apache_current_connections", normalizeName(createGauge("apache.current_connections", "connections"), ""))
	require.Equal(t, "apache_workers_connections", normalizeName(createGauge("apache.workers", "connections"), ""))
	require.Equal(t, "apache_requests_total", normalizeName(createCounter("apache.requests", "1"), ""))
	require.Equal(t, "bigip_virtual_server_request_count_total", normalizeName(createCounter("bigip.virtual_server.request.count", "{requests}"), ""))
	require.Equal(t, "system_cpu_utilization_ratio", normalizeName(createGauge("system.cpu.utilization", "1"), ""))
	require.Equal(t, "system_disk_operation_time_seconds_total", normalizeName(createCounter("system.disk.operation_time", "s"), ""))
	require.Equal(t, "system_cpu_load_average_15m_ratio", normalizeName(createGauge("system.cpu.load_average.15m", "1"), ""))
	require.Equal(t, "memcached_operation_hit_ratio_percent", normalizeName(createGauge("memcached.operation_hit_ratio", "%"), ""))
	require.Equal(t, "mongodbatlas_process_asserts_per_second", normalizeName(createGauge("mongodbatlas.process.asserts", "{assertions}/s"), ""))
	require.Equal(t, "mongodbatlas_process_journaling_data_files_mebibytes", normalizeName(createGauge("mongodbatlas.process.journaling.data_files", "MiBy"), ""))
	require.Equal(t, "mongodbatlas_process_network_io_bytes_per_second", normalizeName(createGauge("mongodbatlas.process.network.io", "By/s"), ""))
	require.Equal(t, "mongodbatlas_process_oplog_rate_gibibytes_per_hour", normalizeName(createGauge("mongodbatlas.process.oplog.rate", "GiBy/h"), ""))
	require.Equal(t, "mongodbatlas_process_db_query_targeting_scanned_per_returned", normalizeName(createGauge("mongodbatlas.process.db.query_targeting.scanned_per_returned", "{scanned}/{returned}"), ""))
	require.Equal(t, "nginx_requests", normalizeName(createGauge("nginx.requests", "requests"), ""))
	require.Equal(t, "nginx_connections_accepted", normalizeName(createGauge("nginx.connections_accepted", "connections"), ""))
	require.Equal(t, "nsxt_node_memory_usage_kilobytes", normalizeName(createGauge("nsxt.node.memory.usage", "KBy"), ""))
	require.Equal(t, "redis_latest_fork_microseconds", normalizeName(createGauge("redis.latest_fork", "us"), ""))
}

func TestTrimPromSuffixes(t *testing.T) {
	assert.Equal(t, "active_directory_ds_replication_network_io", TrimPromSuffixes("active_directory_ds_replication_network_io_bytes_total", pmetric.MetricTypeSum, "bytes"))
	assert.Equal(t, "active_directory_ds_name_cache_hit_rate", TrimPromSuffixes("active_directory_ds_name_cache_hit_rate_percent", pmetric.MetricTypeGauge, "percent"))
	assert.Equal(t, "active_directory_ds_ldap_bind_last_successful_time", TrimPromSuffixes("active_directory_ds_ldap_bind_last_successful_time_milliseconds", pmetric.MetricTypeGauge, "milliseconds"))
	assert.Equal(t, "apache_requests", TrimPromSuffixes("apache_requests_total", pmetric.MetricTypeSum, "1"))
	assert.Equal(t, "system_cpu_utilization", TrimPromSuffixes("system_cpu_utilization_ratio", pmetric.MetricTypeGauge, "ratio"))
	assert.Equal(t, "mongodbatlas_process_journaling_data_files", TrimPromSuffixes("mongodbatlas_process_journaling_data_files_mebibytes", pmetric.MetricTypeGauge, "mebibytes"))
	assert.Equal(t, "mongodbatlas_process_network_io", TrimPromSuffixes("mongodbatlas_process_network_io_bytes_per_second", pmetric.MetricTypeGauge, "bytes_per_second"))
	assert.Equal(t, "mongodbatlas_process_oplog_rate", TrimPromSuffixes("mongodbatlas_process_oplog_rate_gibibytes_per_hour", pmetric.MetricTypeGauge, "gibibytes_per_hour"))
	assert.Equal(t, "nsxt_node_memory_usage", TrimPromSuffixes("nsxt_node_memory_usage_kilobytes", pmetric.MetricTypeGauge, "kilobytes"))
	assert.Equal(t, "redis_latest_fork", TrimPromSuffixes("redis_latest_fork_microseconds", pmetric.MetricTypeGauge, "microseconds"))
	assert.Equal(t, "up", TrimPromSuffixes("up", pmetric.MetricTypeGauge, ""))

	// These are not necessarily valid OM units, only tested for the sake of completeness.
	assert.Equal(t, "active_directory_ds_replication_sync_object_pending", TrimPromSuffixes("active_directory_ds_replication_sync_object_pending_total", pmetric.MetricTypeSum, "{objects}"))
	assert.Equal(t, "apache_current", TrimPromSuffixes("apache_current_connections", pmetric.MetricTypeGauge, "connections"))
	assert.Equal(t, "bigip_virtual_server_request_count", TrimPromSuffixes("bigip_virtual_server_request_count_total", pmetric.MetricTypeSum, "{requests}"))
	assert.Equal(t, "mongodbatlas_process_db_query_targeting_scanned_per_returned", TrimPromSuffixes("mongodbatlas_process_db_query_targeting_scanned_per_returned", pmetric.MetricTypeGauge, "{scanned}/{returned}"))
	assert.Equal(t, "nginx_connections_accepted", TrimPromSuffixes("nginx_connections_accepted", pmetric.MetricTypeGauge, "connections"))
	assert.Equal(t, "apache_workers", TrimPromSuffixes("apache_workers_connections", pmetric.MetricTypeGauge, "connections"))
	assert.Equal(t, "nginx", TrimPromSuffixes("nginx_requests", pmetric.MetricTypeGauge, "requests"))

	// Units shouldn't be trimmed if the unit is not a direct match with the suffix, i.e, a suffix "_seconds" shouldn't be removed if unit is "sec" or "s"
	assert.Equal(t, "system_cpu_load_average_15m_ratio", TrimPromSuffixes("system_cpu_load_average_15m_ratio", pmetric.MetricTypeGauge, "1"))
	assert.Equal(t, "mongodbatlas_process_asserts_per_second", TrimPromSuffixes("mongodbatlas_process_asserts_per_second", pmetric.MetricTypeGauge, "{assertions}/s"))
	assert.Equal(t, "memcached_operation_hit_ratio_percent", TrimPromSuffixes("memcached_operation_hit_ratio_percent", pmetric.MetricTypeGauge, "%"))
	assert.Equal(t, "active_directory_ds_replication_object_rate_per_second", TrimPromSuffixes("active_directory_ds_replication_object_rate_per_second", pmetric.MetricTypeGauge, "{objects}/s"))
	assert.Equal(t, "system_disk_operation_time_seconds", TrimPromSuffixes("system_disk_operation_time_seconds_total", pmetric.MetricTypeSum, "s"))
}

func TestNamespace(t *testing.T) {
	require.Equal(t, "space_test", normalizeName(createGauge("test", ""), "space"))
	require.Equal(t, "space_test", normalizeName(createGauge("#test", ""), "space"))
}

func TestCleanUpUnit(t *testing.T) {
	require.Equal(t, "", cleanUpUnit(""))
	require.Equal(t, "a_b", cleanUpUnit("a b"))
	require.Equal(t, "hello_world", cleanUpUnit("hello, world"))
	require.Equal(t, "hello_you_2", cleanUpUnit("hello you 2"))
	require.Equal(t, "1000", cleanUpUnit("$1000"))
	require.Equal(t, "", cleanUpUnit("*+$^=)"))
}

func TestUnitMapGetOrDefault(t *testing.T) {
	require.Equal(t, "", unitMapGetOrDefault(""))
	require.Equal(t, "seconds", unitMapGetOrDefault("s"))
	require.Equal(t, "invalid", unitMapGetOrDefault("invalid"))
}

func TestPerUnitMapGetOrDefault(t *testing.T) {
	require.Equal(t, "", perUnitMapGetOrDefault(""))
	require.Equal(t, "second", perUnitMapGetOrDefault("s"))
	require.Equal(t, "invalid", perUnitMapGetOrDefault("invalid"))
}

func TestRemoveItem(t *testing.T) {
	require.Equal(t, []string{}, removeItem([]string{}, "test"))
	require.Equal(t, []string{}, removeItem([]string{}, ""))
	require.Equal(t, []string{"a", "b", "c"}, removeItem([]string{"a", "b", "c"}, "d"))
	require.Equal(t, []string{"a", "b", "c"}, removeItem([]string{"a", "b", "c"}, ""))
	require.Equal(t, []string{"a", "b"}, removeItem([]string{"a", "b", "c"}, "c"))
	require.Equal(t, []string{"a", "c"}, removeItem([]string{"a", "b", "c"}, "b"))
	require.Equal(t, []string{"b", "c"}, removeItem([]string{"a", "b", "c"}, "a"))
}

func TestBuildCompliantMetricNameWithSuffixes(t *testing.T) {
	require.Equal(t, "system_io_bytes_total", BuildCompliantMetricName(createCounter("system.io", "By"), "", true))
	require.Equal(t, "system_network_io_bytes_total", BuildCompliantMetricName(createCounter("network.io", "By"), "system", true))
	require.Equal(t, "_3_14_digits", BuildCompliantMetricName(createGauge("3.14 digits", ""), "", true))
	require.Equal(t, "envoy_rule_engine_zlib_buf_error", BuildCompliantMetricName(createGauge("envoy__rule_engine_zlib_buf_error", ""), "", true))
	require.Equal(t, ":foo::bar", BuildCompliantMetricName(createGauge(":foo::bar", ""), "", true))
	require.Equal(t, ":foo::bar_total", BuildCompliantMetricName(createCounter(":foo::bar", ""), "", true))
	// Gauges with unit 1 are considered ratios.
	require.Equal(t, "foo_bar_ratio", BuildCompliantMetricName(createGauge("foo.bar", "1"), "", true))
	// Slashes in units are converted.
	require.Equal(t, "system_io_foo_per_bar_total", BuildCompliantMetricName(createCounter("system.io", "foo/bar"), "", true))
	require.Equal(t, "metric_with_foreign_characters_total", BuildCompliantMetricName(createCounter("metric_with_字符_foreign_characters", ""), "", true))
}

func TestBuildCompliantMetricNameWithoutSuffixes(t *testing.T) {
	require.Equal(t, "system_io", BuildCompliantMetricName(createCounter("system.io", "By"), "", false))
	require.Equal(t, "system_network_io", BuildCompliantMetricName(createCounter("network.io", "By"), "system", false))
	require.Equal(t, "system_network_I_O", BuildCompliantMetricName(createCounter("network (I/O)", "By"), "system", false))
	require.Equal(t, "_3_14_digits", BuildCompliantMetricName(createGauge("3.14 digits", "By"), "", false))
	require.Equal(t, "envoy__rule_engine_zlib_buf_error", BuildCompliantMetricName(createGauge("envoy__rule_engine_zlib_buf_error", ""), "", false))
	require.Equal(t, ":foo::bar", BuildCompliantMetricName(createGauge(":foo::bar", ""), "", false))
	require.Equal(t, ":foo::bar", BuildCompliantMetricName(createCounter(":foo::bar", ""), "", false))
	require.Equal(t, "foo_bar", BuildCompliantMetricName(createGauge("foo.bar", "1"), "", false))
	require.Equal(t, "system_io", BuildCompliantMetricName(createCounter("system.io", "foo/bar"), "", false))
	require.Equal(t, "metric_with___foreign_characters", BuildCompliantMetricName(createCounter("metric_with_字符_foreign_characters", ""), "", false))
}

func TestBuildMetricNameWithSuffixes(t *testing.T) {
	require.Equal(t, "system.io_bytes_total", BuildMetricName(createCounter("system.io", "By"), "", true))
	require.Equal(t, "system_network.io_bytes_total", BuildMetricName(createCounter("network.io", "By"), "system", true))
	require.Equal(t, "3.14 digits", BuildMetricName(createGauge("3.14 digits", ""), "", true))
	require.Equal(t, "envoy__rule_engine_zlib_buf_error", BuildMetricName(createGauge("envoy__rule_engine_zlib_buf_error", ""), "", true))
	require.Equal(t, ":foo::bar", BuildMetricName(createGauge(":foo::bar", ""), "", true))
	require.Equal(t, ":foo::bar_total", BuildMetricName(createCounter(":foo::bar", ""), "", true))
	// Gauges with unit 1 are considered ratios.
	require.Equal(t, "foo.bar_ratio", BuildMetricName(createGauge("foo.bar", "1"), "", true))
	// Slashes in units are converted.
	require.Equal(t, "system.io_foo_per_bar_total", BuildMetricName(createCounter("system.io", "foo/bar"), "", true))
	require.Equal(t, "metric_with_字符_foreign_characters_total", BuildMetricName(createCounter("metric_with_字符_foreign_characters", ""), "", true))

	// Tests below show weird interactions that users can have with the metric names.
	// With BuildMetricName we don't check if units/type suffixes are already present in the metric name, we always add them.
	require.Equal(t, "system_io_seconds_seconds", BuildMetricName(createGauge("system_io_seconds", "s"), "", true))
	require.Equal(t, "system_io_total_total", BuildMetricName(createCounter("system_io_total", ""), "", true))
}

func TestBuildMetricNameWithoutSuffixes(t *testing.T) {
	require.Equal(t, "system.io", BuildMetricName(createCounter("system.io", "By"), "", false))
	require.Equal(t, "system_network.io", BuildMetricName(createCounter("network.io", "By"), "system", false))
	require.Equal(t, "3.14 digits", BuildMetricName(createGauge("3.14 digits", ""), "", false))
	require.Equal(t, "envoy__rule_engine_zlib_buf_error", BuildMetricName(createGauge("envoy__rule_engine_zlib_buf_error", ""), "", false))
	require.Equal(t, ":foo::bar", BuildMetricName(createGauge(":foo::bar", ""), "", false))
	require.Equal(t, ":foo::bar", BuildMetricName(createCounter(":foo::bar", ""), "", false))
	// Gauges with unit 1 are considered ratios.
	require.Equal(t, "foo.bar", BuildMetricName(createGauge("foo.bar", "1"), "", false))
	// Slashes in units are converted.
	require.Equal(t, "system.io", BuildMetricName(createCounter("system.io", "foo/bar"), "", false))
	require.Equal(t, "metric_with_字符_foreign_characters", BuildMetricName(createCounter("metric_with_字符_foreign_characters", ""), "", false))

	require.Equal(t, "system_io_seconds", BuildMetricName(createGauge("system_io_seconds", "s"), "", false))
	require.Equal(t, "system_io_total", BuildMetricName(createCounter("system_io_total", ""), "", false))
}
