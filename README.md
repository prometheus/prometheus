# Prometheus

[![CircleCI](https://circleci.com/gh/prometheus/prometheus/tree/main.svg?style=shield)][circleci]
[![Docker Repository on Quay](https://quay.io/repository/prometheus/prometheus/status)][quay]
[![Docker Pulls](https://img.shields.io/docker/pulls/prom/prometheus.svg?maxAge=604800)][hub]
[![Go Report Card](https://goreportcard.com/badge/github.com/prometheus/prometheus)](https://goreportcard.com/report/github.com/prometheus/prometheus)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/486/badge)](https://bestpractices.coreinfrastructure.org/projects/486)
[![Gitpod ready-to-code](https://img.shields.io/badge/Gitpod-ready--to--code-blue?logo=gitpod)](https://gitpod.io/#https://github.com/prometheus/prometheus)
[![Fuzzing Status](https://oss-fuzz-build-logs.storage.googleapis.com/badges/prometheus.svg)](https://bugs.chromium.org/p/oss-fuzz/issues/list?sort=-opened&can=1&q=proj:prometheus)

Visit [prometheus.io](https://prometheus.io) for the full documentation,
examples and guides.

Prometheus, a [Cloud Native Computing Foundation](https://cncf.io/) project, is a systems and service monitoring system. It collects metrics
from configured targets at given intervals, evaluates rule expressions,
displays the results, and can trigger alerts when specified conditions are observed.

The features that distinguish Prometheus from other metrics and monitoring systems are:

- A **multi-dimensional** data model (time series defined by metric name and set of key/value dimensions)
- PromQL, a **powerful and flexible query language** to leverage this dimensionality
- No dependency on distributed storage; **single server nodes are autonomous**
- An HTTP **pull model** for time series collection
- **Pushing time series** is supported via an intermediary gateway for batch jobs
- Targets are discovered via **service discovery** or **static configuration**
- Multiple modes of **graphing and dashboarding support**
- Support for hierarchical and horizontal **federation**

## Architecture overview

![](https://cdn.jsdelivr.net/gh/prometheus/prometheus@c34257d069c630685da35bcef084632ffd5d6209/documentation/images/architecture.svg)

## Install

There are various ways of installing Prometheus.

### Precompiled binaries

Precompiled binaries for released versions are available in the
[*download* section](https://prometheus.io/download/)
on [prometheus.io](https://prometheus.io). Using the latest production release binary
is the recommended way of installing Prometheus.
See the [Installing](https://prometheus.io/docs/introduction/install/)
chapter in the documentation for all the details.

### Docker images

Docker images are available on [Quay.io](https://quay.io/repository/prometheus/prometheus) or [Docker Hub](https://hub.docker.com/r/prom/prometheus/).

You can launch a Prometheus container for trying it out with

    $ docker run --name prometheus -d -p 127.0.0.1:9090:9090 prom/prometheus

Prometheus will now be reachable at http://localhost:9090/.

### Building from source

To build Prometheus from source code, You need:
* Go [version 1.16 or greater](https://golang.org/doc/install).
* NodeJS [version 16 or greater](https://nodejs.org/).
* npm [version 7 or greater](https://www.npmjs.com/).

You can directly use the `go` tool to download and install the `prometheus`
and `promtool` binaries into your `GOPATH`:

    $ GO111MODULE=on go install github.com/prometheus/prometheus/cmd/...
    $ prometheus --config.file=your_config.yml

*However*, when using `go install` to build Prometheus, Prometheus will expect to be able to
read its web assets from local filesystem directories under `web/ui/static` and
`web/ui/templates`. In order for these assets to be found, you will have to run Prometheus
from the root of the cloned repository. Note also that these directories do not include the
new experimental React UI unless it has been built explicitly using `make assets` or `make build`.

An example of the above configuration file can be found [here.](https://github.com/prometheus/prometheus/blob/main/documentation/examples/prometheus.yml)

You can also clone the repository yourself and build using `make build`, which will compile in
the web assets so that Prometheus can be run from anywhere:

    $ mkdir -p $GOPATH/src/github.com/prometheus
    $ cd $GOPATH/src/github.com/prometheus
    $ git clone https://github.com/prometheus/prometheus.git
    $ cd prometheus
    $ make build
    $ ./prometheus --config.file=your_config.yml

The Makefile provides several targets:

  * *build*: build the `prometheus` and `promtool` binaries (includes building and compiling in web assets)
  * *test*: run the tests
  * *test-short*: run the short tests
  * *format*: format the source code
  * *vet*: check the source code for common errors
  * *assets*: build the new experimental React UI

### Service discovery plugins

Prometheus is bundled with many service discovery plugins.
When building Prometheus from source, you can edit the [plugins.yml](./plugins.yml)
file to disable some service discoveries. The file is a yaml-formated list of go
import path that will be built into the Prometheus binary.

After you have changed the file, you
need to run `make build` again.

If you are using another method to compile Prometheus, `make plugins` will
generate the plugins file accordingly.

If you add out-of-tree plugins, which we do not endorse at the moment,
additional steps might be needed to adjust the `go.mod` and `go.sum` files. As
always, be extra careful when loading third party code.

### Building the Docker image

The `make docker` target is designed for use in our CI system.
You can build a docker image locally with the following commands:

    $ make promu
    $ promu crossbuild -p linux/amd64
    $ make npm_licenses
    $ make common-docker-amd64

*NB* if you are on a Mac, you will need [gnu-tar](https://formulae.brew.sh/formula/gnu-tar).

## React UI Development

For more information on building, running, and developing on the new React-based UI, see the React app's [README.md](web/ui/README.md).

## More information

  * The source code is periodically indexed, but due to an issue with versioning, the "latest" docs shown on Godoc are outdated. Instead, you can use [the docs for v2.31.1](https://pkg.go.dev/github.com/prometheus/prometheus@v1.8.2-0.20211105201321-411021ada9ab).
  * You will find a CircleCI configuration in [`.circleci/config.yml`](.circleci/config.yml).
  * See the [Community page](https://prometheus.io/community) for how to reach the Prometheus developers and users on various communication channels.

## Contributing

Refer to [CONTRIBUTING.md](https://github.com/prometheus/prometheus/blob/main/CONTRIBUTING.md)

## License

Apache License 2.0, see [LICENSE](https://github.com/prometheus/prometheus/blob/main/LICENSE).


[hub]: https://hub.docker.com/r/prom/prometheus/
[circleci]: https://circleci.com/gh/prometheus/prometheus
[quay]: https://quay.io/repository/prometheus/prometheus
