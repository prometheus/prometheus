# Prometheus Go client library

[![Build Status](https://travis-ci.org/prometheus/client_golang.svg?branch=master)](https://travis-ci.org/prometheus/client_golang)
[![Go Report Card](https://goreportcard.com/badge/github.com/prometheus/client_golang)](https://goreportcard.com/report/github.com/prometheus/client_golang)
[![go-doc](https://godoc.org/github.com/prometheus/client_golang?status.svg)](https://godoc.org/github.com/prometheus/client_golang)

This is the [Go](http://golang.org) client library for
[Prometheus](http://prometheus.io). It has two separate parts, one for
instrumenting application code, and one for creating clients that talk to the
Prometheus HTTP API.

__This library requires Go1.7 or later.__

## Important note about releases, versioning, tagging, stability, and your favorite Go dependency management tool

While our goal is to follow [Semantic Versioning](https://semver.org/), this
repository is still pre-1.0.0. To quote the
[Semantic Versioning spec](https://semver.org/#spec-item-4): “Anything may
change at any time. The public API should not be considered stable.” We know
that this is at odds with the widespread use of this library. However, just
declaring something 1.0.0 doesn't make it 1.0.0. Instead, we are working
towards a 1.0.0 release that actually deserves its major version number.

Having said that, we aim for always keeping the tip of master in a workable
state and for only introducing ”mildly” breaking changes up to and including
[v0.9.0](https://github.com/prometheus/client_golang/milestone/1). After that,
a number of ”hard” breaking changes are planned, see the
[v0.10.0 milestone](https://github.com/prometheus/client_golang/milestone/2),
which should get the library much closer to 1.0.0 state.

Dependency management in Go projects is still in flux, and there are many tools
floating around. While [dep](https://golang.github.io/dep/) might develop into
the de-facto standard tool, it is still officially experimental. The roadmap
for this library has been laid out with a lot of sometimes painful experience
in mind. We really cannot adjust it every other month to the needs of the
currently most popular or most promising Go dependency management tool. The
recommended course of action with dependency management tools is the following:

- Do not expect strict post-1.0.0 semver semantics prior to the 1.0.0
  release. If your dependency management tool expects strict post-1.0.0 semver
  semantics, you have to wait. Sorry.
- If you want absolute certainty, please lock to a specific commit. You can
  also lock to tags, but please don't ask for more tagging. This would suggest
  some release or stability testing procedure that simply is not in place. As
  said above, we are aiming for stability of the tip of master, but if we
  tagged every single commit, locking to tags would be the same as locking to
  commits.
- If you want to get the newer features and improvements and are willing to
  take the minor risk of newly introduced bugs and “mild” breakage, just always
  update to the tip of master (which is essentially the original idea of Go
  dependency management). We recommend to not use features marked as
  _deprecated_ in this case.
- Once [v0.9.0](https://github.com/prometheus/client_golang/milestone/1) is
  out, you could lock to v0.9.x to get bugfixes (and perhaps minor new
  features) while avoiding the “hard” breakage that will come with post-0.9
  features.

## Instrumenting applications

[![code-coverage](http://gocover.io/_badge/github.com/prometheus/client_golang/prometheus)](http://gocover.io/github.com/prometheus/client_golang/prometheus) [![go-doc](https://godoc.org/github.com/prometheus/client_golang/prometheus?status.svg)](https://godoc.org/github.com/prometheus/client_golang/prometheus)

The
[`prometheus` directory](https://github.com/prometheus/client_golang/tree/master/prometheus)
contains the instrumentation library. See the
[best practices section](http://prometheus.io/docs/practices/naming/) of the
Prometheus documentation to learn more about instrumenting applications.

The
[`examples` directory](https://github.com/prometheus/client_golang/tree/master/examples)
contains simple examples of instrumented code.

## Client for the Prometheus HTTP API

[![code-coverage](http://gocover.io/_badge/github.com/prometheus/client_golang/api/prometheus/v1)](http://gocover.io/github.com/prometheus/client_golang/api/prometheus/v1) [![go-doc](https://godoc.org/github.com/prometheus/client_golang/api/prometheus?status.svg)](https://godoc.org/github.com/prometheus/client_golang/api)

The
[`api/prometheus` directory](https://github.com/prometheus/client_golang/tree/master/api/prometheus)
contains the client for the
[Prometheus HTTP API](http://prometheus.io/docs/querying/api/). It allows you
to write Go applications that query time series data from a Prometheus
server. It is still in alpha stage.

## Where is `model`, `extraction`, and `text`?

The `model` packages has been moved to
[`prometheus/common/model`](https://github.com/prometheus/common/tree/master/model).

The `extraction` and `text` packages are now contained in
[`prometheus/common/expfmt`](https://github.com/prometheus/common/tree/master/expfmt).

## Contributing and community

See the [contributing guidelines](CONTRIBUTING.md) and the
[Community section](http://prometheus.io/community/) of the homepage.
