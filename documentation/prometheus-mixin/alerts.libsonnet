{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'prometheus',
        rules: [
          {
            alert: 'PrometheusBadConfig',
            expr: |||
              prometheus_config_last_reload_successful{%(prometheusSelector)s} == 0
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Prometheus %(prometheusName)s failed to reload config, see container logs' % $._config,
            },
          },
          {
            alert: 'PrometheusNotificationQueueRunningFull',
            expr: |||
              (
                predict_linear(prometheus_notifications_queue_length{%(prometheusSelector)s}[5m], 60 * 30)
              >
                prometheus_notifications_queue_capacity{%(prometheusSelector)s}
              )
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: "Prometheus's alert notification queue is running full for %(prometheusName)s" % $._config,
            },
          },
          {
            alert: 'PrometheusErrorSendingAlerts',
            expr: |||
              (
                rate(prometheus_notifications_errors_total{%(prometheusSelector)s}[5m])
              /
                rate(prometheus_notifications_sent_total{%(prometheusSelector)s}[5m]) > 1
              )
              * 100
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ printf "%%.1f" $value }}%% errors while sending alerts from Prometheus %(prometheusName)s to Alertmanager {{$labels.Alertmanager}}' % $._config,
            },
          },
          {
            alert: 'PrometheusErrorSendingAlerts',
            expr: |||
              (
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
              message: '{{ printf "%%.1f" $value }}%% errors while sending alerts from Prometheus %(prometheusName)s to Alertmanager {{$labels.Alertmanager}}' % $._config,
            },
          },
          {
            alert: 'PrometheusNotConnectedToAlertmanagers',
            expr: |||
              prometheus_notifications_alertmanagers_discovered{%(prometheusSelector)s} < 1
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Prometheus %(prometheusName)s is not connected to any Alertmanagers' % $._config,
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
              message: 'Prometheus %(prometheusName)s had {{$value | humanize}} reload failures over the last four hours.' % $._config,
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
              message: 'Prometheus %(prometheusName)s had {{$value | humanize}} compaction failures over the last four hours.' % $._config,
            },
          },
          {
            alert: 'PrometheusTSDBWALCorruptions',
            expr: |||
              increase(tsdb_wal_corruptions_total{%(prometheusSelector)s}[3h]) > 0
            ||| % $._config,
            'for': '4h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Prometheus %(prometheusName)s has a corrupted write-ahead log (WAL).' % $._config,
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
              message: "Prometheus %(prometheusName)s isn't ingesting samples." % $._config,
            },
          },
          {
            alert: 'PrometheusTargetScrapesDuplicate',
            expr: |||
              increase(prometheus_target_scrapes_sample_duplicate_timestamp_total{%(prometheusSelector)s}[5m]) > 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Prometheus %(prometheusName)s has many samples rejected due to duplicate timestamps but different values' % $._config,
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
              message: 'Prometheus %(prometheusName)s failed to send {{ printf "%%.1f" $value }}%% samples' % $._config,
            },
          },
          {
            alert: 'PrometheusRemoteWriteBehind',
            expr: |||
              (
                prometheus_remote_storage_highest_timestamp_in_seconds{%(prometheusSelector)s}
              - on(job, instance) group_right
                prometheus_remote_storage_queue_highest_sent_timestamp_seconds{%(prometheusSelector)s}
              )
              > 120
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Prometheus %(prometheusName)s remote write is {{ printf "%%.1f" $value }}s behind.' % $._config,
            },
          },
          {
            alert: 'PrometheusRuleFailures',
            expr: |||
              rate(prometheus_rule_evaluation_failures_total{%(prometheusSelector)s}[5m]) > 0
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Prometheus %(prometheusName)s failed to evaluate {{ printf "%%.1f" $value }} rules / s' % $._config,
            },
          },
        ],
      },
    ],
  },
}
