# Prometheus Mixin

_This is work in progress. We aim for it to become a good role model for alerts
and dashboards eventually, but it is not quite there yet._

The Prometheus Mixin is a set of configurable, reusable, and extensible alerts
and dashboards for Prometheus.

To use them, you need to have `jsonnet` (v0.13+) and `jb` installed. If you
have a working Go development environment, it's easiest to run the following:
```bash
$ go install github.com/google/go-jsonnet/cmd/jsonnet@latest
$ go install github.com/google/go-jsonnet/cmd/jsonnetfmt@latest
$ go install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@latest
```

_Note: The make targets `lint` and `fmt` need the `jsonnetfmt` binary, which is
available from [v.0.16.0](https://github.com/google/jsonnet/releases/tag/v0.16.0) in the Go implementation of `jsonnet`. If your jsonnet version is older than 0.16.0 you have to either upgrade or install the [C++ version of
jsonnetfmt](https://github.com/google/jsonnet) if you want to use `make lint`
or `make fmt`._

Next, install the dependencies by running the following command in this
directory:
```bash
$ jb install
```

You can then build a `prometheus_alerts.yaml` with the alerts and a directory
`dashboards_out` with the Grafana dashboard JSON files:
```bash
$ make prometheus_alerts.yaml
$ make dashboards_out
```

For more advanced uses of mixins, see https://github.com/monitoring-mixins/docs.

