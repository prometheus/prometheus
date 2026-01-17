<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_tsdb_chunk_write_queue_operations_total` | counter | {operation} | Number of operations on the chunk_write_queue. |


## Metric Details


### `prometheus_tsdb_chunk_write_queue_operations_total`

Number of operations on the chunk_write_queue.

- **Type:** counter
- **Unit:** {operation}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `operation` | string | The type of operation. | add, get, complete, shrink |

