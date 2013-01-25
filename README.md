# Prometheus

Bedecke deinen Himmel, Zeus!  A new kid is in town.


## Prerequisites

  1. Go 1.0.X.
  2. LevelDB: (https://code.google.com/p/leveldb/).
  3. Protocol Buffers Compiler: (http://code.google.com/p/protobuf/).
  4. goprotobuf: the code generator and runtime library: (http://code.google.com/p/goprotobuf/).
  5. Levigo, a Go-wrapper around LevelDB's C library: (https://github.com/jmhodges/levigo).
  6. GoRest, a RESTful style web-services framework: (http://code.google.com/p/gorest/).
  7. Prometheus Client, Prometheus in Prometheus (https://github.com/matttproud/golang_instrumentation).

## Getting started

For basic help how to get started:

  * [Getting started on Mac OSX](guides/getting-started-osx.md)


## Initial Hurdles

  1. A bit of this grew organically without an easy way of binding it all together.  The tests will pass but slowly.  They were not optimized for speed but end-to-end coverage of the whole storage model.  This is something immediate to fix.
  2. Protocol Buffer generator for Go changed emitted output API.  This will need to be fixed before other contributors can participate.


## Milestones

  1. In-memory archive, basic rule language, simple computation engine, and naive exposition system.


## License

Apache License 2.0
