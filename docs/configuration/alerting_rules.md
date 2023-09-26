---
title: Alerting rules
sort_rank: 3
---

# Alerting rules

Alerting rules allow you to define alert conditions based on Prometheus
expression language expressions and to send notifications about firing alerts
to an external service. Whenever the alert expression results in one or more
vector elements at a given point in time, the alert counts as active for these
elements' label sets.

### Defining alerting rules

Alerting rules are configured in Prometheus in the same way as [recording
rules](recording_rules.md).

An example rules file with an alert would be:

```yaml
groups:
- name: example
  rules:
  - alert: HighRequestLatency
    expr: job:request_latency_seconds:mean5m{job="myjob"} > 0.5
    for: 10m
    labels:
      severity: page
    annotations:
      summary: High request latency
```

The optional `for` clause causes Prometheus to wait for a certain duration
between first encountering a new expression output vector element and counting
an alert as firing for this element. In this case, Prometheus will check that
the alert continues to be active during each evaluation for 10 minutes before
firing the alert. Elements that are active, but not firing yet, are in the pending state.
Alerting rules without the `for` clause will become active on the first evaluation.

The `labels` clause allows specifying a set of additional labels to be attached
to the alert. Any existing conflicting labels will be overwritten. The label
values can be templated.

The `annotations` clause specifies a set of informational labels that can be used to store longer additional information such as alert descriptions or runbook links. The annotation values can be templated.

#### Templating

Label and annotation values can be templated using [console
templates](https://prometheus.io/docs/visualization/consoles).  The `$labels`
variable holds the label key/value pairs of an alert instance. The configured
external labels can be accessed via the `$externalLabels` variable. The
`$value` variable holds the evaluated value of an alert instance.

    # To insert a firing element's label values:
    {{ $labels.<labelname> }}
    # To insert the numeric expression value of the firing element:
    {{ $value }}

Examples:

```yaml
groups:
- name: example
  rules:

  # Alert for any instance that is unreachable for >5 minutes.
  - alert: InstanceDown
    expr: up == 0
    for: 5m
    labels:
      severity: page
    annotations:
      summary: "Instance {{ $labels.instance }} down"
      description: "{{ $labels.instance }} of job {{ $labels.job }} has been down for more than 5 minutes."

  # Alert for any instance that has a median request latency >1s.
  - alert: APIHighRequestLatency
    expr: api_http_request_latencies_second{quantile="0.5"} > 1
    for: 10m
    annotations:
      summary: "High request latency on {{ $labels.instance }}"
      description: "{{ $labels.instance }} has a median request latency above 1s (current value: {{ $value }}s)"
```

### Inspecting alerts during runtime

To manually inspect which alerts are active (pending or firing), navigate to
the "Alerts" tab of your Prometheus instance. This will show you the exact
label sets for which each defined alert is currently active.

For pending and firing alerts, Prometheus also stores synthetic time series of
the form `ALERTS{alertname="<alert name>", alertstate="<pending or firing>", <additional alert labels>}`.
The sample value is set to `1` as long as the alert is in the indicated active
(pending or firing) state, and the series is marked stale when this is no
longer the case.

### Sending alert notifications

Prometheus's alerting rules are good at figuring what is broken *right now*, but
they are not a fully-fledged notification solution. Another layer is needed to
add summarization, notification rate limiting, silencing and alert dependencies
on top of the simple alert definitions. In Prometheus's ecosystem, the
[Alertmanager](https://prometheus.io/docs/alerting/alertmanager/) takes on this
role. Thus, Prometheus may be configured to periodically send information about
alert states to an Alertmanager instance, which then takes care of dispatching
the right notifications.  
Prometheus can be [configured](configuration.md) to automatically discover available
Alertmanager instances through its service discovery integrations.
