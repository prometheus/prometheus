---
title: Internal metrics
---

Prometheus exposes its own operational metrics at `/metrics`.

## Deprecated summary metrics

The following summaries are deprecated in favor of native histogram counterparts.
Both metrics continue to be exposed and receive the same observations. The
existing summaries, including their count, sum, labels, and configured quantiles,
remain available for compatibility. Their removal is deferred to a future release;
this change does not remove them.

| Deprecated summary | Native histogram replacement |
| --- | --- |
| `prometheus_sd_consul_rpc_duration_seconds` | `prometheus_sd_consul_rpc_duration_histogram_seconds` |
| `prometheus_sd_file_scan_duration_seconds` | `prometheus_sd_file_scan_duration_histogram_seconds` |
| `prometheus_sd_kuma_fetch_duration_seconds` | `prometheus_sd_kuma_fetch_duration_histogram_seconds` |
| `prometheus_sd_kubernetes_http_request_duration_seconds` | `prometheus_sd_kubernetes_http_request_duration_histogram_seconds` |
| `prometheus_sd_kubernetes_workqueue_latency_seconds` | `prometheus_sd_kubernetes_workqueue_latency_histogram_seconds` |
| `prometheus_sd_kubernetes_workqueue_work_duration_seconds` | `prometheus_sd_kubernetes_workqueue_work_duration_histogram_seconds` |
| `prometheus_target_reload_length_seconds` | `prometheus_target_reload_length_histogram_seconds` |
| `prometheus_agent_truncate_duration_seconds` | `prometheus_agent_truncate_duration_histogram_seconds` |
| `prometheus_tsdb_head_gc_duration_seconds` | `prometheus_tsdb_head_gc_duration_histogram_seconds` |
| `prometheus_tsdb_wal_truncate_duration_seconds` | `prometheus_tsdb_wal_truncate_duration_histogram_seconds` |
| `prometheus_tsdb_wal_fsync_duration_seconds` | `prometheus_tsdb_wal_fsync_duration_histogram_seconds` |
| `prometheus_tsdb_out_of_order_wbl_fsync_duration_seconds` | `prometheus_tsdb_out_of_order_wbl_fsync_duration_histogram_seconds` |

The histogram counterparts have distinct names so they can coexist with the
summaries in the registry and in Prometheus. Enable `scrape_native_histograms`
in the [scrape configuration](configuration/configuration.md#scrape_config) used
to collect Prometheus's own metrics to ingest the native histograms.

Update custom dashboards, recording rules, and alerts to use the replacements.
For example, replace a summary's sum/count expression:

```promql
rate(prometheus_tsdb_head_gc_duration_seconds_sum[5m])
/
rate(prometheus_tsdb_head_gc_duration_seconds_count[5m])
```

with the native histogram mean:

```promql
histogram_avg(rate(prometheus_tsdb_head_gc_duration_histogram_seconds[5m]))
```

To compute a quantile from the native histogram, use:

```promql
histogram_quantile(0.99, rate(prometheus_tsdb_head_gc_duration_histogram_seconds[5m]))
```

Native histograms allow quantiles to be calculated across instances by aggregating
the histogram samples before applying `histogram_quantile`. Existing summary
quantiles are calculated by each instance and cannot be aggregated this way.
