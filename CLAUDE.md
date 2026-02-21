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
go test -v ./pkg/             # Verbose output
go test --tags=dedupelabels ./...  # With build tags
```

- Do NOT use `-count=1` unless there is a specific reason to bypass the test cache (e.g., debugging flaky tests). Go's test caching is beneficial and should be left enabled by default.

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
- **tsdb/seriesmetadata/**: Parquet-based series metadata persistence (type, unit, help)
- **promql/**: Query engine and PromQL parser
- **scrape/**: Metric collection from targets
- **discovery/**: 30+ service discovery implementations
- **rules/**: Alerting and recording rule evaluation
- **notifier/**: Alert routing to Alertmanager
- **config/**: YAML configuration parsing
- **model/labels/**: Label storage and manipulation
- **model/histogram/**: Native histogram types
- **web/**: HTTP API and React/Mantine UI

### Storage Interface Evolution

The storage layer is transitioning from `Appender` (V1) to `AppenderV2`. New code should use `AppenderV2` which combines sample, histogram, and exemplar appending into a single method.

### TSDB Structure

- **Head Block**: In-memory storage for recent data with WAL
- **Persistent Blocks**: Immutable on-disk blocks
- **Compaction**: Merges blocks and applies retention
- **Chunks**: Gorilla-compressed time series data
- **Series Metadata**: Parquet-based storage for metric metadata (type, unit, help), enabled via `--enable-feature=native-metadata`

### Series Metadata Persistence

This branch adds Parquet-based series metadata persistence to TSDB, gated behind `--enable-feature=native-metadata` (`Options.EnableNativeMetadata`). Metric metadata (type, unit, help) is stored in `series_metadata.parquet` sidecar files alongside standard TSDB blocks.

- **Feature gate**: `--enable-feature=native-metadata` in `cmd/prometheus/main.go` sets both `tsdb.Options.EnableNativeMetadata` and `web.EnableNativeMetadata` (the latter switches API endpoints to TSDB-backed metadata). When disabled (default), `DB.SeriesMetadata()` returns an empty reader and compaction skips metadata merge/write.
- **Package**: `tsdb/seriesmetadata/` — core types are `VersionedMetadata` (tracks metadata changes over time for a series) and `MetadataVersion` (single version with MinTime/MaxTime and metadata content). Storage backends: `MemSeriesMetadata` (in-memory) and `parquetReader` (Parquet-backed).
- **Reader interface**: `Get(ref)`, `GetByMetricName(name)`, `Iter()`, `IterByMetricName()`, `Total()`, `Close()`, plus `VersionedMetadataReader` methods (`GetVersionedMetadata(ref)`, `IterVersionedMetadata()`, `TotalVersionedMetadata()`).
- **BlockReader interface**: `SeriesMetadata()` was added to `BlockReader` (block.go), so all block-like types expose metadata. `RangeHead` and `OOOCompactionHead` both delegate to the underlying `Head`.
- **Write path**: `memSeries.meta` is `*seriesmetadata.VersionedMetadata`. `commitMetadata()` (head_append.go) calls `series.meta.AddOrExtend(&MetadataVersion{...})` with MinTime/MaxTime from the WAL record. WAL replay (head_wal.go) uses the same `AddOrExtend` pattern. `Head.SeriesMetadata()` reads from `memSeries.meta` across all shards using a two-phase locking pattern (shard RLock to collect refs, then per-series lock to read metadata).
- **Block path**: `Block.SeriesMetadata()` lazily loads the Parquet file via `sync.Once` and returns a `blockSeriesMetadataReader` wrapper that tracks pending readers (same pattern as `blockIndexReader`, `blockTombstoneReader`, `blockChunkReader`). `Block.Size()` includes `numBytesSeriesMetadata`, which is populated after lazy load.
- **Merge path**: `DB.SeriesMetadata()` merges versioned metadata across all blocks and the head, keyed by `labels.StableHash` (guarded by `EnableNativeMetadata`). `DB.SeriesMetadataForMatchers()` delegates to the head for per-target metadata queries.
- **Compaction**: `LeveledCompactor.mergeAndWriteSeriesMetadata()` (compact.go, guarded by `enableNativeMetadata`) runs three phases: (1) collect metadata from source blocks, resolving each block's refs to labels via its index and merging by `labels.StableHash`; (2) open the new block's index and map labels hashes to new refs; (3) build `MemSeriesMetadata` with new refs and write via `WriteFile()`.
- **Web API**: `TSDBAdminStats` interface extended with `SeriesMetadata()` and `SeriesMetadataForMatchers()`; `readyStorage` in main.go implements both. When native metadata is enabled: `/api/v1/metadata` uses TSDB as sole metadata source; `/api/v1/targets/metadata` queries TSDB head per-target via `SeriesMetadataForMatchers()`; `/api/v1/metadata/versions` returns time-versioned metadata history with optional `metric`, `limit`, `start`, `end` parameters; `/api/v1/metadata/series` performs inverse metadata lookup — find metrics by `type` (exact), `unit` (exact), or `help` (RE2 regex), with optional `limit`, `start`, `end` filtering. When disabled, these endpoints fall back to scrape-cache behavior or return empty results.
- **Parquet schema**: Do not introduce new Parquet schema versions (e.g. v2) or schema migration logic. Modify the existing `metadataRow` struct in place.

Demo example in `documentation/examples/metadata-persistence/`.

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
