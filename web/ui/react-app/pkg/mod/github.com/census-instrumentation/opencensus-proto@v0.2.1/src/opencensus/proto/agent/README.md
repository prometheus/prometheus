# OpenCensus Agent Proto

This package describes the OpenCensus Agent protocol. For the architecture and implementation of
OpenCensus Agent, please refer to the specs in
[OpenCensus-Service](https://github.com/census-instrumentation/opencensus-service#opencensus-agent).

## Packages

1. `common` package contains the common messages shared between different services, such as
`Node`, `Service` and `Library` identifiers.
2. `trace` package contains the Trace Service protos.
3. `metrics` package contains the Metrics Service protos.
4. (Coming soon) `stats` package contains the Stats Service protos.
