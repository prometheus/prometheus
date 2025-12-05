---
title: Unit testing rule coverage
sort_rank: 7
---

# Unit testing rule coverage

promtool provides built-in coverage reporting for Prometheus rule tests. This feature helps ensure all alerting and recording rules have adequate test coverage, preventing untested rules from reaching production.

## Overview

The coverage feature tracks which rules are tested by your unit tests and generates reports showing:
- Overall coverage percentage
- Coverage by rule type (alerts vs recording rules)
- Coverage by rule group
- List of untested rules
- Dependency tracking for indirect coverage

## Basic usage

Enable coverage reporting with the `--coverage` flag:

```bash
promtool test rules --coverage --rule-files rules/*.yml tests/*.yml
```

## Coverage flags

| Flag | Description | Default |
|------|-------------|---------|
| `--coverage` | Enable test coverage reporting | `false` |
| `--rule-files` | Rule files to calculate coverage for (can be specified multiple times) | |
| `--coverage-format` | Output format: `text`, `json`, or `junit` | `text` |
| `--coverage-output` | File path to write coverage report | stdout |
| `--min-coverage` | Minimum required coverage percentage (0-100) | `0` |
| `--fail-on-untested` | Exit with error if any rule is untested | `false` |
| `--coverage-by-group` | Report coverage statistics by rule group | `false` |
| `--track-dependencies` | Track transitive coverage through rule dependencies | `false` |
| `--ignore-pattern` | Glob pattern for rules to ignore in coverage calculation | |

## Output formats

### Text format (default)

Human-readable output with colored terminal support:

```
📊 Prometheus Rules Test Coverage Report
════════════════════════════════════════════════════════════

Overall Coverage: 75.0% (9/12 rules)

📈 Coverage by Type:
  ├─ Alert Rules     : 83.3% (5/6)
  └─ Recording Rules : 66.7% (4/6)

⚠️  Untested Rules (3):
  • alerts/HighMemoryUsage [alert] rules/alerts.yml:15
  • records/cpu:rate5m [record] rules/records.yml:8
  • records/memory:usage [record] rules/records.yml:12
```

### JSON format

Machine-readable format for CI/CD integration:

```bash
promtool test rules --coverage --coverage-format json --rule-files rules/*.yml tests/*.yml
```

Output:
```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "summary": {
    "total_rules": 12,
    "tested_rules": 9,
    "untested_rules": 3,
    "coverage_percent": 75.0,
    "alert_coverage": 83.3,
    "record_coverage": 66.7
  },
  "untested_rules": [
    {
      "name": "HighMemoryUsage",
      "group": "alerts",
      "type": "alert",
      "file": "rules/alerts.yml",
      "line": 15
    }
  ]
}
```

### JUnit XML format

For integration with CI/CD tools that support JUnit format:

```bash
promtool test rules --coverage --coverage-format junit --coverage-output coverage.xml --rule-files rules/*.yml tests/*.yml
```

## CI/CD integration

### GitHub Actions

```yaml
- name: Test Prometheus Rules with Coverage
  run: |
    promtool test rules \
      --coverage \
      --rule-files rules/*.yml \
      --min-coverage 80 \
      --fail-on-untested \
      tests/*.yml

- name: Upload Coverage Report
  uses: actions/upload-artifact@v2
  if: always()
  with:
    name: coverage-report
    path: coverage.json
```

### Jenkins

```groovy
stage('Test Rules') {
    sh '''
        promtool test rules \
            --coverage \
            --rule-files rules/*.yml \
            --coverage-format junit \
            --coverage-output coverage.xml \
            --junit test-results.xml \
            --min-coverage 80 \
            tests/*.yml
    '''

    junit 'test-results.xml'
    publishHTML([
        reportDir: '.',
        reportFiles: 'coverage.xml',
        reportName: 'Coverage Report'
    ])
}
```

### GitLab CI

```yaml
test_rules:
  script:
    - promtool test rules --coverage --rule-files rules/*.yml --coverage-format json --coverage-output coverage.json tests/*.yml
  artifacts:
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.json
  coverage: '/Overall Coverage: (\d+\.\d+)%/'
```

## Coverage thresholds

Enforce minimum coverage requirements:

```bash
# Fail if coverage is below 80%
promtool test rules --coverage --min-coverage 80 --rule-files rules/*.yml tests/*.yml

# Fail if any rule is untested
promtool test rules --coverage --fail-on-untested --rule-files rules/*.yml tests/*.yml
```

## Dependency tracking

Enable dependency tracking to identify rules that are indirectly tested through other rules:

```bash
promtool test rules --coverage --track-dependencies --rule-files rules/*.yml tests/*.yml
```

This is useful when recording rules are tested indirectly through alerts that use them:

```yaml
# Recording rule
- record: cpu:usage:rate5m
  expr: rate(cpu_usage_total[5m])

# Alert that uses the recording rule (indirectly tests it)
- alert: HighCPU
  expr: cpu:usage:rate5m > 0.8
```

## Ignoring rules

Exclude specific rules from coverage calculation:

```bash
# Ignore all rules in the "deprecated" group
promtool test rules --coverage --ignore-pattern "deprecated/*" --rule-files rules/*.yml tests/*.yml

# Ignore specific rule patterns
promtool test rules --coverage --ignore-pattern "*_test" --rule-files rules/*.yml tests/*.yml
```

## Best practices

1. **Set coverage thresholds**: Use `--min-coverage` to maintain coverage standards
2. **Fail on untested rules**: Use `--fail-on-untested` in CI to catch missing tests
3. **Track by group**: Use `--coverage-by-group` to identify which teams need to improve coverage
4. **Regular reporting**: Generate coverage reports as part of your CI pipeline
5. **Progressive improvement**: Start with current coverage and gradually increase thresholds

## Example workflow

```bash
# 1. Check current coverage
promtool test rules --coverage --rule-files rules/*.yml tests/*.yml

# 2. Generate detailed report
promtool test rules \
  --coverage \
  --coverage-by-group \
  --track-dependencies \
  --coverage-format json \
  --coverage-output coverage.json \
  --rule-files rules/*.yml \
  tests/*.yml

# 3. Enforce in CI/CD
promtool test rules \
  --coverage \
  --min-coverage 80 \
  --fail-on-untested \
  --coverage-format junit \
  --coverage-output coverage.xml \
  --rule-files rules/*.yml \
  tests/*.yml
```