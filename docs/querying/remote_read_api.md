---
title: Remote Read API
sort_rank: 7
---

# Remote Read API

> This is not currently considered part of the stable API and is subject to change even between non-major version releases of Prometheus.

This API provides data read functionality from Prometheus. This interface expects [snappy](https://github.com/google/snappy) compression.
The API definition is located [here](https://github.com/prometheus/prometheus/blob/master/prompb/remote.proto).

Request are made to the following endpoint.
```
/api/v1/read
```

### Samples

This returns a message that includes a list of raw samples.

### Streamed Chunks

These streamed chunks utilize an XOR algorithm inspired by the [Gorilla](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf)
compression to encode the chunks. However, it provides resolution to the millisecond instead of to the second.
