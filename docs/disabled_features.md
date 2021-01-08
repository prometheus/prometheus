---
title: Disabled Features
sort_rank: 10
---

# Disabled Features

Here is a list of features that are disabled by default since they are breaking changes.
They can be enabled using the `--enable-feature` flag and will be enabled by default
in future when it's adopted widely.

## `@` Modifier in PromQL

`--enable-feature=promql-at-modifier`

This feature lets you specify the evaluation time for instant vector selectors,
range vector selectors, and subqueries. More details can be found [here](querying/basics.md#@-modifier).
