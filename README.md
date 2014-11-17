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

## Prerequisites

In your `PATH`, you must have the following binaries available:
- `curl`
- `xxd`
- `sed`
- `gzip`

If you change any of the `*.proto` files, you need to install [`protoc`, the protobuf compiler](http://code.google.com/p/protobuf/](http://code.google.com/p/protobuf/), v2.5.0 or higher.

TODO: lexer, golex

## Getting Started

For basic help how to get started:

  * The source code is periodically indexed: [Prometheus Core](http://godoc.org/github.com/prometheus/prometheus).
  * For UNIX-like environment users, please consult the Travis CI configuration in _.travis.yml_ and _Makefile_.
  * All of the core developers are accessible via the [Prometheus Developers Mailinglist](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers).

### General

For first time users, simply run the following:

    $ make
    $ ARGUMENTS="-configFile=documentation/examples/prometheus.conf" make run

``${ARGUMENTS}`` is passed verbatim into the makefile and thusly Prometheus as
``$(ARGUMENTS)``.  This is useful for quick one-off invocations and smoke
testing.

If you run into problems, try the following:

    $ SILENCE_THIRD_PARTY_BUILDS=false make

Upon having a satisfactory build, it's possible to create an artifact for
end-user distribution:

    $ make package
    $ find build/package

``build/package`` will be sufficient for whatever archiving mechanism you
choose.  The important thing to note is that Go presently does not
staticly link against C dependency libraries, so including the ``lib``
directory is paramount.  Providing ``LD_LIBRARY_PATH`` or
``DYLD_LIBRARY_PATH`` in a scaffolding shell script is advised.


### Problems
If at any point you run into an error with the ``make`` build system in terms of
its not properly scaffolding things on a given environment, please file a bug or
open a pull request with your changes if you can fix it yourself.

Please note that we're explicitly shooting for stable runtime environments and
not the latest-whiz bang releases; thusly, we ask you to provide ample
architecture and release identification remarks for us.

## Testing

    $ make test

## Packaging

    $ make package

### Race Detector

Go 1.1 includes a [race detector](http://tip.golang.org/doc/articles/race_detector.html)
which can be enabled at build time. Here's how to use it with Prometheus
(assumes that you've already run a successful build).

To run the tests with race detection:

    $ GORACE="log_path=/tmp/foo" go test -race ./...

To run the server with race detection:

    $ go build -race .
    $ GORACE="log_path=/tmp/foo" ./prometheus

[![Build Status](https://travis-ci.org/prometheus/prometheus.png)](https://travis-ci.org/prometheus/prometheus)

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Apache License 2.0
