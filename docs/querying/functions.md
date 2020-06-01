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

## `abs()`

`abs(v instant-vector)` returns the input vector with all sample values converted to
their absolute value.

## `absent()`

`absent(v instant-vector)` returns an empty vector if the vector passed to it
has any elements and a 1-element vector with the value 1 if the vector passed to
it has no elements.

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
passed to it has any elements and a 1-element vector with the value 1 if the
range vector passed to it has no elements.

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

`delta` should only be used with gauges.

## `deriv()`

`deriv(v range-vector)` calculates the per-second derivative of the time series in a range
vector `v`, using [simple linear regression](https://en.wikipedia.org/wiki/Simple_linear_regression).

`deriv` should only be used with gauges.

## `exp()`

`exp(v instant-vector)` calculates the exponential function for all elements in `v`.
Special cases are:

* `Exp(+Inf) = +Inf`
* `Exp(NaN) = NaN`

## `floor()`

`floor(v instant-vector)` rounds the sample values of all elements in `v` down
to the nearest integer.

## `histogram_quantile()`

`histogram_quantile(φ float, b instant-vector)` calculates the φ-quantile (0 ≤ φ
≤ 1) from the buckets `b` of a
[histogram](https://prometheus.io/docs/concepts/metric_types/#histogram). (See
[histograms and summaries](https://prometheus.io/docs/practices/histograms) for
a detailed explanation of φ-quantiles and the usage of the histogram metric type
in general.) The samples in `b` are the counts of observations in each bucket.
Each sample must have a label `le` where the label value denotes the inclusive
upper bound of the bucket. (Samples without such a label are silently ignored.)
The [histogram metric type](https://prometheus.io/docs/concepts/metric_types/#histogram)
automatically provides time series with the `_bucket` suffix and the appropriate
labels.

Use the `rate()` function to specify the time window for the quantile
calculation.

Example: A histogram metric is called `http_request_duration_seconds`. To
calculate the 90th percentile of request durations over the last 10m, use the
following expression:

    histogram_quantile(0.9, rate(http_request_duration_seconds_bucket[10m]))

The quantile is calculated for each label combination in
`http_request_duration_seconds`. To aggregate, use the `sum()` aggregator
around the `rate()` function. Since the `le` label is required by
`histogram_quantile()`, it has to be included in the `by` clause. The following
expression aggregates the 90th percentile by `job`:

    histogram_quantile(0.9, sum by (job, le) (rate(http_request_duration_seconds_bucket[10m])))

To aggregate everything, specify only the `le` label:

    histogram_quantile(0.9, sum by (le) (rate(http_request_duration_seconds_bucket[10m])))

The `histogram_quantile()` function interpolates quantile values by
assuming a linear distribution within a bucket. The highest bucket
must have an upper bound of `+Inf`. (Otherwise, `NaN` is returned.) If
a quantile is located in the highest bucket, the upper bound of the
second highest bucket is returned. A lower limit of the lowest bucket
is assumed to be 0 if the upper bound of that bucket is greater than
0. In that case, the usual linear interpolation is applied within that
bucket. Otherwise, the upper bound of the lowest bucket is returned
for quantiles located in the lowest bucket.

If `b` has 0 observations, `NaN` is returned. If `b` contains fewer than two buckets,
`NaN` is returned. For φ < 0, `-Inf` is returned. For φ > 1, `+Inf` is returned.

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

`increase` should only be used with counters. It is syntactic sugar
for `rate(v)` multiplied by the number of seconds under the specified
time range window, and should be used primarily for human readability.
Use `rate` in recording rules so that increases are tracked consistently
on a per-second basis.

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

This example will return a vector with each time series having a `foo` label with the value `a,b,c` added to it:

```
label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3")
```

## `label_replace()`

For each timeseries in `v`, `label_replace(v instant-vector, dst_label string,
replacement string, src_label string, regex string)` matches the regular
expression `regex` against the label `src_label`.  If it matches, then the
timeseries is returned with the label `dst_label` replaced by the expansion of
`replacement`. `$1` is replaced with the first matching subgroup, `$2` with the
second etc. If the regular expression doesn't match then the timeseries is
returned unchanged.

This example will return a vector with each time series having a `foo`
label with the value `a` added to it:

```
label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*")
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

`rate` should only be used with counters. It is best suited for alerting,
and for graphing of slow-moving counters.

Note that when combining `rate()` with an aggregation operator (e.g. `sum()`)
or a function aggregating over time (any function ending in `_over_time`),
always take a `rate()` first, then aggregate. Otherwise `rate()` cannot detect
counter resets when your target restarts.

## `resets()`

For each input time series, `resets(v range-vector)` returns the number of
counter resets within the provided time range as an instant vector. Any
decrease in the value between two consecutive samples is interpreted as a
counter reset.

`resets` should only be used with counters.

## `round()`

`round(v instant-vector, to_nearest=1 scalar)` rounds the sample values of all
elements in `v` to the nearest integer. Ties are resolved by rounding up. The
optional `to_nearest` argument allows specifying the nearest multiple to which
the sample values should be rounded. This multiple may also be a fraction.

## `scalar()`

Given a single-element input vector, `scalar(v instant-vector)` returns the
sample value of that single element as a scalar. If the input vector does not
have exactly one element, `scalar` will return `NaN`.

## `sort()`

`sort(v instant-vector)` returns vector elements sorted by their sample values,
in ascending order.

## `sort_desc()`

Same as `sort`, but sorts in descending order.

## `sqrt()`

`sqrt(v instant-vector)` calculates the square root of all elements in `v`.

## `time()`

`time()` returns the number of seconds since January 1, 1970 UTC. Note that
this does not actually return the current time, but the time at which the
expression is to be evaluated.

## `timestamp()`

`timestamp(v instant-vector)` returns the timestamp of each of the samples of
the given vector as the number of seconds since January 1, 1970 UTC.

*This function was added in Prometheus 2.0*

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

Note that all values in the specified interval have the same weight in the
aggregation even if the values are not equally spaced throughout the interval.
