---
title: Feature flags
sort_rank: 12
---

Here is a list of features that are disabled by default since they are breaking changes or are considered experimental.
Their behaviour can change in future releases which will be communicated via the [release changelog](https://github.com/prometheus/prometheus/blob/main/CHANGELOG.md).

You can enable them using the `--enable-feature` flag with a comma separated list of features.
They may be enabled by default in future versions.

## Exemplars storage

`--enable-feature=exemplar-storage`

[OpenMetrics](https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#exemplars) introduces the ability for scrape targets to add exemplars to certain metrics. Exemplars are references to data outside of the MetricSet. A common use case are IDs of program traces.

Exemplar storage is implemented as a fixed size circular buffer that stores exemplars in memory for all series. Enabling this feature will enable the storage of exemplars scraped by Prometheus. The config file block [storage](configuration/configuration.md#configuration-file)/[exemplars](configuration/configuration.md#exemplars) can be used to control the size of circular buffer by # of exemplars. An exemplar with just a `trace_id=<jaeger-trace-id>` uses roughly 100 bytes of memory via the in-memory exemplar storage. If the exemplar storage is enabled, we will also append the exemplars to WAL for local persistence (for WAL duration).

## Memory snapshot on shutdown

`--enable-feature=memory-snapshot-on-shutdown`

This takes a snapshot of the chunks that are in memory along with the series information when shutting down and stores it on disk. This will reduce the startup time since the memory state can now be restored with this snapshot
and m-mapped chunks, while a WAL replay from disk is only needed for the parts of the WAL that are not part of the snapshot.

## Extra scrape metrics

`--enable-feature=extra-scrape-metrics`

When enabled, for each instance scrape, Prometheus stores a sample in the following additional time series:

- `scrape_timeout_seconds`. The configured `scrape_timeout` for a target. This allows you to measure each target to find out how close they are to timing out with `scrape_duration_seconds / scrape_timeout_seconds`.
- `scrape_sample_limit`. The configured `sample_limit` for a target. This allows you to measure each target
  to find out how close they are to reaching the limit with `scrape_samples_post_metric_relabeling / scrape_sample_limit`. Note that `scrape_sample_limit` can be zero if there is no limit configured, which means that the query above can return `+Inf` for targets with no limit (as we divide by zero). If you want to query only for targets that do have a sample limit use this query: `scrape_samples_post_metric_relabeling / (scrape_sample_limit > 0)`.
- `scrape_body_size_bytes`. The uncompressed size of the most recent scrape response, if successful. Scrapes failing because `body_size_limit` is exceeded report `-1`, other scrape failures report `0`.

## Per-step stats

`--enable-feature=promql-per-step-stats`

When enabled, passing `stats=all` in a query request returns per-step
statistics. Currently this is limited to totalQueryableSamples.

When disabled in either the engine or the query, per-step statistics are not
computed at all.

## Native Histograms

`--enable-feature=native-histograms`

_This feature flag is being phased out. You should not use it anymore._

Native histograms are a stable feature by now. However, to scrape native
histograms, a scrape config setting `scrape_native_histograms` is required. To
ease the transition, this feature flag sets the default value of
`scrape_native_histograms` to `true`. From v3.9 on, this feature flag will be a
true no-op, and the default value of `scrape_native_histograms` will be always
`false`. If you are still using this feature flag while running v3.8, update
your scrape configs and stop using the feature flag before upgrading to v3.9.

## Experimental PromQL functions

`--enable-feature=promql-experimental-functions`

Enables PromQL functions that are considered experimental. These functions
might change their name, syntax, or semantics. They might also get removed
entirely.

## Created Timestamps Zero Injection

`--enable-feature=created-timestamp-zero-ingestion`

Enables ingestion of created timestamp. Created timestamps are injected as 0 valued samples when appropriate. See [PromCon talk](https://youtu.be/nWf0BfQ5EEA) for details.

Currently Prometheus supports created timestamps only on the traditional
Prometheus Protobuf protocol (WIP for other protocols). Therefore, enabling
this feature pre-sets the global `scrape_protocols` configuration option to 
`[ PrometheusProto, OpenMetricsText1.0.0, OpenMetricsText0.0.1, PrometheusText0.0.4 ]`,
resulting in negotiating the Prometheus Protobuf protocol with first priority
(unless the `scrape_protocols` option is set to a different value explicitly).

Besides enabling this feature in Prometheus, created timestamps need to be exposed by the application being scraped.

## Concurrent evaluation of independent rules

`--enable-feature=concurrent-rule-eval`

By default, rule groups execute concurrently, but the rules within a group execute sequentially; this is because rules can use the
output of a preceding rule as its input. However, if there is no detectable relationship between rules then there is no
reason to run them sequentially.
When the `concurrent-rule-eval` feature flag is enabled, rules without any dependency on other rules within a rule group will be evaluated concurrently.
This has the potential to improve rule group evaluation latency and resource utilization at the expense of adding more concurrent query load.

The number of concurrent rule evaluations can be configured with `--rules.max-concurrent-rule-evals`, which is set to `4` by default.

## Serve old Prometheus UI

Fall back to serving the old (Prometheus 2.x) web UI instead of the new UI. The new UI that was released as part of Prometheus 3.0 is a complete rewrite and aims to be cleaner, less cluttered, and more modern under the hood. However, it is not fully feature complete and battle-tested yet, so some users may still prefer using the old UI.

`--enable-feature=old-ui`

## Metadata WAL Records

`--enable-feature=metadata-wal-records`

When enabled, Prometheus will store metadata in-memory and keep track of
metadata changes as WAL records on a per-series basis.

This must be used if you would like to send metadata using the new remote write 2.0.

## Delay compaction start time

`--enable-feature=delayed-compaction`

A random offset, up to `10%` of the chunk range, is added to the Head compaction start time. This assists Prometheus instances in avoiding simultaneous compactions and reduces the load on shared resources.

Only auto Head compactions and the operations directly resulting from them are subject to this delay.

In the event of multiple consecutive Head compactions being possible, only the first compaction experiences this delay.

Note that during this delay, the Head continues its usual operations, which include serving and appending series.

Despite the delay in compaction, the blocks produced are time-aligned in the same manner as they would be if the delay was not in place.

## Delay `__name__` label removal for PromQL engine

`--enable-feature=promql-delayed-name-removal`

When enabled, Prometheus will change the way in which the `__name__` label is removed from PromQL query results (for functions and expressions for which this is necessary). Specifically, it will delay the removal to the last step of the query evaluation, instead of every time an expression or function creating derived metrics is evaluated.

This allows optionally preserving the `__name__` label via the `label_replace` and `label_join` functions, and helps prevent the "vector cannot contain metrics with the same labelset" error, which can happen when applying a regex-matcher to the `__name__` label.

Note that evaluating parts of the query separately will still trigger the
labelset collision. This commonly happens when analyzing intermediate results
of a query manually or with a tool like PromLens.

If a query refers to the already removed `__name__` label, its behavior may
change while this feature flag is set. (Example: `sum by (__name__)
(rate({foo="bar"}[5m]))`, see [details on
GitHub](https://github.com/prometheus/prometheus/issues/11397#issuecomment-1451998792).)
These queries are rare to occur and easy to fix. (In the above example,
removing `by (__name__)` doesn't change anything without the feature flag and
fixes the possible problem with the feature flag.)

## Auto Reload Config

`--enable-feature=auto-reload-config`

When enabled, Prometheus will automatically reload its configuration file at a
specified interval. The interval is defined by the
`--config.auto-reload-interval` flag, which defaults to `30s`.

Configuration reloads are triggered by detecting changes in the checksum of the
main configuration file or any referenced files, such as rule and scrape
configurations. To ensure consistency and avoid issues during reloads, it's
recommended to update these files atomically.

## OTLP Delta Conversion

`--enable-feature=otlp-deltatocumulative`

When enabled, Prometheus will convert OTLP metrics from delta temporality to their
cumulative equivalent, instead of dropping them. This cannot be enabled in conjunction with `otlp-native-delta-ingestion`.

This uses
[deltatocumulative][d2c]
from the OTel collector, using its default settings.

Delta conversion keeps in-memory state to aggregate delta changes per-series over time.
When Prometheus restarts, this state is lost, starting the aggregation from zero
again. This results in a counter reset in the cumulative series.

This state is periodically ([`max_stale`][d2c]) cleared of inactive series.

Enabling this _can_ have negative impact on performance, because the in-memory
state is mutex guarded. Cumulative-only OTLP requests are not affected.

[d2c]: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/deltatocumulativeprocessor

## PromQL arithmetic expressions in time durations

`--enable-feature=promql-duration-expr`

With this flag, arithmetic expressions can be used in time durations in range queries and offset durations.

In range queries:
```
rate(http_requests_total[5m * 2])  # 10 minute range
rate(http_requests_total[(5+2) * 1m])  # 7 minute range
```

In offset durations:
```
http_requests_total offset (1h / 2)  # 30 minute offset
http_requests_total offset ((2 ^ 3) * 1m)  # 8 minute offset
```

When using offset with duration expressions, you must wrap the expression in
parentheses. Without parentheses, only the first duration value will be used in
the offset calculation.

`step()` can be used in duration expressions.
For a **range query**, it resolves to the step width of the range query.
For an **instant query**, it resolves to `0s`. 

`min(<duration>, <duration>)` and `max(<duration>, <duration>)` can be used to find the minimum or maximum of two duration expressions.

**Note**: Duration expressions are not supported in the @ timestamp operator.

The following operators are supported:

* `+` - addition
* `-` - subtraction
* `*` - multiplication
* `/` - division
* `%` - modulo
* `^` - exponentiation

Examples of equivalent durations:

* `5m * 2` is equivalent to `10m` or `600s`
* `10m - 1m` is equivalent to `9m` or `540s`
* `(5+2) * 1m` is equivalent to `7m` or `420s`
* `1h / 2` is equivalent to `30m` or `1800s`
* `4h % 3h` is equivalent to `1h` or `3600s`
* `(2 ^ 3) * 1m` is equivalent to `8m` or `480s`
* `step() + 1` is equivalent to the query step width increased by 1s.
* `max(step(), 5s)` is equivalent to the larger of the query step width and `5s`.
* `min(2 * step() + 5s, 5m)` is equivalent to the smaller of twice the query step increased by `5s` and `5m`.


## OTLP Native Delta Support

`--enable-feature=otlp-native-delta-ingestion`

When enabled, allows for the native ingestion of delta OTLP metrics, storing the raw sample values without conversion. This cannot be enabled in conjunction with `otlp-deltatocumulative`.

Currently, the StartTimeUnixNano field is ignored, and deltas are given the unknown metric metadata type.

Delta support is in a very early stage of development and the ingestion and querying process my change over time. For the open proposal see [prometheus/proposals#48](https://github.com/prometheus/proposals/pull/48).

### Querying

We encourage users to experiment with deltas and existing PromQL functions; we will collect feedback and likely build features to improve the experience around querying deltas.

Note that standard PromQL counter functions like `rate()` and `increase()` are designed for cumulative metrics and will produce incorrect results when used with delta metrics. This may change in the future, but for now, to get similar results for delta metrics, you need `sum_over_time()`:

* `sum_over_time(delta_metric[<range>])`: Calculates the sum of delta values over the specified time range.
* `sum_over_time(delta_metric[<range>]) / <range>`: Calculates the per-second rate of the delta metric.

These may not work well if the `<range>` is not a multiple of the collection interval of the metric. For example, if you do `sum_over_time(delta_metric[1m]) / 1m` range query (with a 1m step), but the collection interval of a metric is 10m, the graph will show a single point every 10 minutes with a high rate value, rather than 10 points with a lower, constant value.

### Current gotchas

* If delta metrics are exposed via [federation](https://prometheus.io/docs/prometheus/latest/federation/), data can be incorrectly collected if the ingestion interval is not the same as the scrape interval for the federated endpoint.

* It is difficult to figure out whether a metric has delta or cumulative temporality, since there's no indication of temporality in metric names or labels. For now, if you are ingesting a mix of delta and cumulative metrics we advise you to explicitly add your own labels to distinguish them. In the future, we plan to introduce type labels to consistently distinguish metric types and potentially make PromQL functions type-aware (e.g. providing warnings when cumulative-only functions are used with delta metrics).

* If there are multiple samples being ingested at the same timestamp, only one of the points is kept - the samples are **not** summed together (this is how Prometheus works in general - duplicate timestamp samples are rejected). Any aggregation will have to be done before sending samples to Prometheus.

## Type and Unit Labels

`--enable-feature=type-and-unit-labels`

When enabled, Prometheus will start injecting additional, reserved `__type__`
and `__unit__` labels as designed in the [PROM-39 proposal](https://github.com/prometheus/proposals/pull/39).

Those labels are sourced from the metadata structures of the existing scrape and ingestion formats
like OpenMetrics Text, Prometheus Text, Prometheus Proto, Remote Write 2 and OTLP. All the user provided labels with
`__type__` and `__unit__` will be overridden.

PromQL layer will handle those labels the same way __name__ is handled, e.g. dropped
on certain operations like `-` or `+` and affected by `promql-delayed-name-removal` feature.

This feature enables important metadata information to be accessible directly with samples and PromQL layer.

It's especially useful for users who:

* Want to be able to select metrics based on type or unit.
* Want to handle cases of series with the same metric name and different type and units.
  e.g. native histogram migrations or OpenTelemetry metrics from OTLP endpoint, without translation.

In future more [work is planned](https://github.com/prometheus/prometheus/issues/16610) that will depend on this e.g. rich PromQL UX that helps
when wrong types are used on wrong functions, automatic renames, delta types and more.

### Behavior with metadata records

When this feature is enabled and the metadata WAL records exists, in an unlikely situation when type or unit are different across those, 
the Prometheus outputs intends to prefer the `__type__` and `__unit__` labels values. For example on Remote Write 2.0, 
if  the metadata record somehow (e.g. due to bug) says "counter", but `__type__="gauge"` the remote time series will be set to a gauge.

## Use Uncached IO

`--enable-feature=use-uncached-io`

Experimental and only available on Linux.

When enabled, it makes chunks writing bypass the page cache. Its primary
goal is to reduce confusion around page‐cache behavior and to prevent over‑allocation of
memory in response to misleading cache growth.

This is currently implemented using direct I/O.

For more details, see the [proposal](https://github.com/prometheus/proposals/pull/45).

## Extended Range Selectors

`--enable-feature=promql-extended-range-selectors`

Enables experimental `anchored` and `smoothed` modifiers for PromQL range and instant selectors. These modifiers provide more control over how range boundaries are handled in functions like `rate` and `increase`, especially with missing or irregular data.

Native Histograms are not yet supported by the extended range selectors.

### `anchored`

Uses the most recent sample (within the lookback delta) at the beginning of the range, or alternatively the first sample within the range if there is no sample within the lookback delta. The last sample within the range is also used at the end of the range. No extrapolation or interpolation is applied, so this is useful to get the direct difference between sample values.

Anchored range selector work with: `resets`, `changes`, `rate`, `increase`, and `delta`.

Example query:
`increase(http_requests_total[5m] anchored)`

**Note**: When using the anchored modifier with the increase function, the results returned are integers.

### `smoothed`

In range selectors, linearly interpolates values at the range boundaries, using the sample values before and after the boundaries for an improved estimation that is robust against irregular scrapes and missing samples. However, it requires a sample after the evaluation interval to work properly, see note below.

For instant selectors, values are linearly interpolated at the evaluation timestamp using the samples immediately before and after that point.

Smoothed range selectors work with: `rate`, `increase`, and `delta`.

Example query:
`rate(http_requests_total[step()] smoothed)`

> **Note for alerting and recording rules:**
> The `smoothed` modifier requires samples after the evaluation interval, so using it directly in alerting or recording rules will typically *under-estimate* the result, as future samples are not available at evaluation time.
> To use `smoothed` safely in rules, you **must** apply a `query_offset` to the rule group (see [documentation](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/#rule_group)) to ensure the calculation window is fully in the past and all needed samples are available.  
> For critical alerting, set the offset to at least one scrape interval; for less critical or more resilient use cases, consider a larger offset (multiple scrape intervals) to tolerate missed scrapes.

For more details, see the [design doc](https://github.com/prometheus/proposals/blob/main/proposals/2025-04-04_extended-range-selectors-semantics.md).

**Note**: Extended Range Selectors are not supported for subqueries.
