# Prometheus 2.0 Migration Guide.

In line with our [compatibility promise](TODO), the Prometheus 2.0 release contains
a number of backward incompatible changes.  This document aims to offer guidance on
migrating from Prometheus 1.8 to Prometheus 2.0.

## Flags

The format of the Prometheus command line flags have changed.  Instead of a
single dash, all flags need a double dash. Common flags (`--config.file`,
`--web.listen-address` and `--web.external-url`) are still the same but beyond
that, almost all the storage-related flags have been removed.

Some notable flags which have been removed:
- `-alertmanager.url` In Prometheus 2.0, the command line flags for configuring
  a static Alertmanager URL have been removed.  Alertmanager can only be
  discovered via service discovery, see [Alertmanager service discovery](#amsd).

- `-log.format` In Prometheus 2.0 logs can only be streams to standard out.

- `-query.staleness-delta` Prometheus 2.0 introduces a new mechanism for
  handling stalesness, see [Stalesness](#stalesness).

- `-storage.local.*` Prometheus 2.0 introduces a new storage engine, as such all
  flags relating to the old engine have been removed.  For information on the
  new enginer, see [Storage](#storage).

- `-storage.remote.*` Prometheus 2.0 has removed the deprecated remote storage
  flags.  To write to InfluxDB, Graphite, or OpenTSDB use the storage adapter.

## Config File

## Alertmanager Service Discovery

Alertmanager service discovery was introduced in Prometheus 1.4, allowing Prometheus
to dynamically discover Alertmanager replicas using the same mechanism as monitoring
targets.  In Prometheus 2.0, the command line flags for static Alertmanager config
have been removed, so the following command line flag:

```
./prometheus -alertmanager.url=http://alertmanager/
```

Might looks like the following snippet in the `prometheus.yml` config file:

```yml
alerting:
  alertmanagers:
  - static_configs:
    - targets:
      - alertmanager
```

But note, you can use all the usual Prometheus service discovery integrations
and relabelling.  In this snippet I'm instructing Prometheus
to search for Kubernetes pods, in the `default` namespace, with the label
`name: alertmanager` and with a non-empty port.

```yml
alerting:
  alertmanagers:
  - kubernetes_sd_configs:
      - role: pod
    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    relabel_configs:
    - source_labels: [__meta_kubernetes_pod_label_name]
      regex: alertmanager
      action: keep
    - source_labels: [__meta_kubernetes_namespace]
      regex: default
      action: keep
    - source_labels: [__meta_kubernetes_pod_container_port_number]
      regex:
      action: drop
```

## Recording rules an alerts

The format for configuring alerting and recoding rules has been changed to YAML.
An example of an recording rule in the old format:

```
job:request_duration_seconds:99percentile =
  histogram_quantile(0.99, sum(rate(request_duration_seconds_bucket[1m])) by (le, job))

ALERT FrontendRequestLatency
  IF job:request_duration_seconds:99percentile{job="frontend"} > 0.1
  FOR 5m
  ANNOTATIONS {
    summary = "High frontend request latency",
  }
```

Might looks like this in the new format:

```yml
groups:
- name: example.rules
  rules:
  - record: job:request_duration_seconds:99percentile
    expr: histogram_quantile(0.99, sum(rate(request_duration_seconds_bucket[1m]))
      BY (le, job))
  - alert: FrontendRequestLatency
    expr: job:request_duration_seconds:99percentile{job="frontend"} > 0.1
    for: 5m
    annotations:
      summary: High frontend request latency
```

The `promtool` has a command to automate the conversion.  Given a `.rules` file,
it will output a `.rules.yml` file in the new format. For example:

```
$ promtool update rules example.rules
```

## Storage

The data format in Prometheus 2.0 has completely changed and is not backwards
compatible with 1.8. To retain access to your historic monitoring data we recommend
you run a non-scraping Prometheus 1.8 instance in parallel to you Prometheus 2.0.

Your Prometheus 1.8 instance should be started with the follow flags and an empty
config file (`empty.yml`):

```
$ ./prometheus-1.8.1.linux-amd64/prometheus -web.listen-address ":9094" -config.file empty.yml
```

Prometheus 2.0 can then be started (on the same machine) with the follow flags:

```
$ ./prometheus-2.0.0.linux-amd64/prometheus --config.file prometheus.yml
```

Where `prometheus.yml` contains the stanza:

```
remote_read:
  - url: "http://localhost:9094/api/v1/read"
```

TODO: external labels

## PromQL

Minimal changes have been made to PromQL.  

The follow functions have been removed:

- `drop_common_labels`

## Staleness

TODO

## Miscelaneous

### Prometheus non-root user

The Prometheus docker image is now built to [run Prometheus
as a non-root user](https://github.com/prometheus/prometheus/pull/2859).  If you
want the Prometheus UI/API to listen on a low port number (say, port 80), you'll
need to override it in your Kubernetes YAML:

```yml
apiVersion: v1
kind: Pod
metadata:
  name: security-context-demo-2
spec:
  securityContext:
    runAsUser: 0
...
```

Or the following Docker arguments:

```
TODO
```

See https://kubernetes.io/docs/tasks/configure-pod-container/security-context/ for
more details.

## Prometheus Lifecycle

If you use the Prometheus `/-/reload` HTTP endpoint to [automatically reload your
Prometheus config when it changes](https://www.weave.works/blog/prometheus-configmaps-continuous-deployment/),
you'll find these endpoints are disabled by default in Prometheus 2.0.  To enable
them, set the `--web.enable-lifecycle` flag.
