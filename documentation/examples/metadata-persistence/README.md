# Persisted Series Metadata Demo

This demo showcases the persisted series metadata feature in Prometheus TSDB.

## Overview

Prometheus can now persist metric metadata (TYPE, HELP, UNIT) to disk alongside
time series data. This means that even after scrape targets are removed, the
metadata for historical metrics remains available via the `/api/v1/metadata`
API endpoint.

## The Problem This Solves

Previously, Prometheus only kept metadata in memory from active scrape targets.
When a target stopped being scraped, its metadata was lost. This made it
difficult to:

- Understand what historical metrics meant
- Debug old metrics in dashboards
- Maintain documentation for decommissioned services

## How It Works

```
┌─────────────────┐     scrape      ┌─────────────────┐
│  Mock Exporter  │ ←───────────────│  Scrape/Parse   │
│  TYPE/HELP/UNIT │   (protobuf)    └────────┬────────┘
└─────────────────┘                          │
                                             │ append
                                             ↓
                                    ┌─────────────────┐
                                    │   TSDB Head     │
                                    │   (in-memory)   │
                                    └────────┬────────┘
                                             │
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
                                                      │ query
                                                      ↓
                                             ┌─────────────────┐
                                             │ /api/v1/metadata│
                                             └─────────────────┘
```

1. **Scraping**: Metadata is captured from TYPE/HELP/UNIT comments in metrics (via protobuf format)
2. **Storage**: Metadata is stored in TSDB head (in-memory)
3. **WAL Replay**: Metadata survives TSDB restarts via Write-Ahead Log replay
4. **Persistence**: When blocks are compacted, metadata is written to Parquet files
5. **Querying**: The `/api/v1/metadata` endpoint returns both active and persisted metadata

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

### Phase 4: Stop Exporter
Stops the mock exporter to simulate a target being removed. Without metadata
persistence, this metadata would be lost.

### Phase 5: Compact to Disk
Forces head compaction to persist data to a Parquet block file
(`series_metadata.parquet`).

### Phase 6: Query from Blocks
Shows metadata retrieved from the persisted Parquet file, demonstrating that
metadata remains available even after the scrape target is gone.

### Phase 7: API Response Format
Demonstrates the JSON format returned by `/api/v1/metadata`.

### Phase 8: Summary
Summarizes the key concepts demonstrated.

## Key Output

```
Response Content-Type: application/vnd.google.protobuf; ...
Stored 4 metrics with metadata in TSDB head (in-memory)
  - Float samples: 10
  - Native histograms: 1
...
Metadata after WAL replay:
  demo_http_requests_total:
    Type: counter
    Help: Total number of HTTP requests received.
...
Parquet metadata file created: .../series_metadata.parquet
```

This confirms that:
- Protobuf format is used for scraping (required for native histograms)
- Native histogram samples are being stored
- Metadata survives WAL replay (TSDB restart)
- Metadata was persisted to a Parquet file in the block directory

## Demo Components

### Mock Exporter (`exporter.go`)

A simple HTTP server that exposes metrics using the prometheus client library:

- `demo_http_requests_total` (counter) - TYPE, HELP
- `demo_temperature_celsius` (gauge) - TYPE, HELP
- `demo_request_duration_seconds` (native histogram) - TYPE, HELP, with exponential buckets
- `demo_response_size_bytes` (summary) - TYPE, HELP

The exporter uses `promhttp.Handler` which automatically negotiates content type
and serves protobuf format when requested, enabling native histogram support.

### Main Demo (`main.go`)

Orchestrates the demonstration:

1. Creates a temporary TSDB with `EnableNativeMetadata = true`
2. Starts the mock exporter
3. Scrapes and parses metrics using `textparse` (protobuf format)
4. Appends float samples and native histograms to TSDB
5. Closes and reopens TSDB to verify WAL replay
6. Demonstrates persistence by compacting and querying

## API Response Format

The `/api/v1/metadata` endpoint returns metadata in this format:

```json
{
  "status": "success",
  "data": {
    "demo_request_duration_seconds": [
      {
        "type": "histogram",
        "help": "Time spent processing requests.",
        "unit": ""
      }
    ],
    "demo_temperature_celsius": [
      {
        "type": "gauge",
        "help": "Current temperature in the demo environment.",
        "unit": ""
      }
    ]
  }
}
```

## Key Files in Prometheus

- `tsdb/seriesmetadata/seriesmetadata.go` - Parquet reader/writer for metadata
- `tsdb/head.go` - Head block metadata storage
- `tsdb/compact.go` - Metadata merging during compaction
- `web/api/v1/api.go` - API endpoint integration

## Learn More

- [Prometheus TSDB Documentation](https://prometheus.io/docs/prometheus/latest/storage/)
- [Native Histograms](https://prometheus.io/docs/concepts/native_histograms/)
- [OpenMetrics Specification](https://openmetrics.io/) (TYPE, HELP, UNIT)
