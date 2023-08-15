// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestByte(t *testing.T) {
	require.Equal(t, "system_filesystem_usage_bytes", BuildPromCompliantName(createGauge("system.filesystem.usage", "By"), ""))
}

func TestByteCounter(t *testing.T) {
	require.Equal(t, "system_io_bytes_total", BuildPromCompliantName(createCounter("system.io", "By"), ""))
	require.Equal(t, "network_transmitted_bytes_total", BuildPromCompliantName(createCounter("network_transmitted_bytes_total", "By"), ""))
}

func TestWhiteSpaces(t *testing.T) {
	require.Equal(t, "system_filesystem_usage_bytes", BuildPromCompliantName(createGauge("\t system.filesystem.usage       ", "  By\t"), ""))
}

func TestNonStandardUnit(t *testing.T) {
	require.Equal(t, "system_network_dropped", BuildPromCompliantName(createGauge("system.network.dropped", "{packets}"), ""))
}

func TestNonStandardUnitCounter(t *testing.T) {
	require.Equal(t, "system_network_dropped_total", BuildPromCompliantName(createCounter("system.network.dropped", "{packets}"), ""))
}

func TestBrokenUnit(t *testing.T) {
	require.Equal(t, "system_network_dropped_packets", BuildPromCompliantName(createGauge("system.network.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets_dropped", BuildPromCompliantName(createGauge("system.network.packets.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets", BuildPromCompliantName(createGauge("system.network.packets", "packets"), ""))
}

func TestBrokenUnitCounter(t *testing.T) {
	require.Equal(t, "system_network_dropped_packets_total", BuildPromCompliantName(createCounter("system.network.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets_dropped_total", BuildPromCompliantName(createCounter("system.network.packets.dropped", "packets"), ""))
	require.Equal(t, "system_network_packets_total", BuildPromCompliantName(createCounter("system.network.packets", "packets"), ""))
}

func TestRatio(t *testing.T) {
	require.Equal(t, "hw_gpu_memory_utilization_ratio", BuildPromCompliantName(createGauge("hw.gpu.memory.utilization", "1"), ""))
	require.Equal(t, "hw_fan_speed_ratio", BuildPromCompliantName(createGauge("hw.fan.speed_ratio", "1"), ""))
	require.Equal(t, "objects_total", BuildPromCompliantName(createCounter("objects", "1"), ""))
}

func TestHertz(t *testing.T) {
	require.Equal(t, "hw_cpu_speed_limit_hertz", BuildPromCompliantName(createGauge("hw.cpu.speed_limit", "Hz"), ""))
}

func TestPer(t *testing.T) {
	require.Equal(t, "broken_metric_speed_km_per_hour", BuildPromCompliantName(createGauge("broken.metric.speed", "km/h"), ""))
	require.Equal(t, "astro_light_speed_limit_meters_per_second", BuildPromCompliantName(createGauge("astro.light.speed_limit", "m/s"), ""))
}

func TestPercent(t *testing.T) {
	require.Equal(t, "broken_metric_success_ratio_percent", BuildPromCompliantName(createGauge("broken.metric.success_ratio", "%"), ""))
	require.Equal(t, "broken_metric_success_percent", BuildPromCompliantName(createGauge("broken.metric.success_percent", "%"), ""))
}

func TestDollar(t *testing.T) {
	require.Equal(t, "crypto_bitcoin_value_dollars", BuildPromCompliantName(createGauge("crypto.bitcoin.value", "$"), ""))
	require.Equal(t, "crypto_bitcoin_value_dollars", BuildPromCompliantName(createGauge("crypto.bitcoin.value.dollars", "$"), ""))
}

func TestEmpty(t *testing.T) {
	require.Equal(t, "test_metric_no_unit", BuildPromCompliantName(createGauge("test.metric.no_unit", ""), ""))
	require.Equal(t, "test_metric_spaces", BuildPromCompliantName(createGauge("test.metric.spaces", "   \t  "), ""))
}

func TestUnsupportedRunes(t *testing.T) {
	require.Equal(t, "unsupported_metric_temperature_F", BuildPromCompliantName(createGauge("unsupported.metric.temperature", "°F"), ""))
	require.Equal(t, "unsupported_metric_weird", BuildPromCompliantName(createGauge("unsupported.metric.weird", "+=.:,!* & #"), ""))
	require.Equal(t, "unsupported_metric_redundant_test_per_C", BuildPromCompliantName(createGauge("unsupported.metric.redundant", "__test $/°C"), ""))
}

func TestOtelReceivers(t *testing.T) {
	require.Equal(t, "active_directory_ds_replication_network_io_bytes_total", BuildPromCompliantName(createCounter("active_directory.ds.replication.network.io", "By"), ""))
	require.Equal(t, "active_directory_ds_replication_sync_object_pending_total", BuildPromCompliantName(createCounter("active_directory.ds.replication.sync.object.pending", "{objects}"), ""))
	require.Equal(t, "active_directory_ds_replication_object_rate_per_second", BuildPromCompliantName(createGauge("active_directory.ds.replication.object.rate", "{objects}/s"), ""))
	require.Equal(t, "active_directory_ds_name_cache_hit_rate_percent", BuildPromCompliantName(createGauge("active_directory.ds.name_cache.hit_rate", "%"), ""))
	require.Equal(t, "active_directory_ds_ldap_bind_last_successful_time_milliseconds", BuildPromCompliantName(createGauge("active_directory.ds.ldap.bind.last_successful.time", "ms"), ""))
	require.Equal(t, "apache_current_connections", BuildPromCompliantName(createGauge("apache.current_connections", "connections"), ""))
	require.Equal(t, "apache_workers_connections", BuildPromCompliantName(createGauge("apache.workers", "connections"), ""))
	require.Equal(t, "apache_requests_total", BuildPromCompliantName(createCounter("apache.requests", "1"), ""))
	require.Equal(t, "bigip_virtual_server_request_count_total", BuildPromCompliantName(createCounter("bigip.virtual_server.request.count", "{requests}"), ""))
	require.Equal(t, "system_cpu_utilization_ratio", BuildPromCompliantName(createGauge("system.cpu.utilization", "1"), ""))
	require.Equal(t, "system_disk_operation_time_seconds_total", BuildPromCompliantName(createCounter("system.disk.operation_time", "s"), ""))
	require.Equal(t, "system_cpu_load_average_15m_ratio", BuildPromCompliantName(createGauge("system.cpu.load_average.15m", "1"), ""))
	require.Equal(t, "memcached_operation_hit_ratio_percent", BuildPromCompliantName(createGauge("memcached.operation_hit_ratio", "%"), ""))
	require.Equal(t, "mongodbatlas_process_asserts_per_second", BuildPromCompliantName(createGauge("mongodbatlas.process.asserts", "{assertions}/s"), ""))
	require.Equal(t, "mongodbatlas_process_journaling_data_files_mebibytes", BuildPromCompliantName(createGauge("mongodbatlas.process.journaling.data_files", "MiBy"), ""))
	require.Equal(t, "mongodbatlas_process_network_io_bytes_per_second", BuildPromCompliantName(createGauge("mongodbatlas.process.network.io", "By/s"), ""))
	require.Equal(t, "mongodbatlas_process_oplog_rate_gibibytes_per_hour", BuildPromCompliantName(createGauge("mongodbatlas.process.oplog.rate", "GiBy/h"), ""))
	require.Equal(t, "mongodbatlas_process_db_query_targeting_scanned_per_returned", BuildPromCompliantName(createGauge("mongodbatlas.process.db.query_targeting.scanned_per_returned", "{scanned}/{returned}"), ""))
	require.Equal(t, "nginx_requests", BuildPromCompliantName(createGauge("nginx.requests", "requests"), ""))
	require.Equal(t, "nginx_connections_accepted", BuildPromCompliantName(createGauge("nginx.connections_accepted", "connections"), ""))
	require.Equal(t, "nsxt_node_memory_usage_kilobytes", BuildPromCompliantName(createGauge("nsxt.node.memory.usage", "KBy"), ""))
	require.Equal(t, "redis_latest_fork_microseconds", BuildPromCompliantName(createGauge("redis.latest_fork", "us"), ""))
}

func TestTrimPromSuffixes(t *testing.T) {
	require.Equal(t, "active_directory_ds_replication_network_io", TrimPromSuffixes("active_directory_ds_replication_network_io_bytes_total", pmetric.MetricTypeSum, "bytes"))
	require.Equal(t, "active_directory_ds_name_cache_hit_rate", TrimPromSuffixes("active_directory_ds_name_cache_hit_rate_percent", pmetric.MetricTypeGauge, "percent"))
	require.Equal(t, "active_directory_ds_ldap_bind_last_successful_time", TrimPromSuffixes("active_directory_ds_ldap_bind_last_successful_time_milliseconds", pmetric.MetricTypeGauge, "milliseconds"))
	require.Equal(t, "apache_requests", TrimPromSuffixes("apache_requests_total", pmetric.MetricTypeSum, "1"))
	require.Equal(t, "system_cpu_utilization", TrimPromSuffixes("system_cpu_utilization_ratio", pmetric.MetricTypeGauge, "ratio"))
	require.Equal(t, "mongodbatlas_process_journaling_data_files", TrimPromSuffixes("mongodbatlas_process_journaling_data_files_mebibytes", pmetric.MetricTypeGauge, "mebibytes"))
	require.Equal(t, "mongodbatlas_process_network_io", TrimPromSuffixes("mongodbatlas_process_network_io_bytes_per_second", pmetric.MetricTypeGauge, "bytes_per_second"))
	require.Equal(t, "mongodbatlas_process_oplog_rate", TrimPromSuffixes("mongodbatlas_process_oplog_rate_gibibytes_per_hour", pmetric.MetricTypeGauge, "gibibytes_per_hour"))
	require.Equal(t, "nsxt_node_memory_usage", TrimPromSuffixes("nsxt_node_memory_usage_kilobytes", pmetric.MetricTypeGauge, "kilobytes"))
	require.Equal(t, "redis_latest_fork", TrimPromSuffixes("redis_latest_fork_microseconds", pmetric.MetricTypeGauge, "microseconds"))
	require.Equal(t, "up", TrimPromSuffixes("up", pmetric.MetricTypeGauge, ""))

	// These are not necessarily valid OM units, only tested for the sake of completeness.
	require.Equal(t, "active_directory_ds_replication_sync_object_pending", TrimPromSuffixes("active_directory_ds_replication_sync_object_pending_total", pmetric.MetricTypeSum, "{objects}"))
	require.Equal(t, "apache_current", TrimPromSuffixes("apache_current_connections", pmetric.MetricTypeGauge, "connections"))
	require.Equal(t, "bigip_virtual_server_request_count", TrimPromSuffixes("bigip_virtual_server_request_count_total", pmetric.MetricTypeSum, "{requests}"))
	require.Equal(t, "mongodbatlas_process_db_query_targeting_scanned_per_returned", TrimPromSuffixes("mongodbatlas_process_db_query_targeting_scanned_per_returned", pmetric.MetricTypeGauge, "{scanned}/{returned}"))
	require.Equal(t, "nginx_connections_accepted", TrimPromSuffixes("nginx_connections_accepted", pmetric.MetricTypeGauge, "connections"))
	require.Equal(t, "apache_workers", TrimPromSuffixes("apache_workers_connections", pmetric.MetricTypeGauge, "connections"))
	require.Equal(t, "nginx", TrimPromSuffixes("nginx_requests", pmetric.MetricTypeGauge, "requests"))

	// Units shouldn't be trimmed if the unit is not a direct match with the suffix, i.e, a suffix "_seconds" shouldn't be removed if unit is "sec" or "s"
	require.Equal(t, "system_cpu_load_average_15m_ratio", TrimPromSuffixes("system_cpu_load_average_15m_ratio", pmetric.MetricTypeGauge, "1"))
	require.Equal(t, "mongodbatlas_process_asserts_per_second", TrimPromSuffixes("mongodbatlas_process_asserts_per_second", pmetric.MetricTypeGauge, "{assertions}/s"))
	require.Equal(t, "memcached_operation_hit_ratio_percent", TrimPromSuffixes("memcached_operation_hit_ratio_percent", pmetric.MetricTypeGauge, "%"))
	require.Equal(t, "active_directory_ds_replication_object_rate_per_second", TrimPromSuffixes("active_directory_ds_replication_object_rate_per_second", pmetric.MetricTypeGauge, "{objects}/s"))
	require.Equal(t, "system_disk_operation_time_seconds", TrimPromSuffixes("system_disk_operation_time_seconds_total", pmetric.MetricTypeSum, "s"))
}

func TestNamespace(t *testing.T) {
	require.Equal(t, "space_test", BuildPromCompliantName(createGauge("test", ""), "space"))
	require.Equal(t, "space_test", BuildPromCompliantName(createGauge("#test", ""), "space"))
}

func TestCleanUpString(t *testing.T) {
	require.Equal(t, "", CleanUpString(""))
	require.Equal(t, "a_b", CleanUpString("a b"))
	require.Equal(t, "hello_world", CleanUpString("hello, world!"))
	require.Equal(t, "hello_you_2", CleanUpString("hello you 2"))
	require.Equal(t, "1000", CleanUpString("$1000"))
	require.Equal(t, "", CleanUpString("*+$^=)"))
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

func TestBuildPromCompliantName(t *testing.T) {
	require.Equal(t, "system_io_bytes_total", BuildPromCompliantName(createCounter("system.io", "By"), ""))
	require.Equal(t, "system_network_io_bytes_total", BuildPromCompliantName(createCounter("network.io", "By"), "system"))
	require.Equal(t, "_3_14_digits", BuildPromCompliantName(createGauge("3.14 digits", ""), ""))
	require.Equal(t, "envoy_rule_engine_zlib_buf_error", BuildPromCompliantName(createGauge("envoy__rule_engine_zlib_buf_error", ""), ""))
	require.Equal(t, "foo_bar", BuildPromCompliantName(createGauge(":foo::bar", ""), ""))
	require.Equal(t, "foo_bar_total", BuildPromCompliantName(createCounter(":foo::bar", ""), ""))
}
