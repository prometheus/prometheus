# Prometheus

Bedecke deinen Himmel, Zeus!  A new kid is in town.

Prometheus is a generic time series collection and computation server that is
useful in the following fields:

* Industrial Experimentation / Real-Time Behavioral Validation / Software Release Qualification
* Econometric and Natural Sciences
* Operational Concerns and Monitoring

The system is designed to collect telemetry from named targets on given
intervals, evaluate rule expressions, display the results, and trigger an
action if some condition is observed to be true.

TODO: The above description is somewhat esoteric. Rephrase it into
somethith that tells normal people how they will usually benefit from
using Prometheus.

## Install

There are various ways of installing Prometheus.

### Precompiled packages

We plan to provide precompiled binaries for various platforms and even
packages for common Linux distribution soon. Once those are offered,
it will be the recommended way of installing Prometheus.

### Use `make`

In most cirumstances, the following should work:

    $ make
    $ ARGUMENTS="-config.file=documentation/examples/prometheus.conf" make run

``${ARGUMENTS}`` is passed verbatim to the commandline starting the Prometheus binary.
This is useful for quick one-off invocations and smoke testing.

The above requires a number of common tools to be installed, namely
`curl`, `git`, `gzip`, `hg` (Mercurial CLI), `sed`, `xxd`. Should you
need to change any of the protocol buffer definition files
(`*.proto`), you also need the protocol buffer compiler
[`protoc`](http://code.google.com/p/protobuf/](http://code.google.com/p/protobuf/),
v2.5.0 or higher, in your `$PATH`.

Everything else will be downloaded and installed into a staging
environment in the `.build` sub-directory. That includes a Go
development environment of the appropriate version.

The `Makefile` offers a number of useful targets. Some examples:

* `make test` runs tests.
* `make tarball` creates a tar ball with the binary for distribution.
* `make race_condition_run` compiles and runs a binary with the race detector enabled.

### Use your own Go development environment

Using your own Go development environment with the usual tooling is
possible, too, but you have to take care of various generated files
(usually by running `make` in the respective sub-directory):

* Compiling the protocol buffer definitions in `config` (only if you have changed them).
* Generating the parser and lexer code in `rules` (only if you have changed `parser.y` or `lexer.l`).
* The `files.go` blob in `web/blob`, which embeds the static web content into the binary.

Furthermore, the build info (see `build_info.go`) will not be
populated if you simply run `go build`. You have to pass in command
line flags as defined in `Makefile.INCLUDE` (see `${BUILDFLAGS}`) to
do that.

## More information

  * The source code is periodically indexed: [Prometheus Core](http://godoc.org/github.com/prometheus/prometheus).
  * You will find a Travis CI configuration in `.travis.yml`.
  * All of the core developers are accessible via the [Prometheus Developers Mailinglist](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers).

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Apache License 2.0, see [LICENSE](LICENSE).
