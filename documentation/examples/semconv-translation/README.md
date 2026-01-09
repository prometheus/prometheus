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

```bash
go run ./documentation/examples/semconv_translation
```

The demo runs three phases automatically:

### Phase 1: Write metrics with UnderscoreEscapingWithSuffixes

Simulates an OTLP producer using traditional Prometheus naming:
```
test_bytes_total{http_response_status_code="200", tenant="alice"}
```

### Phase 2: Write metrics with NoTranslation

Simulates an OTLP producer using native OTel UTF-8 naming:
```
test{http.response.status_code="200", tenant="alice"}
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

## Expected Output

```
=== OTel Semantic Conventions Translation Demo ===

TSDB data directory: /tmp/semconv-demo-xxxxx

--- Phase 1: Writing metrics with UnderscoreEscapingWithSuffixes strategy ---

  [Written] {__name__="test_bytes_total", http_response_status_code="200", ...} 1500
  [Written] {__name__="test_bytes_total", http_response_status_code="404", ...} 250

--- Phase 2: Writing metrics with NoTranslation strategy ---

  [Written] {__name__="test", http.response.status_code="200", ...} 2300
  [Written] {__name__="test", http.response.status_code="500", ...} 75

--- Phase 3: Querying with __semconv_url__ for unified view ---

Query: test{__semconv_url__="registry/1.1.0"}

Results:
{__name__="test", http.response.status_code="200", instance="producer-escaped", ...} => 1500
{__name__="test", http.response.status_code="200", instance="producer-native", ...} => 2300
{__name__="test", http.response.status_code="404", instance="producer-escaped", ...} => 250
{__name__="test", http.response.status_code="500", instance="producer-native", ...} => 75

--- Summary ---
Both naming conventions merged into canonical OTel names.
```

## OTLP Translation Strategies

Prometheus supports these OTLP translation strategies:

| Strategy | Description | Example |
|----------|-------------|---------|
| `UnderscoreEscapingWithSuffixes` | Traditional Prometheus naming with unit/type suffixes | `test_bytes_total` |
| `UnderscoreEscapingWithoutSuffixes` | Underscores without unit/type suffixes | `test_bytes` |
| `NoUTF8EscapingWithSuffixes` | Preserves dots, adds suffixes | `test_total` |
| `NoTranslation` | Pure OTel naming (requires UTF-8 support) | `test` |

When querying with `__semconv_url__`, Prometheus generates variants for all strategies to ensure complete data retrieval regardless of how the data was originally written.

## How It Works

1. The demo creates a temporary TSDB instance
2. Writes metrics using two different naming conventions (simulating different OTLP producers)
3. Wraps the storage with `semconv.AwareStorage()` for semconv-aware querying
4. Queries using `__semconv_url__` which triggers:
   - Loading semantic conventions from the embedded registry
   - Generating matcher variants for all OTLP translation strategies
   - Merging results with canonical OTel naming
