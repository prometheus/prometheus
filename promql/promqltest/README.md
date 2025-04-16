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

> **Note:** The `eval` command variants (`eval_fail`, `eval_warn`, `eval_info`, and `eval_ordered`) are deprecated. Use the new `expect` lines instead (explained in the [`eval` command](#eval-command) section). Additionally, `expected_fail_message` and `expected_fail_regexp` are also deprecated.

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

… will create a single series with labels `my_metric{env="prod"}`, with the following points:

* t=0: value is 5
* t=1m: value is 2
* t=2m: value is 5
* t=3m: value is 8
* t=4m: no point
* t=5m: stale marker
* t=6m: native histogram with schema 1, sum -3, count 22 and bucket counts 5, 10 and 7

Each `load` command is additive - it does not replace any data loaded in a previous `load` command.
Use `clear` to remove all loaded data.

### Native histograms with custom buckets (NHCB)

When loading a batch of classic histogram float series, you can optionally append the suffix `_with_nhcb` to convert them to native histograms with custom buckets and load both the original float series and the new histogram series.

## `clear` command

`clear` removes all data previously loaded with `load` commands.

## `eval` command

`eval` runs a query against the test environment and asserts that the result is as expected. 
It requires the query to succeed without any failures unless an `expect fail` line is provided. Previously `eval` expected no `info` or `warn` annotation, but now `expect no_info` and `expect no_warn` lines must be explicitly provided.

Both instant and range queries are supported.

The syntax is as follows:

```
# Instant query
eval instant at <time> <query>
    <expect>
    ...
    <expect>
    <series> <points>
    ...
    <series> <points>
    
# Range query
eval range from <start> to <end> step <step> <query>
    <expect>
    ...
    <expect>
    <series> <points>
    ...
    <series> <points>
```

* `<time>` is the timestamp to evaluate the instant query at (eg. `1m`)
* `<start>` and `<end>` specify the time range of the range query, and use the same syntax as `<time>`
* `<step>` is the step of the range query, and uses the same syntax as `<time>` (eg. `30s`) 
* `<expect>`(optional) specifies expected annotations, errors, or result ordering.
* `<series>` and `<points>` specify the expected values, and follow the same syntax as for `load` above

### `expect` Syntax

```
expect <type> <match_type> <string>
```

#### Parameters

* `<type>` is the expectation type:
    * `fail` expects the query to fail.
    * `info` expects the query to return at least one info annotation.
    * `warn` expects the query to return at least one warn annotation.
    * `no_info` expects the query to return no info annotation.
    * `no_warn` expects the query to return no warn annotation.
    * `ordered` expects the query to return the results in the specified order.
* `<match_type>` (optional) specifies message matching type for annotations:
    * `msg` for exact string match.
    * `regex` for regular expression match.
    * **Not applicable** for `ordered`, `no_info`, and `no_warn`.
* `<string>` is the expected annotation message.

For example:

```
eval instant at 1m sum by (env) (my_metric)
    expect warn
    expect no_info
    {env="prod"} 5
    {env="test"} 20
    
eval range from 0 to 3m step 1m sum by (env) (my_metric)
    expect warn msg something went wrong
    expect info regex something went (wrong|boom)
    {env="prod"} 2 5 10 20
    {env="test"} 10 20 30 45

eval instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
expect fail

eval instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
expect fail msg "vector cannot contain metrics with the same labelset"

eval instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
expect fail regex "vector cannot contain metrics .*|something else went wrong"

eval instant at 1m sum by (env) (my_metric)
expect ordered
{env="prod"} 5
{env="test"} 20
```

There can be multiple `<expect>` lines for a given `<type>`. Each `<type>` validates its corresponding annotation, error, or ordering while ignoring others.

Every `<expect>` line must match at least one corresponding annotation or error.

If at least one `<expect>` line of type `warn` or `info` is present, then all corresponding annotations must have a matching `expect` line.
