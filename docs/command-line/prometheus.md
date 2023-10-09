---
title: prometheus
---

# prometheus

The Prometheus monitoring server



## Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">-h</code>, <code class="text-nowrap">--help</code> | Show context-sensitive help (also try --help-long and --help-man). |  |
| <code class="text-nowrap">--version</code> | Show application version. |  |
| <code class="text-nowrap">--config.file</code> | Prometheus configuration file path. | `prometheus.yml` |
| <code class="text-nowrap">--web.listen-address</code> | Address to listen on for UI, API, and telemetry. | `0.0.0.0:9090` |
| <code class="text-nowrap">--web.config.file</code> | [EXPERIMENTAL] Path to configuration file that can enable TLS or authentication. |  |
| <code class="text-nowrap">--web.read-timeout</code> | Maximum duration before timing out read of the request, and closing idle connections. | `5m` |
| <code class="text-nowrap">--web.max-connections</code> | Maximum number of simultaneous connections. | `512` |
| <code class="text-nowrap">--web.external-url</code> | The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically. |  |
| <code class="text-nowrap">--web.route-prefix</code> | Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url. |  |
| <code class="text-nowrap">--web.user-assets</code> | Path to static asset directory, available at /user. |  |
| <code class="text-nowrap">--web.enable-lifecycle</code> | Enable shutdown and reload via HTTP request. | `false` |
| <code class="text-nowrap">--web.enable-admin-api</code> | Enable API endpoints for admin control actions. | `false` |
| <code class="text-nowrap">--web.enable-remote-write-receiver</code> | Enable API endpoint accepting remote write requests. | `false` |
| <code class="text-nowrap">--web.console.templates</code> | Path to the console template directory, available at /consoles. | `consoles` |
| <code class="text-nowrap">--web.console.libraries</code> | Path to the console library directory. | `console_libraries` |
| <code class="text-nowrap">--web.page-title</code> | Document title of Prometheus instance. | `Prometheus Time Series Collection and Processing Server` |
| <code class="text-nowrap">--web.cors.origin</code> | Regex for CORS origin. It is fully anchored. Example: 'https?://(domain1|domain2)\.com' | `.*` |
| <code class="text-nowrap">--storage.tsdb.path</code> | Base path for metrics storage. Use with server mode only. | `data/` |
| <code class="text-nowrap">--storage.tsdb.retention</code> | [DEPRECATED] How long to retain samples in storage. This flag has been deprecated, use "storage.tsdb.retention.time" instead. Use with server mode only. |  |
| <code class="text-nowrap">--storage.tsdb.retention.time</code> | How long to retain samples in storage. When this flag is set it overrides "storage.tsdb.retention". If neither this flag nor "storage.tsdb.retention" nor "storage.tsdb.retention.size" is set, the retention time defaults to 15d. Units Supported: y, w, d, h, m, s, ms. Use with server mode only. |  |
| <code class="text-nowrap">--storage.tsdb.retention.size</code> | Maximum number of bytes that can be stored for blocks. A unit is required, supported units: B, KB, MB, GB, TB, PB, EB. Ex: "512MB". Based on powers-of-2, so 1KB is 1024B. Use with server mode only. |  |
| <code class="text-nowrap">--storage.tsdb.no-lockfile</code> | Do not create lockfile in data directory. Use with server mode only. | `false` |
| <code class="text-nowrap">--storage.tsdb.head-chunks-write-queue-size</code> | Size of the queue through which head chunks are written to the disk to be m-mapped, 0 disables the queue completely. Experimental. Use with server mode only. | `0` |
| <code class="text-nowrap">--storage.agent.path</code> | Base path for metrics storage. Use with agent mode only. | `data-agent/` |
| <code class="text-nowrap">--storage.agent.wal-compression</code> | Compress the agent WAL. Use with agent mode only. | `true` |
| <code class="text-nowrap">--storage.agent.retention.min-time</code> | Minimum age samples may be before being considered for deletion when the WAL is truncated Use with agent mode only. |  |
| <code class="text-nowrap">--storage.agent.retention.max-time</code> | Maximum age samples may be before being forcibly deleted when the WAL is truncated Use with agent mode only. |  |
| <code class="text-nowrap">--storage.agent.no-lockfile</code> | Do not create lockfile in data directory. Use with agent mode only. | `false` |
| <code class="text-nowrap">--storage.remote.flush-deadline</code> | How long to wait flushing sample on shutdown or config reload. | `1m` |
| <code class="text-nowrap">--storage.remote.read-sample-limit</code> | Maximum overall number of samples to return via the remote read interface, in a single query. 0 means no limit. This limit is ignored for streamed response types. Use with server mode only. | `5e7` |
| <code class="text-nowrap">--storage.remote.read-concurrent-limit</code> | Maximum number of concurrent remote read calls. 0 means no limit. Use with server mode only. | `10` |
| <code class="text-nowrap">--storage.remote.read-max-bytes-in-frame</code> | Maximum number of bytes in a single frame for streaming remote read response types before marshalling. Note that client might have limit on frame size as well. 1MB as recommended by protobuf by default. Use with server mode only. | `1048576` |
| <code class="text-nowrap">--rules.alert.for-outage-tolerance</code> | Max time to tolerate prometheus outage for restoring "for" state of alert. Use with server mode only. | `1h` |
| <code class="text-nowrap">--rules.alert.for-grace-period</code> | Minimum duration between alert and restored "for" state. This is maintained only for alerts with configured "for" time greater than grace period. Use with server mode only. | `10m` |
| <code class="text-nowrap">--rules.alert.resend-delay</code> | Minimum amount of time to wait before resending an alert to Alertmanager. Use with server mode only. | `1m` |
| <code class="text-nowrap">--alertmanager.notification-queue-capacity</code> | The capacity of the queue for pending Alertmanager notifications. Use with server mode only. | `10000` |
| <code class="text-nowrap">--query.lookback-delta</code> | The maximum lookback duration for retrieving metrics during expression evaluations and federation. Use with server mode only. | `5m` |
| <code class="text-nowrap">--query.timeout</code> | Maximum time a query may take before being aborted. Use with server mode only. | `2m` |
| <code class="text-nowrap">--query.max-concurrency</code> | Maximum number of queries executed concurrently. Use with server mode only. | `20` |
| <code class="text-nowrap">--query.max-samples</code> | Maximum number of samples a single query can load into memory. Note that queries will fail if they try to load more samples than this into memory, so this also limits the number of samples a query can return. Use with server mode only. | `50000000` |
| <code class="text-nowrap">--enable-feature</code> | Comma separated feature names to enable. Valid options: agent, exemplar-storage, expand-external-labels, memory-snapshot-on-shutdown, promql-at-modifier, promql-negative-offset, promql-per-step-stats, remote-write-receiver (DEPRECATED), extra-scrape-metrics, new-service-discovery-manager, auto-gomaxprocs, no-default-scrape-port, native-histograms, otlp-write-receiver. See https://prometheus.io/docs/prometheus/latest/feature_flags/ for more details. |  |
| <code class="text-nowrap">--log.level</code> | Only log messages with the given severity or above. One of: [debug, info, warn, error] | `info` |
| <code class="text-nowrap">--log.format</code> | Output format of log messages. One of: [logfmt, json] | `logfmt` |


