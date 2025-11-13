---
title: Prometheus 3.0 migration guide
nav_title: Migration
sort_rank: 10
---

In line with our [stability promise](https://prometheus.io/docs/prometheus/latest/stability/),
the Prometheus 3.0 release contains a number of backwards incompatible changes.
This document offers guidance on migrating from Prometheus 2.x to Prometheus 3.0 and newer versions.

## Flags

- The following feature flags have been removed and they have been added to the
  default behavior of Prometheus v3:
  - `promql-at-modifier`
  - `promql-negative-offset`
  - `new-service-discovery-manager`
  - `expand-external-labels`
      - Environment variable references `${var}` or `$var` in external label values
    are replaced according to the values of the current environment variables.
      - References to undefined variables are replaced by the empty string.
    The `$` character can be escaped by using `$$`.
  - `no-default-scrape-port`
      - Prometheus v3 will no longer add ports to scrape targets according to the
    specified scheme. Target will now appear in labels as configured.
      - If you rely on scrape targets like
      `https://example.com/metrics` or `http://example.com/metrics` to be
      represented as `https://example.com/metrics:443` and
      `http://example.com/metrics:80` respectively, add them to your target URLs
  - `agent`
      - Instead use the dedicated `--agent` CLI flag.
  - `remote-write-receiver`
      - Instead use the dedicated `--web.enable-remote-write-receiver` CLI flag to enable the remote write receiver.
  - `auto-gomemlimit`
      - Prometheus v3 will automatically set `GOMEMLIMIT` to match the Linux
      container memory limit. If there is no container limit, or the process is
      running outside of containers, the system memory total is used. To disable
      this, `--no-auto-gomemlimit` is available.
  - `auto-gomaxprocs`
      - Prometheus v3 will automatically set `GOMAXPROCS` to match the Linux
      container CPU quota. To disable this, `--no-auto-gomaxprocs` is available.

  Prometheus v3 will log a warning if you continue to pass these to
  `--enable-feature`.

- Starting from v3.9, the feature flag `native-histograms` is a no-op. Native
  histograms are a stable feature now, but scraping them has to be enabled via
  the `scrape_native_histograms` global or per-scrape configuration option
  (added in v3.8).

## Configuration

- The scrape job level configuration option `scrape_classic_histograms` has
  been renamed to `always_scrape_classic_histograms`. If you use the
  `scrape_native_histograms` scrape configuration option to ingest native
  histograms and you also want to ingest classic histograms that an endpoint
  might expose along with native histograms, be sure to add this configuration
  or change your configuration from the old name.
- The `http_config.enable_http2` in `remote_write` items default has been
  changed to `false`. In Prometheus v2 the remote write http client would
  default to use http2. In order to parallelize multiple remote write queues
  across multiple sockets its preferable to not default to http2.
  If you prefer to use http2 for remote write you must now set
  `http_config.enable_http2: true` in your `remote_write` configuration section.

## PromQL

### Regular expressions match newlines

The `.` pattern in regular expressions in PromQL matches newline characters.
With this change a regular expressions like `.*` matches strings that include
`\n`. This applies to matchers in queries and relabel configs.

For example, the following regular expressions now match the accompanying
strings, whereas in Prometheus v2 these combinations didn't match.
    - `.*` additionally matches `foo\n` and `Foo\nBar`
    - `foo.?bar` additionally matches `foo\nbar`
    - `foo.+bar` additionally matches `foo\nbar`

If you want Prometheus v3 to behave like v2, you will have to change your
regular expressions by replacing all `.` patterns with `[^\n]`, e.g.
`foo[^\n]*`.

### Range selectors and lookback exclude samples coinciding with the left boundary

Lookback and range selectors are now left-open and right-closed (previously
left-closed and right-closed), which makes their behavior more consistent. This
change affects queries where the left boundary of a range or the lookback delta
coincides with the timestamp of one or more samples.

For example, assume we are querying a timeseries with evenly spaced samples
exactly 1 minute apart. Before Prometheus v3, a range query with `5m` would
usually return 5 samples. But if the query evaluation aligns perfectly with a
scrape, it would return 6 samples. In Prometheus v3 queries like this will
always return 5 samples given even spacing.

This change will typically affect subqueries because their evaluation timing is
naturally perfectly evenly spaced and aligned with timestamps that are multiples
of the subquery resolution. Furthermore, query frontends often align subqueries
to multiples of the step size. In combination, this easily creates a situation
of perfect mutual alignment, often unintended and unknown by the user, so that
the new behavior might come as a surprise. Before Prometheus V3, a subquery of
`foo[1m:1m]` on such a system might have always returned two points, allowing
for rate calculations. In Prometheus V3, however, such a subquery will only
return one point, which is insufficient for a rate or increase calculation,
resulting in No Data returned.

Such queries will need to be rewritten to extend the window to properly cover
more than one point. In this example, `foo[2m:1m]` would always return two
points no matter the query alignment. The exact form of the rewritten query may
depend on the intended results and there is no universal drop-in replacement for
queries whose behavior has changed.

Tests are similarly more likely to affected. To fix those either adjust the
expected number of samples or extend the range.

### holt_winters function renamed

The `holt_winters` function has been renamed to `double_exponential_smoothing`
and is now guarded by the `promql-experimental-functions` feature flag.
If you want to keep using `holt_winters`, you have to do both of these things:
  - Rename `holt_winters` to `double_exponential_smoothing` in your queries.
  - Pass `--enable-feature=promql-experimental-functions` in your Prometheus
    CLI invocation.

## Scrape protocols
Prometheus v3 is more strict concerning the Content-Type header received when
scraping. Prometheus v2 would default to the standard Prometheus text protocol
if the target being scraped did not specify a Content-Type header or if the
header was unparsable or unrecognised. This could lead to incorrect data being
parsed in the scrape. Prometheus v3 will now fail the scrape in such cases.

If a scrape target is not providing the correct Content-Type header the
fallback protocol can be specified using the `fallback_scrape_protocol`
parameter. See [Prometheus scrape_config documentation.](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config)

This is a breaking change as scrapes that may have succeeded with Prometheus v2
may now fail if this fallback protocol is not specified.

## Miscellaneous

### TSDB format and downgrade

The TSDB format has been changed slightly in Prometheus v2.55 in preparation for changes
to the index format. Consequently, a Prometheus v3 TSDB can only be read by a
Prometheus v2.55 or newer. Keep that in mind when upgrading to v3 -- you will be only
able to downgrade to v2.55, not lower, without losing your TSDB persistent data.

As an extra safety measure, you could optionally consider upgrading to v2.55 first and
confirm Prometheus works as expected, before upgrading to v3.

### TSDB storage contract

TSDB compatible storage is now expected to return results matching the specified
selectors. This might impact some third party implementations, most likely
implementing `remote_read`.

This contract is not explicitly enforced, but can cause undefined behavior.

### UTF-8 names

Prometheus v3 supports UTF-8 in metric and label names. This means metric and
label names can change after upgrading according to what is exposed by
endpoints. Furthermore, metric and label names that would have previously been
flagged as invalid no longer will be.

Users wishing to preserve the original validation behavior can update their
Prometheus yaml configuration to specify the legacy validation scheme:

```
global:
  metric_name_validation_scheme: legacy
```

Or on a per-scrape basis:

```
scrape_configs:
  - job_name: job1
    metric_name_validation_scheme: utf8
  - job_name: job2
    metric_name_validation_scheme: legacy
```

### Log message format
Prometheus v3 has adopted `log/slog` over the previous `go-kit/log`. This
results in a change of log message format. An example of the old log format is:

```
ts=2024-10-23T22:01:06.074Z caller=main.go:627 level=info msg="No time or size retention was set so using the default time retention" duration=15d
ts=2024-10-23T22:01:06.074Z caller=main.go:671 level=info msg="Starting Prometheus Server" mode=server version="(version=, branch=, revision=91d80252c3e528728b0f88d254dd720f6be07cb8-modified)"
ts=2024-10-23T22:01:06.074Z caller=main.go:676 level=info build_context="(go=go1.23.0, platform=linux/amd64, user=, date=, tags=unknown)"
ts=2024-10-23T22:01:06.074Z caller=main.go:677 level=info host_details="(Linux 5.15.0-124-generic #134-Ubuntu SMP Fri Sep 27 20:20:17 UTC 2024 x86_64 gigafips (none))"
```

a similar sequence in the new log format looks like this:

```
time=2024-10-24T00:03:07.542+02:00 level=INFO source=/home/user/go/src/github.com/prometheus/prometheus/cmd/prometheus/main.go:640 msg="No time or size retention was set so using the default time retention" duration=15d
time=2024-10-24T00:03:07.542+02:00 level=INFO source=/home/user/go/src/github.com/prometheus/prometheus/cmd/prometheus/main.go:681 msg="Starting Prometheus Server" mode=server version="(version=, branch=, revision=7c7116fea8343795cae6da42960cacd0207a2af8)"
time=2024-10-24T00:03:07.542+02:00 level=INFO source=/home/user/go/src/github.com/prometheus/prometheus/cmd/prometheus/main.go:686 msg="operational information" build_context="(go=go1.23.0, platform=linux/amd64, user=, date=, tags=unknown)" host_details="(Linux 5.15.0-124-generic #134-Ubuntu SMP Fri Sep 27 20:20:17 UTC 2024 x86_64 gigafips (none))" fd_limits="(soft=1048576, hard=1048576)" vm_limits="(soft=unlimited, hard=unlimited)"
```

### `le` and `quantile` label values
In Prometheus v3, the values of the `le` label of classic histograms and the
`quantile` label of summaries are normalized upon ingestion. In Prometheus v2
the value of these labels depended on the scrape protocol (protobuf vs text
format) in some situations. This led to label values changing based on the
scrape protocol. E.g. a metric exposed as `my_classic_hist{le="1"}` would be
ingested as `my_classic_hist{le="1"}` via the text format, but as
`my_classic_hist{le="1.0"}` via protobuf. This changed the identity of the
metric and caused problems when querying the metric.
In Prometheus v3 these label values will always be normalized to a float like
representation. I.e. the above example will always result in
`my_classic_hist{le="1.0"}` being ingested into prometheus, no matter via which
protocol. The effect of this change is that alerts, recording rules and
dashboards that directly reference label values as whole numbers such as
`le="1"` will stop working.

Ways to deal with this change either globally or on a per metric basis:

- Fix references to integer `le`, `quantile` label values, but otherwise do
nothing and accept that some queries that span the transition time will produce
inaccurate or unexpected results.
_This is the recommended solution._
- Use `metric_relabel_config` to retain the old labels when scraping targets.
This should **only** be applied to metrics that currently produce such labels.

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

### Disallow configuring Alertmanager with the v1 API
Prometheus 3 no longer supports Alertmanager's v1 API. Effectively Prometheus 3
requires [Alertmanager 0.16.0](https://github.com/prometheus/alertmanager/releases/tag/v0.16.0) or later. Users with older Alertmanager
versions or configurations that use `alerting: alertmanagers: [api_version: v1]`
need to upgrade Alertmanager and change their configuration to use `api_version: v2`.

## Prometheus 2.0 migration guide

For the migration guide from Prometheus 1.8 to 2.0 please refer to the [Prometheus v2.55 documentation](https://prometheus.io/docs/prometheus/2.55/migration/).
