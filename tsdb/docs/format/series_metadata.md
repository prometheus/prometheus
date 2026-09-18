# Series Metadata

> **This is an experimental feature gated behind `--enable-feature=native-metadata`.**

## Overview

Series metadata stores metric metadata (TYPE, HELP, UNIT) alongside TSDB blocks,
with support for time-varying metadata — tracking how metric definitions change
over time (e.g., when an exporter upgrade changes a metric's help text). This
allows metadata to persist across restarts and be available even after scrape
targets are removed.

## File

Each block directory may contain a `series_metadata.parquet` file alongside the
existing `index`, `chunks/`, `tombstones`, and `meta.json` files.

## Format

The file uses [Apache Parquet](https://parquet.apache.org/) format with a unified
row schema. A `namespace` discriminator column splits rows into two logical tables:

| Column         | Type   | Description                                                |
|----------------|--------|------------------------------------------------------------|
| `namespace`    | string | Row type: `"metadata_table"` or `"metadata_mapping"`       |
| `series_ref`   | uint64 | Series reference (foreign key into the block index)        |
| `mint`         | int64  | Start of the time range this metadata version covers (ms)  |
| `maxt`         | int64  | End of the time range this metadata version covers (ms)    |
| `content_hash` | uint64 | Hash linking mapping rows to content table rows            |
| `metric_name`  | string | The `__name__` label value for the metric                  |
| `type`         | string | Metric type (counter, gauge, histogram, etc.)              |
| `unit`         | string | Metric unit (e.g., bytes, seconds)                         |
| `help`         | string | Metric help text                                           |

All columns except `namespace` are optional — each namespace uses a different
subset.

### `metadata_table` rows (content-addressed)

Store unique (type, unit, help) tuples identified by `content_hash`. Used columns:
`content_hash`, `type`, `unit`, `help`.

### `metadata_mapping` rows (series → versioned metadata)

Map each series to its metadata versions over time. Used columns: `series_ref`,
`metric_name`, `content_hash`, `mint`, `maxt`. The `content_hash` links to a
`metadata_table` row, and `mint`/`maxt` define the time range during which that
metadata version was observed.

A single series can have multiple mapping rows — one per metadata version — which
is how metadata changes over time are tracked.

## Compaction

During compaction, `mergeAndWriteSeriesMetadata()` runs three phases:

1. **Collect**: Read versioned metadata from each source block, resolve each
   block's series refs to labels via its index, and merge by `labels.StableHash`.
2. **Remap**: Open the new block's index and map label hashes to new series refs.
3. **Write**: Build `MemSeriesMetadata` with new refs and write via `WriteFile()`.

When the same metric has metadata versions in multiple source blocks, all versions
are preserved and merged by time range.

## Merge behavior

`DB.SeriesMetadata()` merges versioned metadata across all persisted blocks and
the in-memory head. Versions from all sources are combined and deduplicated,
preserving the full version history.

When native metadata is enabled, `/api/v1/metadata` uses TSDB as the sole
metadata source. The `/api/v1/metadata/versions` endpoint exposes the full
version history with time ranges, and `/api/v1/metadata/series` provides inverse
metadata lookup (finding metrics by type, unit, or help text).
