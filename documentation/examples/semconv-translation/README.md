# Semantic Conventions & Telemetry Policies Read Demo

This demo showcases Prometheus's ability to maintain query continuity across semantic-conventions evolutions using both legacy OTel schema `1.1.0` files and OTel Telemetry Policies (`file_format: transform/2.0`, [open-telemetry/weaver#1850](https://github.com/open-telemetry/weaver/pull/1850)).

## The Problem

When semantic conventions evolve, metrics and attributes can be renamed, enum values can be normalized, new attributes can be conditionally derived, or multiplexed metrics can be split into dedicated metrics:

| Evolution | Before (1h–2h ago) | After (last 1h) |
|-----------|--------------------|-----------------|
| Semconv `1.0.0 → 1.1.0` (`registry.yaml`) | `test.counter{user="acme"}` | `test{tenant="acme"}` |
| HTTP `1.20.0 → 1.23.1` (`http_1.20.0_to_1.23.1.yaml`) | `http.server.duration{http.method="POST", http.status_code="503", net.protocol.version="2.0", http.scheme="https"}` | `http.server.request.duration{http.request.method="POST", http.response.status_code="503", network.protocol.version="2", url.scheme="https", error.type="503"}` |
| K8s `1.18.0 → 1.44.0` (`k8s_1.18.0_to_1.44.0.yaml`) | `k8s.resource_quota.hard_limit{resource="limits.memory" \| "requests.hugepages-2Mi", k8s.resource_quota.name="compute-quota"}` | `k8s.resourcequota.memory.limit.hard{k8s.resourcequota.name="compute-quota"}` & `k8s.resourcequota.hugepage_count.request.hard{k8s.hugepage.size="2Mi", k8s.resourcequota.name="compute-quota"}` |

**This breaks existing queries and dashboards:**
- Queries for the old or new name only return data from one side of the upgrade.
- Even when metric names are bridged, mismatched attribute names, unmapped enum values (`"2.0"` vs `"2"`), or missing derived attributes (`error.type`) split the series.

(Native OTel names contain dots, so they're written with PromQL's quoted selector syntax, e.g. `{"test.counter"}`.)

## The Solution

### 1. Legacy OTel Schema (`file_format: 1.1.0`)

Add `__semconv_url__` (which registry version to consult) together with
`__schema_url__` (which turns on schema-version rename fan-out). Prometheus
walks the schema's `versions` section, recovers the metric's historical names,
and merges the results under the requested version's name:

```promql
test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"}
sum by (tenant) (test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"})
```

### 2. OTel Telemetry Policies (`file_format: transform/2.0`)

When `__schema_url__` selects a `transform/2.0` policy file (`__semconv_url__` is optional because `source` and `target` schema URLs are declared in the policy file itself), Prometheus executes the 7-stage policy pipeline (`remove` → `rename`/`rename_prefix` → `split` → `add` → `map` → `convert` → `discard`) across matching `entity_transform`, `metric_transform`, and `metric_filter` rules:

```promql
# HTTP 1.20.0 → 1.23.1: metric/attribute renames + enum value mapping ("2.0" → "2") + 5xx error.type derivation
{"http.server.request.duration", __schema_url__="registry/http_1.20.0_to_1.23.1.yaml", "http.request.method"="POST", "network.protocol.version"="2", "error.type"="503"}

# K8s 1.18.0 → 1.44.0: predicate-based metric splits + regex split/discard attribute extraction + entity_transform renames
{"k8s.resourcequota.memory.limit.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml"}
{"k8s.resourcequota.hugepage_count.request.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml", "k8s.hugepage.size"="2Mi"}
{"k8s.pod.status.phase", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml", "k8s.pod.status.phase"="Running"}

# Multi-policy target schema_url discovery (fans out across both gRPC A66 and RPC 1.37.0 -> 1.45.0):
{"rpc.client.call.duration", __schema_url__="registry/1.45.0", "rpc.method"="checkout.CheckoutService/PlaceOrder"}

# Classic Prometheus <-> OTel UTF-8 conventions (prometheus_to_otel.yaml / otel_to_prometheus.yaml):
{"k8s.pod.network.io", __schema_url__="registry/prometheus_to_otel.yaml", "network.io.direction"="receive"}
```

### 3. Authoring-Time PromQL AST Rewriter (`semconv.RewritePromQLQuery`)

In addition to query-time storage translation (`semconv.AwareStorage`), `semconv.RewritePromQLQuery(query, defaultSchemaURL)` rewrites schema-aware PromQL queries at authoring time into standard PromQL (`or`, `label_replace`, `label_join`, and sample value filters) that can execute against an unmodified Prometheus TSDB.

## Running the Demo

### Option 1: Terminal Demo (Quick)

```bash
go run ./documentation/examples/semconv-translation
```

This runs seven phases: writing two eras (historical `1.0.0` / HTTP `1.20.0` / K8s `1.18.0` and upgraded `1.1.0` / HTTP `1.23.1` / K8s `1.44.0`), demonstrating how raw queries break at the upgrade boundary, and showing how `1.1.0` schemas and `transform/2.0` policies unify both eras.

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
- `sum by (tenant) (test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"})` — the 1.0.0 `user` series folded into `tenant`
- `{"http.server.request.duration", __schema_url__="registry/http_1.20.0_to_1.23.1.yaml", "network.protocol.version"="2", "error.type"="503"}` — HTTP `1.20.0` and `1.23.1` unified
- `{"k8s.resourcequota.memory.limit.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml"}` — K8s `1.18.0` multiplexed quota split into `1.44.0` memory limit
- `{"k8s.resourcequota.hugepage_count.request.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml", "k8s.hugepage.size"="2Mi"}` — K8s `1.18.0` hugepage quota split and size extracted

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
4. Results are merged and reported under the requested version's metric name,
   and historical attribute names are rewritten to the queried version's names
   (e.g. `user`→`tenant`) so series from every era share one label set.

Both values resolve exclusively against the embedded registry shipped with the
binary under `storage/semconv/registry/`; arbitrary HTTP URLs and local
filesystem paths are rejected. Queries without these matchers (or without a
`__name__` matcher) pass through to the underlying storage unchanged.

## Key Benefits

- **Dashboard continuity**: existing dashboards keep working across a rename.
- **No data gaps**: data from every era is accessible with one query.
- **Canonical naming**: results use the requested version's metric and attribute names.
