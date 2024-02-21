---
title: Feature flags
sort_rank: 12
---

# Feature flags

Here is a list of features that are disabled by default since they are breaking changes or are considered experimental.
Their behaviour can change in future releases which will be communicated via the [release changelog](https://github.com/prometheus/prometheus/blob/main/CHANGELOG.md).

You can enable them using the `--enable-feature` flag with a comma separated list of features.
They may be enabled by default in future versions.

## Expand environment variables in external labels

`--enable-feature=expand-external-labels`

Replace `${var}` or `$var` in the [`external_labels`](configuration/configuration.md#configuration-file)
values according to the values of the current environment variables. References
to undefined variables are replaced by the empty string.
The `$` character can be escaped by using `$$`.

## Remote Write Receiver

`--enable-feature=remote-write-receiver`

The remote write receiver allows Prometheus to accept remote write requests from other Prometheus servers. More details can be found [here](storage.md#overview).

Activating the remote write receiver via a feature flag is deprecated. Use `--web.enable-remote-write-receiver` instead. This feature flag will be ignored in future versions of Prometheus.

## Exemplars storage

`--enable-feature=exemplar-storage`

[OpenMetrics](https://github.com/OpenObservability/OpenMetrics/blob/main/specification/OpenMetrics.md#exemplars) introduces the ability for scrape targets to add exemplars to certain metrics. Exemplars are references to data outside of the MetricSet. A common use case are IDs of program traces.

Exemplar storage is implemented as a fixed size circular buffer that stores exemplars in memory for all series. Enabling this feature will enable the storage of exemplars scraped by Prometheus. The config file block [storage](configuration/configuration.md#configuration-file)/[exemplars](configuration/configuration.md#exemplars) can be used to control the size of circular buffer by # of exemplars. An exemplar with just a `trace_id=<jaeger-trace-id>` uses roughly 100 bytes of memory via the in-memory exemplar storage. If the exemplar storage is enabled, we will also append the exemplars to WAL for local persistence (for WAL duration).

## Memory snapshot on shutdown

`--enable-feature=memory-snapshot-on-shutdown`

This takes the snapshot of the chunks that are in memory along with the series information when shutting down and stores
it on disk. This will reduce the startup time since the memory state can be restored with this snapshot and m-mapped
chunks without the need of WAL replay.

## Extra scrape metrics

`--enable-feature=extra-scrape-metrics`

When enabled, for each instance scrape, Prometheus stores a sample in the following additional time series:

- `scrape_timeout_seconds`. The configured `scrape_timeout` for a target. This allows you to measure each target to find out how close they are to timing out with `scrape_duration_seconds / scrape_timeout_seconds`.
- `scrape_sample_limit`. The configured `sample_limit` for a target. This allows you to measure each target
  to find out how close they are to reaching the limit with `scrape_samples_post_metric_relabeling / scrape_sample_limit`. Note that `scrape_sample_limit` can be zero if there is no limit configured, which means that the query above can return `+Inf` for targets with no limit (as we divide by zero). If you want to query only for targets that do have a sample limit use this query: `scrape_samples_post_metric_relabeling / (scrape_sample_limit > 0)`.
- `scrape_body_size_bytes`. The uncompressed size of the most recent scrape response, if successful. Scrapes failing because `body_size_limit` is exceeded report `-1`, other scrape failures report `0`.

## New service discovery manager

`--enable-feature=new-service-discovery-manager`

When enabled, Prometheus uses a new service discovery manager that does not
restart unchanged discoveries upon reloading. This makes reloads faster and reduces
pressure on service discoveries' sources.

Users are encouraged to test the new service discovery manager and report any
issues upstream.

In future releases, this new service discovery manager will become the default and
this feature flag will be ignored.

## Prometheus agent

`--enable-feature=agent`

When enabled, Prometheus runs in agent mode. The agent mode is limited to
discovery, scrape and remote write.

This is useful when you do not need to query the Prometheus data locally, but
only from a central [remote endpoint](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage).

## Per-step stats

`--enable-feature=promql-per-step-stats`

When enabled, passing `stats=all` in a query request returns per-step
statistics. Currently this is limited to totalQueryableSamples.

When disabled in either the engine or the query, per-step statistics are not
computed at all.

## Auto GOMAXPROCS

`--enable-feature=auto-gomaxprocs`

When enabled, GOMAXPROCS variable is automatically set to match Linux container CPU quota.

## Auto GOMEMLIMIT

`--enable-feature=auto-gomemlimit`

When enabled, the GOMEMLIMIT variable is automatically set to match the Linux container memory limit. If there is no container limit, or the process is runing outside of containers, the system memory total is used.

There is also an additional tuning flag, `--auto-gomemlimit.ratio`, which allows controling how much of the memory is used for Prometheus. The remainder is reserved for memory outside the process. For example, kernel page cache. Page cache is important for Prometheus TSDB query performance. The default is `0.9`, which means 90% of the memory limit will be used for Prometheus.

## No default scrape port

`--enable-feature=no-default-scrape-port`

When enabled, the default ports for HTTP (`:80`) or HTTPS (`:443`) will _not_ be added to
the address used to scrape a target (the value of the `__address_` label), contrary to the default behavior.
In addition, if a default HTTP or HTTPS port has already been added either in a static configuration or
by a service discovery mechanism and the respective scheme is specified (`http` or `https`), that port will be removed.

## Native Histograms

`--enable-feature=native-histograms`

When enabled, Prometheus will ingest native histograms (formerly also known as
sparse histograms or high-res histograms). Native histograms are still highly
experimental. Expect breaking changes to happen (including those rendering the
TSDB unreadable).

Native histograms are currently only supported in the traditional Prometheus
protobuf exposition format. This feature flag therefore also enables a new (and
also experimental) protobuf parser, through which _all_ metrics are ingested
(i.e. not only native histograms). Prometheus will try to negotiate the
protobuf format first. The instrumented target needs to support the protobuf
format, too, _and_ it needs to expose native histograms. The protobuf format
allows to expose classic and native histograms side by side. With this feature
flag disabled, Prometheus will continue to parse the classic histogram (albeit
via the text format). With this flag enabled, Prometheus will still ingest
those classic histograms that do not come with a corresponding native
histogram. However, if a native histogram is present, Prometheus will ignore
the corresponding classic histogram, with the notable exception of exemplars,
which are always ingested. To keep the classic histograms as well, enable
`scrape_classic_histograms` in the scrape job.

_Note about the format of `le` and `quantile` label values:_

In certain situations, the protobuf parsing changes the number formatting of
the `le` labels of classic histograms and the `quantile` labels of
summaries. Typically, this happens if the scraped target is instrumented with
[client_golang](https://github.com/prometheus/client_golang) provided that
[promhttp.HandlerOpts.EnableOpenMetrics](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus/promhttp#HandlerOpts)
is set to `false`. In such a case, integer label values are represented in the
text format as such, e.g. `quantile="1"` or `le="2"`. However, the protobuf parsing
changes the representation to float-like (following the OpenMetrics
specification), so the examples above become `quantile="1.0"` and `le="2.0"` after
ingestion into Prometheus, which changes the identity of the metric compared to
what was ingested before via the text format.

The effect of this change is that alerts, recording rules and dashboards that
directly reference label values as whole numbers such as `le="1"` will stop
working.

Aggregation by the `le` and `quantile` labels for vectors that contain the old and
new formatting will lead to unexpected results, and range vectors that span the
transition between the different formatting will contain additional series.
The most common use case for both is the quantile calculation via
`histogram_quantile`, e.g.
`histogram_quantile(0.95, sum by (le) (rate(histogram_bucket[10m])))`.
The `histogram_quantile` function already tries to mitigate the effects to some
extent, but there will be inaccuracies, in particular for shorter ranges that
cover only a few samples.

Ways to deal with this change either globally or on a per metric basis:

- Fix references to integer `le`, `quantile` label values, but otherwise do
nothing and accept that some queries that span the transition time will produce
inaccurate or unexpected results.
_This is the recommended solution, to get consistently normalized label values._
Also Prometheus 3.0 is expected to enforce normalization of these label values.
- Use `metric_relabel_config` to retain the old labels when scraping targets.
This should **only** be applied to metrics that currently produce such labels.

<!-- The following config snippet is unit tested in scrape/scrape_test.go. -->
```yaml
    metric_relabel_configs:
      - source_labels:
          - quantile
        target_label: quantile
        regex: (\d+)\.0+
      - source_labels:
          - le
          - __name__
        target_label: le
        regex: (\d+)\.0+;.*_bucket
```

## OTLP Receiver

`--enable-feature=otlp-write-receiver`

The OTLP receiver allows Prometheus to accept [OpenTelemetry](https://opentelemetry.io/) metrics writes.
Prometheus is best used as a Pull based system, and staleness, `up` metric, and other Pull enabled features 
won't work when you push OTLP metrics.

## Experimental PromQL functions

`--enable-feature=promql-experimental-functions`

Enables PromQL functions that are considered experimental and whose name or
semantics could change.

## Created Timestamps Zero Injection

`--enable-feature=created-timestamp-zero-ingestion`

Enables ingestion of created timestamp. Created timestamps are injected as 0 valued samples when appropriate. See [PromCon talk](https://youtu.be/nWf0BfQ5EEA) for details.

Currently Prometheus supports created timestamps only on the traditional Prometheus Protobuf protocol (WIP for other protocols). As a result, when enabling this feature, the Prometheus protobuf scrape protocol will be prioritized (See `scrape_config.scrape_protocols` settings for more details).

Besides enabling this feature in Prometheus, created timestamps need to be exposed by the application being scraped.

## Concurrent evaluation of independent rules

`--enable-feature=concurrent-rule-eval`

By default, rule groups execute concurrently, but the rules within a group execute sequentially; this is because rules can use the
output of a preceding rule as its input. However, if there is no detectable relationship between rules then there is no
reason to run them sequentially.
When the `concurrent-rule-eval` feature flag is enabled, rules without any dependency on other rules within a rule group will be evaluated concurrently.
This has the potential to improve rule group evaluation latency and resource utilization at the expense of adding more concurrent query load.

The number of concurrent rule evaluations can be configured with `--rules.max-concurrent-rule-evals`, which is set to `4` by default.
