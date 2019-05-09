---
title: Querying examples
nav_title: Examples
sort_rank: 4
---

# Query examples

## Simple time series selection

Return all time series with the metric `http_requests_total`:

    http_requests_total

Return all time series with the metric `http_requests_total` and the given
`job` and `handler` labels:

    http_requests_total{job="apiserver", handler="/api/comments"}

Return a whole range of time (in this case 5 minutes) for the same vector,
making it a range vector:

    http_requests_total{job="apiserver", handler="/api/comments"}[5m]

Note that an expression resulting in a range vector cannot be graphed directly,
but viewed in the tabular ("Console") view of the expression browser.

Using regular expressions, you could select time series only for jobs whose
name match a certain pattern, in this case, all jobs that end with `server`:

    http_requests_total{job=~".*server"}

All regular expressions in Prometheus use [RE2
syntax](https://github.com/google/re2/wiki/Syntax).

To select all HTTP status codes except 4xx ones, you could run:

    http_requests_total{status!~"4.."}

## Subquery

Return the 5-minute rate of the `http_requests_total` metric for the past 30 minutes, with a resolution of 1 minute.

    rate(http_requests_total[5m])[30m:1m]

This is an example of a nested subquery. The subquery for the `deriv` function uses the default resolution. Note that using subqueries unnecessarily is unwise.

    max_over_time(deriv(rate(distance_covered_total[5s])[30s:5s])[10m:])

## Using functions, operators, etc.

Return the per-second rate for all time series with the `http_requests_total`
metric name, as measured over the last 5 minutes:

    rate(http_requests_total[5m])

Assuming that the `http_requests_total` time series all have the labels `job`
(fanout by job name) and `instance` (fanout by instance of the job), we might
want to sum over the rate of all instances, so we get fewer output time series,
but still preserve the `job` dimension:

    sum(rate(http_requests_total[5m])) by (job)

If we have two different metrics with the same dimensional labels, we can apply
binary operators to them and elements on both sides with the same label set
will get matched and propagated to the output. For example, this expression
returns the unused memory in MiB for every instance (on a fictional cluster
scheduler exposing these metrics about the instances it runs):

    (instance_memory_limit_bytes - instance_memory_usage_bytes) / 1024 / 1024

The same expression, but summed by application, could be written like this:

    sum(
      instance_memory_limit_bytes - instance_memory_usage_bytes
    ) by (app, proc) / 1024 / 1024

If the same fictional cluster scheduler exposed CPU usage metrics like the
following for every instance:

    instance_cpu_time_ns{app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager"}
    instance_cpu_time_ns{app="elephant", proc="worker", rev="34d0f99", env="prod", job="cluster-manager"}
    instance_cpu_time_ns{app="turtle", proc="api", rev="4d3a513", env="prod", job="cluster-manager"}
    instance_cpu_time_ns{app="fox", proc="widget", rev="4d3a513", env="prod", job="cluster-manager"}
    ...

...we could get the top 3 CPU users grouped by application (`app`) and process
type (`proc`) like this:

    topk(3, sum(rate(instance_cpu_time_ns[5m])) by (app, proc))

Assuming this metric contains one time series per running instance, you could
count the number of running instances per application like this:

    count(instance_cpu_time_ns) by (app)
