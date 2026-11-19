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

## Query sharding

`Query.hints` can select a shard of the matching series with `shard_count` and
`shard_index` when the server is started with
[`--enable-feature=remote-read-sharding`](../feature_flags.md#remote-read-query-sharding).
This feature is required to shard the in-memory TSDB Head. Without it, requests
that read from the Head fail with HTTP 500 and `sharding is disabled`;
unsharded requests are unaffected.

For example, `shard_count: 4` and `shard_index: 2` request the
third of four shards. Set `shard_count` to zero to disable sharding; in that
case `shard_index` must also be zero. When sharding is enabled, the index must
be less than the count. Invalid shard hints return HTTP 400, including when
they occur after a valid query in the same request.

Sharding works for both sampled and streamed chunk responses. It uses the
stable hash of the series' TSDB labels modulo `shard_count`, before adding
external labels. Query all indices with the same count and matchers to retrieve
the complete series set without overlaps between shards.

Older remote-read servers may ignore these optional hints. Clients that rely
on server-side sharding must ensure the server supports them.

## Samples

This returns a message that includes a list of raw samples matching the
requested query.

## Streamed Chunks

These streamed chunks utilize an XOR algorithm inspired by the [Gorilla](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf)
compression to encode the chunks. However, it provides resolution to the millisecond instead of to the second.
