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
- **tsdb/seriesmetadata/**: OTel resource/scope attribute and entity persistence
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
- **Series Metadata**: Optional Parquet-based storage for OTel resource/scope attributes

### OTel Native Metadata

Prometheus supports persisting OTel resource attributes, instrumentation scopes, and entities per time series. Enabled via `--enable-feature=native-metadata`.

- **Feature gate**: `--enable-feature=native-metadata` in `cmd/prometheus/main.go` sets `tsdb.Options.EnableNativeMetadata` and `web.EnableNativeMetadata`. When disabled (default), `DB.SeriesMetadata()` returns an empty reader and compaction skips metadata merge/write.
- **Package**: `tsdb/seriesmetadata/` — uses a **kind framework** with Go generics (`Versioned[V]`, `MemStore[V]`) for type-safe hot paths and a `KindDescriptor` interface for runtime dispatch at serialization boundaries. Core types are `MemSeriesMetadata` (wraps `map[KindID]any` of per-kind stores) and `parquetReader` (Parquet-backed). The `Reader` interface provides `Close()`, `IterKind()`, `KindLen()`, `LabelsForHash()`, `LookupResourceAttr()`, plus type-safe `VersionedResourceReader` and `VersionedScopeReader` methods.
- **labelsHash**: All metadata is keyed by `labels.StableHash(lset)` — a stable xxhash of the series' label set guaranteed not to change across builds or label storage variants. Always use `labels.StableHash` (not `Labels.Hash()` or `HashForLabels`) when computing keys for metadata stores, Parquet mapping rows, inverted indices, and API lookups. Using a different hash function would silently break cross-block merges and metadata-to-series joins.
- **BlockReader interface**: `SeriesMetadata()` was added to `BlockReader` (block.go), so all block-like types expose metadata. `RangeHead` and `OOOCompactionHead` both delegate to the underlying `Head`.
- **Write path**: `memSeries.meta` is `*metadata.Metadata` (simple pointer, non-versioned) set by `commitMetadata()`. OTel metadata is stored as `kindMeta []kindMetaEntry` on `memSeries` (each entry is `{kind KindID, data any}` holding a `*Versioned[V]`). The `kindMetaAccessor` interface (`GetKindMeta`/`SetKindMeta`) provides kind-generic access. `Head.SeriesMetadata()` collects metadata across all shards via `AllKinds()` iteration.
- **Block path**: `Block.SeriesMetadata()` lazily loads the Parquet file via `sync.Once` and returns a `blockSeriesMetadataReader` wrapper that tracks pending readers (same pattern as `blockIndexReader`, `blockTombstoneReader`, `blockChunkReader`). `Block.Size()` includes `numBytesSeriesMetadata`, which is populated after lazy load.
- **Merge path**: `DB.SeriesMetadata()` merges metadata across all blocks and the head via kind-aware iteration (`AllKinds()` + `IterKind()` + `SetVersioned()`), keyed by `labels.StableHash` (guarded by `EnableNativeMetadata`). After merge, `BuildResourceAttrIndex()` builds an inverted index for O(1) reverse lookup by resource attribute key:value pairs.
- **Compaction**: `LeveledCompactor.mergeAndWriteSeriesMetadata()` (compact.go, guarded by `enableNativeMetadata`) collects metadata from source blocks via kind-aware iteration, merges by labels hash per kind, opens the new block's index to build a `labelsHash → seriesRef` map, and writes via `WriteFileWithOptions()` with a `RefResolver`.
- **Web API**: `TSDBAdminStats` interface extended with `SeriesMetadata()`; `readyStorage` in main.go implements it. The `/api/v1/metadata` and `/api/v1/targets/metadata` endpoints always use scrape-cache regardless of native metadata setting.
- **Parquet schema**: Do not introduce new Parquet schema versions (e.g. v2) or schema migration logic. Modify the existing `metadataRow` struct in place. The single `metadataRow` struct covers all four namespace types; unused fields are zero/empty per namespace:
  - Common columns: `namespace` (discriminator), `series_ref` (mapping rows only), `mint`/`maxt` (optional time range), `content_hash` (content-addressed key)
  - Resource table columns: `identifying_attrs` (list), `descriptive_attrs` (list), `entities` (list of `EntityRow` with nested ID/description attr lists)
  - Scope table columns: `scope_name`, `scope_version_str`, `schema_url`, `scope_attrs` (list)
  - Mapping rows use only: `namespace`, `series_ref`, `content_hash`, `mint`, `maxt`
- **Kind framework**: Each metadata type (resource, scope) is a registered `KindDescriptor` with `KindOps[V]` for type-safe operations. `Versioned[V]` is the generic versioned container; `MemStore[V]` is the generic in-memory store. Type aliases (`VersionedResource = Versioned[*ResourceVersion]`, `MemResourceStore = MemStore[*ResourceVersion]`, etc.) provide backward compatibility. Adding a new kind: define version struct, implement `KindOps` + `KindDescriptor`, register in `init()` — framework layers (WAL dispatch, Parquet, head commit, compaction, DB merge) work automatically via the registry.
- **Resources**: `UpdateResource()` on `storage.ResourceUpdater` ingests identifying/descriptive attributes plus entities. Data is versioned over time per series (`Versioned[*ResourceVersion]` → `[]*ResourceVersion`); `AddOrExtend(ops, version)` creates a new version when attributes change or extends the time range when they match
- **Scopes**: `UpdateScope()` ingests OTel InstrumentationScope data (name, version, schema URL, attributes). Stored as `Versioned[*ScopeVersion]` → `[]*ScopeVersion` in `MemStore[*ScopeVersion]`
- **Entities**: `Entity` type in `tsdb/seriesmetadata/entity.go` with 7 predefined types: `resource`, `service`, `host`, `container`, `k8s.pod`, `k8s.node`, `process`. Each entity has typed ID (identifying) and Description (descriptive) attribute maps. Entities are embedded in `ResourceVersion`
- **Identifying Attributes**: `service.name`, `service.namespace`, `service.instance.id` used for resource identification
- **info() Function**: PromQL experimental function to enrich metrics with resource/scope attributes. Three modes controlled by `--query.info-resource-strategy`:
  - `target-info` (default): metric-join against `target_info` only (no native metadata needed)
  - `resource-attributes`: uses only stored native metadata via `ResourceQuerier`
  - `hybrid`: combines native metadata for `target_info` with metric-join for other info metrics; native metadata takes precedence on conflicts
  - Mode is selected per-call by `classifyInfoMode()` in `promql/info.go` based on `__name__` matchers
- **Label Name Translation**: `LabelNamerConfig` in `promql/engine.go` controls mapping OTel attribute names to Prometheus label names (UTF-8 handling, underscore sanitization). Used by `buildAttrNameMappings()` to create bidirectional name mappings
- **API Endpoints**:
  - `/api/v1/resources`: Forward lookup — given series (via `match[]`), return their resource attributes (supports `format=attributes` for autocomplete). Returns 400 when native metadata is disabled
  - `/api/v1/resources/series`: Reverse lookup — given resource attribute filters, find matching series with full version history. Filter: `resource.attr=key:value` (repeatable, AND logic). Supports `match[]` pre-filter, `start`/`end` time range, and `limit`. Returns 400 when no `resource.attr` filter provided or native metadata disabled. Uses an inverted index (`LookupResourceAttr`) for O(1) candidate lookup per filter, then intersects candidates and verifies with time range + attribute checks; falls back to full scan if index is not built. Architectural choice: reverse lookup only supports resource attribute filters — scope/entity filters are intentionally excluded for simplicity. Matched series include their scope versions in the response as supplementary data
- **OTLP Integration**: `CombinedAppender` in `storage/remote/otlptranslator/prometheusremotewrite/` handles OTLP ingestion
- **Observability**: Instrumentation metrics for monitoring the metadata pipeline:
  - `prometheus_tsdb_head_resource_updates_committed_total` — resource attribute updates committed
  - `prometheus_tsdb_head_scope_updates_committed_total` — scope updates committed
  - `prometheus_tsdb_storage_series_metadata_bytes` — bytes used by Parquet metadata files across all blocks
  - `prometheus_engine_info_function_calls_total{mode}` — info() calls by resolution mode (`native`, `metric-join`, `hybrid`)

Demo examples in `documentation/examples/`:
- `info-autocomplete-demo/`: Interactive demo for info() function autocomplete
- `otlp-resource-attributes/`: OTLP ingestion with resource attributes, including reverse lookup by metadata criteria

### Parquet Usage: parquet-common vs tsdb/seriesmetadata

These two systems both use `parquet-go` but solve different problems and should not be merged:

- **parquet-common** (`github.com/prometheus-community/parquet-common`): Replaces entire TSDB block format (labels + sample chunks) with columnar Parquet for cloud-scale analytical storage (Cortex/Thanos). Dynamic schema with one column per label name. Uses advanced Parquet features (projections, row group stats, bloom filters, page-level I/O).
- **tsdb/seriesmetadata**: Small sidecar Parquet file alongside standard TSDB blocks storing OTel resource/scope attributes. Static struct-based schema with nested lists. Loads entire file into memory for O(1) hash lookup. Typically kilobytes, not gigabytes.

**Why they can't converge**: Incompatible schemas (columnar per-label vs row-oriented with nested lists), incompatible scale assumptions (distributed cloud vs single-node local), and resource attributes are versioned (multiple values over time per series) which doesn't fit parquet-common's one-value-per-row label model. parquet-common exposes no reusable Parquet I/O primitives — its API is purpose-built for time series data.

**Techniques ported from parquet-common to seriesmetadata**:
- Explicit zstd compression (`zstd.SpeedBetterCompression`) instead of parquet-go defaults
- Row sorting before write (by namespace, series_ref, content_hash, MinTime) for better compression
- Footer key-value metadata for schema evolution and row counts
- Namespace-partitioned row groups: each namespace written as separate row group(s) via `WriteFileWithOptions`, enabling selective reads
- Optional bloom filters on `series_ref` and `content_hash` columns (`WriterOptions.EnableBloomFilters`). Write-only in this package; querying is done by the consumer (e.g., Mimir store-gateway)
- Configurable row group size limits (`WriterOptions.MaxRowsPerRowGroup`) for bounded memory on read
- `io.ReaderAt` read API (`ReadSeriesMetadataFromReaderAt`) decouples from `*os.File`, enabling `objstore.Bucket`-backed readers
- Namespace filtering on read (`WithNamespaceFilter`) skips non-matching row groups using Parquet column index min/max bounds

**Not ported** (inapplicable to this schema): Column projections (fixed ~15 column schema, not 500+ dynamic columns), two-file projections, page-level I/O (row groups are KB-to-MB, not GB), sharding.

**Normalized Parquet Storage**: The Parquet file uses content-addressed tables to eliminate cross-series duplication of resources and scopes. Four namespace types: `resource_table` (unique resource content keyed by xxhash `ContentHash`), `resource_mapping` (series `SeriesRef` → `ContentHash` + time range), `scope_table`, `scope_mapping`. Mapping rows store `SeriesRef` (block-level series reference) rather than `labelsHash`; the conversion happens at the Parquet boundary via resolver functions: `WriterOptions.RefResolver` converts `labelsHash → seriesRef` on write, `WithRefResolver` reader option converts `seriesRef → labelsHash` on read. When no resolver is provided (head/test writes), `labelsHash` is stored directly as `SeriesRef`. N series sharing the same OTel resource produce 1 table row + N mapping rows instead of N full copies. The in-memory model remains denormalized (`MemStore[V]` per kind) — normalization happens only during `WriteFile()` and is reversed during read. Parquet write/read dispatches per kind via `AllKinds()` + `KindDescriptor.BuildTableRow()`/`ParseTableRow()`. Content hashing uses `xxhash.Sum64` with sorted keys for determinism. Footer metadata tracks `resource_table_count`, `resource_mapping_count`, `scope_table_count`, `scope_mapping_count`.

**Distributed-Scale Considerations**: The namespace-partitioned row groups, bloom filters, `io.ReaderAt` API, and namespace filtering are designed for object-storage access patterns in clustered HA implementations (e.g., Grafana Mimir store-gateway). A Mimir integration would wrap `objstore.Bucket` as `io.ReaderAt`, use `WithNamespaceFilter` to read only the needed namespaces, and query bloom filters at the store-gateway layer to skip row groups before deserialization.

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
