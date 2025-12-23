# info() Autocomplete Demo

This demo starts Prometheus with the web UI to interactively test autocomplete
for the `info()` function with OTel resource attributes.

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

You should see suggestions for resource attribute names:
- `service_name`
- `service_namespace`
- `service_instance_id`
- `host_name`
- `cloud_region`
- `cloud_provider`
- `db_system`

### 2. Label Value Autocomplete

Type:
```
info(http_requests_total, {service_name="
```

You should see suggestions for values:
- `api-gateway`
- `database`

### 3. Full Query Execution

Execute a complete query:
```
info(http_requests_total)
```

This returns metrics enriched with their resource attributes.

## Test Data

The demo sends metrics from 3 service instances:

| Service | Namespace | Metrics |
|---------|-----------|---------|
| api-gateway | production | http_requests_total |
| api-gateway | staging | http_requests_total |
| database | production | db_queries_total |

### Resource Attributes

| Attribute | Values |
|-----------|--------|
| service_name | api-gateway, database |
| service_namespace | production, staging |
| host_name | prod-gateway-1.example.com, prod-db-1.example.com, staging-gateway-1.example.com |
| cloud_region | us-west-2, us-east-1 |
| cloud_provider | aws |
| db_system | postgresql (database only) |

## Architecture

```
OTLP Metrics ──► OTLP Handler ──► TSDB ◄── Web Handler ──► Browser UI
                (port 9091)                (port 9090)
```

The demo runs two HTTP servers:
- **Port 9090**: Prometheus web UI and API
- **Port 9091**: OTLP receiver with resource attribute persistence enabled
