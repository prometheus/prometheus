{
  _config+:: {
    // prometheusSelector is inserted as part of the label selector in
    // PromQL queries to identify metrics collected from Prometheus
    // servers.
    prometheusSelector: 'job="prometheus"',

    // prometheusHAGroupLabels is a string with comma-separated labels
    // that are common labels of instances belonging to the same
    // high-availability group of Prometheus servers, i.e. identically
    // configured Prometheus servers. Include not only enough labels
    // to identify the members of the HA group, but also all common
    // labels you want to keep for resulting HA-group-level alerts.
    //
    // If this is set to an empty string, no HA-related alerts are applied.
    prometheusHAGroupLabels: '',

    // prometheusName is inserted into annotations to name the Prometheus
    // instance affected by the alert.
    prometheusName: '{{$labels.instance}}',
    // If you run Prometheus on Kubernetes with the Prometheus
    // Operator, you can make use of the configured target labels for
    // nicer naming:
    // prometheusNameTemplate: '{{$labels.namespace}}/{{$labels.pod}}'

    // prometheusHAGroupName is inserted into annotations to name an
    // HA group. All labels used here must also be present in
    // prometheusHAGroupLabels above.
    prometheusHAGroupName: '{{$labels.job}}',

    // nonNotifyingAlertmanagerRegEx can be used to mark Alertmanager
    // instances that are not part of the Alertmanager cluster
    // delivering production notifications. This is important for the
    // PrometheusErrorSendingAlertsToAnyAlertmanager alert. Otherwise,
    // a still working test or auditing instance could mask a full
    // failure of all the production instances. The provided regular
    // expression is matched against the `alertmanager` label.
    // Example: @'http://test-alertmanager\..*'
    nonNotifyingAlertmanagerRegEx: @'',

    grafanaPrometheus: {
      prefix: 'Prometheus / ',
      tags: ['prometheus-mixin'],
      // The default refresh time for all dashboards, default to 60s
      refresh: '60s',
    },
  },
}
