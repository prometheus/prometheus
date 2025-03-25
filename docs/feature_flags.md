---
title: Feature flags
sort_rank: 12
---

# Feature flags

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
`always_scrape_classic_histograms` in the scrape job.

## Experimental PromQL functions

`--enable-feature=promql-experimental-functions`

Enables PromQL functions that are considered experimental. These functions
might change their name, syntax, or semantics. They might also get removed
entirely.

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
cumulative equivalent, instead of dropping them.

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
