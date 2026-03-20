// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This file contains example request bodies and response data for OpenAPI documentation.
// Examples are included in the generated spec to provide realistic usage scenarios for API consumers.
package v1

import (
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

// Example builders for request bodies.

func queryPostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("simpleQuery", &base.Example{
		Summary: "Simple instant query",
		Value:   createYAMLNode(map[string]any{"query": "up"}),
	})

	examples.Set("queryWithTime", &base.Example{
		Summary: "Query with specific timestamp",
		Value: createYAMLNode(map[string]any{
			"query": "up{job=\"prometheus\"}",
			"time":  "2026-01-02T13:37:00.000Z",
		}),
	})

	examples.Set("queryWithLimit", &base.Example{
		Summary: "Query with limit and statistics",
		Value: createYAMLNode(map[string]any{
			"query": "rate(prometheus_http_requests_total{handler=\"/api/v1/query\"}[5m])",
			"limit": 100,
			"stats": "all",
		}),
	})

	return examples
}

// queryRangePostExamples returns examples for POST /query_range endpoint.
func queryRangePostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("basicRange", &base.Example{
		Summary: "Basic range query",
		Value: createYAMLNode(map[string]any{
			"query": "up",
			"start": "2026-01-02T12:37:00.000Z",
			"end":   "2026-01-02T13:37:00.000Z",
			"step":  "15s",
		}),
	})

	examples.Set("rateQuery", &base.Example{
		Summary: "Rate calculation over time range",
		Value: createYAMLNode(map[string]any{
			"query":   "rate(prometheus_http_requests_total{handler=\"/api/v1/query\"}[5m])",
			"start":   "2026-01-02T12:37:00.000Z",
			"end":     "2026-01-02T13:37:00.000Z",
			"step":    "30s",
			"timeout": "30s",
		}),
	})

	return examples
}

// queryExemplarsPostExamples returns examples for POST /query_exemplars endpoint.
func queryExemplarsPostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("basicExemplar", &base.Example{
		Summary: "Query exemplars for a metric",
		Value:   createYAMLNode(map[string]any{"query": "prometheus_http_requests_total"}),
	})

	examples.Set("exemplarWithTimeRange", &base.Example{
		Summary: "Exemplars within specific time range",
		Value: createYAMLNode(map[string]any{
			"query": "prometheus_http_requests_total{job=\"prometheus\"}",
			"start": "2026-01-02T12:37:00.000Z",
			"end":   "2026-01-02T13:37:00.000Z",
		}),
	})

	return examples
}

// formatQueryPostExamples returns examples for POST /format_query endpoint.
func formatQueryPostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("simpleFormat", &base.Example{
		Summary: "Format a simple query",
		Value:   createYAMLNode(map[string]any{"query": "up{job=\"prometheus\"}"}),
	})

	examples.Set("complexFormat", &base.Example{
		Summary: "Format a complex query",
		Value:   createYAMLNode(map[string]any{"query": "sum(rate(http_requests_total[5m])) by (job, status)"}),
	})

	return examples
}

// parseQueryPostExamples returns examples for POST /parse_query endpoint.
func parseQueryPostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("simpleParse", &base.Example{
		Summary: "Parse a simple query",
		Value:   createYAMLNode(map[string]any{"query": "up"}),
	})

	examples.Set("complexParse", &base.Example{
		Summary: "Parse a complex query",
		Value:   createYAMLNode(map[string]any{"query": "rate(http_requests_total{job=\"api\"}[5m])"}),
	})

	return examples
}

// labelsPostExamples returns examples for POST /labels endpoint.
func labelsPostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("allLabels", &base.Example{
		Summary: "Get all label names",
		Value:   createYAMLNode(map[string]any{}),
	})

	examples.Set("labelsWithTimeRange", &base.Example{
		Summary: "Get label names within time range",
		Value: createYAMLNode(map[string]any{
			"start": "2026-01-02T12:37:00.000Z",
			"end":   "2026-01-02T13:37:00.000Z",
		}),
	})

	examples.Set("labelsWithMatch", &base.Example{
		Summary: "Get label names matching series selector",
		Value: createYAMLNode(map[string]any{
			"match[]": []string{"up", "process_start_time_seconds{job=\"prometheus\"}"},
		}),
	})

	return examples
}

// seriesPostExamples returns examples for POST /series endpoint.
func seriesPostExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("seriesMatch", &base.Example{
		Summary: "Find series by label matchers",
		Value: createYAMLNode(map[string]any{
			"match[]": []string{"up"},
		}),
	})

	examples.Set("seriesWithTimeRange", &base.Example{
		Summary: "Find series with time range",
		Value: createYAMLNode(map[string]any{
			"match[]": []string{"up", "process_cpu_seconds_total{job=\"prometheus\"}"},
			"start":   "2026-01-02T12:37:00.000Z",
			"end":     "2026-01-02T13:37:00.000Z",
		}),
	})

	return examples
}

// Example builders for response bodies.

// queryResponseExamples returns examples for /query response.
func queryResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	vectorResult := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings("__name__", "up", "job", "prometheus", "instance", "demo.prometheus.io:9090"),
			T:      1767436620000,
			F:      1,
		},
		promql.Sample{
			Metric: labels.FromStrings("__name__", "up", "env", "demo", "job", "alertmanager", "instance", "demo.prometheus.io:9093"),
			T:      1767436620000,
			F:      1,
		},
	}

	examples.Set("vectorResult", &base.Example{
		Summary: "Instant vector query: up",
		Value:   vectorExample(vectorResult),
	})

	examples.Set("scalarResult", &base.Example{
		Summary: "Scalar query: scalar(42)",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "scalar",
				"result":     []any{1767436620, "42"},
			},
		}),
	})

	matrixResult := promql.Matrix{
		promql.Series{
			Metric: labels.FromStrings("__name__", "up", "job", "prometheus", "instance", "demo.prometheus.io:9090"),
			Floats: []promql.FPoint{
				{T: 1767436320000, F: 1},
				{T: 1767436620000, F: 1},
			},
		},
	}

	examples.Set("matrixResult", &base.Example{
		Summary: "Range vector query: up[5m]",
		Value:   matrixExample(matrixResult),
	})

	// TODO: Add native histogram example.

	return examples
}

// queryRangeResponseExamples returns examples for /query_range response.
func queryRangeResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	matrixResult := promql.Matrix{
		promql.Series{
			Metric: labels.FromStrings("__name__", "up", "job", "prometheus", "instance", "demo.prometheus.io:9090"),
			Floats: []promql.FPoint{
				{T: 1767433020000, F: 1},
				{T: 1767434820000, F: 1},
				{T: 1767436620000, F: 1},
			},
		},
	}

	examples.Set("matrixResult", &base.Example{
		Summary: "Range query: rate(prometheus_http_requests_total[5m])",
		Value:   matrixExample(matrixResult),
	})

	// TODO: Add native histogram example.

	return examples
}

// labelsResponseExamples returns examples for /labels response.
func labelsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("labelNames", &base.Example{
		Summary: "List of label names",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": []string{
				"__name__", "active", "address", "alertmanager", "alertname", "alertstate",
				"backend", "branch", "code", "collector", "component", "device",
				"env", "endpoint", "fstype", "handler", "instance", "job",
				"le", "method", "mode", "name",
			},
		}),
	})

	return examples
}

// seriesResponseExamples returns examples for /series response.
func seriesResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("seriesList", &base.Example{
		Summary: "List of series matching the selector",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": []map[string]string{
				{
					"__name__": "up",
					"env":      "demo",
					"instance": "demo.prometheus.io:8080",
					"job":      "cadvisor",
				},
				{
					"__name__": "up",
					"env":      "demo",
					"instance": "demo.prometheus.io:9093",
					"job":      "alertmanager",
				},
				{
					"__name__": "up",
					"env":      "demo",
					"instance": "demo.prometheus.io:9100",
					"job":      "node",
				},
				{
					"__name__": "up",
					"instance": "demo.prometheus.io:3000",
					"job":      "grafana",
				},
				{
					"__name__": "up",
					"instance": "demo.prometheus.io:8996",
					"job":      "random",
				},
			},
		}),
	})

	return examples
}

// targetsResponseExamples returns examples for /targets response.
func targetsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("targetsList", &base.Example{
		Summary: "Active and dropped targets",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"activeTargets": []map[string]any{
					{
						"discoveredLabels": map[string]string{
							"__address__":      "demo.prometheus.io:9093",
							"__meta_filepath":  "/etc/prometheus/file_sd/alertmanager.yml",
							"__metrics_path__": "/metrics",
							"__scheme__":       "http",
							"env":              "demo",
							"job":              "alertmanager",
						},
						"labels": map[string]string{
							"env":      "demo",
							"instance": "demo.prometheus.io:9093",
							"job":      "alertmanager",
						},
						"scrapePool":         "alertmanager",
						"scrapeUrl":          "http://demo.prometheus.io:9093/metrics",
						"globalUrl":          "http://demo.prometheus.io:9093/metrics",
						"lastError":          "",
						"lastScrape":         "2026-01-02T13:36:40.200Z",
						"lastScrapeDuration": 0.006576866,
						"health":             "up",
						"scrapeInterval":     "15s",
						"scrapeTimeout":      "10s",
					},
				},
				"droppedTargets": []map[string]any{},
				"droppedTargetCounts": map[string]int{
					"alertmanager": 0,
					"blackbox":     0,
					"caddy":        0,
					"cadvisor":     0,
					"grafana":      0,
					"node":         0,
					"prometheus":   0,
					"random":       0,
				},
			},
		}),
	})

	return examples
}

// rulesResponseExamples returns examples for /rules response.
func rulesResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("ruleGroups", &base.Example{
		Summary: "Alerting and recording rules",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"groups": []map[string]any{
					{
						"name":     "ansible managed alert rules",
						"file":     "/etc/prometheus/rules/ansible_managed.yml",
						"interval": 15,
						"limit":    0,
						"rules": []map[string]any{
							{
								"state":          "firing",
								"name":           "Watchdog",
								"query":          "vector(1)",
								"duration":       600,
								"keepFiringFor":  0,
								"labels":         map[string]string{"severity": "warning"},
								"annotations":    map[string]string{"description": "This is an alert meant to ensure that the entire alerting pipeline is functional. This alert is always firing, therefore it should always be firing in Alertmanager and always fire against a receiver. There are integrations with various notification mechanisms that send a notification when this alert is not firing. For example the \"DeadMansSnitch\" integration in PagerDuty.", "summary": "Ensure entire alerting pipeline is functional"},
								"health":         "ok",
								"evaluationTime": 0.000356688,
								"lastEvaluation": "2026-01-02T13:36:56.874Z",
								"type":           "alerting",
							},
						},
						"evaluationTime": 0.000561635,
						"lastEvaluation": "2026-01-02T13:36:56.874Z",
					},
				},
			},
		}),
	})

	return examples
}

// alertsResponseExamples returns examples for /alerts response.
func alertsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("activeAlerts", &base.Example{
		Summary: "Currently active alerts",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"alerts": []map[string]any{
					{
						"labels": map[string]string{
							"alertname": "Watchdog",
							"severity":  "warning",
						},
						"annotations": map[string]string{
							"description": "This is an alert meant to ensure that the entire alerting pipeline is functional. This alert is always firing, therefore it should always be firing in Alertmanager and always fire against a receiver. There are integrations with various notification mechanisms that send a notification when this alert is not firing. For example the \"DeadMansSnitch\" integration in PagerDuty.",
							"summary":     "Ensure entire alerting pipeline is functional",
						},
						"state":    "firing",
						"activeAt": "2026-01-02T13:30:00.000Z",
						"value":    "1e+00",
					},
				},
			},
		}),
	})

	return examples
}

// queryExemplarsResponseExamples returns examples for /query_exemplars response.
func queryExemplarsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("exemplarsResult", &base.Example{
		Summary: "Exemplars for a metric with trace IDs",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": []map[string]any{
				{
					"seriesLabels": map[string]string{
						"__name__": "http_requests_total",
						"job":      "api-server",
						"method":   "GET",
					},
					"exemplars": []map[string]any{
						{
							"labels": map[string]string{
								"traceID": "abc123def456",
							},
							"value":     "1.5",
							"timestamp": 1689956451.781,
						},
					},
				},
			},
		}),
	})

	return examples
}

// formatQueryResponseExamples returns examples for /format_query response.
func formatQueryResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("formattedQuery", &base.Example{
		Summary: "Formatted PromQL query",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data":   "sum by(job, status) (rate(http_requests_total[5m]))",
		}),
	})

	return examples
}

// parseQueryResponseExamples returns examples for /parse_query response.
func parseQueryResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("parsedQuery", &base.Example{
		Summary: "Parsed PromQL expression tree",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
			},
		}),
	})

	return examples
}

// labelValuesResponseExamples returns examples for /label/{name}/values response.
func labelValuesResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("labelValues", &base.Example{
		Summary: "List of values for a label",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data":   []string{"alertmanager", "blackbox", "caddy", "cadvisor", "grafana", "node", "prometheus", "random"},
		}),
	})

	return examples
}

// metadataResponseExamples returns examples for /metadata response.
func metadataResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("metricMetadata", &base.Example{
		Summary: "Metadata for metrics",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string][]map[string]any{
				"prometheus_rule_group_iterations_missed_total": {
					{
						"type": "counter",
						"help": "The total number of rule group evaluations missed due to slow rule group evaluation.",
						"unit": "",
					},
				},
				"prometheus_sd_updates_total": {
					{
						"type": "counter",
						"help": "Total number of update events sent to the SD consumers.",
						"unit": "",
					},
				},
				"go_gc_stack_starting_size_bytes": {
					{
						"type": "gauge",
						"help": "The stack size of new goroutines. Sourced from /gc/stack/starting-size:bytes.",
						"unit": "",
					},
				},
			},
		}),
	})

	return examples
}

// scrapePoolsResponseExamples returns examples for /scrape_pools response.
func scrapePoolsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("scrapePoolsList", &base.Example{
		Summary: "List of scrape pool names",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"scrapePools": []string{"alertmanager", "blackbox", "caddy", "cadvisor", "grafana", "node", "prometheus", "random"},
			},
		}),
	})

	return examples
}

// targetsMetadataResponseExamples returns examples for /targets/metadata response.
func targetsMetadataResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("targetMetadata", &base.Example{
		Summary: "Metadata for targets",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": []map[string]any{
				{
					"target": map[string]string{
						"instance": "localhost:9090",
						"job":      "prometheus",
					},
					"type":   "gauge",
					"help":   "The current health status of the target",
					"unit":   "",
					"metric": "up",
				},
			},
		}),
	})

	return examples
}

// targetsRelabelStepsResponseExamples returns examples for /targets/relabel_steps response.
func targetsRelabelStepsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("relabelSteps", &base.Example{
		Summary: "Relabel steps for a target",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"steps": []map[string]any{
					{
						"rule": map[string]any{
							"source_labels": []string{"__address__"},
							"target_label":  "instance",
							"action":        "replace",
							"regex":         "(.*)",
							"replacement":   "$1",
						},
						"output": map[string]string{
							"__address__": "localhost:9090",
							"instance":    "localhost:9090",
							"job":         "prometheus",
						},
						"keep": true,
					},
				},
			},
		}),
	})

	return examples
}

// alertmanagersResponseExamples returns examples for /alertmanagers response.
func alertmanagersResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("alertmanagerDiscovery", &base.Example{
		Summary: "Alertmanager discovery results",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"activeAlertmanagers": []map[string]any{
					{
						"url": "http://demo.prometheus.io:9093/api/v2/alerts",
					},
				},
				"droppedAlertmanagers": []map[string]any{},
			},
		}),
	})

	return examples
}

// statusConfigResponseExamples returns examples for /status/config response.
func statusConfigResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("configYAML", &base.Example{
		Summary: "Prometheus configuration",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  evaluation_interval: 15s\n  external_labels:\n    environment: demo-prometheus-io\nalerting:\n  alertmanagers:\n  - scheme: http\n    static_configs:\n    - targets:\n      - demo.prometheus.io:9093\nrule_files:\n- /etc/prometheus/rules/*.yml\n",
			},
		}),
	})

	return examples
}

// statusRuntimeInfoResponseExamples returns examples for /status/runtimeinfo response.
func statusRuntimeInfoResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("runtimeInfo", &base.Example{
		Summary: "Runtime information",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"startTime":           "2026-01-01T13:37:00.000Z",
				"CWD":                 "/",
				"hostname":            "demo-prometheus-io",
				"serverTime":          "2026-01-02T13:37:00.000Z",
				"reloadConfigSuccess": true,
				"lastConfigTime":      "2026-01-01T13:37:00.000Z",
				"corruptionCount":     0,
				"goroutineCount":      88,
				"GOMAXPROCS":          2,
				"GOMEMLIMIT":          int64(3703818240),
				"GOGC":                "75",
				"GODEBUG":             "",
				"storageRetention":    "31d",
			},
		}),
	})

	return examples
}

// statusBuildInfoResponseExamples returns examples for /status/buildinfo response.
func statusBuildInfoResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("buildInfo", &base.Example{
		Summary: "Build information",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"version":   "3.7.3",
				"revision":  "0a41f0000705c69ab8e0f9a723fc73e39ed62b07",
				"branch":    "HEAD",
				"buildUser": "root@08c890a84441",
				"buildDate": "20251030-07:26:10",
				"goVersion": "go1.25.3",
			},
		}),
	})

	return examples
}

// statusFlagsResponseExamples returns examples for /status/flags response.
func statusFlagsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("flags", &base.Example{
		Summary: "Command-line flags",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]string{
				"agent": "false",
				"alertmanager.notification-queue-capacity": "10000",
				"config.file":                 "/etc/prometheus/prometheus.yml",
				"enable-feature":              "exemplar-storage,native-histograms",
				"query.max-concurrency":       "20",
				"query.timeout":               "2m",
				"storage.tsdb.path":           "/prometheus",
				"storage.tsdb.retention.time": "15d",
				"web.console.libraries":       "/usr/share/prometheus/console_libraries",
				"web.console.templates":       "/usr/share/prometheus/consoles",
				"web.enable-admin-api":        "true",
				"web.enable-lifecycle":        "true",
				"web.listen-address":          "0.0.0.0:9090",
				"web.page-title":              "Prometheus Time Series Collection and Processing Server",
			},
		}),
	})

	return examples
}

// statusTSDBResponseExamples returns examples for /status/tsdb response.
func statusTSDBResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("tsdbStats", &base.Example{
		Summary: "TSDB statistics",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"headStats": map[string]any{
					"numSeries":     9925,
					"numLabelPairs": 2512,
					"chunkCount":    37525,
					"minTime":       int64(1767362400712),
					"maxTime":       int64(1767436620000),
				},
				"seriesCountByMetricName": []map[string]any{
					{
						"name":  "up",
						"value": 100,
					},
					{
						"name":  "http_requests_total",
						"value": 500,
					},
				},
				"labelValueCountByLabelName": []map[string]any{
					{
						"name":  "__name__",
						"value": 5,
					},
					{
						"name":  "job",
						"value": 3,
					},
				},
				"memoryInBytesByLabelName": []map[string]any{
					{
						"name":  "__name__",
						"value": 1024,
					},
					{
						"name":  "job",
						"value": 512,
					},
				},
				"seriesCountByLabelValuePair": []map[string]any{
					{
						"name":  "job=prometheus",
						"value": 100,
					},
					{
						"name":  "instance=localhost:9090",
						"value": 100,
					},
				},
			},
		}),
	})

	return examples
}

// statusTSDBBlocksResponseExamples returns examples for /status/tsdb/blocks response.
func statusTSDBBlocksResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("tsdbBlocks", &base.Example{
		Summary: "TSDB block information",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"blocks": []map[string]any{
					{
						"ulid":    "01KC4D6GXQA4CRHYKV78NEBVAE",
						"minTime": int64(1764568801099),
						"maxTime": int64(1764763200000),
						"stats": map[string]any{
							"numSamples": 129505582,
							"numSeries":  10661,
							"numChunks":  1073962,
						},
						"compaction": map[string]any{
							"level": 4,
							"sources": []string{
								"01KBCJ7TR8A4QAJ3AA1J651P5S",
								"01KBCS3J0E34567YPB8Y5W0E24",
								"01KBCZZ9KRTYGG3E7HVQFGC3S3",
							},
						},
						"version": 1,
					},
				},
			},
		}),
	})

	return examples
}

// statusWALReplayResponseExamples returns examples for /status/walreplay response.
func statusWALReplayResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("walReplay", &base.Example{
		Summary: "WAL replay status",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"min":     3209,
				"max":     3214,
				"current": 3214,
			},
		}),
	})

	return examples
}

// deleteSeriesResponseExamples returns examples for /admin/tsdb/delete_series response.
func deleteSeriesResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("deletionSuccess", &base.Example{
		Summary: "Successful series deletion",
		Value: createYAMLNode(map[string]any{
			"status": "success",
		}),
	})

	return examples
}

// cleanTombstonesResponseExamples returns examples for /admin/tsdb/clean_tombstones response.
func cleanTombstonesResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("tombstonesCleaned", &base.Example{
		Summary: "Tombstones cleaned successfully",
		Value: createYAMLNode(map[string]any{
			"status": "success",
		}),
	})

	return examples
}

// seriesDeleteResponseExamples returns examples for DELETE /series response.
func seriesDeleteResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("seriesDeleted", &base.Example{
		Summary: "Series marked for deletion",
		Value: createYAMLNode(map[string]any{
			"status": "success",
		}),
	})

	return examples
}

// snapshotResponseExamples returns examples for /admin/tsdb/snapshot response.
func snapshotResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("snapshotCreated", &base.Example{
		Summary: "Snapshot created successfully",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"name": "20260102T133700Z-a1b2c3d4e5f67890",
			},
		}),
	})

	return examples
}

// notificationsResponseExamples returns examples for /notifications response.
func notificationsResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("notifications", &base.Example{
		Summary: "Server notifications",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data": []map[string]any{
				{
					"text":   "Configuration reload has failed.",
					"date":   "2026-01-02T16:14:50.046Z",
					"active": true,
				},
			},
		}),
	})

	return examples
}

// notificationLiveExamples provides example SSE messages for the live notifications endpoint.
func notificationLiveExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("activeNotification", &base.Example{
		Summary:     "Active notification SSE message",
		Description: "An SSE message containing an active server notification.",
		Value: createYAMLNode(map[string]any{
			"data": "{\"text\":\"Configuration reload has failed.\",\"date\":\"2026-01-02T16:14:50.046Z\",\"active\":true}",
		}),
	})

	return examples
}

// featuresResponseExamples returns examples for /features response.
func featuresResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("enabledFeatures", &base.Example{
		Summary: "Enabled feature flags",
		Value: createYAMLNode(map[string]any{
			"status": "success",
			"data":   []string{"exemplar-storage", "remote-write-receiver"},
		}),
	})

	return examples
}

// errorResponseExamples returns examples for error responses.
func errorResponseExamples() *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()

	examples.Set("tsdbNotReady", &base.Example{
		Summary: "TSDB not ready",
		Value: createYAMLNode(map[string]any{
			"status":    "error",
			"errorType": "internal",
			"error":     "TSDB not ready",
		}),
	})

	return examples
}
