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

Generic placeholders are defined as follows:

* `<rfc3339 | unix_timestamp>`: Input timestamps may be provided either in
[RFC3339](https://www.ietf.org/rfc/rfc3339.txt) format or as a Unix timestamp
in seconds, with optional decimal places for sub-second precision. Output
timestamps are always represented as Unix timestamps in seconds.
* `<series_selector>`: Prometheus [time series
selectors](basics.md#time-series-selectors) like `http_requests_total` or
`http_requests_total{method=~"(GET|POST)"}` and need to be URL-encoded.
* `<duration>`: [Prometheus duration strings](basics.md#time-durations).
For example, `5m` refers to a duration of 5 minutes.
* `<bool>`: boolean values (strings `true` and `false`).

Note: Names of query parameters that may be repeated end with `[]`.

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
- `start=<rfc3339 | unix_timestamp>`: Start timestamp, inclusive.
- `end=<rfc3339 | unix_timestamp>`: End timestamp, inclusive.
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

## Formatting query expressions

The following endpoint formats a PromQL expression in a prettified way:

```
GET /api/v1/format_query
POST /api/v1/format_query
```

URL query parameters:

- `query=<string>`: Prometheus expression query string.

You can URL-encode these parameters directly in the request body by using the `POST` method and
`Content-Type: application/x-www-form-urlencoded` header. This is useful when specifying a large
query that may breach server-side URL character limits.

The `data` section of the query result is a string containing the formatted query expression. Note that any comments are removed in the formatted string.

The following example formats the expression `foo/bar`:

```json
$ curl 'http://localhost:9090/api/v1/format_query?query=foo/bar'
{
   "status" : "success",
   "data" : "foo / bar"
}
```

## Querying metadata

Prometheus offers a set of API endpoints to query metadata about series and their labels.

NOTE: These API endpoints may return metadata for series for which there is no sample within the selected time range, and/or for series whose samples have been marked as deleted via the deletion API endpoint. The exact extent of additionally returned series metadata is an implementation detail that may change in the future.

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
$ curl -g 'http://localhost:9090/api/v1/series?' --data-urlencode 'match[]=up' --data-urlencode 'match[]=process_start_time_seconds{job="prometheus"}'
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

URL query parameters:

- `start=<rfc3339 | unix_timestamp>`: Start timestamp. Optional.
- `end=<rfc3339 | unix_timestamp>`: End timestamp. Optional.
- `match[]=<series_selector>`: Repeated series selector argument that selects the
  series from which to read the label names. Optional.


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

URL query parameters:

- `start=<rfc3339 | unix_timestamp>`: Start timestamp. Optional.
- `end=<rfc3339 | unix_timestamp>`: End timestamp. Optional.
- `match[]=<series_selector>`: Repeated series selector argument that selects the
  series from which to read the label values. Optional.


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

## Querying exemplars

This is **experimental** and might change in the future.
The following endpoint returns a list of exemplars for a valid PromQL query for a specific time range:

```
GET /api/v1/query_exemplars
POST /api/v1/query_exemplars
```

URL query parameters:

- `query=<string>`: Prometheus expression query string.
- `start=<rfc3339 | unix_timestamp>`: Start timestamp.
- `end=<rfc3339 | unix_timestamp>`: End timestamp.

```json
$ curl -g 'http://localhost:9090/api/v1/query_exemplars?query=test_exemplar_metric_total&start=2020-09-14T15:22:25.479Z&end=2020-09-14T15:23:25.479Z'
{
    "status": "success",
    "data": [
        {
            "seriesLabels": {
                "__name__": "test_exemplar_metric_total",
                "instance": "localhost:8090",
                "job": "prometheus",
                "service": "bar"
            },
            "exemplars": [
                {
                    "labels": {
                        "traceID": "EpTxMJ40fUus7aGY"
                    },
                    "value": "6",
                    "timestamp": 1600096945.479
                }
            ]
        },
        {
            "seriesLabels": {
                "__name__": "test_exemplar_metric_total",
                "instance": "localhost:8090",
                "job": "prometheus",
                "service": "foo"
            },
            "exemplars": [
                {
                    "labels": {
                        "traceID": "Olp9XHlq763ccsfa"
                    },
                    "value": "19",
                    "timestamp": 1600096955.479
                },
                {
                    "labels": {
                        "traceID": "hCtjygkIHwAN9vs4"
                    },
                    "value": "20",
                    "timestamp": 1600096965.489
                }
            ]
        }
    ]
}
```

## Expression query result formats

Expression queries may return the following response values in the `result`
property of the `data` section. `<sample_value>` placeholders are numeric
sample values. JSON does not support special float values such as `NaN`, `Inf`,
and `-Inf`, so sample values are transferred as quoted JSON strings rather than
raw numbers.

The keys `"histogram"` and `"histograms"` only show up if the experimental
native histograms are present in the response. Their placeholder `<histogram>`
is explained in detail in its own section below. 

### Range vectors

Range vectors are returned as result type `matrix`. The corresponding
`result` property has the following format:

```
[
  {
    "metric": { "<label_name>": "<label_value>", ... },
    "values": [ [ <unix_time>, "<sample_value>" ], ... ],
    "histograms": [ [ <unix_time>, <histogram> ], ... ]
  },
  ...
]
```

Each series could have the `"values"` key, or the `"histograms"` key, or both. 
For a given timestamp, there will only be one sample of either float or histogram type.

### Instant vectors

Instant vectors are returned as result type `vector`. The corresponding
`result` property has the following format:

```
[
  {
    "metric": { "<label_name>": "<label_value>", ... },
    "value": [ <unix_time>, "<sample_value>" ],
    "histogram": [ <unix_time>, <histogram> ]
  },
  ...
]
```

Each series could have the `"value"` key, or the `"histogram"` key, but not both.

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

### Native histograms

The `<histogram>` placeholder used above is formatted as follows.

_Note that native histograms are an experimental feature, and the format below
might still change._

```
{
  "count": "<count_of_observations>",
  "sum": "<sum_of_observations>",
  "buckets": [ [ <boundary_rule>, "<left_boundary>", "<right_boundary>", "<count_in_bucket>" ], ... ]
}
```

The `<boundary_rule>` placeholder is an integer between 0 and 3 with the
following meaning:

* 0: “open left” (left boundary is exclusive, right boundary in inclusive)
* 1: “open right” (left boundary is inclusive, right boundary in exclusive)
* 2: “open both” (both boundaries are exclusive)
* 3: “closed both” (both boundaries are inclusive)

Note that with the currently implemented bucket schemas, positive buckets are
“open left”, negative buckets are “open right”, and the zero bucket (with a
negative left boundary and a positive right boundary) is “closed both”.

## Targets

The following endpoint returns an overview of the current state of the
Prometheus target discovery:

```
GET /api/v1/targets
```

Both the active and dropped targets are part of the response by default.
Dropped targets are subject to `keep_dropped_targets` limit, if set.
`labels` represents the label set after relabeling has occurred.
`discoveredLabels` represent the unmodified labels retrieved during service discovery before relabeling has occurred.

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
        "scrapePool": "prometheus",
        "scrapeUrl": "http://127.0.0.1:9090/metrics",
        "globalUrl": "http://example-prometheus:9090/metrics",
        "lastError": "",
        "lastScrape": "2017-01-17T15:07:44.723715405+01:00",
        "lastScrapeDuration": 0.050688943,
        "health": "up",
        "scrapeInterval": "1m",
        "scrapeTimeout": "10s"
      }
    ],
    "droppedTargets": [
      {
        "discoveredLabels": {
          "__address__": "127.0.0.1:9100",
          "__metrics_path__": "/metrics",
          "__scheme__": "http",
          "__scrape_interval__": "1m",
          "__scrape_timeout__": "10s",
          "job": "node"
        },
      }
    ]
  }
}
```

The `state` query parameter allows the caller to filter by active or dropped targets,
(e.g., `state=active`, `state=dropped`, `state=any`).
Note that an empty array is still returned for targets that are filtered out.
Other values are ignored.

```json
$ curl 'http://localhost:9090/api/v1/targets?state=active'
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
        "scrapePool": "prometheus",
        "scrapeUrl": "http://127.0.0.1:9090/metrics",
        "globalUrl": "http://example-prometheus:9090/metrics",
        "lastError": "",
        "lastScrape": "2017-01-17T15:07:44.723715405+01:00",
        "lastScrapeDuration": 50688943,
        "health": "up"
      }
    ],
    "droppedTargets": []
  }
}
```

The `scrapePool` query parameter allows the caller to filter by scrape pool name.

```json
$ curl 'http://localhost:9090/api/v1/targets?scrapePool=node_exporter'
{
  "status": "success",
  "data": {
    "activeTargets": [
      {
        "discoveredLabels": {
          "__address__": "127.0.0.1:9091",
          "__metrics_path__": "/metrics",
          "__scheme__": "http",
          "job": "node_exporter"
        },
        "labels": {
          "instance": "127.0.0.1:9091",
          "job": "node_exporter"
        },
        "scrapePool": "node_exporter",
        "scrapeUrl": "http://127.0.0.1:9091/metrics",
        "globalUrl": "http://example-prometheus:9091/metrics",
        "lastError": "",
        "lastScrape": "2017-01-17T15:07:44.723715405+01:00",
        "lastScrapeDuration": 50688943,
        "health": "up"
      }
    ],
    "droppedTargets": []
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

URL query parameters:

- `type=alert|record`: return only the alerting rules (e.g. `type=alert`) or the recording rules (e.g. `type=record`). When the parameter is absent or empty, no filtering is done.
- `rule_name[]=<string>`: only return rules with the given rule name. If the parameter is repeated, rules with any of the provided names are returned. If we've filtered out all the rules of a group, the group is not returned. When the parameter is absent or empty, no filtering is done.
- `rule_group[]=<string>`: only return rules with the given rule group name. If the parameter is repeated, rules with any of the provided rule group names are returned. When the parameter is absent or empty, no filtering is done.
- `file[]=<string>`: only return rules with the given filepath. If the parameter is repeated, rules with any of the provided filepaths are returned. When the parameter is absent or empty, no filtering is done.

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
                                "value": "1e+00"
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
                        "query": "sum by (job) (http_inprogress_requests)",
                        "type": "recording"
                    }
                ],
                "file": "/rules.yaml",
                "interval": 60,
                "limit": 0,
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
                "value": "1e+00"
            }
        ]
    },
    "status": "success"
}
```

## Querying target metadata

The following endpoint returns metadata about metrics currently scraped from targets.
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

## Querying metric metadata

It returns metadata about metrics currently scraped from targets. However, it does not provide any target information.
This is considered **experimental** and might change in the future.

```
GET /api/v1/metadata
```

URL query parameters:

- `limit=<number>`: Maximum number of metrics to return.
- `limit_per_metric=<number>`: Maximum number of metadata to return per metric.
- `metric=<string>`: A metric name to filter metadata for. All metric metadata is retrieved if left empty.

The `data` section of the query result consists of an object where each key is a metric name and each value is a list of unique metadata objects, as exposed for that metric name across all targets.

The following example returns two metrics. Note that the metric `http_requests_total` has more than one object in the list. At least one target has a value for `HELP` that do not match with the rest.

```json
curl -G http://localhost:9090/api/v1/metadata?limit=2

{
  "status": "success",
  "data": {
    "cortex_ring_tokens": [
      {
        "type": "gauge",
        "help": "Number of tokens in the ring",
        "unit": ""
      }
    ],
    "http_requests_total": [
      {
        "type": "counter",
        "help": "Number of HTTP requests",
        "unit": ""
      },
      {
        "type": "counter",
        "help": "Amount of HTTP requests",
        "unit": ""
      }
    ]
  }
}
```

The following example returns only one metadata entry for each metric.

```json
curl -G http://localhost:9090/api/v1/metadata?limit_per_metric=1

{
  "status": "success",
  "data": {
    "cortex_ring_tokens": [
      {
        "type": "gauge",
        "help": "Number of tokens in the ring",
        "unit": ""
      }
    ],
    "http_requests_total": [
      {
        "type": "counter",
        "help": "Number of HTTP requests",
        "unit": ""
      }
    ]
  }
}
```

The following example returns metadata only for the metric `http_requests_total`.

```json
curl -G http://localhost:9090/api/v1/metadata?metric=http_requests_total

{
  "status": "success",
  "data": {
    "http_requests_total": [
      {
        "type": "counter",
        "help": "Number of HTTP requests",
        "unit": ""
      },
      {
        "type": "counter",
        "help": "Amount of HTTP requests",
        "unit": ""
      }
    ]
  }
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

All values are of the result type `string`.

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

### Runtime Information

The following endpoint returns various runtime information properties about the Prometheus server:

```
GET /api/v1/status/runtimeinfo
```

The returned values are of different types, depending on the nature of the runtime property.

```json
$ curl http://localhost:9090/api/v1/status/runtimeinfo
{
  "status": "success",
  "data": {
    "startTime": "2019-11-02T17:23:59.301361365+01:00",
    "CWD": "/",
    "reloadConfigSuccess": true,
    "lastConfigTime": "2019-11-02T17:23:59+01:00",
    "timeSeriesCount": 873,
    "corruptionCount": 0,
    "goroutineCount": 48,
    "GOMAXPROCS": 4,
    "GOGC": "",
    "GODEBUG": "",
    "storageRetention": "15d"
  }
}
```

NOTE: The exact returned runtime properties may change without notice between Prometheus versions.

*New in v2.14*

### Build Information

The following endpoint returns various build information properties about the Prometheus server:

```
GET /api/v1/status/buildinfo
```

All values are of the result type `string`.

```json
$ curl http://localhost:9090/api/v1/status/buildinfo
{
  "status": "success",
  "data": {
    "version": "2.13.1",
    "revision": "cb7cbad5f9a2823a622aaa668833ca04f50a0ea7",
    "branch": "master",
    "buildUser": "julius@desktop",
    "buildDate": "20191102-16:19:59",
    "goVersion": "go1.13.1"
  }
}
```

NOTE: The exact returned build properties may change without notice between Prometheus versions.

*New in v2.14*

### TSDB Stats

The following endpoint returns various cardinality statistics about the Prometheus TSDB:

```
GET /api/v1/status/tsdb
```
URL query parameters:
- `limit=<number>`: Limit the number of returned items to a given number for each set of statistics. By default, 10 items are returned.

The `data` section of the query result consists of
- **headStats**: This provides the following data about the head block of the TSDB:
  - **numSeries**: The number of series.
  - **chunkCount**: The number of chunks.
  - **minTime**: The current minimum timestamp in milliseconds.
  - **maxTime**: The current maximum timestamp in milliseconds.
- **seriesCountByMetricName:**  This will provide a list of metrics names and their series count.
- **labelValueCountByLabelName:** This will provide a list of the label names and their value count.
- **memoryInBytesByLabelName** This will provide a list of the label names and memory used in bytes. Memory usage is calculated by adding the length of all values for a given label name.
- **seriesCountByLabelPair** This will provide a list of label value pairs and their series count.

```json
$ curl http://localhost:9090/api/v1/status/tsdb
{
  "status": "success",
  "data": {
    "headStats": {
      "numSeries": 508,
      "chunkCount": 937,
      "minTime": 1591516800000,
      "maxTime": 1598896800143,
    },
    "seriesCountByMetricName": [
      {
        "name": "net_conntrack_dialer_conn_failed_total",
        "value": 20
      },
      {
        "name": "prometheus_http_request_duration_seconds_bucket",
        "value": 20
      }
    ],
    "labelValueCountByLabelName": [
      {
        "name": "__name__",
        "value": 211
      },
      {
        "name": "event",
        "value": 3
      }
    ],
    "memoryInBytesByLabelName": [
      {
        "name": "__name__",
        "value": 8266
      },
      {
        "name": "instance",
        "value": 28
      }
    ],
    "seriesCountByLabelValuePair": [
      {
        "name": "job=prometheus",
        "value": 425
      },
      {
        "name": "instance=localhost:9090",
        "value": 425
      }
    ]
  }
}
```

*New in v2.15*

### WAL Replay Stats

The following endpoint returns information about the WAL replay:

```
GET /api/v1/status/walreplay
```

**read**: The number of segments replayed so far.
**total**: The total number segments needed to be replayed.
**progress**: The progress of the replay (0 - 100%).
**state**: The state of the replay. Possible states:
- **waiting**: Waiting for the replay to start.
- **in progress**: The replay is in progress.
- **done**: The replay has finished.

```json
$ curl http://localhost:9090/api/v1/status/walreplay
{
  "status": "success",
  "data": {
    "min": 2,
    "max": 5,
    "current": 40,
    "state": "in progress"
  }
}
```

NOTE: This endpoint is available before the server has been marked ready and is updated in real time to facilitate monitoring the progress of the WAL replay.

*New in v2.28*

## TSDB Admin APIs
These are APIs that expose database functionalities for the advanced user. These APIs are not enabled unless the `--web.enable-admin-api` is set.

### Snapshot
Snapshot creates a snapshot of all current data into `snapshots/<datetime>-<rand>` under the TSDB's data directory and returns the directory as response.
It will optionally skip snapshotting data that is only present in the head block, and which has not yet been compacted to disk.

```
POST /api/v1/admin/tsdb/snapshot
PUT /api/v1/admin/tsdb/snapshot
```

URL query parameters:

- `skip_head=<bool>`: Skip data present in the head block. Optional.

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
DeleteSeries deletes data for a selection of series in a time range. The actual data still exists on disk and is cleaned up in future compactions or can be explicitly cleaned up by hitting the [Clean Tombstones](#clean-tombstones) endpoint.

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

NOTE: This endpoint marks samples from series as deleted, but will not necessarily prevent associated series metadata from still being returned in metadata queries for the affected time range (even after cleaning tombstones). The exact extent of metadata deletion is an implementation detail that may change in the future.

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

## Remote Write Receiver

Prometheus can be configured as a receiver for the Prometheus remote write
protocol. This is not considered an efficient way of ingesting samples. Use it
with caution for specific low-volume use cases. It is not suitable for
replacing the ingestion via scraping and turning Prometheus into a push-based
metrics collection system.

Enable the remote write receiver by setting
`--web.enable-remote-write-receiver`. When enabled, the remote write receiver
endpoint is `/api/v1/write`. Find more details [here](../storage.md#overview).

*New in v2.33*

## OTLP Receiver

Prometheus can be configured as a receiver for the OTLP Metrics protocol. This 
is not considered an efficient way of ingesting samples. Use it
with caution for specific low-volume use cases. It is not suitable for
replacing the ingestion via scraping.

Enable the OTLP receiver by the feature flag
`--enable-feature=otlp-write-receiver`. When enabled, the OTLP receiver
endpoint is `/api/v1/otlp/v1/metrics`.

*New in v2.47*