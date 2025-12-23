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
- **tsdb/seriesmetadata/**: Series metadata, OTel resource/scope attribute, and entity persistence
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

The storage layer is transitioning from `Appender` (V1) to `AppenderV2`. New code should use `AppenderV2` which combines sample, histogram, and exemplar appending into a single method. The `ResourceQuerier` interface (in `storage/interface.go`) provides `GetResourceAt()` and `IterUniqueAttributeNames()` for querying stored resource data. `ResourceUpdater` provides `UpdateResource()` for ingesting resource attributes with entities.

### TSDB Structure

- **Head Block**: In-memory storage for recent data with WAL
- **Persistent Blocks**: Immutable on-disk blocks
- **Compaction**: Merges blocks and applies retention
- **Chunks**: Gorilla-compressed time series data
- **Series Metadata**: Optional Parquet-based storage for metric metadata and OTel resource attributes

### OTel Native Metadata

Prometheus supports persisting OTel resource attributes, instrumentation scopes, and entities per time series. Enabled via `--enable-feature=native-metadata`.

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
- **Resources**: `UpdateResource()` on `storage.ResourceUpdater` ingests identifying/descriptive attributes plus entities. Data is versioned over time per series (`VersionedResource` → `[]ResourceVersion`); `AddOrExtend()` creates a new version when attributes change or extends the time range when they match
- **Scopes**: `UpdateScope()` ingests OTel InstrumentationScope data (name, version, schema URL, attributes). Stored as `VersionedScope` → `[]ScopeVersion` in `MemScopeStore`
- **Entities**: `Entity` type in `tsdb/seriesmetadata/entity.go` with 7 predefined types: `resource`, `service`, `host`, `container`, `k8s.pod`, `k8s.node`, `process`. Each entity has typed ID (identifying) and Description (descriptive) attribute maps. Entities are embedded in `ResourceVersion`
- **Identifying Attributes**: `service.name`, `service.namespace`, `service.instance.id` used for resource identification
- **info() Function**: PromQL experimental function to enrich metrics with resource/scope attributes. Three modes controlled by `--query.info-resource-strategy`:
  - `target-info` (default): metric-join against `target_info` only (no native metadata needed)
  - `resource-attributes`: uses only stored native metadata via `ResourceQuerier`
  - `hybrid`: combines native metadata for `target_info` with metric-join for other info metrics; native metadata takes precedence on conflicts
  - Mode is selected per-call by `classifyInfoMode()` in `promql/info.go` based on `__name__` matchers
- **Label Name Translation**: `LabelNamerConfig` in `promql/engine.go` controls mapping OTel attribute names to Prometheus label names (UTF-8 handling, underscore sanitization). Used by `buildAttrNameMappings()` to create bidirectional name mappings
- **API Endpoint**: `/api/v1/resources` for querying stored attributes (supports `format=attributes` for autocomplete). Returns 400 when native metadata is disabled
- **OTLP Integration**: `CombinedAppender` in `storage/remote/otlptranslator/prometheusremotewrite/` handles OTLP ingestion
- **Observability**: Instrumentation metrics for monitoring the metadata pipeline:
  - `prometheus_tsdb_head_resource_updates_committed_total` — resource attribute updates committed
  - `prometheus_tsdb_head_scope_updates_committed_total` — scope updates committed
  - `prometheus_tsdb_storage_series_metadata_bytes` — bytes used by Parquet metadata files across all blocks
  - `prometheus_engine_info_function_calls_total{mode}` — info() calls by resolution mode (`native`, `metric-join`, `hybrid`)

Demo examples in `documentation/examples/`:
- `info-autocomplete-demo/`: Interactive demo for info() function autocomplete
- `otlp-resource-attributes/`: OTLP ingestion with resource attributes
- `metadata-persistence/`: Basic metadata persistence demo

### Parquet Usage: parquet-common vs tsdb/seriesmetadata

These two systems both use `parquet-go` but solve different problems and should not be merged:

- **parquet-common** (`github.com/prometheus-community/parquet-common`): Replaces entire TSDB block format (labels + sample chunks) with columnar Parquet for cloud-scale analytical storage (Cortex/Thanos). Dynamic schema with one column per label name. Uses advanced Parquet features (projections, row group stats, bloom filters, page-level I/O).
- **tsdb/seriesmetadata**: Small sidecar Parquet file alongside standard TSDB blocks storing metric metadata and OTel resource attributes. Static struct-based schema with nested lists. Loads entire file into memory for O(1) hash lookup. Typically kilobytes, not gigabytes.

**Why they can't converge**: Incompatible schemas (columnar per-label vs row-oriented with nested lists), incompatible scale assumptions (distributed cloud vs single-node local), and resource attributes are versioned (multiple values over time per series) which doesn't fit parquet-common's one-value-per-row label model. parquet-common exposes no reusable Parquet I/O primitives — its API is purpose-built for time series data.

**Techniques ported from parquet-common to seriesmetadata**:
- Explicit zstd compression (`zstd.SpeedBetterCompression`) instead of parquet-go defaults
- Row sorting before write (by namespace, labels_hash, MinTime) for better compression
- Footer key-value metadata (`schema_version`, `metric_count`, `resource_count`, `scope_count`) for schema evolution

**Not worth porting**: Bloom filters, two-file projections, page-level I/O, row group tuning, sharding, `objstore.Bucket` integration — all designed for cloud-scale data that doesn't apply to a small local metadata file.

**seriesmetadata Storage Model**: Fully denormalized — one Parquet row per resource version per series. Each `metadataRow` embeds all attributes inline (`IdentifyingAttrs`, `DescriptiveAttrs`, `Entities` as nested lists); there is no resource ID table or foreign-key reference. N series sharing the same OTel resource produce N copies of all attributes in the file. The in-memory model mirrors this: `MemResourceStore` is `map[uint64]*versionedResourceEntry` keyed by series `labelsHash`, with independent attribute maps per series. Deduplication is temporal only — `AddOrExtend()` extends time ranges when attributes are unchanged; `MergeVersionedResources()` merges during compaction. No cross-series deduplication exists; Parquet columnar encoding + row sorting (by namespace, labels_hash, MinTime) + zstd compression absorb the redundancy. This works because files are typically KB-sized for single-node Prometheus.

**Distributed-Scale Considerations**: The denormalized model becomes costly at millions of series in clustered HA implementations (e.g., Grafana Mimir) — object storage costs and transfer overhead scale with series count times attribute count. A normalized design would use a content-addressed resource table plus a series-to-resource mapping table to eliminate cross-series duplication. Advanced Parquet techniques (column projections, bloom filters, page-level I/O, row group tuning) become essential for object-storage access patterns. Versioned resources add further complexity: time-range-aware joins, and the compactor must understand version merge semantics. These are forward-looking design notes, not planned work for single-node Prometheus.

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
