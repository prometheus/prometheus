OpenCensus Proto - Language Independent Interface Types For OpenCensus
===============================================================
[![Gitter chat][gitter-image]][gitter-url]
[![Build Status][travis-image]][travis-url]
[![Maven Central][maven-image]][maven-url]
[![Javadocs][javadoc-image]][javadoc-url]
[![GoDoc][godoc-image]][godoc-url]
[![PyPI][pypi-image]][pypi-url]

Census provides a framework to define and collect stats against metrics and to
break those stats down across user-defined dimensions.

The Census framework is natively available in many languages (e.g. C++, Go,
and Java). The API interface types are defined using protos to ensure
consistency and interoperability for the different implementations.

## Quickstart

### Install to Go

```bash
$ go get -u github.com/census-instrumentation/opencensus-proto
```

In most cases you should depend on the gen-go files directly. If you are
building with Bazel, there are also go_proto_library build rules available.
See [PR/132](https://github.com/census-instrumentation/opencensus-proto/pull/132)
for details. However, please note that Bazel doesn't generate the final
artifacts.

### Add the dependencies to your Java project

For Maven add to `pom.xml`:
```xml
<dependency>
  <groupId>io.opencensus</groupId>
  <artifactId>opencensus-proto</artifactId>
  <version>0.2.0</version>
</dependency>
```

For Gradle add to dependencies:
```gradle
compile 'io.opencensus:opencensus-proto:0.2.0'
```

[travis-image]: https://travis-ci.org/census-instrumentation/opencensus-proto.svg?branch=master
[travis-url]: https://travis-ci.org/census-instrumentation/opencensus-proto
[maven-image]: https://maven-badges.herokuapp.com/maven-central/io.opencensus/opencensus-proto/badge.svg
[maven-url]: https://maven-badges.herokuapp.com/maven-central/io.opencensus/opencensus-proto
[gitter-image]: https://badges.gitter.im/census-instrumentation/lobby.svg
[gitter-url]: https://gitter.im/census-instrumentation/lobby
[javadoc-image]: https://www.javadoc.io/badge/io.opencensus/opencensus-proto.svg
[javadoc-url]: https://www.javadoc.io/doc/io.opencensus/opencensus-proto
[godoc-image]: https://godoc.org/github.com/census-instrumentation/opencensus-proto?status.svg
[godoc-url]: https://godoc.org/github.com/census-instrumentation/opencensus-proto
[pypi-image]: https://badge.fury.io/py/opencensus-proto.svg
[pypi-url]: https://pypi.org/project/opencensus-proto/

### Add the dependencies to Bazel project

In WORKSPACE, add:
```
git_repository(
    name = "io_opencensus_proto",
    strip_prefix = "src",
    tag = "v0.2.0", # CURRENT_OPENCENSUS_PROTO_VERSION
    remote = "https://github.com/census-instrumentation/opencensus-proto",
)
```
or

```
http_archive(
    name = "io_opencensus_proto",
    strip_prefix = "opencensus-proto-master/src",
    urls = ["https://github.com/census-instrumentation/opencensus-proto/archive/master.zip"],
)
```

In BUILD.bazel:
```bazel
proto_library(
    name = "foo_proto",
    srcs = ["foo.proto"],
    deps = [
      "@io_opencensus_proto//opencensus/proto/metrics/v1:metrics_proto",
      "@io_opencensus_proto//opencensus/proto/trace/v1:trace_proto",
      # etc.
    ],
)
```
