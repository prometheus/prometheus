---
title: HTTP API
sort_rank: 7
---

# HTTP API

The current stable HTTP API is reachable under `/api/v1` on a Prometheus
server. Any non-breaking additions will be added under that endpoint.

## Format overview

The API response format is JSON. Every successful API request returns a `2xx`
status code.

Invalid requests that reach the API handlers return a JSON error object
and one of the following HTTP response codes:

- `400 Bad Request` when parameters are missing or incorrect.
- `422 Unprocessable Entity` when an expression can't be executed
  ([RFC4918](http://tools.ietf.org/html/rfc4918#page-78)).
- `503 Service Unavailable` when queries time out or abort.

Other non-`2xx` codes may be returned for errors occurring before the API
endpoint is reached.

The JSON response envelope format is as follows:

```
{
  "status": "success" | "error",
  "data": <data>,

  // Only set if status is "error". The data field may still hold
  // additional data.
  "errorType": "<string>",
  "error": "<string>"
}
```

Input timestamps may be provided either in
[RFC3339](https://www.ietf.org/rfc/rfc3339.txt) format or as a Unix timestamp
in seconds, with optional decimal places for sub-second precision. Output
timestamps are always represented as Unix timestamps in seconds.

Names of query parameters that may be repeated end with `[]`.

`<series_selector>` placeholders refer to Prometheus [time series
selectors](basics.md#time-series-selectors) like `http_requests_total` or
`http_requests_total{method=~"(GET|POST)"}` and need to be URL-encoded.

`<duration>` placeholders refer to Prometheus duration strings of the form
`[0-9]+[smhdwy]`. For example, `5m` refers to a duration of 5 minutes.

## Expression queries

Query language expressions may be evaluated at a single instant or over a range
of time. The sections below describe the API endpoints for each type of
expression query.

### Instant queries

The following endpoint evaluates an instant query at a single point in time:

```
GET /api/v1/query
```

URL query parameters:

- `query=<string>`: Prometheus expression query string.
- `time=<rfc3339 | unix_timestamp>`: Evaluation timestamp. Optional.
- `timeout=<duration>`: Evaluation timeout. Optional. Defaults to and
   is capped by the value of the `-query.timeout` flag.

The current server time is used if the `time` parameter is omitted.

The `data` section of the query result has the following format:

```
{
  "resultType": "matrix" | "vector" | "scalar" | "string",
  "result": <value>
}
```

`<value>` refers to the query result data, which has varying formats
depending on the `resultType`. See the [expression query result
formats](#expression-query-result-formats).

The following example evaluates the expression `up` at the time
`2015-07-01T20:10:51.781Z`:

```json
$ curl 'http://localhost:9090/api/v1/query?query=up&time=2015-07-01T20:10:51.781Z'
{
   "status" : "success",
   "data" : {
      "resultType" : "vector",
      "result" : [
         {
            "metric" : {
               "__name__" : "up",
               "job" : "prometheus",
               "instance" : "localhost:9090"
            },
            "value": [ 1435781451.781, "1" ]
         },
         {
            "metric" : {
               "__name__" : "up",
               "job" : "node",
               "instance" : "localhost:9100"
            },
            "value" : [ 1435781451.781, "0" ]
         }
      ]
   }
}
```

### Range queries

The following endpoint evaluates an expression query over a range of time:

```
GET /api/v1/query_range
```

URL query parameters:

- `query=<string>`: Prometheus expression query string.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp.
- `end=<rfc3339 | unix_timestamp>`: End timestamp.
- `step=<duration>`: Query resolution step width.
- `timeout=<duration>`: Evaluation timeout. Optional. Defaults to and
   is capped by the value of the `-query.timeout` flag.

The `data` section of the query result has the following format:

```
{
  "resultType": "matrix",
  "result": <value>
}
```

For the format of the `<value>` placeholder, see the [range-vector result
format](#range-vectors).

The following example evaluates the expression `up` over a 30-second range with
a query resolution of 15 seconds.

```json
$ curl 'http://localhost:9090/api/v1/query_range?query=up&start=2015-07-01T20:10:30.781Z&end=2015-07-01T20:11:00.781Z&step=15s'
{
   "status" : "success",
   "data" : {
      "resultType" : "matrix",
      "result" : [
         {
            "metric" : {
               "__name__" : "up",
               "job" : "prometheus",
               "instance" : "localhost:9090"
            },
            "values" : [
               [ 1435781430.781, "1" ],
               [ 1435781445.781, "1" ],
               [ 1435781460.781, "1" ]
            ]
         },
         {
            "metric" : {
               "__name__" : "up",
               "job" : "node",
               "instance" : "localhost:9091"
            },
            "values" : [
               [ 1435781430.781, "0" ],
               [ 1435781445.781, "0" ],
               [ 1435781460.781, "1" ]
            ]
         }
      ]
   }
}
```

## Querying metadata

### Finding series by label matchers

The following endpoint returns the list of time series that match a certain label set.

```
GET /api/v1/series
```

URL query parameters:

- `match[]=<series_selector>`: Repeated series selector argument that selects the
  series to return. At least one `match[]` argument must be provided.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp.
- `end=<rfc3339 | unix_timestamp>`: End timestamp.

The `data` section of the query result consists of a list of objects that
contain the label name/value pairs which identify each series.

The following example returns all series that match either of the selectors
`up` or `process_start_time_seconds{job="prometheus"}`:

```json
$ curl -g 'http://localhost:9090/api/v1/series?match[]=up&match[]=process_start_time_seconds{job="prometheus"}'
{
   "status" : "success",
   "data" : [
      {
         "__name__" : "up",
         "job" : "prometheus",
         "instance" : "localhost:9090"
      },
      {
         "__name__" : "up",
         "job" : "node",
         "instance" : "localhost:9091"
      },
      {
         "__name__" : "process_start_time_seconds",
         "job" : "prometheus",
         "instance" : "localhost:9090"
      }
   ]
}
```

### Querying label values

The following endpoint returns a list of label values for a provided label name:

```
GET /api/v1/label/<label_name>/values
```

The `data` section of the JSON response is a list of string label names.

This example queries for all label values for the `job` label:

```json
$ curl http://localhost:9090/api/v1/label/job/values
{
   "status" : "success",
   "data" : [
      "node",
      "prometheus"
   ]
}
```

## Expression query result formats

Expression queries may return the following response values in the `result`
property of the `data` section. `<sample_value>` placeholders are numeric
sample values. JSON does not support special float values such as `NaN`, `Inf`,
and `-Inf`, so sample values are transferred as quoted JSON strings rather than
raw numbers.

### Range vectors

Range vectors are returned as result type `matrix`. The corresponding
`result` property has the following format:

```
[
  {
    "metric": { "<label_name>": "<label_value>", ... },
    "values": [ [ <unix_time>, "<sample_value>" ], ... ]
  },
  ...
]
```

### Instant vectors

Instant vectors are returned as result type `vector`. The corresponding
`result` property has the following format:

```
[
  {
    "metric": { "<label_name>": "<label_value>", ... },
    "value": [ <unix_time>, "<sample_value>" ]
  },
  ...
]
```

### Scalars

Scalar results are returned as result type `scalar`. The corresponding
`result` property has the following format:

```
[ <unix_time>, "<scalar_value>" ]
```

### Strings

String results are returned as result type `string`. The corresponding
`result` property has the following format:

```
[ <unix_time>, "<string_value>" ]
```

## Targets

> This API is experimental as it is intended to be extended with targets
> dropped due to relabelling in the future.

The following endpoint returns an overview of the current state of the
Prometheus target discovery:

```
GET /api/v1/targets
```

Currently only the active targets are part of the response.

```json
$ curl http://localhost:9090/api/v1/targets
{
  "status": "success",                                                                                                                                [3/11]
  "data": {
    "activeTargets": [
      {
        "discoveredLabels": {
          "__address__": "127.0.0.1:9090",
          "__metrics_path__": "/metrics",
          "__scheme__": "http",
          "job": "prometheus"
        },
        "labels": {
          "instance": "127.0.0.1:9090",
          "job": "prometheus"
        },
        "scrapeUrl": "http://127.0.0.1:9090/metrics",
        "lastError": "",
        "lastScrape": "2017-01-17T15:07:44.723715405+01:00",
        "health": "up"
      }
    ]
  }
}
```

## Alertmanagers

> This API is experimental as it is intended to be extended with Alertmanagers
> dropped due to relabelling in the future.

The following endpoint returns an overview of the current state of the
Prometheus alertmanager discovery:

```
GET /api/v1/alertmanagers
```

Currently only the active Alertmanagers are part of the response.

```json
$ curl http://localhost:9090/api/v1/alertmanagers
{
  "status": "success",
  "data": {
    "activeAlertmanagers": [
      {
        "url": "http://127.0.0.1:9090/api/v1/alerts"
      }
    ]
  }
}
```


## TSDB Admin APIs
These are APIs that expose database functionalities for the advanced user. These APIs are not enabled unless the `--web.enable-admin-api` is set.

We also expose a gRPC API whose definition can be found [here](https://github.com/prometheus/prometheus/blob/master/prompb/rpc.proto). This is experimental and might change in the future.

### Snapshot
Snapshot creates a snapshot of all current data into `snapshots/<datetime>-<rand>` under the TSDB's data directory and returns the directory as response.

```
POST /api/v1/admin/tsdb/snapshot
```

```json
$ curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot
{
  "status": "success",
  "data": {
    "name": "20171210T211224Z-2be650b6d019eb54"
  }
}
```

The snapshot now exists at `<data-dir>/snapshots/20171210T211224Z-2be650b6d019eb54`

*New in v2.1*

### Delete Series
DeleteSeries deletes data for a selection of series in a time range. The actual data still exists on disk and is cleaned up in future compactions or can be explicitly cleaned up by hitting the Clean Tombstones endpoint.

If successful, a `204` is returned.

```
POST /api/v1/admin/tsdb/delete_series
```

URL query parameters:

- `match[]=<series_selector>`: Repeated label matcher argument that selects the series to delete. At least one `match[]` argument must be provided.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp. Optional and defaults to minimum possible time.
- `end=<rfc3339 | unix_timestamp>`: End timestamp. Optional and defaults to maximum possible time.

Not mentioning both start and end times would clear all the data for the matched series in the database.

Example:

```json
$ curl -X DELETE \                                                              
  -g 'http://localhost:9090/api/v1/series?match[]=up&match[]=process_start_time_seconds{job="prometheus"}'
```
*New in v2.1*

### Clean Tombstones
CleanTombstones removes the deleted data from disk and cleans up the existing tombstones. This can be used after deleting series to free up space.

If successful, a `204` is returned.

```
POST /api/v1/admin/tsdb/clean_tombstones
```

This takes no parameters or body.

```json
$ curl -XPOST http://localhost:9090/api/v1/admin/tsdb/clean_tombstones
```

*New in v2.1*
