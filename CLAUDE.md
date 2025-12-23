# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
make build                    # Full build with embedded assets
go build ./cmd/prometheus/    # Quick build without assets (for development)
go build ./cmd/promtool/      # Build utility tool
```

## Testing

```bash
make test                     # Full test suite with race detector and UI tests
make test-short               # Short tests only
go test ./tsdb/...            # Test specific package
go test -run TestName ./pkg/  # Run single test
go test -race ./...           # With race detector
go test -v ./pkg/             # Verbose (use -count=1 to disable caching if needed)
go test --tags=dedupelabels ./...  # With build tags
```

## Linting and Formatting

```bash
make lint                     # Run golangci-lint
make format                   # Run gofmt and lint fixes
make lint-fix                 # Auto-fix linting issues
```

## Parser Generation

After modifying `promql/parser/generated_parser.y`:
```bash
make parser                   # Regenerate parser via goyacc
make check-generated-parser   # Verify parser is up-to-date
```

## Architecture Overview

Prometheus is a monitoring system with these core components:

### Data Flow
```
Service Discovery → Scrape → Storage (TSDB) → PromQL Engine → Web API/UI
                                    ↓
                              Rules Engine → Notifier → Alertmanager
```

### Key Packages

- **cmd/prometheus/**: Main server entry point
- **cmd/promtool/**: CLI utility for validation, TSDB operations
- **storage/**: Storage abstraction layer with `Appender`, `Querier` interfaces
- **tsdb/**: Time series database (head block, WAL, compaction, chunks)
- **tsdb/seriesmetadata/**: Series metadata and OTel resource attribute persistence
- **promql/**: Query engine and PromQL parser
- **scrape/**: Metric collection from targets
- **discovery/**: 30+ service discovery implementations
- **rules/**: Alerting and recording rule evaluation
- **notifier/**: Alert routing to Alertmanager
- **config/**: YAML configuration parsing
- **model/labels/**: Label storage and manipulation
- **model/histogram/**: Native histogram types
- **web/**: HTTP API and React/Mantine UI
- **storage/remote/otlptranslator/**: OTLP to Prometheus translation including CombinedAppender

### Storage Interface Evolution

The storage layer is transitioning from `Appender` (V1) to `AppenderV2`. New code should use `AppenderV2` which combines sample, histogram, and exemplar appending into a single method.

### TSDB Structure

- **Head Block**: In-memory storage for recent data with WAL
- **Persistent Blocks**: Immutable on-disk blocks
- **Compaction**: Merges blocks and applies retention
- **Chunks**: Gorilla-compressed time series data
- **Series Metadata**: Optional Parquet-based storage for metric metadata and OTel resource attributes

### OTel Resource Attributes

Prometheus supports persisting OTel resource attributes per time series:

- **Storage**: Resource attributes stored in `tsdb/seriesmetadata/` using Parquet format
- **Appender Interface**: `UpdateResource()` method on `storage.Appender` for ingesting attributes
- **Identifying Attributes**: `service.name`, `service.namespace`, `service.instance.id` used for resource identification
- **info() Function**: PromQL experimental function to enrich metrics with resource attributes
- **API Endpoint**: `/api/v1/resources` for querying stored attributes (supports `format=attributes` for autocomplete)
- **OTLP Integration**: `CombinedAppender` in `storage/remote/otlptranslator/prometheusremotewrite/` handles OTLP ingestion

Demo examples in `documentation/examples/`:
- `info-autocomplete-demo/`: Interactive demo for info() function autocomplete
- `otlp-resource-attributes/`: OTLP ingestion with resource attributes
- `metadata-persistence/`: Basic metadata persistence demo

### Build Tags

- `dedupelabels`, `slicelabels`: Label storage variants
- `forcedirectio`: Direct I/O for TSDB
- `builtinassets`: Embed web assets in binary

## Test Patterns

- Tests use `testify/require` for assertions
- Test helpers in `util/testutil/` and `util/teststorage/`
- Head tests use `newTestHead()` which auto-cleans up via `t.Cleanup`
- Table-driven tests are common throughout

## Pull Requests and Issues

- Use the PR template in `.github/PULL_REQUEST_TEMPLATE.md`
- Use the issue templates in `.github/ISSUE_TEMPLATE/` (bug reports, feature requests)
- PR titles should follow the format `area: short description` (e.g., `tsdb: reduce disk usage`)
- Sign commits with `-s` / `--signoff` for DCO compliance
- Do not mention Claude or AI assistance in commits, issues, or PRs
