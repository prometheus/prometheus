---
title: Operators
sort_rank: 2
---

# Operators

## Binary operators

Prometheus's query language supports basic logical and arithmetic operators.
For operations between two instant vectors, the [matching behavior](#vector-matching)
can be modified.

### Arithmetic binary operators

The following binary arithmetic operators exist in Prometheus:

* `+` (addition)
* `-` (subtraction)
* `*` (multiplication)
* `/` (division)
* `%` (modulo)
* `^` (power/exponentiation)

Binary arithmetic operators are defined between scalar/scalar, vector/scalar,
and vector/vector value pairs.

**Between two scalars**, the behavior is obvious: they evaluate to another
scalar that is the result of the operator applied to both scalar operands.

**Between an instant vector and a scalar**, the operator is applied to the
value of every data sample in the vector. E.g. if a time series instant vector
is multiplied by 2, the result is another vector in which every sample value of
the original vector is multiplied by 2. The metric name is dropped.

**Between two instant vectors**, a binary arithmetic operator is applied to
each entry in the left-hand side vector and its [matching element](#vector-matching)
in the right-hand vector. The result is propagated into the result vector with the
grouping labels becoming the output label set. The metric name is dropped. Entries
for which no matching entry in the right-hand vector can be found are not part of
the result.

### Trigonometric binary operators

The following trigonometric binary operators, which work in radians, exist in Prometheus:

* `atan2` (based on https://pkg.go.dev/math#Atan2)

Trigonometric operators allow trigonometric functions to be executed on two vectors using
vector matching, which isn't available with normal functions. They act in the same manner
as arithmetic operators.

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
comparison result is `false` get dropped from the result vector. If the `bool`
modifier is provided, vector elements that would be dropped instead have the value
`0` and vector elements that would be kept have the value `1`. The metric name
is dropped if the `bool` modifier is provided.

**Between two instant vectors**, these operators behave as a filter by default,
applied to matching entries. Vector elements for which the expression is not
true or which do not find a match on the other side of the expression get
dropped from the result, while the others are propagated into a result vector
with the grouping labels becoming the output label set.
If the `bool` modifier is provided, vector elements that would have been
dropped instead have the value `0` and vector elements that would be kept have
the value `1`, with the grouping labels again becoming the output label set.
The metric name is dropped if the `bool` modifier is provided.

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

* `sum` (calculate sum over dimensions)
* `min` (select minimum over dimensions)
* `max` (select maximum over dimensions)
* `avg` (calculate the average over dimensions)
* `group` (all values in the resulting vector are 1)
* `stddev` (calculate population standard deviation over dimensions)
* `stdvar` (calculate population standard variance over dimensions)
* `count` (count number of elements in the vector)
* `count_values` (count number of elements with the same value)
* `bottomk` (smallest k elements by sample value)
* `topk` (largest k elements by sample value)
* `quantile` (calculate φ-quantile (0 ≤ φ ≤ 1) over dimensions)

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

`parameter` is only required for `count_values`, `quantile`, `topk` and
`bottomk`.

`count_values` outputs one time series per unique sample value. Each series has
an additional label. The name of that label is given by the aggregation
parameter, and the label value is the unique sample value. The value of each
time series is the number of times that sample value was present.

`topk` and `bottomk` are different from other aggregators in that a subset of
the input samples, including the original labels, are returned in the result
vector. `by` and `without` are only used to bucket the input vector.

`quantile` calculates the φ-quantile, the value that ranks at number φ*N among
the N metric values of the dimensions aggregated over. φ is provided as the
aggregation parameter. For example, `quantile(0.5, ...)` calculates the median,
`quantile(0.95, ...)` the 95th percentile. For φ = `NaN`, `NaN` is returned. For φ < 0, `-Inf` is returned. For φ > 1, `+Inf` is returned.

Example:

If the metric `http_requests_total` had time series that fan out by
`application`, `instance`, and `group` labels, we could calculate the total
number of seen HTTP requests per application and group over all instances via:

    sum without (instance) (http_requests_total)

Which is equivalent to:

     sum by (application, group) (http_requests_total)

If we are just interested in the total of HTTP requests we have seen in **all**
applications, we could simply write:

    sum(http_requests_total)

To count the number of binaries running each build version we could write:

    count_values("version", build_version)

To get the 5 largest HTTP requests counts across all instances we could write:

    topk(5, http_requests_total)

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

## Operators for native histograms

Native histograms are an experimental feature. Ingesting native histograms has
to be enabled via a [feature flag](../../feature_flags.md#native-histograms). Once
native histograms have been ingested, they can be queried (even after the
feature flag has been disabled again). However, the operator support for native
histograms is still very limited.

Logical/set binary operators work as expected even if histogram samples are
involved. They only check for the existence of a vector element and don't
change their behavior depending on the sample type of an element (float or
histogram). The `count` aggregation operator works similarly.

The binary `+` and `-` operators between two native histograms and the `sum`
and `avg` aggregation operators to aggregate native histograms are fully
supported. Even if the histograms involved have different bucket layouts, the
buckets are automatically converted appropriately so that the operation can be
performed. (With the currently supported bucket schemas, that's always
possible.) If either operator has to aggregate a mix of histogram samples and
float samples, the corresponding vector element is removed from the output
vector entirely.

The binary `*` operator works between a native histogram and a float in any
order, while the binary `/` operator can be used between a native histogram
and a float in that exact order.

All other operators (and unmentioned cases for the above operators) do not
behave in a meaningful way. They either treat the histogram sample as if it
were a float sample of value 0, or (in case of arithmetic operations between a
scalar and a vector) they leave the histogram sample unchanged. This behavior
will change to a meaningful one before native histograms are a stable feature.
