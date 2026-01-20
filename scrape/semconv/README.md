<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_target_interval_length_histogram_seconds` | histogram | s | Actual intervals between scrapes as a histogram. |
| `prometheus_target_interval_length_seconds` | histogram | s | Actual intervals between scrapes. |
| `prometheus_target_metadata_cache_bytes` | gauge | By | The number of bytes that are currently used for storing metric metadata in the cache. |
| `prometheus_target_metadata_cache_entries` | gauge | {entry} | Total number of metric metadata entries in the cache. |
| `prometheus_target_reload_length_seconds` | histogram | s | Actual interval to reload the scrape pool with a given configuration. |
| `prometheus_target_scrape_duration_seconds` | histogram | s | Scrape request latency histogram. |
| `prometheus_target_scrape_pool_exceeded_label_limits_total` | counter | {occurrence} | Total number of times scrape pools hit the label limits. |
| `prometheus_target_scrape_pool_exceeded_target_limit_total` | counter | {occurrence} | Total number of times scrape pools hit the target limit. |
| `prometheus_target_scrape_pool_reloads_failed_total` | counter | {reload} | Total number of failed scrape pool reloads. |
| `prometheus_target_scrape_pool_reloads_total` | counter | {reload} | Total number of scrape pool reloads. |
| `prometheus_target_scrape_pool_symboltable_items` | gauge | {symbol} | Current number of symbols in the scrape pool symbol table. |
| `prometheus_target_scrape_pool_sync_total` | counter | {sync} | Total number of syncs that were executed on a scrape pool. |
| `prometheus_target_scrape_pool_target_limit` | gauge | {target} | Maximum number of targets allowed in this scrape pool. |
| `prometheus_target_scrape_pool_targets` | gauge | {target} | Current number of targets in this scrape pool. |
| `prometheus_target_scrape_pools_failed_total` | counter | {pool} | Total number of scrape pool creations that failed. |
| `prometheus_target_scrape_pools_total` | counter | {pool} | Total number of scrape pool creation attempts. |
| `prometheus_target_scrapes_cache_flush_forced_total` | counter | {scrape} | Total number of scrapes that forced a complete label cache flush. |
| `prometheus_target_scrapes_exceeded_body_size_limit_total` | counter | {scrape} | Total number of scrapes that hit the body size limit. |
| `prometheus_target_scrapes_exceeded_native_histogram_bucket_limit_total` | counter | {scrape} | Total number of scrapes that hit the native histogram bucket limit. |
| `prometheus_target_scrapes_exceeded_sample_limit_total` | counter | {scrape} | Total number of scrapes that hit the sample limit. |
| `prometheus_target_scrapes_exemplar_out_of_order_total` | counter | {exemplar} | Total number of exemplar rejected due to not being out of the expected order. |
| `prometheus_target_scrapes_sample_duplicate_timestamp_total` | counter | {sample} | Total number of samples rejected due to duplicate timestamps but different values. |
| `prometheus_target_scrapes_sample_out_of_bounds_total` | counter | {sample} | Total number of samples rejected due to timestamp falling outside of the time bounds. |
| `prometheus_target_scrapes_sample_out_of_order_total` | counter | {sample} | Total number of samples rejected due to not being out of the expected order. |
| `prometheus_target_sync_failed_total` | counter | {sync} | Total number of target sync failures. |
| `prometheus_target_sync_length_histogram_seconds` | histogram | s | Actual interval to sync the scrape pool as a histogram. |
| `prometheus_target_sync_length_seconds` | histogram | s | Actual interval to sync the scrape pool. |


## Metric Details


### `prometheus_target_interval_length_histogram_seconds`

Actual intervals between scrapes as a histogram.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `interval` | string | The configured scrape interval. | 15s, 30s |



### `prometheus_target_interval_length_seconds`

Actual intervals between scrapes.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `interval` | string | The configured scrape interval. | 15s, 30s |



### `prometheus_target_metadata_cache_bytes`

The number of bytes that are currently used for storing metric metadata in the cache.

- **Type:** gauge
- **Unit:** By
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_metadata_cache_entries`

Total number of metric metadata entries in the cache.

- **Type:** gauge
- **Unit:** {entry}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_reload_length_seconds`

Actual interval to reload the scrape pool with a given configuration.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `interval` | string | The configured scrape interval. | 15s, 30s |



### `prometheus_target_scrape_duration_seconds`

Scrape request latency histogram.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_target_scrape_pool_exceeded_label_limits_total`

Total number of times scrape pools hit the label limits.

- **Type:** counter
- **Unit:** {occurrence}
- **Stability:** development


### `prometheus_target_scrape_pool_exceeded_target_limit_total`

Total number of times scrape pools hit the target limit.

- **Type:** counter
- **Unit:** {occurrence}
- **Stability:** development


### `prometheus_target_scrape_pool_reloads_failed_total`

Total number of failed scrape pool reloads.

- **Type:** counter
- **Unit:** {reload}
- **Stability:** development


### `prometheus_target_scrape_pool_reloads_total`

Total number of scrape pool reloads.

- **Type:** counter
- **Unit:** {reload}
- **Stability:** development


### `prometheus_target_scrape_pool_symboltable_items`

Current number of symbols in the scrape pool symbol table.

- **Type:** gauge
- **Unit:** {symbol}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_scrape_pool_sync_total`

Total number of syncs that were executed on a scrape pool.

- **Type:** counter
- **Unit:** {sync}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_scrape_pool_target_limit`

Maximum number of targets allowed in this scrape pool.

- **Type:** gauge
- **Unit:** {target}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_scrape_pool_targets`

Current number of targets in this scrape pool.

- **Type:** gauge
- **Unit:** {target}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_scrape_pools_failed_total`

Total number of scrape pool creations that failed.

- **Type:** counter
- **Unit:** {pool}
- **Stability:** development


### `prometheus_target_scrape_pools_total`

Total number of scrape pool creation attempts.

- **Type:** counter
- **Unit:** {pool}
- **Stability:** development


### `prometheus_target_scrapes_cache_flush_forced_total`

Total number of scrapes that forced a complete label cache flush.

- **Type:** counter
- **Unit:** {scrape}
- **Stability:** development


### `prometheus_target_scrapes_exceeded_body_size_limit_total`

Total number of scrapes that hit the body size limit.

- **Type:** counter
- **Unit:** {scrape}
- **Stability:** development


### `prometheus_target_scrapes_exceeded_native_histogram_bucket_limit_total`

Total number of scrapes that hit the native histogram bucket limit.

- **Type:** counter
- **Unit:** {scrape}
- **Stability:** development


### `prometheus_target_scrapes_exceeded_sample_limit_total`

Total number of scrapes that hit the sample limit.

- **Type:** counter
- **Unit:** {scrape}
- **Stability:** development


### `prometheus_target_scrapes_exemplar_out_of_order_total`

Total number of exemplar rejected due to not being out of the expected order.

- **Type:** counter
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_target_scrapes_sample_duplicate_timestamp_total`

Total number of samples rejected due to duplicate timestamps but different values.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development


### `prometheus_target_scrapes_sample_out_of_bounds_total`

Total number of samples rejected due to timestamp falling outside of the time bounds.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development


### `prometheus_target_scrapes_sample_out_of_order_total`

Total number of samples rejected due to not being out of the expected order.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development


### `prometheus_target_sync_failed_total`

Total number of target sync failures.

- **Type:** counter
- **Unit:** {sync}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_sync_length_histogram_seconds`

Actual interval to sync the scrape pool as a histogram.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |



### `prometheus_target_sync_length_seconds`

Actual interval to sync the scrape pool.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `scrape_job` | string | The scrape job name. | prometheus, node_exporter |

