{
  _config+:: {
    // Selectors are inserted between {} in Prometheus queries.
    prometheusSelector: 'job="prometheus"',
    alertmanagerSelector: 'job="alertmanager"',

    // prometheusName is inserted into annotations to name the Prometheus
    // instance affected by the alert.
    prometheusName: '{{$labels.instance}}',
    // If you run Prometheus on Kubernetes with the Prometheus
    // Operator, you can make use of the configured target labels for
    // nicer naming:
    // prometheusNameTemplate: '{{$labels.namespace}}/{{$labels.pod}}'
  },
}
