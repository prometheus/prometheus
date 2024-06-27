# The PromQL test scripting language

This package contains two things:

* an implementation of a test scripting language for PromQL engines
* a predefined set of tests written in that scripting language

The predefined set of tests can be run against any PromQL engine implementation by calling `promqltest.RunBuiltinTests()`.
Any other test script can be run with `promqltest.RunTest()`.

The rest of this document explains the test scripting language.

Each test script is written in plain text.

Comments can be given by prefixing the comment with a `#`, for example:

```
# This is a comment.
```

Each test file contains a series of commands. There are three kinds of commands:

* `load`
* `clear`
* `eval`

Each command is executed in the order given in the file.

## `load` command

`load` adds some data to the test environment.

The syntax is as follows:

```
load <interval>
    <series> <points>
    ...
    <series> <points>
```

* `<interval>` is the step between points (eg. `1m` or `30s`)
* `<series>` is a Prometheus series name in the usual `metric{label="value"}` syntax
* `<points>` is a specification of the points to add for that series, following the same expanding syntax as for `promtool unittest` documented [here](../../docs/configuration/unit_testing_rules.md#series)

For example:

```
load 1m
    my_metric{env="prod"} 5 2+3x2 _ stale {{schema:1 sum:3 count:22 buckets:[5 10 7]}}
```

...will create a single series with labels `my_metric{env="prod"}`, with the following points:

* t=0: value is 5
* t=1m: value is 2
* t=2m: value is 5
* t=3m: value is 7
* t=4m: no point
* t=5m: stale marker
* t=6m: native histogram with schema 1, sum -3, count 22 and bucket counts 5, 10 and 7

Each `load` command is additive - it does not replace any data loaded in a previous `load` command.
Use `clear` to remove all loaded data.

## `clear` command

`clear` removes all data previously loaded with `load` commands.

## `eval` command

`eval` runs a query against the test environment and asserts that the result is as expected.

Both instant and range queries are supported.

The syntax is as follows:

```
# Instant query
eval instant at <time> <query>
    <series> <points>
    ...
    <series> <points>
    
# Range query
eval range from <start> to <end> step <step> <query>
    <series> <points>
    ...
    <series> <points>
```

* `<time>` is the timestamp to evaluate the instant query at (eg. `1m`)
* `<start>` and `<end>` specify the time range of the range query, and use the same syntax as `<time>`
* `<step>` is the step of the range query, and uses the same syntax as `<time>` (eg. `30s`)
* `<series>` and `<points>` specify the expected values, and follow the same syntax as for `load` above

For example:

```
eval instant at 1m sum by (env) (my_metric)
    {env="prod"} 5
    {env="test"} 20
    
eval range from 0 to 3m step 1m sum by (env) (my_metric)
    {env="prod"} 2 5 10 20
    {env="test"} 10 20 30 45
```

Instant queries also support asserting that the series are returned in exactly the order specified: use `eval_ordered instant ...` instead of `eval instant ...`.
This is not supported for range queries.

It is also possible to test that queries fail: use `eval_fail instant ...` or `eval_fail range ...`.
`eval_fail` optionally takes an expected error message string or regexp to assert that the error message is as expected.

For example:

```
# Assert that the query fails for any reason without asserting on the error message.
eval_fail instant at 1m ceil({__name__=~'testmetric1|testmetric2'})

# Assert that the query fails with exactly the provided error message string.
eval_fail instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
    expected_fail_message vector cannot contain metrics with the same labelset

# Assert that the query fails with an error message matching the regexp provided.
eval_fail instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
    expected_fail_regexp (vector cannot contain metrics .*|something else went wrong)
```
