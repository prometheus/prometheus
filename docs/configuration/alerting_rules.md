---
title: Alerting rules
sort_rank: 3
---

Alerting rules allow you to define alert conditions based on Prometheus
expression language expressions and to send notifications about firing alerts
to an external service. Whenever the alert expression results in one or more
vector elements at a given point in time, the alert counts as active for these
elements' label sets.

## Defining alerting rules

Alerting rules are configured in Prometheus in the same way as [recording
rules](recording_rules.md).

An example rules file with an alert would be:

```yaml
groups:
- name: example
  labels:
    team: myteam
  rules:
  - alert: HighRequestLatency
    expr: job:request_latency_seconds:mean5m{job="myjob"} > 0.5
    for: 10m
    keep_firing_for: 5m
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

There is also an optional `keep_firing_for` clause that tells Prometheus to keep
this alert firing for the specified duration after the firing condition was last met.
This can be used to prevent situations such as flapping alerts, false resolutions
due to lack of data loss, etc. Alerting rules without the `keep_firing_for` clause
will deactivate on the first evaluation where the condition is not met (assuming
any optional `for` duration described above has been satisfied).

The `labels` clause allows specifying a set of additional labels to be attached
to the alert. Any existing conflicting labels will be overwritten. The label
values can be templated.

The `annotations` clause specifies a set of informational labels that can be used to store longer additional information such as alert descriptions or runbook links. The annotation values can be templated.

### Templating

Label and annotation values can be templated using Go templates, see the
[template reference](template_reference.md) for the available functions and the
[template examples](template_examples.md) for more ideas. The `$labels`
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

### Querying additional data in annotations

The alert expression often doesn't carry everything you'd like to see in a
notification. The `query` template function runs a PromQL expression every time
the rule is evaluated, so you can pull extra context into labels and
annotations. It returns a list of samples, which you can feed through `first`,
`value`, `label` and the humanizing functions described in the [template
reference](template_reference.md).

Wrap the query in `with` so nothing is rendered when it returns no results,
since `first` fails on an empty result. To reuse labels of the firing alert in
the query, build the query string with `printf`. Use single quotes for label
matchers inside the query so they don't clash with the YAML and template
quoting.

```yaml
groups:
- name: example
  rules:

  # Alert when more than 5% of requests of a job fail, and show the
  # job's total request rate next to the error ratio.
  - alert: HighErrorRatio
    expr: |
      sum by (job) (rate(http_requests_total{code=~"5.."}[5m]))
        /
      sum by (job) (rate(http_requests_total[5m]))
        > 0.05
    for: 10m
    labels:
      severity: page
    annotations:
      summary: "High error ratio for {{ $labels.job }}"
      description: |
        {{ $labels.job }} is failing {{ $value | humanizePercentage }} of its requests.
        {{ with printf "sum(rate(http_requests_total{job='%s'}[5m]))" $labels.job | query }}
        Total request rate is {{ . | first | value | humanize }} requests per second.
        {{ end }}

  # Alert when a filesystem is almost full, and include its total size,
  # which isn't part of the alert expression.
  - alert: FilesystemAlmostFull
    expr: |
      node_filesystem_avail_bytes{fstype!="tmpfs"}
        / node_filesystem_size_bytes{fstype!="tmpfs"}
        < 0.1
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "{{ $labels.mountpoint }} on {{ $labels.instance }} is almost full"
      description: |
        Only {{ $value | humanizePercentage }} of {{ $labels.mountpoint }} is left.
        {{ with printf "node_filesystem_size_bytes{instance='%s',mountpoint='%s'}" $labels.instance $labels.mountpoint | query }}
        The filesystem is {{ . | first | value | humanize1024 }}B in total.
        {{ end }}
```

The queries run against the local Prometheus on every rule evaluation, once
per alert, so keep them cheap. They can only return instant vectors, not range
vectors.

## Inspecting alerts during runtime

To manually inspect which alerts are active (pending or firing), navigate to
the "Alerts" tab of your Prometheus instance. This will show you the exact
label sets for which each defined alert is currently active.

For pending and firing alerts, Prometheus also stores synthetic time series of
the form `ALERTS{alertname="<alert name>", alertstate="<pending or firing>", <additional alert labels>}`.
The sample value is set to `1` as long as the alert is in the indicated active
(pending or firing) state, and the series is marked stale when this is no
longer the case.

## Sending alert notifications

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
