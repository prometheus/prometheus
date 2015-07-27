# Prometheus [![Build Status](https://travis-ci.org/prometheus/prometheus.svg)](https://travis-ci.org/prometheus/prometheus) [![Circle CI](https://circleci.com/gh/prometheus/prometheus/tree/master.svg?style=svg)](https://circleci.com/gh/prometheus/prometheus/tree/master)

Prometheus is a systems and service monitoring system. It collects metrics
from configured targets at given intervals, evaluates rule expressions,
displays the results, and can trigger alerts if some condition is observed
to be true.

Prometheus' main distinguishing features as compared to other monitoring systems are:

- a **multi-dimensional** data model (timeseries defined by metric name and set of key/value dimensions)
- a **flexible query language** to leverage this dimensionality
- no dependency on distributed storage; **single server nodes are autonomous**
- timeseries collection happens via a **pull model** over HTTP
- **pushing timeseries** is supported via an intermediary gateway
- targets are discovered via **service discovery** or **static configuration**
- multiple modes of **graphing and dashboarding support**
- **federation support** coming soon

## Architecture overview

![](https://cdn.rawgit.com/prometheus/prometheus/62b69b0/documentation/images/architecture.svg)

## Install

There are various ways of installing Prometheus.

### Precompiled binaries

Precompiled binaries for released versions are available in the
[*releases* section](https://github.com/prometheus/prometheus/releases)
of the GitHub repository. Using the latest production release binary
is the recommended way of installing Prometheus.

Debian and RPM packages are being worked on.

### Use `make`

Clone the repository in the usual way with `git clone`. (If you
download and unpack the source archives provided by GitHub instead of
using `git clone`, you need to set an environment variable `VERSION`
to make the below work. See
[issue #609](https://github.com/prometheus/prometheus/issues/609) for
context.)

In most circumstances, the following should work:

    $ make build
    $ ./prometheus -config.file=documentation/examples/prometheus.yml

The above requires a number of common tools to be installed, namely
`curl`, `git`, `gzip`, `hg` (Mercurial CLI).

Everything else will be downloaded and installed into a staging
environment in the `.build` sub-directory. That includes a Go
development environment of the appropriate version.

The `Makefile` offers a number of useful targets. Some examples:

* `make test` runs tests.
* `make tarball` creates a tarball with the binary for distribution.
* `make race_condition_run` compiles and runs a binary with the race detector enabled. To pass arguments when running Prometheus this way, set the `ARGUMENTS` environment variable (e.g. `ARGUMENTS="-config.file=./prometheus.conf" make race_condition_run`).

### Use your own Go development environment

Using your own Go development environment with the usual tooling is
possible, too. After making changes to the files in `web/static` you
have to run `make` in the `web/` directory. This generates the respective
`web/blob/files.go` file which embedds the static assets in the compiled binary.

Furthermore, the version information (see `version/info.go`) will not be
populated if you simply run `go build`. You have to pass in command
line flags as defined in `Makefile.INCLUDE` (see `${BUILDFLAGS}`) to
do that.

## Release checklist

To cut a new release of Prometheus, follow this checklist:

1. Consider updating all or some of the vendored dependencies before the
   release. Make sure that https://github.com/prometheus/client_golang is
   vendored at a release version.
1. Create a PR that updates the `CHANGELOG.md` along with the `version/VERSION` file and get it merged.
1. Run `make tag` to tag the new release and push it to GitHub.
1. Head to https://github.com/prometheus/prometheus/releases and create a new release for the pushed tag.
1. Attach the relevant changelog excerpt to the release on GitHub.
1. Build `linux-amd64`, `linux-386`, and `darwin-amd64` binaries via `make tarball` and attach them to the release.
1. Update the `stable` branch to point to the new release (to tag the latest release image on https://registry.hub.docker.com/u/prom/prometheus/):

   ```bash
   git branch -f stable <new-release-tag>
   git push origin stable
   ```

1. Announce the release on the mailing list and on Twitter.

## More information

  * The source code is periodically indexed: [Prometheus Core](http://godoc.org/github.com/prometheus/prometheus).
  * You will find a Travis CI configuration in `.travis.yml`.
  * All of the core developers are accessible via the [Prometheus Developers Mailinglist](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers) and the `#prometheus` channel on `irc.freenode.net`.

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Apache License 2.0, see [LICENSE](LICENSE).
