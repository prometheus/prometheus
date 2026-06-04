# Semantic Conventions Versioned Read Demo

This demo showcases Prometheus's ability to maintain query continuity when an OTLP producer's metric is renamed across semantic-conventions versions.

## The Problem

When a metric is renamed between semconv versions, its name changes:

| semconv version | Metric name |
|-----------------|-------------|
| 1.0.0 (before)  | `test.counter` |
| 1.1.0 (after)   | `test` |

**This breaks existing queries and dashboards:**
- Queries for `{"test.counter"}` only return data from before the rename.
- Queries for `test` only return data from after the rename.
- Dashboards show a gap at the rename point.

(Native OTel names contain dots, so they're written with PromQL's quoted
selector syntax, e.g. `{"test.counter"}`.)

## The Solution

Add `__semconv_url__` (which registry version to consult) together with
`__schema_url__` (which turns on schema-version rename fan-out). Prometheus
walks the schema's `versions` section, recovers the metric's historical names,
and merges the results under the requested version's name:

```promql
test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"}
```

This single query matches both the 1.0.0 (`test.counter`) and 1.1.0 (`test`)
eras, returning all data under the canonical `test` name.

## Running the Demo

### Option 1: Terminal Demo (Quick)

```bash
go run ./documentation/examples/semconv-translation
```

This runs four phases: writing two eras (semconv 1.0.0 `test.counter`, semconv
1.1.0 `test`), showing how each raw name covers only its own era, and how
`__schema_url__` spans the rename under one name.

### Option 2: Browser Demo with Prometheus UI

```bash
cd documentation/examples/semconv-translation
./demo.sh
```

This populates a TSDB with 2 hours of history, launches Prometheus serving it,
and opens the UI. Then try:

- `{"test.counter"}` — only the semconv 1.0.0 era
- `test` — only the semconv 1.1.0 era
- `test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"}` — both eras under `test`

### Option 3: Manual Browser Demo

```bash
# Populate the TSDB
go run ./documentation/examples/semconv-translation \
  --data-dir=/tmp/demo-data \
  --populate-only

# Launch Prometheus with the semconv feature enabled
./prometheus \
  --storage.tsdb.path=/tmp/demo-data \
  --config.file=/dev/null \
  --enable-feature=semconv-versioned-read

# Open http://localhost:9090 in your browser
```

## How It Works

This requires the experimental feature flag `--enable-feature=semconv-versioned-read`.

1. `__semconv_url__="registry/<version>"` selects a semantic-conventions version
   from the embedded registry. It supplies metric metadata and has no effect on
   its own.
2. `__schema_url__="registry/<file>"` selects an OTel schema file and triggers
   fan-out across that schema's per-version metric and attribute renames.
3. For a query carrying both plus a `__name__=…` matcher, Prometheus generates
   one variant per historical name (for the embedded registry: `test.counter`
   in 1.0.0 and `test.v2` in 1.2.0 alongside `test` in 1.1.0) and queries them
   in parallel.
4. Results are merged and reported under the requested version's metric name.

Both values resolve exclusively against the embedded registry shipped with the
binary under `storage/semconv/registry/`; arbitrary HTTP URLs and local
filesystem paths are rejected. Queries without these matchers (or without a
`__name__` matcher) pass through to the underlying storage unchanged.

## Key Benefits

- **Dashboard continuity**: existing dashboards keep working across a rename.
- **No data gaps**: data from every era is accessible with one query.
- **Canonical naming**: results use the requested version's metric name.
