0.17.0 / 2021-08-10
===================

* **[Feature]**: Support `present_over_time`
* **[Feature]**: HTTP method used to contact Prometheus is now configurable.

0.16.0 / 2021-05-20
===================

* **[Feature]**: Support partial PromQL language called `MetricName`. Can be used to autocomplete only the metric
  name. (#142)
* **[Feature]**: Autocomplete `NaN` and `Inf` (#141)
* **[Enhancement]**: Fetch series using the HTTP `POST` method (#139)
* **[Enhancement]**: Upgrade lezer-promql that fixed the parsing of metric names starting with `Inf`/`NaN` like infra (#142)  
* **[BreakingChange]**: The constant `promQLLanguage` has been changed to be a function. It takes a `LanguageType` as a 
  parameter (#142)

0.15.0 / 2021-04-13
===================

* **[Feature]**: Provide a way to inject an initial metric list for the autocompletion (#134)
* **[Enhancement]**: Autocomplete metrics/function/aggregation when the editor is empty (#133)
* **[Enhancement]**: Improve the documentation to reflect what the lib is providing. (#134)
* **[Change]**: Export the essential interface in the root index of the lib. (#132)
* **[Change]**: Downgrade the NodeJS version required (from 14 to 12) (#112)
* **[BreakingChange]**: Support CommonJS module. (#130)

Note that this requires to change the import path if you are using something not exported by the root index of lib. For
example: `import { labelMatchersToString } from 'codemirror-promql/parser/matcher';`
becomes `import { labelMatchersToString } from 'codemirror-promql/esm/parser/matcher';`
or `import { labelMatchersToString } from 'codemirror-promql/cjs/parser/matcher';`

0.14.1 / 2021-04-07
===================

* **[Enhancement]**: Provide getter and setter to easily manipulate the different objects exposed by the lib
* **[BugFix]**: fix the autocompletion of the labels after a comma (in a label matcher list or in a grouping label list)

0.14.0 / 2021-03-26
===================

* **[Feature]**: Through the update of [lezer-promql](https://github.com/promlabs/lezer-promql/releases/tag/0.18.0)
  support negative offset
* **[Enhancement]**: Add snippet to ease the usage of the aggregation `topk`, `bottomk` and `count_value`
* **[Enhancement]**: Autocomplete the 2nd hard of subquery time selector

0.13.0 / 2021-03-22
===================
* **[Feature]**: Linter and Autocompletion support 3 new PromQL functions: `clamp` , `last_over_time`, `sgn`
* **[Feature]**: Linter and Autocompletion support the `@` expression.
* **[Enhancement]**: Signature of `CompleteStrategy.promQL` has been updated to support the type `Promise<null>`
* **[BreakingChange]**: Support last version of Codemirror.next (v0.18.0)
* **[BreakingChange]**: Remove the function `enricher`

0.12.0 / 2021-01-12
===================

* **[Enhancement]**: Improve the parsing of `BinExpr` thanks to the changes provided by lezer-promql (v0.15.0)
* **[BreakingChange]**: Support the new version of codemirror v0.17.x

0.11.0 / 2020-12-08
===================

* **[Feature]**: Add the completion of the keyword `bool`. (#89)
* **[Feature]**: Add a function `enricher` that can be used to enrich the completion with a custom one.
* **[Feature]**: Add a LRU caching system. (#71)
* **[Feature]**: You can now configure the maximum number of metrics in Prometheus for which metadata is fetched.
* **[Feature]**: Allow the possibility to inject a custom `CompleteStrategy`. (#83)
* **[Feature]**: Provide the Matchers in the PrometheusClient for the method `labelValues` and `series`. (#84)
* **[Feature]**: Add the method `metricName` in the PrometheusClient that supports a prefix of the metric searched. (#84)
* **[Enhancement]**: Caching mechanism and PrometheusClient are splitted. (#71)
* **[Enhancement]**: Optimize the code of the PrometheusClient when no cache is used.
* **[Enhancement]**: General improvement of the code thanks to Codemirror.next v0.14.0 (for the new tree management) and v0.15.0 (for the new tags/highlight management)
* **[Enhancement]**: Improve the code coverage of the parser concerning the parsing of the function / aggregation.
* **[BugFix]**: In certain case, the linter didn't ignore the comments. (#78)
* **[BreakingChange]**: Use an object instead of a map when querying the metrics metadata.
* **[BreakingChange]**: Support last version of Codemirror.next (v0.15.0).
* **[BreakingChange]**: Change the way the completion configuration is structured.

0.10.2 / 2020-10-18
===================

* **[BugFix]**: Fixed missing autocompletion of binary operators after aggregations

0.10.1 / 2020-10-16
===================

* **[Enhancement]**: Caching of series label names and values for autocompletion is now optimized to be much faster
* **[BugFix]**: Fixed incorrect linter errors around binary operator arguments not separated from the operator by a space

0.10.0 / 2020-10-14
===================

* **[Enhancement]**: The Linter is now checking operation many-to-many, one-to-one, many-to-one and one-to-many
* **[Enhancement]**: The autocompletion is now showing the type of the metric if the type is same for every possible definition of the same metric
* **[Enhancement]**: The autocompletion is supporting the completion of the duration
* **[Enhancement]**: Descriptions have been added for the snippet, the binary operator modifier and the aggregation operator modifier
* **[Enhancement]**: Coverage of the code has been increased (a lot).
* **[BreakingChange]**: Removing LSP support
