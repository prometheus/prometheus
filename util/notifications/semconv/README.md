<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_api_notification_active_subscribers` | gauge | {subscriber} | The current number of active notification subscribers. |
| `prometheus_api_notification_updates_dropped_total` | counter | {update} | Total number of notification updates dropped. |
| `prometheus_api_notification_updates_sent_total` | counter | {update} | Total number of notification updates sent. |


## Metric Details


### `prometheus_api_notification_active_subscribers`

The current number of active notification subscribers.

- **Type:** gauge
- **Unit:** {subscriber}
- **Stability:** development


### `prometheus_api_notification_updates_dropped_total`

Total number of notification updates dropped.

- **Type:** counter
- **Unit:** {update}
- **Stability:** development


### `prometheus_api_notification_updates_sent_total`

Total number of notification updates sent.

- **Type:** counter
- **Unit:** {update}
- **Stability:** development
