<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_notifications_alertmanagers_discovered` | gauge | {alertmanager} | The number of alertmanagers discovered and active. |
| `prometheus_notifications_dropped_total` | counter | {notification} | Total number of alerts dropped due to errors when sending to Alertmanager. |
| `prometheus_notifications_errors_total` | counter | {notification} | Total number of sent alerts affected by errors. |
| `prometheus_notifications_latency_histogram_seconds` | histogram | s | Latency histogram for sending alert notifications. |
| `prometheus_notifications_latency_seconds` | histogram | s | Latency quantiles for sending alert notifications. |
| `prometheus_notifications_queue_capacity` | gauge | {notification} | The capacity of the alert notifications queue. |
| `prometheus_notifications_queue_length` | gauge | {notification} | The number of alert notifications in the queue. |
| `prometheus_notifications_sent_total` | counter | {notification} | Total number of alerts sent. |


## Metric Details


### `prometheus_notifications_alertmanagers_discovered`

The number of alertmanagers discovered and active.

- **Type:** gauge
- **Unit:** {alertmanager}
- **Stability:** development


### `prometheus_notifications_dropped_total`

Total number of alerts dropped due to errors when sending to Alertmanager.

- **Type:** counter
- **Unit:** {notification}
- **Stability:** development


### `prometheus_notifications_errors_total`

Total number of sent alerts affected by errors.

- **Type:** counter
- **Unit:** {notification}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `alertmanager` | string | The alertmanager instance URL. | http://alertmanager:9093/api/v2/alerts |



### `prometheus_notifications_latency_histogram_seconds`

Latency histogram for sending alert notifications.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `alertmanager` | string | The alertmanager instance URL. | http://alertmanager:9093/api/v2/alerts |



### `prometheus_notifications_latency_seconds`

Latency quantiles for sending alert notifications.

- **Type:** histogram
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `alertmanager` | string | The alertmanager instance URL. | http://alertmanager:9093/api/v2/alerts |



### `prometheus_notifications_queue_capacity`

The capacity of the alert notifications queue.

- **Type:** gauge
- **Unit:** {notification}
- **Stability:** development


### `prometheus_notifications_queue_length`

The number of alert notifications in the queue.

- **Type:** gauge
- **Unit:** {notification}
- **Stability:** development


### `prometheus_notifications_sent_total`

Total number of alerts sent.

- **Type:** counter
- **Unit:** {notification}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `alertmanager` | string | The alertmanager instance URL. | http://alertmanager:9093/api/v2/alerts |

