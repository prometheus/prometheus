{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'prometheus',
        rules: [
          {
            alert: 'PrometheusBadConfig',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              max_over_time(prometheus_config_last_reload_successful{%(prometheusSelector)s}[5m]) == 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Failed Prometheus configuration reload.',
              description: 'Prometheus %(prometheusName)s has failed to reload its configuration.' % $._config,
            },
          },
          {
            alert: 'PrometheusNotificationQueueRunningFull',
            expr: |||
              # Without min_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              (
                predict_linear(prometheus_notifications_queue_length{%(prometheusSelector)s}[5m], 60 * 30)
              >
                min_over_time(prometheus_notifications_queue_capacity{%(prometheusSelector)s}[5m])
              )
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus alert notification queue predicted to run full in less than 30m.',
              description: 'Alert notification queue of Prometheus %(prometheusName)s is running full.' % $._config,
            },
          },
          {
            alert: 'PrometheusErrorSendingAlertsToSomeAlertmanagers',
            expr: |||
              (
                rate(prometheus_notifications_errors_total{%(prometheusSelector)s}[5m])
              /
                rate(prometheus_notifications_sent_total{%(prometheusSelector)s}[5m])
              )
              * 100
              > 1
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus has encountered more than 1% errors sending alerts to a specific Alertmanager.',
              description: '{{ printf "%%.1f" $value }}%% errors while sending alerts from Prometheus %(prometheusName)s to Alertmanager {{$labels.alertmanager}}.' % $._config,
            },
          },
          {
            alert: 'PrometheusErrorSendingAlertsToAnyAlertmanager',
            expr: |||
              min without(alertmanager) (
                rate(prometheus_notifications_errors_total{%(prometheusSelector)s}[5m])
              /
                rate(prometheus_notifications_sent_total{%(prometheusSelector)s}[5m])
              )
              * 100
              > 3
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Prometheus encounters more than 3% errors sending alerts to any Alertmanager.',
              description: '{{ printf "%%.1f" $value }}%% minimum errors while sending alerts from Prometheus %(prometheusName)s to any Alertmanager.' % $._config,
            },
          },
          {
            alert: 'PrometheusNotConnectedToAlertmanagers',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              max_over_time(prometheus_notifications_alertmanagers_discovered{%(prometheusSelector)s}[5m]) < 1
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus is not connected to any Alertmanagers.',
              description: 'Prometheus %(prometheusName)s is not connected to any Alertmanagers.' % $._config,
            },
          },
          {
            alert: 'PrometheusTSDBReloadsFailing',
            expr: |||
              increase(prometheus_tsdb_reloads_failures_total{%(prometheusSelector)s}[3h]) > 0
            ||| % $._config,
            'for': '4h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus has issues reloading blocks from disk.',
              description: 'Prometheus %(prometheusName)s has detected {{$value | humanize}} reload failures over the last 3h.' % $._config,
            },
          },
          {
            alert: 'PrometheusTSDBCompactionsFailing',
            expr: |||
              increase(prometheus_tsdb_compactions_failed_total{%(prometheusSelector)s}[3h]) > 0
            ||| % $._config,
            'for': '4h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus has issues compacting blocks.',
              description: 'Prometheus %(prometheusName)s has detected {{$value | humanize}} compaction failures over the last 3h.' % $._config,
            },
          },
          {
            alert: 'PrometheusNotIngestingSamples',
            expr: |||
              rate(prometheus_tsdb_head_samples_appended_total{%(prometheusSelector)s}[5m]) <= 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus is not ingesting samples.',
              description: 'Prometheus %(prometheusName)s is not ingesting samples.' % $._config,
            },
          },
          {
            alert: 'PrometheusDuplicateTimestamps',
            expr: |||
              rate(prometheus_target_scrapes_sample_duplicate_timestamp_total{%(prometheusSelector)s}[5m]) > 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus is dropping samples with duplicate timestamps.',
              description: 'Prometheus %(prometheusName)s is dropping {{ printf "%%.4g" $value  }} samples/s with different values but duplicated timestamp.' % $._config,
            },
          },
          {
            alert: 'PrometheusOutOfOrderTimestamps',
            expr: |||
              rate(prometheus_target_scrapes_sample_out_of_order_total{%(prometheusSelector)s}[5m]) > 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus drops samples with out-of-order timestamps.',
              description: 'Prometheus %(prometheusName)s is dropping {{ printf "%%.4g" $value  }} samples/s with timestamps arriving out of order.' % $._config,
            },
          },
          {
            alert: 'PrometheusRemoteStorageFailures',
            expr: |||
              (
                rate(prometheus_remote_storage_failed_samples_total{%(prometheusSelector)s}[5m])
              /
                (
                  rate(prometheus_remote_storage_failed_samples_total{%(prometheusSelector)s}[5m])
                +
                  rate(prometheus_remote_storage_succeeded_samples_total{%(prometheusSelector)s}[5m])
                )
              )
              * 100
              > 1
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Prometheus fails to send samples to remote storage.',
              description: 'Prometheus %(prometheusName)s failed to send {{ printf "%%.1f" $value }}%% of the samples to {{ $labels.remote_name}}:{{ $labels.url }}' % $._config,
            },
          },
          {
            alert: 'PrometheusRemoteWriteBehind',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              (
                max_over_time(prometheus_remote_storage_highest_timestamp_in_seconds{%(prometheusSelector)s}[5m])
              - on(job, instance) group_right
                max_over_time(prometheus_remote_storage_queue_highest_sent_timestamp_seconds{%(prometheusSelector)s}[5m])
              )
              > 120
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Prometheus remote write is behind.',
              description: 'Prometheus %(prometheusName)s remote write is {{ printf "%%.1f" $value }}s behind for {{ $labels.remote_name}}:{{ $labels.url }}.' % $._config,
            },
          },
          {
            alert: 'PrometheusRemoteWriteDesiredShards',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              (
                max_over_time(prometheus_remote_storage_shards_desired{%(prometheusSelector)s}[5m])
              >
                max_over_time(prometheus_remote_storage_shards_max{%(prometheusSelector)s}[5m])
              )
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus remote write desired shards calculation wants to run more than configured max shards.',
              description: 'Prometheus %(prometheusName)s remote write desired shards calculation wants to run {{ $value }} shards for queue {{ $labels.remote_name}}:{{ $labels.url }}, which is more than the max of {{ printf `prometheus_remote_storage_shards_max{instance="%%s",%(prometheusSelector)s}` $labels.instance | query | first | value }}.' % $._config,
            },
          },
          {
            alert: 'PrometheusRuleFailures',
            expr: |||
              increase(prometheus_rule_evaluation_failures_total{%(prometheusSelector)s}[5m]) > 0
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Prometheus is failing rule evaluations.',
              description: 'Prometheus %(prometheusName)s has failed to evaluate {{ printf "%%.0f" $value }} rules in the last 5m.' % $._config,
            },
          },
          {
            alert: 'PrometheusMissingRuleEvaluations',
            expr: |||
              increase(prometheus_rule_group_iterations_missed_total{%(prometheusSelector)s}[5m]) > 0
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'Prometheus is missing rule evaluations due to slow rule group evaluation.',
              description: 'Prometheus %(prometheusName)s has missed {{ printf "%%.0f" $value }} rule group evaluations in the last 5m.' % $._config,
            },
          },
        ],
      },
    ],
  },
}
