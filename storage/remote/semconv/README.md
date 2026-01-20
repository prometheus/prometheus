<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_remote_read_handler_queries` | gauge | {query} | The number of in-flight remote read queries. |
| `prometheus_remote_storage_exemplars_in_total` | counter | {exemplar} | Exemplars in to remote storage, compare to determine dropped exemplars. |
| `prometheus_remote_storage_highest_timestamp_in_seconds` | gauge | s | Highest timestamp that has come into the remote storage via the Appender interface. |
| `prometheus_remote_storage_histograms_in_total` | counter | {histogram} | Histograms in to remote storage, compare to determine dropped histograms. |
| `prometheus_remote_storage_samples_in_total` | counter | {sample} | Samples in to remote storage, compare to determine dropped samples. |
| `prometheus_remote_storage_string_interner_zero_reference_releases_total` | counter | {release} | The number of times release has been called for strings that are not interned. |


## Metric Details


### `prometheus_remote_read_handler_queries`

The number of in-flight remote read queries.

- **Type:** gauge
- **Unit:** {query}
- **Stability:** development


### `prometheus_remote_storage_exemplars_in_total`

Exemplars in to remote storage, compare to determine dropped exemplars.

- **Type:** counter
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_remote_storage_highest_timestamp_in_seconds`

Highest timestamp that has come into the remote storage via the Appender interface.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_remote_storage_histograms_in_total`

Histograms in to remote storage, compare to determine dropped histograms.

- **Type:** counter
- **Unit:** {histogram}
- **Stability:** development


### `prometheus_remote_storage_samples_in_total`

Samples in to remote storage, compare to determine dropped samples.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development


### `prometheus_remote_storage_string_interner_zero_reference_releases_total`

The number of times release has been called for strings that are not interned.

- **Type:** counter
- **Unit:** {release}
- **Stability:** development
