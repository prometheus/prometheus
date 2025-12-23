# Persisted Series Metadata Demo

This demo showcases the persisted series metadata feature in Prometheus TSDB,
including time-varying metadata that tracks how metric definitions change over time.

## Overview

Prometheus can now persist metric metadata (TYPE, HELP, UNIT) to disk alongside
time series data. This means that even after scrape targets are removed, the
metadata for historical metrics remains available via the `/api/v1/metadata`
API endpoint. When metadata changes over time (e.g., an exporter binary upgrade
changes a metric's help text), each version is tracked with its time range and
queryable through the `/api/v1/metadata/versions` endpoint.

## The Problem This Solves

Previously, Prometheus only kept metadata in memory from active ingestion
sources (scrape targets, remote write senders, OTLP exporters). When a source
stopped sending data, its metadata was lost. This made it difficult to:

- Understand what historical metrics meant
- Debug old metrics in dashboards
- Maintain documentation for decommissioned services
- Track when and how metric definitions changed across exporter upgrades

## How It Works

```
┌─────────────────┐     scrape      ┌─────────────────┐
│    Exporter     │ ←───────────────│  Scrape/Parse   │
│  TYPE/HELP/UNIT │   (protobuf)    └────────┬────────┘
└─────────────────┘                          │
        │                                    │ append
   upgrade (help                             ↓
   text changes)                    ┌─────────────────┐
        │                           │   TSDB Head     │
        ↓                           │   (in-memory)   │
┌─────────────────┐                 │  v1: help="old" │
│    Exporter     │  scrape again   │  v2: help="new" │
│  (new version)  │ ←───────────    └────────┬────────┘
└─────────────────┘                          │
                                    ┌────────┴────────┐
                                    │                 │
                              WAL   │                 │ compact
                             replay │                 │
                                    ↓                 ↓
                           ┌─────────────┐   ┌─────────────────┐
                           │   TSDB Head │   │ Persisted Block │
                           │  (restored) │   │ series_metadata │
                           └─────────────┘   │    .parquet     │
                                             └────────┬────────┘
                                                      │
                                             ┌────────┴────────┐
                                             │                 │
                                             ↓                 ↓
                                    ┌────────────────┐ ┌───────────────────┐
                                    │/api/v1/metadata│ │/api/v1/metadata/  │
                                    │  (latest only) │ │    versions       │
                                    └────────────────┘ │ (version history) │
                                                       └───────────────────┘
```

1. **Ingestion**: Metadata is captured from any ingestion path — scraping (TYPE/HELP/UNIT comments), remote write v2 (proto metadata fields), or OTLP (metric kind, description, unit). See [Ingestion Paths](#ingestion-paths) below
2. **Storage**: Metadata is stored in TSDB head (in-memory) with version tracking
3. **WAL Replay**: Metadata survives TSDB restarts via Write-Ahead Log replay
4. **Versioning**: When metadata changes, a new version is created with its time range
5. **Persistence**: When blocks are compacted, metadata versions are written to Parquet files
6. **Querying**: The `/api/v1/metadata/versions` endpoint returns version history with time ranges
7. **Inverse lookup**: The `/api/v1/metadata/series` endpoint finds metrics by type, unit, or help text

## Ingestion Paths

Metadata persistence works identically for scrape, remote write v2, and OTLP.
These paths produce the same `metadata.Metadata{Type, Help, Unit}` struct, and
once metadata reaches the TSDB appender, storage, versioning, WAL replay,
compaction, and querying behave the same. Remote write v1 is an exception — see below.

| Path | TYPE source | HELP source | UNIT source |
|------|------------|-------------|-------------|
| **Scrape** | `# TYPE` comment in exposition format (text or protobuf) | `# HELP` comment | `# UNIT` comment |
| **Remote write v2** | `Metadata.Type` enum in `writev2.TimeSeries` proto | `Metadata.HelpRef` (symbol table) | `Metadata.UnitRef` (symbol table) |
| **Remote write v1**\* | `MetricMetadata.Type` enum in top-level `WriteRequest.metadata` list | `MetricMetadata.Help` string | `MetricMetadata.Unit` string |
| **OTLP** | Mapped from OTel metric kind (e.g. monotonic `Sum` → counter) | `metric.Description()` | `metric.Unit()` with Prometheus unit convention transforms |

\* Metadata from remote write v1 is not persisted to TSDB — see below.

**Scrape** parses `# TYPE`, `# HELP`, and `# UNIT` comments from the exposition
format (protobuf or text). Metadata appending is gated by the
`--enable-feature=metadata-wal-records` global CLI flag.

**Remote write v2** carries metadata in the `Metadata` message of each
`writev2.TimeSeries` proto — type as an enum, help and unit as symbol table
references. Metadata appending is gated by the `appendMetadata` flag on the
write handler.

**Remote write v1** carries metadata in a separate top-level `WriteRequest.metadata`
list, decoupled from timeseries — each `MetricMetadata` message identifies itself
by `metric_family_name` rather than series labels. On the sender side, metadata is
sent as out-of-band batches via the `MetadataWatcher` (gated by
`metadata_config.send`, disabled by default). However, the v1 spec explicitly
excludes metadata as experimental, and the Prometheus receiver does not persist it.
**Metadata from remote write v1 is not written to TSDB.** Users needing metadata
persistence over remote write should use remote write v2.

**OTLP** maps from OpenTelemetry metric attributes — type from the metric kind
(e.g. monotonic `Sum` → counter, `Gauge` → gauge), help from
`metric.Description()`, and unit from `metric.Unit()` with Prometheus unit
convention transforms (e.g. `ms` → `milliseconds`). OTLP metadata ingestion is
always enabled with no additional flag gating.

This demo uses scraping as the example since it is the most common ingestion
path, but the same metadata storage and querying applies to all three.

## Running the Demo

```bash
# From the Prometheus repository root
go run ./documentation/examples/metadata-persistence/...
```

## Configuration

To enable native metadata persistence in Prometheus, use the `--enable-feature` flag:

```
prometheus --enable-feature=native-metadata
```

Or when using the TSDB directly:

```go
opts := tsdb.DefaultOptions()
opts.EnableNativeMetadata = true
```

## Demo Phases

### Phase 1: Scrape Metrics
Scrapes metrics from a mock exporter using protobuf format and stores them in
TSDB with their metadata (TYPE, HELP, UNIT).

### Phase 2: Query from Head
Shows metadata stored in-memory in the TSDB head.

### Phase 3: WAL Replay
Closes and reopens the TSDB to exercise Write-Ahead Log (WAL) replay. Verifies
that metadata records survive a restart by querying the replayed head.

### Phase 4: Simulate Exporter Upgrade
Upgrades the mock exporter with changed help text for two metrics, then scrapes
again at a later timestamp. This creates version history in the metadata store:
- `demo_http_requests_total`: Help changes from "Total number of HTTP requests received." to "Total HTTP requests processed by the server."
- `demo_request_duration_seconds`: Help changes from "Time spent processing requests." to "Latency of request processing in seconds."

### Phase 5: Query Version History
Queries versioned metadata from the TSDB head, showing all versions per metric
with their time ranges. Changed metrics show 2 versions; unchanged metrics show 1.

### Phase 6: Stop Exporter
Stops the mock exporter to simulate a target being removed. Without metadata
persistence, this metadata would be lost.

### Phase 7: Compact to Disk
Forces head compaction to persist data to a Parquet block file
(`series_metadata.parquet`), including version history.

### Phase 8: Query from Blocks
Shows metadata retrieved from the persisted Parquet file, demonstrating that
metadata remains available even after the scrape target is gone.

### Phase 9: Metadata API Response Formats
Demonstrates the JSON formats returned by `/api/v1/metadata/versions` (version
history with `minTime`/`maxTime` per version) and `/api/v1/metadata/series`
(inverse metadata lookup — finding metrics by type, unit, or help text).

### Phase 10: Summary
Summarizes the key concepts demonstrated.

## Key Output

```
--- Phase 5: Querying metadata version history ---

Versioned metadata from TSDB head:
  demo_http_requests_total (2 versions):
    v1 [2026-02-15T10:15:06Z → 2026-02-15T10:15:06Z]
      Type: counter
      Help: Total number of HTTP requests received.
    v2 [2026-02-15T11:15:06Z → 2026-02-15T11:15:06Z]
      Type: counter
      Help: Total HTTP requests processed by the server.
  demo_request_duration_seconds (2 versions):
    v1 [2026-02-15T10:15:06Z → 2026-02-15T10:15:06Z]
      Type: histogram
      Help: Time spent processing requests.
    v2 [2026-02-15T11:15:06Z → 2026-02-15T11:15:06Z]
      Type: histogram
      Help: Latency of request processing in seconds.
  ...
```

## Demo Components

### Mock Exporter (`exporter.go`)

A simple HTTP server that exposes metrics using the prometheus client library:

- `demo_http_requests_total` (counter) - TYPE, HELP
- `demo_temperature_celsius` (gauge) - TYPE, HELP
- `demo_request_duration_seconds` (native histogram) - TYPE, HELP, with exponential buckets
- `demo_response_size_bytes` (summary) - TYPE, HELP

The exporter supports an `UpgradeMetrics()` method that simulates a binary upgrade
by swapping to a fresh registry with changed help text for two metrics.

### Main Demo (`main.go`)

Orchestrates the demonstration:

1. Creates a temporary TSDB with `EnableNativeMetadata = true`
2. Starts the mock exporter and scrapes metrics
3. Closes and reopens TSDB to verify WAL replay
4. Upgrades the exporter and scrapes again to create version history
5. Queries versioned metadata showing per-metric version history
6. Demonstrates persistence by compacting and querying
7. Shows the `/api/v1/metadata/versions` and `/api/v1/metadata/series` API response formats

## API Response Format

The `/api/v1/metadata/versions` endpoint returns versioned metadata:

```json
{
  "status": "success",
  "data": [
    {
      "labels": {"__name__": "demo_http_requests_total", "instance": "127.0.0.1:8080", "job": "demo"},
      "versions": [
        {"type": "counter", "help": "Total number of HTTP requests received.", "unit": "", "minTime": 1771150506221, "maxTime": 1771150506221},
        {"type": "counter", "help": "Total HTTP requests processed by the server.", "unit": "", "minTime": 1771154106221, "maxTime": 1771154106221}
      ]
    },
    {
      "labels": {"__name__": "demo_request_duration_seconds", "instance": "127.0.0.1:8080", "job": "demo"},
      "versions": [
        {"type": "histogram", "help": "Time spent processing requests.", "unit": "", "minTime": 1771150506221, "maxTime": 1771150506221},
        {"type": "histogram", "help": "Latency of request processing in seconds.", "unit": "", "minTime": 1771154106221, "maxTime": 1771154106221}
      ]
    }
  ]
}
```

The `/api/v1/metadata/series` endpoint performs inverse metadata lookup — finding
metrics that match given metadata criteria (type, unit, or help regex):

```json
{
  "status": "success",
  "data": [
    {
      "labels": {"__name__": "demo_http_requests_total", "instance": "127.0.0.1:8080", "job": "demo"},
      "versions": [
        {"type": "counter", "help": "Total number of HTTP requests received.", "unit": "", "minTime": 1771150506221, "maxTime": 1771150506221},
        {"type": "counter", "help": "Total HTTP requests processed by the server.", "unit": "", "minTime": 1771154106221, "maxTime": 1771154106221}
      ]
    }
  ]
}
```

## Key Files in Prometheus

- `tsdb/seriesmetadata/seriesmetadata.go` - Parquet reader/writer for metadata
- `tsdb/seriesmetadata/versioned_metadata.go` - Version tracking and merging
- `tsdb/head.go` - Head block metadata storage
- `tsdb/compact.go` - Metadata merging during compaction
- `web/api/v1/api.go` - API endpoint integration (`/api/v1/metadata/versions`, `/api/v1/metadata/series`)

## Learn More

- [Prometheus TSDB Documentation](https://prometheus.io/docs/prometheus/latest/storage/)
- [Native Histograms](https://prometheus.io/docs/concepts/native_histograms/)
- [OpenMetrics Specification](https://openmetrics.io/) (TYPE, HELP, UNIT)
