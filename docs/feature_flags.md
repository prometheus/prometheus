---
title: Feature flags
sort_rank: 11
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

Exemplar storage is implemented as a fixed size circular buffer that stores exemplars in memory for all series. Enabling this feature will enable the storage of exemplars scraped by Prometheus. The config file block [storage](configuration/configuration.md#configuration-file)/[exemplars](configuration/configuration.md#exemplars) can be used to control the size of circular buffer by # of exemplars. An exemplar with just a `traceID=<jaeger-trace-id>` uses roughly 100 bytes of memory via the in-memory exemplar storage. If the exemplar storage is enabled, we will also append the exemplars to WAL for local persistence (for WAL duration).

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

## No default scrape port

`--enable-feature=no-default-scrape-port`

When enabled, the default ports for HTTP (`:80`) or HTTPS (`:443`) will _not_ be added to
the address used to scrape a target (the value of the `__address_` label), contrary to the default behavior.
In addition, if a default HTTP or HTTPS port has already been added either in a static configuration or
by a service discovery mechanism and the respective scheme is specified (`http` or `https`), that port will be removed.
