# tsdb/seriesmetadata

This package persists OTel (OpenTelemetry) resource attributes, instrumentation scopes, and entities alongside Prometheus TSDB blocks. Data is stored in a Parquet sidecar file (`series_metadata.parquet`) within each block directory.

Enabled via `--enable-feature=native-metadata`.

## Overview

Each time series in Prometheus can have associated OTel metadata:

- **Resource attributes**: identifying (e.g. `service.name`) and descriptive (e.g. `host.name`) attributes from the OTel Resource
- **Entities**: typed OTel entities (service, host, container, etc.) with their own ID and description attributes, embedded within resource versions
- **Scopes**: OTel InstrumentationScope data (library name, version, schema URL, custom attributes)

All metadata is **versioned over time** per series. When a descriptive attribute changes (e.g. a service migrates to a new host), a new version is created with its own time range. Identifying attributes remain constant across versions.

## Architecture

```
                    ┌─────────────────────┐
                    │   OTLP Ingestion    │
                    │  (CombinedAppender) │
                    └─────────┬───────────┘
                              │ UpdateResource() / UpdateScope()
                              ▼
                    ┌─────────────────────┐
                    │   TSDB Head Block   │
                    │  (memSeries)        │
                    │  kindMeta []entry   │
                    └─────────┬───────────┘
                              │ Compaction
                              ▼
┌──────────────────────────────────────────────────────────┐
│                  Parquet Sidecar File                     │
│              series_metadata.parquet                      │
│                                                          │
│  ┌──────────────┐  ┌──────────────────┐                  │
│  │resource_table│  │resource_mapping  │                  │
│  │ (deduplicated│  │ (series_ref →    │                  │
│  │  content)    │  │  content_hash +  │                  │
│  │              │  │  time range)     │                  │
│  └──────────────┘  └──────────────────┘                  │
│  ┌──────────────┐  ┌──────────────────┐                  │
│  │ scope_table  │  │ scope_mapping    │                  │
│  └──────────────┘  └──────────────────┘                  │
└──────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────────────────┐
                    │        Query / API           │
                    │  info(), /resources,         │
                    │  /resources/series           │
                    └──────────────────────────────┘
```

The `/resources/series` reverse-lookup endpoint only supports filtering by resource attributes (`resource.attr=key:value`). Scope and entity filters are intentionally excluded as an architectural choice for simplicity — matched series include their scope versions in the response as supplementary data but scopes are not filterable.

## Kind Framework

The metadata subsystem uses a **kind framework** to handle different metadata types (resources, scopes) generically. This avoids duplicating nearly identical code at every layer (WAL, Parquet, head commit/replay, compaction, DB merge).

### Architecture

- **Go generics** for type-safe hot paths: `Versioned[V]` (versioned container), `MemStore[V]` (in-memory store), `KindOps[V]` (equality/copy operations)
- **`KindDescriptor` interface** for runtime dispatch at serialization boundaries: WAL encode/decode, Parquet conversion, head commit, store operations. Methods use `any` for type erasure since the registry is not generic.
- **Kind registry** with lookup by `KindID`, WAL record type, table namespace, and mapping namespace. Each kind registers itself in an `init()` function.

Adding a new metadata kind requires: (1) define the version struct, (2) implement `KindOps` and `KindDescriptor`, (3) register in `init()`. The framework layers (stores, Parquet normalization, WAL dispatch, head commit/replay, compaction merge, DB merge) all work automatically via the registry.

### Import Cycle Avoidance

`seriesmetadata` cannot import `tsdb/record` (which would create a cycle). The registry defines `WALRecordType = uint8` and each kind descriptor uses pluggable function variables (`ResourceDecodeWAL`, `ScopeDecodeWAL`, etc.) that are set by the `tsdb` package during init.

## In-Memory Model

Data is stored per-series, keyed by `labels.StableHash` (a 64-bit hash of the series' label set).

### Generic Containers

`Versioned[V]` is a generic container holding `[]V` ordered by MinTime ascending. Type aliases provide backward compatibility:

- `VersionedResource = Versioned[*ResourceVersion]` → `[]*ResourceVersion`
  - Each `ResourceVersion` has: `Identifying` attrs, `Descriptive` attrs, `[]*Entity`, `MinTime`, `MaxTime`
- `VersionedScope = Versioned[*ScopeVersion]` → `[]*ScopeVersion`
  - Each `ScopeVersion` has: `Name`, `Version`, `SchemaURL`, `Attrs`, `MinTime`, `MaxTime`

### Generic Stores

`MemStore[V]` is a generic in-memory store (`map[uint64]*versionedEntry[V]`). Type aliases provide backward compatibility:

| Alias | Underlying Type | Key | Value |
|-------|-----------------|-----|-------|
| `MemResourceStore` | `MemStore[*ResourceVersion]` | labelsHash | `*Versioned[*ResourceVersion]` |
| `MemScopeStore` | `MemStore[*ScopeVersion]` | labelsHash | `*Versioned[*ScopeVersion]` |

`MemSeriesMetadata` wraps a `map[KindID]any` (each value is a `*MemStore[V]` for the appropriate V) and implements the `Reader` interface. It provides `StoreForKind(id)` for generic access and type-safe accessors `ResourceStore()` / `ScopeStore()`.

### Resource Attribute Inverted Index

`MemSeriesMetadata` supports an optional inverted index for O(1) reverse lookup by resource attribute key:value pairs. The index maps `"key\x00value"` → set of labelsHashes.

**Selective indexing**: The index uses selective attribute indexing to control its size:
- **Identifying attributes** (from `ResourceVersion.Identifying`, e.g. `service.name`, `service.namespace`, `service.instance.id`) are **always** indexed
- **Descriptive attributes** (from `ResourceVersion.Descriptive`) are only indexed if their key is in `indexedResourceAttrs` — a configurable set passed via `SetIndexedResourceAttrs()`, sourced from `tsdb.Options.IndexedResourceAttrs`
- Default (nil `indexedResourceAttrs`) means only identifying attributes are indexed, reducing index size by ~10x at scale

This filtering applies consistently across all index operations: `BuildResourceAttrIndex()`, `UpdateResourceAttrIndex()`, `RemoveFromResourceAttrIndex()`, and Parquet `buildResourceAttrIndexRows()`.

- **`BuildResourceAttrIndex()`**: Iterates all resource versions once and populates the index. Called by `DB.buildSeriesMetadata()` after merging blocks + head. Since the merged reader is cached for 30 seconds, the index build cost is amortized across many API requests.
- **`LookupResourceAttr(key, value)`**: Returns the set of labelsHashes matching the given attribute, or nil if the index has not been built. The `/resources/series` handler intersects candidate sets from multiple filters, then verifies each candidate with `GetVersionedResource` + time range + attribute checks. Falls back to a full `IterVersionedResources` scan if the index is nil.

The index is **time-unaware** — it includes all labelsHashes that have *any* version with the attribute. Time-range filtering happens during the verification step after index lookup. This is a deliberate trade-off: the index stays simple and the handler already performs per-version time filtering.

### Head Storage

On `memSeries`, metadata is stored as `kindMeta []kindMetaEntry` where each entry is `{kind KindID, data any}`. Linear scan of 0-2 entries is faster than map lookup. The `kindMetaAccessor` interface (`GetKindMeta`/`SetKindMeta`) provides kind-generic access.

The `Head` also maintains a shared `*MemSeriesMetadata` (`seriesMeta`) that is incrementally updated during `commitResources()`/`commitScopes()` and WAL replay via `updateSharedMetadata()`. This avoids an O(ALL_SERIES) scan that would otherwise be required to collect metadata across all shards. `Head.SeriesMetadata()` returns an O(1) `headMetadataReader` wrapper around this live store instead of scanning.

The head does **not** populate `seriesMeta.labelsMap` — labels are resolved on-demand via `stripeSeries.getByID` in `headMetadataReader.LabelsForHash`. This saves ~3GB at 10M series by avoiding label set duplication.

Two sharded stripe arrays (`metaRefStripes` and `metaHashStripes`, 256-way) track `HeadSeriesRef ↔ labelsHash` mappings for GC cleanup and label resolution. Lock ordering: series lock → stripe lock. `MemStore[V]` operations are internally concurrent-safe via their own mutexes.

### Versioning

`AddOrExtend(ops, version)` on `Versioned[V]` handles ingestion:
- If the latest version's content matches (via `KindOps.Equal`), extends `MaxTime` (same content, later timestamp)
- If content differs, appends a new version (attributes changed)

## Parquet File Format

### Content-Addressed Normalization

The file eliminates cross-series duplication using a **table + mapping** pattern. Many series sharing the same OTel resource produce **1 table row + N mapping rows** instead of N full copies.

Content is keyed by `xxhash.Sum64` of sorted attributes (deterministic regardless of map iteration order). The hash covers all content fields but excludes time ranges.

### Namespace Types

Each row has a `namespace` discriminator field:

| Namespace | Purpose | Key Fields Used |
|-----------|---------|-----------------|
| `resource_table` | Unique resource content | `content_hash`, `identifying_attrs`, `descriptive_attrs`, `entities` |
| `resource_mapping` | Series → resource + time range | `series_ref`, `content_hash`, `mint`, `maxt` |
| `scope_table` | Unique scope content | `content_hash`, `scope_name`, `scope_version_str`, `schema_url`, `scope_attrs` |
| `scope_mapping` | Series → scope + time range | `series_ref`, `content_hash`, `mint`, `maxt` |
| `resource_attr_index` | Inverted index: attr → series | `series_ref`, `content_hash`, `identifying_attrs[0]`, `attr_key`, `attr_value` |

### Parquet Schema

A single `metadataRow` struct covers all four namespace types. Fields unused by a namespace are zero-valued.

```
metadataRow
├── namespace          string        (row type discriminator)
├── series_ref         uint64        (series identifier for mapping rows)
├── mint               int64?        (minimum timestamp, milliseconds)
├── maxt               int64?        (maximum timestamp, milliseconds)
├── content_hash       uint64?       (xxhash content key)
├── identifying_attrs  list?         (resource identifying attributes)
│   └── element
│       ├── key        string
│       └── value      string
├── descriptive_attrs  list?         (resource descriptive attributes)
│   └── element
│       ├── key        string
│       └── value      string
├── entities           list?         (typed OTel entities)
│   └── element
│       ├── type       string
│       ├── id         list          (entity identifying attributes)
│       │   └── element
│       │       ├── key    string
│       │       └── value  string
│       └── description list         (entity descriptive attributes)
│           └── element
│               ├── key    string
│               └── value  string
├── scope_name         string?       (InstrumentationScope name)
├── scope_version_str  string?       (InstrumentationScope version)
├── schema_url         string?       (InstrumentationScope schema URL)
├── scope_attrs        list?         (InstrumentationScope attributes)
│   └── element
│       ├── key        string
│       └── value      string
├── attr_key           string?       (resource_attr_index: attribute name)
└── attr_value         string?       (resource_attr_index: attribute value)
```

### SeriesRef and the Resolver Pattern

Mapping rows store `series_ref` — the block-level series reference (a small integer from the block's index). The in-memory model uses `labelsHash` (a stable 64-bit hash of the series' labels). Conversion between the two happens at the Parquet boundary:

- **Write path**: `WriterOptions.RefResolver` converts `labelsHash → seriesRef`. During compaction, this resolver is built by scanning the new block's postings. When no resolver is provided (head/test writes), `labelsHash` is written directly as `series_ref`.
- **Read path**: `WithRefResolver` reader option converts `seriesRef → labelsHash`. During block open, this resolver calls the block's index reader to look up the series' labels and compute `labels.StableHash()`. When no resolver is provided, `series_ref` is used as-is.

### Row Groups and Physical Layout

Each namespace is written as a separate row group (or multiple row groups if `MaxRowsPerRowGroup` is set). This enables selective reads — a reader can skip entire row groups based on namespace column statistics.

Write order (alphabetical by namespace value): `resource_mapping`, `resource_table`, `scope_mapping`, `scope_table`, then `resource_attr_index` (when enabled).

Within each namespace, rows are sorted by `(series_ref, content_hash, mint)` for better zstd compression.

### Footer Metadata

Parquet footer key-value pairs:

| Key | Description |
|-----|-------------|
| `schema_version` | Currently `"1"` |
| `resource_table_count` | Number of unique resource content rows |
| `resource_mapping_count` | Number of series→resource mapping rows |
| `scope_table_count` | Number of unique scope content rows |
| `scope_mapping_count` | Number of series→scope mapping rows |
| `resource_attr_index_count` | Number of inverted index rows (when enabled) |
| `row_group_layout` | `"namespace_partitioned"` |

### Compression

All data is zstd-compressed at `SpeedBetterCompression` level. Typical file sizes are kilobytes to low megabytes.

## API

### Writing

```go
// Simple write (no resolver, labelsHash stored as series_ref)
size, err := WriteFile(logger, blockDir, reader)

// With options (compaction uses this)
size, err := WriteFileWithOptions(logger, blockDir, reader, WriterOptions{
    RefResolver:          func(labelsHash uint64) (seriesRef uint64, ok bool) { ... },
    MaxRowsPerRowGroup:   10000,
    EnableBloomFilters:   true,
    EnableInvertedIndex:  true,
    IndexedResourceAttrs: map[string]struct{}{"k8s.namespace.name": {}},
})
```

### Reading

```go
// From file path (returns empty reader if file doesn't exist)
reader, size, err := ReadSeriesMetadata(logger, blockDir,
    WithRefResolver(func(seriesRef uint64) (labelsHash uint64, ok bool) { ... }),
)
defer reader.Close()

// From io.ReaderAt (for object storage / distributed systems)
reader, err := ReadSeriesMetadataFromReaderAt(logger, readerAt, size,
    WithNamespaceFilter("resource_table", "resource_mapping"),
    WithRefResolver(resolver),
)
```

### Querying

```go
// Get latest resource for a series
rv, ok := reader.GetResource(labelsHash)

// Get resource active at a specific timestamp
rv, ok := reader.GetResourceAt(labelsHash, timestampMs)

// Get all versions
vr, ok := reader.GetVersionedResource(labelsHash)
for _, version := range vr.Versions {
    fmt.Println(version.MinTime, version.Identifying, version.Descriptive)
}

// Iterate all series (type-safe)
reader.IterVersionedResources(ctx, func(labelsHash uint64, vr *VersionedResource) error {
    // ...
    return nil
})

// Iterate via kind framework (generic)
reader.IterKind(ctx, KindResource, func(labelsHash uint64, versioned any) error {
    vr := versioned.(*VersionedResource)
    // ...
    return nil
})

// Reverse lookup: find series by resource attribute (O(1) with index)
hashes := reader.LookupResourceAttr("service.name", "payment-service")
for hash := range hashes {
    vr, _ := reader.GetVersionedResource(hash)
    // verify time range / additional filters...
}
```

## Entities

Entities represent typed OTel resources within a `ResourceVersion`. Seven predefined types:

| Entity Type | Example Identifying Attributes | Example Descriptive Attributes |
|-------------|-------------------------------|-------------------------------|
| `service` | `service.name`, `service.namespace`, `service.instance.id` | `deployment.environment` |
| `host` | `host.name` | `cloud.region`, `cloud.provider` |
| `container` | `container.id` | `container.image.name` |
| `k8s.pod` | `k8s.pod.uid` | `k8s.pod.name` |
| `k8s.node` | `k8s.node.uid` | `k8s.node.name` |
| `process` | `process.pid` | `process.command` |
| `resource` | (default type) | (all non-identifying attributes) |

Entities are derived from OTel resource attributes using `entity.ResourceEntityRefs()` from the xpdata package during OTLP ingestion.

## Distributed-Scale Features

Several features are designed for object-storage access patterns in clustered implementations (e.g. Grafana Mimir store-gateway):

- **`io.ReaderAt` API**: `ReadSeriesMetadataFromReaderAt()` decouples from `*os.File`, enabling `objstore.Bucket`-backed readers
- **Namespace filtering**: `WithNamespaceFilter()` skips non-matching row groups using Parquet column index min/max bounds
- **Bloom filters**: `WriterOptions.EnableBloomFilters` adds split-block bloom filters on `series_ref`, `content_hash`, `attr_key`, and `attr_value` columns. Write-only in this package; querying happens in the consumer. The `attr_key`/`attr_value` bloom filters enable store-gateways to find matching `resource_attr_index` rows via Parquet-native filtering
- **Selective resource attribute indexing**: `WriterOptions.IndexedResourceAttrs` controls which descriptive attributes appear in the inverted index. Identifying attributes are always indexed. This reduces index size by ~10x at scale
- **Row group size limits**: `WriterOptions.MaxRowsPerRowGroup` bounds memory usage when reading large row groups

## File Organization

| File | Contents |
|------|----------|
| `seriesmetadata.go` | Core types (`Reader`, `MemSeriesMetadata`, `parquetReader`), write/read paths, denormalization, resource attribute inverted index |
| `versioned.go` | Generic `Versioned[V]` container, `VersionConstraint` interface, `KindOps[V]`, `MergeVersioned()` |
| `mem_store.go` | Generic `MemStore[V]` in-memory store |
| `registry.go` | `KindDescriptor` interface, `KindID`, global kind registry, `kindMetaAccessor` |
| `resource_kind.go` | `resourceKindDescriptor` (implements `KindDescriptor` for resources), `ResourceOps`, `ResourceCommitData`, content hashing |
| `scope_kind.go` | `scopeKindDescriptor` (implements `KindDescriptor` for scopes), `ScopeOps`, `ScopeCommitData`, content hashing |
| `parquet_schema.go` | Parquet schema (`metadataRow`, `EntityRow`, `EntityAttributeEntry`), namespace constants |
| `entity.go` | `Entity`, `ResourceVersion`, type aliases (`VersionedResource`, `MemResourceStore`, `VersionedResourceReader`) |
| `scope.go` | `ScopeVersion`, type aliases (`VersionedScope`, `MemScopeStore`, `VersionedScopeReader`) |
| `content_hash.go` | Shared `hashAttrs()` utility for deterministic xxhash of attribute maps |
| `resource_attributes.go` | `SplitAttributes()`, `IsIdentifyingAttribute()`, `AttributesEqual()` |
| `writer_options.go` | `WriterOptions` (bloom filters, row group limits, `IndexedResourceAttrs`, `RefResolver`) |
| `reader_options.go` | `ReaderOption`, `WithNamespaceFilter()`, `WithRefResolver()`, `ReadSeriesMetadataFromReaderAt()` |
