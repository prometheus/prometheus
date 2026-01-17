<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_tsdb_wal_reader_corruption_errors_total` | counter | {error} | Errors encountered when reading the WAL. |


## Metric Details


### `prometheus_tsdb_wal_reader_corruption_errors_total`

Errors encountered when reading the WAL.

- **Type:** counter
- **Unit:** {error}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `error` | string | The type of error. | invalid |

