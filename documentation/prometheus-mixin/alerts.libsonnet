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
              message: 'Prometheus failed to reload config, see container logs',
            },
          },
          {
            alert: 'PrometheusNotificationQueueRunningFull',
            expr: |||
              predict_linear(prometheus_notifications_queue_length{%(prometheusSelector)s}[5m], 60 * 30)
              >
              prometheus_notifications_queue_capacity{%(prometheusSelector)s}
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: "Prometheus' alert notification queue is running full for {{$labels.namespace}}/{{ $labels.pod}}",
            },
          },
          {
            alert: 'PrometheusErrorSendingAlerts',
            expr: |||
              100 * rate(prometheus_notifications_errors_total{%(prometheusSelector)s}[5m])
              /
              rate(prometheus_notifications_sent_total{%(prometheusSelector)s}[5m]) > 1
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ printf "%.1f" $value }}% errors while sending alerts from Prometheus {{$labels.namespace}}/{{ $labels.pod}} to Alertmanager {{$labels.Alertmanager}}',
            },
          },
          {
            alert: 'PrometheusErrorSendingAlerts',
            expr: |||
              100 * rate(prometheus_notifications_errors_total{%(prometheusSelector)s}[5m])
              /
              rate(prometheus_notifications_sent_total{%(prometheusSelector)s}[5m]) > 3
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: '{{ printf "%.1f" $value }}% errors while sending alerts from Prometheus {{$labels.namespace}}/{{ $labels.pod}} to Alertmanager {{$labels.Alertmanager}}',
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
              message: 'Prometheus {{ $labels.namespace }}/{{ $labels.pod}} is not connected to any Alertmanagers',
            },
          },
          {
            alert: 'PrometheusTSDBReloadsFailing',
            expr: |||
              increase(prometheus_tsdb_reloads_failures_total{%(prometheusSelector)s}[2h]) > 0
            ||| % $._config,
            'for': '12h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{$labels.job}} at {{$labels.instance}} had {{$value | humanize}} reload failures over the last four hours.',
            },
          },
          {
            alert: 'PrometheusTSDBCompactionsFailing',
            expr: |||
              increase(prometheus_tsdb_compactions_failed_total{%(prometheusSelector)s}[2h]) > 0
            ||| % $._config,
            'for': '12h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{$labels.job}} at {{$labels.instance}} had {{$value | humanize}} compaction failures over the last four hours.',
            },
          },
          {
            alert: 'PrometheusTSDBWALCorruptions',
            expr: |||
              tsdb_wal_corruptions_total{%(prometheusSelector)s} > 0
            ||| % $._config,
            'for': '4h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{$labels.job}} at {{$labels.instance}} has a corrupted write-ahead log (WAL).',
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
              message: "Prometheus {{ $labels.namespace }}/{{ $labels.pod}} isn't ingesting samples.",
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
              message: '{{$labels.namespace}}/{{$labels.pod}} has many samples rejected due to duplicate timestamps but different values',
            },
          },
          {
            alert: 'PrometheusRemoteStorageFailures',
            expr: |||
              (rate(prometheus_remote_storage_failed_samples_total{%(prometheusSelector)s}[1m]) * 100)
                /
              (rate(prometheus_remote_storage_failed_samples_total{%(prometheusSelector)s}[1m]) + rate(prometheus_remote_storage_succeeded_samples_total{%(prometheusSelector)s}[1m]))
                > 1
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Prometheus failed to send {{ printf "%.1f" $value }}% samples',
            },
          },
          {
            alert: 'PrometheusRemoteWriteBehind',
            expr: |||
              prometheus_remote_storage_highest_timestamp_in_seconds{%(prometheusSelector)s}
                - on(job, instance) group_right
              prometheus_remote_storage_queue_highest_sent_timestamp_seconds{%(prometheusSelector)s}
                > 120
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Prometheus remote write is {{ printf "%.1f" $value }}s behind.',
            },
          },
          {
            alert: 'PrometheusRuleFailures',
            'for': '15m',
            expr: |||
              rate(prometheus_rule_evaluation_failures_total{%(prometheusSelector)s}[1m]) > 0
            ||| % $._config,
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Prometheus failed to evaluate {{ printf "%.1f" $value }} rules / s',
            },
          },
        ],
      },
    ],
  },
}
