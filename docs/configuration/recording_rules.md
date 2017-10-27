---
title: Recording rules
sort_rank: 2
---

# Defining recording rules

## Configuring rules

Prometheus supports two types of rules which may be configured and then
evaluated at regular intervals: recording rules and [alerting
rules](alerting_rules.md). To include rules in Prometheus, create a file
containing the necessary rule statements and have Prometheus load the file via
the `rule_files` field in the [Prometheus configuration](configuration.md).

The rule files can be reloaded at runtime by sending `SIGHUP` to the Prometheus
process. The changes are only applied if all rule files are well-formatted.

## Syntax-checking rules

To quickly check whether a rule file is syntactically correct without starting
a Prometheus server, install and run Prometheus's `promtool` command-line
utility tool:

```bash
go get github.com/prometheus/prometheus/cmd/promtool
promtool check-rules /path/to/example.rules
```

When the file is syntactically valid, the checker prints a textual
representation of the parsed rules to standard output and then exits with
a `0` return status.

If there are any syntax errors, it prints an error message to standard error
and exits with a `1` return status. On invalid input arguments the exit status
is `2`.

## Recording rules

Recording rules allow you to precompute frequently needed or computationally
expensive expressions and save their result as a new set of time series.
Querying the precomputed result will then often be much faster than executing
the original expression every time it is needed. This is especially useful for
dashboards, which need to query the same expression repeatedly every time they
refresh.

To add a new recording rule, add a line of the following syntax to your rule
file:

    <new time series name>[{<label overrides>}] = <expression to record>

Some examples:

    # Saving the per-job HTTP in-progress request count as a new set of time series:
    job:http_inprogress_requests:sum = sum(http_inprogress_requests) by (job)

    # Drop or rewrite labels in the result time series:
    new_time_series{label_to_change="new_value",label_to_drop=""} = old_time_series

Recording rules are evaluated at the interval specified by the
`evaluation_interval` field in the Prometheus configuration. During each
evaluation cycle, the right-hand-side expression of the rule statement is
evaluated at the current instant in time and the resulting sample vector is
stored as a new set of time series with the current timestamp and a new metric
name (and perhaps an overridden set of labels).
