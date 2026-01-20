<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_engine_queries` | gauge | {query} | The current number of queries being executed or waiting. |
| `prometheus_engine_queries_concurrent_max` | gauge | {query} | The max number of concurrent queries. |
| `prometheus_engine_query_duration_histogram_seconds` | histogram | s | Histogram of query timings. |
| `prometheus_engine_query_duration_seconds` | histogram | s | Query timings. |
| `prometheus_engine_query_log_enabled` | gauge | 1 | State of the query log. |
| `prometheus_engine_query_log_failures_total` | counter | {failure} | The number of query log failures. |
| `prometheus_engine_query_samples_total` | counter | {sample} | The total number of samples loaded by all queries. |


## Metric Details


### `prometheus_engine_queries`

The current number of queries being executed or waiting.

- **Type:** gauge
- **Unit:** {query}
- **Stability:** development


### `prometheus_engine_queries_concurrent_max`

The max number of concurrent queries.

- **Type:** gauge
- **Unit:** {query}
- **Stability:** development


### `prometheus_engine_query_duration_histogram_seconds`

Histogram of query timings.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `slice` | string | The query execution phase. | inner_eval, prepare_time, queue_time, result_sort |



### `prometheus_engine_query_duration_seconds`

Query timings.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `slice` | string | The query execution phase. | inner_eval, prepare_time, queue_time, result_sort |



### `prometheus_engine_query_log_enabled`

State of the query log.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_engine_query_log_failures_total`

The number of query log failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development


### `prometheus_engine_query_samples_total`

The total number of samples loaded by all queries.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development
