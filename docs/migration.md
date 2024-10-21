---
title: Migration
sort_rank: 10
---

# Prometheus 3.0 migration guide

In line with our [stability promise](https://prometheus.io/docs/prometheus/latest/stability/),
the Prometheus 3.0 release contains a number of backwards incompatible changes.
This document offers guidance on migrating from Prometheus 2.x to Prometheus 3.0 and newer versions.

## Flags

- The following feature flags have been removed and they have been added to the 
  default behavior of Prometheus v3:
  - `promql-at-modifier`
  - `promql-negative-offset`
  - `remote-write-receiver`
  - `new-service-discovery-manager`
  - `expand-external-labels`
    Environment variable references `${var}` or `$var` in external label values 
    are replaced according to the values of the current environment variables.  
    References to undefined variables are replaced by the empty string.
    The `$` character can be escaped by using `$$`.
  - `no-default-scrape-port`
    Prometheus v3 will no longer add ports to scrape targets according to the 
    specified scheme. Target will now appear in labels as configured.
    If you rely on scrape targets like 
    `https://example.com/metrics` or `http://exmaple.com/metrics` to be 
    represented as `https://example.com/metrics:443` and 
    `http://example.com/metrics:80` respectively, add them to your target URLs
    - `agent`
      Instead use the dedicated `--agent` cli flag.

  Prometheus v3 will log a warning if you continue to pass these to 
  `--enable-feature`.

## PromQL

- The `.` pattern in regular expressions in PromQL matches newline characters. 
  With this change a regular expressions like `.*` matches strings that include 
  `\n`. This applies to matchers in queries and relabel configs. For example the 
  following regular expressions now match the accompanying strings, wheras in 
  Prometheus v2 these combinations didn't match.

| Regex      | Additional matches  |
| -----      | ------              |
| ".*"       | "foo\n", "Foo\nBar" |
| "foo.?bar" | "foo\nbar"          |
| "foo.+bar" | "foo\nbar"          |

  If you want Prometheus v3 to behave like v2 did, you will have to change your 
  regular expressions by replacing all `.` patterns with `[^\n]`, e.g.  
  `foo[^\n]*`.
- Lookback and range selectors are left open and right closed (previously left 
  closed and right closed). This change affects queries when the evaluation time 
  perfectly aligns with the sample timestamps. For example assume querying a 
  timeseries with even spaced samples exactly 1 minute apart. Before Prometheus 
  3.x, range query with `5m` will mostly return 5 samples. But if the query 
  evaluation aligns perfectly with a scrape, it would return 6 samples. In 
  Prometheus 3.x queries like this will always return 5 samples.
  This change has likely few effects for everyday use, except for some sub query 
  use cases.
  Query front-ends that align queries usually align sub-queries to multiples of 
  the step size. These sub queries will likely be affected.
  Tests are more likely to affected. To fix those either adjust the expected 
  number of samples or extend to range by less then one sample interval.
- The `holt_winters` function has been renamed to `double_exponential_smoothing` 
  and is now guarded by the `promql-experimental-functions` feature flag.
  If you want to keep using holt_winters, you have to do both of these things:
    - Rename holt_winters to double_exponential_smoothing in your queries.
    - Pass `--enable-feature=promql-experimental-functions` in your Prometheus 
      cli invocation..

## Scrape protocols
Prometheus v3 is more strict concerning the Content-Type header received when
scraping. Prometheus v2 would default to the standard Prometheus text protocol
if the target being scraped did not specify a Content-Type header or if the
header was unparsable or unrecognised. This could lead to incorrect data being
parsed in the scrape. Prometheus v3 will now fail the scrape in such cases.

If a scrape target is not providing the correct Content-Type header the
fallback protocol can be specified using the fallback_scrape_protocol
parameter. See [Prometheus scrape_config documentation.](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config)

This is a breaking change as scrapes that may have succeeded with Prometheus v2
may now fail if this fallback protocol is not specified.

## Miscellaneous

### TSDB format and downgrade
The TSDB format has been changed in Prometheus v2.55 in preparation for changes 
to the index format. Consequently a Prometheus v3 tsdb can only be read by a 
Prometheus v2.55 or newer.

### TSDB Storage contract
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
prometheus yaml configuration to specify the legacy validation scheme:

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

# Prometheus 2.0 migration guide

For the Prometheus 1.8 to 2.0 please refer to the [Prometheus v2.55 documentation](https://prometheus.io/docs/prometheus/2.55/migration/).
