import React from 'react';

const funcDocs: Record<string, React.ReactNode> = {
  abs: (
    <>
      <p>
        <code>abs(v instant-vector)</code> returns the input vector with all sample values converted to their absolute value.
      </p>
    </>
  ),
  absent: (
    <>
      <p>
        <code>absent(v instant-vector)</code> returns an empty vector if the vector passed to it has any elements (floats or
        native histograms) and a 1-element vector with the value 1 if the vector passed to it has no elements.
      </p>

      <p>This is useful for alerting on when no time series exist for a given metric name and label combination.</p>

      <pre>
        <code>
          absent(nonexistent{'{'}job=&quot;myjob&quot;{'}'}) # =&gt; {'{'}job=&quot;myjob&quot;{'}'}
          absent(nonexistent{'{'}job=&quot;myjob&quot;,instance=~&quot;.*&quot;{'}'}) # =&gt; {'{'}job=&quot;myjob&quot;{'}'}
          absent(sum(nonexistent{'{'}job=&quot;myjob&quot;{'}'})) # =&gt; {'{'}
          {'}'}
        </code>
      </pre>

      <p>
        In the first two examples, <code>absent()</code> tries to be smart about deriving labels of the 1-element output
        vector from the input vector.
      </p>
    </>
  ),
  absent_over_time: (
    <>
      <p>
        <code>absent_over_time(v range-vector)</code> returns an empty vector if the range vector passed to it has any
        elements (floats or native histograms) and a 1-element vector with the value 1 if the range vector passed to it has
        no elements.
      </p>

      <p>
        This is useful for alerting on when no time series exist for a given metric name and label combination for a certain
        amount of time.
      </p>

      <pre>
        <code>
          absent_over_time(nonexistent{'{'}job=&quot;myjob&quot;{'}'}[1h]) # =&gt; {'{'}job=&quot;myjob&quot;{'}'}
          absent_over_time(nonexistent{'{'}job=&quot;myjob&quot;,instance=~&quot;.*&quot;{'}'}[1h]) # =&gt; {'{'}
          job=&quot;myjob&quot;{'}'}
          absent_over_time(sum(nonexistent{'{'}job=&quot;myjob&quot;{'}'})[1h:]) # =&gt; {'{'}
          {'}'}
        </code>
      </pre>

      <p>
        In the first two examples, <code>absent_over_time()</code> tries to be smart about deriving labels of the 1-element
        output vector from the input vector.
      </p>
    </>
  ),
  acos: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  acosh: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  asin: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  asinh: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  atan: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  atanh: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  avg_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  ceil: (
    <>
      <p>
        <code>ceil(v instant-vector)</code> rounds the sample values of all elements in <code>v</code> up to the nearest
        integer value greater than or equal to v.
      </p>

      <ul>
        <li>
          <code>ceil(+Inf) = +Inf</code>
        </li>
        <li>
          <code>ceil(±0) = ±0</code>
        </li>
        <li>
          <code>ceil(1.49) = 2.0</code>
        </li>
        <li>
          <code>ceil(1.78) = 2.0</code>
        </li>
      </ul>
    </>
  ),
  changes: (
    <>
      <p>
        For each input time series, <code>changes(v range-vector)</code> returns the number of times its value has changed
        within the provided time range as an instant vector.
      </p>
    </>
  ),
  clamp: (
    <>
      <p>
        <code>clamp(v instant-vector, min scalar, max scalar)</code>
        clamps the sample values of all elements in <code>v</code> to have a lower limit of <code>min</code> and an upper
        limit of <code>max</code>.
      </p>

      <p>Special cases:</p>

      <ul>
        <li>
          Return an empty vector if <code>min &gt; max</code>
        </li>
        <li>
          Return <code>NaN</code> if <code>min</code> or <code>max</code> is <code>NaN</code>
        </li>
      </ul>
    </>
  ),
  clamp_max: (
    <>
      <p>
        <code>clamp_max(v instant-vector, max scalar)</code> clamps the sample values of all elements in <code>v</code> to
        have an upper limit of <code>max</code>.
      </p>
    </>
  ),
  clamp_min: (
    <>
      <p>
        <code>clamp_min(v instant-vector, min scalar)</code> clamps the sample values of all elements in <code>v</code> to
        have a lower limit of <code>min</code>.
      </p>
    </>
  ),
  cos: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  cosh: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  count_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  day_of_month: (
    <>
      <p>
        <code>day_of_month(v=vector(time()) instant-vector)</code> returns the day of the month for each of the given times
        in UTC. Returned values are from 1 to 31.
      </p>
    </>
  ),
  day_of_week: (
    <>
      <p>
        <code>day_of_week(v=vector(time()) instant-vector)</code> returns the day of the week for each of the given times in
        UTC. Returned values are from 0 to 6, where 0 means Sunday etc.
      </p>
    </>
  ),
  day_of_year: (
    <>
      <p>
        <code>day_of_year(v=vector(time()) instant-vector)</code> returns the day of the year for each of the given times in
        UTC. Returned values are from 1 to 365 for non-leap years, and 1 to 366 in leap years.
      </p>
    </>
  ),
  days_in_month: (
    <>
      <p>
        <code>days_in_month(v=vector(time()) instant-vector)</code> returns number of days in the month for each of the given
        times in UTC. Returned values are from 28 to 31.
      </p>
    </>
  ),
  deg: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  delta: (
    <>
      <p>
        <code>delta(v range-vector)</code> calculates the difference between the first and last value of each time series
        element in a range vector <code>v</code>, returning an instant vector with the given deltas and equivalent labels.
        The delta is extrapolated to cover the full time range as specified in the range vector selector, so that it is
        possible to get a non-integer result even if the sample values are all integers.
      </p>

      <p>The following example expression returns the difference in CPU temperature between now and 2 hours ago:</p>

      <pre>
        <code>
          delta(cpu_temp_celsius{'{'}host=&quot;zeus&quot;{'}'}[2h])
        </code>
      </pre>

      <p>
        <code>delta</code> acts on native histograms by calculating a new histogram where each component (sum and count of
        observations, buckets) is the difference between the respective component in the first and last native histogram in
        <code>v</code>. However, each element in <code>v</code> that contains a mix of float and native histogram samples
        within the range, will be missing from the result vector.
      </p>

      <p>
        <code>delta</code> should only be used with gauges and native histograms where the components behave like gauges
        (so-called gauge histograms).
      </p>
    </>
  ),
  deriv: (
    <>
      <p>
        <code>deriv(v range-vector)</code> calculates the per-second derivative of the time series in a range vector{' '}
        <code>v</code>, using <a href="https://en.wikipedia.org/wiki/Simple_linear_regression">simple linear regression</a>.
        The range vector must have at least two samples in order to perform the calculation. When <code>+Inf</code> or
        <code>-Inf</code> are found in the range vector, the slope and offset value calculated will be <code>NaN</code>.
      </p>

      <p>
        <code>deriv</code> should only be used with gauges.
      </p>
    </>
  ),
  exp: (
    <>
      <p>
        <code>exp(v instant-vector)</code> calculates the exponential function for all elements in <code>v</code>. Special
        cases are:
      </p>

      <ul>
        <li>
          <code>Exp(+Inf) = +Inf</code>
        </li>
        <li>
          <code>Exp(NaN) = NaN</code>
        </li>
      </ul>
    </>
  ),
  floor: (
    <>
      <p>
        <code>floor(v instant-vector)</code> rounds the sample values of all elements in <code>v</code> down to the nearest
        integer value smaller than or equal to v.
      </p>

      <ul>
        <li>
          <code>floor(+Inf) = +Inf</code>
        </li>
        <li>
          <code>floor(±0) = ±0</code>
        </li>
        <li>
          <code>floor(1.49) = 1.0</code>
        </li>
        <li>
          <code>floor(1.78) = 1.0</code>
        </li>
      </ul>
    </>
  ),
  histogram_avg: (
    <>
      <p>
        <em>
          This function only acts on native histograms.
        </em>
      </p>

      <p>
        <code>histogram_avg(v instant-vector)</code> returns the arithmetic average of observed values stored in a native
        histogram. Samples that are not native histograms are ignored and do not show up in the returned vector.
      </p>

      <p>
        Use <code>histogram_avg</code> as demonstrated below to compute the average request duration over a 5-minute window
        from a native histogram:
      </p>

      <pre>
        <code>histogram_avg(rate(http_request_duration_seconds[5m]))</code>
      </pre>

      <p>Which is equivalent to the following query:</p>

      <pre>
        <code>
          {' '}
          histogram_sum(rate(http_request_duration_seconds[5m])) / histogram_count(rate(http_request_duration_seconds[5m]))
        </code>
      </pre>
    </>
  ),
  'histogram_count()` and `histogram_sum': (
    <>
      <p>
        <em>
          Both functions only act on native histograms.
        </em>
      </p>

      <p>
        <code>histogram_count(v instant-vector)</code> returns the count of observations stored in a native histogram.
        Samples that are not native histograms are ignored and do not show up in the returned vector.
      </p>

      <p>
        Similarly, <code>histogram_sum(v instant-vector)</code> returns the sum of observations stored in a native histogram.
      </p>

      <p>
        Use <code>histogram_count</code> in the following way to calculate a rate of observations (in this case corresponding
        to “requests per second”) from a native histogram:
      </p>

      <pre>
        <code>histogram_count(rate(http_request_duration_seconds[10m]))</code>
      </pre>
    </>
  ),
  histogram_fraction: (
    <>
      <p>
        <em>
          This function only acts on native histograms.
        </em>
      </p>

      <p>
        For a native histogram, <code>histogram_fraction(lower scalar, upper scalar, v instant-vector)</code> returns the
        estimated fraction of observations between the provided lower and upper values. Samples that are not native
        histograms are ignored and do not show up in the returned vector.
      </p>

      <p>
        For example, the following expression calculates the fraction of HTTP requests over the last hour that took 200ms or
        less:
      </p>

      <pre>
        <code>histogram_fraction(0, 0.2, rate(http_request_duration_seconds[1h]))</code>
      </pre>

      <p>
        The error of the estimation depends on the resolution of the underlying native histogram and how closely the provided
        boundaries are aligned with the bucket boundaries in the histogram.
      </p>

      <p>
        <code>+Inf</code> and <code>-Inf</code> are valid boundary values. For example, if the histogram in the expression
        above included negative observations (which shouldn&rsquo;t be the case for request durations), the appropriate lower
        boundary to include all observations less than or equal 0.2 would be <code>-Inf</code> rather than <code>0</code>.
      </p>

      <p>
        Whether the provided boundaries are inclusive or exclusive is only relevant if the provided boundaries are precisely
        aligned with bucket boundaries in the underlying native histogram. In this case, the behavior depends on the schema
        definition of the histogram. The currently supported schemas all feature inclusive upper boundaries and exclusive
        lower boundaries for positive values (and vice versa for negative values). Without a precise alignment of boundaries,
        the function uses linear interpolation to estimate the fraction. With the resulting uncertainty, it becomes
        irrelevant if the boundaries are inclusive or exclusive.
      </p>
    </>
  ),
  histogram_quantile: (
    <>
      <p>
        <code>histogram_quantile(φ scalar, b instant-vector)</code> calculates the φ-quantile (0 ≤ φ ≤ 1) from a{' '}
        <a href="https://prometheus.io/docs/concepts/metric_types/#histogram">classic histogram</a> or from a native
        histogram. (See <a href="https://prometheus.io/docs/practices/histograms">histograms and summaries</a> for a detailed
        explanation of φ-quantiles and the usage of the (classic) histogram metric type in general.)
      </p>

      <p>
        The float samples in <code>b</code> are considered the counts of observations in each bucket of one or more classic
        histograms. Each float sample must have a label
        <code>le</code> where the label value denotes the inclusive upper bound of the bucket. (Float samples without such a
        label are silently ignored.) The other labels and the metric name are used to identify the buckets belonging to each
        classic histogram. The{' '}
        <a href="https://prometheus.io/docs/concepts/metric_types/#histogram">histogram metric type</a>
        automatically provides time series with the <code>_bucket</code> suffix and the appropriate labels.
      </p>

      <p>
        The native histogram samples in <code>b</code> are treated each individually as a separate histogram to calculate the
        quantile from.
      </p>

      <p>
        As long as no naming collisions arise, <code>b</code> may contain a mix of classic and native histograms.
      </p>

      <p>
        Use the <code>rate()</code> function to specify the time window for the quantile calculation.
      </p>

      <p>
        Example: A histogram metric is called <code>http_request_duration_seconds</code> (and therefore the metric name for
        the buckets of a classic histogram is
        <code>http_request_duration_seconds_bucket</code>). To calculate the 90th percentile of request durations over the
        last 10m, use the following expression in case
        <code>http_request_duration_seconds</code> is a classic histogram:
      </p>

      <pre>
        <code>histogram_quantile(0.9, rate(http_request_duration_seconds_bucket[10m]))</code>
      </pre>

      <p>For a native histogram, use the following expression instead:</p>

      <pre>
        <code>histogram_quantile(0.9, rate(http_request_duration_seconds[10m]))</code>
      </pre>

      <p>
        The quantile is calculated for each label combination in
        <code>http_request_duration_seconds</code>. To aggregate, use the <code>sum()</code> aggregator around the{' '}
        <code>rate()</code> function. Since the <code>le</code> label is required by
        <code>histogram_quantile()</code> to deal with classic histograms, it has to be included in the <code>by</code>{' '}
        clause. The following expression aggregates the 90th percentile by <code>job</code> for classic histograms:
      </p>

      <pre>
        <code>histogram_quantile(0.9, sum by (job, le) (rate(http_request_duration_seconds_bucket[10m])))</code>
      </pre>

      <p>When aggregating native histograms, the expression simplifies to:</p>

      <pre>
        <code>histogram_quantile(0.9, sum by (job) (rate(http_request_duration_seconds[10m])))</code>
      </pre>

      <p>
        To aggregate all classic histograms, specify only the <code>le</code> label:
      </p>

      <pre>
        <code>histogram_quantile(0.9, sum by (le) (rate(http_request_duration_seconds_bucket[10m])))</code>
      </pre>

      <p>
        With native histograms, aggregating everything works as usual without any <code>by</code> clause:
      </p>

      <pre>
        <code>histogram_quantile(0.9, sum(rate(http_request_duration_seconds[10m])))</code>
      </pre>

      <p>
        The <code>histogram_quantile()</code> function interpolates quantile values by assuming a linear distribution within
        a bucket.
      </p>

      <p>
        If <code>b</code> has 0 observations, <code>NaN</code> is returned. For φ &lt; 0, <code>-Inf</code> is returned. For
        φ &gt; 1, <code>+Inf</code> is returned. For φ = <code>NaN</code>, <code>NaN</code> is returned.
      </p>

      <p>
        The following is only relevant for classic histograms: If <code>b</code> contains fewer than two buckets,{' '}
        <code>NaN</code> is returned. The highest bucket must have an upper bound of <code>+Inf</code>. (Otherwise,{' '}
        <code>NaN</code> is returned.) If a quantile is located in the highest bucket, the upper bound of the second highest
        bucket is returned. A lower limit of the lowest bucket is assumed to be 0 if the upper bound of that bucket is
        greater than 0. In that case, the usual linear interpolation is applied within that bucket. Otherwise, the upper
        bound of the lowest bucket is returned for quantiles located in the lowest bucket.
      </p>

      <p>
        You can use <code>histogram_quantile(0, v instant-vector)</code> to get the estimated minimum value stored in a
        histogram.
      </p>

      <p>
        You can use <code>histogram_quantile(1, v instant-vector)</code> to get the estimated maximum value stored in a
        histogram.
      </p>

      <p>Buckets of classic histograms are cumulative. Therefore, the following should always be the case:</p>

      <ul>
        <li>The counts in the buckets are monotonically increasing (strictly non-decreasing).</li>
        <li>
          A lack of observations between the upper limits of two consecutive buckets results in equal counts in those two
          buckets.
        </li>
      </ul>

      <p>
        However, floating point precision issues (e.g. small discrepancies introduced by computing of buckets with{' '}
        <code>sum(rate(...))</code>) or invalid data might violate these assumptions. In that case,
        <code>histogram_quantile</code> would be unable to return meaningful results. To mitigate the issue,
        <code>histogram_quantile</code> assumes that tiny relative differences between consecutive buckets are happening
        because of floating point precision errors and ignores them. (The threshold to ignore a difference between two
        buckets is a trillionth (1e-12) of the sum of both buckets.) Furthermore, if there are non-monotonic bucket counts
        even after this adjustment, they are increased to the value of the previous buckets to enforce monotonicity. The
        latter is evidence for an actual issue with the input data and is therefore flagged with an informational annotation
        reading <code>input to histogram_quantile needed to be fixed for monotonicity</code>. If you encounter this
        annotation, you should find and remove the source of the invalid data.
      </p>
    </>
  ),
  'histogram_stddev()` and `histogram_stdvar': (
    <>
      <p>
        <em>
          Both functions only act on native histograms.
        </em>
      </p>

      <p>
        
        <code>histogram_stddev(v instant-vector)</code> returns the estimated standard deviation of observations in a native
        histogram. For this estimation, all observations in a bucket are assumed to have the value of the mean of the bucket boundaries.
        For the zero bucket and for buckets with custom boundaries, the arithmetic mean is used. For the usual exponential buckets,
        the geometric mean is used. Samples that are not native histograms are ignored and do not show up in the returned vector.
      </p>

      <p>
        Similarly, <code>histogram_stdvar(v instant-vector)</code> returns the estimated standard variance of observations in
        a native histogram.
      </p>
    </>
  ),
  double_exponential_smoothing: (
    <>
      <p>
        <code>double_exponential_smoothing(v range-vector, sf scalar, tf scalar)</code> produces a smoothed value for time series based on
        the range in <code>v</code>. The lower the smoothing factor <code>sf</code>, the more importance is given to old
        data. The higher the trend factor <code>tf</code>, the more trends in the data is considered. Both <code>sf</code>{' '}
        and <code>tf</code> must be between 0 and 1.
      </p>

      <p>
        <code>double_exponential_smoothing</code> should only be used with gauges.
      </p>
    </>
  ),
  hour: (
    <>
      <p>
        <code>hour(v=vector(time()) instant-vector)</code> returns the hour of the day for each of the given times in UTC.
        Returned values are from 0 to 23.
      </p>
    </>
  ),
  idelta: (
    <>
      <p>
        <code>idelta(v range-vector)</code> calculates the difference between the last two samples in the range vector{' '}
        <code>v</code>, returning an instant vector with the given deltas and equivalent labels.
      </p>

      <p>
        <code>idelta</code> should only be used with gauges.
      </p>
    </>
  ),
  increase: (
    <>
      <p>
        <code>increase(v range-vector)</code> calculates the increase in the time series in the range vector. Breaks in
        monotonicity (such as counter resets due to target restarts) are automatically adjusted for. The increase is
        extrapolated to cover the full time range as specified in the range vector selector, so that it is possible to get a
        non-integer result even if a counter increases only by integer increments.
      </p>

      <p>
        The following example expression returns the number of HTTP requests as measured over the last 5 minutes, per time
        series in the range vector:
      </p>

      <pre>
        <code>
          increase(http_requests_total{'{'}job=&quot;api-server&quot;{'}'}[5m])
        </code>
      </pre>

      <p>
        <code>increase</code> acts on native histograms by calculating a new histogram where each component (sum and count of
        observations, buckets) is the increase between the respective component in the first and last native histogram in
        <code>v</code>. However, each element in <code>v</code> that contains a mix of float and native histogram samples
        within the range, will be missing from the result vector.
      </p>

      <p>
        <code>increase</code> should only be used with counters and native histograms where the components behave like
        counters. It is syntactic sugar for <code>rate(v)</code> multiplied by the number of seconds under the specified time
        range window, and should be used primarily for human readability. Use <code>rate</code> in recording rules so that
        increases are tracked consistently on a per-second basis.
      </p>
    </>
  ),
  irate: (
    <>
      <p>
        <code>irate(v range-vector)</code> calculates the per-second instant rate of increase of the time series in the range
        vector. This is based on the last two data points. Breaks in monotonicity (such as counter resets due to target
        restarts) are automatically adjusted for.
      </p>

      <p>
        The following example expression returns the per-second rate of HTTP requests looking up to 5 minutes back for the
        two most recent data points, per time series in the range vector:
      </p>

      <pre>
        <code>
          irate(http_requests_total{'{'}job=&quot;api-server&quot;{'}'}[5m])
        </code>
      </pre>

      <p>
        <code>irate</code> should only be used when graphing volatile, fast-moving counters. Use <code>rate</code> for alerts
        and slow-moving counters, as brief changes in the rate can reset the <code>FOR</code> clause and graphs consisting
        entirely of rare spikes are hard to read.
      </p>

      <p>
        Note that when combining <code>irate()</code> with an
        <a href="operators.md#aggregation-operators">aggregation operator</a> (e.g. <code>sum()</code>) or a function
        aggregating over time (any function ending in <code>_over_time</code>), always take a <code>irate()</code> first,
        then aggregate. Otherwise <code>irate()</code> cannot detect counter resets when your target restarts.
      </p>
    </>
  ),
  label_join: (
    <>
      <p>
        For each timeseries in <code>v</code>,{' '}
        <code>
          label_join(v instant-vector, dst_label string, separator string, src_label_1 string, src_label_2 string, ...)
        </code>{' '}
        joins all the values of all the <code>src_labels</code>
        using <code>separator</code> and returns the timeseries with the label <code>dst_label</code> containing the joined
        value. There can be any number of <code>src_labels</code> in this function.
      </p>

      <p>
        <code>label_join</code> acts on float and histogram samples in the same way.
      </p>

      <p>
        This example will return a vector with each time series having a <code>foo</code> label with the value{' '}
        <code>a,b,c</code> added to it:
      </p>

      <pre>
        <code>
          label_join(up{'{'}job=&quot;api-server&quot;,src1=&quot;a&quot;,src2=&quot;b&quot;,src3=&quot;c&quot;{'}'},
          &quot;foo&quot;, &quot;,&quot;, &quot;src1&quot;, &quot;src2&quot;, &quot;src3&quot;)
        </code>
      </pre>
    </>
  ),
  label_replace: (
    <>
      <p>
        For each timeseries in <code>v</code>,{' '}
        <code>label_replace(v instant-vector, dst_label string, replacement string, src_label string, regex string)</code>
        matches the <a href="https://github.com/google/re2/wiki/Syntax">regular expression</a> <code>regex</code> against the
        value of the label <code>src_label</code>. If it matches, the value of the label <code>dst_label</code> in the
        returned timeseries will be the expansion of <code>replacement</code>, together with the original labels in the
        input. Capturing groups in the regular expression can be referenced with <code>$1</code>, <code>$2</code>, etc. Named
        capturing groups in the regular expression can be referenced with <code>$name</code> (where <code>name</code> is the
        capturing group name). If the regular expression doesn&rsquo;t match then the timeseries is returned unchanged.
      </p>

      <p>
        <code>label_replace</code> acts on float and histogram samples in the same way.
      </p>

      <p>
        This example will return timeseries with the values <code>a:c</code> at label <code>service</code> and <code>a</code>{' '}
        at label <code>foo</code>:
      </p>

      <pre>
        <code>
          label_replace(up{'{'}job=&quot;api-server&quot;,service=&quot;a:c&quot;{'}'}, &quot;foo&quot;, &quot;$1&quot;,
          &quot;service&quot;, &quot;(.*):.*&quot;)
        </code>
      </pre>

      <p>This second example has the same effect than the first example, and illustrates use of named capturing groups:</p>

      <pre>
        <code>
          label_replace(up{'{'}job=&quot;api-server&quot;,service=&quot;a:c&quot;{'}'}, &quot;foo&quot;, &quot;$name&quot;,
          &quot;service&quot;, &quot;(?P&lt;name&gt;.*):(?P&lt;version&gt;.*)&quot;)
        </code>
      </pre>
    </>
  ),
  last_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  ln: (
    <>
      <p>
        <code>ln(v instant-vector)</code> calculates the natural logarithm for all elements in <code>v</code>. Special cases
        are:
      </p>

      <ul>
        <li>
          <code>ln(+Inf) = +Inf</code>
        </li>
        <li>
          <code>ln(0) = -Inf</code>
        </li>
        <li>
          <code>ln(x &lt; 0) = NaN</code>
        </li>
        <li>
          <code>ln(NaN) = NaN</code>
        </li>
      </ul>
    </>
  ),
  log10: (
    <>
      <p>
        <code>log10(v instant-vector)</code> calculates the decimal logarithm for all elements in <code>v</code>. The special
        cases are equivalent to those in <code>ln</code>.
      </p>
    </>
  ),
  log2: (
    <>
      <p>
        <code>log2(v instant-vector)</code> calculates the binary logarithm for all elements in <code>v</code>. The special
        cases are equivalent to those in <code>ln</code>.
      </p>
    </>
  ),
  mad_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  max_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  min_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  minute: (
    <>
      <p>
        <code>minute(v=vector(time()) instant-vector)</code> returns the minute of the hour for each of the given times in
        UTC. Returned values are from 0 to 59.
      </p>
    </>
  ),
  month: (
    <>
      <p>
        <code>month(v=vector(time()) instant-vector)</code> returns the month of the year for each of the given times in UTC.
        Returned values are from 1 to 12, where 1 means January etc.
      </p>
    </>
  ),
  pi: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  predict_linear: (
    <>
      <p>
        <code>predict_linear(v range-vector, t scalar)</code> predicts the value of time series
        <code>t</code> seconds from now, based on the range vector <code>v</code>, using{' '}
        <a href="https://en.wikipedia.org/wiki/Simple_linear_regression">simple linear regression</a>. The range vector must
        have at least two samples in order to perform the calculation. When <code>+Inf</code> or <code>-Inf</code> are found
        in the range vector, the slope and offset value calculated will be <code>NaN</code>.
      </p>

      <p>
        <code>predict_linear</code> should only be used with gauges.
      </p>
    </>
  ),
  present_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  quantile_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  rad: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  rate: (
    <>
      <p>
        <code>rate(v range-vector)</code> calculates the per-second average rate of increase of the time series in the range
        vector. Breaks in monotonicity (such as counter resets due to target restarts) are automatically adjusted for. Also,
        the calculation extrapolates to the ends of the time range, allowing for missed scrapes or imperfect alignment of
        scrape cycles with the range&rsquo;s time period.
      </p>

      <p>
        The following example expression returns the per-second rate of HTTP requests as measured over the last 5 minutes,
        per time series in the range vector:
      </p>

      <pre>
        <code>
          rate(http_requests_total{'{'}job=&quot;api-server&quot;{'}'}[5m])
        </code>
      </pre>

      <p>
        <code>rate</code> acts on native histograms by calculating a new histogram where each component (sum and count of
        observations, buckets) is the rate of increase between the respective component in the first and last native
        histogram in
        <code>v</code>. However, each element in <code>v</code> that contains a mix of float and native histogram samples
        within the range, will be missing from the result vector.
      </p>

      <p>
        <code>rate</code> should only be used with counters and native histograms where the components behave like counters.
        It is best suited for alerting, and for graphing of slow-moving counters.
      </p>

      <p>
        Note that when combining <code>rate()</code> with an aggregation operator (e.g. <code>sum()</code>) or a function
        aggregating over time (any function ending in <code>_over_time</code>), always take a <code>rate()</code> first, then
        aggregate. Otherwise <code>rate()</code> cannot detect counter resets when your target restarts.
      </p>
    </>
  ),
  resets: (
    <>
      <p>
        For each input time series, <code>resets(v range-vector)</code> returns the number of counter resets within the
        provided time range as an instant vector. Any decrease in the value between two consecutive float samples is
        interpreted as a counter reset. A reset in a native histogram is detected in a more complex way: Any decrease in any
        bucket, including the zero bucket, or in the count of observation constitutes a counter reset, but also the
        disappearance of any previously populated bucket, an increase in bucket resolution, or a decrease of the zero-bucket
        width.
      </p>

      <p>
        <code>resets</code> should only be used with counters and counter-like native histograms.
      </p>

      <p>
        If the range vector contains a mix of float and histogram samples for the same series, counter resets are detected
        separately and their numbers added up. The change from a float to a histogram sample is <em>not</em> considered a
        counter reset. Each float sample is compared to the next float sample, and each histogram is comprared to the next
        histogram.
      </p>
    </>
  ),
  round: (
    <>
      <p>
        <code>round(v instant-vector, to_nearest=1 scalar)</code> rounds the sample values of all elements in <code>v</code>{' '}
        to the nearest integer. Ties are resolved by rounding up. The optional <code>to_nearest</code> argument allows
        specifying the nearest multiple to which the sample values should be rounded. This multiple may also be a fraction.
      </p>
    </>
  ),
  scalar: (
    <>
      <p>
        Given a single-element input vector, <code>scalar(v instant-vector)</code> returns the sample value of that single
        element as a scalar. If the input vector does not have exactly one element, <code>scalar</code> will return{' '}
        <code>NaN</code>.
      </p>
    </>
  ),
  sgn: (
    <>
      <p>
        <code>sgn(v instant-vector)</code> returns a vector with all sample values converted to their sign, defined as this:
        1 if v is positive, -1 if v is negative and 0 if v is equal to zero.
      </p>
    </>
  ),
  sin: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  sinh: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  sort: (
    <>
      <p>
        <code>sort(v instant-vector)</code> returns vector elements sorted by their sample values, in ascending order. Native
        histograms are sorted by their sum of observations.
      </p>

      <p>
        Please note that <code>sort</code> only affects the results of instant queries, as range query results always have a
        fixed output ordering.
      </p>
    </>
  ),
  sort_by_label: (
    <>
      <p>
        <strong>
          This function has to be enabled via the{' '}
          <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>{' '}
          <code>--enable-feature=promql-experimental-functions</code>.
        </strong>
      </p>

      <p>
        <code>sort_by_label(v instant-vector, label string, ...)</code> returns vector elements sorted by the values of the
        given labels in ascending order. In case these label values are equal, elements are sorted by their full label sets.
      </p>

      <p>
        Please note that the sort by label functions only affect the results of instant queries, as range query results
        always have a fixed output ordering.
      </p>

      <p>
        This function uses <a href="https://en.wikipedia.org/wiki/Natural_sort_order">natural sort order</a>.
      </p>
    </>
  ),
  sort_by_label_desc: (
    <>
      <p>
        <strong>
          This function has to be enabled via the{' '}
          <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>{' '}
          <code>--enable-feature=promql-experimental-functions</code>.
        </strong>
      </p>

      <p>
        Same as <code>sort_by_label</code>, but sorts in descending order.
      </p>

      <p>
        Please note that the sort by label functions only affect the results of instant queries, as range query results
        always have a fixed output ordering.
      </p>

      <p>
        This function uses <a href="https://en.wikipedia.org/wiki/Natural_sort_order">natural sort order</a>.
      </p>
    </>
  ),
  sort_desc: (
    <>
      <p>
        Same as <code>sort</code>, but sorts in descending order.
      </p>

      <p>
        Like <code>sort</code>, <code>sort_desc</code> only affects the results of instant queries, as range query results
        always have a fixed output ordering.
      </p>
    </>
  ),
  sqrt: (
    <>
      <p>
        <code>sqrt(v instant-vector)</code> calculates the square root of all elements in <code>v</code>.
      </p>
    </>
  ),
  stddev_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  stdvar_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  sum_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant vector
        with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all points in the specified interval.
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all points in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all points in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all values in the specified interval.
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all values in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of the values in the specified
          interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of the values in the specified
          interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of the values in the specified
          interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent point value in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all points in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are not
        equally spaced throughout the interval.
      </p>

      <p>
        <code>avg_over_time</code>, <code>sum_over_time</code>, <code>count_over_time</code>, <code>last_over_time</code>,
        and
        <code>present_over_time</code> handle native histograms as expected. All other functions ignore histogram samples.
      </p>
    </>
  ),
  tan: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  tanh: (
    <>
      <p>The trigonometric functions work in radians:</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all elements in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all elements in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all elements in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  time: (
    <>
      <p>
        <code>time()</code> returns the number of seconds since January 1, 1970 UTC. Note that this does not actually return
        the current time, but the time at which the expression is to be evaluated.
      </p>
    </>
  ),
  timestamp: (
    <>
      <p>
        <code>timestamp(v instant-vector)</code> returns the timestamp of each of the samples of the given vector as the
        number of seconds since January 1, 1970 UTC. It also works with histogram samples.
      </p>
    </>
  ),
  vector: (
    <>
      <p>
        <code>vector(s scalar)</code> returns the scalar <code>s</code> as a vector with no labels.
      </p>
    </>
  ),
  year: (
    <>
      <p>
        <code>year(v=vector(time()) instant-vector)</code> returns the year for each of the given times in UTC.
      </p>
    </>
  ),
};

export default funcDocs;
