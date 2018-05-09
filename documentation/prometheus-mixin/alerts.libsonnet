{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'prometheus',
        rules: [
          {
            alert: 'PromScrapeFailed',
            expr: |||
              up != 1
            |||,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Prometheus failed to scrape a target {{ $labels.job }} / {{ $labels.instance }}',
            },
          },
          {
            alert: 'PromBadConfig',
            expr: |||
              prometheus_config_last_reload_successful{%(prometheusSelector)s} == 0
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              mesage: 'Prometheus failed to reload config, see container logs',
            },
          },
          {
            alert: 'PromAlertmanagerBadConfig',
            expr: |||
              alertmanager_config_last_reload_successful{%(alertmanagerSelector)s} == 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Alertmanager failed to reload config, see container logs',
            },
          },
          {
            alert: 'PromAlertsFailed',
            expr: |||
              sum(increase(alertmanager_notifications_failed_total{%(alertmanagerSelector)s}[5m])) by (namespace) > 0
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Alertmanager failed to send an alert.',
            },
          },
          {
            alert: 'PromRemoteStorageFailures',
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
            alert: 'PromRuleFailures',
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
