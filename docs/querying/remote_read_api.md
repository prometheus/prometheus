---
title: Remote Read API
sort_rank: 7
---

NOTE: This is not currently considered part of the stable API and is subject to change even between non-major version releases of Prometheus.

This API provides data read functionality from Prometheus. This interface expects [snappy](https://github.com/google/snappy) compression.
The API definition is located [here](https://github.com/prometheus/prometheus/blob/main/prompb/remote.proto).
Protobuf definitions are also available on [buf.build](https://buf.build/prometheus/prometheus/docs/main:prometheus#prometheus.ReadRequest).

Request are made to the following endpoint.
```
/api/v1/read
```

## External labels and remote read

The `external_labels` setting in `global` affects remote read on both sides:

- **Exposing remote read:** Prometheus appends `external_labels` to every time series returned via `/api/v1/read`. The labels are not stored in TSDB; they are added at response time. Incoming equality matchers that match an external label are rewritten to match the empty string so that TSDB can satisfy the query.
- **Using remote read:** The querying Prometheus sends its own `external_labels` as additional equality matchers in the request, then strips them from the response.

If the querying Prometheus has `external_labels` that differ from or are absent on the remote server, queries may return no results because the matchers will not match. Ensure that any `external_labels` on the querying side are consistent with the labels present on the remote side.

## Samples

This returns a message that includes a list of raw samples matching the
requested query.

## Streamed Chunks

These streamed chunks utilize an XOR algorithm inspired by the [Gorilla](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf)
compression to encode the chunks. However, it provides resolution to the millisecond instead of to the second.
