# Series Metadata

> **This is an experimental feature gated behind `--enable-feature=native-metadata`.**

## Overview

Series metadata stores metric metadata (TYPE, HELP, UNIT) alongside TSDB blocks.
This allows metadata to persist across restarts and be available even after scrape
targets are removed.

## File

Each block directory may contain a `series_metadata.parquet` file alongside the
existing `index`, `chunks/`, `tombstones`, and `meta.json` files.

## Format

The file uses [Apache Parquet](https://parquet.apache.org/) format with the
following schema:

| Column         | Type   | Description                                          |
|----------------|--------|------------------------------------------------------|
| `metric_name`  | string | The `__name__` label value for the metric            |
| `labels_hash`  | uint64 | Reserved for future use (always 0 in output)         |
| `type`         | string | Metric type (counter, gauge, histogram, etc.)        |
| `unit`         | string | Metric unit (e.g., bytes, seconds)                   |
| `help`         | string | Metric help text                                     |

## Deduplication

During compaction, metadata from source blocks is merged and deduplicated by
metric name. When the same metric name exists in multiple source blocks, the
metadata from the later block takes precedence.

## Merge behavior

`DB.SeriesMetadata()` merges metadata across all persisted blocks and the
in-memory head. Block metadata is applied in order (oldest first), and head
metadata is applied last, so the most recent metadata always wins.

The `/api/v1/metadata` endpoint supplements scrape-target metadata with persisted
TSDB metadata. Active scrape target metadata always takes precedence.
