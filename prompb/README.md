This directory contains Protocol Buffer (protobuf) definitions for Prometheus' remote read and write protocols. These definitions are used to serialize and deserialize time series data, such as metrics, labels, samples, and queries, for network communication in Prometheus.

The files here are synced to [buf.build](https://buf.build/prometheus/prometheus), a public protobuf schema registry, from the `main` branch of the Prometheus repository.

## What This Package/Directory Hosts

This module hosts protobuf messages and services for:
- Remote Write: Sending time series data to Prometheus (e.g., `WriteRequest`, `TimeSeries`).
- Remote Read: Querying data from Prometheus (e.g., `ReadRequest`, `Query`, `ChunkedReadResponse`).
- Core types: Shared definitions like `Label`, `MetricMetadata`, and exemplars.

Key files include:
- `remote.proto`: Defines the remote read/write services and messages.
- `types.proto`: Common types used across protocols.

## Stability Guarantees

These protobuf definitions follow the stability policies of the Prometheus project. Backward-compatible changes may occur in minor releases, but breaking changes are reserved for major versions (e.g., Prometheus 3.0). Experimental or unstable features are clearly marked in the documentation. Always check the Prometheus release notes for details on protocol changes.

## Related Specifications

- Remote Write Spec v2: [https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/).

## How to Change or Contribute

To modify these definitions or fix issues:
- View and edit the source in the Prometheus GitHub repository: [https://github.com/prometheus/prometheus/tree/main/prompb](https://github.com/prometheus/prometheus/tree/main/prompb).
- Follow the Prometheus contribution guidelines: [guides](../CONTRIBUTING.md).
- Open a pull request with your changes. Changes here may affect compatibility, so include tests and update related specs if needed.
- Once merged to `main`, updates will sync to Buf.build automatically.

## How to Use

### Steps

- Run `make proto` in the root directory to regenerate the compiled protobuf code (uses Gogo protobuf extensions).
- The compiled Go code is version-controlled in the repository, so you typically don't need to re-generate unless making changes.
- In order for the [script](../scripts/genproto.sh) to run, you'll need `protoc` (version 3.15.8) in your PATH.

### Note 
Prometheus uses [Gogo Protobuf](https://github.com/gogo/protobuf) for code generation, which includes custom extensions. If generating code for non-Gogo environments (e.g., standard `protoc`), you may need to remove leftover Gogo imports manually to avoid compilation errors (related [issue](https://github.com/bufbuild/buf/issues/3275)).

```bash
	@find io/prometheus/write/ -type f -exec sed -i '' 's/_ "github.com\/gogo\/protobuf\/gogoproto"//g' {} \;
```