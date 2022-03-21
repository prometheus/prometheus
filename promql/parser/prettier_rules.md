# Prettifying PromQL expressions
This files contains rules for prettifying PromQL expressions.

### Keywords
`max_characters_per_line`: Maximum number of characters that will be allowed in a single stringified node

## Rules
1. A node exceeding the `max_characters_per_line` will qualify for split unless
   1. It is a function call with one argument eg. `rate(foo[1m])`
   2. It is a `VectorSelector`, `SubqueryExpr` and `MatrixSelector`
2. Nodes that are nested within another node will be prettified only if they exceed the `max_characters_per_line`
3. Following conditions do not involve splitting:
   1. Label sets in a `VectorSelector`. These will be in the same line as metric_name, separated by commas and a space
   2. Label groupings like `by`, `without`, `on`, `ignoring`
4. Expressions like `sum(expression) without (label_matchers)` will be modified to `sum without(label_matchers) (expression)`
5. Functional call args will be split to different lines if they are more than 1.

Example:
`rate(vector_selector[5m])` will not split
`label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*")` will be split to

```text
label_replace(
  up{job="api-server",service="a:c"},
  "foo",
  "$1",
  "service",
  "(.*):.*"
)
```
8. At present, the prettier does not preserve comments
