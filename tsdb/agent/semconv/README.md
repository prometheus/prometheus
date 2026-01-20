<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_agent_active_series` | gauge | {series} | Number of active series being tracked by the WAL storage. |
| `prometheus_agent_checkpoint_creations_failed_total` | counter | {creation} | Total number of checkpoint creations that failed. |
| `prometheus_agent_checkpoint_creations_total` | counter | {creation} | Total number of checkpoint creations attempted. |
| `prometheus_agent_checkpoint_deletions_failed_total` | counter | {deletion} | Total number of checkpoint deletions that failed. |
| `prometheus_agent_checkpoint_deletions_total` | counter | {deletion} | Total number of checkpoint deletions attempted. |
| `prometheus_agent_corruptions_total` | counter | {corruption} | Total number of WAL corruptions. |
| `prometheus_agent_data_replay_duration_seconds` | gauge | s | Time taken to replay the data on disk. |
| `prometheus_agent_deleted_series` | gauge | {series} | Number of series pending deletion from the WAL. |
| `prometheus_agent_exemplars_appended_total` | counter | {exemplar} | Total number of exemplars appended to the storage. |
| `prometheus_agent_out_of_order_samples_total` | counter | {sample} | Total number of out of order samples ingestion failed attempts. |
| `prometheus_agent_samples_appended_total` | counter | {sample} | Total number of samples appended to the storage. |
| `prometheus_agent_truncate_duration_seconds` | histogram | s | Duration of WAL truncation. |


## Metric Details


### `prometheus_agent_active_series`

Number of active series being tracked by the WAL storage.

- **Type:** gauge
- **Unit:** {series}
- **Stability:** development


### `prometheus_agent_checkpoint_creations_failed_total`

Total number of checkpoint creations that failed.

- **Type:** counter
- **Unit:** {creation}
- **Stability:** development


### `prometheus_agent_checkpoint_creations_total`

Total number of checkpoint creations attempted.

- **Type:** counter
- **Unit:** {creation}
- **Stability:** development


### `prometheus_agent_checkpoint_deletions_failed_total`

Total number of checkpoint deletions that failed.

- **Type:** counter
- **Unit:** {deletion}
- **Stability:** development


### `prometheus_agent_checkpoint_deletions_total`

Total number of checkpoint deletions attempted.

- **Type:** counter
- **Unit:** {deletion}
- **Stability:** development


### `prometheus_agent_corruptions_total`

Total number of WAL corruptions.

- **Type:** counter
- **Unit:** {corruption}
- **Stability:** development


### `prometheus_agent_data_replay_duration_seconds`

Time taken to replay the data on disk.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_agent_deleted_series`

Number of series pending deletion from the WAL.

- **Type:** gauge
- **Unit:** {series}
- **Stability:** development


### `prometheus_agent_exemplars_appended_total`

Total number of exemplars appended to the storage.

- **Type:** counter
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_agent_out_of_order_samples_total`

Total number of out of order samples ingestion failed attempts.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development


### `prometheus_agent_samples_appended_total`

Total number of samples appended to the storage.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `type` | string | The type of sample. | float, histogram |



### `prometheus_agent_truncate_duration_seconds`

Duration of WAL truncation.

- **Type:** histogram
- **Unit:** s
- **Stability:** development
