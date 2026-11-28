---
name: prometheus-promql
description: >-
  Write, review and debug PromQL: selectors and matchers, range vectors and
  rate/increase, aggregation with by/without, vector matching, histograms
  (classic buckets and native), subqueries, staleness and lookback, and keeping
  queries cheap. Also run queries from the shell with promtool query. Use when
  building dashboards, alerts or ad hoc investigations against Prometheus.
---

# PromQL

References: `docs/querying/basics.md`, `docs/querying/operators.md`,
`docs/querying/functions.md`, `docs/querying/examples.md`.

## Building blocks

```promql
http_requests_total{job="api", code=~"5.."}           # instant vector
http_requests_total{job="api"}[5m]                     # range vector
sum by (code) (rate(http_requests_total[5m]))          # per-second rate, aggregated
max_over_time(up[1h:1m])                               # subquery
```

- Matchers: `=`, `!=`, `=~`, `!~` (regexes are fully anchored).
- `rate()` for per-second averages of counters, `irate()` for spiky
  last-two-sample rates, `increase()` for totals over the window. Apply them
  **before** aggregating: `sum(rate(x[5m]))`, never `rate(sum(x)[5m])`.
- Use a range at least 4x the scrape interval for `rate`.
- `by (...)` keeps listed labels; `without (...)` drops them.

## Vector matching

```promql
rate(errors_total[5m]) / on (job, instance) rate(requests_total[5m])
sum by (pod) (x) * on (pod) group_left (node) kube_pod_info
```

Use `on`/`ignoring` to choose match labels and `group_left`/`group_right`
for many-to-one joins.

## Histograms

```promql
# classic
histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))
# native
histogram_quantile(0.95, sum(rate(http_request_duration_seconds[5m])))
histogram_avg(rate(http_request_duration_seconds[5m]))
histogram_count(rate(http_request_duration_seconds[5m]))
```

Keep `le` in the `by` clause for classic histograms.

## Staleness and lookback

Instant queries take the newest sample within the lookback period (5m by
default, `--query.lookback-delta` or per-query `lookback_delta`). Series
stop appearing once marked stale (target gone or no longer exported).

## Keep queries cheap

- Start in the table view; aim for hundreds of series, not thousands,
  before graphing.
- A bare metric name can expand to thousands of series; filter first.
- Pre-compute expensive dashboard queries with recording rules (see
  `prometheus-rules`).

## From the shell

```bash
promtool query instant http://localhost:9090 'up == 0'
promtool query range --start=2026-11-27T00:00:00Z --end=2026-11-28T00:00:00Z \
  --step=5m http://localhost:9090 'sum(rate(http_requests_total[5m]))'
promtool query series --match='up' http://localhost:9090
promtool query labels http://localhost:9090 job
```

Or the HTTP API: `GET /api/v1/query?query=...` and `/api/v1/query_range`
(`docs/querying/api.md`).
