# Semantic Conventions Translation Demo

This demo showcases Prometheus's ability to maintain query continuity when an OTLP producer's metric and label names change — both when it switches translation strategy (e.g. `UnderscoreEscapingWithSuffixes` → `NoTranslation`) and when it adopts a newer semantic-conventions version that renames metrics.

## The Problem

When you migrate an OTLP producer to a new translation strategy, the metric and label names change:

| Strategy | Metric Name | Label Name |
|----------|-------------|------------|
| `UnderscoreEscapingWithSuffixes` (before) | `test_bytes_total` | `http_response_status_code` |
| `NoTranslation` (after) | `test` | `http.response.status_code` |

**This breaks existing queries and dashboards:**
- Queries for `test_bytes_total` only return historical data (before migration)
- Queries for `test` only return new data (after migration)
- Dashboards show gaps at the migration point

The same breakage occurs across **semantic-convention versions**: semconv 1.0.0 named this metric `test.counter`, which 1.1.0 renamed to `test`. A producer that has lived through both a version upgrade and a strategy switch ends up with data under three different names.

## The Solution

Add `__semconv_url__` (which registry to consult) together with `__otlp_strategy__` (which turns on OTLP-strategy fan-out and picks the result dialect). Prometheus then generates query variants for all OTLP translation strategies and merges them, giving seamless data continuity across the migration:

```promql
test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}
```

This single query matches both semconv 1.1.0 naming conventions — escaped (`test_bytes_total`) and native (`test`) — and returns continuous data under native OTel names (`NoTranslation`). For existing dashboards that use the escaped names, query `test_bytes_total{__semconv_url__="registry/1.1.0", __otlp_strategy__="UnderscoreEscapingWithSuffixes"}` instead to get the same data under escaped names. (The queried name must be written in the declared dialect.) Recovering the earlier semconv 1.0.0 era as well requires adding `__schema_url__` (covered below).

## Running the Demo

### Option 1: Terminal Demo (Quick)

```bash
go run ./documentation/examples/semconv-translation
```

This runs eight phases and demonstrates:
1. **Semconv 1.0.0 era**: Writing metrics under the historical name `test.counter` with `UnderscoreEscapingWithSuffixes`
2. **Semconv 1.1.0 era, escaped**: Writing metrics under the renamed `test` with `UnderscoreEscapingWithSuffixes`
3. **Semconv 1.1.0 era, native**: Writing metrics under `test` with `NoTranslation`
4. **The problem**: Showing how each direct query covers only its own era
5. **OTLP-strategy solution**: Using `__otlp_strategy__` (with `__semconv_url__`) to cover the strategy migration within one semconv version
6. **Aggregation**: `sum()` works across the strategy migration boundary
7. **Rate calculation**: `rate()` works across the strategy migration boundary
8. **Schema-version solution**: Adding `__schema_url__` to cover the cross-version axis and surface all three eras

### Option 2: Browser Demo with Prometheus UI

```bash
cd documentation/examples/semconv-translation
./demo.sh
```

This script:
1. Populates a TSDB with sample data (3 hours of history)
2. Launches Prometheus serving that data
3. Opens your browser to the Prometheus UI with the demo query

Then try these queries in the UI:

**The Problem — each raw name covers only its own era:**
- `test_counter_bytes_total` — only the semconv 1.0.0 era
- `test_bytes_total` — only the escaped semconv 1.1.0 era
- `test` — only the native semconv 1.1.0 era

**The Solution — query continuity:**
- `test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}` — both 1.1.0 eras, native names
- `test_bytes_total{__semconv_url__="registry/1.1.0", __otlp_strategy__="UnderscoreEscapingWithSuffixes"}` — same data, escaped names
- `sum(test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"})` — aggregation works across the boundary
- `rate(test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}[5m])` — rate works too
- `test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml", __otlp_strategy__="NoTranslation"}` — all three eras

### Option 3: Manual Browser Demo

```bash
# Populate the TSDB
go run ./documentation/examples/semconv-translation \
  --data-dir=/tmp/demo-data \
  --populate-only

# Launch Prometheus with semconv feature enabled
./prometheus \
  --storage.tsdb.path=/tmp/demo-data \
  --config.file=/dev/null \
  --enable-feature=semconv-versioned-read

# Open http://localhost:9090 in your browser
```

## Demo Scenario

The demo simulates a single producer (`myapp:8080`) that evolves over three hours along two independent axes — its semantic-conventions version and its OTLP translation strategy.

### Data population (three eras)

**Semconv 1.0.0, escaped (3 hours ago to 2 hours ago):** the metric was `test.counter`, written with `UnderscoreEscapingWithSuffixes`:
```
test_counter_bytes_total{http_response_status_code="200", instance="myapp:8080"}
test_counter_bytes_total{http_response_status_code="404", instance="myapp:8080"}
```

**Semconv 1.1.0, escaped (2 hours ago to 1 hour ago):** the 1.1.0 schema renamed `test.counter` → `test`, still written with `UnderscoreEscapingWithSuffixes`:
```
test_bytes_total{http_response_status_code="200", instance="myapp:8080"}
test_bytes_total{http_response_status_code="404", instance="myapp:8080"}
```

**Semconv 1.1.0, native (1 hour ago to now):** same schema, but the producer switched to `NoTranslation` (native OTel names):
```
test{http.response.status_code="200", instance="myapp:8080"}
test{http.response.status_code="404", instance="myapp:8080"}
```

### The Problem (Phase 4)

Querying `test_counter_bytes_total`, `test_bytes_total`, or `test` each returns `[PARTIAL]` — only its own era. No single raw name shows the complete picture.

### The OTLP-strategy solution (Phase 5)

Querying `test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}` returns `[FULL]` across the strategy migration: it covers both 1.1.0 eras (escaped and native) under one unified OTel name. The older semconv 1.0.0 era is still missing — that is what `__schema_url__` adds in Phase 8.

### Aggregation and rate (Phases 6 & 7)

Both `sum()` and `rate()` work seamlessly across the naming boundary, producing continuous results.

### The schema-version solution (Phase 8)

Adding `__schema_url__="registry/registry.yaml"` makes Prometheus also walk the OTel schema's `versions` section, recovering the historical `test.counter` name from semconv 1.0.0. The query `test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml", __otlp_strategy__="NoTranslation"}` then surfaces all three eras under the canonical OTel name.

## Command-Line Flags

| Flag | Description |
|------|-------------|
| `--data-dir=PATH` | Save TSDB to specified directory (default: temp, deleted on exit) |
| `--populate-only` | Only populate data, skip query phase (for use with browser demo) |

## How It Works

This requires the experimental feature flag `--enable-feature=semconv-versioned-read`.

1. The `__semconv_url__` parameter points to a semantic conventions file in the embedded registry.
2. Prometheus loads the metric definition (name, unit, type, attributes).
3. For each query that carries `__semconv_url__`, `__otlp_strategy__`, and a `__name__=…` matcher, Prometheus reverse-resolves the (possibly already-translated) name to the canonical metric, then generates variants for every enabled OTLP translation strategy plus the canonical OTel name (identity). For the demo's `test` metric (counter, unit `By`) the strategies produce:
   - Identity / `NoTranslation` → `test{http.response.status_code=…}`
   - `UnderscoreEscapingWithSuffixes` → `test_bytes_total{http_response_status_code=…}`
   - `UnderscoreEscapingWithoutSuffixes` → `test{http_response_status_code=…}`
   - `NoUTF8EscapingWithSuffixes` → `test_bytes_total{http.response.status_code=…}`

   After dedup the underlying storage is queried for `test` and `test_bytes_total`, finding both the post- and pre-migration series.
4. Results from all variants are merged and rendered in the dialect named by `__otlp_strategy__` (e.g. `NoTranslation` yields canonical `http.response.status_code`; `UnderscoreEscapingWithSuffixes` yields `test_bytes_total` / `http_response_status_code`).

Queries without a fan-out trigger (`__otlp_strategy__` or `__schema_url__`), without `__semconv_url__`, or without a `__name__` matcher pass through to the underlying storage unchanged; there is no overhead.

A few additional details worth knowing:

- **Strategy selects the dialect, not the probed set.** `__otlp_strategy__` chooses how results are named — and lets you query a name already written in that dialect — but every enabled strategy is still probed on every qualifying query; there is no matcher to restrict the probed list.
- **Unknown metrics still fan out.** If the `__name__` matcher names a metric that is not in the referenced semconv file, the strategies still run but without unit/type suffixes (the wrapper has no metadata to apply), so variants differ only in escape style.
- **Parallel execution.** Variants are queried in parallel via `errgroup` with a concurrency limit of 16 — enough for the realistic cross-product but bounded so concurrent PromQL evaluations don't spawn unbounded goroutines.
- **Partial failures aggregate.** If one variant's underlying query errors, the wrapper joins per-variant errors via `errors.Join` and still returns the surviving series. Callers see the partial result plus the aggregated error.

### Cross-version fan-out (optional)

`__schema_url__` is an independent fan-out axis from `__otlp_strategy__`: it triggers version-rename fan-out on its own (producing raw OTel historical names), and combines with `__otlp_strategy__` to also translate those names into a dialect. Prometheus walks the OTel schema's `versions` section to recover the metric's historical and future names (for example, the embedded registry has `test.counter` in 1.0.0 and `test.v2` in 1.2.0 alongside `test` in 1.1.0). When `__otlp_strategy__` is set, each variant from that walk then goes through the OTLP-strategy fan-out using its **own** version's `unit` and `instrument` metadata — the engine loads each version's semconv file from the embedded registry so escapings reflect the right shape per historical name.

The demo's Phase 8 exercises this end-to-end: it writes a third era under the historical name `test_counter_bytes_total` (semconv 1.0.0, escaped) and then runs two queries side-by-side. The query with `__schema_url__` recovers the extra hour at the start; the one without it cannot.

## Key Benefits

- **Dashboard continuity**: Existing dashboards keep working after migration
- **No data gaps**: All historical and new data accessible with one query
- **Aggregation support**: `sum()`, `avg()`, etc. work across naming boundaries
- **Rate continuity**: `rate()` and `increase()` produce continuous results
- **Canonical naming**: Results use standard OTel semantic convention names

## OTLP Translation Strategies

| Strategy | Description | Metric Example | Label Example |
|----------|-------------|----------------|---------------|
| `UnderscoreEscapingWithSuffixes` | Traditional Prometheus naming with unit/type suffixes | `test_bytes_total` | `http_response_status_code` |
| `UnderscoreEscapingWithoutSuffixes` | Underscores without unit/type suffixes | `test` | `http_response_status_code` |
| `NoUTF8EscapingWithSuffixes` | Dots in names preserved, adds unit/type suffixes | `test_bytes_total` | `http.response.status_code` |
| `NoTranslation` | Pure OTel naming (requires UTF-8 support) | `test` | `http.response.status_code` |
