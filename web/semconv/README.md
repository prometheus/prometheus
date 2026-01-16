<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_http_request_duration_seconds` | histogram | s | Histogram of latencies for HTTP requests. |
| `prometheus_http_requests_total` | counter | {request} | Counter of HTTP requests. |
| `prometheus_http_response_size_bytes` | histogram | By | Histogram of response size for HTTP requests. |
| `prometheus_ready` | gauge | 1 | Whether Prometheus startup was fully completed and the server is ready for normal operation. |
| `prometheus_web_federation_errors_total` | counter | {error} | Total number of errors that occurred while sending federation responses. |
| `prometheus_web_federation_warnings_total` | counter | {warning} | Total number of warnings that occurred while sending federation responses. |


## Metric Details


### `prometheus_http_request_duration_seconds`

Histogram of latencies for HTTP requests.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `handler` | string | The HTTP handler path. | /, /-/healthy, /-/ready, /api/v1/query |



### `prometheus_http_requests_total`

Counter of HTTP requests.

- **Type:** counter
- **Unit:** {request}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `code` | string | The HTTP response status code. | 200, 400, 404, 500 |
| `handler` | string | The HTTP handler path. | /, /-/healthy, /-/ready, /api/v1/query |



### `prometheus_http_response_size_bytes`

Histogram of response size for HTTP requests.

- **Type:** histogram
- **Unit:** By
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `handler` | string | The HTTP handler path. | /, /-/healthy, /-/ready, /api/v1/query |



### `prometheus_ready`

Whether Prometheus startup was fully completed and the server is ready for normal operation.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_web_federation_errors_total`

Total number of errors that occurred while sending federation responses.

- **Type:** counter
- **Unit:** {error}
- **Stability:** development


### `prometheus_web_federation_warnings_total`

Total number of warnings that occurred while sending federation responses.

- **Type:** counter
- **Unit:** {warning}
- **Stability:** development
