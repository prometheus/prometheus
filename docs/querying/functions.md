---
title: Query functions
nav_title: Functions
sort_rank: 3
---

# Functions

Some functions have default arguments, e.g. `year(v=vector(time())
instant-vector)`. This means that there is one argument `v` which is an instant
vector, which if not provided it will default to the value of the expression
`vector(time())`.

_Notes about the experimental native histograms:_

* Ingesting native histograms has to be enabled via a [feature
  flag](../feature_flags.md#native-histograms). As long as no native histograms
  have been ingested into the TSDB, all functions will behave as usual.
* Functions that do not explicitly mention native histograms in their
  documentation (see below) will ignore histogram samples.
* Functions that do already act on native histograms might still change their
  behavior in the future.
* If a function requires the same bucket layout between multiple native
  histograms it acts on, it will automatically convert them
  appropriately. (With the currently supported bucket schemas, that's always
  possible.)

## `abs()`

`abs(v instant-vector)` returns the input vector with all sample values converted to
their absolute value.

## `absent()`

`absent(v instant-vector)` returns an empty vector if the vector passed to it
has any elements (floats or native histograms) and a 1-element vector with the
value 1 if the vector passed to it has no elements.

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
passed to it has any elements (floats or native histograms) and a 1-element
vector with the value 1 if the range vector passed to it has no elements.

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

`ceil(v instant-vector)` rounds the sample values of all elements in `v` up to
the nearest integer.

## `changes()`

For each input time series, `changes(v range-vector)` returns the number of
times its value has changed within the provided time range as an instant
vector.

## `clamp()`

`clamp(v instant-vector, min scalar, max scalar)`
clamps the sample values of all elements in `v` to have a lower limit of `min` and an upper limit of `max`.

Special cases:
- Return an empty vector if `min > max`
- Return `NaN` if `min` or `max` is `NaN`

## `clamp_max()`

`clamp_max(v instant-vector, max scalar)` clamps the sample values of all
elements in `v` to have an upper limit of `max`.

## `clamp_min()`

`clamp_min(v instant-vector, min scalar)` clamps the sample values of all
elements in `v` to have a lower limit of `min`.

## `day_of_month()`

`day_of_month(v=vector(time()) instant-vector)` returns the day of the month
for each of the given times in UTC. Returned values are from 1 to 31.

## `day_of_week()`

`day_of_week(v=vector(time()) instant-vector)` returns the day of the week for
each of the given times in UTC. Returned values are from 0 to 6, where 0 means
Sunday etc.

## `day_of_year()`

`day_of_year(v=vector(time()) instant-vector)` returns the day of the year for
each of the given times in UTC. Returned values are from 1 to 365 for non-leap years,
and 1 to 366 in leap years.

## `days_in_month()`

`days_in_month(v=vector(time()) instant-vector)` returns number of days in the
month for each of the given times in UTC. Returned values are from 28 to 31.

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

`delta` acts on native histograms by calculating a new histogram where each
component (sum and count of observations, buckets) is the difference between
the respective component in the first and last native histogram in
`v`. However, each element in `v` that contains a mix of float and native
histogram samples within the range, will be missing from the result vector.

`delta` should only be used with gauges and native histograms where the
components behave like gauges (so-called gauge histograms).

## `deriv()`

`deriv(v range-vector)` calculates the per-second derivative of the time series in a range
vector `v`, using [simple linear regression](https://en.wikipedia.org/wiki/Simple_linear_regression).
The range vector must have at least two samples in order to perform the calculation. When `+Inf` or 
`-Inf` are found in the range vector, the slope and offset value calculated will be `NaN`.

`deriv` should only be used with gauges.

## `exp()`

`exp(v instant-vector)` calculates the exponential function for all elements in `v`.
Special cases are:

* `Exp(+Inf) = +Inf`
* `Exp(NaN) = NaN`

## `floor()`

`floor(v instant-vector)` rounds the sample values of all elements in `v` down
to the nearest integer.

## `histogram_count()` and `histogram_sum()`

_Both functions only act on native histograms, which are an experimental
feature. The behavior of these functions may change in future versions of
Prometheus, including their removal from PromQL._

`histogram_count(v instant-vector)` returns the count of observations stored in
a native histogram. Samples that are not native histograms are ignored and do
not show up in the returned vector.

Similarly, `histogram_sum(v instant-vector)` returns the sum of observations
stored in a native histogram.

Use `histogram_count` in the following way to calculate a rate of observations
(in this case corresponding to “requests per second”) from a native histogram:

    histogram_count(rate(http_request_duration_seconds[10m]))

The additional use of `histogram_sum` enables the calculation of the average of
observed values (in this case corresponding to “average request duration”):

      histogram_sum(rate(http_request_duration_seconds[10m]))
	/
      histogram_count(rate(http_request_duration_seconds[10m]))

## `histogram_fraction()`

_This function only acts on native histograms, which are an experimental
feature. The behavior of this function may change in future versions of
Prometheus, including its removal from PromQL._

For a native histogram, `histogram_fraction(lower scalar, upper scalar, v
instant-vector)` returns the estimated fraction of observations between the
provided lower and upper values. Samples that are not native histograms are
ignored and do not show up in the returned vector.

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
definition of the histogram. The currently supported schemas all feature
inclusive upper boundaries and exclusive lower boundaries for positive values
(and vice versa for negative values). Without a precise alignment of
boundaries, the function uses linear interpolation to estimate the
fraction. With the resulting uncertainty, it becomes irrelevant if the
boundaries are inclusive or exclusive.

## `histogram_quantile()`

`histogram_quantile(φ scalar, b instant-vector)` calculates the φ-quantile (0 ≤
φ ≤ 1) from a [classic
histogram](https://prometheus.io/docs/concepts/metric_types/#histogram) or from
a native histogram. (See [histograms and
summaries](https://prometheus.io/docs/practices/histograms) for a detailed
explanation of φ-quantiles and the usage of the (classic) histogram metric
type in general.)

_Note that native histograms are an experimental feature. The behavior of this
function when dealing with native histograms may change in future versions of
Prometheus._

The float samples in `b` are considered the counts of observations in each
bucket of one or more classic histograms. Each float sample must have a label
`le` where the label value denotes the inclusive upper bound of the bucket.
(Float samples without such a label are silently ignored.) The other labels and
the metric name are used to identify the buckets belonging to each classic
histogram. The [histogram metric
type](https://prometheus.io/docs/concepts/metric_types/#histogram)
automatically provides time series with the `_bucket` suffix and the
appropriate labels.

The native histogram samples in `b` are treated each individually as a separate
histogram to calculate the quantile from.

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

The `histogram_quantile()` function interpolates quantile values by
assuming a linear distribution within a bucket. 

If `b` has 0 observations, `NaN` is returned. For φ < 0, `-Inf` is
returned. For φ > 1, `+Inf` is returned. For φ = `NaN`, `NaN` is returned.

The following is only relevant for classic histograms: If `b` contains
fewer than two buckets, `NaN` is returned. The highest bucket must have an
upper bound of `+Inf`. (Otherwise, `NaN` is returned.) If a quantile is located
in the highest bucket, the upper bound of the second highest bucket is
returned. A lower limit of the lowest bucket is assumed to be 0 if the upper
bound of that bucket is greater than
0. In that case, the usual linear interpolation is applied within that
bucket. Otherwise, the upper bound of the lowest bucket is returned for
quantiles located in the lowest bucket. 

You can use `histogram_quantile(0, v instant-vector)` to get the estimated minimum value stored in
a histogram.

You can use `histogram_quantile(1, v instant-vector)` to get the estimated maximum value stored in
a histogram.

Buckets of classic histograms are cumulative. Therefore, the following should always be the case:

- The counts in the buckets are monotonically increasing (strictly non-decreasing).
- A lack of observations between the upper limits of two consecutive buckets results in equal counts
in those two buckets.

However, floating point precision issues (e.g. small discrepancies introduced by computing of buckets
with `sum(rate(...))`) or invalid data might violate these assumptions. In that case,
`histogram_quantile` would be unable to return meaningful results. To mitigate the issue,
`histogram_quantile` assumes that tiny relative differences between consecutive buckets are happening
because of floating point precision errors and ignores them. (The threshold to ignore a difference
between two buckets is a trillionth (1e-12) of the sum of both buckets.) Furthermore, if there are
non-monotonic bucket counts even after this adjustment, they are increased to the value of the
previous buckets to enforce monotonicity. The latter is evidence for an actual issue with the input
data and is therefore flagged with an informational annotation reading `input to histogram_quantile
needed to be fixed for monotonicity`. If you encounter this annotation, you should find and remove
the source of the invalid data.

## `histogram_stddev()` and `histogram_stdvar()`

_Both functions only act on native histograms, which are an experimental
feature. The behavior of these functions may change in future versions of
Prometheus, including their removal from PromQL._

`histogram_stddev(v instant-vector)` returns the estimated standard deviation
of observations in a native histogram, based on the geometric mean of the buckets
where the observations lie. Samples that are not native histograms are ignored and
do not show up in the returned vector.

Similarly, `histogram_stdvar(v instant-vector)` returns the estimated standard
variance of observations in a native histogram.

## `holt_winters()`

`holt_winters(v range-vector, sf scalar, tf scalar)` produces a smoothed value
for time series based on the range in `v`. The lower the smoothing factor `sf`,
the more importance is given to old data. The higher the trend factor `tf`, the
more trends in the data is considered. Both `sf` and `tf` must be between 0 and
1.

`holt_winters` should only be used with gauges.

## `hour()`

`hour(v=vector(time()) instant-vector)` returns the hour of the day
for each of the given times in UTC. Returned values are from 0 to 23.

## `idelta()`

`idelta(v range-vector)` calculates the difference between the last two samples
in the range vector `v`, returning an instant vector with the given deltas and
equivalent labels.

`idelta` should only be used with gauges.

## `increase()`

`increase(v range-vector)` calculates the increase in the
time series in the range vector. Breaks in monotonicity (such as counter
resets due to target restarts) are automatically adjusted for. The
increase is extrapolated to cover the full time range as specified
in the range vector selector, so that it is possible to get a
non-integer result even if a counter increases only by integer
increments.

The following example expression returns the number of HTTP requests as measured
over the last 5 minutes, per time series in the range vector:

```
increase(http_requests_total{job="api-server"}[5m])
```

`increase` acts on native histograms by calculating a new histogram where each
component (sum and count of observations, buckets) is the increase between
the respective component in the first and last native histogram in
`v`. However, each element in `v` that contains a mix of float and native
histogram samples within the range, will be missing from the result vector.

`increase` should only be used with counters and native histograms where the
components behave like counters. It is syntactic sugar for `rate(v)` multiplied
by the number of seconds under the specified time range window, and should be
used primarily for human readability.  Use `rate` in recording rules so that
increases are tracked consistently on a per-second basis.

## `irate()`

`irate(v range-vector)` calculates the per-second instant rate of increase of
the time series in the range vector. This is based on the last two data points.
Breaks in monotonicity (such as counter resets due to target restarts) are
automatically adjusted for.

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
always take a `irate()` first, then aggregate. Otherwise `irate()` cannot detect
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
matches the [regular expression](https://github.com/google/re2/wiki/Syntax) `regex` against the value of the label `src_label`. If it
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

`ln(v instant-vector)` calculates the natural logarithm for all elements in `v`.
Special cases are:

* `ln(+Inf) = +Inf`
* `ln(0) = -Inf`
* `ln(x < 0) = NaN`
* `ln(NaN) = NaN`

## `log2()`

`log2(v instant-vector)` calculates the binary logarithm for all elements in `v`.
The special cases are equivalent to those in `ln`.

## `log10()`

`log10(v instant-vector)` calculates the decimal logarithm for all elements in `v`.
The special cases are equivalent to those in `ln`.

## `minute()`

`minute(v=vector(time()) instant-vector)` returns the minute of the hour for each
of the given times in UTC. Returned values are from 0 to 59.

## `month()`

`month(v=vector(time()) instant-vector)` returns the month of the year for each
of the given times in UTC. Returned values are from 1 to 12, where 1 means
January etc.

## `predict_linear()`

`predict_linear(v range-vector, t scalar)` predicts the value of time series
`t` seconds from now, based on the range vector `v`, using [simple linear
regression](https://en.wikipedia.org/wiki/Simple_linear_regression).
The range vector must have at least two samples in order to perform the 
calculation. When `+Inf` or `-Inf` are found in the range vector, 
the slope and offset value calculated will be `NaN`.

`predict_linear` should only be used with gauges.

## `rate()`

`rate(v range-vector)` calculates the per-second average rate of increase of the
time series in the range vector. Breaks in monotonicity (such as counter
resets due to target restarts) are automatically adjusted for. Also, the
calculation extrapolates to the ends of the time range, allowing for missed
scrapes or imperfect alignment of scrape cycles with the range's time period.

The following example expression returns the per-second rate of HTTP requests as measured
over the last 5 minutes, per time series in the range vector:

```
rate(http_requests_total{job="api-server"}[5m])
```

`rate` acts on native histograms by calculating a new histogram where each
component (sum and count of observations, buckets) is the rate of increase
between the respective component in the first and last native histogram in
`v`. However, each element in `v` that contains a mix of float and native
histogram samples within the range, will be missing from the result vector.

`rate` should only be used with counters and native histograms where the
components behave like counters. It is best suited for alerting, and for
graphing of slow-moving counters.

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
previously populated bucket, an increase in bucket resolution, or a decrease of
the zero-bucket width.

`resets` should only be used with counters and counter-like native
histograms.

If the range vector contains a mix of float and histogram samples for the same
series, counter resets are detected separately and their numbers added up. The
change from a float to a histogram sample is _not_ considered a counter
reset. Each float sample is compared to the next float sample, and each
histogram is comprared to the next histogram.

## `round()`

`round(v instant-vector, to_nearest=1 scalar)` rounds the sample values of all
elements in `v` to the nearest integer. Ties are resolved by rounding up. The
optional `to_nearest` argument allows specifying the nearest multiple to which
the sample values should be rounded. This multiple may also be a fraction.

## `scalar()`

Given a single-element input vector, `scalar(v instant-vector)` returns the
sample value of that single element as a scalar. If the input vector does not
have exactly one element, `scalar` will return `NaN`.

## `sgn()`

`sgn(v instant-vector)` returns a vector with all sample values converted to their sign, defined as this: 1 if v is positive, -1 if v is negative and 0 if v is equal to zero.

## `sort()`

`sort(v instant-vector)` returns vector elements sorted by their sample values,
in ascending order. Native histograms are sorted by their sum of observations.

## `sort_desc()`

Same as `sort`, but sorts in descending order.

## `sort_by_label()`

**This function has to be enabled via the [feature flag](../feature_flags/) `--enable-feature=promql-experimental-functions`.**

`sort_by_label(v instant-vector, label string, ...)` returns vector elements sorted by their label values and sample value in case of label values being equal, in ascending order.

Please note that the sort by label functions only affect the results of instant queries, as range query results always have a fixed output ordering.

This function uses [natural sort order](https://en.wikipedia.org/wiki/Natural_sort_order).

## `sort_by_label_desc()`

**This function has to be enabled via the [feature flag](../feature_flags/) `--enable-feature=promql-experimental-functions`.**

Same as `sort_by_label`, but sorts in descending order.

Please note that the sort by label functions only affect the results of instant queries, as range query results always have a fixed output ordering.

This function uses [natural sort order](https://en.wikipedia.org/wiki/Natural_sort_order).

## `sqrt()`

`sqrt(v instant-vector)` calculates the square root of all elements in `v`.

## `time()`

`time()` returns the number of seconds since January 1, 1970 UTC. Note that
this does not actually return the current time, but the time at which the
expression is to be evaluated.

## `timestamp()`

`timestamp(v instant-vector)` returns the timestamp of each of the samples of
the given vector as the number of seconds since January 1, 1970 UTC. It also
works with histogram samples.

## `vector()`

`vector(s scalar)` returns the scalar `s` as a vector with no labels.

## `year()`

`year(v=vector(time()) instant-vector)` returns the year
for each of the given times in UTC.

## `<aggregation>_over_time()`

The following functions allow aggregating each series of a given range vector
over time and return an instant vector with per-series aggregation results:

* `avg_over_time(range-vector)`: the average value of all points in the specified interval.
* `min_over_time(range-vector)`: the minimum value of all points in the specified interval.
* `max_over_time(range-vector)`: the maximum value of all points in the specified interval.
* `sum_over_time(range-vector)`: the sum of all values in the specified interval.
* `count_over_time(range-vector)`: the count of all values in the specified interval.
* `quantile_over_time(scalar, range-vector)`: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified interval.
* `stddev_over_time(range-vector)`: the population standard deviation of the values in the specified interval.
* `stdvar_over_time(range-vector)`: the population standard variance of the values in the specified interval.
* `last_over_time(range-vector)`: the most recent point value in the specified interval.
* `present_over_time(range-vector)`: the value 1 for any series in the specified interval.

If the [feature flag](../feature_flags/)
`--enable-feature=promql-experimental-functions` is set, the following
additional functions are available:

* `mad_over_time(range-vector)`: the median absolute deviation of all points in the specified interval.

Note that all values in the specified interval have the same weight in the
aggregation even if the values are not equally spaced throughout the interval.

`avg_over_time`, `sum_over_time`, `count_over_time`, `last_over_time`, and
`present_over_time` handle native histograms as expected. All other functions
ignore histogram samples.

## Trigonometric Functions

The trigonometric functions work in radians:

- `acos(v instant-vector)`: calculates the arccosine of all elements in `v` ([special cases](https://pkg.go.dev/math#Acos)).
- `acosh(v instant-vector)`: calculates the inverse hyperbolic cosine of all elements in `v` ([special cases](https://pkg.go.dev/math#Acosh)).
- `asin(v instant-vector)`: calculates the arcsine of all elements in `v` ([special cases](https://pkg.go.dev/math#Asin)).
- `asinh(v instant-vector)`: calculates the inverse hyperbolic sine of all elements in `v` ([special cases](https://pkg.go.dev/math#Asinh)).
- `atan(v instant-vector)`: calculates the arctangent of all elements in `v` ([special cases](https://pkg.go.dev/math#Atan)).
- `atanh(v instant-vector)`: calculates the inverse hyperbolic tangent of all elements in `v` ([special cases](https://pkg.go.dev/math#Atanh)).
- `cos(v instant-vector)`: calculates the cosine of all elements in `v` ([special cases](https://pkg.go.dev/math#Cos)).
- `cosh(v instant-vector)`: calculates the hyperbolic cosine of all elements in `v` ([special cases](https://pkg.go.dev/math#Cosh)).
- `sin(v instant-vector)`: calculates the sine of all elements in `v` ([special cases](https://pkg.go.dev/math#Sin)).
- `sinh(v instant-vector)`: calculates the hyperbolic sine of all elements in `v` ([special cases](https://pkg.go.dev/math#Sinh)).
- `tan(v instant-vector)`: calculates the tangent of all elements in `v` ([special cases](https://pkg.go.dev/math#Tan)).
- `tanh(v instant-vector)`: calculates the hyperbolic tangent of all elements in `v` ([special cases](https://pkg.go.dev/math#Tanh)).

The following are useful for converting between degrees and radians:

- `deg(v instant-vector)`: converts radians to degrees for all elements in `v`.
- `pi()`: returns pi.
- `rad(v instant-vector)`: converts degrees to radians for all elements in `v`.
