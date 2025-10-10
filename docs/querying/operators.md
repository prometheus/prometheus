---
title: Operators
sort_rank: 2
---

PromQL supports unary, binary, and aggregation operators.

## Unary operator

The only unary operator in PromQL is `-` (unary minus). It can be applied to a
scalar or an instant vector. In the former case, it returns a scalar with
inverted sign. In the latter case, it returns an instant vector with inverted
sign for each element. The sign of a histogram sample is inverted by inverting
the sign of all bucket populations and the count and the sum of observations.
The resulting histogram sample is always considered a gauge histogram.

NOTE: A histogram with any negative bucket population or a negative count of
observations should only be used as an intermediate result. If such a negative
histogram is the final outcome of a recording rule, the rule evaluation will
fail. Negative histograms cannot be represented by any of the exchange formats
(exposition, remote-write, OTLP), so they cannot ingested into Prometheus in
any way and are only created by PromQL expressions.

## Binary operators

Binary operators cover basic logical and arithmetic operations. For operations
between two instant vectors, the [matching behavior](#vector-matching) can be
modified.

### Arithmetic binary operators

The following binary arithmetic operators exist in PromQL:

* `+` (addition)
* `-` (subtraction)
* `*` (multiplication)
* `/` (division)
* `%` (modulo)
* `^` (power/exponentiation)

Binary arithmetic operators are defined between scalar/scalar, vector/scalar,
and vector/vector value pairs. They follow the usual [IEEE 754 floating point
arithmetic](https://en.wikipedia.org/wiki/IEEE_754), including the handling of
special values like `NaN`, `+Inf`, and `-Inf`.

**Between two scalars**, the behavior is straightforward: they evaluate to another
scalar that is the result of the operator applied to both scalar operands.

**Between an instant vector and a scalar**, the operator is applied to the
value of every data sample in the vector. 

If the data sample is a float, the operation is performed between that float and the scalar. 
For example, if an instant vector of float samples is multiplied by 2,
the result is another vector of float samples in which every sample value of
the original vector is multiplied by 2.

For vector elements that are histogram samples, the behavior is the
following:

* For `*`, all bucket populations and the count and the sum of observations are
  multiplied by the scalar. If the scalar is negative, the resulting histogram
  is considered a gauge histogram. Otherwise, the counter vs. gauge flavor of
  the input histogram sample is retained.

* For `/`, the histogram sample has to be on the left hand side (LHS), followed
  by the scalar on the right hand side (RHS). All bucket populations and the
  count and the sum of observations are then divided by the scalar. A division
  by zero results in a histogram with no regular buckets and the zero bucket
  population and the count and sum of observations all set to `+Inf`, `-Inf`,
  or `NaN`, depending on their values in the input histogram (positive,
  negative, or zero/`NaN`, respectively). If the scalar is negative, the
  resulting histogram is considered a gauge histogram. Otherwise, the counter
  vs. gauge flavor of the input histogram sample is retained.

* For `/` with a scalar on the LHS and a histogram sample on the RHS, and
  similarly for all other arithmetic binary operators in any combination of a
  scalar and a histogram sample, there is no result and the corresponding
  element is removed from the resulting vector. Such a removal is flagged by an
  info-level annotation.

**Between two instant vectors**, a binary arithmetic operator is applied to
each entry in the LHS vector and its [matching element](#vector-matching) in
the RHS vector. The result is propagated into the result vector with the
grouping labels becoming the output label set. Entries for which no matching
entry in the right-hand vector can be found are not part of the result.

If two float samples are matched, the arithmetic operator is applied to the two
input values.

If a float sample is matched with a histogram sample, the behavior follows the
same logic as between a scalar and a histogram sample (see above), i.e. `*` and
`/` (the latter with the histogram sample on the LHS) are valid operations,
while all others lead to the removal of the corresponding element from the
resulting vector.

If two histogram samples are matched, only `+` and `-` are valid operations,
each adding or subtracting all matching bucket populations and the count and
the sum of observations. All other operations result in the removal of the
corresponding element from the output vector, flagged by an info-level
annotation. The `+` and -` operations should generally only be applied to gauge
histograms, but PromQL allows them for counter histograms, too, to cover
specific use cases, for which special attention is required to avoid problems
with unaligned counter resets. (Certain incompatibilities of counter resets can
be detected by PromQL and are flagged with a warn-level annotations.) Adding
two counter histograms results in a counter histogram. All other combination of
operands and all subtractions result in a gauge histogram.

**In any arithmetic binary operation involving vectors**, the metric name is
dropped. This occurs even if `__name__` is explicitly mentioned in `on` 
(see https://github.com/prometheus/prometheus/issues/16631 for further discussion).

**For any arithmetic binary operation that may result in a negative
histogram**, take into account the [respective note above](#unary-operator).

### Trigonometric binary operators

The following trigonometric binary operators, which work in radians, exist in Prometheus:

* `atan2` (based on https://pkg.go.dev/math#Atan2)

Trigonometric operators allow trigonometric functions to be executed on two
vectors using vector matching, which isn't available with normal functions.
They act in the same manner as arithmetic operators. They only operate on float
samples. Operations involving histogram samples result in the removal of the
corresponding vector elements from the output vector, flagged by an
info-level annotation.

### Comparison binary operators

The following binary comparison operators exist in Prometheus:

* `==` (equal)
* `!=` (not-equal)
* `>` (greater-than)
* `<` (less-than)
* `>=` (greater-or-equal)
* `<=` (less-or-equal)

Comparison operators are defined between scalar/scalar, vector/scalar,
and vector/vector value pairs. By default they filter. Their behavior can be
modified by providing `bool` after the operator, which will return `0` or `1`
for the value rather than filtering.

**Between two scalars**, the `bool` modifier must be provided and these
operators result in another scalar that is either `0` (`false`) or `1`
(`true`), depending on the comparison result.

**Between an instant vector and a scalar**, these operators are applied to the
value of every data sample in the vector, and vector elements between which the
comparison result is false get dropped from the result vector. These
operations only work with float samples in the vector. For histogram samples,
the corresponding element is removed from the result vector, flagged by an
info-level annotation.

**Between two instant vectors**, these operators behave as a filter by default,
applied to matching entries. Vector elements for which the expression is not
true or which do not find a match on the other side of the expression get
dropped from the result, while the others are propagated into a result vector
with the grouping labels becoming the output label set. 

Matches between two float samples work as usual. 

Matches between a float sample and a histogram sample are invalid, and the
corresponding element is removed from the result vector, flagged by an info-level
annotation.

Between two histogram samples, `==` and `!=` work as expected, but all other
comparison binary operations are again invalid.

**In any comparison binary operation involving vectors**, providing the `bool`
modifier changes the behavior in the following ways:

* Vector elements which find a match on the other side of the expression but for
  which the expression is false instead have the value `0` and vector elements
  that do find a match and for which the expression is true have the value `1`. 
  (Note that elements with no match or invalid operations involving histogram
  samples still return no result rather than the value `0`.)
* The metric name is dropped.

If the `bool` modifier is not provided, then the metric name from the left side
is retained, with some exceptions:

* If `on` is used, then the metric name is dropped.
* If `group_right` is used, then the metric name from the right side is retained,
  to avoid collisions.

### Logical/set binary operators

These logical/set binary operators are only defined between instant vectors:

* `and` (intersection)
* `or` (union)
* `unless` (complement)

`vector1 and vector2` results in a vector consisting of the elements of
`vector1` for which there are elements in `vector2` with exactly matching
label sets. Other elements are dropped. The metric name and values are carried
over from the left-hand side vector.

`vector1 or vector2` results in a vector that contains all original elements
(label sets + values) of `vector1` and additionally all elements of `vector2`
which do not have matching label sets in `vector1`.

`vector1 unless vector2` results in a vector consisting of the elements of
`vector1` for which there are no elements in `vector2` with exactly matching
label sets. All matching elements in both vectors are dropped.

As these logical/set binary operators do not interact with the sample values,
they work in the same way for float samples and histogram samples.

## Vector matching

Operations between vectors attempt to find a matching element in the right-hand side
vector for each entry in the left-hand side. There are two basic types of
matching behavior: One-to-one and many-to-one/one-to-many.

### Vector matching keywords

These vector matching keywords allow for matching between series with different label sets
providing:

* `on`
* `ignoring`

Label lists provided to matching keywords will determine how vectors are combined. Examples
can be found in [One-to-one vector matches](#one-to-one-vector-matches) and in
[Many-to-one and one-to-many vector matches](#many-to-one-and-one-to-many-vector-matches)

### Group modifiers

These group modifiers enable many-to-one/one-to-many vector matching:

* `group_left`
* `group_right`

Label lists can be provided to the group modifier which contain labels from the "one"-side to
be included in the result metrics.

_Many-to-one and one-to-many matching are advanced use cases that should be carefully considered.
Often a proper use of `ignoring(<labels>)` provides the desired outcome._

_Grouping modifiers can only be used for
[comparison](#comparison-binary-operators) and
[arithmetic](#arithmetic-binary-operators). Operations as `and`, `unless` and
`or` operations match with all possible entries in the right vector by
default._

### One-to-one vector matches

**One-to-one** finds a unique pair of entries from each side of the operation.
In the default case, that is an operation following the format `vector1 <operator> vector2`.
Two entries match if they have the exact same set of labels and corresponding values.
The `ignoring` keyword allows ignoring certain labels when matching, while the
`on` keyword allows reducing the set of considered labels to a provided list:

    <vector expr> <bin-op> ignoring(<label list>) <vector expr>
    <vector expr> <bin-op> on(<label list>) <vector expr>

Example input:

    method_code:http_errors:rate5m{method="get", code="500"}  24
    method_code:http_errors:rate5m{method="get", code="404"}  30
    method_code:http_errors:rate5m{method="put", code="501"}  3
    method_code:http_errors:rate5m{method="post", code="500"} 6
    method_code:http_errors:rate5m{method="post", code="404"} 21

    method:http_requests:rate5m{method="get"}  600
    method:http_requests:rate5m{method="del"}  34
    method:http_requests:rate5m{method="post"} 120

Example query:

    method_code:http_errors:rate5m{code="500"} / ignoring(code) method:http_requests:rate5m

This returns a result vector containing the fraction of HTTP requests with status code
of 500 for each method, as measured over the last 5 minutes. Without `ignoring(code)` there
would have been no match as the metrics do not share the same set of labels.
The entries with methods `put` and `del` have no match and will not show up in the result:

    {method="get"}  0.04            //  24 / 600
    {method="post"} 0.05            //   6 / 120

### Many-to-one and one-to-many vector matches

**Many-to-one** and **one-to-many** matchings refer to the case where each vector element on
the "one"-side can match with multiple elements on the "many"-side. This has to
be explicitly requested using the `group_left` or `group_right` [modifiers](#group-modifiers), where
left/right determines which vector has the higher cardinality.

    <vector expr> <bin-op> ignoring(<label list>) group_left(<label list>) <vector expr>
    <vector expr> <bin-op> ignoring(<label list>) group_right(<label list>) <vector expr>
    <vector expr> <bin-op> on(<label list>) group_left(<label list>) <vector expr>
    <vector expr> <bin-op> on(<label list>) group_right(<label list>) <vector expr>

The label list provided with the [group modifier](#group-modifiers) contains additional labels from
the "one"-side to be included in the result metrics. For `on` a label can only
appear in one of the lists. Every time series of the result vector must be
uniquely identifiable.

Example query:

    method_code:http_errors:rate5m / ignoring(code) group_left method:http_requests:rate5m

In this case the left vector contains more than one entry per `method` label
value. Thus, we indicate this using `group_left`. The elements from the right
side are now matched with multiple elements with the same `method` label on the
left:

    {method="get", code="500"}  0.04            //  24 / 600
    {method="get", code="404"}  0.05            //  30 / 600
    {method="post", code="500"} 0.05            //   6 / 120
    {method="post", code="404"} 0.175           //  21 / 120


## Aggregation operators

Prometheus supports the following built-in aggregation operators that can be
used to aggregate the elements of a single instant vector, resulting in a new
vector of fewer elements with aggregated values:

* `sum(v)` (calculate sum over dimensions)
* `avg(v)` (calculate the arithmetic average over dimensions)
* `min(v)` (select minimum over dimensions)
* `max(v)` (select maximum over dimensions)
* `bottomk(k, v)` (smallest `k` elements by sample value)
* `topk(k, v)` (largest `k` elements by sample value)
* `limitk(k, v)` (sample `k` elements, **experimental**, must be enabled with `--enable-feature=promql-experimental-functions`)
* `limit_ratio(r, v)` (sample a pseudo-random ratio `r` of elements, **experimental**, must be enabled with `--enable-feature=promql-experimental-functions`)
* `group(v)` (all values in the resulting vector are 1)
* `count(v)` (count number of elements in the vector)
* `count_values(l, v)` (count number of elements with the same value)

* `stddev(v)` (calculate population standard deviation over dimensions)
* `stdvar(v)` (calculate population standard variance over dimensions)
* `quantile(φ, v)` (calculate φ-quantile (0 ≤ φ ≤ 1) over dimensions)

These operators can either be used to aggregate over **all** label dimensions
or preserve distinct dimensions by including a `without` or `by` clause. These
clauses may be used before or after the expression.

    <aggr-op> [without|by (<label list>)] ([parameter,] <vector expression>)

or

    <aggr-op>([parameter,] <vector expression>) [without|by (<label list>)]

`label list` is a list of unquoted labels that may include a trailing comma, i.e.
both `(label1, label2)` and `(label1, label2,)` are valid syntax.

`without` removes the listed labels from the result vector, while
all other labels are preserved in the output. `by` does the opposite and drops
labels that are not listed in the `by` clause, even if their label values are
identical between all elements of the vector.

### Detailed explanations

#### `sum`

`sum(v)` sums up sample values in `v` in the same way as the `+` binary operator does
between two values. 

All sample values being aggregated into a single resulting vector element must either be
float samples or histogram samples. An aggregation of a mix of both is invalid,
resulting in the removal of the corresponding vector element from the output
vector, flagged by a warn-level annotation.

##### Examples

If the metric `memory_consumption_bytes` had time series that fan out by
`application`, `instance`, and `group` labels, we could calculate the total
memory consumption per application and group over all instances via:

    sum without (instance) (memory_consumption_bytes)

Which is equivalent to:

    sum by (application, group) (memory_consumption_bytes)

If we are just interested in the total memory consumption in **all**
applications, we could simply write:

    sum(memory_consumption_bytes)

#### `avg`

`avg(v)` divides the sum of `v` by the number of aggregated samples in the same way
as the `/` binary operator.

All sample values being aggregated into a single resulting vector element must either be
float samples or histogram samples. An aggregation of a mix of both is invalid,
resulting in the removal of the corresponding vector element from the output
vector, flagged by a warn-level annotation.

#### `min` and `max`

`min(v)` and `max(v)` return the minimum or maximum value, respectively, in `v`. 

They only operate on float samples, following IEEE 754 floating
point arithmetic, which in particular implies that `NaN` is only ever
considered a minimum or maximum if all aggregated values are `NaN`. Histogram
samples in the input vector are ignored, flagged by an info-level annotation.

#### `topk` and `bottomk`

`topk(k, v)` and `bottomk(k, v)` are different from other aggregators in that a subset of
`k` values from the input samples, including the original labels, are returned in the result vector. 

`by` and `without` are only used to bucket the input vector. 

Similar to `min` and `max`, they only operate on float samples, considering `NaN` values
to be farthest from the top or bottom, respectively. Histogram samples in the
input vector are ignored, flagged by an info-level annotation.

If used in an instant query, `topk` and `bottomk` return series ordered by
value in descending or ascending order, respectively. If used with `by` or
`without`, then series within each bucket are sorted by value, and series in
the same bucket are returned consecutively, but there is no guarantee that
buckets of series will be returned in any particular order. 

No sorting applies to range queries.

##### Example

To get the 5 instances with the highest memory consumption across all instances we could write:

    topk(5, memory_consumption_bytes)

#### `limitk` and `limit_ratio`

`limitk(k, v)` returns a subset of `k` input samples, including
the original labels in the result vector. 

The subset is selected in a deterministic pseudo-random way.
This happens independent of the sample type. 
Therefore, it works for both float samples and histogram samples. 

##### Example

To sample 10 timeseries we could write:

    limitk(10, memory_consumption_bytes)

#### `limit_ratio`

`limit_ratio(r, v)` returns a subset of the input samples, including
the original labels in the result vector.

The subset is selected in a deterministic pseudo-random way.
This happens independent of the sample type.
Therefore, it works for both float samples and histogram samples.

`r` can be between +1 and -1. The absolute value of `r` is used as the selection ratio,
but the selection order is inverted for a negative `r`, which can be used to select complements.
For example, `limit_ratio(0.1, ...)` returns a deterministic set of approximatiely 10% of
the input samples, while `limit_ratio(-0.9, ...)` returns precisely the
remaining approximately 90% of the input samples not returned by `limit_ratio(0.1, ...)`.

#### `group`

`group(v)` returns 1 for each group that contains any value at that timestamp.

The value may be a float or histogram sample.

#### `count`

`count(v)` returns the number of values at that timestamp, or no value at all
if no values are present at that timestamp.

The value may be a float or histogram sample.

#### `count_values`

`count_values(l, v)` outputs one time series per unique sample value in `v`. 
Each series has an additional label, given by `l`, and the label value is the 
unique sample value. The value of each time series is the number of times that sample value was present.

`count_values` works with both float samples and histogram samples. For the
latter, a compact string representation of the histogram sample value is used
as the label value.

##### Example

To count the number of binaries running each build version we could write:

    count_values("version", build_version)

#### `stddev`

`stddev(v)` returns the standard deviation of `v`. 

`stddev` only works with float samples, following IEEE 754 floating
point arithmetic. Histogram samples in the input vector are ignored, flagged by
an info-level annotation.

#### `stdvar`

`stdvar(v)` returns the standard deviation of `v`. 

`stdvar` only works with float samples, following IEEE 754 floating
point arithmetic. Histogram samples in the input vector are ignored, flagged by
an info-level annotation.

#### `quantile`

`quantile(φ, v)` calculates the φ-quantile, the value that ranks at number φ*N among
the N metric values of the dimensions aggregated over.

`quantile` only works with float samples. Histogram samples in the input vector
are ignored, flagged by an info-level annotation.

`NaN` is considered the smallest possible value.

For example, `quantile(0.5, ...)` calculates the median, `quantile(0.95, ...)` the 95th percentile. 

Special cases:

* For φ = `NaN`, `NaN` is returned.
* For φ < 0, `-Inf` is returned. 
* For φ > 1, `+Inf` is returned.

## Binary operator precedence

The following list shows the precedence of binary operators in Prometheus, from
highest to lowest.

1. `^`
2. `*`, `/`, `%`, `atan2`
3. `+`, `-`
4. `==`, `!=`, `<=`, `<`, `>=`, `>`
5. `and`, `unless`
6. `or`

Operators on the same precedence level are left-associative. For example,
`2 * 3 % 2` is equivalent to `(2 * 3) % 2`. However `^` is right associative,
so `2 ^ 3 ^ 2` is equivalent to `2 ^ (3 ^ 2)`.
