---
name: prometheus-config
description: >-
  Write and validate prometheus.yml: global settings, scrape_configs, static
  and service discovery targets, relabel_configs and metric_relabel_configs,
  rule_files, alerting, remote_write, agent mode, and checking it with
  promtool check config. Use when adding scrape targets, fixing relabeling,
  or reviewing a Prometheus configuration change.
---

# Configuring Prometheus

References: `docs/configuration/configuration.md`,
`docs/command-line/prometheus.md`, `docs/command-line/promtool.md`,
`docs/prometheus_agent.md`, `documentation/examples/`.

## Skeleton

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: prod

rule_files:
  - "rules/*.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ["alertmanager:9093"]

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["localhost:9090"]

  - job_name: kubernetes-pods
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: "true"
      - source_labels: [__meta_kubernetes_namespace]
        target_label: namespace
```

## Relabeling

- `relabel_configs` run **before** the scrape on target labels (including
  `__meta_*` from service discovery and `__address__`, `__metrics_path__`,
  `__scheme__`). Labels starting with `__` are dropped afterwards.
- `metric_relabel_configs` run **after** the scrape on every sample: use
  them to drop expensive series (`action: drop`) or labels (`labeldrop`).
- Actions: `replace` (default), `keep`, `drop`, `keepequal`, `dropequal`,
  `hashmod`, `labelmap`, `labeldrop`, `labelkeep`, `lowercase`,
  `uppercase`.
- Debug with the `/service-discovery` and `/targets` pages, which show
  labels before and after relabeling.

## Validate before reload

```bash
promtool check config prometheus.yml
promtool check config --syntax-only prometheus.yml
promtool check config --lint=all --lint-fatal prometheus.yml
promtool check config --agent prometheus.yml           # agent mode
promtool check web-config web.yml
```

Reload a running server with `SIGHUP`, or `POST /-/reload` when started
with `--web.enable-lifecycle`. An invalid file is rejected and the old
config keeps running.

## Agent mode and remote write

```bash
prometheus --agent --config.file=agent.yml
```

Agent mode scrapes and forwards with `remote_write` only: no local
querying, rules or alerting.

```yaml
remote_write:
  - url: https://example.com/api/v1/write
    basic_auth:
      username: user
      password_file: /etc/prometheus/rw-password
```

Keep secrets in `*_file` fields, not inline.

## Review checklist

- Every `job_name` is unique.
- `scrape_timeout` <= `scrape_interval`.
- High-cardinality labels (user IDs, request IDs) are dropped in
  `metric_relabel_configs`.
- `promtool check config` passes.
