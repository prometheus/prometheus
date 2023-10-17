---
title: Migration
sort_rank: 10
---

# Prometheus 2.0 migration guide

In line with our [stability promise](https://prometheus.io/blog/2016/07/18/prometheus-1-0-released/#fine-print),
the Prometheus 2.0 release contains a number of backwards incompatible changes.
This document offers guidance on migrating from Prometheus 1.8 to Prometheus 2.0 and newer versions.

## Flags

The format of Prometheus command line flags has changed. Instead of a
single dash, all flags now use a double dash. Common flags (`--config.file`,
`--web.listen-address` and `--web.external-url`) remain but
almost all storage-related flags have been removed.

Some notable flags which have been removed:

- `-alertmanager.url` In Prometheus 2.0, the command line flags for configuring
  a static Alertmanager URL have been removed. Alertmanager must now be
  discovered via service discovery, see [Alertmanager service discovery](#alertmanager-service-discovery).

- `-log.format` In Prometheus 2.0 logs can only be streamed to standard error.

- `-query.staleness-delta` has been renamed to `--query.lookback-delta`; Prometheus
  2.0 introduces a new mechanism for handling staleness, see [staleness](querying/basics.md#staleness).

- `-storage.local.*` Prometheus 2.0 introduces a new storage engine; as such all
  flags relating to the old engine have been removed.  For information on the
  new engine, see [Storage](#storage).

- `-storage.remote.*` Prometheus 2.0 has removed the deprecated remote
  storage flags, and will fail to start if they are supplied. To write to
  InfluxDB, Graphite, or OpenTSDB use the relevant storage adapter.

## Alertmanager service discovery

Alertmanager service discovery was introduced in Prometheus 1.4, allowing Prometheus
to dynamically discover Alertmanager replicas using the same mechanism as scrape
targets. In Prometheus 2.0, the command line flags for static Alertmanager config
have been removed, so the following command line flag:

```
./prometheus -alertmanager.url=http://alertmanager:9093/
```

Would be replaced with the following in the `prometheus.yml` config file:

```yaml
alerting:
  alertmanagers:
  - static_configs:
    - targets:
      - alertmanager:9093
```

You can also use all the usual Prometheus service discovery integrations and
relabeling in your Alertmanager configuration. This snippet instructs
Prometheus to search for Kubernetes pods, in the `default` namespace, with the
label `name: alertmanager` and with a non-empty port.

```yaml
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

## Recording rules and alerts

The format for configuring alerting and recording rules has been changed to YAML.
An example of a recording rule and alert in the old format:

```
job:request_duration_seconds:histogram_quantile99 =
  histogram_quantile(0.99, sum by (le, job) (rate(request_duration_seconds_bucket[1m])))

ALERT FrontendRequestLatency
  IF job:request_duration_seconds:histogram_quantile99{job="frontend"} > 0.1
  FOR 5m
  ANNOTATIONS {
    summary = "High frontend request latency",
  }
```

Would look like this:

```yaml
groups:
- name: example.rules
  rules:
  - record: job:request_duration_seconds:histogram_quantile99
    expr: histogram_quantile(0.99, sum by (le, job) (rate(request_duration_seconds_bucket[1m])))
  - alert: FrontendRequestLatency
    expr: job:request_duration_seconds:histogram_quantile99{job="frontend"} > 0.1
    for: 5m
    annotations:
      summary: High frontend request latency
```

To help with the change, the `promtool` tool has a mode to automate the rules conversion.  Given a `.rules` file, it will output a `.rules.yml` file in the
new format. For example:

```
$ promtool update rules example.rules
```

You will need to use `promtool` from [Prometheus 2.5](https://github.com/prometheus/prometheus/releases/tag/v2.5.0) as later versions no longer contain the above subcommand.

## Storage

The data format in Prometheus 2.0 has completely changed and is not backwards
compatible with 1.8 and older versions. To retain access to your historic monitoring data we
recommend you run a non-scraping Prometheus instance running at least version
1.8.1 in parallel with your Prometheus 2.0 instance, and have the new server
read existing data from the old one via the remote read protocol.

Your Prometheus 1.8 instance should be started with the following flags and an
config file containing only the `external_labels` setting (if any):

```
$ ./prometheus-1.8.1.linux-amd64/prometheus -web.listen-address ":9094" -config.file old.yml
```

Prometheus 2.0 can then be started (on the same machine) with the following flags:

```
$ ./prometheus-2.0.0.linux-amd64/prometheus --config.file prometheus.yml
```

Where `prometheus.yml` contains in addition to your full existing configuration, the stanza:

```yaml
remote_read:
  - url: "http://localhost:9094/api/v1/read"
```

## PromQL

The following features have been removed from PromQL:

- `drop_common_labels` function - the `without` aggregation modifier should be used
  instead.
- `keep_common` aggregation modifier - the `by` modifier should be used instead.
- `count_scalar` function - use cases are better handled by `absent()` or correct
  propagation of labels in operations.

See [issue #3060](https://github.com/prometheus/prometheus/issues/3060) for more
details.

## Miscellaneous

### Prometheus non-root user

The Prometheus Docker image is now built to [run Prometheus
as a non-root user](https://github.com/prometheus/prometheus/pull/2859). If you
want the Prometheus UI/API to listen on a low port number (say, port 80), you'll
need to override it. For Kubernetes, you would use the following YAML:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: security-context-demo-2
spec:
  securityContext:
    runAsUser: 0
...
```

See [Configure a Security Context for a Pod or Container](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)
for more details.

If you're using Docker, then the following snippet would be used:

```
docker run -p 9090:9090 prom/prometheus:latest
```

### Prometheus lifecycle

If you use the Prometheus `/-/reload` HTTP endpoint to [automatically reload your
Prometheus config when it changes](configuration/configuration.md),
these endpoints are disabled by default for security reasons in Prometheus 2.0.
To enable them, set the `--web.enable-lifecycle` flag.
