go-resiliency
=============

[![Build Status](https://travis-ci.org/eapache/go-resiliency.svg?branch=master)](https://travis-ci.org/eapache/go-resiliency)
[![GoDoc](https://godoc.org/github.com/eapache/go-resiliency?status.svg)](https://godoc.org/github.com/eapache/go-resiliency)
[![Code of Conduct](https://img.shields.io/badge/code%20of%20conduct-active-blue.svg)](https://eapache.github.io/conduct.html)

Resiliency patterns for golang.
Based in part on [Hystrix](https://github.com/Netflix/Hystrix),
[Semian](https://github.com/Shopify/semian), and others.

Currently implemented patterns include:
- circuit-breaker (in the `breaker` directory)
- semaphore (in the `semaphore` directory)
- deadline/timeout (in the `deadline` directory)
- batching (in the `batcher` directory)
- retriable (in the `retrier` directory)

Follows semantic versioning using https://gopkg.in/ - import from
[`gopkg.in/eapache/go-resiliency.v1`](https://gopkg.in/eapache/go-resiliency.v1)
for guaranteed API stability.
