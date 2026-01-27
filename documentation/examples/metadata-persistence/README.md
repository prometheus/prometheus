# Persisted Series Metadata Demo

This demo showcases the persisted series metadata feature in Prometheus TSDB.

## Overview

Prometheus can now persist metric metadata (TYPE, HELP, UNIT) to disk alongside time series data. This means that even after scrape targets are removed, the metadata for historical metrics remains available via the `/api/v1/metadata` API endpoint.

## The Problem This Solves

Previously, Prometheus only kept metadata in memory from active scrape targets. When a target stopped being scraped, its metadata was lost. This made it difficult to:

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
                                             │ compact
                                             ↓
                                    ┌─────────────────┐
                                    │ Persisted Block │
                                    │ series_metadata │
                                    │    .parquet     │
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
3. **Persistence**: When blocks are compacted, metadata is written to Parquet files
4. **Querying**: The `/api/v1/metadata` endpoint returns both active and persisted metadata

## Running the Demo

```bash
# From the Prometheus repository root
go run ./documentation/examples/metadata-persistence/...
```

## Expected Output

The demo runs through 7 phases:

1. **Phase 1**: Scrapes metrics from mock exporter (using protobuf format), stores in TSDB
2. **Phase 2**: Queries metadata from TSDB head (in-memory)
3. **Phase 3**: Stops the mock exporter (simulates target removal)
4. **Phase 4**: Compacts TSDB head to persist metadata to disk
5. **Phase 5**: Queries metadata from persisted blocks
6. **Phase 6**: Shows API response format
7. **Phase 7**: Summary

Key output to look for:

```
Response Content-Type: application/vnd.google.protobuf; ...
Stored 4 metrics with metadata in TSDB head (in-memory)
  - Float samples: 10
  - Native histograms: 1
...
Metadata file created: .../series_metadata.parquet
```

This confirms that:
- Protobuf format is used for scraping (required for native histograms)
- Native histogram samples are being stored
- Metadata was persisted to a Parquet file in the block directory

## Demo Components

### Mock Exporter (`exporter.go`)

A simple HTTP server that exposes metrics using the prometheus client library:

- `demo_http_requests_total` (counter) - TYPE, HELP
- `demo_temperature_celsius` (gauge) - TYPE, HELP
- `demo_request_duration_seconds` (native histogram) - TYPE, HELP, with exponential buckets
- `demo_response_size_bytes` (summary) - TYPE, HELP

The exporter uses `promhttp.Handler` which automatically negotiates content type and serves protobuf format when requested, enabling native histogram support.

### Main Demo (`main.go`)

Orchestrates the demonstration:

1. Creates a temporary TSDB
2. Starts the mock exporter
3. Scrapes and parses metrics using `textparse` (protobuf format)
4. Appends float samples and native histograms to TSDB
5. Demonstrates persistence by compacting and querying

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
