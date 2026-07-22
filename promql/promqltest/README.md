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

### Start timestamps (ST)

Each sample loaded by a `load` command can optionally carry a *start timestamp* (ST). Start
timestamps allow PromQL functions like `rate()` and `increase()` to detect counter resets and
delta temporality data (OpenTelemetry-style cumulative and delta metrics).

#### Per-metric `@st` line

Place an `@st` line immediately before a sample line to assign start timestamp offsets
to each corresponding sample:

```
load 1m
    my_counter@st -1mx4
    my_counter 0+60x14
```

The `@st` offset sequence uses the same expanding syntax as sample values, but with
durations instead of numbers:

| Pattern | Meaning |
|---------|---------|
| `-1m` | One position with offset -1 minute |
| `-1mx4` | Five positions, each with offset -1 minute |
| `-30s-1mx2` | Three positions: -30s, -90s, -150s |
| `-1m+30sx2` | Three positions: -60s, -30s, 0s |
| `_` | One omitted position (no ST) |
| `_x3` | Three omitted positions |
| `*` | Repeat the previous non-omitted offset |
| `*x3` | Repeat the previous offset three more times |

The ST for each sample is computed as `sample_timestamp + offset`. The `@st` line must
have the same number of positions as its following sample line and use the same metric name.

#### Standalone `@st` default

A standalone `@st` line (without a metric name) sets a default ST offset sequence for all
subsequent sample lines in the same `load` block. This is especially useful for cumulative
counters with constant start timestamps:

```
load 1m
    @st -1m
    counter_a 0+60x14
    counter_b 0+60x10
```

Here every sample of both `counter_a` and `counter_b` gets ST = sample_timestamp - 1 minute.
The default ST sequence is cycled if it has fewer positions than the sample line.

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
* `<expect range vector>` (optional) for an instant query you can specify expected range vector timestamps
* `<expect string> "<string>"` (optional) for matching a string literal
* `<series>` and `<points>` specify the expected values, and follow the same syntax as for `load` above

### Special handling of counter reset hints in native histograms

Native histograms as part of `<points>` may or may not contain an explicit
`counter_reset_hint` property. If a `counter_reset_hint` is provided
explicitly, the counter reset hint of the histogram is tested to have the
provided value (`unknown`, `reset`, `not_reset`, or `gauge`). However, if no
`counter_reset_hint` is specified, the `counter_reset_hint` is not tested at
all (rather than testing for the usual default value `unknown`).

### `expect string`

This can be used to specify that a string literal is the expected result.

Note that this is only supported on instant queries.

For example;

```
eval instant at 50m ("Foo")
 expect string "Foo"
```

The expected string value must be within quotes. Double or back quotes are supported.

### `expect range vector`

This can be used to specify the expected timestamps on a range vector resulting from an instant query.

```
expect range vector <start> to <end> step <step>
```

For example;
```
load 10s
  some_metric{env="a"} 1+1x5
  some_metric{env="b"} 2+2x5
eval instant at 1m some_metric[1m]
  expect range vector from 10s to 1m step 10s
  some_metric{env="a"} 2 3 4 5 6
  some_metric{env="b"} 4 6 8 10 12
```

### `expect` Syntax

```
expect <type> <match_type>: <string>
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
    expect warn msg: something went wrong
    expect info regex: something went (wrong|boom)
    {env="prod"} 2 5 10 20
    {env="test"} 10 20 30 45

eval instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
expect fail

eval instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
expect fail msg: "vector cannot contain metrics with the same labelset"

eval instant at 1m ceil({__name__=~'testmetric1|testmetric2'})
expect fail regex: "vector cannot contain metrics .*|something else went wrong"

eval instant at 1m sum by (env) (my_metric)
expect ordered
{env="prod"} 5
{env="test"} 20
```

There can be multiple `<expect>` lines for a given `<type>`. Each `<type>` validates its corresponding annotation, error, or ordering while ignoring others.

Every `<expect>` line must match at least one corresponding annotation or error.

If at least one `<expect>` line of type `warn` or `info` is present, then all corresponding annotations must have a matching `expect` line.

#### Migrating Test Files to the New Syntax

- All `.test` files in the directory specified by the --dir flag will be updated in place.
- Deprecated syntax will be replaced with the recommended `expect` line statements.

Usage:
```sh
go run ./promql/promqltest/cmd/migrate/main.go --mode=strict [--dir=<directory>]
```

The `--mode` flag controls how expectations are migrated:
- `strict`: Strictly migrates all expectations to the new syntax.
  This is probably more verbose than intended because the old syntax
  implied many constraints that are often not needed.
- `basic`: Like `strict` but never creates `no_info` and `no_warn`
  expectations. This can be a good starting point to manually add 
  `no_info` and `no_warn` expectations and/or remove `info` and 
  `warn` expectations as needed.
- `tolerant`: Only creates `expect fail` and `expect ordered` where
  appropriate. All desired expectations about presence or absence 
  of `info` and `warn` have to be added manually.

All three modes create valid passing tests from previously passing tests.
`basic` and `tolerant` just test fewer expectations than the previous tests.

The --dir flag specifies the directory containing test files to migrate.
