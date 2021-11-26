# TSDB 

[![GoPkg](https://pkg.go.dev/badge/github.com/prometheus/prometheus/tsdb.svg)](https://pkg.go.dev/github.com/prometheus/prometheus@v1.8.2-0.20211105201321-411021ada9ab/tsdb)

This directory contains the Prometheus TSDB (Time Series DataBase) library,
which handles storage and querying of all Prometheus v2 data.

Due to an issue with versioning, the "latest" docs shown on Godoc are outdated.
Instead you may use [the docs for v2.31.1](https://pkg.go.dev/github.com/prometheus/prometheus@v1.8.2-0.20211105201321-411021ada9ab)

## Documentation

* [Data format](docs/format/README.md).
* [Usage](docs/usage.md).
* [Bstream details](docs/bstream.md).

## External resources

* A writeup of the original design can be found [here](https://fabxc.org/blog/2017-04-10-writing-a-tsdb/).
* Video: [Storing 16 Bytes at Scale](https://youtu.be/b_pEevMAC3I) from [PromCon 2017](https://promcon.io/2017-munich/).
* Compression is based on the Gorilla TSDB [white paper](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf).


A series of blog posts explaining different components of TSDB:
* [The Head Block](https://ganeshvernekar.com/blog/prometheus-tsdb-the-head-block/)
* [WAL and Checkpoint](https://ganeshvernekar.com/blog/prometheus-tsdb-wal-and-checkpoint/)
* [Memory Mapping of Head Chunks from Disk](https://ganeshvernekar.com/blog/prometheus-tsdb-mmapping-head-chunks-from-disk/)
* [Persistent Block and its Index](https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/)
* [Queries](https://ganeshvernekar.com/blog/prometheus-tsdb-queries/)
* [Compaction and Retention](https://ganeshvernekar.com/blog/prometheus-tsdb-compaction-and-retention/)
* [Snapshot on Shutdown](https://ganeshvernekar.com/blog/prometheus-tsdb-snapshot-on-shutdown/)
