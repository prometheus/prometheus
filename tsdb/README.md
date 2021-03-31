# TSDB 

[![GoDoc](https://godoc.org/github.com/prometheus/prometheus/tsdb?status.svg)](https://godoc.org/github.com/prometheus/prometheus/tsdb)

This directory contains the Prometheus storage layer that is used in its 2.x releases.

A writeup of its design can be found [here](https://fabxc.org/blog/2017-04-10-writing-a-tsdb/).

Based on the Gorilla TSDB [white papers](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf).

Video: [Storing 16 Bytes at Scale](https://youtu.be/b_pEevMAC3I) from [PromCon 2017](https://promcon.io/2017-munich/).

See also the [format documentation](docs/format/README.md).

A series of blog posts explaining different components of TSDB:
* [The Head Block](https://ganeshvernekar.com/blog/prometheus-tsdb-the-head-block/)
* [WAL and Checkpoint](https://ganeshvernekar.com/blog/prometheus-tsdb-wal-and-checkpoint/)
* [Memory Mapping of Head Chunks from Disk](https://ganeshvernekar.com/blog/prometheus-tsdb-mmapping-head-chunks-from-disk/)
* [Persistent Block and its Index](https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/)
* [Queries](https://ganeshvernekar.com/blog/prometheus-tsdb-queries/)