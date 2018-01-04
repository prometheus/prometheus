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
Rule files use YAML.

The rule files can be reloaded at runtime by sending `SIGHUP` to the Prometheus
process. The changes are only applied if all rule files are well-formatted.

## Syntax-checking rules

To quickly check whether a rule file is syntactically correct without starting
a Prometheus server, install and run Prometheus's `promtool` command-line
utility tool:

```bash
go get github.com/prometheus/prometheus/cmd/promtool
promtool check rules /path/to/example.rules.yml
```

When the file is syntactically valid, the checker prints a textual
representation of the parsed rules to standard output and then exits with
a `0` return status.

If there are any syntax errors or invalid input arguments, it prints an error 
message to standard error and exits with a `1` return status.

## Recording rules

Recording rules allow you to precompute frequently needed or computationally
expensive expressions and save their result as a new set of time series.
Querying the precomputed result will then often be much faster than executing
the original expression every time it is needed. This is especially useful for
dashboards, which need to query the same expression repeatedly every time they
refresh.

Recording and alerting rules exist in a rule group. Rules within a group are
run sequentially at a regular interval.

The syntax of a rule file is:

```yaml
groups:
  [ - <rule_group> ]
```

A simple example rules file would be:

```yaml
groups:
  - name: example
    rules:
    - record: job:http_inprogress_requests:sum
      expr: sum(http_inprogress_requests) by (job)
```

### `<rule_group>`
```
# The name of the group. Must be unique within a file.
name: <string>

# How often rules in the group are evaluated.
[ interval: <duration> | default = global.evaluation_interval ]

rules:
  [ - <rule> ... ]
```

### `<rule>`

The syntax for recording rules is:

```
# The name of the time series to output to. Must be a valid metric name.
record: <string>

# The PromQL expression to evaluate. Every evaluation cycle this is
# evaluated at the current time, and the result recorded as a new set of
# time series with the metric name as given by 'record'.
expr: <string>

# Labels to add or overwrite before storing the result.
labels:
  [ <labelname>: <labelvalue> ]
```

The syntax for alerting rules is:

```
# The name of the alert. Must be a valid metric name.
alert: <string>

# The PromQL expression to evaluate. Every evaluation cycle this is
# evaluated at the current time, and all resultant time series become
# pending/firing alerts.
expr: <string>

# Alerts are considered firing once they have been returned for this long.
# Alerts which have not yet fired for long enough are considered pending.
[ for: <duration> | default = 0s ]

# Labels to add or overwrite for each alert.
labels:
  [ <labelname>: <tmpl_string> ]

# Annotations to add to each alert.
annotations:
  [ <labelname>: <tmpl_string> ]
```

