0.20.0 / 2021-08-10
===================

* **[Feature]**: Support `present_over_time`

0.19.0 / 2021-05-19
===================

* **[Feature]**: Provide an alternative `MetricName` top-level entrypoint
* **[Enhancement]**: Improve parsing of numbers
* **[BugFix]**: Fix parsing metric names starting with `Inf`/`NaN` like `infra`

0.18.0 / 2021-03-26
===================

* **[Feature]**: Support negative offset

0.17.0 / 2021-03-22
===================

* **[Feature]** Support signed numbers and special float value literals

0.16.0 / 2021-03-05
===================

* **[Feature]** Support `@` modifiers. You can learn more about it in the [Prometheus Blog](https://prometheus.io/blog/2021/02/18/introducing-the-@-modifier/)
* **[Feature]** Support new PromQL functions (`clamp()`, `sgn()`, `last_over_time()`)
