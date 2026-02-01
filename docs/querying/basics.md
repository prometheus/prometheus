---
title: Querying basics
nav_title: Basics
sort_rank: 1
---

Prometheus provides a functional query language called PromQL (Prometheus Query
Language) that lets the user select and aggregate time series data in real
time.

When you send a query request to Prometheus, it can be an _instant query_, evaluated at one point in time,
or a _range query_ at equally-spaced steps between a start and an end time. PromQL works exactly the same
in each case; the range query is just like an instant query run multiple times at different timestamps.

In the Prometheus UI, the "Table" tab is for instant queries and the "Graph" tab is for range queries.

Other programs can fetch the result of a PromQL expression via the [HTTP API](api.md).

## Examples

This document is a Prometheus basic language reference. For learning, it may be easier to
start with a couple of [examples](examples.md).

## Samples

The value of a sample at a given timestamp returned by PromQL may be a float or
a [native histogram](https://prometheus.io/docs/specs/native_histograms). A
float sample is a simple floating point number, whereas a native histograms
sample contains a full histogram including count, sum, and buckets.

Note that the term “histogram sample” in the PromQL documentation always refers
to a native histogram. The term "classic histogram" refers to a set of time
series containing float samples with the `_bucket`, `_count`, and `_sum` 
suffixes that together describe a histogram. From the perspective of PromQL,
these contain just float samples, there are no “classic histogram samples”.

Both float samples and histogram samples can have a counter or a gauge “flavor”.
Float samples with a counter or gauge flavor are generally simply called
“counters” or “gauges”, respectively, while their histogram counterparts are
called “counter histograms” or “gauge histograms”. Float samples do not store
their flavor, leaving it to the user to take their flavor into account when
writing PromQL queries. (By convention, time series containing float counters
have a name ending on `_total` to help with the distinction.)

Since histogram samples “know” their counter or gauge flavor, this allows
reliable warnings about mismatched operations. For example, applying the `rate`
function to gauge floats will most likely produce a
nonsensical result, but the query will be processed without complains. However,
if applied to gauge histograms, the result of the query will be
annotated with a warning.

## Expression language data types

In Prometheus's expression language, an expression or sub-expression can
evaluate to one of four types:

* **Instant vector** - a set of time series containing a single sample for each time series, all sharing the same timestamp
* **Range vector** - a set of time series containing a range of data points over time for each time series
* **Scalar** - a simple numeric floating point value
* **String** - a simple string value; currently unused

Depending on the use case (e.g. when graphing vs. displaying the output of an
expression), only some of these types are legal as the result of a
user-specified expression.
For [instant queries](api.md#instant-queries), any of the above data types are allowed as the root of the expression.
[Range queries](api.md#range-queries) only support scalar-typed and instant-vector-typed expressions.

Both vectors and time series may contain a mix of float samples and histogram
samples.

## Reconciliation of histogram bucket layouts

Native histograms can have different bucket layouts, but they are generally
convertible to compatible versions to apply binary and aggregation operations
to them. Functions acting on range vectors that are applicable to native
histograms also perform such reconciliation. In binary operations this
reconciliation is performed pairwise, in aggregation operations and functions
all histogram samples are reconciled to one compatible bucket layout.

Not all bucket layouts can be reconciled, if incompatible histograms are
encountered in an operation, the corresponding output vector element is removed
from the result, flagged with a warn-level annotation.
More details can be found in the
[native histogram specification](https://prometheus.io/docs/specs/native_histograms/#compatibility-between-histograms).

## Literals

The following section describes literal values of various kinds.
Note that there is no “histogram literal”.

### String literals

String literals are designated by single quotes, double quotes or backticks.

PromQL follows the same [escaping rules as
Go](https://golang.org/ref/spec#String_literals). For string literals in single or double quotes, a
backslash begins an escape sequence, which may be followed by `a`, `b`, `f`,
`n`, `r`, `t`, `v` or `\`.  Specific characters can be provided using octal
(`\nnn`) or hexadecimal (`\xnn`, `\unnnn` and `\Unnnnnnnn`) notations.

Conversely, escape characters are not parsed in string literals designated by backticks. It is important to note that, unlike Go, Prometheus does not discard newlines inside backticks.

Example:

    "this is a string"
    'these are unescaped: \n \\ \t'
    `these are not unescaped: \n ' " \t`

### Float literals and time durations

Scalar float values can be written as literal integer or floating-point numbers
in the format (whitespace only included for better readability):

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

Additionally, underscores (`_`) can be used in between decimal or hexadecimal
digits to improve readability.

Examples:

    1_000_000
    .123_456_789
    0x_53_AB_F3_82

Float literals are also used to specify durations in seconds. For convenience,
decimal integer numbers may be combined with the following
time units:

* `ms` – milliseconds
* `s` – seconds – 1s equals 1000ms
* `m` – minutes – 1m equals 60s (ignoring leap seconds)
* `h` – hours – 1h equals 60m
* `d` – days – 1d equals 24h (ignoring so-called daylight saving time)
* `w` – weeks – 1w equals 7d
* `y` – years – 1y equals 365d (ignoring leap days)

Suffixing a decimal integer number with one of the units above is a different
representation of the equivalent number of seconds as a bare float literal.

Examples:

    1s # Equivalent to 1.
    2m # Equivalent to 120.
    1ms # Equivalent to 0.001.
    -2h # Equivalent to -7200.

The following examples do _not_ work:

    0xABm # No suffixing of hexadecimal numbers.
    1.5h # Time units cannot be combined with a floating point.
    +Infd # No suffixing of ±Inf or NaN.

Multiple units can be combined by concatenation of suffixed integers. Units
must be ordered from the longest to the shortest. A given unit must only appear
once per float literal.

Examples:

    1h30m # Equivalent to 5400s and thus 5400.
    12h34m56s # Equivalent to 45296s and thus 45296.
    54s321ms # Equivalent to 54.321.

## Time series selectors

These are the basic building-blocks that instruct PromQL what data to fetch.

### Instant vector selectors

Instant vector selectors allow the selection of a set of time series and a
single sample value for each at a given timestamp (point in time).  In the simplest
form, only a metric name is specified, which results in an instant vector
containing elements for all time series that have this metric name.

The value returned will be that of the most recent sample at or before the
query's evaluation timestamp (in the case of an
[instant query](api.md#instant-queries))
or the current step within the query (in the case of a
[range query](api.md#range-queries)).
The [`@` modifier](#modifier) allows overriding the timestamp relative to which
the selection takes place. Time series are only returned if their most recent sample is less than the [lookback period](#staleness) ago.

This example selects all time series that have the `http_requests_total` metric
name, returning the most recent sample for each:

    http_requests_total

It is possible to filter these time series further by appending a comma-separated list of label
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

[Regex](#regular-expressions) matches are fully anchored. A match of `env=~"foo"` is treated as `env=~"^foo$"`.

For example, this selects all `http_requests_total` time series for `staging`,
`testing`, and `development` environments and HTTP methods other than `GET`.

    http_requests_total{environment=~"staging|testing|development",method!="GET"}

Label matchers that match empty label values also select all time series that
do not have the specific label set at all. It is possible to have multiple matchers for the same label name.

For example, given the dataset:

    http_requests_total
    http_requests_total{replica="rep-a"}
    http_requests_total{replica="rep-b"}
    http_requests_total{environment="development"}

The query `http_requests_total{environment=""}` would match and return:

    http_requests_total
    http_requests_total{replica="rep-a"}
    http_requests_total{replica="rep-b"}

and would exclude:

    http_requests_total{environment="development"}

Multiple matchers can be used for the same label name; they all must pass for a result to be returned.

The query:

    http_requests_total{replica!="rep-a",replica=~"rep.*"}

Would then match:

    http_requests_total{replica="rep-b"}

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

### Range Vector Selectors

Range vector literals work like instant vector literals, except that they
select a range of samples back from the current instant. Syntactically, a
[float literal](#float-literals-and-time-durations) is appended in square
brackets (`[]`) at the end of a vector selector to specify for how many seconds
back in time values should be fetched for each resulting range vector element.
Commonly, the float literal uses the syntax with one or more time units, e.g.
`[5m]`. The range is a left-open and right-closed interval, i.e. samples with
timestamps coinciding with the left boundary of the range are excluded from the
selection, while samples coinciding with the right boundary of the range are
included in the selection.

In this example, we select all the values recorded less than 5m ago for all
time series that have the metric name `http_requests_total` and a `job` label
set to `prometheus`:

    http_requests_total{job="prometheus"}[5m]

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

The same works for range vectors. This returns the 5-minute [rate](./functions.md#rate)
that `http_requests_total` had a week ago:

    rate(http_requests_total[5m] offset 1w)

When querying for samples in the past, a negative offset will enable temporal comparisons forward in time:

    rate(http_requests_total[5m] offset -1w)

Note that this allows a query to look ahead of its evaluation time.

### @ modifier

The `@` modifier allows changing the evaluation time for individual instant
and range vectors in a query. The time supplied to the `@` modifier
is a Unix timestamp and described with a float literal.

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

The `@` modifier supports all representations of numeric literals described above.
It works with the `offset` modifier where the offset is applied relative to the `@`
modifier time.  The results are the same irrespective of the order of the modifiers.

For example, these two queries will produce the same result:

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

Syntax: `<instant_query> '[' <range> ':' [<resolution>] ']' [ @ <float_literal> ] [ offset <float_literal> ]`

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

## Regular expressions

All regular expressions in Prometheus use [RE2 syntax](https://github.com/google/re2/wiki/Syntax).

Regex matches are always fully anchored.

## Gotchas

### Staleness

The timestamps at which to sample data, during a query, are selected
independently of the actual present time series data. This is mainly to support
cases like aggregation (`sum`, `avg`, and so on), where multiple aggregated
time series do not precisely align in time. Because of their independence,
Prometheus needs to assign a value at those timestamps for each relevant time
series. It does so by taking the newest sample that is less than the lookback period ago.
The lookback period is 5 minutes by default, but can be
[set with the `--query.lookback-delta` flag](../command-line/prometheus.md)
or overridden on an individual query via the `lookback_delta` parameter.

If a target scrape or rule evaluation no longer returns a sample for a time
series that was previously present, this time series will be marked as stale.
If a target is removed, the previously retrieved time series will be marked as
stale soon after removal.

If a query is evaluated at a sampling timestamp after a time series is marked
as stale, then no value is returned for that time series. If new samples are
subsequently ingested for that time series, they will be returned as expected.

A time series will go stale when it is no longer exported, or the target no
longer exists. Such time series will disappear from graphs
at the times of their latest collected sample, and they will not be returned
in queries after they are marked stale.

Some exporters, which put their own timestamps on samples, get a different behaviour:
series that stop being exported take the last value for (by default) 5 minutes before
disappearing. The `track_timestamps_staleness` setting can change this.

### Avoiding slow queries and overloads

If a query needs to operate on a substantial amount of data, graphing it might
time out or overload the server or browser. Thus, when constructing queries
over unknown data, always start building the query in the tabular view of
Prometheus's expression browser until the result set seems reasonable
(hundreds, not thousands, of time series at most).  Only when you have filtered
or aggregated your data sufficiently, switch to graph mode. If the expression
still takes too long to graph ad-hoc, pre-record it via a [recording
rule](../configuration/recording_rules.md#recording-rules).

This is especially relevant for Prometheus's query language, where a bare
metric name selector like `api_http_requests_total` could expand to thousands
of time series with different labels. Also, keep in mind that expressions that
aggregate over many time series will generate load on the server even if the
output is only a small number of time series. This is similar to how it would
be slow to sum all values of a column in a relational database, even if the
output value is only a single number.
