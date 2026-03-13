# Semantic Conventions Translation Demo

This demo showcases Prometheus's ability to maintain query continuity when migrating between OTLP translation strategies. It simulates a real-world scenario where a producer changes from `UnderscoreEscapingWithSuffixes` to `NoTranslation` (native OTel naming).

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

## The Solution

Using the `__semconv_url__` selector, Prometheus automatically generates query variants for all OTLP translation strategies, providing seamless data continuity across the migration:

```promql
test{__semconv_url__="registry/1.1.0"}
```

This single query matches both naming conventions and returns all data with unified OTel semantic convention names.

## Running the Demo

### Option 1: Terminal Demo (Quick)

```bash
go run ./documentation/examples/semconv-translation
```

This runs six phases and demonstrates:
1. **Before migration**: Writing metrics with `UnderscoreEscapingWithSuffixes`
2. **After migration**: Writing metrics with `NoTranslation`
3. **The problem**: Showing how queries only return partial data
4. **The solution**: Using `__semconv_url__` for full data coverage
5. **Aggregation**: `sum()` works across the migration boundary
6. **Rate calculation**: `rate()` works across the migration boundary

### Option 2: Browser Demo with Prometheus UI

```bash
cd documentation/examples/semconv-translation
./demo.sh
```

This script:
1. Populates a TSDB with sample data (2 hours of history)
2. Launches Prometheus serving that data
3. Opens your browser to the Prometheus UI with the demo query

Then try these queries in the UI:

**The Problem - Queries break after migration:**
- `test_bytes_total` — Only shows old data (before migration)
- `test` — Only shows new data (after migration)

**The Solution - Seamless query continuity:**
- `test{__semconv_url__="registry/1.1.0"}` — Shows ALL data with unified naming
- `sum(test{__semconv_url__="registry/1.1.0"})` — Aggregation works across boundary
- `rate(test{__semconv_url__="registry/1.1.0"}[5m])` — Rate calculation works too

### Option 3: Manual Browser Demo

```bash
# Populate the TSDB
go run ./documentation/examples/semconv-translation \
  --data-dir=./semconv-demo-data \
  --populate-only

# Launch Prometheus with semconv feature enabled
./prometheus \
  --storage.tsdb.path=./semconv-demo-data \
  --config.file=/dev/null \
  --enable-feature=semconv-versioned-read

# Open http://localhost:9090 in your browser
```

## Demo Scenario

The demo simulates a single producer (`myapp:8080`) that changes OTLP translation strategy:

### Phase 1 & 2: Data Population

**Before migration (2 hours ago to 1 hour ago):**
```
test_bytes_total{http_response_status_code="200", instance="myapp:8080"}
test_bytes_total{http_response_status_code="404", instance="myapp:8080"}
```

**After migration (1 hour ago to now):**
```
test{http.response.status_code="200", instance="myapp:8080"}
test{http.response.status_code="404", instance="myapp:8080"}
```

### Phase 3: The Problem

Querying `test_bytes_total` returns `[PARTIAL]` — only pre-migration data.
Querying `test` returns `[PARTIAL]` — only post-migration data.

Neither query alone shows the complete picture!

### Phase 4: The Solution

Querying `test{__semconv_url__="registry/1.1.0"}` returns `[FULL]` — all data spanning the migration point, with unified OTel naming.

### Phase 5 & 6: Aggregation and Rate

Both `sum()` and `rate()` calculations work seamlessly across the naming boundary, producing continuous results.

## Command-Line Flags

| Flag | Description |
|------|-------------|
| `--data-dir=PATH` | Save TSDB to specified directory (default: temp, deleted on exit) |
| `--populate-only` | Only populate data, skip query phase (for use with browser demo) |

## How It Works

1. The `__semconv_url__` parameter points to a semantic conventions file in the embedded registry
2. Prometheus loads the metric definition (name, unit, type, attributes)
3. For each query, Prometheus generates variants for all OTLP translation strategies:
   - `UnderscoreEscapingWithSuffixes` → `test_bytes_total{http_response_status_code=...}`
   - `UnderscoreEscapingWithoutSuffixes` → `test_bytes{http_response_status_code=...}`
   - `NoUTF8EscapingWithSuffixes` → `test_total{http.response.status_code=...}`
   - `NoTranslation` → `test{http.response.status_code=...}`
4. Results from all variants are merged with canonical OTel naming

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
| `UnderscoreEscapingWithoutSuffixes` | Underscores without unit/type suffixes | `test_bytes` | `http_response_status_code` |
| `NoUTF8EscapingWithSuffixes` | Dots in labels preserved, adds suffixes | `test_total` | `http.response.status_code` |
| `NoTranslation` | Pure OTel naming (requires UTF-8 support) | `test` | `http.response.status_code` |
