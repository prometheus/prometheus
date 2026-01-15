<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_rule_evaluation_duration_histogram_seconds` | histogram | s | The duration of rule evaluations as a histogram. |
| `prometheus_rule_evaluation_duration_seconds` | histogram | s | The duration of rule group evaluations. |
| `prometheus_rule_evaluation_failures_total` | counter | {failure} | The total number of rule evaluation failures. |
| `prometheus_rule_evaluations_total` | counter | {evaluation} | The total number of rule evaluations. |
| `prometheus_rule_group_duration_histogram_seconds` | histogram | s | The duration of rule group evaluations as a histogram. |
| `prometheus_rule_group_duration_seconds` | histogram | s | The duration of rule group evaluations. |
| `prometheus_rule_group_interval_seconds` | gauge | s | The interval of a rule group. |
| `prometheus_rule_group_iterations_missed_total` | counter | {iteration} | The total number of rule group evaluations missed due to slow rule group evaluation. |
| `prometheus_rule_group_iterations_total` | counter | {iteration} | The total number of scheduled rule group evaluations. |
| `prometheus_rule_group_last_duration_seconds` | gauge | s | The duration of the last rule group evaluation. |
| `prometheus_rule_group_last_evaluation_samples` | gauge | {sample} | The number of samples returned during the last rule group evaluation. |
| `prometheus_rule_group_last_evaluation_timestamp_seconds` | gauge | s | The timestamp of the last rule group evaluation. |
| `prometheus_rule_group_last_restore_duration_seconds` | gauge | s | The duration of the last alert restoration from the ALERTS_FOR_STATE series. |
| `prometheus_rule_group_last_rule_duration_sum_seconds` | gauge | s | The sum of the durations of all rules in the last rule group evaluation. |
| `prometheus_rule_group_rules` | gauge | {rule} | The number of rules in a rule group. |


## Metric Details


### `prometheus_rule_evaluation_duration_histogram_seconds`

The duration of rule evaluations as a histogram.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_rule_evaluation_duration_seconds`

The duration of rule group evaluations.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_rule_evaluation_failures_total`

The total number of rule evaluation failures.

- **Type:** counter
- **Unit:** {failure}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_evaluations_total`

The total number of rule evaluations.

- **Type:** counter
- **Unit:** {evaluation}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_duration_histogram_seconds`

The duration of rule group evaluations as a histogram.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_rule_group_duration_seconds`

The duration of rule group evaluations.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_rule_group_interval_seconds`

The interval of a rule group.

- **Type:** gauge
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_iterations_missed_total`

The total number of rule group evaluations missed due to slow rule group evaluation.

- **Type:** counter
- **Unit:** {iteration}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_iterations_total`

The total number of scheduled rule group evaluations.

- **Type:** counter
- **Unit:** {iteration}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_last_duration_seconds`

The duration of the last rule group evaluation.

- **Type:** gauge
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_last_evaluation_samples`

The number of samples returned during the last rule group evaluation.

- **Type:** gauge
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_last_evaluation_timestamp_seconds`

The timestamp of the last rule group evaluation.

- **Type:** gauge
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_last_restore_duration_seconds`

The duration of the last alert restoration from the ALERTS_FOR_STATE series.

- **Type:** gauge
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_last_rule_duration_sum_seconds`

The sum of the durations of all rules in the last rule group evaluation.

- **Type:** gauge
- **Unit:** s
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |



### `prometheus_rule_group_rules`

The number of rules in a rule group.

- **Type:** gauge
- **Unit:** {rule}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `rule_group` | string | The rule group name. | alerting_rules.yml;my_group |

