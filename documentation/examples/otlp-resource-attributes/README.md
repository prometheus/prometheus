# OTLP Resource & Scope Attributes Persistence Demo

This demo showcases how Prometheus persists OTel resource attributes and scope
(instrumentation library) attributes from OTLP metrics, making them queryable
via the `/api/v1/resources` endpoint.

## Overview

When Prometheus receives metrics via OTLP, each resource contains attributes
that describe the source of the metrics (service.name, host.name, etc.), and
each scope contains attributes that describe the instrumentation library that
produced the metrics (name, version, custom attributes).

This demo shows how these attributes are:

1. Ingested from OTLP metrics
2. Stored per-series in the TSDB head (in-memory)
3. Persisted to Parquet files during block compaction
4. Retrieved from both head and compacted blocks
5. Exposed via the `/api/v1/resources` API

## Running the Demo

```bash
go run ./documentation/examples/otlp-resource-attributes
```

## Demo Phases

### Phase 1: Send OTLP Metrics
Sends metrics from multiple services with diverse resource and scope attributes:
- payment-service in production (scope: `github.com/example/payment` v1.2.0)
- order-service in production (scope: `github.com/example/orders` v0.9.1)
- payment-service in staging (scope: `github.com/example/payment` v1.1.0)

### Phase 2: Query from Head
Shows resource and scope attributes stored in-memory in the TSDB head.

### Phase 3: WAL Replay
Closes and reopens the TSDB to exercise Write-Ahead Log (WAL) replay. Verifies
that resource and scope records survive a restart by querying the replayed head.

### Phase 4: Compact to Disk
Forces head compaction to persist data to a Parquet block file.

### Phase 5: Query from Blocks
Shows resource and scope attributes retrieved from the persisted Parquet file.

### Phase 6: Descriptive Attributes Changing Over Time
Demonstrates how non-identifying (descriptive) attributes can change while
identifying attributes remain constant. Simulates a service migration where:
- **Identifying attributes stay the same**: service.name, service.namespace, service.instance.id
- **Descriptive attributes change**:
  - `host.name`: prod-payment-1 → prod-payment-2
  - `cloud.region`: us-west-2 → eu-west-1
  - `k8s.pod.name`: NEW attribute added
- **Scope version changes**: v1.2.0 → v1.3.0 (library upgraded during migration)

### Phase 7: info() Query
Uses the `info()` PromQL function to enrich metrics with resource attributes,
demonstrating time-varying attribute resolution.

### Phase 8: API Response Format
Demonstrates the JSON format returned by `/api/v1/resources`, including both
resource versions and scope versions.

### Phase 9: Summary
Summarizes the key concepts demonstrated.

## Resource Attributes

The demo uses these OTel resource attributes:

**Identifying Attributes** (constant for a series, used for correlation):
- `service.name` - The logical name of the service
- `service.namespace` - The namespace/environment
- `service.instance.id` - Unique instance identifier

These attributes uniquely identify the resource and remain constant throughout
the lifetime of a series. They enable correlation with traces and logs.

**Descriptive Attributes** (can change over time):
- `host.name` - Hostname of the service (can change during migration)
- `cloud.region` - Cloud provider region (can change during migration)
- `deployment.environment` - Deployment environment
- `k8s.pod.name` - Kubernetes pod name (changes on pod restart)

These attributes describe the current state of the resource and may change
over time as infrastructure evolves (e.g., during migrations, scaling, restarts).

## Scope Attributes

Scope attributes describe the instrumentation library that produced the metrics:

- **Name** - The library package path (e.g., `github.com/example/payment`)
- **Version** - The library version (e.g., `1.2.0`)
- **Schema URL** - The OpenTelemetry schema URL for the scope
- **Custom Attributes** - Key-value pairs like `library.language=go`

Scope attributes are versioned just like resource attributes. When an
instrumentation library is upgraded (e.g., during a deployment), the new scope
version is recorded alongside the old one with appropriate time ranges.

## Architecture

```
OTLP Metrics                 TSDB Head              Parquet Block
┌─────────────────┐         ┌────────────┐         ┌────────────┐
│ ResourceMetrics │ ──────► │ In-memory  │ ──────► │ series_    │
│   ├─ Resource   │ Ingest  │ storage    │ Compact │ metadata.  │
│   │  └─ Attrs   │         │            │         │ parquet    │
│   └─ ScopeMetrics         │  Resources │         │            │
│      ├─ Scope   │         │  + Scopes  │         │ Resources  │
│      │  ├─ Name │         │  per series│         │ + Scopes   │
│      │  ├─ Ver  │         │            │         │ per series │
│      │  └─ Attrs│         └────────────┘         └────────────┘
│      └─ Metrics │                │                      │
└─────────────────┘                ▼                      ▼
                            ┌─────────────────────────────────┐
                            │       /api/v1/resources          │
                            │   (combined head + blocks)      │
                            └─────────────────────────────────┘
```

## Configuration

To enable resource attribute persistence in Prometheus, set the TSDB option:

```go
opts := tsdb.DefaultOptions()
opts.EnableNativeMetadata = true
```

## Use Cases

- **Trace-to-Metrics Correlation**: Use service.name, service.namespace, and
  service.instance.id to correlate metrics with distributed traces
- **Resource Discovery**: Query what resources have reported metrics
- **Historical Analysis**: Understand which services were active during time ranges
- **Library Version Tracking**: Track which instrumentation library versions are
  producing metrics across your fleet
- **Migration Validation**: Verify that library upgrades propagated correctly by
  comparing scope versions before and after deployment
