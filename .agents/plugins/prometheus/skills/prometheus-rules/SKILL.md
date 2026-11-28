---
name: prometheus-rules
description: >-
  Write Prometheus recording and alerting rules, template alert annotations,
  check them with promtool check rules, and unit-test them with promtool test
  rules (input_series notation, alert_rule_test, promql_expr_test). Use when
  adding or changing rule files, debugging an alert that does or does not
  fire, or setting up rule tests in CI.
---

# Recording and alerting rules

References: `docs/configuration/recording_rules.md`,
`docs/configuration/alerting_rules.md`,
`docs/configuration/unit_testing_rules.md`,
`docs/configuration/template_examples.md`.

## Rule files

```yaml
groups:
  - name: api
    interval: 1m            # optional; defaults to global evaluation_interval
    rules:
      - record: job:http_requests:rate5m
        expr: sum by (job) (rate(http_requests_total[5m]))

      - alert: InstanceDown
        expr: up == 0
        for: 5m
        keep_firing_for: 5m  # optional
        labels:
          severity: page
        annotations:
          summary: "Instance {{ $labels.instance }} down"
          description: "{{ $labels.instance }} of job {{ $labels.job }} has been down for more than 5 minutes."
```

- Recording rule names must be valid metric names; use the
  `level:metric:operations` convention.
- Alert names must be valid label values.
- Rules in a group run sequentially at the same evaluation time, so a
  recording rule can feed a later rule in the same group.
- Templates: `$labels`, `$value`, `$externalLabels`, plus template
  functions such as `humanize`.

Load them from `prometheus.yml`:

```yaml
rule_files:
  - "rules/*.yml"
```

## Check

```bash
promtool check rules rules/*.yml
promtool check rules --lint=all --lint-fatal rules/*.yml
```

Exit 0 when valid, 1 on errors, 3 on lint errors with `--lint-fatal`.

## Unit test

`test.yml`:

```yaml
rule_files:
  - alerts.yml
evaluation_interval: 1m
tests:
  - interval: 1m
    input_series:
      - series: 'up{job="prometheus", instance="localhost:9090"}'
        values: '0 0 0 0 0 0 0 0 0 0 0'
      - series: 'go_goroutines{job="prometheus", instance="localhost:9090"}'
        values: '10+10x2 30+20x5'   # 10 20 30 30 50 70 90 110 130
    alert_rule_test:
      - eval_time: 10m
        alertname: InstanceDown
        exp_alerts:
          - exp_labels: {severity: page, instance: localhost:9090, job: prometheus}
            exp_annotations:
              summary: "Instance localhost:9090 down"
    promql_expr_test:
      - expr: go_goroutines > 5
        eval_time: 4m
        exp_samples:
          - labels: 'go_goroutines{job="prometheus",instance="localhost:9090"}'
            value: 50
```

```bash
promtool test rules test.yml
```

Series notation: `a+bxn` starts at `a` and adds `b` `n` times; `_` is a
missing sample; `stale` marks staleness. Time functions start at 0 (the Unix
epoch) inside tests.

## Debugging an alert

- `ALERTS{alertname="X"}` shows pending/firing state.
- The `/alerts` and `/rules` pages show the last evaluation and errors.
- An alert stays `pending` until `for` has elapsed on consecutive evaluations.
