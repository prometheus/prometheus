<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_sd_azure_cache_hit_total` | counter | {hit} | Number of cache hits during Azure SD. |
| `prometheus_sd_azure_failures_total` | counter | {failure} | Number of Azure SD failures. |
| `prometheus_sd_consul_rpc_duration_seconds` | histogram | s | The duration of a Consul RPC call. |
| `prometheus_sd_consul_rpc_failures_total` | counter | {failure} | Number of Consul RPC call failures. |
| `prometheus_sd_discovered_targets` | gauge | {target} | Current number of discovered targets. |
| `prometheus_sd_dns_lookup_failures_total` | counter | {failure} | Number of DNS SD lookup failures. |
| `prometheus_sd_dns_lookups_total` | counter | {lookup} | Number of DNS SD lookups. |
| `prometheus_sd_failed_configs` | gauge | {config} | Current number of service discovery configurations that failed to load. |
| `prometheus_sd_file_mtime_seconds` | gauge | s | The modification time of the SD file. |
| `prometheus_sd_file_read_errors_total` | counter | {error} | Number of file SD read errors. |
| `prometheus_sd_file_scan_duration_seconds` | histogram | s | The duration of the file SD scan. |
| `prometheus_sd_file_watcher_errors_total` | counter | {error} | Number of file SD watcher errors. |
| `prometheus_sd_http_failures_total` | counter | {failure} | Number of HTTP SD failures. |
| `prometheus_sd_kubernetes_events_total` | counter | {event} | Number of Kubernetes events processed. |
| `prometheus_sd_kubernetes_failures_total` | counter | {failure} | Number of Kubernetes SD failures. |
| `prometheus_sd_kuma_fetch_duration_seconds` | histogram | s | The duration of a Kuma MADS fetch call. |
| `prometheus_sd_kuma_fetch_failures_total` | counter | {failure} | Number of Kuma SD fetch failures. |
| `prometheus_sd_kuma_fetch_skipped_updates_total` | counter | {update} | Number of Kuma SD updates skipped due to no changes. |
| `prometheus_sd_linode_failures_total` | counter | {failure} | Number of Linode SD failures. |
| `prometheus_sd_nomad_failures_total` | counter | {failure} | Number of Nomad SD failures. |
| `prometheus_sd_received_updates_total` | counter | {update} | Total number of update events received from the SD providers. |
| `prometheus_sd_refresh_duration_histogram_seconds` | histogram | s | The duration of a SD refresh cycle as a histogram. |
| `prometheus_sd_refresh_duration_seconds` | histogram | s | The duration of a SD refresh cycle. |
| `prometheus_sd_refresh_failures_total` | counter | {failure} | Number of SD refresh failures. |
| `prometheus_sd_updates_delayed_total` | counter | {update} | Total number of update events that couldn't be sent immediately. |
| `prometheus_sd_updates_total` | counter | {update} | Total number of update events sent to the SD consumers. |
| `prometheus_treecache_watcher_goroutines` | gauge | {goroutine} | The current number of treecache watcher goroutines. |
| `prometheus_treecache_zookeeper_failures_total` | counter | {failure} | Total number of ZooKeeper failures. |


## Metric Details


### `prometheus_sd_azure_cache_hit_total`

Number of cache hits during Azure SD.

- **Type:** counter
- **Unit:** {hit}
- **Stability:** development


### `prometheus_sd_azure_failures_total`

Number of Azure SD failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_consul_rpc_duration_seconds`

The duration of a Consul RPC call.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_sd_consul_rpc_failures_total`

Number of Consul RPC call failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_discovered_targets`

Current number of discovered targets.

- **Type:** gauge
- **Unit:** {target}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `config` | string | The scrape config name. | prometheus, node_exporter |
| `name` | string | The discovery manager name. | scrape, notify |



### `prometheus_sd_dns_lookup_failures_total`

Number of DNS SD lookup failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_dns_lookups_total`

Number of DNS SD lookups.

- **Type:** counter
- **Unit:** {lookup}
- **Stability:** development


### `prometheus_sd_failed_configs`

Current number of service discovery configurations that failed to load.

- **Type:** gauge
- **Unit:** {config}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `name` | string | The discovery manager name. | scrape, notify |



### `prometheus_sd_file_mtime_seconds`

The modification time of the SD file.

- **Type:** gauge
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `filename` | string | The file path. | /etc/prometheus/file_sd/targets.json |



### `prometheus_sd_file_read_errors_total`

Number of file SD read errors.

- **Type:** counter
- **Unit:** {error}
- **Stability:** development


### `prometheus_sd_file_scan_duration_seconds`

The duration of the file SD scan.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_sd_file_watcher_errors_total`

Number of file SD watcher errors.

- **Type:** counter
- **Unit:** {error}
- **Stability:** development


### `prometheus_sd_http_failures_total`

Number of HTTP SD failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_kubernetes_events_total`

Number of Kubernetes events processed.

- **Type:** counter
- **Unit:** {event}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `event` | string | The event type. | add, update, delete |
| `role` | string | The Kubernetes role. | pod, node, service, endpoints |



### `prometheus_sd_kubernetes_failures_total`

Number of Kubernetes SD failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_kuma_fetch_duration_seconds`

The duration of a Kuma MADS fetch call.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_sd_kuma_fetch_failures_total`

Number of Kuma SD fetch failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_kuma_fetch_skipped_updates_total`

Number of Kuma SD updates skipped due to no changes.

- **Type:** counter
- **Unit:** {update}
- **Stability:** development


### `prometheus_sd_linode_failures_total`

Number of Linode SD failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_nomad_failures_total`

Number of Nomad SD failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_sd_received_updates_total`

Total number of update events received from the SD providers.

- **Type:** counter
- **Unit:** {update}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `name` | string | The discovery manager name. | scrape, notify |



### `prometheus_sd_refresh_duration_histogram_seconds`

The duration of a SD refresh cycle as a histogram.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `mechanism` | string | The service discovery mechanism. | dns, kubernetes, consul |



### `prometheus_sd_refresh_duration_seconds`

The duration of a SD refresh cycle.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_sd_refresh_failures_total`

Number of SD refresh failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `config` | string | The scrape config name. | prometheus |
| `mechanism` | string | The service discovery mechanism. | dns, kubernetes |



### `prometheus_sd_updates_delayed_total`

Total number of update events that couldn't be sent immediately.

- **Type:** counter
- **Unit:** {update}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `name` | string | The discovery manager name. | scrape, notify |



### `prometheus_sd_updates_total`

Total number of update events sent to the SD consumers.

- **Type:** counter
- **Unit:** {update}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `name` | string | The discovery manager name. | scrape, notify |



### `prometheus_treecache_watcher_goroutines`

The current number of treecache watcher goroutines.

- **Type:** gauge
- **Unit:** {goroutine}
- **Stability:** development


### `prometheus_treecache_zookeeper_failures_total`

Total number of ZooKeeper failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development
