This files in the `prometheus/` and `prometheusremotewrite/` are copied from the OpenTelemetry Project[^1].

This is done instead of adding a go.mod dependency because OpenTelemetry depends on `prometheus/prometheus` and a cyclic dependency will be created. This is just a temporary solution and the long-term solution is to move the required packages from OpenTelemetry into `prometheus/prometheus`. But currently the APIs in the packages are regularly changing and it would be a hassle to have the translation packages out of the opentelemetry-collector-contrib repo.

To update the dependency is a multi-step process:
1. Vendor the latest `main` into [`opentelemetry/opentelemetry-collector-contrib`](https://github.com/open-telemetry/opentelemetry-collector-contrib)
1. Update the VERSION in `update-copy.sh`.
1. Run `./update-copy.sh`.


[^1]: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheus and https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheusremotewrite