---
title: Remote Read API
sort_rank: 7
---

# Remote Read API

This is not currently considered part of the stable API and is subject to change
even between non-major version releases of Prometheus.

## Format overview

The API response format is JSON. Every successful API request returns a `2xx`
status code.

Invalid requests that reach the API handlers return a JSON error object
and one of the following HTTP response codes:

- `400 Bad Request` when parameters are missing or incorrect.
- `422 Unprocessable Entity` when an expression can't be executed
  ([RFC4918](https://tools.ietf.org/html/rfc4918#page-78)).
- `503 Service Unavailable` when queries time out or abort.

Other non-`2xx` codes may be returned for errors occurring before the API
endpoint is reached.

An array of warnings may be returned if there are errors that do
not inhibit the request execution. All of the data that was successfully
collected will be returned in the data field.

The JSON response envelope format is as follows:

```
{
  "status": "success" | "error",
  "data": <data>,

  // Only set if status is "error". The data field may still hold
  // additional data.
  "errorType": "<string>",
  "error": "<string>",

  // Only if there were warnings while executing the request.
  // There will still be data in the data field.
  "warnings": ["<string>"]
}
```

Generic placeholders are defined as follows:

* `<rfc3339 | unix_timestamp>`: Input timestamps may be provided either in
[RFC3339](https://www.ietf.org/rfc/rfc3339.txt) format or as a Unix timestamp
in seconds, with optional decimal places for sub-second precision. Output
timestamps are always represented as Unix timestamps in seconds.
* `<series_selector>`: Prometheus [time series
selectors](basics.md#time-series-selectors) like `http_requests_total` or
`http_requests_total{method=~"(GET|POST)"}` and need to be URL-encoded.
* `<duration>`: [Prometheus duration strings](basics.md#time_durations).
For example, `5m` refers to a duration of 5 minutes.
* `<bool>`: boolean values (strings `true` and `false`).

Note: Names of query parameters that may be repeated end with `[]`.

## Remote Read API

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


