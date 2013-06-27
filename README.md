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
If you read below in the _Getting Started_ section, the build infrastructure
will take care of the following things for you in most cases:

  1. Go 1.1.
  2. GVM: [https://github.com/moovweb/gvm](https://github.com/moovweb/gvm) is highly recommended as well.
  3. LevelDB: [https://code.google.com/p/leveldb/](https://code.google.com/p/leveldb/).
  4. Protocol Buffers Compiler: [http://code.google.com/p/protobuf/](http://code.google.com/p/protobuf/).
  5. goprotobuf: the code generator and runtime library: [http://code.google.com/p/goprotobuf/](http://code.google.com/p/goprotobuf/).
  6. Levigo, a Go-wrapper around LevelDB's C library: [https://github.com/jmhodges/levigo](https://github.com/jmhodges/levigo).
  7. GoRest, a RESTful style web-services framework: [http://code.google.com/p/gorest/](http://code.google.com/p/gorest/).
  8. Prometheus Client, Prometheus in Prometheus [https://github.com/prometheus/client_golang](https://github.com/prometheus/client_golang).
  9. Snappy, a compression library for LevelDB and Levigo [http://code.google.com/p/snappy/](http://code.google.com/p/snappy/).

## Getting Started

For basic help how to get started:

  * The source code is periodically indexed: [Prometheus Core](http://godoc.org/github.com/prometheus/prometheus).
  * For UNIX-like environment users users, please consult the Travis CI configuration in _.travis.yml_ and _Makefile_.
  * All of the core developers are accessible via the [Prometheus Developers Mailinglist](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers).

### Working with GVM
Starting out, the following workflow is advised:

    $ gvm install go1.1
    $ gvm use go1.1
    $ gvm pkgset create prometheus
    $ gvm pkgset use prometheus

This is mainly due to the fact that it allows the user to partition the
compile-time side-effects from the rest of one's work environment, particularly
``${GOPATH}``.

Read below in the _General_ section.

### Not Using GVM
It is entirely possible to build Prometheus without ``gvm``.  I presently do not
advise taking this route due to build environment pollution both against
Prometheus and from its build infrastructure onto the system---namely
``${GOPATH}``.

If you really want to avoid GVM, execute the following:

    $ touch build/gvm-stamp

Read below in the _General_ section.

### General

For first time users, simply run the following:

    $ make

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

Executing the following target will start up Prometheus for lazy users:

   $ ARGUMENTS="-foo -bar -baz" make run

``${ARGUMENTS}`` is passed verbatim into the makefile and thusly Prometheus as
``$(ARGUMENTS)``.  This is useful for quick one-off invocations and smoke
testing.

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

Go 1.1 includes a [race
detector](http://tip.golang.org/doc/articles/race_detector.html) which can be
enabled at build time. Here's how to use it with prometheus (assumes that
you've already run a successful build).

To run the tests with race detection:

    $ GORACE="log_path=/tmp/foo" go test -race ./...

To run the server with race detection:

    $ go build -race .
    $ GORACE="log_path=/tmp/foo" ./prometheus

[![Build Status](https://travis-ci.org/prometheus/prometheus.png)](https://travis-ci.org/prometheus/prometheus)

## Contributing

1. To start, reach out via our mailing list (mentioned above) and ask us what
   the current priorities are.  We can find a good isolated starter project for
   you.

2. Keeping code hygiene is important.  We thusly have a practical preference
   for the following:

   1. Run ``make format`` to ensure the correctness of the Go code's layout.

   2. Run ``make advice`` to find facial errors with a static analyzer.

   3. Try to capture your changes in some form of a test.  Go makes it easy to
      write [Table Driven Tests](https://code.google.com/p/go-wiki/wiki/TableDrivenTests).
      There is no mandate to use this said scaffolding mechanism, but it _can_
      make your life easier in the right circumstances.

3. Welcome aboard!

## License

Apache License 2.0
