# Prometheus

Bedecke deinen Himmel, Zeus!  A new kid is in town.

Prometheus is a generic time series collection and computation server that is
useful in the following fields:

1. Industrial Experimentation / Real-Time Behavioral Validation / Software Release Qualification
2. Econometric and Natural Sciences
3. Operational Concerns and Monitoring

The system is designed to collect telemetry from named targets on given
intervals, evaluate rule expressions, display the results, and trigger an
action if some condition is observed to be true.

## Prerequisites

  1. Go 1.0.X. [GVM](https://github.com/moovweb/gvm) is highly recommended as well.
  2. LevelDB: (https://code.google.com/p/leveldb/).
  3. Protocol Buffers Compiler: (http://code.google.com/p/protobuf/).
  4. goprotobuf: the code generator and runtime library: (http://code.google.com/p/goprotobuf/).
  5. Levigo, a Go-wrapper around LevelDB's C library: (https://github.com/jmhodges/levigo).
  6. GoRest, a RESTful style web-services framework: (http://code.google.com/p/gorest/).
  7. Prometheus Client, Prometheus in Prometheus (https://github.com/prometheus/client_golang).
  8. Snappy, a compression library for LevelDB and Levigo (http://code.google.com/p/snappy/).

## Getting started

For basic help how to get started:

  * For Linux users, please consult the Travis CI configuration in _.travis.yml_.
  * [Getting started on Mac OSX](documentation/guides/getting-started-osx.md)

## License

Apache License 2.0
