<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_notifications_alertmanagers_discovered` | gauge | {alertmanager} | The number of alertmanagers discovered and active. |
| `prometheus_notifications_dropped_total` | counter | {notification} | Total number of alerts dropped due to errors when sending to Alertmanager. |
| `prometheus_notifications_queue_capacity` | gauge | {notification} | The capacity of the alert notifications queue. |
| `prometheus_notifications_queue_length` | gauge | {notification} | The number of alert notifications in the queue. |


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
