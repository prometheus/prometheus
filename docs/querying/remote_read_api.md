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

## Behavior with external_labels

When using `external_labels` in the global configuration, it's important to understand how they interact with remote read:

**When exposing a remote read endpoint:**
- Prometheus serves the data as-is from its local storage through the remote read API
- The `external_labels` configured in `global.external_labels` are **not** automatically added to the time series returned via remote read
- Remote read returns the raw data exactly as it is stored in the Prometheus TSDB

**When using remote read to query another Prometheus:**
- When Prometheus queries a remote read endpoint, it receives the time series without any external labels from the remote source
- The querying Prometheus instance will match series based on the selectors in the query, ignoring any `external_labels` that may be configured on either side
- If you need to distinguish between local and remote data sources in queries, consider using source-specific labels during metric ingestion rather than relying on `external_labels`

**Important considerations:**
- `external_labels` are primarily used for federation, remote write, and Alertmanager integrations, not for remote read
- If `external_labels` are set and remote read is being used, there will be no errors, but the external labels won't affect remote read queries
- For proper remote read functionality across Prometheus instances with `external_labels`, ensure that label matching in your queries accounts for the actual labels present in the stored metrics, not the external labels

## Samples

This returns a message that includes a list of raw samples matching the
requested query.

## Streamed Chunks

These streamed chunks utilize an XOR algorithm inspired by the [Gorilla](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf)
compression to encode the chunks. However, it provides resolution to the millisecond instead of to the second.
