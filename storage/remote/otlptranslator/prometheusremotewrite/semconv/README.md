<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_api_otlp_appended_samples_without_metadata_total` | counter | {sample} | The total number of samples ingested from OTLP without corresponding metadata. |
| `prometheus_api_otlp_out_of_order_exemplars_total` | counter | {exemplar} | The total number of received OTLP exemplars which were rejected because they were out of order. |


## Metric Details


### `prometheus_api_otlp_appended_samples_without_metadata_total`

The total number of samples ingested from OTLP without corresponding metadata.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development


### `prometheus_api_otlp_out_of_order_exemplars_total`

The total number of received OTLP exemplars which were rejected because they were out of order.

- **Type:** counter
- **Unit:** {exemplar}
- **Stability:** development
