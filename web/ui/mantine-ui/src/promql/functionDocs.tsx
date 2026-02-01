import React from "react";

const funcDocs: Record<string, React.ReactNode> = {
  abs: (
    <>
      <p>
        <code>abs(v instant-vector)</code> returns a vector containing all float samples in the input vector converted
        to their absolute value. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  absent: (
    <>
      <p>
        <code>absent(v instant-vector)</code> returns an empty vector if the vector passed to it has any elements (float
        samples or histogram samples) and a 1-element vector with the value 1 if the vector passed to it has no
        elements.
      </p>

      <p>This is useful for alerting on when no time series exist for a given metric name and label combination.</p>

      <pre>
        <code>
          absent(nonexistent{"{"}job=&quot;myjob&quot;{"}"}) # =&gt; {"{"}job=&quot;myjob&quot;{"}"}
          absent(nonexistent{"{"}job=&quot;myjob&quot;,instance=~&quot;.*&quot;{"}"}) # =&gt; {"{"}job=&quot;myjob&quot;
          {"}"}
          absent(sum(nonexistent{"{"}job=&quot;myjob&quot;{"}"})) # =&gt; {"{"}
          {"}"}
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
        elements (float samples or histogram samples) and a 1-element vector with the value 1 if the range vector passed
        to it has no elements.
      </p>

      <p>
        This is useful for alerting on when no time series exist for a given metric name and label combination for a
        certain amount of time.
      </p>

      <pre>
        <code>
          absent_over_time(nonexistent{"{"}job=&quot;myjob&quot;{"}"}[1h]) # =&gt; {"{"}job=&quot;myjob&quot;{"}"}
          absent_over_time(nonexistent{"{"}job=&quot;myjob&quot;,instance=~&quot;.*&quot;{"}"}[1h]) # =&gt; {"{"}
          job=&quot;myjob&quot;{"}"}
          absent_over_time(sum(nonexistent{"{"}job=&quot;myjob&quot;{"}"})[1h:]) # =&gt; {"{"}
          {"}"}
        </code>
      </pre>

      <p>
        In the first two examples, <code>absent_over_time()</code> tries to be smart about deriving labels of the
        1-element output vector from the input vector.
      </p>
    </>
  ),
  acos: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  acosh: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  asin: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  asinh: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  atan: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  atanh: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  avg_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  ceil: (
    <>
      <p>
        <code>ceil(v instant-vector)</code> returns a vector containing all float samples in the input vector rounded up
        to the nearest integer value greater than or equal to their original value. Histogram samples in the input
        vector are ignored silently.
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
        For each input time series, <code>changes(v range-vector)</code> returns the number of times its value has
        changed within the provided time range as an instant vector. A float sample followed by a histogram sample, or
        vice versa, counts as a change. A counter histogram sample followed by a gauge histogram sample with otherwise
        exactly the same values, or vice versa, does not count as a change.
      </p>
    </>
  ),
  clamp: (
    <>
      <p>
        <code>clamp(v instant-vector, min scalar, max scalar)</code> clamps the values of all float samples in{" "}
        <code>v</code> to have a lower limit of <code>min</code> and an upper limit of
        <code>max</code>. Histogram samples in the input vector are ignored silently.
      </p>

      <p>Special cases:</p>

      <ul>
        <li>
          Return an empty vector if <code>min &gt; max</code>
        </li>
        <li>
          Float samples are clamped to <code>NaN</code> if <code>min</code> or <code>max</code> is <code>NaN</code>
        </li>
      </ul>
    </>
  ),
  clamp_max: (
    <>
      <p>
        <code>clamp_max(v instant-vector, max scalar)</code> clamps the values of all float samples in <code>v</code> to
        have an upper limit of <code>max</code>. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  clamp_min: (
    <>
      <p>
        <code>clamp_min(v instant-vector, min scalar)</code> clamps the values of all float samples in <code>v</code> to
        have a lower limit of <code>min</code>. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  cos: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  cosh: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  count_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  day_of_month: (
    <>
      <p>
        <code>day_of_month(v=vector(time()) instant-vector)</code> interprets float samples in
        <code>v</code> as timestamps (number of seconds since January 1, 1970 UTC) and returns the day of the month (in
        UTC) for each of those timestamps. Returned values are from 1 to 31. Histogram samples in the input vector are
        ignored silently.
      </p>
    </>
  ),
  day_of_week: (
    <>
      <p>
        <code>day_of_week(v=vector(time()) instant-vector)</code> interprets float samples in <code>v</code>
        as timestamps (number of seconds since January 1, 1970 UTC) and returns the day of the week (in UTC) for each of
        those timestamps. Returned values are from 0 to 6, where 0 means Sunday etc. Histogram samples in the input
        vector are ignored silently.
      </p>
    </>
  ),
  day_of_year: (
    <>
      <p>
        <code>day_of_year(v=vector(time()) instant-vector)</code> interprets float samples in <code>v</code>
        as timestamps (number of seconds since January 1, 1970 UTC) and returns the day of the year (in UTC) for each of
        those timestamps. Returned values are from 1 to 365 for non-leap years, and 1 to 366 in leap years. Histogram
        samples in the input vector are ignored silently.
      </p>
    </>
  ),
  days_in_month: (
    <>
      <p>
        <code>days_in_month(v=vector(time()) instant-vector)</code> interprets float samples in
        <code>v</code> as timestamps (number of seconds since January 1, 1970 UTC) and returns the number of days in the
        month of each of those timestamps (in UTC). Returned values are from 28 to 31. Histogram samples in the input
        vector are ignored silently.
      </p>
    </>
  ),
  deg: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  delta: (
    <>
      <p>
        <code>delta(v range-vector)</code> calculates the difference between the first and last value of each time
        series element in a range vector <code>v</code>, returning an instant vector with the given deltas and
        equivalent labels. The delta is extrapolated to cover the full time range as specified in the range vector
        selector, so that it is possible to get a non-integer result even if the sample values are all integers.
      </p>

      <p>The following example expression returns the difference in CPU temperature between now and 2 hours ago:</p>

      <pre>
        <code>
          delta(cpu_temp_celsius{"{"}host=&quot;zeus&quot;{"}"}[2h])
        </code>
      </pre>

      <p>
        <code>delta</code> acts on histogram samples by calculating a new histogram where each component (sum and count
        of observations, buckets) is the difference between the respective component in the first and last native
        histogram in <code>v</code>. However, each element in <code>v</code> that contains a mix of float samples and
        histogram samples within the range will be omitted from the result vector, flagged by a warn-level annotation.
      </p>

      <p>
        <code>delta</code> should only be used with gauges (for both floats and histograms).
      </p>
    </>
  ),
  deriv: (
    <>
      <p>
        <code>deriv(v range-vector)</code> calculates the per-second derivative of each float time series in the range
        vector <code>v</code>, using{" "}
        <a href="https://en.wikipedia.org/wiki/Simple_linear_regression">simple linear regression</a>. The range vector
        must have at least two float samples in order to perform the calculation. When <code>+Inf</code> or{" "}
        <code>-Inf</code> are found in the range vector, the slope and offset value calculated will be <code>NaN</code>.
      </p>

      <p>
        <code>deriv</code> should only be used with gauges and only works for float samples. Elements in the range
        vector that contain only histogram samples are ignored entirely. For elements that contain a mix of float and
        histogram samples, only the float samples are used as input, which is flagged by an info-level annotation.
      </p>
    </>
  ),
  double_exponential_smoothing: (
    <>
      <p>
        <strong>
          This function has to be enabled via the{" "}
          <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
          <code>--enable-feature=promql-experimental-functions</code>.
        </strong>
      </p>

      <p>
        <code>double_exponential_smoothing(v range-vector, sf scalar, tf scalar)</code> produces a smoothed value for
        each float time series in the range in <code>v</code>. The lower the smoothing factor <code>sf</code>, the more
        importance is given to old data. The higher the trend factor <code>tf</code>, the more trends in the data is
        considered. Both <code>sf</code> and
        <code>tf</code> must be between 0 and 1. For additional details, refer to{" "}
        <a href="https://www.itl.nist.gov/div898/handbook/pmc/section4/pmc433.htm">
          NIST Engineering Statistics Handbook
        </a>
        . In Prometheus V2 this function was called <code>holt_winters</code>. This caused confusion since the
        Holt-Winters method usually refers to triple exponential smoothing. Double exponential smoothing as implemented
        here is also referred to as &ldquo;Holt Linear&rdquo;.
      </p>

      <p>
        <code>double_exponential_smoothing</code> should only be used with gauges and only works for float samples.
        Elements in the range vector that contain only histogram samples are ignored entirely. For elements that contain
        a mix of float and histogram samples, only the float samples are used as input, which is flagged by an
        info-level annotation.
      </p>
    </>
  ),
  exp: (
    <>
      <p>
        <code>exp(v instant-vector)</code> calculates the exponential function for all float samples in <code>v</code>.
        Histogram samples are ignored silently. Special cases are:
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
  first_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  floor: (
    <>
      <p>
        <code>floor(v instant-vector)</code> returns a vector containing all float samples in the input vector rounded
        down to the nearest integer value smaller than or equal to their original value. Histogram samples in the input
        vector are ignored silently.
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
        <code>histogram_avg(v instant-vector)</code> returns the arithmetic average of observed values stored in each
        histogram sample in <code>v</code>. Float samples are ignored and do not show up in the returned vector.
      </p>

      <p>
        Use <code>histogram_avg</code> as demonstrated below to compute the average request duration over a 5-minute
        window from a native histogram:
      </p>

      <pre>
        <code>histogram_avg(rate(http_request_duration_seconds[5m]))</code>
      </pre>

      <p>Which is equivalent to the following query:</p>

      <pre>
        <code>
          {" "}
          histogram_sum(rate(http_request_duration_seconds[5m])) /
          histogram_count(rate(http_request_duration_seconds[5m]))
        </code>
      </pre>
    </>
  ),
  histogram_count: (
    <>
      <p>
        <code>histogram_count(v instant-vector)</code> returns the count of observations stored in each histogram sample
        in <code>v</code>. Float samples are ignored and do not show up in the returned vector.
      </p>

      <p>
        Similarly, <code>histogram_sum(v instant-vector)</code> returns the sum of observations stored in each histogram
        sample.
      </p>

      <p>
        Use <code>histogram_count</code> in the following way to calculate a rate of observations (in this case
        corresponding to “requests per second”) from a series of histogram samples:
      </p>

      <pre>
        <code>histogram_count(rate(http_request_duration_seconds[10m]))</code>
      </pre>
    </>
  ),
  histogram_fraction: (
    <>
      <p>
        <code>histogram_fraction(lower scalar, upper scalar, b instant-vector)</code> returns the estimated fraction of
        observations between the provided lower and upper values for each classic or native histogram contained in{" "}
        <code>b</code>. Float samples in <code>b</code> are considered the counts of observations in each bucket of one
        or more classic histograms, while native histogram samples in <code>b</code> are treated each individually as a
        separate histogram. This works in the same way as for <code>histogram_quantile()</code>. (See there for more
        details.)
      </p>

      <p>
        If the provided lower and upper values do not coincide with bucket boundaries, the calculated fraction is an
        estimate, using the same interpolation method as for
        <code>histogram_quantile()</code>. (See there for more details.) Especially with classic histograms, it is easy
        to accidentally pick lower or upper values that are very far away from any bucket boundary, leading to large
        margins of error. Rather than using <code>histogram_fraction()</code> with classic histograms, it is often a
        more robust approach to directly act on the bucket series when calculating fractions. See the
        <a href="https://prometheus.io/docs/practices/histograms/#apdex-score">calculation of the Apdex score</a>
        as a typical example.
      </p>

      <p>
        For example, the following expression calculates the fraction of HTTP requests over the last hour that took
        200ms or less:
      </p>

      <pre>
        <code>histogram_fraction(0, 0.2, rate(http_request_duration_seconds[1h]))</code>
      </pre>

      <p>
        The error of the estimation depends on the resolution of the underlying native histogram and how closely the
        provided boundaries are aligned with the bucket boundaries in the histogram.
      </p>

      <p>
        <code>+Inf</code> and <code>-Inf</code> are valid boundary values. For example, if the histogram in the
        expression above included negative observations (which shouldn&rsquo;t be the case for request durations), the
        appropriate lower boundary to include all observations less than or equal 0.2 would be <code>-Inf</code> rather
        than <code>0</code>.
      </p>

      <p>
        Whether the provided boundaries are inclusive or exclusive is only relevant if the provided boundaries are
        precisely aligned with bucket boundaries in the underlying native histogram. In this case, the behavior depends
        on the schema definition of the histogram. (The usual standard exponential schemas all feature inclusive upper
        boundaries and exclusive lower boundaries for positive values, and vice versa for negative values.) Without a
        precise alignment of boundaries, the function uses interpolation to estimate the fraction. With the resulting
        uncertainty, it becomes irrelevant if the boundaries are inclusive or exclusive.
      </p>

      <p>
        Special case for native histograms with standard exponential buckets:
        <code>NaN</code> observations are considered outside of any buckets in this case.
        <code>histogram_fraction(-Inf, +Inf, b)</code> effectively returns the fraction of non-<code>NaN</code>{" "}
        observations and may therefore be less than 1.
      </p>
    </>
  ),
  histogram_quantile: (
    <>
      <p>
        <code>histogram_quantile(φ scalar, b instant-vector)</code> calculates the φ-quantile (0 ≤ φ ≤ 1) from a{" "}
        <a href="https://prometheus.io/docs/concepts/metric_types/#histogram">classic histogram</a> or from a native
        histogram. (See <a href="https://prometheus.io/docs/practices/histograms">histograms and summaries</a> for a
        detailed explanation of φ-quantiles and the usage of the (classic) histogram metric type in general.)
      </p>

      <p>
        The float samples in <code>b</code> are considered the counts of observations in each bucket of one or more
        classic histograms. Each float sample must have a label
        <code>le</code> where the label value denotes the inclusive upper bound of the bucket. (Float samples without
        such a label are silently ignored.) The other labels and the metric name are used to identify the buckets
        belonging to each classic histogram. The{" "}
        <a href="https://prometheus.io/docs/concepts/metric_types/#histogram">histogram metric type</a>
        automatically provides time series with the <code>_bucket</code> suffix and the appropriate labels.
      </p>

      <p>
        The (native) histogram samples in <code>b</code> are treated each individually as a separate histogram to
        calculate the quantile from.
      </p>

      <p>
        As long as no naming collisions arise, <code>b</code> may contain a mix of classic and native histograms.
      </p>

      <p>
        Use the <code>rate()</code> function to specify the time window for the quantile calculation.
      </p>

      <p>
        Example: A histogram metric is called <code>http_request_duration_seconds</code> (and therefore the metric name
        for the buckets of a classic histogram is
        <code>http_request_duration_seconds_bucket</code>). To calculate the 90th percentile of request durations over
        the last 10m, use the following expression in case
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
        <code>http_request_duration_seconds</code>. To aggregate, use the <code>sum()</code> aggregator around the{" "}
        <code>rate()</code> function. Since the <code>le</code> label is required by
        <code>histogram_quantile()</code> to deal with classic histograms, it has to be included in the <code>by</code>{" "}
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
        In the (common) case that a quantile value does not coincide with a bucket boundary, the{" "}
        <code>histogram_quantile()</code> function interpolates the quantile value within the bucket the quantile value
        falls into. For classic histograms, for native histograms with custom bucket boundaries, and for the zero bucket
        of other native histograms, it assumes a uniform distribution of observations within the bucket (also called{" "}
        <em>linear interpolation</em>). For the non-zero-buckets of native histograms with a standard exponential
        bucketing schema, the interpolation is done under the assumption that the samples within the bucket are
        distributed in a way that they would uniformly populate the buckets in a hypothetical histogram with higher
        resolution. (This is also called <em>exponential interpolation</em>. See the{" "}
        <a href="https://prometheus.io/docs/specs/native_histograms/#interpolation-within-a-bucket">
          native histogram specification
        </a>
        for more details.)
      </p>

      <p>
        If <code>b</code> has 0 observations, <code>NaN</code> is returned. For φ &lt; 0, <code>-Inf</code> is returned.
        For φ &gt; 1, <code>+Inf</code> is returned. For φ = <code>NaN</code>, <code>NaN</code> is returned.
      </p>

      <p>Special cases for classic histograms:</p>

      <ul>
        <li>
          If <code>b</code> contains fewer than two buckets, <code>NaN</code> is returned.
        </li>
        <li>
          The highest bucket must have an upper bound of <code>+Inf</code>. (Otherwise, <code>NaN</code> is returned.)
        </li>
        <li>
          If a quantile is located in the highest bucket, the upper bound of the second highest bucket is returned.
        </li>
        <li>
          The lower limit of the lowest bucket is assumed to be 0 if the upper bound of that bucket is greater than 0.
          In that case, the usual linear interpolation is applied within that bucket. Otherwise, the upper bound of the
          lowest bucket is returned for quantiles located in the lowest bucket.
        </li>
      </ul>

      <p>Special cases for native histograms:</p>

      <ul>
        <li>
          If a native histogram with standard exponential buckets has <code>NaN</code>
          observations and the quantile falls into one of the existing exponential buckets, the result is skewed towards
          higher values due to <code>NaN</code>
          observations treated as <code>+Inf</code>. This is flagged with an info level annotation.
        </li>
        <li>
          If a native histogram with standard exponential buckets has <code>NaN</code>
          observations and the quantile falls above all of the existing exponential buckets, <code>NaN</code> is
          returned. This is flagged with an info level annotation.
        </li>
        <li>
          A zero bucket with finite width is assumed to contain no negative observations if the histogram has
          observations in positive buckets, but none in negative buckets.
        </li>
        <li>
          A zero bucket with finite width is assumed to contain no positive observations if the histogram has
          observations in negative buckets, but none in positive buckets.
        </li>
      </ul>

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
          A lack of observations between the upper limits of two consecutive buckets results in equal counts in those
          two buckets.
        </li>
      </ul>

      <p>
        However, floating point precision issues (e.g. small discrepancies introduced by computing of buckets with{" "}
        <code>sum(rate(...))</code>) or invalid data might violate these assumptions. In that case,{" "}
        <code>histogram_quantile</code> would be unable to return meaningful results. To mitigate the issue,{" "}
        <code>histogram_quantile</code> assumes that tiny relative differences between consecutive buckets are happening
        because of floating point precision errors and ignores them. (The threshold to ignore a difference between two
        buckets is a trillionth (1e-12) of the sum of both buckets.) Furthermore, if there are non-monotonic bucket
        counts even after this adjustment, they are increased to the value of the previous buckets to enforce
        monotonicity. The latter is evidence for an actual issue with the input data and is therefore flagged by an
        info-level annotation reading <code>input to histogram_quantile needed to be fixed for monotonicity</code>. If
        you encounter this annotation, you should find and remove the source of the invalid data.
      </p>
    </>
  ),
  histogram_stddev: (
    <>
      <p>
        <code>histogram_stddev(v instant-vector)</code> returns the estimated standard deviation of observations for
        each histogram sample in <code>v</code>. For this estimation, all observations in a bucket are assumed to have
        the value of the mean of the bucket boundaries. For the zero bucket and for buckets with custom boundaries, the
        arithmetic mean is used. For the usual exponential buckets, the geometric mean is used. Float samples are
        ignored and do not show up in the returned vector.
      </p>

      <p>
        Similarly, <code>histogram_stdvar(v instant-vector)</code> returns the estimated standard variance of
        observations for each histogram sample in <code>v</code>.
      </p>
    </>
  ),
  histogram_stdvar: (
    <>
      <p>
        <code>histogram_stddev(v instant-vector)</code> returns the estimated standard deviation of observations for
        each histogram sample in <code>v</code>. For this estimation, all observations in a bucket are assumed to have
        the value of the mean of the bucket boundaries. For the zero bucket and for buckets with custom boundaries, the
        arithmetic mean is used. For the usual exponential buckets, the geometric mean is used. Float samples are
        ignored and do not show up in the returned vector.
      </p>

      <p>
        Similarly, <code>histogram_stdvar(v instant-vector)</code> returns the estimated standard variance of
        observations for each histogram sample in <code>v</code>.
      </p>
    </>
  ),
  histogram_sum: (
    <>
      <p>
        <code>histogram_count(v instant-vector)</code> returns the count of observations stored in each histogram sample
        in <code>v</code>. Float samples are ignored and do not show up in the returned vector.
      </p>

      <p>
        Similarly, <code>histogram_sum(v instant-vector)</code> returns the sum of observations stored in each histogram
        sample.
      </p>

      <p>
        Use <code>histogram_count</code> in the following way to calculate a rate of observations (in this case
        corresponding to “requests per second”) from a series of histogram samples:
      </p>

      <pre>
        <code>histogram_count(rate(http_request_duration_seconds[10m]))</code>
      </pre>
    </>
  ),
  hour: (
    <>
      <p>
        <code>hour(v=vector(time()) instant-vector)</code> interprets float samples in <code>v</code> as timestamps
        (number of seconds since January 1, 1970 UTC) and returns the hour of the day (in UTC) for each of those
        timestamps. Returned values are from 0 to 23. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  idelta: (
    <>
      <p>
        <code>idelta(v range-vector)</code> calculates the difference between the last two samples in the range vector{" "}
        <code>v</code>, returning an instant vector with the given deltas and equivalent labels. Both samples must be
        either float samples or histogram samples. Elements in <code>v</code> where one of the last two samples is a
        float sample and the other is a histogram sample will be omitted from the result vector, flagged by a warn-level
        annotation.
      </p>

      <p>
        <code>idelta</code> should only be used with gauges (for both floats and histograms).
      </p>
    </>
  ),
  increase: (
    <>
      <p>
        <code>increase(v range-vector)</code> calculates the increase in the time series in the range vector. Breaks in
        monotonicity (such as counter resets due to target restarts) are automatically adjusted for. The increase is
        extrapolated to cover the full time range as specified in the range vector selector, so that it is possible to
        get a non-integer result even if a counter increases only by integer increments.
      </p>

      <p>
        The following example expression returns the number of HTTP requests as measured over the last 5 minutes, per
        time series in the range vector:
      </p>

      <pre>
        <code>
          increase(http_requests_total{"{"}job=&quot;api-server&quot;{"}"}[5m])
        </code>
      </pre>

      <p>
        <code>increase</code> acts on histogram samples by calculating a new histogram where each component (sum and
        count of observations, buckets) is the increase between the respective component in the first and last native
        histogram in <code>v</code>. However, each element in <code>v</code> that contains a mix of float samples and
        histogram samples within the range, will be omitted from the result vector, flagged by a warn-level annotation.
      </p>

      <p>
        <code>increase</code> should only be used with counters (for both floats and histograms). It is syntactic sugar
        for <code>rate(v)</code> multiplied by the number of seconds under the specified time range window, and should
        be used primarily for human readability. Use <code>rate</code> in recording rules so that increases are tracked
        consistently on a per-second basis.
      </p>
    </>
  ),
  info: (
    <>
      <p>
        _The <code>info</code> function is an experiment to improve UX around including labels from{" "}
        <a href="https://grafana.com/blog/2021/08/04/how-to-use-promql-joins-for-more-effective-queries-of-prometheus-metrics-at-scale/#info-metrics">
          info metrics
        </a>
        . The behavior of this function may change in future versions of Prometheus, including its removal from PromQL.{" "}
        <code>info</code> has to be enabled via the
        <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>{" "}
        <code>--enable-feature=promql-experimental-functions</code>._
      </p>

      <p>
        <code>info(v instant-vector, [data-label-selector instant-vector])</code> finds, for each time series in{" "}
        <code>v</code>, all info series with matching <em>identifying</em> labels (more on this later), and adds the
        union of their <em>data</em> (i.e., non-identifying) labels to the time series. The second argument{" "}
        <code>data-label-selector</code> is optional. It is not a real instant vector, but uses a subset of its syntax.
        It must start and end with curly braces (
        <code>
          {"{"} ... {"}"}
        </code>
        ) and may only contain label matchers. The label matchers are used to constrain which info series to consider
        and which data labels to add to <code>v</code>.
      </p>

      <p>
        Identifying labels of an info series are the subset of labels that uniquely identify the info series. The
        remaining labels are considered
        <em>data labels</em> (also called non-identifying). (Note that Prometheus&rsquo;s concept of time series
        identity always includes <em>all</em> the labels. For the sake of the <code>info</code>
        function, we “logically” define info series identity in a different way than in the conventional Prometheus
        view.) The identifying labels of an info series are used to join it to regular (non-info) series, i.e. those
        series that have the same labels as the identifying labels of the info series. The data labels, which are the
        ones added to the regular series by the <code>info</code> function, effectively encode metadata key value pairs.
        (This implies that a change in the data labels in the conventional Prometheus view constitutes the end of one
        info series and the beginning of a new info series, while the “logical” view of the <code>info</code> function
        is that the same info series continues to exist, just with different “data”.)
      </p>

      <p>
        The conventional approach of adding data labels is sometimes called a “join query”, as illustrated by the
        following example:
      </p>

      <pre>
        <code>
          {" "}
          rate(http_server_request_duration_seconds_count[2m]) * on (job, instance) group_left (k8s_cluster_name)
          target_info
        </code>
      </pre>

      <p>
        The core of the query is the expression <code>rate(http_server_request_duration_seconds_count[2m])</code>. But
        to add data labels from an info metric, the user has to use elaborate (and not very obvious) syntax to specify
        which info metric to use (<code>target_info</code>), what the identifying labels are (
        <code>on (job, instance)</code>), and which data labels to add (<code>group_left (k8s_cluster_name)</code>).
      </p>

      <p>
        This query is not only verbose and hard to write, it might also run into an “identity crisis”: If any of the
        data labels of <code>target_info</code> changes, Prometheus sees that as a change of series (as alluded to
        above, Prometheus just has no native concept of non-identifying labels). If the old <code>target_info</code>{" "}
        series is not properly marked as stale (which can happen with certain ingestion paths), the query above will
        fail for up to 5m (the lookback delta) because it will find a conflicting match with both the old and the new
        version of <code>target_info</code>.
      </p>

      <p>
        The <code>info</code> function not only resolves this conflict in favor of the newer series, it also simplifies
        the syntax because it knows about the available info series and what their identifying labels are. The example
        query looks like this with the <code>info</code> function:
      </p>

      <pre>
        <code>
          info( rate(http_server_request_duration_seconds_count[2m]),
          {"{"}k8s_cluster_name=~&quot;.+&quot;{"}"})
        </code>
      </pre>

      <p>
        The common case of adding <em>all</em> data labels can be achieved by omitting the 2nd argument of the{" "}
        <code>info</code> function entirely, simplifying the example even more:
      </p>

      <pre>
        <code>info(rate(http_server_request_duration_seconds_count[2m]))</code>
      </pre>

      <p>
        While <code>info</code> normally automatically finds all matching info series, it&rsquo;s possible to restrict
        them by providing a <code>__name__</code> label matcher, e.g.
        <code>
          {"{"}__name__=&quot;target_info&quot;{"}"}
        </code>
        .
      </p>

      <p>
        Note that if there are any time series in <code>v</code> that match the <code>data-label-selector</code> (or the
        default <code>target_info</code> if that argument is not specified), they will be treated as info series and
        will be returned unchanged.
      </p>

      <h3>Limitations</h3>

      <p>
        In its current iteration, <code>info</code> defaults to considering only info series with the name{" "}
        <code>target_info</code>. It also assumes that the identifying info series labels are
        <code>instance</code> and <code>job</code>. <code>info</code> does support other info series names however,
        through
        <code>__name__</code> label matchers. E.g., one can explicitly say to consider both
        <code>target_info</code> and <code>build_info</code> as follows:
        <code>
          {"{"}__name__=~&quot;(target|build)_info&quot;{"}"}
        </code>
        . However, the identifying labels always have to be <code>instance</code> and <code>job</code>.
      </p>

      <p>
        These limitations are partially defeating the purpose of the <code>info</code> function. At the current stage,
        this is an experiment to find out how useful the approach turns out to be in practice. A final version of the{" "}
        <code>info</code> function will indeed consider all matching info series and with their appropriate identifying
        labels.
      </p>
    </>
  ),
  irate: (
    <>
      <p>
        <code>irate(v range-vector)</code> calculates the per-second instant rate of increase of the time series in the
        range vector. This is based on the last two data points. Breaks in monotonicity (such as counter resets due to
        target restarts) are automatically adjusted for. Both samples must be either float samples or histogram samples.
        Elements in <code>v</code> where one of the last two samples is a float sample and the other is a histogram
        sample will be omitted from the result vector, flagged by a warn-level annotation.
      </p>

      <p>
        <code>irate</code> should only be used with counters (for both floats and histograms).
      </p>

      <p>
        The following example expression returns the per-second rate of HTTP requests looking up to 5 minutes back for
        the two most recent data points, per time series in the range vector:
      </p>

      <pre>
        <code>
          irate(http_requests_total{"{"}job=&quot;api-server&quot;{"}"}[5m])
        </code>
      </pre>

      <p>
        <code>irate</code> should only be used when graphing volatile, fast-moving counters. Use <code>rate</code> for
        alerts and slow-moving counters, as brief changes in the rate can reset the <code>FOR</code> clause and graphs
        consisting entirely of rare spikes are hard to read.
      </p>

      <p>
        Note that when combining <code>irate()</code> with an
        <a href="operators.md#aggregation-operators">aggregation operator</a> (e.g. <code>sum()</code>) or a function
        aggregating over time (any function ending in <code>_over_time</code>), always take an <code>irate()</code>{" "}
        first, then aggregate. Otherwise <code>irate()</code> cannot detect counter resets when your target restarts.
      </p>
    </>
  ),
  label_join: (
    <>
      <p>
        For each timeseries in <code>v</code>,{" "}
        <code>
          label_join(v instant-vector, dst_label string, separator string, src_label_1 string, src_label_2 string, ...)
        </code>{" "}
        joins all the values of all the <code>src_labels</code>
        using <code>separator</code> and returns the timeseries with the label <code>dst_label</code> containing the
        joined value. There can be any number of <code>src_labels</code> in this function.
      </p>

      <p>
        <code>label_join</code> acts on float and histogram samples in the same way.
      </p>

      <p>
        This example will return a vector with each time series having a <code>foo</code> label with the value{" "}
        <code>a,b,c</code> added to it:
      </p>

      <pre>
        <code>
          label_join(up{"{"}job=&quot;api-server&quot;,src1=&quot;a&quot;,src2=&quot;b&quot;,src3=&quot;c&quot;{"}"},
          &quot;foo&quot;, &quot;,&quot;, &quot;src1&quot;, &quot;src2&quot;, &quot;src3&quot;)
        </code>
      </pre>
    </>
  ),
  label_replace: (
    <>
      <p>
        For each timeseries in <code>v</code>,{" "}
        <code>
          label_replace(v instant-vector, dst_label string, replacement string, src_label string, regex string)
        </code>
        matches the <a href="./basics.md#regular-expressions">regular expression</a> <code>regex</code> against the
        value of the label <code>src_label</code>. If it matches, the value of the label <code>dst_label</code> in the
        returned timeseries will be the expansion of <code>replacement</code>, together with the original labels in the
        input. Capturing groups in the regular expression can be referenced with <code>$1</code>, <code>$2</code>, etc.
        Named capturing groups in the regular expression can be referenced with <code>$name</code> (where{" "}
        <code>name</code> is the capturing group name). If the regular expression doesn&rsquo;t match then the
        timeseries is returned unchanged.
      </p>

      <p>
        <code>label_replace</code> acts on float and histogram samples in the same way.
      </p>

      <p>
        This example will return timeseries with the values <code>a:c</code> at label <code>service</code> and{" "}
        <code>a</code> at label <code>foo</code>:
      </p>

      <pre>
        <code>
          label_replace(up{"{"}job=&quot;api-server&quot;,service=&quot;a:c&quot;{"}"}, &quot;foo&quot;, &quot;$1&quot;,
          &quot;service&quot;, &quot;(.*):.*&quot;)
        </code>
      </pre>

      <p>
        This second example has the same effect than the first example, and illustrates use of named capturing groups:
      </p>

      <pre>
        <code>
          label_replace(up{"{"}job=&quot;api-server&quot;,service=&quot;a:c&quot;{"}"}, &quot;foo&quot;,
          &quot;$name&quot;, &quot;service&quot;, &quot;(?P&lt;name&gt;.*):(?P&lt;version&gt;.*)&quot;)
        </code>
      </pre>
    </>
  ),
  last_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  ln: (
    <>
      <p>
        <code>ln(v instant-vector)</code> calculates the natural logarithm for all float samples in <code>v</code>.
        Histogram samples in the input vector are ignored silently. Special cases are:
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
        <code>log10(v instant-vector)</code> calculates the decimal logarithm for all float samples in <code>v</code>.
        Histogram samples in the input vector are ignored silently. The special cases are equivalent to those in{" "}
        <code>ln</code>.
      </p>
    </>
  ),
  log2: (
    <>
      <p>
        <code>log2(v instant-vector)</code> calculates the binary logarithm for all float samples in <code>v</code>.
        Histogram samples in the input vector are ignored silently. The special cases are equivalent to those in{" "}
        <code>ln</code>.
      </p>
    </>
  ),
  mad_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  max_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  min_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  minute: (
    <>
      <p>
        <code>minute(v=vector(time()) instant-vector)</code> interprets float samples in <code>v</code> as timestamps
        (number of seconds since January 1, 1970 UTC) and returns the minute of the hour (in UTC) for each of those
        timestamps. Returned values are from 0 to 59. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  month: (
    <>
      <p>
        <code>month(v=vector(time()) instant-vector)</code> interprets float samples in <code>v</code> as timestamps
        (number of seconds since January 1, 1970 UTC) and returns the month of the year (in UTC) for each of those
        timestamps. Returned values are from 1 to 12, where 1 means January etc. Histogram samples in the input vector
        are ignored silently.
      </p>
    </>
  ),
  pi: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  predict_linear: (
    <>
      <p>
        <code>predict_linear(v range-vector, t scalar)</code> predicts the value of time series
        <code>t</code> seconds from now, based on the range vector <code>v</code>, using{" "}
        <a href="https://en.wikipedia.org/wiki/Simple_linear_regression">simple linear regression</a>. The range vector
        must have at least two float samples in order to perform the calculation. When <code>+Inf</code> or{" "}
        <code>-Inf</code> are found in the range vector, the predicted value will be <code>NaN</code>.
      </p>

      <p>
        <code>predict_linear</code> should only be used with gauges and only works for float samples. Elements in the
        range vector that contain only histogram samples are ignored entirely. For elements that contain a mix of float
        and histogram samples, only the float samples are used as input, which is flagged by an info-level annotation.
      </p>
    </>
  ),
  present_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  quantile_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  rad: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  rate: (
    <>
      <p>
        <code>rate(v range-vector)</code> calculates the per-second average rate of increase of the time series in the
        range vector. Breaks in monotonicity (such as counter resets due to target restarts) are automatically adjusted
        for. Also, the calculation extrapolates to the ends of the time range, allowing for missed scrapes or imperfect
        alignment of scrape cycles with the range&rsquo;s time period.
      </p>

      <p>
        The following example expression returns the per-second average rate of HTTP requests over the last 5 minutes,
        per time series in the range vector:
      </p>

      <pre>
        <code>
          rate(http_requests_total{"{"}job=&quot;api-server&quot;{"}"}[5m])
        </code>
      </pre>

      <p>
        <code>rate</code> acts on native histograms by calculating a new histogram where each component (sum and count
        of observations, buckets) is the rate of increase between the respective component in the first and last native
        histogram in <code>v</code>. However, each element in <code>v</code> that contains a mix of float and native
        histogram samples within the range, will be omitted from the result vector, flagged by a warn-level annotation.
      </p>

      <p>
        <code>rate</code> should only be used with counters (for both floats and histograms). It is best suited for
        alerting, and for graphing of slow-moving counters.
      </p>

      <p>
        Note that when combining <code>rate()</code> with an aggregation operator (e.g. <code>sum()</code>) or a
        function aggregating over time (any function ending in <code>_over_time</code>), always take a{" "}
        <code>rate()</code> first, then aggregate. Otherwise <code>rate()</code> cannot detect counter resets when your
        target restarts.
      </p>
    </>
  ),
  resets: (
    <>
      <p>
        For each input time series, <code>resets(v range-vector)</code> returns the number of counter resets within the
        provided time range as an instant vector. Any decrease in the value between two consecutive float samples is
        interpreted as a counter reset. A reset in a native histogram is detected in a more complex way: Any decrease in
        any bucket, including the zero bucket, or in the count of observation constitutes a counter reset, but also the
        disappearance of any previously populated bucket, a decrease of the zero-bucket width, or any schema change that
        is not a compatible decrease of resolution.
      </p>

      <p>
        <code>resets</code> should only be used with counters (for both floats and histograms).
      </p>

      <p>
        A float sample followed by a histogram sample, or vice versa, counts as a reset. A counter histogram sample
        followed by a gauge histogram sample, or vice versa, also counts as a reset (but note that <code>resets</code>{" "}
        should not be used on gauges in the first place, see above).
      </p>
    </>
  ),
  round: (
    <>
      <p>
        <code>round(v instant-vector, to_nearest=1 scalar)</code> rounds the sample values of all elements in{" "}
        <code>v</code> to the nearest integer. Ties are resolved by rounding up. The optional <code>to_nearest</code>{" "}
        argument allows specifying the nearest multiple to which the sample values should be rounded. This multiple may
        also be a fraction. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  scalar: (
    <>
      <p>
        Given an input vector that contains only one element with a float sample,
        <code>scalar(v instant-vector)</code> returns the sample value of that float sample as a scalar. If the input
        vector does not have exactly one element with a float sample, <code>scalar</code> will return <code>NaN</code>.
        Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  sgn: (
    <>
      <p>
        <code>sgn(v instant-vector)</code> returns a vector with all float sample values converted to their sign,
        defined as this: 1 if v is positive, -1 if v is negative and 0 if v is equal to zero. Histogram samples in the
        input vector are ignored silently.
      </p>
    </>
  ),
  sin: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  sinh: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  sort: (
    <>
      <p>
        <code>sort(v instant-vector)</code> returns vector elements sorted by their float sample values, in ascending
        order. Histogram samples in the input vector are ignored silently.
      </p>

      <p>
        Please note that <code>sort</code> only affects the results of instant queries, as range query results always
        have a fixed output ordering.
      </p>
    </>
  ),
  sort_by_label: (
    <>
      <p>
        <strong>
          This function has to be enabled via the{" "}
          <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
          <code>--enable-feature=promql-experimental-functions</code>.
        </strong>
      </p>

      <p>
        <code>sort_by_label(v instant-vector, label string, ...)</code> returns vector elements sorted by the values of
        the given labels in ascending order. In case these label values are equal, elements are sorted by their full
        label sets.
        <code>sort_by_label</code> acts on float and histogram samples in the same way.
      </p>

      <p>
        Please note that <code>sort_by_label</code> only affects the results of instant queries, as range query results
        always have a fixed output ordering.
      </p>

      <p>
        <code>sort_by_label</code> uses{" "}
        <a href="https://en.wikipedia.org/wiki/Natural_sort_order">natural sort order</a>.
      </p>
    </>
  ),
  sort_by_label_desc: (
    <>
      <p>
        <strong>
          This function has to be enabled via the{" "}
          <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
          <code>--enable-feature=promql-experimental-functions</code>.
        </strong>
      </p>

      <p>
        Same as <code>sort_by_label</code>, but sorts in descending order.
      </p>
    </>
  ),
  sort_desc: (
    <>
      <p>
        Same as <code>sort</code>, but sorts in descending order.
      </p>
    </>
  ),
  sqrt: (
    <>
      <p>
        <code>sqrt(v instant-vector)</code> calculates the square root of all float samples in
        <code>v</code>. Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
  stddev_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  stdvar_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  sum_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  tan: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  tanh: (
    <>
      <p>The trigonometric functions work in radians. They ignore histogram samples in the input vector.</p>

      <ul>
        <li>
          <code>acos(v instant-vector)</code>: calculates the arccosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Acos">special cases</a>).
        </li>
        <li>
          <code>acosh(v instant-vector)</code>: calculates the inverse hyperbolic cosine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Acosh">special cases</a>).
        </li>
        <li>
          <code>asin(v instant-vector)</code>: calculates the arcsine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Asin">special cases</a>).
        </li>
        <li>
          <code>asinh(v instant-vector)</code>: calculates the inverse hyperbolic sine of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Asinh">special cases</a>).
        </li>
        <li>
          <code>atan(v instant-vector)</code>: calculates the arctangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Atan">special cases</a>).
        </li>
        <li>
          <code>atanh(v instant-vector)</code>: calculates the inverse hyperbolic tangent of all float samples in{" "}
          <code>v</code> (<a href="https://pkg.go.dev/math#Atanh">special cases</a>).
        </li>
        <li>
          <code>cos(v instant-vector)</code>: calculates the cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cos">special cases</a>).
        </li>
        <li>
          <code>cosh(v instant-vector)</code>: calculates the hyperbolic cosine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Cosh">special cases</a>).
        </li>
        <li>
          <code>sin(v instant-vector)</code>: calculates the sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sin">special cases</a>).
        </li>
        <li>
          <code>sinh(v instant-vector)</code>: calculates the hyperbolic sine of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Sinh">special cases</a>).
        </li>
        <li>
          <code>tan(v instant-vector)</code>: calculates the tangent of all float samples in <code>v</code> (
          <a href="https://pkg.go.dev/math#Tan">special cases</a>).
        </li>
        <li>
          <code>tanh(v instant-vector)</code>: calculates the hyperbolic tangent of all float samples in <code>v</code>{" "}
          (<a href="https://pkg.go.dev/math#Tanh">special cases</a>).
        </li>
      </ul>

      <p>The following are useful for converting between degrees and radians:</p>

      <ul>
        <li>
          <code>deg(v instant-vector)</code>: converts radians to degrees for all float samples in <code>v</code>.
        </li>
        <li>
          <code>pi()</code>: returns pi.
        </li>
        <li>
          <code>rad(v instant-vector)</code>: converts degrees to radians for all float samples in <code>v</code>.
        </li>
      </ul>
    </>
  ),
  time: (
    <>
      <p>
        <code>time()</code> returns the number of seconds since January 1, 1970 UTC. Note that this does not actually
        return the current time, but the time at which the expression is to be evaluated.
      </p>
    </>
  ),
  timestamp: (
    <>
      <p>
        <code>timestamp(v instant-vector)</code> returns the timestamp of each of the samples of the given vector as the
        number of seconds since January 1, 1970 UTC. It acts on float and histogram samples in the same way.
      </p>
    </>
  ),
  ts_of_first_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  ts_of_last_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  ts_of_max_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  ts_of_min_over_time: (
    <>
      <p>
        The following functions allow aggregating each series of a given range vector over time and return an instant
        vector with per-series aggregation results:
      </p>

      <ul>
        <li>
          <code>avg_over_time(range-vector)</code>: the average value of all float or histogram samples in the specified
          interval (see details below).
        </li>
        <li>
          <code>min_over_time(range-vector)</code>: the minimum value of all float samples in the specified interval.
        </li>
        <li>
          <code>max_over_time(range-vector)</code>: the maximum value of all float samples in the specified interval.
        </li>
        <li>
          <code>sum_over_time(range-vector)</code>: the sum of all float or histogram samples in the specified interval
          (see details below).
        </li>
        <li>
          <code>count_over_time(range-vector)</code>: the count of all samples in the specified interval.
        </li>
        <li>
          <code>quantile_over_time(scalar, range-vector)</code>: the φ-quantile (0 ≤ φ ≤ 1) of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stddev_over_time(range-vector)</code>: the population standard deviation of all float samples in the
          specified interval.
        </li>
        <li>
          <code>stdvar_over_time(range-vector)</code>: the population standard variance of all float samples in the
          specified interval.
        </li>
        <li>
          <code>last_over_time(range-vector)</code>: the most recent sample in the specified interval.
        </li>
        <li>
          <code>present_over_time(range-vector)</code>: the value 1 for any series in the specified interval.
        </li>
      </ul>

      <p>
        If the <a href="../feature_flags.md#experimental-promql-functions">feature flag</a>
        <code>--enable-feature=promql-experimental-functions</code> is set, the following additional functions are
        available:
      </p>

      <ul>
        <li>
          <code>mad_over_time(range-vector)</code>: the median absolute deviation of all float samples in the specified
          interval.
        </li>
        <li>
          <code>ts_of_min_over_time(range-vector)</code>: the timestamp of the last float sample that has the minimum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_max_over_time(range-vector)</code>: the timestamp of the last float sample that has the maximum
          value of all float samples in the specified interval.
        </li>
        <li>
          <code>ts_of_last_over_time(range-vector)</code>: the timestamp of last sample in the specified interval.
        </li>
        <li>
          <code>first_over_time(range-vector)</code>: the oldest sample in the specified interval.
        </li>
        <li>
          <code>ts_of_first_over_time(range-vector)</code>: the timestamp of earliest sample in the specified interval.
        </li>
      </ul>

      <p>
        Note that all values in the specified interval have the same weight in the aggregation even if the values are
        not equally spaced throughout the interval.
      </p>

      <p>These functions act on histograms in the following way:</p>

      <ul>
        <li>
          <code>count_over_time</code>, <code>first_over_time</code>, <code>last_over_time</code>, and
          <code>present_over_time()</code> act on float and histogram samples in the same way.
        </li>
        <li>
          <code>avg_over_time()</code> and <code>sum_over_time()</code> act on histogram samples in a way that
          corresponds to the respective aggregation operators. If a series contains a mix of float samples and histogram
          samples within the range, the corresponding result is removed entirely from the output vector. Such a removal
          is flagged by a warn-level annotation.
        </li>
        <li>
          All other functions ignore histogram samples in the following way: Input ranges containing only histogram
          samples are silently removed from the output. For ranges with a mix of histogram and float samples, only the
          float samples are processed and the omission of the histogram samples is flagged by an info-level annotation.
        </li>
      </ul>

      <p>
        <code>first_over_time(m[1m])</code> differs from <code>m offset 1m</code> in that the former will select the
        first sample of <code>m</code> <em>within</em> the 1m range, where <code>m offset 1m</code> will select the most
        recent sample within the lookback interval <em>outside and prior to</em> the 1m offset. This is particularly
        useful with <code>first_over_time(m[step()])</code>
        in range queries (available when <code>--enable-feature=promql-duration-expr</code> is set) to ensure that the
        sample selected is within the range step.
      </p>
    </>
  ),
  vector: (
    <>
      <p>
        <code>vector(s scalar)</code> converts the scalar <code>s</code> to a float sample and returns it as a
        single-element instant vector with no labels.
      </p>
    </>
  ),
  year: (
    <>
      <p>
        <code>year(v=vector(time()) instant-vector)</code> returns the year for each of the given times in UTC.
        Histogram samples in the input vector are ignored silently.
      </p>
    </>
  ),
};

export default funcDocs;
