This directory contains Protocol Buffer (protobuf) definitions for Prometheus'
remote read and write protocols. These definitions are used to serialize and
deserialize time series data, such as metrics, labels, samples, and queries,
for network communication to Prometheus.

The files here are synced to [buf.build](https://buf.build/prometheus/prometheus),
a public protobuf schema registry, from the `main` branch of the Prometheus
repository.

## What This Package/Directory Hosts

Protobuf messages and services for:
- Remote Write: Sending time series data to Prometheus (e.g., `WriteRequest`,
  `TimeSeries`).
- Remote Read: Querying data from Prometheus (e.g., `ReadRequest`, `Query`,
  `ChunkedReadResponse`).
- Core types: Shared definitions like `Label`, `MetricMetadata`, and exemplars.

Key files include:
- `remote.proto`: Defines the remote read/write services and messages.
- `types.proto`: Common types used across protocols.
- `io/prometheus/client/metrics.proto`: Client metrics definitions.
- `io/prometheus/write/v2/types.proto`: Remote Write v2 protocol types.

## Stability Guarantees

These protobuf definitions follow the stability policies of the Prometheus
project. Backward-compatible changes may occur in minor releases, but breaking
changes are reserved for major versions (e.g., Prometheus 3.0). Experimental
or unstable features are clearly marked in the documentation.

## Related Specifications

- Remote Write Spec v1:
  [https://prometheus.io/docs/specs/remote_write_spec/](https://prometheus.io/docs/specs/remote_write_spec/).
- Remote Write Spec v2:
  [https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/).
- Experimental Prometheus client packages:
  [https://github.com/prometheus/client_golang/tree/main/exp](https://github.com/prometheus/client_golang/tree/main/exp).

## How to Change or Contribute

To modify these definitions, view and edit the source in the Prometheus GitHub
repository: [https://github.com/prometheus/prometheus/tree/main/prompb](https://github.com/prometheus/prometheus/tree/main/prompb).

## How to Use

### Steps

- Run `make proto` in the root directory to regenerate the compiled protobuf
  code.
- The compiled Go code is version-controlled in the repository, so you
  typically don't need to re-generate unless making changes.
