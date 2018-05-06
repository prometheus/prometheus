---
title: Querying examples
nav_title: Examples
sort_rank: 4
---

# Query examples

Most of these examples will work on a standard installation of Prometheus.
Once these examples start using metric names starting with `instance_`, you
will also need a working a working node_exporter running and configured for
these examples to work.

## Simple time series selection

Return all time series with the metric `http_requests_total`:

    http_requests_total

Return all time series with the metric `http_requests_total` and the given
`job` and `handler` labels:

    http_requests_total{job="prometheus", handler="query"}

Return a whole range of time (in this case 5 minutes) for the same vector,
making it a range vector:

    http_requests_total{job="prometheus", handler="query"}[5m]

Note that an expression resulting in a range vector cannot be graphed directly,
but viewed in the tabular ("Console") view of the expression browser.

Using regular expressions, you could select time series only for jobs whose
name match a certain pattern, in this case, all jobs that start with `prom`.
Note that this does a substring match, not a full string match:

    http_requests_total{job=~"prom.*"}

All regular expressions in Prometheus use [RE2
syntax](https://github.com/google/re2/wiki/Syntax).

To select all HTTP status codes except 4xx ones, you could run:

    http_requests_total{status!~"4.."}

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
returns the amount of RAM without any swap memory, in MiB:

    (node_memory_MemTotal - node_memory_SwapTotal) / 2^20

The same expression, but summed by job, could be written like this:

    sum(
    	node_memory_MemTotal - node_memory_SwapTotal
    ) by (job) / 2^20

It's also possible to sum by more than one dimension, e.g. the sum of
all non-idle CPU seconds by both instance and cpu:

    sum(
		rate(
			node_cpu{mode=~"idle"}[5m]
			)
		) by (instance, cpu)

or in shorter form

    sum(rate(node_cpu{mode=~"idle"}[5m])) by (instance, cpu)

of which we can also get the top three:

    topk(3, sum(rate(node_cpu{mode=~"idle"}[5m])) by (instance, cpu))

It's also possible to count the amount of time series:

    count(node_cpu) by (instance, cpu)
