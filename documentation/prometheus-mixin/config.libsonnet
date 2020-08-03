{
  _config+:: {
    // prometheusSelector is inserted as part of the label selector in
    // PromQL queries to identify metrics collected from Prometheus
    // servers.
    prometheusSelector: 'job="prometheus"',

    // prometheusName is inserted into annotations to name the Prometheus
    // instance affected by the alert.
    prometheusName: '{{$labels.instance}}',
    // If you run Prometheus on Kubernetes with the Prometheus
    // Operator, you can make use of the configured target labels for
    // nicer naming:
    // prometheusNameTemplate: '{{$labels.namespace}}/{{$labels.pod}}'
  },
}
