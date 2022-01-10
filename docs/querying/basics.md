---
title: Querying basics
nav_title: Basics
sort_rank: 1
---

# Querying Prometheus

Prometheus provides a functional query language called PromQL (Prometheus Query
Language) that lets the user select and aggregate time series data in real
time. The result of an expression can either be shown as a graph, viewed as
tabular data in Prometheus's expression browser, or consumed by external
systems via the [HTTP API](api.md).

## Examples

This document is meant as a reference. For learning, it might be easier to
start with a couple of [examples](examples.md).

## Expression language data types

In Prometheus's expression language, an expression or sub-expression can
evaluate to one of four types:

* **Instant vector** - a set of time series containing a single sample for each time series, all sharing the same timestamp
* **Range vector** - a set of time series containing a range of data points over time for each time series
* **Scalar** - a simple numeric floating point value
* **String** - a simple string value; currently unused

Depending on the use-case (e.g. when graphing vs. displaying the output of an
expression), only some of these types are legal as the result from a
user-specified expression. For example, an expression that returns an instant
vector is the only type that can be directly graphed.

## Literals

### String literals

Strings may be specified as literals in single quotes, double quotes or
backticks.

PromQL follows the same [escaping rules as
Go](https://golang.org/ref/spec#String_literals). In single or double quotes a
backslash begins an escape sequence, which may be followed by `a`, `b`, `f`,
`n`, `r`, `t`, `v` or `\`. Specific characters can be provided using octal
(`\nnn`) or hexadecimal (`\xnn`, `\unnnn` and `\Unnnnnnnn`).

No escaping is processed inside backticks. Unlike Go, Prometheus does not discard newlines inside backticks.

Example:

    "this is a string"
    'these are unescaped: \n \\ \t'
    `these are not unescaped: \n ' " \t`

### Float literals

Scalar float values can be written as literal integer or floating-point numbers in the format (whitespace only included for better readability):

    [-+]?(
          [0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?
        | 0[xX][0-9a-fA-F]+
        | [nN][aA][nN]
        | [iI][nN][fF]
    )

Examples:

    23
    -2.43
    3.4e-9
    0x8f
    -Inf
    NaN

## Time series Selectors

### Instant vector selectors

Instant vector selectors allow the selection of a set of time series and a
single sample value for each at a given timestamp (instant): in the simplest
form, only a metric name is specified. This results in an instant vector
containing elements for all time series that have this metric name.

This example selects all time series that have the `http_requests_total` metric
name:

    http_requests_total

It is possible to filter these time series further by appending a comma separated list of label
matchers in curly braces (`{}`).

This example selects only those time series with the `http_requests_total`
metric name that also have the `job` label set to `prometheus` and their
`group` label set to `canary`:

    http_requests_total{job="prometheus",group="canary"}

It is also possible to negatively match a label value, or to match label values
against regular expressions. The following label matching operators exist:

* `=`: Select labels that are exactly equal to the provided string.
* `!=`: Select labels that are not equal to the provided string.
* `=~`: Select labels that regex-match the provided string.
* `!~`: Select labels that do not regex-match the provided string.

Regex matches are fully anchored. A match of `env=~"foo"` is treated as `env=~"^foo$"`.

For example, this selects all `http_requests_total` time series for `staging`,
`testing`, and `development` environments and HTTP methods other than `GET`.

    http_requests_total{environment=~"staging|testing|development",method!="GET"}

Label matchers that match empty label values also select all time series that
do not have the specific label set at all. It is possible to have multiple matchers for the same label name.

Vector selectors must either specify a name or at least one label matcher
that does not match the empty string. The following expression is illegal:

    {job=~".*"} # Bad!

In contrast, these expressions are valid as they both have a selector that does not
match empty label values.

    {job=~".+"}              # Good!
    {job=~".*",method="get"} # Good!

Label matchers can also be applied to metric names by matching against the internal
`__name__` label. For example, the expression `http_requests_total` is equivalent to
`{__name__="http_requests_total"}`. Matchers other than `=` (`!=`, `=~`, `!~`) may also be used.
The following expression selects all metrics that have a name starting with `job:`:

    {__name__=~"job:.*"}

The metric name must not be one of the keywords `bool`, `on`, `ignoring`, `group_left` and `group_right`. The following expression is illegal:

    on{} # Bad!

A workaround for this restriction is to use the `__name__` label:

    {__name__="on"} # Good!

All regular expressions in Prometheus use [RE2
syntax](https://github.com/google/re2/wiki/Syntax).

### Range Vector Selectors

Range vector literals work like instant vector literals, except that they
select a range of samples back from the current instant. Syntactically, a [time
duration](#time-durations) is appended in square brackets (`[]`) at the end of a
vector selector to specify how far back in time values should be fetched for
each resulting range vector element.

In this example, we select all the values we have recorded within the last 5
minutes for all time series that have the metric name `http_requests_total` and
a `job` label set to `prometheus`:

    http_requests_total{job="prometheus"}[5m]

### Time Durations

Time durations are specified as a number, followed immediately by one of the
following units:

* `ms` - milliseconds
* `s` - seconds
* `m` - minutes
* `h` - hours
* `d` - days - assuming a day has always 24h
* `w` - weeks - assuming a week has always 7d
* `y` - years - assuming a year has always 365d

Time durations can be combined, by concatenation. Units must be ordered from the
longest to the shortest. A given unit must only appear once in a time duration.

Here are some examples of valid time durations:

    5h
    1h30m
    5m
    10s

### Offset modifier

The `offset` modifier allows changing the time offset for individual
instant and range vectors in a query.

For example, the following expression returns the value of
`http_requests_total` 5 minutes in the past relative to the current
query evaluation time:

    http_requests_total offset 5m

Note that the `offset` modifier always needs to follow the selector
immediately, i.e. the following would be correct:

    sum(http_requests_total{method="GET"} offset 5m) // GOOD.

While the following would be *incorrect*:

    sum(http_requests_total{method="GET"}) offset 5m // INVALID.

The same works for range vectors. This returns the 5-minute rate that
`http_requests_total` had a week ago:

    rate(http_requests_total[5m] offset 1w)

For comparisons with temporal shifts forward in time, a negative offset
can be specified:

    rate(http_requests_total[5m] offset -1w)

Note that this allows a query to look ahead of its evaluation time.

### @ modifier

The `@` modifier allows changing the evaluation time for individual instant
and range vectors in a query. The time supplied to the `@` modifier
is a unix timestamp and described with a float literal. 

For example, the following expression returns the value of
`http_requests_total` at `2021-01-04T07:40:00+00:00`:

    http_requests_total @ 1609746000

Note that the `@` modifier always needs to follow the selector
immediately, i.e. the following would be correct:

    sum(http_requests_total{method="GET"} @ 1609746000) // GOOD.

While the following would be *incorrect*:

    sum(http_requests_total{method="GET"}) @ 1609746000 // INVALID.

The same works for range vectors. This returns the 5-minute rate that
`http_requests_total` had at `2021-01-04T07:40:00+00:00`:

    rate(http_requests_total[5m] @ 1609746000)

The `@` modifier supports all representation of float literals described
above within the limits of `int64`. It can also be used along
with the `offset` modifier where the offset is applied relative to the `@`
modifier time irrespective of which modifier is written first.
These 2 queries will produce the same result.

    # offset after @
    http_requests_total @ 1609746000 offset 5m
    # offset before @
    http_requests_total offset 5m @ 1609746000

Additionally, `start()` and `end()` can also be used as values for the `@` modifier as special values.

For a range query, they resolve to the start and end of the range query respectively and remain the same for all steps.

For an instant query, `start()` and `end()` both resolve to the evaluation time.

    http_requests_total @ start()
    rate(http_requests_total[5m] @ end())

Note that the `@` modifier allows a query to look ahead of its evaluation time.

## Subquery

Subquery allows you to run an instant query for a given range and resolution. The result of a subquery is a range vector.

Syntax: `<instant_query> '[' <range> ':' [<resolution>] ']' [ @ <float_literal> ] [ offset <duration> ]`

* `<resolution>` is optional. Default is the global evaluation interval.

## Operators

Prometheus supports many binary and aggregation operators. These are described
in detail in the [expression language operators](operators.md) page.

## Functions

Prometheus supports several functions to operate on data. These are described
in detail in the [expression language functions](functions.md) page.

## Comments

PromQL supports line comments that start with `#`. Example:

        # This is a comment

## Gotchas

### Staleness

When queries are run, timestamps at which to sample data are selected
independently of the actual present time series data. This is mainly to support
cases like aggregation (`sum`, `avg`, and so on), where multiple aggregated
time series do not exactly align in time. Because of their independence,
Prometheus needs to assign a value at those timestamps for each relevant time
series. It does so by simply taking the newest sample before this timestamp.

If a target scrape or rule evaluation no longer returns a sample for a time
series that was previously present, that time series will be marked as stale.
If a target is removed, its previously returned time series will be marked as
stale soon afterwards.

If a query is evaluated at a sampling timestamp after a time series is marked
stale, then no value is returned for that time series. If new samples are
subsequently ingested for that time series, they will be returned as normal.

If no sample is found (by default) 5 minutes before a sampling timestamp,
no value is returned for that time series at this point in time. This
effectively means that time series "disappear" from graphs at times where their
latest collected sample is older than 5 minutes or after they are marked stale.

Staleness will not be marked for time series that have timestamps included in
their scrapes. Only the 5 minute threshold will be applied in that case.

### Avoiding slow queries and overloads

If a query needs to operate on a very large amount of data, graphing it might
time out or overload the server or browser. Thus, when constructing queries
over unknown data, always start building the query in the tabular view of
Prometheus's expression browser until the result set seems reasonable
(hundreds, not thousands, of time series at most).  Only when you have filtered
or aggregated your data sufficiently, switch to graph mode. If the expression
still takes too long to graph ad-hoc, pre-record it via a [recording
rule](../configuration/recording_rules.md#recording-rules).

This is especially relevant for Prometheus's query language, where a bare
metric name selector like `api_http_requests_total` could expand to thousands
of time series with different labels. Also keep in mind that expressions which
aggregate over many time series will generate load on the server even if the
output is only a small number of time series. This is similar to how it would
be slow to sum all values of a column in a relational database, even if the
output value is only a single number.
