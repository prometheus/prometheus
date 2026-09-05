# Semantic convention registry

`registry.yaml` declares every metric the Prometheus binary exposes. It follows
the [OpenTelemetry semantic convention][semconv] schema, so the common fields
(`id`, `brief`, `metric_name`, `instrument`, `unit`, `stability`) mean what that
specification says they mean. Everything else here is Prometheus specific.

[semconv]: https://opentelemetry.io/docs/specs/semconv/

A metric with a label and a histogram configuration looks like this:

```yaml
groups:
  - id: attr.alertmanager
    type: attribute_group
    brief: Attributes for alertmanager metrics.
    attributes:
      - id: alertmanager
        type: string
        stability: development
        brief: The alertmanager instance URL.
        examples:
          - "http://alertmanager:9093/api/v2/alerts"

  - id: metric.prometheus_notifications_latency_histogram_seconds
    type: metric
    stability: development
    brief: Latency histogram for sending alert notifications.
    metric_name: prometheus_notifications_latency_histogram_seconds
    instrument: histogram
    unit: s
    attributes:
      - ref: alertmanager
    annotations:
      prometheus:
        histogram_type: mixed_histogram
        buckets: [0.01, 0.1, 1, 10]
        bucket_factor: 1.1
        max_bucket_number: 100
        min_reset_duration: "1h"
```

## Attributes are labels

An OpenTelemetry attribute is a Prometheus label. A label is defined once in an
`attribute_group`, with its type and an example value, and every metric that
carries it refers to that definition with `ref`. One definition serves every
metric that uses the label, and Weaver fails resolution on a `ref` that matches
no definition.

The registry does not record whether a label is variable or fixed at
construction time. Both are declared the same way, and the code decides. A
metric that lists no attributes has no labels.

## Histograms and summaries

OpenTelemetry has one histogram instrument. Prometheus has four shapes, so
`annotations.prometheus.histogram_type` is required on every metric declared
with `instrument: histogram`, and it selects which bucket fields apply.

| `histogram_type`    | Prometheus type              | Bucket fields                                                |
| ------------------- | ---------------------------- | ------------------------------------------------------------ |
| `classic_histogram` | Fixed buckets                | `buckets` or `exponential_buckets`                             |
| `native_histogram`  | Exponential buckets          | `bucket_factor`, `max_bucket_number`, `min_reset_duration`     |
| `mixed_histogram`   | Both at once                 | all of the above                                               |
| `summary`           | Summary, not a histogram     | `objectives`, mapping quantile to allowed error                |

Semantic conventions have no summary instrument, so a Prometheus summary is
declared as a histogram carrying
`histogram_type: summary`. Anything that reads `instrument` to decide a metric's
type has to check this annotation first, or it will report a summary as a
histogram.

`exponential_buckets` takes `start`, `factor`, and `count` rather than a list of
boundaries.

## Callback metrics

Some metrics compute their value when scraped rather than being set by the
program. `prometheus.GaugeFunc` is the common case. A generated constructor has
nothing to do for these, so the registry marks them `only_opts` and generation
emits only an `Opts()` accessor carrying the schema-owned name, help, and unit.
The closure stays in hand-written code.

```yaml
annotations:
  prometheus:
    only_opts: true # Implemented as GaugeFunc.
```

## Stability

`stability` records where a metric sits in its lifecycle so that a change to it
shows up in review. What `stable` obligates Prometheus to, and how long a
deprecation runs, are policy questions this registry does not settle. Metrics carry `development` until that policy
exists.
