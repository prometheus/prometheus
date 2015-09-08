sarama
======

[![GoDoc](https://godoc.org/github.com/Shopify/sarama?status.png)](https://godoc.org/github.com/Shopify/sarama)
[![Build Status](https://travis-ci.org/Shopify/sarama.svg?branch=master)](https://travis-ci.org/Shopify/sarama)

Sarama is an MIT-licensed Go client library for Apache Kafka 0.8 (and later).

### Getting started

- API documentation and example are available via godoc at https://godoc.org/github.com/Shopify/sarama.
- Mocks for testing are available in the [mocks](./mocks) subpackage.
- The [examples](./examples) directory contains more elaborate example applications.
- The [tools](./tools) directory contains command line tools that can be useful for testing, diagnostics, and instrumentation.
- There is a google group for Kafka client users and authors at https://groups.google.com/forum/#!forum/kafka-clients

### Compatibility and API stability

Sarama provides a "2 releases + 2 months" compatibility guarantee: we support the two latest releases of Kafka
and Go, and we provide a two month grace period for older releases. This means we currently officially
support Go 1.3, 1.4, and 1.5, and Kafka 0.8.1 and 0.8.2.

Sarama follows semantic versioning and provides API stability via the gopkg.in service.
You can import a version with a guaranteed stable API via http://gopkg.in/Shopify/sarama.v1.
A changelog is available [here](CHANGELOG.md).

### Other

* [Sarama wiki](https://github.com/Shopify/sarama/wiki) to get started hacking on sarama itself.
* [Kafka Project Home](https://kafka.apache.org/)
* [Kafka Protocol Specification](https://cwiki.apache.org/confluence/display/KAFKA/A+Guide+To+The+Kafka+Protocol)
