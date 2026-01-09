# Semantic Conventions Translation Demo

This self-contained demo showcases Prometheus's ability to query metrics using OpenTelemetry semantic conventions, automatically matching metrics that were written with different OTLP translation strategies.

## The Problem

When OTLP metrics are written to Prometheus, different translation strategies produce different metric and label names:

| Strategy | Metric Name | Label Name |
|----------|-------------|------------|
| `UnderscoreEscapingWithSuffixes` | `test_bytes_total` | `http_response_status_code` |
| `NoTranslation` | `test` | `http.response.status_code` |

If your infrastructure changes translation strategies over time (e.g., migrating to UTF-8 support), or you have different producers using different strategies, you end up with the same logical metric stored under different names. Traditional queries would miss data from one strategy or the other.

## The Solution

Using the `__semconv_url__` selector, Prometheus automatically generates query variants for all OTLP translation strategies, merging results into a unified series with consistent naming.

## Running the Demo

### Option 1: Terminal Demo (Quick)

```bash
go run ./documentation/examples/semconv-translation
```

This runs three phases automatically and prints results to the terminal.

### Option 2: Browser Demo with Prometheus UI

```bash
cd documentation/examples/semconv-translation
./demo.sh
```

This script:
1. Populates a TSDB with sample data (2 hours of history)
2. Launches Prometheus serving that data
3. Opens your browser to the Prometheus UI with the demo query

You can then try these queries in the UI:
- `test_bytes_total` - Shows only escaped metrics (old naming, 2h-1h ago)
- `test` - Shows only native metrics (new naming, 1h ago-now)
- `test{__semconv_url__="registry/1.1.0"}` - Shows **BOTH** merged!

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

## Demo Phases

### Phase 1: Write metrics with UnderscoreEscapingWithSuffixes

Simulates an OTLP producer using traditional Prometheus naming (2 hours ago to 1 hour ago):
```
test_bytes_total{http_response_status_code="200", instance="producer-escaped", tenant="alice"}
```

### Phase 2: Write metrics with NoTranslation

Simulates an OTLP producer using native OTel UTF-8 naming (1 hour ago to now):
```
test{http.response.status_code="200", instance="producer-native", tenant="alice"}
```

### Phase 3: Query with __semconv_url__

Uses the `__semconv_url__` selector to query both naming conventions:
```promql
test{__semconv_url__="registry/1.1.0"}
```

This query automatically matches both:
- `test_bytes_total{http_response_status_code="200"}` (from Phase 1)
- `test{http.response.status_code="200"}` (from Phase 2)

The results are merged and returned with consistent OTel semantic naming.

## Command-Line Flags

| Flag | Description |
|------|-------------|
| `--data-dir=PATH` | Save TSDB to specified directory (default: temp, deleted on exit) |
| `--populate-only` | Only populate data, skip query phase (for use with browser demo) |

## OTLP Translation Strategies

Prometheus supports these OTLP translation strategies:

| Strategy | Description | Metric Example | Label Example |
|----------|-------------|----------------|---------------|
| `UnderscoreEscapingWithSuffixes` | Traditional Prometheus naming with unit/type suffixes | `test_bytes_total` | `http_response_status_code` |
| `UnderscoreEscapingWithoutSuffixes` | Underscores without unit/type suffixes | `test_bytes` | `http_response_status_code` |
| `NoUTF8EscapingWithSuffixes` | Dots in labels preserved, adds suffixes | `test_total` | `http.response.status_code` |
| `NoTranslation` | Pure OTel naming (requires UTF-8 support) | `test` | `http.response.status_code` |

When querying with `__semconv_url__`, Prometheus generates variants for all strategies to ensure complete data retrieval regardless of how the data was originally written.

## How It Works

1. The demo creates a TSDB instance (temp or specified directory)
2. Writes metrics at different timestamps using different naming conventions
3. When querying with `__semconv_url__`:
   - Prometheus loads semantic conventions from the embedded registry
   - Generates matcher variants for all OTLP translation strategies
   - Merges results with canonical OTel naming
