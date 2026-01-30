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

// This file defines all API path specifications including parameters, request bodies,
// and response schemas. Each path definition corresponds to an endpoint registered in api.go.
package v1

import (
	"time"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// Path definition methods for API endpoints.

func (*OpenAPIBuilder) queryPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("limit", "The maximum number of metrics to return.", false, integerSchema(), []example{{"example", 100}}),
		queryParamWithExample("time", "The evaluation timestamp (optional, defaults to current time).", false, timestampSchema(), timestampExamples(exampleTime)),
		queryParamWithExample("query", "The PromQL query to execute.", true, stringSchema(), []example{{"example", "up"}}),
		queryParamWithExample("timeout", "Evaluation timeout. Optional. Defaults to and is capped by the value of the -query.timeout flag.", false, stringSchema(), []example{{"example", "30s"}}),
		queryParamWithExample("lookback_delta", "Override the lookback period for this query. Optional.", false, stringSchema(), []example{{"example", "5m"}}),
		queryParamWithExample("stats", "When provided, include query statistics in the response. The special value 'all' enables more comprehensive statistics.", false, stringSchema(), []example{{"example", "all"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "query",
			Summary:     "Evaluate an instant query",
			Tags:        []string{"query"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("QueryOutputBody", queryResponseExamples(), errorResponseExamples(), "Query executed successfully.", "Error executing query."),
		},
		Post: &v3.Operation{
			OperationId: "query-post",
			Summary:     "Evaluate an instant query",
			Tags:        []string{"query"},
			RequestBody: formRequestBodyWithExamples("QueryPostInputBody", queryPostExamples(), "Submit an instant query. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("QueryOutputBody", queryResponseExamples(), errorResponseExamples(), "Instant query executed successfully.", "Error executing instant query."),
		},
	}
}

func (*OpenAPIBuilder) queryRangePath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("limit", "The maximum number of metrics to return.", false, integerSchema(), []example{{"example", 100}}),
		queryParamWithExample("start", "The start time of the query.", true, timestampSchema(), timestampExamples(exampleTime.Add(-1*time.Hour))),
		queryParamWithExample("end", "The end time of the query.", true, timestampSchema(), timestampExamples(exampleTime)),
		queryParamWithExample("step", "The step size of the query.", true, stringSchema(), []example{{"example", "15s"}}),
		queryParamWithExample("query", "The query to execute.", true, stringSchema(), []example{{"example", "rate(prometheus_http_requests_total{handler=\"/api/v1/query\"}[5m])"}}),
		queryParamWithExample("timeout", "Evaluation timeout. Optional. Defaults to and is capped by the value of the -query.timeout flag.", false, stringSchema(), []example{{"example", "30s"}}),
		queryParamWithExample("lookback_delta", "Override the lookback period for this query. Optional.", false, stringSchema(), []example{{"example", "5m"}}),
		queryParamWithExample("stats", "When provided, include query statistics in the response. The special value 'all' enables more comprehensive statistics.", false, stringSchema(), []example{{"example", "all"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "query-range",
			Summary:     "Evaluate a range query",
			Tags:        []string{"query"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("QueryRangeOutputBody", queryRangeResponseExamples(), errorResponseExamples(), "Range query executed successfully.", "Error executing range query."),
		},
		Post: &v3.Operation{
			OperationId: "query-range-post",
			Summary:     "Evaluate a range query",
			Tags:        []string{"query"},
			RequestBody: formRequestBodyWithExamples("QueryRangePostInputBody", queryRangePostExamples(), "Submit a range query. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("QueryRangeOutputBody", queryRangeResponseExamples(), errorResponseExamples(), "Range query executed successfully.", "Error executing range query."),
		},
	}
}

func (*OpenAPIBuilder) queryExemplarsPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("start", "Start timestamp for exemplars query.", false, timestampSchema(), timestampExamples(exampleTime.Add(-1*time.Hour))),
		queryParamWithExample("end", "End timestamp for exemplars query.", false, timestampSchema(), timestampExamples(exampleTime)),
		queryParamWithExample("query", "PromQL query to extract exemplars for.", true, stringSchema(), []example{{"example", "prometheus_http_requests_total"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "query-exemplars",
			Summary:     "Query exemplars",
			Tags:        []string{"query"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("QueryExemplarsOutputBody", queryExemplarsResponseExamples(), errorResponseExamples(), "Exemplars retrieved successfully.", "Error retrieving exemplars."),
		},
		Post: &v3.Operation{
			OperationId: "query-exemplars-post",
			Summary:     "Query exemplars",
			Tags:        []string{"query"},
			RequestBody: formRequestBodyWithExamples("QueryExemplarsPostInputBody", queryExemplarsPostExamples(), "Submit an exemplars query. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("QueryExemplarsOutputBody", queryExemplarsResponseExamples(), errorResponseExamples(), "Exemplars query completed successfully.", "Error processing exemplars query."),
		},
	}
}

func (*OpenAPIBuilder) formatQueryPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("query", "PromQL expression to format.", true, stringSchema(), []example{{"example", "sum(rate(http_requests_total[5m])) by (job)"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "format-query",
			Summary:     "Format a PromQL query",
			Tags:        []string{"query"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("FormatQueryOutputBody", formatQueryResponseExamples(), errorResponseExamples(), "Query formatted successfully.", "Error formatting query."),
		},
		Post: &v3.Operation{
			OperationId: "format-query-post",
			Summary:     "Format a PromQL query",
			Tags:        []string{"query"},
			RequestBody: formRequestBodyWithExamples("FormatQueryPostInputBody", formatQueryPostExamples(), "Submit a PromQL query to format. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("FormatQueryOutputBody", formatQueryResponseExamples(), errorResponseExamples(), "Query formatting completed successfully.", "Error formatting query."),
		},
	}
}

func (*OpenAPIBuilder) parseQueryPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("query", "PromQL expression to parse.", true, stringSchema(), []example{{"example", "up{job=\"prometheus\"}"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "parse-query",
			Summary:     "Parse a PromQL query",
			Tags:        []string{"query"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("ParseQueryOutputBody", parseQueryResponseExamples(), errorResponseExamples(), "Query parsed successfully.", "Error parsing query."),
		},
		Post: &v3.Operation{
			OperationId: "parse-query-post",
			Summary:     "Parse a PromQL query",
			Tags:        []string{"query"},
			RequestBody: formRequestBodyWithExamples("ParseQueryPostInputBody", parseQueryPostExamples(), "Submit a PromQL query to parse. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("ParseQueryOutputBody", parseQueryResponseExamples(), errorResponseExamples(), "Query parsed successfully via POST.", "Error parsing query via POST."),
		},
	}
}

func (*OpenAPIBuilder) labelsPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("start", "Start timestamp for label names query.", false, timestampSchema(), timestampExamples(exampleTime.Add(-1*time.Hour))),
		queryParamWithExample("end", "End timestamp for label names query.", false, timestampSchema(), timestampExamples(exampleTime)),
		queryParamWithExample("match[]", "Series selector argument.", false, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"{job=\"prometheus\"}"}}}),
		queryParamWithExample("limit", "Maximum number of label names to return.", false, integerSchema(), []example{{"example", 100}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "labels",
			Summary:     "Get label names",
			Tags:        []string{"labels"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("LabelsOutputBody", labelsResponseExamples(), errorResponseExamples(), "Label names retrieved successfully.", "Error retrieving label names."),
		},
		Post: &v3.Operation{
			OperationId: "labels-post",
			Summary:     "Get label names",
			Tags:        []string{"labels"},
			RequestBody: formRequestBodyWithExamples("LabelsPostInputBody", labelsPostExamples(), "Submit a label names query. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("LabelsOutputBody", labelsResponseExamples(), errorResponseExamples(), "Label names retrieved successfully via POST.", "Error retrieving label names via POST."),
		},
	}
}

func (*OpenAPIBuilder) labelValuesPath() *v3.PathItem {
	params := []*v3.Parameter{
		pathParam("name", "Label name.", stringSchema()),
		queryParamWithExample("start", "Start timestamp for label values query.", false, timestampSchema(), timestampExamples(exampleTime.Add(-1*time.Hour))),
		queryParamWithExample("end", "End timestamp for label values query.", false, timestampSchema(), timestampExamples(exampleTime)),
		queryParamWithExample("match[]", "Series selector argument.", false, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"{job=\"prometheus\"}"}}}),
		queryParamWithExample("limit", "Maximum number of label values to return.", false, integerSchema(), []example{{"example", 1000}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "label-values",
			Summary:     "Get label values",
			Tags:        []string{"labels"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("LabelValuesOutputBody", labelValuesResponseExamples(), errorResponseExamples(), "Label values retrieved successfully.", "Error retrieving label values."),
		},
	}
}

func (*OpenAPIBuilder) seriesPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("start", "Start timestamp for series query.", false, timestampSchema(), timestampExamples(exampleTime.Add(-1*time.Hour))),
		queryParamWithExample("end", "End timestamp for series query.", false, timestampSchema(), timestampExamples(exampleTime)),
		queryParamWithExample("match[]", "Series selector argument.", true, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"{job=\"prometheus\"}"}}}),
		queryParamWithExample("limit", "Maximum number of series to return.", false, integerSchema(), []example{{"example", 100}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "series",
			Summary:     "Find series by label matchers",
			Tags:        []string{"series"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("SeriesOutputBody", seriesResponseExamples(), errorResponseExamples(), "Series returned matching the provided label matchers.", "Error retrieving series."),
		},
		Post: &v3.Operation{
			OperationId: "series-post",
			Summary:     "Find series by label matchers",
			Tags:        []string{"series"},
			RequestBody: formRequestBodyWithExamples("SeriesPostInputBody", seriesPostExamples(), "Submit a series query. This endpoint accepts the same parameters as the GET version."),
			Responses:   responsesWithErrorExamples("SeriesOutputBody", seriesResponseExamples(), errorResponseExamples(), "Series returned matching the provided label matchers via POST.", "Error retrieving series via POST."),
		},
		Delete: &v3.Operation{
			OperationId: "delete-series",
			Summary:     "Delete series",
			Description: "Delete series matching selectors. Note: This is deprecated, use POST /admin/tsdb/delete_series instead.",
			Tags:        []string{"series"},
			Responses:   responsesWithErrorExamples("SeriesDeleteOutputBody", seriesDeleteResponseExamples(), errorResponseExamples(), "Series marked for deletion.", "Error deleting series."),
		},
	}
}

func (*OpenAPIBuilder) metadataPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("limit", "The maximum number of metrics to return.", false, integerSchema(), []example{{"example", 100}}),
		queryParamWithExample("limit_per_metric", "The maximum number of metadata entries per metric.", false, integerSchema(), []example{{"example", 10}}),
		queryParamWithExample("metric", "A metric name to filter metadata for.", false, stringSchema(), []example{{"example", "http_requests_total"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-metadata",
			Summary:     "Get metadata",
			Tags:        []string{"metadata"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("MetadataOutputBody", metadataResponseExamples(), errorResponseExamples(), "Metric metadata retrieved successfully.", "Error retrieving metadata."),
		},
	}
}

func (*OpenAPIBuilder) scrapePoolsPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-scrape-pools",
			Summary:     "Get scrape pools",
			Tags:        []string{"targets"},
			Responses:   responsesWithErrorExamples("ScrapePoolsOutputBody", scrapePoolsResponseExamples(), errorResponseExamples(), "Scrape pools retrieved successfully.", "Error retrieving scrape pools."),
		},
	}
}

func (*OpenAPIBuilder) targetsPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("scrapePool", "Filter targets by scrape pool name.", false, stringSchema(), []example{{"example", "prometheus"}}),
		queryParamWithExample("state", "Filter by state: active, dropped, or any.", false, stringSchema(), []example{{"example", "active"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-targets",
			Summary:     "Get targets",
			Tags:        []string{"targets"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("TargetsOutputBody", targetsResponseExamples(), errorResponseExamples(), "Target discovery information retrieved successfully.", "Error retrieving targets."),
		},
	}
}

func (*OpenAPIBuilder) targetsMetadataPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("match_target", "Label selector to filter targets.", false, stringSchema(), []example{{"example", "{job=\"prometheus\"}"}}),
		queryParamWithExample("metric", "Metric name to retrieve metadata for.", false, stringSchema(), []example{{"example", "http_requests_total"}}),
		queryParamWithExample("limit", "Maximum number of targets to match.", false, integerSchema(), []example{{"example", 10}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-targets-metadata",
			Summary:     "Get targets metadata",
			Tags:        []string{"targets"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("TargetMetadataOutputBody", targetsMetadataResponseExamples(), errorResponseExamples(), "Target metadata retrieved successfully.", "Error retrieving target metadata."),
		},
	}
}

func (*OpenAPIBuilder) targetsRelabelStepsPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("scrapePool", "Name of the scrape pool.", true, stringSchema(), []example{{"example", "prometheus"}}),
		queryParamWithExample("labels", "JSON-encoded labels to apply relabel rules to.", true, stringSchema(), []example{{"example", "{\"__address__\":\"localhost:9090\",\"job\":\"prometheus\"}"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-targets-relabel-steps",
			Summary:     "Get targets relabel steps",
			Tags:        []string{"targets"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("TargetRelabelStepsOutputBody", targetsRelabelStepsResponseExamples(), errorResponseExamples(), "Relabel steps retrieved successfully.", "Error retrieving relabel steps."),
		},
	}
}

func (*OpenAPIBuilder) rulesPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("type", "Filter by rule type: alert or record.", false, stringSchema(), []example{{"example", "alert"}}),
		queryParamWithExample("rule_name[]", "Filter by rule name.", false, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"HighErrorRate"}}}),
		queryParamWithExample("rule_group[]", "Filter by rule group name.", false, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"example_alerts"}}}),
		queryParamWithExample("file[]", "Filter by file path.", false, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"/etc/prometheus/rules.yml"}}}),
		queryParamWithExample("match[]", "Label matchers to filter rules.", false, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"{severity=\"critical\"}"}}}),
		queryParamWithExample("exclude_alerts", "Exclude active alerts from response.", false, stringSchema(), []example{{"example", "false"}}),
		queryParamWithExample("group_limit", "Maximum number of rule groups to return.", false, integerSchema(), []example{{"example", 100}}),
		queryParamWithExample("group_next_token", "Pagination token for next page.", false, stringSchema(), []example{{"example", "abc123"}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "rules",
			Summary:     "Get alerting and recording rules",
			Tags:        []string{"rules"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("RulesOutputBody", rulesResponseExamples(), errorResponseExamples(), "Rules retrieved successfully.", "Error retrieving rules."),
		},
	}
}

func (*OpenAPIBuilder) alertsPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "alerts",
			Summary:     "Get active alerts",
			Tags:        []string{"alerts"},
			Responses:   responsesWithErrorExamples("AlertsOutputBody", alertsResponseExamples(), errorResponseExamples(), "Active alerts retrieved successfully.", "Error retrieving alerts."),
		},
	}
}

func (*OpenAPIBuilder) alertmanagersPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "alertmanagers",
			Summary:     "Get Alertmanager discovery",
			Tags:        []string{"alerts"},
			Responses:   responsesWithErrorExamples("AlertmanagersOutputBody", alertmanagersResponseExamples(), errorResponseExamples(), "Alertmanager targets retrieved successfully.", "Error retrieving Alertmanager targets."),
		},
	}
}

func (*OpenAPIBuilder) statusConfigPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-status-config",
			Summary:     "Get status config",
			Tags:        []string{"status"},
			Responses:   responsesWithErrorExamples("StatusConfigOutputBody", statusConfigResponseExamples(), errorResponseExamples(), "Configuration retrieved successfully.", "Error retrieving configuration."),
		},
	}
}

func (*OpenAPIBuilder) statusRuntimeInfoPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-status-runtimeinfo",
			Summary:     "Get status runtimeinfo",
			Tags:        []string{"status"},
			Responses:   responsesWithErrorExamples("StatusRuntimeInfoOutputBody", statusRuntimeInfoResponseExamples(), errorResponseExamples(), "Runtime information retrieved successfully.", "Error retrieving runtime information."),
		},
	}
}

func (*OpenAPIBuilder) statusBuildInfoPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-status-buildinfo",
			Summary:     "Get status buildinfo",
			Tags:        []string{"status"},
			Responses:   responsesWithErrorExamples("StatusBuildInfoOutputBody", statusBuildInfoResponseExamples(), errorResponseExamples(), "Build information retrieved successfully.", "Error retrieving build information."),
		},
	}
}

func (*OpenAPIBuilder) statusFlagsPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-status-flags",
			Summary:     "Get status flags",
			Tags:        []string{"status"},
			Responses:   responsesWithErrorExamples("StatusFlagsOutputBody", statusFlagsResponseExamples(), errorResponseExamples(), "Command-line flags retrieved successfully.", "Error retrieving flags."),
		},
	}
}

func (*OpenAPIBuilder) statusTSDBPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("limit", "The maximum number of items to return per category.", false, integerSchema(), []example{{"example", 10}}),
	}
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "status-tsdb",
			Summary:     "Get TSDB status",
			Tags:        []string{"status"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("StatusTSDBOutputBody", statusTSDBResponseExamples(), errorResponseExamples(), "TSDB status retrieved successfully.", "Error retrieving TSDB status."),
		},
	}
}

func (*OpenAPIBuilder) statusTSDBBlocksPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "status-tsdb-blocks",
			Summary:     "Get TSDB blocks information",
			Tags:        []string{"status"},
			Responses:   responsesWithErrorExamples("StatusTSDBBlocksOutputBody", statusTSDBBlocksResponseExamples(), errorResponseExamples(), "TSDB blocks information retrieved successfully.", "Error retrieving TSDB blocks."),
		},
	}
}

func (*OpenAPIBuilder) statusWALReplayPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-status-walreplay",
			Summary:     "Get status walreplay",
			Tags:        []string{"status"},
			Responses:   responsesWithErrorExamples("StatusWALReplayOutputBody", statusWALReplayResponseExamples(), errorResponseExamples(), "WAL replay status retrieved successfully.", "Error retrieving WAL replay status."),
		},
	}
}

func (*OpenAPIBuilder) adminDeleteSeriesPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("match[]", "Series selectors to identify series to delete.", true, base.CreateSchemaProxy(&base.Schema{
			Type:  []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		}), []example{{"example", []string{"{__name__=~\"test.*\"}"}}}),
		queryParamWithExample("start", "Start timestamp for deletion.", false, timestampSchema(), timestampExamples(exampleTime.Add(-1*time.Hour))),
		queryParamWithExample("end", "End timestamp for deletion.", false, timestampSchema(), timestampExamples(exampleTime)),
	}
	return &v3.PathItem{
		Post: &v3.Operation{
			OperationId: "deleteSeriesPost",
			Summary:     "Delete series matching selectors",
			Description: "Deletes data for a selection of series in a time range.",
			Tags:        []string{"admin"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("DeleteSeriesOutputBody", deleteSeriesResponseExamples(), errorResponseExamples(), "Series deleted successfully.", "Error deleting series."),
		},
		Put: &v3.Operation{
			OperationId: "deleteSeriesPut",
			Summary:     "Delete series matching selectors via PUT",
			Description: "Deletes data for a selection of series in a time range using PUT method.",
			Tags:        []string{"admin"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("DeleteSeriesOutputBody", deleteSeriesResponseExamples(), errorResponseExamples(), "Series deleted successfully via PUT.", "Error deleting series via PUT."),
		},
	}
}

func (*OpenAPIBuilder) adminCleanTombstonesPath() *v3.PathItem {
	return &v3.PathItem{
		Post: &v3.Operation{
			OperationId: "cleanTombstonesPost",
			Summary:     "Clean tombstones in the TSDB",
			Description: "Removes deleted data from disk and cleans up existing tombstones.",
			Tags:        []string{"admin"},
			Responses:   responsesWithErrorExamples("CleanTombstonesOutputBody", cleanTombstonesResponseExamples(), errorResponseExamples(), "Tombstones cleaned successfully.", "Error cleaning tombstones."),
		},
		Put: &v3.Operation{
			OperationId: "cleanTombstonesPut",
			Summary:     "Clean tombstones in the TSDB via PUT",
			Description: "Removes deleted data from disk and cleans up existing tombstones using PUT method.",
			Tags:        []string{"admin"},
			Responses:   responsesWithErrorExamples("CleanTombstonesOutputBody", cleanTombstonesResponseExamples(), errorResponseExamples(), "Tombstones cleaned successfully via PUT.", "Error cleaning tombstones via PUT."),
		},
	}
}

func (*OpenAPIBuilder) adminSnapshotPath() *v3.PathItem {
	params := []*v3.Parameter{
		queryParamWithExample("skip_head", "If true, do not snapshot data in the head block.", false, stringSchema(), []example{{"example", "false"}}),
	}
	return &v3.PathItem{
		Post: &v3.Operation{
			OperationId: "snapshotPost",
			Summary:     "Create a snapshot of the TSDB",
			Description: "Creates a snapshot of all current data.",
			Tags:        []string{"admin"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("SnapshotOutputBody", snapshotResponseExamples(), errorResponseExamples(), "Snapshot created successfully.", "Error creating snapshot."),
		},
		Put: &v3.Operation{
			OperationId: "snapshotPut",
			Summary:     "Create a snapshot of the TSDB via PUT",
			Description: "Creates a snapshot of all current data using PUT method.",
			Tags:        []string{"admin"},
			Parameters:  params,
			Responses:   responsesWithErrorExamples("SnapshotOutputBody", snapshotResponseExamples(), errorResponseExamples(), "Snapshot created successfully via PUT.", "Error creating snapshot via PUT."),
		},
	}
}

func (*OpenAPIBuilder) remoteReadPath() *v3.PathItem {
	return &v3.PathItem{
		Post: &v3.Operation{
			OperationId: "remoteRead",
			Summary:     "Remote read endpoint",
			Description: "Prometheus remote read endpoint for federated queries. Accepts and returns Protocol Buffer encoded data.",
			Tags:        []string{"remote"},
			Responses:   responsesNoContent(),
		},
	}
}

func (*OpenAPIBuilder) remoteWritePath() *v3.PathItem {
	return &v3.PathItem{
		Post: &v3.Operation{
			OperationId: "remoteWrite",
			Summary:     "Remote write endpoint",
			Description: "Prometheus remote write endpoint for sending metrics. Accepts Protocol Buffer encoded write requests.",
			Tags:        []string{"remote"},
			Responses:   responsesNoContent(),
		},
	}
}

func (*OpenAPIBuilder) otlpWritePath() *v3.PathItem {
	return &v3.PathItem{
		Post: &v3.Operation{
			OperationId: "otlpWrite",
			Summary:     "OTLP metrics write endpoint",
			Description: "OpenTelemetry Protocol metrics ingestion endpoint. Accepts OTLP/HTTP metrics in Protocol Buffer format.",
			Tags:        []string{"otlp"},
			Responses:   responsesNoContent(),
		},
	}
}

func (*OpenAPIBuilder) notificationsPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-notifications",
			Summary:     "Get notifications",
			Tags:        []string{"notifications"},
			Responses:   responsesWithErrorExamples("NotificationsOutputBody", notificationsResponseExamples(), errorResponseExamples(), "Notifications retrieved successfully.", "Error retrieving notifications."),
		},
	}
}

// notificationsLivePath defines the /notifications/live endpoint.
// This endpoint uses OpenAPI 3.2's itemSchema feature for documenting SSE streams.
// It is excluded from the OpenAPI 3.1 specification.
func (*OpenAPIBuilder) notificationsLivePath() *v3.PathItem {
	codes := orderedmap.New[string, *v3.Response]()
	content := orderedmap.New[string, *v3.MediaType]()

	// Create a schema for the SSE message structure.
	// Each SSE message has a 'data' field containing JSON.
	sseItemProps := orderedmap.New[string, *base.SchemaProxy]()
	sseItemProps.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type:             []string{"string"},
		Description:      "SSE data field containing JSON-encoded notification.",
		ContentMediaType: "application/json",
		ContentSchema:    schemaRef("#/components/schemas/Notification"),
	}))

	content.Set("text/event-stream", &v3.MediaType{
		// Use ItemSchema (OpenAPI 3.2) instead of Schema to describe each SSE message.
		ItemSchema: base.CreateSchemaProxy(&base.Schema{
			Type:                 []string{"object"},
			Title:                "Server Sent Event Message",
			Description:          "A single SSE message. The data field contains a JSON-encoded Notification object.",
			Properties:           sseItemProps,
			Required:             []string{"data"},
			AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		}),
		Examples: notificationLiveExamples(),
	})

	codes.Set("200", &v3.Response{
		Description: "Server-sent events stream established.",
		Content:     content,
	})
	codes.Set("default", errorResponse())

	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "notifications-live",
			Summary:     "Stream live notifications via Server-Sent Events",
			Description: "Subscribe to real-time server notifications using SSE. Each event contains a JSON-encoded Notification object in the data field.",
			Tags:        []string{"notifications"},
			Responses:   &v3.Responses{Codes: codes},
		},
	}
}

func (*OpenAPIBuilder) featuresPath() *v3.PathItem {
	return &v3.PathItem{
		Get: &v3.Operation{
			OperationId: "get-features",
			Summary:     "Get features",
			Tags:        []string{"features"},
			Responses:   responsesWithErrorExamples("FeaturesOutputBody", featuresResponseExamples(), errorResponseExamples(), "Feature flags retrieved successfully.", "Error retrieving features."),
		},
	}
}
