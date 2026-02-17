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

// This file defines all OpenAPI schema definitions for API request and response types.
// Schemas are organized by functional area: query, labels, series, metadata, targets,
// rules, alerts, and status endpoints.
package v1

import (
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// Schema definitions and components builder.

func (b *OpenAPIBuilder) buildComponents() *v3.Components {
	schemas := orderedmap.New[string, *base.SchemaProxy]()

	// Core schemas.
	schemas.Set("Error", b.errorSchema())
	schemas.Set("Labels", b.labelsSchema())

	// Query schemas.
	schemas.Set("QueryOutputBody", b.responseBodySchema("QueryData", "Response body for instant query."))
	schemas.Set("QueryRangeOutputBody", b.responseBodySchema("QueryData", "Response body for range query."))
	schemas.Set("QueryPostInputBody", b.queryPostInputBodySchema())
	schemas.Set("QueryRangePostInputBody", b.queryRangePostInputBodySchema())
	schemas.Set("QueryExemplarsOutputBody", b.simpleResponseBodySchema())
	schemas.Set("QueryExemplarsPostInputBody", b.queryExemplarsPostInputBodySchema())
	schemas.Set("FormatQueryOutputBody", b.formatQueryOutputBodySchema())
	schemas.Set("FormatQueryPostInputBody", b.formatQueryPostInputBodySchema())
	schemas.Set("ParseQueryOutputBody", b.simpleResponseBodySchema())
	schemas.Set("ParseQueryPostInputBody", b.parseQueryPostInputBodySchema())
	schemas.Set("QueryData", b.queryDataSchema())
	schemas.Set("QueryStats", b.queryStatsSchema())
	schemas.Set("FloatSample", b.floatSampleSchema())
	schemas.Set("HistogramSample", b.histogramSampleSchema())
	schemas.Set("FloatSeries", b.floatSeriesSchema())
	schemas.Set("HistogramSeries", b.histogramSeriesSchema())
	schemas.Set("HistogramValue", b.histogramValueSchema())

	// Label schemas.
	schemas.Set("LabelsOutputBody", b.stringArrayResponseBodySchema())
	schemas.Set("LabelsPostInputBody", b.labelsPostInputBodySchema())
	schemas.Set("LabelValuesOutputBody", b.stringArrayResponseBodySchema())

	// Series schemas.
	schemas.Set("SeriesOutputBody", b.labelsArrayResponseBodySchema())
	schemas.Set("SeriesPostInputBody", b.seriesPostInputBodySchema())
	schemas.Set("SeriesDeleteOutputBody", b.simpleResponseBodySchema())

	// Metadata schemas.
	schemas.Set("Metadata", b.metadataSchema())
	schemas.Set("MetadataOutputBody", b.metadataOutputBodySchema())
	schemas.Set("MetricMetadata", b.metricMetadataSchema())

	// Target schemas.
	schemas.Set("Target", b.targetSchema())
	schemas.Set("DroppedTarget", b.droppedTargetSchema())
	schemas.Set("TargetDiscovery", b.targetDiscoverySchema())
	schemas.Set("TargetsOutputBody", b.refResponseBodySchema("TargetDiscovery", "Response body for targets endpoint."))
	schemas.Set("TargetMetadataOutputBody", b.metricMetadataArrayResponseBodySchema())
	schemas.Set("ScrapePoolsDiscovery", b.scrapePoolsDiscoverySchema())
	schemas.Set("ScrapePoolsOutputBody", b.refResponseBodySchema("ScrapePoolsDiscovery", "Response body for scrape pools endpoint."))

	// Relabel schemas.
	schemas.Set("Config", b.configSchema())
	schemas.Set("RelabelStep", b.relabelStepSchema())
	schemas.Set("RelabelStepsResponse", b.relabelStepsResponseSchema())
	schemas.Set("TargetRelabelStepsOutputBody", b.refResponseBodySchema("RelabelStepsResponse", "Response body for target relabel steps endpoint."))

	// Rule schemas.
	schemas.Set("RuleGroup", b.ruleGroupSchema())
	schemas.Set("RuleDiscovery", b.ruleDiscoverySchema())
	schemas.Set("RulesOutputBody", b.refResponseBodySchema("RuleDiscovery", "Response body for rules endpoint."))

	// Alert schemas.
	schemas.Set("Alert", b.alertSchema())
	schemas.Set("AlertDiscovery", b.alertDiscoverySchema())
	schemas.Set("AlertsOutputBody", b.refResponseBodySchema("AlertDiscovery", "Response body for alerts endpoint."))
	schemas.Set("AlertmanagerTarget", b.alertmanagerTargetSchema())
	schemas.Set("AlertmanagerDiscovery", b.alertmanagerDiscoverySchema())
	schemas.Set("AlertmanagersOutputBody", b.refResponseBodySchema("AlertmanagerDiscovery", "Response body for alertmanagers endpoint."))

	// Status schemas.
	schemas.Set("StatusConfigData", b.statusConfigDataSchema())
	schemas.Set("StatusConfigOutputBody", b.refResponseBodySchema("StatusConfigData", "Response body for status config endpoint."))
	schemas.Set("RuntimeInfo", b.runtimeInfoSchema())
	schemas.Set("StatusRuntimeInfoOutputBody", b.refResponseBodySchema("RuntimeInfo", "Response body for status runtime info endpoint."))
	schemas.Set("PrometheusVersion", b.prometheusVersionSchema())
	schemas.Set("StatusBuildInfoOutputBody", b.refResponseBodySchema("PrometheusVersion", "Response body for status build info endpoint."))
	schemas.Set("StatusFlagsOutputBody", b.statusFlagsOutputBodySchema())
	schemas.Set("HeadStats", b.headStatsSchema())
	schemas.Set("TSDBStat", b.tsdbStatSchema())
	schemas.Set("TSDBStatus", b.tsdbStatusSchema())
	schemas.Set("StatusTSDBOutputBody", b.refResponseBodySchema("TSDBStatus", "Response body for status TSDB endpoint."))
	schemas.Set("BlockDesc", b.blockDescSchema())
	schemas.Set("BlockStats", b.blockStatsSchema())
	schemas.Set("BlockMetaCompaction", b.blockMetaCompactionSchema())
	schemas.Set("BlockMeta", b.blockMetaSchema())
	schemas.Set("StatusTSDBBlocksData", b.statusTSDBBlocksDataSchema())
	schemas.Set("StatusTSDBBlocksOutputBody", b.refResponseBodySchema("StatusTSDBBlocksData", "Response body for status TSDB blocks endpoint."))
	schemas.Set("StatusWALReplayData", b.statusWALReplayDataSchema())
	schemas.Set("StatusWALReplayOutputBody", b.refResponseBodySchema("StatusWALReplayData", "Response body for status WAL replay endpoint."))

	// Admin schemas.
	schemas.Set("DeleteSeriesOutputBody", b.statusOnlyResponseBodySchema())
	schemas.Set("CleanTombstonesOutputBody", b.statusOnlyResponseBodySchema())
	schemas.Set("DataStruct", b.dataStructSchema())
	schemas.Set("SnapshotOutputBody", b.refResponseBodySchema("DataStruct", "Response body for snapshot endpoint."))

	// Notification schemas.
	schemas.Set("Notification", b.notificationSchema())
	schemas.Set("NotificationsOutputBody", b.notificationArrayResponseBodySchema())

	// Features schema.
	schemas.Set("FeaturesOutputBody", b.simpleResponseBodySchema())

	return &v3.Components{Schemas: schemas}
}

// Schema definitions using high-level structs.

func (*OpenAPIBuilder) errorSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("errorType", stringSchemaWithDescriptionAndExample("Type of error that occurred.", "bad_data"))
	props.Set("error", stringSchemaWithDescriptionAndExample("Human-readable error message.", "invalid parameter"))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Error response.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "errorType", "error"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) labelsSchema() *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Label set represented as a key-value map.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: true},
	})
}

func (*OpenAPIBuilder) responseBodySchema(dataSchemaRef, description string) *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", schemaRef("#/components/schemas/"+dataSchemaRef))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          description,
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (b *OpenAPIBuilder) refResponseBodySchema(dataSchemaRef, description string) *base.SchemaProxy {
	return b.responseBodySchema(dataSchemaRef, description)
}

func (*OpenAPIBuilder) simpleResponseBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Description: "Response data (structure varies by endpoint).",
		Example:     createYAMLNode(map[string]any{"result": "ok"}),
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Generic response body.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) statusOnlyResponseBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body containing only status.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) stringArrayResponseBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type:    []string{"array"},
		Items:   &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		Example: createYAMLNode([]string{"__name__", "job", "instance"}),
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body with an array of strings.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) labelsArrayResponseBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type:    []string{"array"},
		Items:   &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/Labels")},
		Example: createYAMLNode([]map[string]string{{"__name__": "up", "job": "prometheus", "instance": "localhost:9090"}}),
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body with an array of label sets.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) metricMetadataArrayResponseBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/MetricMetadata")},
		Example: createYAMLNode([]map[string]any{
			{
				"target": map[string]string{
					"instance": "localhost:9090",
					"job":      "prometheus",
				},
				"metric": "up",
				"type":   "gauge",
				"help":   "The current health status of the target",
				"unit":   "",
			},
		}),
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body with an array of metric metadata.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) notificationArrayResponseBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/Notification")},
		Example: createYAMLNode([]map[string]any{
			{"text": "Server is running", "date": "2023-07-21T20:00:00.000Z", "active": true},
		}),
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body with an array of notifications.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) floatSampleSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("metric", schemaRef("#/components/schemas/Labels"))
	props.Set("value", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Timestamp and float value as [unixTimestamp, stringValue].",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			OneOf: []*base.SchemaProxy{
				base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}}),
				stringSchema(),
			},
		})},
		MinItems: int64Ptr(2),
		MaxItems: int64Ptr(2),
		Example:  createYAMLNode([]any{1767436620, "1"}),
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "A sample with a float value.",
		Required:             []string{"metric", "value"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) histogramValueSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("count", stringSchemaWithDescription("Total count of observations."))
	props.Set("sum", stringSchemaWithDescription("Sum of all observed values."))
	props.Set("buckets", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Histogram buckets as [boundary_rule, lower, upper, count].",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			Type: []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
				OneOf: []*base.SchemaProxy{
					base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}}),
					stringSchema(),
				},
			})},
		})},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Native histogram value representation.",
		Required:             []string{"count", "sum"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) histogramSampleSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("metric", schemaRef("#/components/schemas/Labels"))
	props.Set("histogram", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Timestamp and histogram value as [unixTimestamp, histogramObject].",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			OneOf: []*base.SchemaProxy{
				base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}}),
				schemaRef("#/components/schemas/HistogramValue"),
			},
		})},
		MinItems: int64Ptr(2),
		MaxItems: int64Ptr(2),
		Example:  createYAMLNode([]any{1767436620, map[string]any{"count": "60", "sum": "120", "buckets": []any{}}}),
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "A sample with a native histogram value.",
		Required:             []string{"metric", "histogram"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) floatSeriesSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("metric", schemaRef("#/components/schemas/Labels"))
	props.Set("values", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Array of [timestamp, stringValue] pairs for float values.",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			Type: []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
				OneOf: []*base.SchemaProxy{
					base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}}),
					stringSchema(),
				},
			})},
			MinItems: int64Ptr(2),
			MaxItems: int64Ptr(2),
		})},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "A time series with float values.",
		Required:             []string{"metric", "values"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) histogramSeriesSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("metric", schemaRef("#/components/schemas/Labels"))
	props.Set("histograms", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Array of [timestamp, histogramObject] pairs for histogram values.",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			Type: []string{"array"},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
				OneOf: []*base.SchemaProxy{
					base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}}),
					schemaRef("#/components/schemas/HistogramValue"),
				},
			})},
			MinItems: int64Ptr(2),
			MaxItems: int64Ptr(2),
		})},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "A time series with native histogram values.",
		Required:             []string{"metric", "histograms"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) queryDataSchema() *base.SchemaProxy {
	// Vector query result.
	vectorProps := orderedmap.New[string, *base.SchemaProxy]()
	vectorProps.Set("resultType", stringSchemaWithConstValue("vector"))
	vectorProps.Set("result", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Array of samples (either float or histogram).",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			AnyOf: []*base.SchemaProxy{
				schemaRef("#/components/schemas/FloatSample"),
				schemaRef("#/components/schemas/HistogramSample"),
			},
		})},
	}))
	vectorProps.Set("stats", schemaRef("#/components/schemas/QueryStats"))

	// Matrix query result.
	matrixProps := orderedmap.New[string, *base.SchemaProxy]()
	matrixProps.Set("resultType", stringSchemaWithConstValue("matrix"))
	matrixProps.Set("result", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Array of time series (either float or histogram).",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			AnyOf: []*base.SchemaProxy{
				schemaRef("#/components/schemas/FloatSeries"),
				schemaRef("#/components/schemas/HistogramSeries"),
			},
		})},
	}))
	matrixProps.Set("stats", schemaRef("#/components/schemas/QueryStats"))

	// Scalar query result.
	scalarProps := orderedmap.New[string, *base.SchemaProxy]()
	scalarProps.Set("resultType", stringSchemaWithConstValue("scalar"))
	scalarProps.Set("result", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Scalar value as [timestamp, stringValue].",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			OneOf: []*base.SchemaProxy{
				base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}}),
				stringSchema(),
			},
		})},
		MinItems: int64Ptr(2),
		MaxItems: int64Ptr(2),
	}))
	scalarProps.Set("stats", schemaRef("#/components/schemas/QueryStats"))

	// String query result.
	stringResultProps := orderedmap.New[string, *base.SchemaProxy]()
	stringResultProps.Set("resultType", stringSchemaWithConstValue("string"))
	stringResultProps.Set("result", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "String value as [timestamp, stringValue].",
		Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		MinItems:    int64Ptr(2),
		MaxItems:    int64Ptr(2),
	}))
	stringResultProps.Set("stats", schemaRef("#/components/schemas/QueryStats"))

	return base.CreateSchemaProxy(&base.Schema{
		Description: "Query result data. The structure of 'result' depends on 'resultType'.",
		AnyOf: []*base.SchemaProxy{
			// resultType: vector -> result: array of samples.
			base.CreateSchemaProxy(&base.Schema{
				Type:                 []string{"object"},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
				Properties:           vectorProps,
			}),
			// resultType: matrix -> result: array of series.
			base.CreateSchemaProxy(&base.Schema{
				Type:                 []string{"object"},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
				Properties:           matrixProps,
			}),
			// resultType: scalar -> result: [timestamp, value].
			base.CreateSchemaProxy(&base.Schema{
				Type:                 []string{"object"},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
				Properties:           scalarProps,
			}),
			// resultType: string -> result: [timestamp, stringValue].
			base.CreateSchemaProxy(&base.Schema{
				Type:                 []string{"object"},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
				Properties:           stringResultProps,
			}),
		},
		Example: createYAMLNode(map[string]any{
			"resultType": "vector",
			"result": []map[string]any{
				{
					"metric": map[string]string{"__name__": "up", "job": "prometheus"},
					"value":  []any{1627845600, "1"},
				},
			},
		}),
	})
}

func (*OpenAPIBuilder) queryStatsSchema() *base.SchemaProxy {
	// Timings object.
	timingsProps := orderedmap.New[string, *base.SchemaProxy]()
	timingsProps.Set("evalTotalTime", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Description: "Total evaluation time in seconds.",
	}))
	timingsProps.Set("resultSortTime", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Description: "Time spent sorting results in seconds.",
	}))
	timingsProps.Set("queryPreparationTime", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Description: "Query preparation time in seconds.",
	}))
	timingsProps.Set("innerEvalTime", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Description: "Inner evaluation time in seconds.",
	}))
	timingsProps.Set("execQueueTime", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Description: "Execution queue wait time in seconds.",
	}))
	timingsProps.Set("execTotalTime", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Description: "Total execution time in seconds.",
	}))

	// Samples object.
	samplesProps := orderedmap.New[string, *base.SchemaProxy]()
	samplesProps.Set("totalQueryableSamples", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"integer"},
		Description: "Total number of samples that were queryable.",
	}))
	samplesProps.Set("peakSamples", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"integer"},
		Description: "Peak number of samples in memory.",
	}))
	samplesProps.Set("totalQueryableSamplesPerStep", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Total queryable samples per step (only included with stats=all).",
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{
			Type:        []string{"array"},
			Description: "Timestamp and sample count as [timestamp, count].",
			Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{Type: []string{"number"}})},
			MinItems:    int64Ptr(2),
			MaxItems:    int64Ptr(2),
		})},
	}))

	// Main stats object.
	statsProps := orderedmap.New[string, *base.SchemaProxy]()
	statsProps.Set("timings", base.CreateSchemaProxy(&base.Schema{
		Type:       []string{"object"},
		Properties: timingsProps,
	}))
	statsProps.Set("samples", base.CreateSchemaProxy(&base.Schema{
		Type:       []string{"object"},
		Properties: samplesProps,
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"object"},
		Description: "Query execution statistics (included when the stats query parameter is provided).",
		Properties:  statsProps,
	})
}

func (*OpenAPIBuilder) queryPostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("query", stringSchemaWithDescriptionAndExample("Form field: The PromQL query to execute.", "up"))
	props.Set("time", stringSchemaWithDescriptionAndExample("Form field: The evaluation timestamp (optional, defaults to current time).", "2023-07-21T20:10:51.781Z"))
	props.Set("limit", integerSchemaWithDescriptionAndExample("Form field: The maximum number of metrics to return.", 100))
	props.Set("timeout", stringSchemaWithDescriptionAndExample("Form field: Evaluation timeout (optional, defaults to and is capped by the value of the -query.timeout flag).", "30s"))
	props.Set("lookback_delta", stringSchemaWithDescriptionAndExample("Form field: Override the lookback period for this query (optional).", "5m"))
	props.Set("stats", stringSchemaWithDescriptionAndExample("Form field: When provided, include query statistics in the response (the special value 'all' enables more comprehensive statistics).", "all"))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for instant query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"query"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) queryRangePostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("query", stringSchemaWithDescriptionAndExample("Form field: The query to execute.", "rate(http_requests_total[5m])"))
	props.Set("start", stringSchemaWithDescriptionAndExample("Form field: The start time of the query.", "2023-07-21T20:10:30.781Z"))
	props.Set("end", stringSchemaWithDescriptionAndExample("Form field: The end time of the query.", "2023-07-21T20:20:30.781Z"))
	props.Set("step", stringSchemaWithDescriptionAndExample("Form field: The step size of the query.", "15s"))
	props.Set("limit", integerSchemaWithDescriptionAndExample("Form field: The maximum number of metrics to return.", 100))
	props.Set("timeout", stringSchemaWithDescriptionAndExample("Form field: Evaluation timeout (optional, defaults to and is capped by the value of the -query.timeout flag).", "30s"))
	props.Set("lookback_delta", stringSchemaWithDescriptionAndExample("Form field: Override the lookback period for this query (optional).", "5m"))
	props.Set("stats", stringSchemaWithDescriptionAndExample("Form field: When provided, include query statistics in the response (the special value 'all' enables more comprehensive statistics).", "all"))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for range query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"query", "start", "end", "step"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) queryExemplarsPostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("query", stringSchemaWithDescriptionAndExample("Form field: The query to execute.", "http_requests_total"))
	props.Set("start", stringSchemaWithDescriptionAndExample("Form field: The start time of the query.", "2023-07-21T20:00:00.000Z"))
	props.Set("end", stringSchemaWithDescriptionAndExample("Form field: The end time of the query.", "2023-07-21T21:00:00.000Z"))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for exemplars query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"query"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) formatQueryOutputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", stringSchemaWithDescriptionAndExample("Formatted query string.", "sum by(status) (rate(http_requests_total[5m]))"))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body for format query endpoint.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) formatQueryPostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("query", stringSchemaWithDescriptionAndExample("Form field: The query to format.", "sum(rate(http_requests_total[5m])) by (status)"))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for format query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"query"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) parseQueryPostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("query", stringSchemaWithDescriptionAndExample("Form field: The query to parse.", "sum(rate(http_requests_total[5m]))"))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for parse query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"query"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) labelsPostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("start", stringSchemaWithDescriptionAndExample("Form field: The start time of the query.", "2023-07-21T20:00:00.000Z"))
	props.Set("end", stringSchemaWithDescriptionAndExample("Form field: The end time of the query.", "2023-07-21T21:00:00.000Z"))
	props.Set("match[]", stringArraySchemaWithDescriptionAndExample("Form field: Series selector argument that selects the series from which to read the label names.", []string{"{job=\"prometheus\"}"}))
	props.Set("limit", integerSchemaWithDescriptionAndExample("Form field: The maximum number of label names to return.", 100))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for labels query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) seriesPostInputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("start", stringSchemaWithDescriptionAndExample("Form field: The start time of the query.", "2023-07-21T20:00:00.000Z"))
	props.Set("end", stringSchemaWithDescriptionAndExample("Form field: The end time of the query.", "2023-07-21T21:00:00.000Z"))
	props.Set("match[]", stringArraySchemaWithDescriptionAndExample("Form field: Series selector argument that selects the series to return.", []string{"{job=\"prometheus\"}"}))
	props.Set("limit", integerSchemaWithDescriptionAndExample("Form field: The maximum number of series to return.", 100))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "POST request body for series query.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"match[]"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) metadataSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("type", stringSchemaWithDescription("Metric type (counter, gauge, histogram, summary, or untyped)."))
	props.Set("unit", stringSchemaWithDescription("Unit of the metric."))
	props.Set("help", stringSchemaWithDescription("Help text describing the metric."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Metric metadata.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"type", "unit", "help"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) metadataOutputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type: []string{"object"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{
			A: base.CreateSchemaProxy(&base.Schema{
				Type:  []string{"array"},
				Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/Metadata")},
			}),
		},
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body for metadata endpoint.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) metricMetadataSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("target", schemaRef("#/components/schemas/Labels"))
	props.Set("metric", stringSchemaWithDescription("Metric name."))
	props.Set("type", stringSchemaWithDescription("Metric type (counter, gauge, histogram, summary, or untyped)."))
	props.Set("help", stringSchemaWithDescription("Help text describing the metric."))
	props.Set("unit", stringSchemaWithDescription("Unit of the metric."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Target metric metadata.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"target", "type", "help", "unit"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) targetSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("discoveredLabels", schemaRef("#/components/schemas/Labels"))
	props.Set("labels", schemaRef("#/components/schemas/Labels"))
	props.Set("scrapePool", stringSchemaWithDescription("Name of the scrape pool."))
	props.Set("scrapeUrl", stringSchemaWithDescription("URL of the target."))
	props.Set("globalUrl", stringSchemaWithDescription("Global URL of the target."))
	props.Set("lastError", stringSchemaWithDescription("Last error message from scraping."))
	props.Set("lastScrape", dateTimeSchemaWithDescription("Timestamp of the last scrape."))
	props.Set("lastScrapeDuration", numberSchemaWithDescription("Duration of the last scrape in seconds."))
	props.Set("health", stringSchemaWithDescription("Health status of the target (up, down, or unknown)."))
	props.Set("scrapeInterval", stringSchemaWithDescription("Scrape interval for this target."))
	props.Set("scrapeTimeout", stringSchemaWithDescription("Scrape timeout for this target."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Scrape target information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"discoveredLabels", "labels", "scrapePool", "scrapeUrl", "globalUrl", "lastError", "lastScrape", "lastScrapeDuration", "health", "scrapeInterval", "scrapeTimeout"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) droppedTargetSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("discoveredLabels", schemaRef("#/components/schemas/Labels"))
	props.Set("scrapePool", stringSchemaWithDescription("Name of the scrape pool."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Dropped target information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"discoveredLabels", "scrapePool"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) targetDiscoverySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("activeTargets", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/Target")},
	}))
	props.Set("droppedTargets", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/DroppedTarget")},
	}))
	props.Set("droppedTargetCounts", base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: integerSchema()},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Target discovery information including active and dropped targets.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"activeTargets", "droppedTargets", "droppedTargetCounts"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) scrapePoolsDiscoverySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("scrapePools", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "List of all configured scrape pools.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"scrapePools"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) configSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("source_labels", stringArraySchemaWithDescription("Source labels for relabeling."))
	props.Set("separator", stringSchemaWithDescription("Separator for source label values."))
	props.Set("regex", stringSchemaWithDescription("Regular expression for matching."))
	props.Set("modulus", integerSchemaWithDescription("Modulus for hash-based relabeling."))
	props.Set("target_label", stringSchemaWithDescription("Target label name."))
	props.Set("replacement", stringSchemaWithDescription("Replacement value."))
	props.Set("action", stringSchemaWithDescription("Relabel action."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Relabel configuration.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) relabelStepSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("rule", schemaRef("#/components/schemas/Config"))
	props.Set("output", schemaRef("#/components/schemas/Labels"))
	props.Set("keep", base.CreateSchemaProxy(&base.Schema{Type: []string{"boolean"}}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Relabel step showing the rule, output, and whether the target was kept.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"rule", "output", "keep"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) relabelStepsResponseSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("steps", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/RelabelStep")},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Relabeling steps response.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"steps"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) ruleGroupSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("name", stringSchemaWithDescription("Name of the rule group."))
	props.Set("file", stringSchemaWithDescription("File containing the rule group."))
	props.Set("rules", base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Description: "Rules in this group.",
		Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{Type: []string{"object"}, Description: "Rule definition."})},
	}))
	props.Set("interval", numberSchemaWithDescription("Evaluation interval in seconds."))
	props.Set("limit", integerSchemaWithDescription("Maximum number of alerts for this group."))
	props.Set("evaluationTime", numberSchemaWithDescription("Time taken to evaluate the group in seconds."))
	props.Set("lastEvaluation", dateTimeSchemaWithDescription("Timestamp of the last evaluation."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Rule group information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"name", "file", "rules", "interval", "limit", "evaluationTime", "lastEvaluation"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) ruleDiscoverySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("groups", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/RuleGroup")},
	}))
	props.Set("groupNextToken", stringSchemaWithDescription("Pagination token for the next page of groups."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Rule discovery information containing all rule groups.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"groups"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) alertSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("labels", schemaRef("#/components/schemas/Labels"))
	props.Set("annotations", schemaRef("#/components/schemas/Labels"))
	props.Set("state", stringSchemaWithDescription("State of the alert (pending, firing, or inactive)."))
	props.Set("value", stringSchemaWithDescription("Value of the alert expression."))
	props.Set("activeAt", dateTimeSchemaWithDescription("Timestamp when the alert became active."))
	props.Set("keepFiringSince", dateTimeSchemaWithDescription("Timestamp since the alert has been kept firing."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Alert information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"labels", "annotations", "state", "value"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) alertDiscoverySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("alerts", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/Alert")},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Alert discovery information containing all active alerts.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"alerts"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) alertmanagerTargetSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("url", stringSchemaWithDescription("URL of the Alertmanager instance."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Alertmanager target information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"url"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) alertmanagerDiscoverySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("activeAlertmanagers", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/AlertmanagerTarget")},
	}))
	props.Set("droppedAlertmanagers", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/AlertmanagerTarget")},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Alertmanager discovery information including active and dropped instances.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"activeAlertmanagers", "droppedAlertmanagers"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) statusConfigDataSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("yaml", stringSchemaWithDescription("Prometheus configuration in YAML format."))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Prometheus configuration.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"yaml"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) runtimeInfoSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("startTime", base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}, Format: "date-time"}))
	props.Set("CWD", stringSchema())
	props.Set("hostname", stringSchema())
	props.Set("serverTime", base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}, Format: "date-time"}))
	props.Set("reloadConfigSuccess", base.CreateSchemaProxy(&base.Schema{Type: []string{"boolean"}}))
	props.Set("lastConfigTime", base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}, Format: "date-time"}))
	props.Set("corruptionCount", integerSchema())
	props.Set("goroutineCount", integerSchema())
	props.Set("GOMAXPROCS", integerSchema())
	props.Set("GOMEMLIMIT", integerSchema())
	props.Set("GOGC", stringSchema())
	props.Set("GODEBUG", stringSchema())
	props.Set("storageRetention", stringSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Prometheus runtime information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"startTime", "CWD", "hostname", "serverTime", "reloadConfigSuccess", "lastConfigTime", "corruptionCount", "goroutineCount", "GOMAXPROCS", "GOMEMLIMIT", "GOGC", "GODEBUG", "storageRetention"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) prometheusVersionSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("version", stringSchema())
	props.Set("revision", stringSchema())
	props.Set("branch", stringSchema())
	props.Set("buildUser", stringSchema())
	props.Set("buildDate", stringSchema())
	props.Set("goVersion", stringSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Prometheus version information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"version", "revision", "branch", "buildUser", "buildDate", "goVersion"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) statusFlagsOutputBodySchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("status", statusSchema())
	props.Set("data", base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
	}))
	props.Set("warnings", warningsSchema())
	props.Set("infos", infosSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Response body for status flags endpoint.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"status", "data"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) headStatsSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("numSeries", integerSchema())
	props.Set("numLabelPairs", integerSchema())
	props.Set("chunkCount", integerSchema())
	props.Set("minTime", integerSchema())
	props.Set("maxTime", integerSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "TSDB head statistics.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"numSeries", "numLabelPairs", "chunkCount", "minTime", "maxTime"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) tsdbStatSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("name", stringSchema())
	props.Set("value", integerSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "TSDB statistic.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"name", "value"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) tsdbStatusSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("headStats", schemaRef("#/components/schemas/HeadStats"))
	props.Set("seriesCountByMetricName", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/TSDBStat")},
	}))
	props.Set("labelValueCountByLabelName", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/TSDBStat")},
	}))
	props.Set("memoryInBytesByLabelName", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/TSDBStat")},
	}))
	props.Set("seriesCountByLabelValuePair", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/TSDBStat")},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "TSDB status information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"headStats", "seriesCountByMetricName", "labelValueCountByLabelName", "memoryInBytesByLabelName", "seriesCountByLabelValuePair"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) blockDescSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("ulid", stringSchema())
	props.Set("minTime", integerSchema())
	props.Set("maxTime", integerSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Block descriptor.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"ulid", "minTime", "maxTime"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) blockStatsSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("numSamples", integerSchema())
	props.Set("numSeries", integerSchema())
	props.Set("numChunks", integerSchema())
	props.Set("numTombstones", integerSchema())
	props.Set("numFloatSamples", integerSchema())
	props.Set("numHistogramSamples", integerSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Block statistics.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) blockMetaCompactionSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("level", integerSchema())
	props.Set("sources", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
	}))
	props.Set("parents", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/BlockDesc")},
	}))
	props.Set("failed", base.CreateSchemaProxy(&base.Schema{Type: []string{"boolean"}}))
	props.Set("deletable", base.CreateSchemaProxy(&base.Schema{Type: []string{"boolean"}}))
	props.Set("hints", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Block compaction metadata.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"level"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) blockMetaSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("ulid", stringSchema())
	props.Set("minTime", integerSchema())
	props.Set("maxTime", integerSchema())
	props.Set("stats", schemaRef("#/components/schemas/BlockStats"))
	props.Set("compaction", schemaRef("#/components/schemas/BlockMetaCompaction"))
	props.Set("version", integerSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Block metadata.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"ulid", "minTime", "maxTime", "compaction", "version"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) statusTSDBBlocksDataSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("blocks", base.CreateSchemaProxy(&base.Schema{
		Type:  []string{"array"},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schemaRef("#/components/schemas/BlockMeta")},
	}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "TSDB blocks information.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"blocks"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) statusWALReplayDataSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("min", integerSchema())
	props.Set("max", integerSchema())
	props.Set("current", integerSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "WAL replay status.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"min", "max", "current"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) dataStructSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("name", stringSchema())

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Generic data structure with a name field.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"name"},
		Properties:           props,
	})
}

func (*OpenAPIBuilder) notificationSchema() *base.SchemaProxy {
	props := orderedmap.New[string, *base.SchemaProxy]()
	props.Set("text", stringSchema())
	props.Set("date", base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}, Format: "date-time"}))
	props.Set("active", base.CreateSchemaProxy(&base.Schema{Type: []string{"boolean"}}))

	return base.CreateSchemaProxy(&base.Schema{
		Type:                 []string{"object"},
		Description:          "Server notification.",
		AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: false},
		Required:             []string{"text", "date", "active"},
		Properties:           props,
	})
}
