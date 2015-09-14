Queue
=====

[![Build Status](https://travis-ci.org/eapache/queue.svg)](https://travis-ci.org/eapache/queue)
[![GoDoc](https://godoc.org/github.com/eapache/queue?status.png)](https://godoc.org/github.com/eapache/queue)
[![Code of Conduct](https://img.shields.io/badge/code%20of%20conduct-active-blue.svg)](https://eapache.github.io/conduct.html)

A fast Golang queue using a ring-buffer, based on the version suggested by Dariusz Górecki.
Using this instead of other, simpler, queue implementations (slice+append or linked list) provides
substantial memory and time benefits, and fewer GC pauses.

The queue implemented here is as fast as it is in part because it is *not* thread-safe.

Follows semantic versioning using https://gopkg.in/ - import from
[`gopkg.in/eapache/queue.v1`](https://gopkg.in/eapache/queue.v1)
for guaranteed API stability.
