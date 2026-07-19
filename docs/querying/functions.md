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

## `burst_score()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`burst_score(v range-vector, alpha scalar=0.1)` detects sudden spikes or bursts
in a time series using an Exponentially Weighted Moving Average (EWMA) baseline
and variance. It returns a score in `[0, 1]` where `1` means a strong burst.

The `alpha` parameter controls the EWMA decay rate and must be in `(0, 1]`.
Smaller values make the baseline slower to adapt, making the function more
sensitive to sustained changes. Larger values make it react faster.

The score is computed as:

`score = clamp(abs(last_value - baseline) / (3 * ewma_stddev))`

Works with both float and native histogram series (using histogram average).

**When to use:** Use `burst_score` when you need to detect sudden, short-lived
spikes that stand out from a slowly-varying baseline — for example, a sudden
traffic burst on an otherwise stable endpoint.

**Example:**

```
burst_score(rate(http_requests_total[5m])[1h], 0.1) > 0.8
```

Alerts when the current request rate is a strong burst relative to the recent
EWMA baseline.

## `changepoint()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`changepoint(v range-vector)` detects sudden baseline shifts in a time series
using the CUSUM (Cumulative Sum) algorithm. It returns a score in `[0, 1]`
normalised so that a CUSUM value of 5 sigma-steps maps to `1.0`.

The algorithm accumulates positive and negative deviations from the overall
mean. A large cumulative sum indicates a persistent shift in the baseline level.

Works with both float and native histogram series (using histogram average).

**When to use:** Use `changepoint` when you need to detect a sustained level
shift rather than a momentary spike — for example, a deployment that permanently
changes the memory footprint of a service.

**Example:**

```
changepoint(process_resident_memory_bytes[6h]) > 0.7
```

Alerts when the memory baseline has shifted significantly over the last 6 hours.

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

## `end()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`end()` returns the end timestamp of the current query range evaluation as the
number of seconds since January 1, 1970 UTC. For instant queries, this is equal
to the evaluation timestamp.

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

## `ewma()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`ewma(v range-vector, alpha scalar=0.2)` calculates the Exponentially Weighted Moving 
Average (EWMA) anomaly score for each float time series in the range vector `v` 
using the smoothing factor `alpha`.

The `alpha` parameter must be a scalar between 0 (exclusive) and 1 (inclusive). 
The function computes a running baseline and running variance on historical samples 
within the range window. The anomaly score is computed as:

`score = 1 - exp(-abs(last_value - baseline) / (3 * stddev))`

This outputs a value between `0` (normal) and `1` (highly anomalous).

Works with both float and native histogram series (using histogram average).

**When to use:** Use EWMA when you want to detect sudden, unexpected spikes or drops
in volatile system metrics, but want to ignore slow, gradual changes. It is highly
responsive to recent changes because the `alpha` parameter controls the decay rate
of older history.

**Usecase example (HTTP Response Latency Spikes):**

An API server normally responds in 50ms. During a database lock, response times 
suddenly jump to 1500ms. Since EWMA dynamically updates the running baseline, 
it will immediately identify this massive divergence from the standard deviation 
and return a score near `1.0`. If the latency stays high for a long time, the 
baseline will gradually adapt to the new normal and the score will return toward `0.0`.

*   **Query**: `ewma(http_request_duration_seconds_sum{job="api"}[2h], 0.2) > 0.85`
*   **Why**: Setting `alpha = 0.2` ensures the moving average weights recent history 
highly (around 80% weight on the last few minutes in a 2h window). An alert threshold 
of `> 0.85` filters out normal statistical noise, ensuring we only alert when the
deviation is at least 3 standard deviations away.

## `entropy()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`entropy(v range-vector)` computes the normalised Shannon entropy of the value
distribution within the range window. It returns a score in `[0, 1]` where `0`
means all values are identical and `1` means values are spread uniformly across
all histogram bins.

Bins are determined by Sturges\' rule: `ceil(log2(n)) + 1` bins. The result is
normalised by `log2(n_bins)` so it is always in `[0, 1]` regardless of window
size.

Works with both float and native histogram series (using histogram average).

**When to use:** Use `entropy` to measure the diversity or unpredictability of a
metric. High entropy means the metric is spread across many different values
(e.g. a healthy mix of response codes). Low entropy means it is concentrated
(e.g. all requests returning the same status code, which may indicate a stuck
state).

**Example:**

```
entropy(http_response_status_code[1h]) < 0.2
```

Alerts when HTTP response codes have collapsed to a single value (e.g. all 500s).

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
values stored in each native histogram sample in `v`. Float samples are ignored and do
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
each native histogram sample in `v`. Float samples are ignored and do not show up in
the returned vector.

Similarly, `histogram_sum(v instant-vector)` returns the sum of observations
stored in each native histogram sample.

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

## `histogram_quantiles()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`histogram_quantiles(v instant-vector, quantile_label string, φ_1 scalar, φ_2 scalar, ...)` calculates multiple (between 1 and 10) φ-quantiles (0 ≤
φ ≤ 1) from a [classic
histogram](https://prometheus.io/docs/concepts/metric_types/#histogram) or from
a native histogram. Quantile calculation works the same way as in `histogram_quantile()`.
The second argument (a string) specifies the label name that is used to identify different quantiles in the query result.
```
histogram_quantiles(sum(rate(foo[1m])), "quantile", 0.9, 0.99)
# => {quantile="0.9"} 123
     {quantile="0.99"} 128
```

## `histogram_stddev()` and `histogram_stdvar()`

`histogram_stddev(v instant-vector)` returns the estimated standard deviation
of observations for each native histogram sample in `v`. For this estimation, all observations
in a bucket are assumed to have the value of the mean of the bucket boundaries. For
the zero bucket and for buckets with custom boundaries, the arithmetic mean is used.
For the usual exponential buckets, the geometric mean is used. Float samples are ignored
and do not show up in the returned vector.

Similarly, `histogram_stdvar(v instant-vector)` returns the estimated
variance of observations for each native histogram sample in `v`.

## `hw()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`hw(v range-vector, alpha scalar=0.2, beta scalar=0.1)` computes the Holt-Winters 
(double exponential smoothing) anomaly score for each float time series in the range vector `v`.

The `alpha` parameter (level smoothing factor) and `beta` parameter (trend smoothing
factor) must be scalars between `0` and `1`. Holt-Winters continuously estimates 
both the current level and the underlying trend of the time series, projects the 
expected value of the latest sample, and returns a normalized anomaly score based on
the absolute deviation between the observed value and the trend-adjusted forecast.

**When to use:** Use this function specifically for metrics that have a clear linear 
trend (e.g., constant growth or reduction over time) where you want to detect abnormal 
deviations from the slope. Regular mean/standard deviation functions fail on trended 
metrics because the baseline is constantly changing.

**Usecase example (Disk Space Depletion Acceleration):**
A logging system writes log files at a steady rate, causing free disk space to decline
linearly by 2 GB per hour. If a bug suddenly causes logs to be written at 50 GB per 
hour, standard statistical alerts won't fire immediately because they don't model the 
trend. `hw` models the 2 GB/hour slope; it will immediately detect 
that the new 50 GB/hour decline is a massive anomaly relative to the expected trend.

*   **Query**: `hw(node_filesystem_free_bytes{mountpoint="/var/log"}[4h], 0.3, 0.1) > 0.8`
*   **Why**: `alpha = 0.3` controls how quickly the level adjusts, and `beta = 0.1` 
ensures the trend slope estimate is stable and doesn't bounce around with temporary spikes. 
A score above `0.8` indicates that the disk is depleting significantly faster than the linear trend forecast.

Works with both float and native histogram series (using histogram average).

## `hst()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`hst(v range-vector, trees scalar=100, depth scalar=8)` computes the density-based Half-Space 
Trees (HST) anomaly score for each float time series in the range vector `v`.

The `trees` parameter specifies the number of randomized trees to construct, and `depth` 
specifies the maximum depth of each tree. HST is an online anomaly detection algorithm 
that constructs recursive half-space partitions of the input space. Data points landing 
in historically low-density leaf nodes receive higher anomaly scores between `0` and `1`.

**When to use:** Use HST when your metric is multi-modal (meaning it has multiple 
distinct "normal" states or ranges, like CPU usage being 5% at idle and 85% during batch jobs) 
and you want to detect values that fall into the empty spaces between these normal states. 
Traditional average/stddev methods will incorrectly assume the middle value (e.g. 45% CPU) 
is the most "normal" because it matches the mean.

**Usecase example (CPU Usage Multimodal Profiling):**
A worker server alternates between sleeping (5% CPU) and processing large video transcodes 
(90% CPU). A CPU utilization of 45% is anomalous because the server should never be running 
at half-capacity for long. HST partitions the space and learns that the 5% region is dense, 
and the 90% region is dense, but the 45% region has zero density. It will return a score 
close to `1.0` for a 45% sample.

*   **Query**: `hst(instance:node_cpu_utilisation:rate1m[1h], 100, 15) > 0.75`
*   **Why**: `100` trees provide statistical stability while keeping memory low. 
A depth of `15` splits the 0-100% CPU range into fine-grained partitions.

Works with both float and native histogram series (using histogram average).

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

If there is no matching info series for a given time series in `v` at a
particular timestamp (e.g. because the info series has gone stale), the
behavior depends on the data label matchers: If the `data-label-selector`
contains any matcher that does not match the empty string (e.g.
`{data=~".+"}`), then that time series is dropped from the result at that
timestamp, because the required enrichment is unavailable. If all matchers
match the empty string (e.g. `{data=~".*"}`), or if no `data-label-selector`
is provided, the time series is returned without enrichment.

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

Note that if there are any time series in `v` that match the `data-label-selector` (or the default `target_info` if that argument is not specified), they will be treated as info series and will be returned unchanged.

### Limitations

In its current iteration, `info` defaults to considering only info series with
the name `target_info`. It also assumes that the identifying info series labels are
`instance` and `job`. `info` does support other info series names however, through
`__name__` label matchers. E.g., one can explicitly say to consider both
`target_info` and `build_info` as follows:
`{__name__=~"(target|build)_info"}`. However, the identifying labels always
have to be `instance` and `job`.

When only negated `__name__` matchers are provided (e.g.
`{__name__!="target_info"}`), `info` considers all metrics matching
`.+_info` and then applies the negated matchers as filters. This is
because negated matchers alone cannot positively identify which info
metrics to consider.

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

## `isf()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`isf(v range-vector, trees scalar=100, sample_size scalar=256)` computes the 
path-length based Isolation Forest anomaly score for each float time series in the 
range vector `v`.

The `trees` parameter specifies the number of isolation trees to build, and 
`sample_size` specifies the number of historical points sampled to construct each tree. 
Isolation Forest isolates anomalies by randomly partitioning feature paths. 
Anomalies have shorter path lengths in the trees and thus score closer to `1`.

**When to use:** Use this when you have a large set of timeseries instances 
(e.g. dozens of servers in a cluster) and want to isolate individual instances that 
behave completely differently from the rest of the group (outlier detection).

**Usecase example (Microservice Memory Leak):**
A Kubernetes deployment runs 50 replicas of an API gateway. Replicas typically 
consume between 200MB and 300MB of RAM. One replica develops a memory leak, 
and its consumption slowly increases to 1.5GB. Since Isolation Forest works by isolating
points, it will easily find that the server, requiring very few random partitions 
compared to the 49 servers clustered at 250MB, and returns a score close to `1.0`.

*   **Query**: `isf(container_memory_working_set_bytes{container="api"}[12h], 100, 256) > 0.8`
*   **Why**: Setting `sample_size = 256` ensures there are enough data points in 
each tree to create distinct partition paths. A score threshold of `> 0.8` ensures 
we only alert on extreme outliers that are easily isolated.

Works with both float and native histogram series (using histogram average).

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

## `max_of()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`max_of(a scalar, b scalar)` returns the larger of the two scalar values `a`
and `b`.

## `min_of()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`min_of(a scalar, b scalar)` returns the smaller of the two scalar values `a`
and `b`.

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

## `mad()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`mad(v range-vector, threshold scalar=3)` computes the Median Absolute Deviation (MAD) 
anomaly score for each float time series in the range vector `v`.

The `threshold` parameter (standard deviation scale factor) must be positive. 
Unlike Z-Score, MAD uses the median instead of the mean, making it highly robust against
extreme historical outliers. The score is computed as:

`score = clamp(abs(last_value - median) / (1.4826 * MAD * threshold))`

Works with both float and native histogram series (using histogram average).

**When to use:** Use MAD when your historical baseline data is "dirty" and contains 
massive, extreme spikes (e.g. daily cron jobs, data backups). In Z-score, a single 
massive spike inflates the standard deviation so much that smaller, real anomalies 
go undetected (called the "masking effect"). 
MAD is highly resistant to this because it uses medians.

**Usecase example (Database Query Rate Anomalies):**
A PostGreSQL database handles 100 queries/sec normally. Every midnight, a backup job runs,
causing a massive, brief spike to 5000 queries/sec. If a slow memory leak later causes the 
query rate to drop to 5 queries/sec, Z-score won't alert because the standard deviation 
is massive (inflated by the 5000 QPS spike). MAD ignores the midnight spike and correctly
triggers an anomaly alert for the 5 QPS drop.

*   **Query**: `mad(pg_stat_database_xact_commit[6h], 3.0) > 1.0`
*   **Why**: A threshold of `3.0` scales the median absolute deviation to be equivalent 
to 3-sigma boundaries. Any score above `1.0` indicates a robust outlier.

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

## `qscore()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`qscore(v range-vector, lower scalar=0.05, upper scalar=0.95)` computes the 
quantile-based deviation score for each float time series in the range vector `v`.

The parameters `lower` and `upper` define the quantile boundaries (e.g. `0.05` and `0.95`). 
If the latest value exceeds the upper quantile or falls below the lower quantile, 
it calculates the relative deviation towards the historical bounds.

**When to use:** Use this when you want to alert on breach of historical percentiles
(SLA compliance) or when you want to ignore normal peak hours (e.g. day vs night traffic)
and only alert when a metric breaks the historical 5th or 95th percentiles.

**Usecase example (Active User Session SLA):**
A customer web portal has highly variable traffic: 100 users at night, 1000 users during lunch.
The 95th percentile of user sessions over 24 hours is 900. If the active session count climbs 
to 1200 due to a scraping bot, the value is beyond the 95th percentile. 
`qscore` calculates the deviation above the 95th percentile toward the absolute max, 
yielding a score close to `1.0` to trigger an alert.

*   **Query**: `qscore(istio_requests_in_flight{destination_service="portal.default.svc.cluster.local"}[24h], 0.05, 0.95) > 0.8`
*   **Why**: Evaluates the latest active session count against the 5% (lower limit) and
95% (upper limit) quantiles over the last 24 hours. A score `> 0.8` means the current value 
is near or beyond the historic maximum limits.

Works with both float and native histogram series (using histogram average).

## `range()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`range()` returns the range duration of the current query range evaluation in
seconds and is equivalent to `end() - start()`. For instant queries, this returns `0`.

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

## `random_cut_score()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`random_cut_score(v range-vector, trees scalar=100)` computes a stateless
random-cut anomaly score for each float time series in the range vector `v`.

The algorithm repeatedly partitions the history with random axis-aligned cuts
until the query point is isolated, returning a normalised depth score in
`[0, 1]`. It is **not** a streaming Random Cut Forest: no model is persisted
between evaluations and the cost is O(trees × N log N) per call.

The `trees` parameter specifies the number of independent random-cut trials.
Features are derived from the raw value, velocity, acceleration, and EWMA
statistics of the time series.

**When to use:** Use `random_cut_score` for complex, multi-dimensional
anomalies such as phase shifts, sudden variance changes (jitter), or structural
breaks that cannot be detected by simple value thresholds.

**Usecase example (Latency Jitter):**
A service normally responds in around 50 ms with very little variation. After a
deployment, lock contention causes response times to rapidly oscillate between
20 ms and 80 ms while the average latency remains close to 50 ms.
Traditional threshold- or mean-based detectors may not trigger because the
baseline is unchanged. `random_cut_score` captures the changing shape of the
time series through features such as velocity, acceleration, and exponentially
weighted moving averages, producing an anomaly score close to 1.0.

*   **Query**: `random_cut_score(http_requests_total{job="gateway"}[24h], 100) > 0.8`
*   **Why**: Using `100` trees offers a great balance between accuracy and CPU evaluation time.

Works with both float and native histogram series (using histogram average).

## `rcf()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`rcf(v range-vector, trees scalar=100, sample_size scalar=256)` computes a
streaming Random Cut Forest (RCF) anomaly score for each float time series in
the range vector `v`, based on Guha et al. (ICML 2016).

Unlike `random_cut_score`, `rcf` maintains a **persistent per-series model**
across PromQL evaluations. Each model is an ensemble of `trees` Random Cut
Trees sharing a bounded reservoir of `sample_size` points. New samples are
inserted incrementally and old samples are evicted via reservoir sampling.
The anomaly score is the average collusive displacement of the latest feature
vector, normalised to `[0, 1]`.

The six feature dimensions used are: `value` (z-score), `delta`, `velocity`,
`acceleration`, `ewma_deviation`, and `coefficient_of_variation`.

Models are stored in an in-memory LRU cache and persisted to disk so they
survive Prometheus restarts. By default the store path is
`<storage.tsdb.path>/rcf` (i.e. alongside the TSDB data), so persistence
works out of the box with no configuration required.

Both settings are configurable in the Prometheus config file under
`storage.rcf`:

| Config key | Default | Description |
|---|---|---|
| `storage.rcf.store_path` | `<storage.tsdb.path>/rcf` | Directory for on-disk model persistence. Set to empty string to disable. |
| `storage.rcf.cache_size` | `1024` | Maximum number of per-series models kept in memory. |

Example:

```
storage:
  rcf:
    store_path: /data/rcf
    cache_size: 2048
```

**When to use:** Use `rcf` when you need a stateful, incrementally-updated
anomaly detector that improves over time as it sees more data — for example,
detecting gradual drift, multi-dimensional anomalies, or patterns that only
become visible after a warm-up period.

**Example:**

```
rcf(http_request_duration_seconds_sum[1h], 50, 128) > 0.8
```

Alerts when the request duration time series deviates significantly from its
learned normal behaviour.

Works with both float and native histogram series (using histogram average).

## `rcf_attribution()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`rcf_attribution(v range-vector, trees scalar=100, sample_size scalar=256)`
uses the same streaming Random Cut Forest model as `rcf()` and returns the
per-dimension contribution to the anomaly score for each input series.

The output carries a `rcf_dim` label identifying the feature dimension
(`value`, `delta`, `velocity`, `acceleration`, `ewma_dev`, `cv`). Higher
values indicate that dimension contributed more to the anomaly.

**Example:**

```
rcf_attribution(http_request_duration_seconds_sum[1h], 50, 128)
```

Returns one result per input series showing which feature dimension is driving
the anomaly score.

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

## `seasonal()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`seasonal(v range-vector, period scalar=24, alpha scalar=0.2)` computes the seasonal time-series
pattern anomaly score for each float time series in the range vector `v`.

The `period` parameter specifies the seasonal cycle duration in seconds (e.g. `86400` 
for a daily cycle). The `alpha` parameter is the learning rate for the seasonal EWMA baseline. 
It compares the current value with the historical average of the corresponding seasonal slot 
(e.g., comparing Monday 10:00 AM with prior Mondays at 10:00 AM).

**When to use:** Use this for metrics that exhibit strong, regular periodic patterns (daily, weekly), 
where the definition of "normal" depends entirely on the time of day or day of the week.

**Usecase example (E-Commerce Traffic Drop):**
An e-commerce app receives 10,000 requests/sec at 2:00 PM and 100 requests/sec at 3:00 AM. 
A simple flat threshold alert would either trigger false alerts at night (as traffic naturally drops) 
or fail to detect if traffic drops from 10,000 to 500 at 2:00 PM (since 500 is still above 
the 100 night-time minimum). `seasonal` compares the 2:00 PM traffic to previous 2:00 PM windows,
and will immediately flag a drop from 10,000 to 500 as an extreme anomaly.

*   **Query**: `seasonal(http_requests_total[7d], 86400, 0.2) > 0.8`
*   **Why**: `86400` seconds represents a 24-hour cycle. `alpha = 0.2` updates the seasonal baseline
with a 20% weight from the most recent cycle, allowing it to adapt to slow daylight 
savings or organic business changes.

Works with both float and native histogram series (using histogram average).

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

## `start()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`start()` returns the start timestamp of the current query range evaluation as the
number of seconds since January 1, 1970 UTC. For instant queries, this is equal
to the evaluation timestamp.

## `step()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`step()` returns the query resolution step as the number of seconds. For instant
queries, this returns `0`.

## `time()`

`time()` returns the number of seconds since January 1, 1970 UTC. Note that
this does not actually return the current time, but the time at which the
expression is to be evaluated.

## `timestamp()`

`timestamp(v instant-vector)` returns the timestamp of each of the samples of
the given vector as the number of seconds since January 1, 1970 UTC. It acts on
float and histogram samples in the same way.

## `trend_score()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`trend_score(v range-vector)` detects values that deviate abnormally from the
linear trend of the series using ordinary least-squares regression. It returns
a score in `[0, 1]` where `0` means the last value fits the trend perfectly and
`1` means it is more than 3 residual standard deviations away.

The score is computed as:

`score = clamp(abs(last_residual) / (3 * residual_stddev))`

Works with both float and native histogram series (using histogram average).

**When to use:** Use `trend_score` when your metric has a clear directional
trend (growing or shrinking) and you want to detect values that break that
trend — for example, a disk that is filling up faster than its historical rate,
or a queue that suddenly stops draining.

**Example:**

```
trend_score(node_filesystem_avail_bytes[24h]) > 0.8
```

Alerts when the available disk space deviates significantly from its 24-hour
linear trend, which can indicate an unexpected write burst or a stuck cleanup
job.

## `vector()`

`vector(s scalar)` converts the scalar `s` to a float sample and returns it as
a single-element instant vector with no labels.

## `year()`

`year(v=vector(time()) instant-vector)` returns the year for each of the given
times in UTC. Histogram samples in the input vector are ignored silently.

## `zscore()`

**This function has to be enabled via the [feature
flag](../feature_flags.md#experimental-promql-functions)
`--enable-feature=promql-experimental-functions`.**

`zscore(v range-vector, threshold scalar=3)` computes the standard deviation (Z-score) 
deviation score for each float time series in the range vector `v`.

The `threshold` parameter is the multiplier for standard deviation (e.g., `3.0` for 3-sigma). 
The score is computed as:

`z = abs(last_value - mean) / stddev`
`score = clamp(z / threshold)`

Works with both float and native histogram series (using histogram average).

**When to use:** Use this for normally distributed, stationary metrics 
(meaning they have a stable baseline and do not have trends or cyclic patterns). 
It is the computationally cheapest way to detect when a metric moves 
significantly far from its historical average.

**Usecase example (Connection Pool Exhaustion):**
An database client maintains a connection pool that fluctuates stably around 20
active connections with a standard deviation of 3 connections. If the active 
connection count suddenly spikes to 32 (which is 4 standard deviations away from the mean), 
the Z-score is 4.0. Divided by a threshold of `3.0`, this yields a normalized score of `1.33`
(clamped to `1.0`), which will trigger an anomaly alert.

*   **Query**: `zscore(db_connections_active{service="orders"}[12h], 3.0) > 1.0`
*   **Why**: Setting `threshold = 3.0` matches the classic 3-sigma boundary of 
statistical process control. A score above `1.0` indicates that the queue depth is more than
3 standard deviations away from the 12-hour average.

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
* `stdvar_over_time(range-vector)`: the population variance of all float samples in the specified interval.
* `last_over_time(range-vector)`: the most recent sample in the specified interval.
* `first_over_time(range-vector)`: the oldest sample in the specified interval.
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
in range queries to ensure that the sample selected is within the range step.

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
