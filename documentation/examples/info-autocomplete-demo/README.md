# info() Autocomplete Demo

This demo starts Prometheus with the web UI to interactively test autocomplete
for the `info()` function using `target_info` metrics.

## Running the Demo

```bash
go run ./documentation/examples/info-autocomplete-demo
```

Then open http://localhost:9090 in your browser.

## What to Test

### 1. Label Name Autocomplete

In the query editor, type:
```
info(http_requests_total, {
```

You should see suggestions for data labels from `target_info`:
- `version`
- `env`
- `cluster`
- `region`

### 2. Label Value Autocomplete

Type:
```
info(http_requests_total, {version="
```

You should see suggestions for values:
- `v1.0`
- `v2.0`
- `v2.1`

### 3. Full Query Execution

Execute a complete query:
```
info(http_requests_total)
```

This returns metrics enriched with their `target_info` labels.

### 4. API Endpoint

Test the new `/api/v1/info_labels` endpoint:

```bash
# Get all data labels from target_info
curl 'http://localhost:9090/api/v1/info_labels'

# Filter by expression results
curl 'http://localhost:9090/api/v1/info_labels?expr=http_requests_total{job="api-gateway"}'

# Use a different info metric
curl 'http://localhost:9090/api/v1/info_labels?metric_match=build_info'
```

## Test Data

The demo creates metrics from 3 service instances:

| Job | Instance | Info Labels |
|-----|----------|-------------|
| api-gateway | prod-gateway-1:8080 | version=v1.0, env=prod, cluster=us-east |
| api-gateway | staging-gateway-1:8080 | version=v2.0, env=staging, cluster=us-west |
| database | prod-db-1:5432 | version=v2.1, env=prod, region=us-east |

### Available Metrics

| Metric | Labels |
|--------|--------|
| http_requests_total | job, instance, method, status |
| db_queries_total | job, instance, operation |
| target_info | job, instance, version, env, cluster/region |
| build_info | job, instance, go_version, revision |

### Data Labels (from target_info)

| Label | Values |
|-------|--------|
| version | v1.0, v2.0, v2.1 |
| env | prod, staging |
| cluster | us-east, us-west (api-gateway only) |
| region | us-east (database only) |

## Architecture

```
Test Metrics ──► TSDB ◄── Web Handler ──► Browser UI
   (direct)       │       (port 9090)
                  │
                  └──► /api/v1/info_labels endpoint
```

The demo directly appends metrics to the TSDB Head using the `Appender` interface,
simulating what would happen after scraping targets with `target_info` metrics.
