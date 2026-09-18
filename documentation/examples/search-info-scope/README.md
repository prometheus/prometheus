# Info scope on existing search endpoints: minimal PoC

This prototype explores extending `/api/v1/search/label_names` and
`/api/v1/search/label_values` with `scope=info`, instead of adding two new
info-specific endpoints. It implements the API experiment discussed in
[prometheus/proposals#85](https://github.com/prometheus/proposals/pull/85).
There are no UI changes or new HTTP routes.

Start this checkout with a Prometheus configuration that scrapes base metrics
and their corresponding `target_info` series:

```sh
go run ./cmd/prometheus --config.file=prometheus.yml \
  --enable-feature=search-api,promql-experimental-functions
```

Check `/api/v1/features`: `data.api.search_scope_info` must be `true` before
sending scoped requests. Older servers can silently ignore unknown parameters.

Find data-label names on info series associated with a base expression:

```sh
curl --no-buffer --get http://localhost:9090/api/v1/search/label_names \
  --data-urlencode 'scope=info' \
  --data-urlencode 'expr=rate(http_requests_total[5m])' \
  --data-urlencode 'data_match[]=env="prod"'
```

Then find values of one exact label:

```sh
curl --no-buffer --get http://localhost:9090/api/v1/search/label_values \
  --data-urlencode 'scope=info' \
  --data-urlencode 'expr=rate(http_requests_total[5m])' \
  --data-urlencode 'data_match[]=env="prod"' \
  --data-urlencode 'label=version'
```

Both operations also accept form-encoded POST. Existing `search[]`, sorting,
scoring, `limit`, `batch_size`, and NDJSON response handling are reused. A name
result has `name`; a value result has `value`. Consume the success trailer and
preserve warnings and `has_more`.

`data_match[]` accepts up to 32 individual full PromQL matchers. All are ANDed;
`__name__` selects info families, defaulting to `target_info`. For example,
`data_match[]=__name__="build_info"` selects another family. In info mode,
`match[]` is rejected to avoid competing scopes. Omit the matcher being edited.
Identifying labels (`job`, `instance`, `__name__`) are excluded before limiting
name results and rejected as the selected value label.

With `expr`, the server evaluates one instant vector at `time` (default `end`,
default now) and reuses the evaluator's identifying-label and temporal rules.
`lookback_delta`, selector offsets, and `@` are honored. `start` is parsed but
ignored for expression-derived selection. An empty vector or a vector without
identifying labels returns no candidates, never an unscoped search. Without
`expr`, the ordinary search time window is used.

Without `scope`, ordinary search keeps its existing behavior. The new `expr`
and `data_match[]` parameters require `scope=info`. Unknown scopes and info scope
on metric-name search are rejected.

## Deliberate prototype limits

- Scoped requests have a fixed 30-second total deadline. A positive `timeout`
  can shorten it. The expression still uses the engine's query limits. Production
  integration should share the configured query timeout across both phases.
- Expression scope preparation rejects more than 10,000 output samples or 1 MiB
  of raw identifying-label bytes. These conservative bounds are simpler than
  PROM-85's unique-value and escaped-regexp-byte accounting.
- Storage capability and partial-backend behavior are inherited from existing
  search. This does not implement PROM-85's composite-storage changes.
- Candidates can include conservative job/instance cross-pairs and indexed
  series without a sample at the selected time. Execute the completed query to
  validate actual results.
- Each request evaluates its expression independently. There is no shared scope
  cache, pagination, frontend integration, or generated OpenAPI update.
- These search routes now execute queries when scoped. Deployments authorizing
  only by URL path must protect them with query-level authorization.

The focused integration test uses real TSDB storage and the query engine:

```sh
go test ./web/api/v1 -run TestSearchInfoScope -count=1
```
