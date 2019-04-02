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

`<bool>` placeholders refer to boolean values (strings `true` and `false`).

## Expression queries

Query language expressions may be evaluated at a single instant or over a range
of time. The sections below describe the API endpoints for each type of
expression query.

### Instant queries

The following endpoint evaluates an instant query at a single point in time:

```
GET /api/v1/query
POST /api/v1/query
```

URL query parameters:

- `query=<string>`: Prometheus expression query string.
- `time=<rfc3339 | unix_timestamp>`: Evaluation timestamp. Optional.
- `timeout=<duration>`: Evaluation timeout. Optional. Defaults to and
   is capped by the value of the `-query.timeout` flag.

The current server time is used if the `time` parameter is omitted.

You can URL-encode these parameters directly in the request body by using the `POST` method and
`Content-Type: application/x-www-form-urlencoded` header. This is useful when specifying a large
query that may breach server-side URL character limits.

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
POST /api/v1/query_range
```

URL query parameters:

- `query=<string>`: Prometheus expression query string.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp.
- `end=<rfc3339 | unix_timestamp>`: End timestamp.
- `step=<duration | float>`: Query resolution step width in `duration` format or float number of seconds.
- `timeout=<duration>`: Evaluation timeout. Optional. Defaults to and
   is capped by the value of the `-query.timeout` flag.

You can URL-encode these parameters directly in the request body by using the `POST` method and
`Content-Type: application/x-www-form-urlencoded` header. This is useful when specifying a large
query that may breach server-side URL character limits.

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
POST /api/v1/series
```

URL query parameters:

- `match[]=<series_selector>`: Repeated series selector argument that selects the
  series to return. At least one `match[]` argument must be provided.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp.
- `end=<rfc3339 | unix_timestamp>`: End timestamp.

You can URL-encode these parameters directly in the request body by using the `POST` method and
`Content-Type: application/x-www-form-urlencoded` header. This is useful when specifying a large
or dynamic number of series selectors that may breach server-side URL character limits.

The `data` section of the query result consists of a list of objects that
contain the label name/value pairs which identify each series.

The following example returns all series that match either of the selectors
`up` or `process_start_time_seconds{job="prometheus"}`:

```json
$ curl -g 'http://localhost:9090/api/v1/series?' --data-urlencode='match[]=up' --data-urlencode='match[]=process_start_time_seconds{job="prometheus"}'
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

### Getting label names

The following endpoint returns a list of label names:

```
GET /api/v1/labels
POST /api/v1/labels
```

The `data` section of the JSON response is a list of string label names.

Here is an example.

```json
$ curl 'localhost:9090/api/v1/labels'
{
    "status": "success",
    "data": [
        "__name__",
        "call",
        "code",
        "config",
        "dialer_name",
        "endpoint",
        "event",
        "goversion",
        "handler",
        "instance",
        "interval",
        "job",
        "le",
        "listener_name",
        "name",
        "quantile",
        "reason",
        "role",
        "scrape_job",
        "slice",
        "version"
    ]
}
```

### Querying label values

The following endpoint returns a list of label values for a provided label name:

```
GET /api/v1/label/<label_name>/values
```

The `data` section of the JSON response is a list of string label values.

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

The following endpoint returns an overview of the current state of the
Prometheus target discovery:

```
GET /api/v1/targets
```

Both the active and dropped targets are part of the response.
`labels` represents the label set after relabelling has occurred.
`discoveredLabels` represent the unmodified labels retrieved during service discovery before relabelling has occurred.

```json
$ curl http://localhost:9090/api/v1/targets
{
  "status": "success",
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
    ],
    "droppedTargets": [
      {
        "discoveredLabels": {
          "__address__": "127.0.0.1:9100",
          "__metrics_path__": "/metrics",
          "__scheme__": "http",
          "job": "node"
        },
      }
    ]
  }
}
```


## Rules

The `/rules` API endpoint returns a list of alerting and recording rules that
are currently loaded. In addition it returns the currently active alerts fired
by the Prometheus instance of each alerting rule.

As the `/rules` endpoint is fairly new, it does not have the same stability
guarantees as the overarching API v1.

```
GET /api/v1/rules
```

```json
$ curl http://localhost:9090/api/v1/rules

{
    "data": {
        "groups": [
            {
                "rules": [
                    {
                        "alerts": [
                            {
                                "activeAt": "2018-07-04T20:27:12.60602144+02:00",
                                "annotations": {
                                    "summary": "High request latency"
                                },
                                "labels": {
                                    "alertname": "HighRequestLatency",
                                    "severity": "page"
                                },
                                "state": "firing",
                                "value": 1
                            }
                        ],
                        "annotations": {
                            "summary": "High request latency"
                        },
                        "duration": 600,
                        "health": "ok",
                        "labels": {
                            "severity": "page"
                        },
                        "name": "HighRequestLatency",
                        "query": "job:request_latency_seconds:mean5m{job=\"myjob\"} > 0.5",
                        "type": "alerting"
                    },
                    {
                        "health": "ok",
                        "name": "job:http_inprogress_requests:sum",
                        "query": "sum(http_inprogress_requests) by (job)",
                        "type": "recording"
                    }
                ],
                "file": "/rules.yaml",
                "interval": 60,
                "name": "example"
            }
        ]
    },
    "status": "success"
}
```


## Alerts

The `/alerts` endpoint returns a list of all active alerts.

As the `/alerts` endpoint is fairly new, it does not have the same stability
guarantees as the overarching API v1.

```
GET /api/v1/alerts
```

```json
$ curl http://localhost:9090/api/v1/alerts

{
    "data": {
        "alerts": [
            {
                "activeAt": "2018-07-04T20:27:12.60602144+02:00",
                "annotations": {},
                "labels": {
                    "alertname": "my-alert"
                },
                "state": "firing",
                "value": 1
            }
        ]
    },
    "status": "success"
}
```

## Querying target metadata

The following endpoint returns metadata about metrics currently scraped by targets.
This is **experimental** and might change in the future.

```
GET /api/v1/targets/metadata
```

URL query parameters:

- `match_target=<label_selectors>`: Label selectors that match targets by their label sets. All targets are selected if left empty.
- `metric=<string>`: A metric name to retrieve metadata for. All metric metadata is retrieved if left empty.
- `limit=<number>`: Maximum number of targets to match.

The `data` section of the query result consists of a list of objects that
contain metric metadata and the target label set.

The following example returns all metadata entries for the `go_goroutines` metric
from the first two targets with label `job="prometheus"`.

```json
curl -G http://localhost:9091/api/v1/targets/metadata \
    --data-urlencode 'metric=go_goroutines' \
    --data-urlencode 'match_target={job="prometheus"}' \
    --data-urlencode 'limit=2'
{
  "status": "success",
  "data": [
    {
      "target": {
        "instance": "127.0.0.1:9090",
        "job": "prometheus"
      },
      "type": "gauge",
      "help": "Number of goroutines that currently exist.",
      "unit": ""
    },
    {
      "target": {
        "instance": "127.0.0.1:9091",
        "job": "prometheus"
      },
      "type": "gauge",
      "help": "Number of goroutines that currently exist.",
      "unit": ""
    }
  ]
}
```

The following example returns metadata for all metrics for all targets with
label `instance="127.0.0.1:9090`.

```json
curl -G http://localhost:9091/api/v1/targets/metadata \
    --data-urlencode 'match_target={instance="127.0.0.1:9090"}'
{
  "status": "success",
  "data": [
    // ...
    {
      "target": {
        "instance": "127.0.0.1:9090",
        "job": "prometheus"
      },
      "metric": "prometheus_treecache_zookeeper_failures_total",
      "type": "counter",
      "help": "The total number of ZooKeeper failures.",
      "unit": ""
    },
    {
      "target": {
        "instance": "127.0.0.1:9090",
        "job": "prometheus"
      },
      "metric": "prometheus_tsdb_reloads_total",
      "type": "counter",
      "help": "Number of times the database reloaded block data from disk.",
      "unit": ""
    },
    // ...
  ]
}
```

## Alertmanagers

The following endpoint returns an overview of the current state of the
Prometheus alertmanager discovery:

```
GET /api/v1/alertmanagers
```

Both the active and dropped Alertmanagers are part of the response.

```json
$ curl http://localhost:9090/api/v1/alertmanagers
{
  "status": "success",
  "data": {
    "activeAlertmanagers": [
      {
        "url": "http://127.0.0.1:9090/api/v1/alerts"
      }
    ],
    "droppedAlertmanagers": [
      {
        "url": "http://127.0.0.1:9093/api/v1/alerts"
      }
    ]
  }
}
```

## Status

Following status endpoints expose current Prometheus configuration.

### Config

The following endpoint returns currently loaded configuration file:

```
GET /api/v1/status/config
```

The config is returned as dumped YAML file. Due to limitation of the YAML
library, YAML comments are not included.

```json
$ curl http://localhost:9090/api/v1/status/config
{
  "status": "success",
  "data": {
    "yaml": "<content of the loaded config file in YAML>",
  }
}
```

### Flags

The following endpoint returns flag values that Prometheus was configured with:

```
GET /api/v1/status/flags
```

All values are in a form of "string".

```json
$ curl http://localhost:9090/api/v1/status/flags
{
  "status": "success",
  "data": {
    "alertmanager.notification-queue-capacity": "10000",
    "alertmanager.timeout": "10s",
    "log.level": "info",
    "query.lookback-delta": "5m",
    "query.max-concurrency": "20",
    ...
  }
}
```

*New in v2.2*

## TSDB Admin APIs
These are APIs that expose database functionalities for the advanced user. These APIs are not enabled unless the `--web.enable-admin-api` is set.

We also expose a gRPC API whose definition can be found [here](https://github.com/prometheus/prometheus/blob/master/prompb/rpc.proto). This is experimental and might change in the future.

### Snapshot
Snapshot creates a snapshot of all current data into `snapshots/<datetime>-<rand>` under the TSDB's data directory and returns the directory as response.
It will optionally skip snapshotting data that is only present in the head block, and which has not yet been compacted to disk.

```
POST /api/v1/admin/tsdb/snapshot?skip_head=<bool>
PUT /api/v1/admin/tsdb/snapshot?skip_head=<bool>
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

*New in v2.1 and supports PUT from v2.9*

### Delete Series
DeleteSeries deletes data for a selection of series in a time range. The actual data still exists on disk and is cleaned up in future compactions or can be explicitly cleaned up by hitting the Clean Tombstones endpoint.

If successful, a `204` is returned.

```
POST /api/v1/admin/tsdb/delete_series
PUT /api/v1/admin/tsdb/delete_series
```

URL query parameters:

- `match[]=<series_selector>`: Repeated label matcher argument that selects the series to delete. At least one `match[]` argument must be provided.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp. Optional and defaults to minimum possible time.
- `end=<rfc3339 | unix_timestamp>`: End timestamp. Optional and defaults to maximum possible time.

Not mentioning both start and end times would clear all the data for the matched series in the database.

Example:

```json
$ curl -X POST \
  -g 'http://localhost:9090/api/v1/admin/tsdb/delete_series?match[]=up&match[]=process_start_time_seconds{job="prometheus"}'
```
*New in v2.1 and supports PUT from v2.9*

### Clean Tombstones
CleanTombstones removes the deleted data from disk and cleans up the existing tombstones. This can be used after deleting series to free up space.

If successful, a `204` is returned.

```
POST /api/v1/admin/tsdb/clean_tombstones
PUT /api/v1/admin/tsdb/clean_tombstones
```

This takes no parameters or body.

```json
$ curl -XPOST http://localhost:9090/api/v1/admin/tsdb/clean_tombstones
```

*New in v2.1 and supports PUT from v2.9*
