---
title: Federation
sort_rank: 6
---

Federation allows a Prometheus server to scrape selected time series from
another Prometheus server.

_Note about native histograms: To scrape native histograms via federation, the
scraping Prometheus server needs to set `scrape_native_histograms: true` in its
scrape config, implying that the protobuf format is used for scraping. Should
the federated metrics contain a mix of different sample types (floats of any
flavor, counter histograms, or gauge histograms) for the same metric name, the
federation payload will contain multiple metric families with the same name
(but different types – for technical reasons, float samples are always federated
as `untyped`, while histogram samples are federated with their full type
information). Technically, this violates the rules of the protobuf exposition
format, but Prometheus is nevertheless able to ingest all metrics correctly._

## Use cases

There are different use cases for federation. Commonly, it is used to either
achieve scalable Prometheus monitoring setups or to pull related metrics from
one service's Prometheus into another.

### Hierarchical federation

Hierarchical federation allows Prometheus to scale to environments with tens of
data centers and millions of nodes. In this use case, the federation topology
resembles a tree, with higher-level Prometheus servers collecting aggregated
time series data from a larger number of subordinated servers.

For example, a setup might consist of many per-datacenter Prometheus servers
that collect data in high detail (instance-level drill-down), and a set of
global Prometheus servers which collect and store only aggregated data
(job-level drill-down) from those local servers. This provides an aggregate
global view and detailed local views.

### Cross-service federation

In cross-service federation, a Prometheus server of one service is configured
to scrape selected data from another service's Prometheus server to enable
alerting and queries against both datasets within a single server.

For example, a cluster scheduler running multiple services might expose
resource usage information (like memory and CPU usage) about service instances
running on the cluster. On the other hand, a service running on that cluster
will only expose application-specific service metrics. Often, these two sets of
metrics are scraped by separate Prometheus servers. Using federation, the
Prometheus server containing service-level metrics may pull in the cluster
resource usage metrics about its specific service from the cluster Prometheus,
so that both sets of metrics can be used within that server.

## Configuring federation

On any given Prometheus server, the `/federate` endpoint allows retrieving the
current value for a selected set of time series in that server. At least one
`match[]` URL parameter must be specified to select the series to expose. Each
`match[]` argument needs to specify an
[instant vector selector](querying/basics.md#instant-vector-selectors) like
`up` or `{job="api-server"}`. If multiple `match[]` parameters are provided,
the union of all matched series is selected.

To federate metrics from one server to another, configure your destination
Prometheus server to scrape from the `/federate` endpoint of a source server,
while also enabling the `honor_labels` scrape option (to not overwrite any
labels exposed by the source server) and passing in the desired `match[]`
parameters. For example, the following `scrape_configs` federates any series
with the label `job="prometheus"` or a metric name starting with `job:` from
the Prometheus servers at `source-prometheus-{1,2,3}:9090` into the scraping
Prometheus:

```yaml
scrape_configs:
  - job_name: 'federate'
    scrape_interval: 15s

    honor_labels: true
    metrics_path: '/federate'

    params:
      'match[]':
        - '{job="prometheus"}'
        - '{__name__=~"job:.*"}'

    static_configs:
      - targets:
        - 'source-prometheus-1:9090'
        - 'source-prometheus-2:9090'
        - 'source-prometheus-3:9090'
```
