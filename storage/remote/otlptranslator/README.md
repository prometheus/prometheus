## Copying from opentelemetry/opentelemetry-collector-contrib

This files in the `prometheus/` and `prometheusremotewrite/` are copied from the OpenTelemetry Project[^1].

This is done instead of adding a go.mod dependency because OpenTelemetry depends on `prometheus/prometheus` and a cyclic dependency will be created. This is just a temporary solution and the long-term solution is to move the required packages from OpenTelemetry into `prometheus/prometheus`. 
We don't copy in `./prometheus` through this script because that package imports a collector specific featuregate package we don't want to import. The featuregate package is being removed now, and in the future we will copy this folder too.

To update the dependency is a multi-step process:
1. Vendor the latest `prometheus/prometheus`@`main` into [`opentelemetry/opentelemetry-collector-contrib`](https://github.com/open-telemetry/opentelemetry-collector-contrib)
1. Update the VERSION in `update-copy.sh`.
1. Run `./update-copy.sh`.

### Why copy?

This is because the packages we copy depend on the [`prompb`](https://github.com/prometheus/prometheus/blob/main/prompb) package. While the package is relatively stable, there are still changes. For example, https://github.com/prometheus/prometheus/pull/11935 changed the types.
This means if we depend on the upstream packages directly, we will never able to make the changes like above. Hence we're copying the code for now.

### I need to manually change these files

When we do want to make changes to the types in `prompb`, we might need to edit the files directly. That is OK, please let @gouthamve or @jesusvazquez know so they can take care of updating the upstream code (by vendoring in `prometheus/prometheus` upstream and resolving conflicts) and then will run the copy
script again to keep things updated.

[^1]: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheus and https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheusremotewrite