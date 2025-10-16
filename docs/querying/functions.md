---
title: Query functions
nav_title: Functions
sort_rank: 3
---

Some functions have default arguments, e.g. `year(v=vector(time()) instant-vector)`. This means that there is one argument `v` which is an instant
vector, which if not provided it will default to the value of the expression
`vector(time())`.

## `abs()`

`abs(v instant-vector)` returns a vector containing all float samples in the
input vector converted to their absolute value. Histogram samples in the input
vector are ignored silently.

## `absent()`

`absent(v instant-vector)` returns an empty vector if the vector passed to it
has any elements (float samples or histogram samples) and a 1-element vector
with the value 1 if the vector passed to it has no elements.

This is useful for alerting on when no time series exist for a given metric name
and label combination.

```
absent(nonexistent{job="myjob"})
# => {job="myjob"}

absent(nonexistent{job="myjob",instance=~".*"})
# => {job="myjob"}

absent(sum(nonexistent{job="myjob"}))
# => {}
```

In the first two examples, `absent()` tries to be smart about deriving labels
of the 1-element output vector from the input vector.

## `absent_over_time()`

`absent_over_time(v range-vector)` returns an empty vector if the range vector
passed to it has any elements (float samples or histogram samples) and a
1-element vector with the value 1 if the range vector passed to it has no
elements.

This is useful for alerting on when no time series exist for a given metric name
and label combination for a certain amount of time.

```
absent_over_time(nonexistent{job="myjob"}[1h])
# => {job="myjob"}

absent_over_time(nonexistent{job="myjob",instance=~".*"}[1h])
# => {job="myjob"}

absent_over_time(sum(nonexistent{job="myjob"})[1h:])
# => {}
```

In the first two examples, `absent_over_time()` tries to be smart about deriving
labels of the 1-element output vector from the input vector.

## `ceil()`

`ceil(v instant-vector)` returns a vector containing all float samples in the
input vector rounded up to the nearest integer value greater than or equal to
their original value. Histogram samples in the input vector are ignored silently.

* `ceil(+Inf) = +Inf`
* `ceil(±0) = ±0`
* `ceil(1.49) = 2.0`
* `ceil(1.78) = 2.0`

## `changes()`

For each input time series, `changes(v range-vector)` returns the number of
times its value has changed within the provided time range as an instant
vector. A float sample followed by a histogram sample, or vice versa, counts as
a change. A counter histogram sample followed by a gauge histogram sample with
otherwise exactly the same values, or vice versa, does not count as a change.

## `clamp()`

`clamp(v instant-vector, min scalar, max scalar)` clamps the values of all
float samples in `v` to have a lower limit of `min` and an upper limit of
`max`. Histogram samples in the input vector are ignored silently.

Special cases:

* Return an empty vector if `min > max`
* Float samples are clamped to `NaN` if `min` or `max` is `NaN`

## `clamp_max()`

`clamp_max(v instant-vector, max scalar)` clamps the values of all float
samples in `v` to have an upper limit of `max`. Histogram samples in the input
vector are ignored silently.

## `clamp_min()`

`clamp_min(v instant-vector, min scalar)` clamps the values of all float
samples in `v` to have a lower limit of `min`. Histogram samples in the input
vector are ignored silently.

## `day_of_month()`

`day_of_month(v=vector(time()) instant-vector)` interprets float samples in
`v` as timestamps (number of seconds since January 1, 1970 UTC) and returns the
day of the month (in UTC) for each of those timestamps. Returned values are
from 1 to 31. Histogram samples in the input vector are ignored silently.

## `day_of_week()`

`day_of_week(v=vector(time()) instant-vector)` interprets float samples in `v`
as timestamps (number of seconds since January 1, 1970 UTC) and returns the day
of the week (in UTC) for each of those timestamps. Returned values are from 0
to 6, where 0 means Sunday etc. Histogram samples in the input vector are
ignored silently.

## `day_of_year()`

`day_of_year(v=vector(time()) instant-vector)` interprets float samples in `v`
as timestamps (number of seconds since January 1, 1970 UTC) and returns the day
of the year (in UTC) for each of those timestamps. Returned values are from 1
to 365 for non-leap years, and 1 to 366 in leap years. Histogram samples in the
input vector are ignored silently.

## `days_in_month()`

`days_in_month(v=vector(time()) instant-vector)` interprets float samples in
`v` as timestamps (number of seconds since January 1, 1970 UTC) and returns the
number of days in the month of each of those timestamps (in UTC). Returned
values are from 28 to 31. Histogram samples in the input vector are ignored silently.

## `delta()`

`delta(v range-vector)` calculates the difference between the
first and last value of each time series element in a range vector `v`,
returning an instant vector with the given deltas and equivalent labels.
The delta is extrapolated to cover the full time range as specified in
the range vector selector, so that it is possible to get a non-integer
result even if the sample values are all integers.

The following example expression returns the difference in CPU temperature
between now and 2 hours ago:

```
delta(cpu_temp_celsius{host="zeus"}[2h])
```

`delta` acts on histogram samples by calculating a new histogram where each
component (sum and count of observations, buckets) is the difference between
the respective component in the first and last native histogram in `v`.
However, each element in `v` that contains a mix of float samples and histogram
samples within the range will be omitted from the result vector, flagged by a
warn-level annotation.

`delta` should only be used with gauges (for both floats and histograms).

## `deriv()`

`deriv(v range-vector)` calculates the per-second derivative of each float time
series in the range vector `v`, using [simple linear
regression](https://en.wikipedia.org/wiki/Simple_linear_regression). The range
vector must have at least two float samples in order to perform the
calculation. When `+Inf` or `-Inf` are found in the range vector, the slope and
offset value calculated will be `NaN`.

`deriv` should only be used with gauges and only works for float samples.
Elements in the range vector that contain only histogram samples are ignored
entirely. For elements that contain a mix of float and histogram samples, only
the float samples are used as input, which is flagged by an info-level
annotation.

## `double_exponential_smoothing()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`double_exponential_smoothing(v range-vector, sf scalar, tf scalar)` produces a
smoothed value for each float time series in the range in `v`. The lower the
smoothing factor `sf`, the more importance is given to old data. The higher the
trend factor `tf`, the more trends in the data is considered. Both `sf` and
`tf` must be between 0 and 1. For additional details, refer to [NIST
Engineering Statistics
Handbook](https://www.itl.nist.gov/div898/handbook/pmc/section4/pmc433.htm). In
Prometheus V2 this function was called `holt_winters`. This caused confusion
since the Holt-Winters method usually refers to triple exponential smoothing.
Double exponential smoothing as implemented here is also referred to as "Holt
Linear".

`double_exponential_smoothing` should only be used with gauges and only works
for float samples. Elements in the range vector that contain only histogram
samples are ignored entirely. For elements that contain a mix of float and
histogram samples, only the float samples are used as input, which is flagged
by an info-level annotation.

## `exp()`

`exp(v instant-vector)` calculates the exponential function for all float
samples in `v`. Histogram samples are ignored silently. Special cases are:

* `Exp(+Inf) = +Inf`
* `Exp(NaN) = NaN`

## `floor()`

`floor(v instant-vector)` returns a vector containing all float samples in the
input vector rounded down to the nearest integer value smaller than or equal
to their original value. Histogram samples in the input vector are ignored silently.

* `floor(+Inf) = +Inf`
* `floor(±0) = ±0`
* `floor(1.49) = 1.0`
* `floor(1.78) = 1.0`

## `histogram_avg()`

`histogram_avg(v instant-vector)` returns the arithmetic average of observed
values stored in each histogram sample in `v`. Float samples are ignored and do
not show up in the returned vector.

Use `histogram_avg` as demonstrated below to compute the average request duration
over a 5-minute window from a native histogram:

    histogram_avg(rate(http_request_duration_seconds[5m]))

Which is equivalent to the following query:

      histogram_sum(rate(http_request_duration_seconds[5m]))
    /
      histogram_count(rate(http_request_duration_seconds[5m]))

## `histogram_count()` and `histogram_sum()`

`histogram_count(v instant-vector)` returns the count of observations stored in
each histogram sample in `v`. Float samples are ignored and do not show up in
the returned vector.

Similarly, `histogram_sum(v instant-vector)` returns the sum of observations
stored in each histogram sample.

Use `histogram_count` in the following way to calculate a rate of observations
(in this case corresponding to “requests per second”) from a series of
histogram samples:

    histogram_count(rate(http_request_duration_seconds[10m]))

## `histogram_fraction()`

`histogram_fraction(lower scalar, upper scalar, b instant-vector)` returns the
estimated fraction of observations between the provided lower and upper values
for each classic or native histogram contained in `b`. Float samples in `b` are
considered the counts of observations in each bucket of one or more classic
histograms, while native histogram samples in `b` are treated each individually
as a separate histogram. This works in the same way as for `histogram_quantile()`.
(See there for more details.)

If the provided lower and upper values do not coincide with bucket boundaries,
the calculated fraction is an estimate, using the same interpolation method as for
`histogram_quantile()`. (See there for more details.) Especially with classic
histograms, it is easy to accidentally pick lower or upper values that are very
far away from any bucket boundary, leading to large margins of error. Rather than
using `histogram_fraction()` with classic histograms, it is often a more robust approach
to directly act on the bucket series when calculating fractions. See the
[calculation of the Apdex score](https://prometheus.io/docs/practices/histograms/#apdex-score)
as a typical example.

For example, the following expression calculates the fraction of HTTP requests
over the last hour that took 200ms or less:

    histogram_fraction(0, 0.2, rate(http_request_duration_seconds[1h]))

The error of the estimation depends on the resolution of the underlying native
histogram and how closely the provided boundaries are aligned with the bucket
boundaries in the histogram.

`+Inf` and `-Inf` are valid boundary values. For example, if the histogram in
the expression above included negative observations (which shouldn't be the
case for request durations), the appropriate lower boundary to include all
observations less than or equal 0.2 would be `-Inf` rather than `0`.

Whether the provided boundaries are inclusive or exclusive is only relevant if
the provided boundaries are precisely aligned with bucket boundaries in the
underlying native histogram. In this case, the behavior depends on the schema
definition of the histogram. (The usual standard exponential schemas all
feature inclusive upper boundaries and exclusive lower boundaries for positive
values, and vice versa for negative values.) Without a precise alignment of
boundaries, the function uses interpolation to estimate the fraction. With the
resulting uncertainty, it becomes irrelevant if the boundaries are inclusive or
exclusive.

Special case for native histograms with standard exponential buckets:
`NaN` observations are considered outside of any buckets in this case.
`histogram_fraction(-Inf, +Inf, b)` effectively returns the fraction of
non-`NaN` observations and may therefore be less than 1.

## `histogram_quantile()`

`histogram_quantile(φ scalar, b instant-vector)` calculates the φ-quantile (0 ≤
φ ≤ 1) from a [classic
histogram](https://prometheus.io/docs/concepts/metric_types/#histogram) or from
a native histogram. (See [histograms and
summaries](https://prometheus.io/docs/practices/histograms) for a detailed
explanation of φ-quantiles and the usage of the (classic) histogram metric
type in general.)

The float samples in `b` are considered the counts of observations in each
bucket of one or more classic histograms. Each float sample must have a label
`le` where the label value denotes the inclusive upper bound of the bucket.
(Float samples without such a label are silently ignored.) The other labels and
the metric name are used to identify the buckets belonging to each classic
histogram. The [histogram metric
type](https://prometheus.io/docs/concepts/metric_types/#histogram)
automatically provides time series with the `_bucket` suffix and the
appropriate labels.

The (native) histogram samples in `b` are treated each individually as a
separate histogram to calculate the quantile from.

As long as no naming collisions arise, `b` may contain a mix of classic
and native histograms.

Use the `rate()` function to specify the time window for the quantile
calculation.

Example: A histogram metric is called `http_request_duration_seconds` (and
therefore the metric name for the buckets of a classic histogram is
`http_request_duration_seconds_bucket`). To calculate the 90th percentile of request
durations over the last 10m, use the following expression in case
`http_request_duration_seconds` is a classic histogram:

    histogram_quantile(0.9, rate(http_request_duration_seconds_bucket[10m]))

For a native histogram, use the following expression instead:

    histogram_quantile(0.9, rate(http_request_duration_seconds[10m]))

The quantile is calculated for each label combination in
`http_request_duration_seconds`. To aggregate, use the `sum()` aggregator
around the `rate()` function. Since the `le` label is required by
`histogram_quantile()` to deal with classic histograms, it has to be
included in the `by` clause. The following expression aggregates the 90th
percentile by `job` for classic histograms:

    histogram_quantile(0.9, sum by (job, le) (rate(http_request_duration_seconds_bucket[10m])))

When aggregating native histograms, the expression simplifies to:

    histogram_quantile(0.9, sum by (job) (rate(http_request_duration_seconds[10m])))

To aggregate all classic histograms, specify only the `le` label:

    histogram_quantile(0.9, sum by (le) (rate(http_request_duration_seconds_bucket[10m])))

With native histograms, aggregating everything works as usual without any `by` clause:

    histogram_quantile(0.9, sum(rate(http_request_duration_seconds[10m])))

In the (common) case that a quantile value does not coincide with a bucket
boundary, the `histogram_quantile()` function interpolates the quantile value
within the bucket the quantile value falls into. For classic histograms, for
native histograms with custom bucket boundaries, and for the zero bucket of
other native histograms, it assumes a uniform distribution of observations
within the bucket (also called _linear interpolation_). For the
non-zero-buckets of native histograms with a standard exponential bucketing
schema, the interpolation is done under the assumption that the samples within
the bucket are distributed in a way that they would uniformly populate the
buckets in a hypothetical histogram with higher resolution. (This is also
called _exponential interpolation_. See the [native histogram
specification](https://prometheus.io/docs/specs/native_histograms/#interpolation-within-a-bucket)
for more details.)

If `b` has 0 observations, `NaN` is returned. For φ < 0, `-Inf` is
returned. For φ > 1, `+Inf` is returned. For φ = `NaN`, `NaN` is returned.

Special cases for classic histograms:

* If `b` contains fewer than two buckets, `NaN` is returned.
* The highest bucket must have an upper bound of `+Inf`. (Otherwise, `NaN` is
  returned.)
* If a quantile is located in the highest bucket, the upper bound of the second
  highest bucket is returned.
* The lower limit of the lowest bucket is assumed to be 0 if the upper bound of
  that bucket is greater than 0. In that case, the usual linear interpolation
  is applied within that bucket. Otherwise, the upper bound of the lowest
  bucket is returned for quantiles located in the lowest bucket.

Special cases for native histograms:

* If a native histogram with standard exponential buckets has `NaN`
  observations and the quantile falls into one of the existing exponential
  buckets, the result is skewed towards higher values due to `NaN`
  observations treated as `+Inf`. This is flagged with an info level
  annotation.
* If a native histogram with standard exponential buckets has `NaN`
  observations and the quantile falls above all of the existing exponential
  buckets, `NaN` is returned. This is flagged with an info level annotation.
* A zero bucket with finite width is assumed to contain no negative
  observations if the histogram has observations in positive buckets, but none
  in negative buckets.
* A zero bucket with finite width is assumed to contain no positive
  observations if the histogram has observations in negative buckets, but none
  in positive buckets.

You can use `histogram_quantile(0, v instant-vector)` to get the estimated
minimum value stored in a histogram.

You can use `histogram_quantile(1, v instant-vector)` to get the estimated
maximum value stored in a histogram.

Buckets of classic histograms are cumulative. Therefore, the following should
always be the case:

* The counts in the buckets are monotonically increasing (strictly
  non-decreasing).
* A lack of observations between the upper limits of two consecutive buckets
  results in equal counts in those two buckets.

However, floating point precision issues (e.g. small discrepancies introduced
by computing of buckets with `sum(rate(...))`) or invalid data might violate
these assumptions. In that case, `histogram_quantile` would be unable to return
meaningful results. To mitigate the issue, `histogram_quantile` assumes that
tiny relative differences between consecutive buckets are happening because of
floating point precision errors and ignores them. (The threshold to ignore a
difference between two buckets is a trillionth (1e-12) of the sum of both
buckets.) Furthermore, if there are non-monotonic bucket counts even after this
adjustment, they are increased to the value of the previous buckets to enforce
monotonicity. The latter is evidence for an actual issue with the input data
and is therefore flagged by an info-level annotation reading `input to
histogram_quantile needed to be fixed for monotonicity`. If you encounter this
annotation, you should find and remove the source of the invalid data.

## `histogram_stddev()` and `histogram_stdvar()`

`histogram_stddev(v instant-vector)` returns the estimated standard deviation
of observations for each histogram sample in `v`. For this estimation, all observations
in a bucket are assumed to have the value of the mean of the bucket boundaries. For
the zero bucket and for buckets with custom boundaries, the arithmetic mean is used.
For the usual exponential buckets, the geometric mean is used. Float samples are ignored
and do not show up in the returned vector.

Similarly, `histogram_stdvar(v instant-vector)` returns the estimated standard
variance of observations for each histogram sample in `v`.

## `hour()`

`hour(v=vector(time()) instant-vector)` interprets float samples in `v` as
timestamps (number of seconds since January 1, 1970 UTC) and returns the hour
of the day (in UTC) for each of those timestamps. Returned values are from 0
to 23. Histogram samples in the input vector are ignored silently.

## `idelta()`

`idelta(v range-vector)` calculates the difference between the last two samples
in the range vector `v`, returning an instant vector with the given deltas and
equivalent labels. Both samples must be either float samples or histogram
samples. Elements in `v` where one of the last two samples is a float sample
and the other is a histogram sample will be omitted from the result vector,
flagged by a warn-level annotation.

`idelta` should only be used with gauges (for both floats and histograms).

## `increase()`

`increase(v range-vector)` calculates the increase in the time series in the
range vector. Breaks in monotonicity (such as counter resets due to target
restarts) are automatically adjusted for. The increase is extrapolated to cover
the full time range as specified in the range vector selector, so that it is
possible to get a non-integer result even if a counter increases only by
integer increments.

The following example expression returns the number of HTTP requests as measured
over the last 5 minutes, per time series in the range vector:

```
increase(http_requests_total{job="api-server"}[5m])
```

`increase` acts on histogram samples by calculating a new histogram where each
component (sum and count of observations, buckets) is the increase between the
respective component in the first and last native histogram in `v`. However,
each element in `v` that contains a mix of float samples and histogram samples
within the range, will be omitted from the result vector, flagged by a
warn-level annotation.

`increase` should only be used with counters (for both floats and histograms).
It is syntactic sugar for `rate(v)` multiplied by the number of seconds under
the specified time range window, and should be used primarily for human
readability. Use `rate` in recording rules so that increases are tracked
consistently on a per-second basis.

## `info()`

_The `info` function is an experiment to improve UX
around including labels from [info metrics](https://grafana.com/blog/2021/08/04/how-to-use-promql-joins-for-more-effective-queries-of-prometheus-metrics-at-scale/#info-metrics).
The behavior of this function may change in future versions of Prometheus,
including its removal from PromQL. `info` has to be enabled via the
[feature flag](../feature_flags.md#experimental-promql-functions) `--enable-feature=promql-experimental-functions`._

`info(v instant-vector, [data-label-selector instant-vector])` finds, for each time
series in `v`, all info series with matching _identifying_ labels (more on
this later), and adds the union of their _data_ (i.e., non-identifying) labels
to the time series. The second argument `data-label-selector` is optional.
It is not a real instant vector, but uses a subset of its syntax.
It must start and end with curly braces (`{ ... }`) and may only contain label matchers.
The label matchers are used to constrain which info series to consider
and which data labels to add to `v`.

Identifying labels of an info series are the subset of labels that uniquely
identify the info series. The remaining labels are considered
_data labels_ (also called non-identifying). (Note that Prometheus's concept
of time series identity always includes _all_ the labels. For the sake of the `info`
function, we “logically” define info series identity in a different way than
in the conventional Prometheus view.) The identifying labels of an info series
are used to join it to regular (non-info) series, i.e. those series that have
the same labels as the identifying labels of the info series. The data labels, which are
the ones added to the regular series by the `info` function, effectively encode
metadata key value pairs. (This implies that a change in the data labels
in the conventional Prometheus view constitutes the end of one info series and
the beginning of a new info series, while the “logical” view of the `info` function is
that the same info series continues to exist, just with different “data”.)

The conventional approach of adding data labels is sometimes called a “join query”,
as illustrated by the following example:

```
  rate(http_server_request_duration_seconds_count[2m])
* on (job, instance) group_left (k8s_cluster_name)
  target_info
```

The core of the query is the expression `rate(http_server_request_duration_seconds_count[2m])`.
But to add data labels from an info metric, the user has to use elaborate
(and not very obvious) syntax to specify which info metric to use (`target_info`), what the
identifying labels are (`on (job, instance)`), and which data labels to add
(`group_left (k8s_cluster_name)`).

This query is not only verbose and hard to write, it might also run into an “identity crisis”:
If any of the data labels of `target_info` changes, Prometheus sees that as a change of series
(as alluded to above, Prometheus just has no native concept of non-identifying labels).
If the old `target_info` series is not properly marked as stale (which can happen with certain ingestion paths),
the query above will fail for up to 5m (the lookback delta) because it will find a conflicting
match with both the old and the new version of `target_info`.

The `info` function not only resolves this conflict in favor of the newer series, it also simplifies the syntax
because it knows about the available info series and what their identifying labels are. The example query
looks like this with the `info` function:

```
info(
  rate(http_server_request_duration_seconds_count[2m]),
  {k8s_cluster_name=~".+"}
)
```

The common case of adding _all_ data labels can be achieved by
omitting the 2nd argument of the `info` function entirely, simplifying
the example even more:

```
info(rate(http_server_request_duration_seconds_count[2m]))
```

While `info` normally automatically finds all matching info series, it's possible to
restrict them by providing a `__name__` label matcher, e.g.
`{__name__="target_info"}`.

### Limitations

In its current iteration, `info` defaults to considering only info series with
the name `target_info`. It also assumes that the identifying info series labels are
`instance` and `job`. `info` does support other info series names however, through
`__name__` label matchers. E.g., one can explicitly say to consider both
`target_info` and `build_info` as follows:
`{__name__=~"(target|build)_info"}`. However, the identifying labels always
have to be `instance` and `job`.

These limitations are partially defeating the purpose of the `info` function.
At the current stage, this is an experiment to find out how useful the approach
turns out to be in practice. A final version of the `info` function will indeed
consider all matching info series and with their appropriate identifying labels.

## `irate()`

`irate(v range-vector)` calculates the per-second instant rate of increase of
the time series in the range vector. This is based on the last two data points.
Breaks in monotonicity (such as counter resets due to target restarts) are
automatically adjusted for. Both samples must be either float samples or
histogram samples. Elements in `v` where one of the last two samples is a float
sample and the other is a histogram sample will be omitted from the result
vector, flagged by a warn-level annotation.

`irate` should only be used with counters (for both floats and histograms).

The following example expression returns the per-second rate of HTTP requests
looking up to 5 minutes back for the two most recent data points, per time
series in the range vector:

```
irate(http_requests_total{job="api-server"}[5m])
```

`irate` should only be used when graphing volatile, fast-moving counters.
Use `rate` for alerts and slow-moving counters, as brief changes
in the rate can reset the `FOR` clause and graphs consisting entirely of rare
spikes are hard to read.

Note that when combining `irate()` with an
[aggregation operator](operators.md#aggregation-operators) (e.g. `sum()`)
or a function aggregating over time (any function ending in `_over_time`),
always take an `irate()` first, then aggregate. Otherwise `irate()` cannot detect
counter resets when your target restarts.

## `label_join()`

For each timeseries in `v`, `label_join(v instant-vector, dst_label string, separator string, src_label_1 string, src_label_2 string, ...)` joins all the values of all the `src_labels`
using `separator` and returns the timeseries with the label `dst_label` containing the joined value.
There can be any number of `src_labels` in this function.

`label_join` acts on float and histogram samples in the same way.

This example will return a vector with each time series having a `foo` label with the value `a,b,c` added to it:

```
label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3")
```

## `label_replace()`

For each timeseries in `v`, `label_replace(v instant-vector, dst_label string, replacement string, src_label string, regex string)`
matches the [regular expression](./basics.md#regular-expressions) `regex` against the value of the label `src_label`. If it
matches, the value of the label `dst_label` in the returned timeseries will be the expansion
of `replacement`, together with the original labels in the input. Capturing groups in the
regular expression can be referenced with `$1`, `$2`, etc. Named capturing groups in the regular expression can be referenced with `$name` (where `name` is the  capturing group name). If the regular expression doesn't match then the timeseries is returned unchanged.

`label_replace` acts on float and histogram samples in the same way.

This example will return timeseries with the values `a:c` at label `service` and `a` at label `foo`:

```
label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*")
```

This second example has the same effect than the first example, and illustrates use of named capturing groups:
```
label_replace(up{job="api-server",service="a:c"}, "foo", "$name", "service", "(?P<name>.*):(?P<version>.*)")
```

## `ln()`

`ln(v instant-vector)` calculates the natural logarithm for all float samples
in `v`. Histogram samples in the input vector are ignored silently. Special cases are:

* `ln(+Inf) = +Inf`
* `ln(0) = -Inf`
* `ln(x < 0) = NaN`
* `ln(NaN) = NaN`

## `log2()`

`log2(v instant-vector)` calculates the binary logarithm for all float samples
in `v`. Histogram samples in the input vector are ignored silently. The special cases
are equivalent to those in `ln`.

## `log10()`

`log10(v instant-vector)` calculates the decimal logarithm for all float
samples in `v`. Histogram samples in the input vector are ignored silently. The special
cases are equivalent to those in `ln`.

## `minute()`

`minute(v=vector(time()) instant-vector)` interprets float samples in `v` as
timestamps (number of seconds since January 1, 1970 UTC) and returns the minute
of the hour (in UTC) for each of those timestamps. Returned values are from 0
to 59. Histogram samples in the input vector are ignored silently.

## `month()`

`month(v=vector(time()) instant-vector)` interprets float samples in `v` as
timestamps (number of seconds since January 1, 1970 UTC) and returns the month
of the year (in UTC) for each of those timestamps. Returned values are from 1
to 12, where 1 means January etc. Histogram samples in the input vector are
ignored silently.

## `predict_linear()`

`predict_linear(v range-vector, t scalar)` predicts the value of time series
`t` seconds from now, based on the range vector `v`, using [simple linear
regression](https://en.wikipedia.org/wiki/Simple_linear_regression). The range
vector must have at least two float samples in order to perform the
calculation. When `+Inf` or `-Inf` are found in the range vector, the predicted
value will be `NaN`.

`predict_linear` should only be used with gauges and only works for float
samples. Elements in the range vector that contain only histogram samples are
ignored entirely. For elements that contain a mix of float and histogram
samples, only the float samples are used as input, which is flagged by an
info-level annotation.

## `rate()`

`rate(v range-vector)` calculates the per-second average rate of increase of the
time series in the range vector. Breaks in monotonicity (such as counter
resets due to target restarts) are automatically adjusted for. Also, the
calculation extrapolates to the ends of the time range, allowing for missed
scrapes or imperfect alignment of scrape cycles with the range's time period.

The following example expression returns the per-second average rate of HTTP requests
over the last 5 minutes, per time series in the range vector:

```
rate(http_requests_total{job="api-server"}[5m])
```

`rate` acts on native histograms by calculating a new histogram where each
component (sum and count of observations, buckets) is the rate of increase
between the respective component in the first and last native histogram in `v`.
However, each element in `v` that contains a mix of float and native histogram
samples within the range, will be omitted from the result vector, flagged by a
warn-level annotation.

`rate` should only be used with counters (for both floats and histograms). It
is best suited for alerting, and for graphing of slow-moving counters.

Note that when combining `rate()` with an aggregation operator (e.g. `sum()`)
or a function aggregating over time (any function ending in `_over_time`),
always take a `rate()` first, then aggregate. Otherwise `rate()` cannot detect
counter resets when your target restarts.

## `resets()`

For each input time series, `resets(v range-vector)` returns the number of
counter resets within the provided time range as an instant vector. Any
decrease in the value between two consecutive float samples is interpreted as a
counter reset. A reset in a native histogram is detected in a more complex way:
Any decrease in any bucket, including the zero bucket, or in the count of
observation constitutes a counter reset, but also the disappearance of any
previously populated bucket, a decrease of the zero-bucket width, or any schema
change that is not a compatible decrease of resolution.

`resets` should only be used with counters (for both floats and histograms).

A float sample followed by a histogram sample, or vice versa, counts as a
reset. A counter histogram sample followed by a gauge histogram sample, or vice
versa, also counts as a reset (but note that `resets` should not be used on
gauges in the first place, see above).

## `round()`

`round(v instant-vector, to_nearest=1 scalar)` rounds the sample values of all
elements in `v` to the nearest integer. Ties are resolved by rounding up. The
optional `to_nearest` argument allows specifying the nearest multiple to which
the sample values should be rounded. This multiple may also be a fraction.
Histogram samples in the input vector are ignored silently.

## `scalar()`

Given an input vector that contains only one element with a float sample,
`scalar(v instant-vector)` returns the sample value of that float sample as a
scalar. If the input vector does not have exactly one element with a float
sample, `scalar` will return `NaN`. Histogram samples in the input vector are
ignored silently.

## `sgn()`

`sgn(v instant-vector)` returns a vector with all float sample values converted
to their sign, defined as this: 1 if v is positive, -1 if v is negative and 0
if v is equal to zero. Histogram samples in the input vector are ignored silently.

## `sort()`

`sort(v instant-vector)` returns vector elements sorted by their float sample
values, in ascending order. Histogram samples in the input vector are ignored silently.

Please note that `sort` only affects the results of instant queries, as range
query results always have a fixed output ordering.

## `sort_desc()`

Same as `sort`, but sorts in descending order.

## `sort_by_label()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`sort_by_label(v instant-vector, label string, ...)` returns vector elements
sorted by the values of the given labels in ascending order. In case these
label values are equal, elements are sorted by their full label sets.
`sort_by_label` acts on float and histogram samples in the same way.

Please note that `sort_by_label` only affects the results of instant queries, as
range query results always have a fixed output ordering.

`sort_by_label` uses [natural sort
order](https://en.wikipedia.org/wiki/Natural_sort_order).

## `sort_by_label_desc()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

Same as `sort_by_label`, but sorts in descending order.

## `sqrt()`

`sqrt(v instant-vector)` calculates the square root of all float samples in
`v`. Histogram samples in the input vector are ignored silently.

## `time()`

`time()` returns the number of seconds since January 1, 1970 UTC. Note that
this does not actually return the current time, but the time at which the
expression is to be evaluated.

## `timestamp()`

`timestamp(v instant-vector)` returns the timestamp of each of the samples of
the given vector as the number of seconds since January 1, 1970 UTC. It acts on
float and histogram samples in the same way.

## `vector()`

`vector(s scalar)` converts the scalar `s` to a float sample and returns it as
a single-element instant vector with no labels.

## `year()`

`year(v=vector(time()) instant-vector)` returns the year for each of the given
times in UTC. Histogram samples in the input vector are ignored silently.

## `<aggregation>_over_time()`

The following functions allow aggregating each series of a given range vector
over time and return an instant vector with per-series aggregation results:

* `avg_over_time(range-vector)`: the average value of all float or histogram samples in the specified interval (see details below).
* `min_over_time(range-vector)`: the minimum value of all float samples in the specified interval.
* `max_over_time(range-vector)`: the maximum value of all float samples in the specified interval.
* `sum_over_time(range-vector)`: the sum of all float or histogram samples in the specified interval (see details below).
* `count_over_time(range-vector)`: the count of all samples in the specified interval.
* `quantile_over_time(scalar, range-vector)`: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the specified interval.
* `stddev_over_time(range-vector)`: the population standard deviation of all float samples in the specified interval.
* `stdvar_over_time(range-vector)`: the population standard variance of all float samples in the specified interval.
* `last_over_time(range-vector)`: the most recent sample in the specified interval.
* `present_over_time(range-vector)`: the value 1 for any series in the specified interval.

If the [feature flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions` is set, the following
additional functions are available:

* `mad_over_time(range-vector)`: the median absolute deviation of all float
  samples in the specified interval.
* `ts_of_min_over_time(range-vector)`: the timestamp of the last float sample
  that has the minimum value of all float samples in the specified interval.
* `ts_of_max_over_time(range-vector)`: the timestamp of the last float sample
  that has the maximum value of all float samples in the specified interval.
* `ts_of_last_over_time(range-vector)`: the timestamp of last sample in the
  specified interval.
* `first_over_time(range-vector)`: the oldest sample in the specified interval.
* `ts_of_first_over_time(range-vector)`: the timestamp of earliest sample in the
  specified interval.

Note that all values in the specified interval have the same weight in the
aggregation even if the values are not equally spaced throughout the interval.

These functions act on histograms in the following way:

- `count_over_time`, `first_over_time`, `last_over_time`, and
  `present_over_time()` act on float and histogram samples in the same way.
- `avg_over_time()` and `sum_over_time()` act on histogram samples in a way
  that corresponds to the respective aggregation operators. If a series
  contains a mix of float samples and histogram samples within the range, the
  corresponding result is removed entirely from the output vector. Such a
  removal is flagged by a warn-level annotation.
- All other functions ignore histogram samples in the following way: Input
  ranges containing only histogram samples are silently removed from the
  output. For ranges with a mix of histogram and float samples, only the float
  samples are processed and the omission of the histogram samples is flagged by
  an info-level annotation.

`first_over_time(m[1m])` differs from `m offset 1m` in that the former will
select the first sample of `m` _within_ the 1m range, where `m offset 1m` will
select the most recent sample within the lookback interval _outside and prior
to_ the 1m offset. This is particularly useful with `first_over_time(m[step()])`
in range queries (available when `--enable-feature=promql-duration-expr` is set)
to ensure that the sample selected is within the range step.

## Trigonometric Functions

The trigonometric functions work in radians. They ignore histogram samples in
the input vector.

* `acos(v instant-vector)`: calculates the arccosine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Acos)).
* `acosh(v instant-vector)`: calculates the inverse hyperbolic cosine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Acosh)).
* `asin(v instant-vector)`: calculates the arcsine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Asin)).
* `asinh(v instant-vector)`: calculates the inverse hyperbolic sine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Asinh)).
* `atan(v instant-vector)`: calculates the arctangent of all float samples in `v` ([special cases](https://pkg.go.dev/math#Atan)).
* `atanh(v instant-vector)`: calculates the inverse hyperbolic tangent of all float samples in `v` ([special cases](https://pkg.go.dev/math#Atanh)).
* `cos(v instant-vector)`: calculates the cosine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Cos)).
* `cosh(v instant-vector)`: calculates the hyperbolic cosine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Cosh)).
* `sin(v instant-vector)`: calculates the sine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Sin)).
* `sinh(v instant-vector)`: calculates the hyperbolic sine of all float samples in `v` ([special cases](https://pkg.go.dev/math#Sinh)).
* `tan(v instant-vector)`: calculates the tangent of all float samples in `v` ([special cases](https://pkg.go.dev/math#Tan)).
* `tanh(v instant-vector)`: calculates the hyperbolic tangent of all float samples in `v` ([special cases](https://pkg.go.dev/math#Tanh)).

The following are useful for converting between degrees and radians:

* `deg(v instant-vector)`: converts radians to degrees for all float samples in `v`.
* `pi()`: returns pi.
* `rad(v instant-vector)`: converts degrees to radians for all float samples in `v`.
