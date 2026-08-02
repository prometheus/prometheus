# tsdb/seriesmetadata

In-memory storage of OTel resource attributes per time series, gated by
`--enable-feature=native-metadata`.

This package is the per-head store; it does not persist to disk. Resource
attributes are reconstructed from the WAL on restart and dropped when the
head is compacted into a block.

## Concepts

- **ResourceVersion**: `{Identifying, Descriptive map[string]string, MinTime, MaxTime}`.
  Identifying attributes (`service.name`, `service.namespace`, `service.instance.id`
  per OTel semantic conventions) define resource identity; descriptive attributes
  are everything else. Each version carries a time range during which the
  `(identifying, descriptive)` tuple was observed.
- **Versioned[V]**: an ordered (by MinTime) chain of versions per series.
  Identical content across appends extends the latest version's MaxTime; a
  content change appends a new version.
- **MemStore[V]**: 256-way sharded map keyed by `labels.StableHash(lset)` of
  the series. Implements content-addressed deduplication via a parallel content
  table: many series sharing the same resource attributes point at a single
  canonical `*ResourceVersion`.
- **MemSeriesMetadata**: the head-side reader/writer wrapping
  `MemStore[*ResourceVersion]` plus a grow-only `uniqueAttrNames` cache used
  by PromQL `info()` for OTel↔Prometheus name translation.

## Lifecycle

1. OTLP ingest builds `storage.ResourceContext` per `ResourceMetrics` and
   carries it via `AppendV2Options.Resource` on every `Append`.
2. The head's `headAppender(V2)` buffers the update and at `Commit` writes a
   `record.ResourceUpdate` (type 14) WAL record before mutating the store.
3. On restart, WAL replay re-applies the records to rebuild the in-memory
   state.
4. Compaction does not write resource attributes to disk; the head's
   `MemSeriesMetadata` is the only authoritative store.

## Read paths

- `MemSeriesMetadata.GetResourceAt(labelsHash, t)` — what `promql.info()`
  uses to enrich query results with native resource attributes.
- `MemSeriesMetadata.UniqueResourceAttrNames()` — snapshot of all attribute
  names seen, used by `info()` to build the OTel↔Prom name mapping.
- `IterResources`, `IterVersionedResources`, `IterVersionedResourcesFlatInline`
  — iteration over all stored entries.
