---
title: Disabled Features
sort_rank: 10
---

# Disabled Features

These features are disabled by default since they are breaking changes or are
considered experimental.  As their behavior may change in future releases,
all feature changes are communicated via the [release
changelog](https://github.com/prometheus/prometheus/blob/master/CHANGELOG.md).

One can enable one or more features using the `--enable-feature` flag with a
single feature or a comma-separated list of features. In future releases,
features may be enabled by default.

## `@` Modifier in PromQL

`--enable-feature=promql-at-modifier`

The `@` modifier lets one specify the evaluation time for instant vector
selectors, range vector selectors, and subqueries. More details can be found
[here](querying/basics.md#-modifier).

## Negative offset in PromQL

`--enable-feature=promql-negative-offset`

In contrast to the positive offset modifier, the negative offset modifier lets
one shift a vector into the past.  An example in which one may want to use a
negative offset is reviewing past data and making temporal comparisons with
more recent data.

More details can be found [here](querying/basics.md#offset-modifier).

## Remote Write Receiver

`--enable-feature=remote-write-receiver`

The remote write receiver allows Prometheus to accept remote write requests from other Prometheus servers. More details can be found [here](storage.md#overview).

