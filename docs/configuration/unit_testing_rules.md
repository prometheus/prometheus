---
title: Unit Testing for Rules
sort_rank: 6
---

# Unit Testing for Rules

You can use `promtool` to test your rules.

```shell
# For a single test file.
./promtool test rules test.yml

# If you have multiple test files, say test1.yml,test2.yml,test2.yml
./promtool test rules test1.yml test2.yml test3.yml
```

## Test file format

```yaml
# This is a list of rule files to consider for testing.
rule_files:
  [ - <file_name> ]

# optional, default = 1m
evaluation_interval: <duration>

# The order in which group names are listed below will be the order of evaluation of 
# rule groups (at a given evaluation time). The order is guaranteed only for the groups mentioned below. 
# All the groups need not be mentioned below.
group_eval_order:
  [ - <group_name> ]

# All the tests are listed here.
tests:
  [ - <test_group> ]
```

### `<test_group>`

``` yaml
# Series data
interval: <duration>
input_series:
  [ - <series> ]

# Unit tests for the above data.

# Unit tests for alerting rules. We consider the alerting rules from the input file.
alert_rule_test:
  [ - <alert_test_case> ]

# Unit tests PromQL expressions.
promql_expr_test:
  [ - <promql_test_case> ]
```

### `<series>`

```yaml
# This follows the usual series notation '<metric name>{<label name>=<label value>, ...}'
# Examples: 
#      series_name{label1="value1", label2="value2"}
#      go_goroutines{job="prometheus", instance="localhost:9090"}
series: <string>

# This uses expanding notation.
# Expanding notation: 
#     'a+bxc' becomes 'a a+b a+(2*b) a+(3*b) … a+(c*b)'
#     'a-bxc' becomes 'a a-b a-(2*b) a-(3*b) … a-(c*b)'
# Examples:
#     1. '-2+4x3' becomes '-2 2 6 10'
#     2. ' 1-2x4' becomes '1 -1 -3 -5 -7'
values: <string>
```

### `<alert_test_case>`

Prometheus allows you to have same alertname for different alerting rules. Hence in this unit testing, you have to list the union of all the firing alerts for the alertname under a single `<alert_test_case>`.

``` yaml
# It's the time elapsed from time=0s when the alerts have to be checked.
eval_time: <duration>

# Name of the alert to be tested.
alertname: <string>

# List of expected alerts which are firing under the given alertname at 
# given evaluation time. If you want to test if an alerting rule should 
# not be firing, then you can mention the above fields and leave 'exp_alerts' empty.
exp_alerts:
  [ - <alert> ]
```

### `<alert>`

``` yaml
# These are the expanded labels and annotations of the expected alert. 
# Note: labels also include the labels of the sample associated with the 
# alert (same as what you see in `/alerts`, without series `__name__` and `alertname`)
exp_labels:
  [ <labelname>: <string> ]
exp_annotations:
  [ <labelname>: <string> ]
```

### `<promql_test_case>`

```yaml
# Expression to evaluate
expr: <string>

# It's the time elapsed from time=0s when the alerts have to be checked.
eval_time: <duration>

# Expected samples at the given evaluation time.
exp_samples:
  [ - <sample> ]
```

### `<sample>`

```yaml
# Labels of the sample in usual series notation '<metric name>{<label name>=<label value>, ...}'
# Examples: 
#      series_name{label1="value1", label2="value2"}
#      go_goroutines{job="prometheus", instance="localhost:9090"}
labels: <string>

# The expected value of the promql expression.
value: <number>
```

## Example 

This is an example input file for unit testing which passes the test. `test.yml` is the test file which follows the syntax above and `alerts.yml` contains the alerting rules.

With `alerts.yml` in the same directory, run `./promtool test rules test.yml`.

### `test.yml`

```yaml
# This is the main input for unit testing. 
# Only this file is passed as command line argument.

rule_files:
    - alerts.yml

evaluation_interval: 1m

tests:
    # Test 1.
    - interval: 1m
      # Series data.
      input_series:
          - series: 'up{job="prometheus", instance="localhost:9090"}'
            values: '0 0 0 0 0 0 0 0 0 0 0 0 0 0 0'
          - series: 'up{job="node_exporter", instance="localhost:9100"}'
            values: '1+0x6 0 0 0 0 0 0 0 0' # 1 1 1 1 1 1 1 0 0 0 0 0 0 0 0
          - series: 'go_goroutines{job="prometheus", instance="localhost:9090"}'
            values: '10+10x2 30+20x5' # 10 20 30 30 50 70 90 110 130
          - series: 'go_goroutines{job="node_exporter", instance="localhost:9100"}'
            values: '10+10x7 10+30x4' # 10 20 30 40 50 60 70 80 10 40 70 100 130

      # Unit test for alerting rules.
      alert_rule_test:
          # Unit test 1.
          - eval_time: 10m
            alertname: InstanceDown
            exp_alerts:
                # Alert 1.
                - exp_labels:
                      severity: page
                      instance: localhost:9090
                      job: prometheus
                  exp_annotations:
                      summary: "Instance localhost:9090 down"
                      description: "localhost:9090 of job prometheus has been down for more than 5 minutes."
      # Unit tests for promql expressions.
      promql_expr_test:
          # Unit test 1.
          - expr: go_goroutines > 5
            eval_time: 4m
            exp_samples:
                # Sample 1.
                - labels: 'go_goroutines{job="prometheus",instance="localhost:9090"}'
                  value: 50
                # Sample 2.
                - labels: 'go_goroutines{job="node_exporter",instance="localhost:9100"}'
                  value: 50
```

### `alerts.yml`

```yaml
# This is the rules file.

groups:
- name: example
  rules:

  - alert: InstanceDown
    expr: up == 0
    for: 5m
    labels:
        severity: page
    annotations:
        summary: "Instance {{ $labels.instance }} down"
        description: "{{ $labels.instance }} of job {{ $labels.job }} has been down for more than 5 minutes."

  - alert: AnotherInstanceDown
    expr: up == 0
    for: 10m
    labels:
        severity: page
    annotations:
        summary: "Instance {{ $labels.instance }} down"
        description: "{{ $labels.instance }} of job {{ $labels.job }} has been down for more than 5 minutes."
```
