{
  _config+:: {
    // Selectors are inserted between {} in Prometheus queries.
    prometheusSelector: 'job="prometheus"',
    alertmanagerSelector: 'job="alertmanager"',
  },
}
