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

package v1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/sse"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/notifications"
)

type HumaOptions struct {
	Enabled     bool
	ExternalURL *url.URL
}

// Time wraps time.Time to support Prometheus time parsing (Unix timestamp or RFC3339)
type Time struct {
	time.Time
}

// UnmarshalText implements the encoding.TextUnmarshaler interface for custom time parsing.
func (t *Time) UnmarshalText(text []byte) error {
	s := string(text)

	// Try parsing as Unix timestamp (float seconds).
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		sec, nsec := math.Modf(v)
		nsec = math.Round(nsec*1000) / 1000
		t.Time = time.Unix(int64(sec), int64(nsec*float64(time.Second))).UTC()
		return nil
	}

	// Try parsing as RFC3339.
	if parsedTime, err := time.Parse(time.RFC3339Nano, s); err == nil {
		t.Time = parsedTime
		return nil
	}

	return fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

// Schema implements huma.SchemaProvider to provide OpenAPI schema information
func (t Time) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Format:      "prometheus-time",
		Description: "Timestamp as Unix time (seconds since epoch, float) or RFC3339 format",
	}
}

// Duration wraps time.Duration to support Prometheus duration parsing (e.g., "1h", "5m", "30s")
type Duration struct {
	time.Duration
}

// UnmarshalText implements the encoding.TextUnmarshaler interface for custom duration parsing.
func (d *Duration) UnmarshalText(text []byte) error {
	s := string(text)

	// Try parsing as float (seconds).
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		ts := v * float64(time.Second)
		if ts > float64(math.MaxInt64) || ts < float64(math.MinInt64) {
			return fmt.Errorf("cannot parse %q to a valid duration. It overflows int64", s)
		}
		d.Duration = time.Duration(ts)
		return nil
	}

	// Try parsing as Prometheus duration (e.g., "1h", "5m").
	if v, err := model.ParseDuration(s); err == nil {
		d.Duration = time.Duration(v)
		return nil
	}

	return fmt.Errorf("cannot parse %q to a valid duration", s)
}

// Schema implements huma.SchemaProvider to provide OpenAPI schema information
func (d Duration) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Format:      "duration",
		Description: "Duration in Prometheus format (e.g., '1h', '5m', '30s') or as seconds (float)",
	}
}

type Error struct {
	Status    string    `json:"status" description:"The status of the response."`
	ErrorType errorType `json:"errorType" description:"The type of error."`
	ErrorMsg  string    `json:"error" description:"The error message."`
	statusInt int
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrorType.str, e.ErrorMsg)
}

func (e *Error) GetStatus() int {
	if e.statusInt != 0 {
		return e.statusInt
	}
	return getDefaultErrorCode(e.ErrorType)
}

func NewError(status int, message string, errs ...error) huma.StatusError {
	if len(errs) == 0 {
		return &Error{
			statusInt: status,
			Status:    "error",
			ErrorType: errorType{ErrorInternal, "internal error"},
		}
	}
	err := errs[0]
	errorType := errorInternal
	switch {
	case errors.Is(err, tsdb.ErrNotReady):
		errorType = errorUnavailable
	case strings.HasPrefix(err.Error(), "invalid"):
		errorType = errorBadData
	case strings.HasPrefix(err.Error(), "timeout"):
		errorType = errorTimeout
	case strings.HasPrefix(err.Error(), "canceled"):
		errorType = errorCanceled
	case strings.HasPrefix(err.Error(), "execution"):
		errorType = errorExec
	}
	errsStrs := make([]string, len(errs))
	for i, err := range errs {
		errsStrs[i] = err.Error()
	}
	var errmsg string
	if message != "" {
		errmsg = message + ": "
	}
	errmsg += strings.Join(errsStrs, ", ")
	return &Error{
		statusInt: status,
		Status:    "error",
		ErrorType: errorType,
		ErrorMsg:  errmsg,
	}
}

func init() {
	huma.NewError = NewError
}

func (api *API) agentMiddleware() func(huma.Context, func(huma.Context)) {
	if !api.isAgent {
		return func(ctx huma.Context, next func(huma.Context)) {
			next(ctx)
		}
	}
	return func(ctx huma.Context, next func(huma.Context)) {
		huma.WriteErr(api.humaAPI, ctx, http.StatusInternalServerError,
			"only available with Prometheus Agent", errors.New("execution error"))
	}
}

func (api *API) initiateHuma(opts *HumaOptions) func(http.HandlerFunc) http.HandlerFunc {
	if !opts.Enabled {
		return func(f http.HandlerFunc) http.HandlerFunc {
			return f
		}
	}

	config := huma.DefaultConfig("Prometheus API", version.Version)
	config.CreateHooks = nil

	// Use our optimized JSONCodec with jsoniter for better performance.
	json := jsoniter.ConfigCompatibleWithStandardLibrary

	config.Formats = map[string]huma.Format{
		"application/json": {
			Marshal: func(w io.Writer, v any) error {
				stream := json.BorrowStream(w)
				defer json.ReturnStream(stream)
				stream.WriteVal(v)
				if stream.Error != nil {
					return stream.Error
				}
				return stream.Flush()
			},
			Unmarshal: func(data []byte, v any) error {
				return json.Unmarshal(data, v)
			},
		},
		"application/x-www-form-urlencoded": urlEncodedFormat,
	}

	config.OpenAPI.Servers = []*huma.Server{
		{
			URL: opts.ExternalURL.ResolveReference(&url.URL{Path: "/api/v1"}).String(),
		},
	}
	config.OpenAPI.Info.Contact = &huma.Contact{
		Name: "Prometheus Community",
		URL:  "https://prometheus.io/community/",
	}
	config.OpenAPI.Info.Description = "Prometheus is an Open-Source monitoring system with a dimensional data model, flexible query language, efficient time series database and modern alerting approach."

	api.humaAPI = humago.New(http.NewServeMux(), config)
	api.humaAPI.UseMiddleware(CORSMiddleware(api.CORSOrigin))
	api.humaAPI.UseMiddleware(FinalizerMiddleware())

	api.registerMetadata()
	api.registerQuery()
	api.registerQueryRange()
	api.registerStatusConfig()
	api.registerStatusRuntimeInfo()
	api.registerStatusBuildInfo()
	api.registerStatusFlags()
	api.registerStatusTSDB()
	api.registerStatusTSDBBlocks()
	api.registerStatusWALReplay()
	api.registerNotifications()
	api.registerNotificationsLive()
	api.registerLabels()
	api.registerLabelValues()
	api.registerSeries()
	api.registerQueryExemplars()
	api.registerFormatQuery()
	api.registerParseQuery()
	api.registerScrapePools()
	api.registerAlertmanagers()
	api.registerTargets()
	api.registerTargetMetadata()
	api.registerTargetRelabelSteps()
	api.registerAlerts()
	api.registerRules()
	api.registerDeleteSeries()
	api.registerSnapshot()
	api.registerCleanTombstones()
	api.registerDropSeries()
	api.registerRemoteRead()
	api.registerRemoteWrite()
	api.registerOTLPWrite()

	// Wrap with compression and ready check at the HTTP level.
	compressedHandler := CompressionHandler{Handler: api.humaAPI.Adapter()}
	readyHandler := api.ready(compressedHandler.ServeHTTP)

	return func(http.HandlerFunc) http.HandlerFunc { return readyHandler }
}

type MetadataInput struct {
	Limit          int    `required:"false" default:"-1" query:"limit" doc:"The maximum number of metrics to return." example:"100"`
	LimitPerMetric int    `required:"false" default:"-1" query:"limitPerMetric" doc:"The maximum number of metadata entries per metric to return." example:"10"`
	Metric         string `required:"false" query:"metric" doc:"A metric name to filter metadata for. All metric metadata is retrieved if left empty." example:"go_goroutines"`
}

type MetadataOutput struct {
	Body struct {
		Status string                         `json:"status"`
		Data   map[string][]metadata.Metadata `json:"data"`
	}
}

func (api *API) registerMetadata() {
	huma.Get(api.humaAPI, "/metadata", func(ctx context.Context, input *MetadataInput) (output *MetadataOutput, err error) {
		res := api.getMetricMetadata(ctx, input.Limit, input.LimitPerMetric, input.Metric)

		out := &MetadataOutput{}
		out.Body.Status = "success"
		out.Body.Data = res
		return out, nil
	})
}

type QueryInput struct {
	Limit int    `required:"false" default:"0" minimum:"0" query:"limit" doc:"The maximum number of metrics to return." example:"100"`
	Time  Time   `required:"false" query:"time" doc:"The evaluation timestamp (optional, defaults to current time)." example:"2021-01-01T00:00:00Z"`
	Query string `required:"true" query:"query" minLength:"1" doc:"The PromQL query to execute." example:"up"`
	Stats string `required:"false" query:"stats" doc:"Include query statistics in the response if not empty. Use 'all' for detailed per-step statistics." example:"all"`
}

type QueryPostInput struct {
	Body struct {
		Limit int    `required:"false" default:"0" minimum:"0" json:"limit,omitempty" schema:"limit" doc:"The maximum number of metrics to return." example:"100"`
		Time  Time   `required:"false" json:"time,omitempty" schema:"time" doc:"The evaluation timestamp (optional, defaults to current time)." example:"2021-01-01T00:00:00Z"`
		Query string `required:"true" json:"query" schema:"query" minLength:"1" doc:"The PromQL query to execute." example:"up"`
		Stats string `required:"false" json:"stats,omitempty" schema:"stats" doc:"Include query statistics in the response if not empty. Use 'all' for detailed per-step statistics." example:"all"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type QueryOutput struct {
	Body struct {
		Status string     `json:"status"`
		Data   *QueryData `json:"data"`
	}
}

func (api *API) registerQuery() {
	// Shared implementation for query execution.
	executeQuery := func(ctx context.Context, query string, ts time.Time, limit int, statsParam string) (*QueryOutput, error) {
		// Use current time if not specified (zero value check).
		evalTime := ts
		if evalTime.IsZero() {
			evalTime = api.now()
		}

		opts := promql.NewPrometheusQueryOpts(false, 0)
		qry, err := api.QueryEngine.NewInstantQuery(ctx, api.Queryable, opts, query, evalTime)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid query", err)
		}
		// Set finalizer to close query after response serialization.
		SetFinalizer(ctx, qry.Close)

		res := qry.Exec(ctx)
		if res.Err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "query execution error", res.Err)
		}

		if limit > 0 {
			var isTruncated bool
			res, isTruncated = truncateResults(res, limit)
			if isTruncated {
				res.Warnings = res.Warnings.Add(errors.New("results truncated due to limit"))
			}
		}

		sr := api.statsRenderer
		if sr == nil {
			sr = DefaultStatsRenderer
		}
		qs := sr(ctx, qry.Stats(), statsParam)

		out := &QueryOutput{}
		out.Body.Status = "success"
		out.Body.Data = &QueryData{
			ResultType: res.Value.Type(),
			Result:     res.Value,
			Stats:      qs,
		}
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *QueryInput) (*QueryOutput, error) {
		return executeQuery(ctx, input.Query, input.Time.Time, input.Limit, input.Stats)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *QueryPostInput) (*QueryOutput, error) {
		return executeQuery(ctx, input.Body.Query, input.Body.Time.Time, input.Body.Limit, input.Body.Stats)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query",
		Method:      http.MethodGet,
		Path:        "/query",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-post",
		Method:      http.MethodPost,
		Path:        "/query",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

// Schema implements huma.SchemaProvider for QueryData to properly describe the result field.
func (q QueryData) Schema(r huma.Registry) *huma.Schema {
	minItems2 := 2
	maxItems2 := 2

	return &huma.Schema{
		OneOf: []*huma.Schema{
			{
				Type:        huma.TypeObject,
				Description: "Matrix result: array of time series with values or histograms",
				Properties: map[string]*huma.Schema{
					"resultType": {
						Type:        huma.TypeString,
						Enum:        []any{"matrix"},
						Description: "The type of the result",
					},
					"result": {
						Type:        huma.TypeArray,
						Description: "Array of time series, each with a metric and either values or histograms",
						Items: &huma.Schema{
							Type: huma.TypeObject,
							Properties: map[string]*huma.Schema{
								"metric": {
									Type:        huma.TypeObject,
									Description: "Label set for this series",
								},
								"values": {
									Type: huma.TypeArray,
									Items: &huma.Schema{
										Type:     huma.TypeArray,
										MinItems: &minItems2,
										MaxItems: &maxItems2,
									},
									Description: "Array of [timestamp, value] pairs",
								},
								"histograms": {
									Type:        huma.TypeArray,
									Description: "Array of histogram data points",
								},
							},
							Required:             []string{"metric"},
							AdditionalProperties: false,
						},
					},
					"stats": {
						Type:        huma.TypeObject,
						Description: "Query statistics (optional)",
					},
				},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: false,
			},
			{
				Type:        huma.TypeObject,
				Description: "Vector result: array of instant samples with value or histogram",
				Properties: map[string]*huma.Schema{
					"resultType": {
						Type:        huma.TypeString,
						Enum:        []any{"vector"},
						Description: "The type of the result",
					},
					"result": {
						Type:        huma.TypeArray,
						Description: "Array of instant samples, each with a metric and either a value or histogram",
						Items: &huma.Schema{
							Type: huma.TypeObject,
							Properties: map[string]*huma.Schema{
								"metric": {
									Type:        huma.TypeObject,
									Description: "Label set for this sample",
								},
								"value": {
									Type:        huma.TypeArray,
									MinItems:    &minItems2,
									MaxItems:    &maxItems2,
									Description: "Single [timestamp, value] pair",
								},
								"histogram": {
									Type:        huma.TypeObject,
									Description: "Histogram data",
								},
							},
							Required:             []string{"metric"},
							AdditionalProperties: false,
						},
					},
					"stats": {
						Type:        huma.TypeObject,
						Description: "Query statistics (optional)",
					},
				},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: false,
			},
			{
				Type:        huma.TypeObject,
				Description: "Scalar result: single [timestamp, value] pair",
				Properties: map[string]*huma.Schema{
					"resultType": {
						Type:        huma.TypeString,
						Enum:        []any{"scalar"},
						Description: "The type of the result",
					},
					"result": {
						Type:        huma.TypeArray,
						Description: "[timestamp, value] pair",
						MinItems:    &minItems2,
						MaxItems:    &maxItems2,
						Items: &huma.Schema{
							OneOf: []*huma.Schema{
								{Type: huma.TypeNumber},
								{Type: huma.TypeString},
							},
						},
					},
					"stats": {
						Type:        huma.TypeObject,
						Description: "Query statistics (optional)",
					},
				},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: false,
			},
			{
				Type:        huma.TypeObject,
				Description: "String result: single [timestamp, string_value] pair",
				Properties: map[string]*huma.Schema{
					"resultType": {
						Type:        huma.TypeString,
						Enum:        []any{"string"},
						Description: "The type of the result",
					},
					"result": {
						Type:        huma.TypeArray,
						Description: "[timestamp, string_value] pair",
						MinItems:    &minItems2,
						MaxItems:    &maxItems2,
						Items: &huma.Schema{
							OneOf: []*huma.Schema{
								{Type: huma.TypeNumber},
								{Type: huma.TypeString},
							},
						},
					},
					"stats": {
						Type:        huma.TypeObject,
						Description: "Query statistics (optional)",
					},
				},
				Required:             []string{"resultType", "result"},
				AdditionalProperties: false,
			},
		},
		Discriminator: &huma.Discriminator{
			PropertyName: "resultType",
			Mapping: map[string]string{
				"matrix": "#/components/schemas/QueryData/oneOf/0",
				"vector": "#/components/schemas/QueryData/oneOf/1",
				"scalar": "#/components/schemas/QueryData/oneOf/2",
				"string": "#/components/schemas/QueryData/oneOf/3",
			},
		},
	}
}

type QueryRangeInput struct {
	Limit int      `required:"false" default:"0" minimum:"0" query:"limit" doc:"The maximum number of metrics to return." example:"100"`
	Start Time     `required:"true" query:"start" minLength:"1" doc:"The start time of the query." example:"2021-01-01T00:00:00Z"`
	End   Time     `required:"true" query:"end" minLength:"1" doc:"The end time of the query." example:"2021-01-01T00:00:00Z"`
	Step  Duration `required:"true" query:"step" minLength:"1" doc:"The step size of the query." example:"1h"`
	Query string   `required:"true" query:"query" minLength:"1" doc:"The query to execute." example:"up"`
}

type QueryRangePostInput struct {
	Body struct {
		Limit int      `required:"false" default:"0" minimum:"0" json:"limit,omitempty" schema:"limit" doc:"The maximum number of metrics to return." example:"100"`
		Start Time     `required:"true" json:"start" schema:"start" minLength:"1" doc:"The start time of the query." example:"2021-01-01T00:00:00Z"`
		End   Time     `required:"true" json:"end" schema:"end" minLength:"1" doc:"The end time of the query." example:"2021-01-01T00:00:00Z"`
		Step  Duration `required:"true" json:"step" schema:"step" minLength:"1" doc:"The step size of the query." example:"1h"`
		Query string   `required:"true" json:"query" schema:"query" minLength:"1" doc:"The query to execute." example:"up"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type QueryRangeOutput struct {
	Body struct {
		Status string     `json:"status"`
		Data   *QueryData `json:"data"`
	}
}

func (api *API) registerQueryRange() {
	// Shared implementation for query range execution.
	executeQueryRange := func(ctx context.Context, query string, start time.Time, end time.Time, step time.Duration, limit int) (*QueryRangeOutput, error) {
		if end.Before(start) {
			return nil, huma.NewError(http.StatusBadRequest, "end timestamp must not be before start time", errors.New("invalid time range"))
		}

		if step <= 0 {
			return nil, huma.NewError(http.StatusBadRequest, "zero or negative query resolution step widths are not accepted. Try a positive integer", errors.New("invalid step"))
		}

		// For safety, limit the number of returned points per timeseries.
		// This is sufficient for 60s resolution for a week or 1h resolution for a year.
		if end.Sub(start)/step > 11000 {
			err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
			return nil, huma.NewError(http.StatusBadRequest, "bad_data", err)
		}

		opts := promql.NewPrometheusQueryOpts(false, 0)
		qry, err := api.QueryEngine.NewRangeQuery(ctx, api.Queryable, opts, query, start, end, step)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid query", err)
		}
		// Set finalizer to close query after response serialization.
		SetFinalizer(ctx, qry.Close)

		res := qry.Exec(ctx)
		if res.Err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "query execution error", res.Err)
		}

		if limit > 0 {
			var isTruncated bool
			res, isTruncated = truncateResults(res, limit)
			if isTruncated {
				res.Warnings = res.Warnings.Add(errors.New("results truncated due to limit"))
			}
		}

		sr := api.statsRenderer
		if sr == nil {
			sr = DefaultStatsRenderer
		}
		qs := sr(ctx, qry.Stats(), "")

		out := &QueryRangeOutput{}
		out.Body.Status = "success"
		out.Body.Data = &QueryData{
			ResultType: res.Value.Type(),
			Result:     res.Value,
			Stats:      qs,
		}
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *QueryRangeInput) (*QueryRangeOutput, error) {
		return executeQueryRange(ctx, input.Query, input.Start.Time, input.End.Time, input.Step.Duration, input.Limit)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *QueryRangePostInput) (*QueryRangeOutput, error) {
		return executeQueryRange(ctx, input.Body.Query, input.Body.Start.Time, input.Body.End.Time, input.Body.Step.Duration, input.Body.Limit)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-range",
		Method:      http.MethodGet,
		Path:        "/query_range",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-range-post",
		Method:      http.MethodPost,
		Path:        "/query_range",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

type StatusConfigData struct {
	YAML string `json:"yaml" doc:"The YAML configuration of the Prometheus server."`
}

type StatusConfigOutput struct {
	Body struct {
		Status string           `json:"status"`
		Data   StatusConfigData `json:"data"`
	}
}

func (api *API) registerStatusConfig() {
	huma.Get(api.humaAPI, "/status/config", func(ctx context.Context, input *struct{}) (output *StatusConfigOutput, err error) {
		cfg := api.config()

		out := &StatusConfigOutput{}
		out.Body.Status = "success"
		out.Body.Data.YAML = cfg.String()
		return out, nil
	})
}

type StatusRuntimeInfoOutput struct {
	Body struct {
		Status string      `json:"status"`
		Data   RuntimeInfo `json:"data"`
	}
}

func (api *API) registerStatusRuntimeInfo() {
	huma.Get(api.humaAPI, "/status/runtimeinfo", func(ctx context.Context, input *struct{}) (output *StatusRuntimeInfoOutput, err error) {
		status, runtimeErr := api.runtimeInfo()
		if runtimeErr != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "error getting runtime info", runtimeErr)
		}

		out := &StatusRuntimeInfoOutput{}
		out.Body.Status = "success"
		out.Body.Data = status
		return out, nil
	})
}

type StatusBuildInfoOutput struct {
	Body struct {
		Status string             `json:"status"`
		Data   *PrometheusVersion `json:"data"`
	}
}

func (api *API) registerStatusBuildInfo() {
	huma.Get(api.humaAPI, "/status/buildinfo", func(ctx context.Context, input *struct{}) (output *StatusBuildInfoOutput, err error) {
		out := &StatusBuildInfoOutput{}
		out.Body.Status = "success"
		out.Body.Data = api.buildInfo
		return out, nil
	})
}

type StatusFlagsOutput struct {
	Body struct {
		Status string            `json:"status"`
		Data   map[string]string `json:"data"`
	}
}

func (api *API) registerStatusFlags() {
	huma.Get(api.humaAPI, "/status/flags", func(ctx context.Context, input *struct{}) (output *StatusFlagsOutput, err error) {
		out := &StatusFlagsOutput{}
		out.Body.Status = "success"
		out.Body.Data = api.flagsMap
		return out, nil
	})
}

type StatusTSDBInput struct {
	Limit int `required:"false" default:"10" minimum:"1" maximum:"10000" query:"limit" doc:"The maximum number of items to return per category." example:"10"`
}

type StatusTSDBOutput struct {
	Body struct {
		Status string     `json:"status"`
		Data   TSDBStatus `json:"data"`
	}
}

func (api *API) registerStatusTSDB() {
	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "status-tsdb",
		Method:      http.MethodGet,
		Path:        "/status/tsdb",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, func(ctx context.Context, input *StatusTSDBInput) (output *StatusTSDBOutput, err error) {
		limit := input.Limit
		if limit < 1 {
			return nil, huma.NewError(http.StatusBadRequest, "limit must be a positive number", errors.New("invalid limit"))
		}

		s, statErr := api.db.Stats(labels.MetricName, limit)
		if statErr != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "error getting stats", statErr)
		}

		metrics, gatherErr := api.gatherer.Gather()
		if gatherErr != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "error gathering runtime status", gatherErr)
		}

		chunkCount := int64(math.NaN())
		for _, mF := range metrics {
			if *mF.Name == "prometheus_tsdb_head_chunks" {
				m := mF.Metric[0]
				if m.Gauge != nil {
					chunkCount = int64(m.Gauge.GetValue())
					break
				}
			}
		}

		out := &StatusTSDBOutput{}
		out.Body.Status = "success"
		out.Body.Data = TSDBStatus{
			HeadStats: HeadStats{
				NumSeries:     s.NumSeries,
				ChunkCount:    chunkCount,
				MinTime:       s.MinTime,
				MaxTime:       s.MaxTime,
				NumLabelPairs: s.IndexPostingStats.NumLabelPairs,
			},
			SeriesCountByMetricName:     TSDBStatsFromIndexStats(s.IndexPostingStats.CardinalityMetricsStats),
			LabelValueCountByLabelName:  TSDBStatsFromIndexStats(s.IndexPostingStats.CardinalityLabelStats),
			MemoryInBytesByLabelName:    TSDBStatsFromIndexStats(s.IndexPostingStats.LabelValueStats),
			SeriesCountByLabelValuePair: TSDBStatsFromIndexStats(s.IndexPostingStats.LabelValuePairsStats),
		}
		return out, nil
	})
}

type StatusTSDBBlocksData struct {
	Blocks []tsdb.BlockMeta `json:"blocks"`
}

type StatusTSDBBlocksOutput struct {
	Body struct {
		Status string               `json:"status"`
		Data   StatusTSDBBlocksData `json:"data"`
	}
}

func (api *API) registerStatusTSDBBlocks() {
	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "status-tsdb-blocks",
		Method:      http.MethodGet,
		Path:        "/status/tsdb/blocks",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, func(ctx context.Context, input *struct{}) (output *StatusTSDBBlocksOutput, err error) {
		blockMetas, blockErr := api.db.BlockMetas()
		if blockErr != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "error getting block metadata", blockErr)
		}

		out := &StatusTSDBBlocksOutput{}
		out.Body.Status = "success"
		out.Body.Data.Blocks = blockMetas
		return out, nil
	})
}

type StatusWALReplayData struct {
	Min     int `json:"min" doc:"Minimum segment number."`
	Max     int `json:"max" doc:"Maximum segment number."`
	Current int `json:"current" doc:"Current segment number being replayed."`
}

type StatusWALReplayOutput struct {
	Body struct {
		Status string              `json:"status"`
		Data   StatusWALReplayData `json:"data"`
	}
}

func (api *API) registerStatusWALReplay() {
	huma.Get(api.humaAPI, "/status/walreplay", func(ctx context.Context, input *struct{}) (output *StatusWALReplayOutput, err error) {
		status, walErr := api.db.WALReplayStatus()
		if walErr != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "error getting WAL replay status", walErr)
		}

		out := &StatusWALReplayOutput{}
		out.Body.Status = "success"
		out.Body.Data.Min = status.Min
		out.Body.Data.Max = status.Max
		out.Body.Data.Current = status.Current
		return out, nil
	})
}

type LabelsInput struct {
	Start Time     `required:"false" query:"start" doc:"Start timestamp for label names query." example:"2021-01-01T00:00:00Z"`
	End   Time     `required:"false" query:"end" doc:"End timestamp for label names query." example:"2021-01-02T00:00:00Z"`
	Match []string `required:"false" query:"match[]" doc:"Series selector argument that selects the series from which to read the label names." example:"up"`
	Limit int      `required:"false" default:"0" query:"limit" doc:"Maximum number of label names to return (0 means no limit)." example:"1000"`
}

type LabelsPostInput struct {
	Body struct {
		Start Time     `required:"false" json:"start,omitempty" schema:"start" doc:"Start timestamp for label names query." example:"2021-01-01T00:00:00Z"`
		End   Time     `required:"false" json:"end,omitempty" schema:"end" doc:"End timestamp for label names query." example:"2021-01-02T00:00:00Z"`
		Match []string `required:"false" json:"match[],omitempty" schema:"match[]" doc:"Series selector argument that selects the series from which to read the label names." example:"up"`
		Limit int      `required:"false" default:"0" json:"limit,omitempty" schema:"limit" doc:"Maximum number of label names to return (0 means no limit)." example:"1000"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type LabelsOutput struct {
	Body struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
}

func (api *API) registerLabels() {
	// Shared implementation for label names execution.
	executeLabels := func(ctx context.Context, start Time, end Time, matchers []string, limit int) (*LabelsOutput, error) {
		// Set defaults.
		startTime := MinTime
		if !start.IsZero() {
			startTime = start.Time
		}
		endTime := MaxTime
		if !end.IsZero() {
			endTime = end.Time
		}

		// Validate limit.
		if limit < 0 {
			return nil, huma.NewError(http.StatusBadRequest, "limit must be non-negative", errors.New("invalid limit"))
		}

		// Parse matchers.
		matcherSets, err := parseMatchersParam(matchers)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid match[] parameter", err)
		}

		hints := &storage.LabelHints{
			Limit: toHintLimit(limit),
		}

		q, err := api.Queryable.Querier(timestamp.FromTime(startTime), timestamp.FromTime(endTime))
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying data", err)
		}
		// Set finalizer to close querier after response serialization.
		SetFinalizer(ctx, func() { q.Close() })

		var (
			names    []string
			warnings annotations.Annotations
		)

		if len(matcherSets) > 1 {
			labelNamesSet := make(map[string]struct{})

			for _, matchers := range matcherSets {
				vals, callWarnings, err := q.LabelNames(ctx, hints, matchers...)
				if err != nil {
					return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying label names", err)
				}

				warnings.Merge(callWarnings)
				for _, val := range vals {
					labelNamesSet[val] = struct{}{}
				}
			}

			// Convert the map to an array.
			names = make([]string, 0, len(labelNamesSet))
			for key := range labelNamesSet {
				names = append(names, key)
			}
			slices.Sort(names)
		} else {
			var matchersSlice []*labels.Matcher
			if len(matcherSets) == 1 {
				matchersSlice = matcherSets[0]
			}
			names, warnings, err = q.LabelNames(ctx, hints, matchersSlice...)
			if err != nil {
				return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying label names", err)
			}
		}

		if names == nil {
			names = []string{}
		}

		if limit > 0 && len(names) > limit {
			names = names[:limit]
			warnings = warnings.Add(errors.New("results truncated due to limit"))
		}

		out := &LabelsOutput{}
		out.Body.Status = "success"
		out.Body.Data = names
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *LabelsInput) (*LabelsOutput, error) {
		return executeLabels(ctx, input.Start, input.End, input.Match, input.Limit)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *LabelsPostInput) (*LabelsOutput, error) {
		return executeLabels(ctx, input.Body.Start, input.Body.End, input.Body.Match, input.Body.Limit)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "labels",
		Method:      http.MethodGet,
		Path:        "/labels",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "labels-post",
		Method:      http.MethodPost,
		Path:        "/labels",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

type LabelValuesInput struct {
	Name  string   `path:"name" doc:"Label name." example:"job"`
	Start Time     `required:"false" query:"start" doc:"Start timestamp for label values query." example:"2021-01-01T00:00:00Z"`
	End   Time     `required:"false" query:"end" doc:"End timestamp for label values query." example:"2021-01-02T00:00:00Z"`
	Match []string `required:"false" query:"match[]" doc:"Series selector argument that selects the series from which to read the label values." example:"up"`
	Limit int      `required:"false" default:"0" query:"limit" doc:"Maximum number of label values to return (0 means no limit)." example:"1000"`
}

type LabelValuesOutput struct {
	Body struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
}

func (api *API) registerLabelValues() {
	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "label-values",
		Method:      http.MethodGet,
		Path:        "/label/{name}/values",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, func(ctx context.Context, input *LabelValuesInput) (*LabelValuesOutput, error) {
		name := input.Name

		// Handle UTF-8 escaped label names.
		if strings.HasPrefix(name, "U__") {
			name = model.UnescapeName(name, model.ValueEncodingEscaping)
		}

		// Validate label name.
		if !model.UTF8Validation.IsValidLabelName(name) {
			return nil, huma.NewError(http.StatusBadRequest, fmt.Sprintf("invalid label name: %q", name), errors.New("invalid label name"))
		}

		// Validate limit.
		if input.Limit < 0 {
			return nil, huma.NewError(http.StatusBadRequest, "limit must be non-negative", errors.New("invalid limit"))
		}

		// Set defaults.
		startTime := MinTime
		if !input.Start.IsZero() {
			startTime = input.Start.Time
		}
		endTime := MaxTime
		if !input.End.IsZero() {
			endTime = input.End.Time
		}

		// Parse matchers.
		matcherSets, err := parseMatchersParam(input.Match)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid match[] parameter", err)
		}

		hints := &storage.LabelHints{
			Limit: toHintLimit(input.Limit),
		}

		q, err := api.Queryable.Querier(timestamp.FromTime(startTime), timestamp.FromTime(endTime))
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying data", err)
		}
		// Set finalizer to close querier after response serialization.
		SetFinalizer(ctx, func() { q.Close() })

		var (
			vals     []string
			warnings annotations.Annotations
		)

		if len(matcherSets) > 1 {
			var callWarnings annotations.Annotations
			labelValuesSet := make(map[string]struct{})
			for _, matchers := range matcherSets {
				vals, callWarnings, err = q.LabelValues(ctx, name, hints, matchers...)
				if err != nil {
					return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying label values", err)
				}
				warnings.Merge(callWarnings)
				for _, val := range vals {
					labelValuesSet[val] = struct{}{}
				}
			}

			vals = make([]string, 0, len(labelValuesSet))
			for val := range labelValuesSet {
				vals = append(vals, val)
			}
		} else {
			var matchers []*labels.Matcher
			if len(matcherSets) == 1 {
				matchers = matcherSets[0]
			}
			vals, warnings, err = q.LabelValues(ctx, name, hints, matchers...)
			if err != nil {
				return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying label values", err)
			}

			if vals == nil {
				vals = []string{}
			}
		}

		slices.Sort(vals)

		if input.Limit > 0 && len(vals) > input.Limit {
			vals = vals[:input.Limit]
			warnings = warnings.Add(errors.New("results truncated due to limit"))
		}

		out := &LabelValuesOutput{}
		out.Body.Status = "success"
		out.Body.Data = vals
		return out, nil
	})
}

type SeriesInput struct {
	Start Time     `required:"false" query:"start" doc:"Start timestamp for series query." example:"2021-01-01T00:00:00Z"`
	End   Time     `required:"false" query:"end" doc:"End timestamp for series query." example:"2021-01-02T00:00:00Z"`
	Match []string `required:"true" query:"match[]" doc:"Series selector argument that selects the series to return. At least one match[] argument is required." example:"up" minLength:"1"`
	Limit int      `required:"false" default:"0" query:"limit" doc:"Maximum number of series to return (0 means no limit)." example:"1000"`
}

type SeriesPostInput struct {
	Body struct {
		Start Time     `required:"false" json:"start,omitempty" schema:"start" doc:"Start timestamp for series query." example:"2021-01-01T00:00:00Z"`
		End   Time     `required:"false" json:"end,omitempty" schema:"end" doc:"End timestamp for series query." example:"2021-01-02T00:00:00Z"`
		Match []string `required:"true" json:"match[]" schema:"match[]" doc:"Series selector argument that selects the series to return. At least one match[] argument is required." example:"up" minLength:"1"`
		Limit int      `required:"false" default:"0" json:"limit,omitempty" schema:"limit" doc:"Maximum number of series to return (0 means no limit)." example:"1000"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type SeriesOutput struct {
	Body struct {
		Status string          `json:"status"`
		Data   []labels.Labels `json:"data"`
	}
}

func (api *API) registerSeries() {
	// Shared implementation for series query execution.
	executeSeries := func(ctx context.Context, start Time, end Time, matchers []string, limit int) (*SeriesOutput, error) {
		// Validate that at least one match[] is provided.
		if len(matchers) == 0 {
			return nil, huma.NewError(http.StatusBadRequest, "no match[] parameter provided", errors.New("match[] required"))
		}

		// Validate limit.
		if limit < 0 {
			return nil, huma.NewError(http.StatusBadRequest, "limit must be non-negative", errors.New("invalid limit"))
		}

		// Set defaults.
		startTime := MinTime
		if !start.IsZero() {
			startTime = start.Time
		}
		endTime := MaxTime
		if !end.IsZero() {
			endTime = end.Time
		}

		// Parse matchers.
		matcherSets, err := parseMatchersParam(matchers)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid match[] parameter", err)
		}

		q, err := api.Queryable.Querier(timestamp.FromTime(startTime), timestamp.FromTime(endTime))
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying data", err)
		}
		// Set finalizer to close querier after response serialization.
		SetFinalizer(ctx, func() { q.Close() })

		hints := &storage.SelectHints{
			Start: timestamp.FromTime(startTime),
			End:   timestamp.FromTime(endTime),
			Func:  "series", // There is no series function, this token is used for lookups that don't need samples.
			Limit: toHintLimit(limit),
		}
		var set storage.SeriesSet

		if len(matcherSets) > 1 {
			var sets []storage.SeriesSet
			for _, mset := range matcherSets {
				// We need to sort this select results to merge (deduplicate) the series sets later.
				s := q.Select(ctx, true, hints, mset...)
				sets = append(sets, s)
			}
			set = storage.NewMergeSeriesSet(sets, 0, storage.ChainedSeriesMerge)
		} else {
			// At this point at least one match exists.
			set = q.Select(ctx, false, hints, matcherSets[0]...)
		}

		metrics := []labels.Labels{}
		warnings := set.Warnings()

		i := 1
		for set.Next() {
			if i%checkContextEveryNIterations == 0 {
				if err := ctx.Err(); err != nil {
					return nil, huma.NewError(http.StatusUnprocessableEntity, "context error", err)
				}
			}
			i++

			metrics = append(metrics, set.At().Labels())

			if limit > 0 && len(metrics) > limit {
				metrics = metrics[:limit]
				warnings.Add(errors.New("results truncated due to limit"))
				break
			}
		}
		if set.Err() != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying series", set.Err())
		}

		out := &SeriesOutput{}
		out.Body.Status = "success"
		out.Body.Data = metrics
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *SeriesInput) (*SeriesOutput, error) {
		return executeSeries(ctx, input.Start, input.End, input.Match, input.Limit)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *SeriesPostInput) (*SeriesOutput, error) {
		return executeSeries(ctx, input.Body.Start, input.Body.End, input.Body.Match, input.Body.Limit)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "series",
		Method:      http.MethodGet,
		Path:        "/series",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "series-post",
		Method:      http.MethodPost,
		Path:        "/series",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

type QueryExemplarsInput struct {
	Start Time   `required:"false" query:"start" doc:"Start timestamp for exemplars query." example:"2021-01-01T00:00:00Z"`
	End   Time   `required:"false" query:"end" doc:"End timestamp for exemplars query." example:"2021-01-02T00:00:00Z"`
	Query string `required:"true" query:"query" minLength:"1" doc:"PromQL query to extract exemplars for." example:"http_requests_total"`
}

type QueryExemplarsPostInput struct {
	Body struct {
		Start Time   `required:"false" json:"start,omitempty" schema:"start" doc:"Start timestamp for exemplars query." example:"2021-01-01T00:00:00Z"`
		End   Time   `required:"false" json:"end,omitempty" schema:"end" doc:"End timestamp for exemplars query." example:"2021-01-02T00:00:00Z"`
		Query string `required:"true" json:"query" schema:"query" minLength:"1" doc:"PromQL query to extract exemplars for." example:"http_requests_total"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type QueryExemplarsOutput struct {
	Body struct {
		Status string `json:"status"`
		Data   any    `json:"data"`
	}
}

func (api *API) registerQueryExemplars() {
	// Shared implementation for exemplars query execution.
	executeQueryExemplars := func(ctx context.Context, start Time, end Time, query string) (*QueryExemplarsOutput, error) {
		// Set defaults.
		startTime := MinTime
		if !start.IsZero() {
			startTime = start.Time
		}
		endTime := MaxTime
		if !end.IsZero() {
			endTime = end.Time
		}

		// Validate time range.
		if endTime.Before(startTime) {
			return nil, huma.NewError(http.StatusBadRequest, "end timestamp must not be before start timestamp", errors.New("invalid time range"))
		}

		// Parse the query expression.
		expr, err := parser.ParseExpr(query)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid query", err)
		}

		// Extract selectors from the expression.
		selectors := parser.ExtractSelectors(expr)
		if len(selectors) < 1 {
			out := &QueryExemplarsOutput{}
			out.Body.Status = "success"
			out.Body.Data = nil
			return out, nil
		}

		// Get exemplar querier.
		eq, err := api.ExemplarQueryable.ExemplarQuerier(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "error creating exemplar querier", err)
		}

		// Query exemplars.
		res, err := eq.Select(timestamp.FromTime(startTime), timestamp.FromTime(endTime), selectors...)
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "error querying exemplars", err)
		}

		out := &QueryExemplarsOutput{}
		out.Body.Status = "success"
		out.Body.Data = res
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *QueryExemplarsInput) (*QueryExemplarsOutput, error) {
		return executeQueryExemplars(ctx, input.Start, input.End, input.Query)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *QueryExemplarsPostInput) (*QueryExemplarsOutput, error) {
		return executeQueryExemplars(ctx, input.Body.Start, input.Body.End, input.Body.Query)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-exemplars",
		Method:      http.MethodGet,
		Path:        "/query_exemplars",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-exemplars-post",
		Method:      http.MethodPost,
		Path:        "/query_exemplars",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

type FormatQueryInput struct {
	Query string `required:"true" query:"query" minLength:"1" doc:"PromQL expression to format." example:"sum(rate(http_requests_total[5m]))"`
}

type FormatQueryPostInput struct {
	Body struct {
		Query string `required:"true" json:"query" schema:"query" minLength:"1" doc:"PromQL expression to format." example:"sum(rate(http_requests_total[5m]))"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type FormatQueryOutput struct {
	Body struct {
		Status string `json:"status"`
		Data   string `json:"data"`
	}
}

func (api *API) registerFormatQuery() {
	// Shared implementation for format query execution.
	executeFormatQuery := func(ctx context.Context, query string) (*FormatQueryOutput, error) {
		expr, err := parser.ParseExpr(query)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid query", err)
		}

		out := &FormatQueryOutput{}
		out.Body.Status = "success"
		out.Body.Data = expr.Pretty(0)
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *FormatQueryInput) (*FormatQueryOutput, error) {
		return executeFormatQuery(ctx, input.Query)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *FormatQueryPostInput) (*FormatQueryOutput, error) {
		return executeFormatQuery(ctx, input.Body.Query)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "format-query",
		Method:      http.MethodGet,
		Path:        "/format_query",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "format-query-post",
		Method:      http.MethodPost,
		Path:        "/format_query",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

type ParseQueryInput struct {
	Query string `required:"true" query:"query" minLength:"1" doc:"PromQL expression to parse." example:"sum(rate(http_requests_total[5m]))"`
}

type ParseQueryPostInput struct {
	Body struct {
		Query string `required:"true" json:"query" schema:"query" minLength:"1" doc:"PromQL expression to parse." example:"sum(rate(http_requests_total[5m]))"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type ParseQueryOutput struct {
	Body struct {
		Status string `json:"status"`
		Data   any    `json:"data"`
	}
}

func (api *API) registerParseQuery() {
	// Shared implementation for parse query execution.
	executeParseQuery := func(ctx context.Context, query string) (*ParseQueryOutput, error) {
		expr, err := parser.ParseExpr(query)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid query", err)
		}

		out := &ParseQueryOutput{}
		out.Body.Status = "success"
		out.Body.Data = translateAST(expr)
		return out, nil
	}

	// GET handler using query parameters.
	getHandler := func(ctx context.Context, input *ParseQueryInput) (*ParseQueryOutput, error) {
		return executeParseQuery(ctx, input.Query)
	}

	// POST handler using form data.
	postHandler := func(ctx context.Context, input *ParseQueryPostInput) (*ParseQueryOutput, error) {
		return executeParseQuery(ctx, input.Body.Query)
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "parse-query",
		Method:      http.MethodGet,
		Path:        "/parse_query",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, getHandler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "parse-query-post",
		Method:      http.MethodPost,
		Path:        "/parse_query",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, postHandler)
}

type ScrapePoolsOutput struct {
	Body struct {
		Status string               `json:"status"`
		Data   ScrapePoolsDiscovery `json:"data"`
	}
}

func (api *API) registerScrapePools() {
	huma.Get(api.humaAPI, "/scrape_pools", func(ctx context.Context, input *struct{}) (*ScrapePoolsOutput, error) {
		names := api.scrapePoolsRetriever(ctx).ScrapePools()
		slices.Sort(names)

		out := &ScrapePoolsOutput{}
		out.Body.Status = "success"
		out.Body.Data = ScrapePoolsDiscovery{ScrapePools: names}
		return out, nil
	})
}

type AlertmanagersOutput struct {
	Body struct {
		Status string                `json:"status"`
		Data   AlertmanagerDiscovery `json:"data"`
	}
}

func (api *API) registerAlertmanagers() {
	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "alertmanagers",
		Method:      http.MethodGet,
		Path:        "/alertmanagers",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, func(ctx context.Context, input *struct{}) (*AlertmanagersOutput, error) {
		urls := api.alertmanagerRetriever(ctx).Alertmanagers()
		droppedURLS := api.alertmanagerRetriever(ctx).DroppedAlertmanagers()

		ams := &AlertmanagerDiscovery{
			ActiveAlertmanagers:  make([]*AlertmanagerTarget, len(urls)),
			DroppedAlertmanagers: make([]*AlertmanagerTarget, len(droppedURLS)),
		}

		for i, url := range urls {
			ams.ActiveAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
		}
		for i, url := range droppedURLS {
			ams.DroppedAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
		}

		out := &AlertmanagersOutput{}
		out.Body.Status = "success"
		out.Body.Data = *ams
		return out, nil
	})
}

type TargetsInput struct {
	ScrapePool string `required:"false" query:"scrapePool" doc:"Filter targets by scrape pool name." example:"prometheus"`
	State      string `required:"false" query:"state" doc:"Filter by state: active, dropped, or any (default: any)." example:"active"`
}

type TargetsOutput struct {
	Body struct {
		Status string          `json:"status"`
		Data   TargetDiscovery `json:"data"`
	}
}

func (api *API) registerTargets() {
	huma.Get(api.humaAPI, "/targets", func(ctx context.Context, input *TargetsInput) (*TargetsOutput, error) {
		getSortedPools := func(targets map[string][]*scrape.Target) ([]string, int) {
			var n int
			pools := make([]string, 0, len(targets))
			for p, t := range targets {
				pools = append(pools, p)
				n += len(t)
			}
			slices.Sort(pools)
			return pools, n
		}

		scrapePool := input.ScrapePool
		state := strings.ToLower(input.State)
		showActive := state == "" || state == "any" || state == "active"
		showDropped := state == "" || state == "any" || state == "dropped"
		res := &TargetDiscovery{}
		builder := labels.NewBuilder(labels.EmptyLabels())

		if showActive {
			targetsActive := api.targetRetriever(ctx).TargetsActive()
			activePools, numTargets := getSortedPools(targetsActive)
			res.ActiveTargets = make([]*Target, 0, numTargets)

			for _, pool := range activePools {
				if scrapePool != "" && pool != scrapePool {
					continue
				}
				for _, target := range targetsActive[pool] {
					lastErrStr := ""
					lastErr := target.LastError()
					if lastErr != nil {
						lastErrStr = lastErr.Error()
					}

					globalURL, err := getGlobalURL(target.URL(), api.globalURLOptions)

					res.ActiveTargets = append(res.ActiveTargets, &Target{
						DiscoveredLabels: target.DiscoveredLabels(builder),
						Labels:           target.Labels(builder),
						ScrapePool:       pool,
						ScrapeURL:        target.URL().String(),
						GlobalURL:        globalURL.String(),
						LastError: func() string {
							switch {
							case err == nil && lastErrStr == "":
								return ""
							case err != nil:
								return fmt.Errorf("%s: %w", lastErrStr, err).Error()
							default:
								return lastErrStr
							}
						}(),
						LastScrape:         target.LastScrape(),
						LastScrapeDuration: target.LastScrapeDuration().Seconds(),
						Health:             target.Health(),
						ScrapeInterval:     target.GetValue(model.ScrapeIntervalLabel),
						ScrapeTimeout:      target.GetValue(model.ScrapeTimeoutLabel),
					})
				}
			}
		} else {
			res.ActiveTargets = []*Target{}
		}

		if showDropped {
			res.DroppedTargetCounts = NullableMap(api.targetRetriever(ctx).TargetsDroppedCounts())

			targetsDropped := api.targetRetriever(ctx).TargetsDropped()
			droppedPools, numTargets := getSortedPools(targetsDropped)
			res.DroppedTargets = make([]*DroppedTarget, 0, numTargets)
			for _, pool := range droppedPools {
				if scrapePool != "" && pool != scrapePool {
					continue
				}
				for _, target := range targetsDropped[pool] {
					res.DroppedTargets = append(res.DroppedTargets, &DroppedTarget{
						DiscoveredLabels: target.DiscoveredLabels(builder),
						ScrapePool:       pool,
					})
				}
			}
		} else {
			res.DroppedTargets = []*DroppedTarget{}
		}

		out := &TargetsOutput{}
		out.Body.Status = "success"
		out.Body.Data = *res
		return out, nil
	})
}

type TargetMetadataInput struct {
	MatchTarget string `required:"false" query:"match_target" doc:"Label selector to filter targets." example:"{job=\"prometheus\"}"`
	Metric      string `required:"false" query:"metric" doc:"Metric name to retrieve metadata for." example:"http_requests_total"`
	Limit       int    `required:"false" default:"-1" query:"limit" doc:"Maximum number of targets to match (default: no limit)." example:"10"`
}

type TargetMetadataOutput struct {
	Body struct {
		Status string           `json:"status"`
		Data   []metricMetadata `json:"data"`
	}
}

func (api *API) registerTargetMetadata() {
	huma.Get(api.humaAPI, "/targets/metadata", func(ctx context.Context, input *TargetMetadataInput) (*TargetMetadataOutput, error) {
		limit := input.Limit

		var matchers []*labels.Matcher
		var err error
		if input.MatchTarget != "" {
			matchers, err = parser.ParseMetricSelector(input.MatchTarget)
			if err != nil {
				return nil, huma.NewError(http.StatusBadRequest, "invalid match_target parameter", err)
			}
		}

		builder := labels.NewBuilder(labels.EmptyLabels())
		metric := input.Metric
		res := []metricMetadata{}

		for _, tt := range api.targetRetriever(ctx).TargetsActive() {
			for _, t := range tt {
				if limit >= 0 && len(res) >= limit {
					break
				}
				targetLabels := t.Labels(builder)
				// Filter targets that don't satisfy the label matchers.
				if input.MatchTarget != "" && !matchLabels(targetLabels, matchers) {
					continue
				}
				// If no metric is specified, get the full list for the target.
				if metric == "" {
					for _, md := range t.ListMetadata() {
						res = append(res, metricMetadata{
							Target:       targetLabels,
							MetricFamily: md.MetricFamily,
							Type:         md.Type,
							Help:         md.Help,
							Unit:         md.Unit,
						})
					}
					continue
				}
				// Get metadata for the specified metric.
				if md, ok := t.GetMetadata(metric); ok {
					res = append(res, metricMetadata{
						Target: targetLabels,
						Type:   md.Type,
						Help:   md.Help,
						Unit:   md.Unit,
					})
				}
			}
		}

		out := &TargetMetadataOutput{}
		out.Body.Status = "success"
		out.Body.Data = res
		return out, nil
	})
}

type AlertsOutput struct {
	Body struct {
		Status string         `json:"status"`
		Data   AlertDiscovery `json:"data"`
	}
}

func (api *API) registerAlerts() {
	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "alerts",
		Method:      http.MethodGet,
		Path:        "/alerts",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, func(ctx context.Context, input *struct{}) (*AlertsOutput, error) {
		alertingRules := api.rulesRetriever(ctx).AlertingRules()
		alerts := []*Alert{}

		for _, alertingRule := range alertingRules {
			alerts = append(
				alerts,
				rulesAlertsToAPIAlerts(alertingRule.ActiveAlerts())...,
			)
		}

		out := &AlertsOutput{}
		out.Body.Status = "success"
		out.Body.Data = AlertDiscovery{Alerts: alerts}
		return out, nil
	})
}

type RulesInput struct {
	Type            string   `required:"false" query:"type" doc:"Filter by rule type: alert or record." example:"alert"`
	RuleName        []string `required:"false" query:"rule_name[]" doc:"Filter by rule name." example:"HighRequestLatency"`
	RuleGroup       []string `required:"false" query:"rule_group[]" doc:"Filter by rule group name." example:"prometheus"`
	File            []string `required:"false" query:"file[]" doc:"Filter by file path." example:"/etc/prometheus/rules.yml"`
	Match           []string `required:"false" query:"match[]" doc:"Label matchers to filter rules." example:"{job=\"prometheus\"}"`
	ExcludeAlerts   string   `required:"false" query:"exclude_alerts" doc:"Exclude active alerts from response (true or false)." example:"false"`
	GroupLimit      int64    `required:"false" default:"-1" query:"group_limit" doc:"Maximum number of rule groups to return (-1 for unlimited)." example:"10"`
	GroupNextToken  string   `required:"false" query:"group_next_token" doc:"Pagination token for next page of rule groups." example:"abc123"`
}

type RulesOutput struct {
	Body struct {
		Status string        `json:"status"`
		Data   RuleDiscovery `json:"data"`
	}
}

func (api *API) registerRules() {
	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "rules",
		Method:      http.MethodGet,
		Path:        "/rules",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, func(ctx context.Context, input *RulesInput) (*RulesOutput, error) {
		queryFormToSet := func(values []string) map[string]struct{} {
			set := make(map[string]struct{}, len(values))
			for _, v := range values {
				set[v] = struct{}{}
			}
			return set
		}

		rnSet := queryFormToSet(input.RuleName)
		rgSet := queryFormToSet(input.RuleGroup)
		fSet := queryFormToSet(input.File)

		matcherSets, err := parseMatchersParam(input.Match)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "invalid match[] parameter", err)
		}

		ruleGroups := api.rulesRetriever(ctx).RuleGroups()
		res := &RuleDiscovery{RuleGroups: make([]*RuleGroup, 0, len(ruleGroups))}
		typ := strings.ToLower(input.Type)

		if typ != "" && typ != "alert" && typ != "record" {
			return nil, huma.NewError(http.StatusBadRequest, fmt.Sprintf("not supported value %q for type parameter", typ), errors.New("invalid type"))
		}

		returnAlerts := typ == "" || typ == "alert"
		returnRecording := typ == "" || typ == "record"

		// Parse exclude_alerts parameter.
		excludeAlerts := false
		if input.ExcludeAlerts != "" {
			excludeAlertsParam := strings.ToLower(input.ExcludeAlerts)
			if excludeAlertsParam != "true" && excludeAlertsParam != "false" {
				return nil, huma.NewError(http.StatusBadRequest, "exclude_alerts must be 'true' or 'false'", errors.New("invalid exclude_alerts"))
			}
			excludeAlerts = excludeAlertsParam == "true"
		}

		// Parse pagination parameters.
		maxGroups := input.GroupLimit
		nextToken := input.GroupNextToken

		if nextToken != "" && maxGroups <= 0 {
			return nil, huma.NewError(http.StatusBadRequest, "group_limit needs to be present in order to paginate over the groups", errors.New("missing group_limit"))
		}

		if maxGroups <= 0 {
			maxGroups = -1
		}

		rgs := make([]*RuleGroup, 0, len(ruleGroups))
		foundToken := false

		for _, grp := range ruleGroups {
			if maxGroups > 0 && nextToken != "" && !foundToken {
				if nextToken != getRuleGroupNextToken(grp.File(), grp.Name()) {
					continue
				}
				foundToken = true
			}

			if len(rgSet) > 0 {
				if _, ok := rgSet[grp.Name()]; !ok {
					continue
				}
			}

			if len(fSet) > 0 {
				if _, ok := fSet[grp.File()]; !ok {
					continue
				}
			}

			apiRuleGroup := &RuleGroup{
				Name:           grp.Name(),
				File:           grp.File(),
				Interval:       grp.Interval().Seconds(),
				Limit:          grp.Limit(),
				Rules:          []Rule{},
				EvaluationTime: grp.GetEvaluationTime().Seconds(),
				LastEvaluation: grp.GetLastEvaluation(),
			}

			for _, rr := range grp.Rules(matcherSets...) {
				var enrichedRule Rule

				if len(rnSet) > 0 {
					if _, ok := rnSet[rr.Name()]; !ok {
						continue
					}
				}

				lastError := ""
				if rr.LastError() != nil {
					lastError = rr.LastError().Error()
				}
				switch rule := rr.(type) {
				case *rules.AlertingRule:
					if !returnAlerts {
						break
					}
					var activeAlerts []*Alert
					if !excludeAlerts {
						activeAlerts = rulesAlertsToAPIAlerts(rule.ActiveAlerts())
					}

					enrichedRule = AlertingRule{
						State:          rule.State().String(),
						Name:           rule.Name(),
						Query:          rule.Query().String(),
						Duration:       rule.HoldDuration().Seconds(),
						KeepFiringFor:  rule.KeepFiringFor().Seconds(),
						Labels:         rule.Labels(),
						Annotations:    rule.Annotations(),
						Alerts:         activeAlerts,
						Health:         rule.Health(),
						LastError:      lastError,
						EvaluationTime: rule.GetEvaluationDuration().Seconds(),
						LastEvaluation: rule.GetEvaluationTimestamp(),
						Type:           "alerting",
					}

				case *rules.RecordingRule:
					if !returnRecording {
						break
					}
					enrichedRule = RecordingRule{
						Name:           rule.Name(),
						Query:          rule.Query().String(),
						Labels:         rule.Labels(),
						Health:         rule.Health(),
						LastError:      lastError,
						EvaluationTime: rule.GetEvaluationDuration().Seconds(),
						LastEvaluation: rule.GetEvaluationTimestamp(),
						Type:           "recording",
					}
				default:
					return nil, huma.NewError(http.StatusInternalServerError, fmt.Sprintf("failed to assert type of rule '%v'", rule.Name()), errors.New("internal error"))
				}

				if enrichedRule != nil {
					apiRuleGroup.Rules = append(apiRuleGroup.Rules, enrichedRule)
				}
			}

			// If the rule group response has no rules, skip it - this means we filtered all the rules of this group.
			if len(apiRuleGroup.Rules) > 0 {
				if maxGroups > 0 && len(rgs) == int(maxGroups) {
					// We've reached the capacity of our page plus one. That means that for sure there will be at least one
					// rule group in a subsequent request. Therefore a next token is required.
					res.GroupNextToken = getRuleGroupNextToken(grp.File(), grp.Name())
					break
				}
				rgs = append(rgs, apiRuleGroup)
			}
		}

		if maxGroups > 0 && nextToken != "" && !foundToken {
			return nil, huma.NewError(http.StatusBadRequest, fmt.Sprintf("invalid group_next_token '%v'. were rule groups changed?", nextToken), errors.New("invalid token"))
		}

		res.RuleGroups = rgs

		out := &RulesOutput{}
		out.Body.Status = "success"
		out.Body.Data = *res
		return out, nil
	})
}

type NotificationsOutput struct {
	Body struct {
		Status string                       `json:"status"`
		Data   []notifications.Notification `json:"data"`
	}
}

func (api *API) registerNotifications() {
	huma.Get(api.humaAPI, "/notifications", func(ctx context.Context, input *struct{}) (output *NotificationsOutput, err error) {
		out := &NotificationsOutput{}
		out.Body.Status = "success"
		out.Body.Data = api.notificationsGetter()
		return out, nil
	})
}

// registerNotificationsLive registers the /notifications/live SSE endpoint.
func (api *API) registerNotificationsLive() {
	// Map event types to their data structures for OpenAPI documentation.
	eventTypeMap := map[string]any{
		"message": notifications.Notification{},
	}

	sse.Register(api.humaAPI, huma.Operation{
		OperationID: "notifications-live",
		Method:      http.MethodGet,
		Path:        "/notifications/live",
		Summary:     "Stream live notifications via Server-Sent Events",
		Description: "Subscribe to real-time server notifications using Server-Sent Events (SSE). The connection remains open and sends notification events as they occur.",
		Tags:        []string{"notifications"},
		Middlewares: huma.Middlewares{SSEMiddleware()},
	}, eventTypeMap, func(ctx context.Context, input *struct{}, send sse.Sender) {
		// Get the flusher from context and perform initial flush.
		// This ensures headers are sent immediately and triggers the client's EventSource 'onopen' event.
		if flusher, ok := getFlusher(ctx); ok {
			flusher.Flush()
		}

		// Subscribe to notifications.
		notificationsChan, unsubscribe, ok := api.notificationsSub()
		if !ok {
			// Subscriber limit reached or notifications not available.
			return
		}
		defer unsubscribe()

		// Stream notifications until the client disconnects.
		for {
			select {
			case notification := <-notificationsChan:
				// Send the notification as an SSE message.
				err := send.Data(notification)
				if err != nil {
					// Client disconnected or error sending.
					return
				}
			case <-ctx.Done():
				// Client disconnected.
				return
			}
		}
	})
}

type TargetRelabelStepsInput struct {
	ScrapePool string `required:"true" query:"scrapePool" doc:"Name of the scrape pool."`
	Labels     string `required:"true" query:"labels" doc:"JSON-encoded labels to apply relabel rules to."`
}

type TargetRelabelStepsOutput struct {
	Body struct {
		Status string               `json:"status"`
		Data   RelabelStepsResponse `json:"data"`
	}
}

func (api *API) registerTargetRelabelSteps() {
	huma.Get(api.humaAPI, "/targets/relabel_steps", func(ctx context.Context, input *TargetRelabelStepsInput) (output *TargetRelabelStepsOutput, err error) {
		json := jsoniter.ConfigCompatibleWithStandardLibrary
		var lbls labels.Labels
		if err := json.Unmarshal([]byte(input.Labels), &lbls); err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "error parsing labels", err)
		}

		scrapeConfig, err := api.targetRetriever(ctx).ScrapePoolConfig(input.ScrapePool)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "error retrieving scrape config", err)
		}

		rules := scrapeConfig.RelabelConfigs
		steps := make([]RelabelStep, len(rules))
		for i, rule := range rules {
			outLabels, keep := relabel.Process(lbls, rules[:i+1]...)
			steps[i] = RelabelStep{
				Rule:   rule,
				Output: outLabels,
				Keep:   keep,
			}
		}

		out := &TargetRelabelStepsOutput{}
		out.Body.Status = "success"
		out.Body.Data = RelabelStepsResponse{Steps: steps}
		return out, nil
	})
}

type DeleteSeriesInput struct {
	Match []string `required:"true" query:"match[]" doc:"Series selectors to identify series to delete."`
	Start Time     `required:"false" query:"start" doc:"Start timestamp for deletion."`
	End   Time     `required:"false" query:"end" doc:"End timestamp for deletion."`
}

type DeleteSeriesOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (api *API) registerDeleteSeries() {
	adminMiddleware := func(ctx huma.Context, next func(huma.Context)) {
		if !api.enableAdmin {
			huma.WriteErr(api.humaAPI, ctx, http.StatusServiceUnavailable, "admin APIs disabled")
			return
		}
		next(ctx)
	}

	executeDeleteSeries := func(ctx context.Context, input *DeleteSeriesInput) (*DeleteSeriesOutput, error) {
		startTime := MinTime
		if !input.Start.IsZero() {
			startTime = input.Start.Time
		}
		endTime := MaxTime
		if !input.End.IsZero() {
			endTime = input.End.Time
		}

		for _, s := range input.Match {
			matchers, err := parser.ParseMetricSelector(s)
			if err != nil {
				return nil, huma.NewError(http.StatusBadRequest, "invalid match[] parameter", err)
			}
			if err := api.db.Delete(ctx, timestamp.FromTime(startTime), timestamp.FromTime(endTime), matchers...); err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "error deleting series", err)
			}
		}

		out := &DeleteSeriesOutput{}
		out.Body.Status = "success"
		return out, nil
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "deleteSeriesPost",
		Method:      http.MethodPost,
		Path:        "/admin/tsdb/delete_series",
		Summary:     "Delete series matching selectors",
		Description: "Deletes data for a selection of series in a time range. Accepts form parameters via POST request body.",
		Tags:        []string{"admin"},
		Middlewares: huma.Middlewares{adminMiddleware},
	}, func(ctx context.Context, input *DeleteSeriesInput) (*DeleteSeriesOutput, error) {
		return executeDeleteSeries(ctx, input)
	})

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "deleteSeriesPut",
		Method:      http.MethodPut,
		Path:        "/admin/tsdb/delete_series",
		Summary:     "Delete series matching selectors",
		Description: "Deletes data for a selection of series in a time range. Accepts query parameters via PUT request.",
		Tags:        []string{"admin"},
		Middlewares: huma.Middlewares{adminMiddleware},
	}, func(ctx context.Context, input *DeleteSeriesInput) (*DeleteSeriesOutput, error) {
		return executeDeleteSeries(ctx, input)
	})
}

type SnapshotInput struct {
	SkipHead string `required:"false" query:"skip_head" doc:"If true, do not snapshot data in the head block."`
}

type SnapshotOutput struct {
	Body struct {
		Status string `json:"status"`
		Data   struct {
			Name string `json:"name"`
		} `json:"data"`
	}
}

func (api *API) registerSnapshot() {
	adminMiddleware := func(ctx huma.Context, next func(huma.Context)) {
		if !api.enableAdmin {
			huma.WriteErr(api.humaAPI, ctx, http.StatusServiceUnavailable, "admin APIs disabled")
			return
		}
		next(ctx)
	}

	executeSnapshot := func(ctx context.Context, input *SnapshotInput) (*SnapshotOutput, error) {
		var skipHead bool
		if input.SkipHead != "" {
			var err error
			skipHead, err = strconv.ParseBool(input.SkipHead)
			if err != nil {
				return nil, huma.NewError(http.StatusBadRequest, "unable to parse skip_head boolean", err)
			}
		}

		var (
			snapdir = filepath.Join(api.dbDir, "snapshots")
			name    = fmt.Sprintf("%s-%016x",
				time.Now().UTC().Format("20060102T150405Z0700"),
				rand.Int63())
			dir = filepath.Join(snapdir, name)
		)
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "create snapshot directory", err)
		}
		if err := api.db.Snapshot(dir, !skipHead); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "create snapshot", err)
		}

		out := &SnapshotOutput{}
		out.Body.Status = "success"
		out.Body.Data.Name = name
		return out, nil
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "snapshotPost",
		Method:      http.MethodPost,
		Path:        "/admin/tsdb/snapshot",
		Summary:     "Create a snapshot of the TSDB",
		Description: "Creates a snapshot of all current data into snapshots/<datetime>-<rand> under the TSDB's data directory and returns the directory name. Accepts form parameters via POST request body.",
		Tags:        []string{"admin"},
		Middlewares: huma.Middlewares{adminMiddleware},
	}, func(ctx context.Context, input *SnapshotInput) (*SnapshotOutput, error) {
		return executeSnapshot(ctx, input)
	})

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "snapshotPut",
		Method:      http.MethodPut,
		Path:        "/admin/tsdb/snapshot",
		Summary:     "Create a snapshot of the TSDB",
		Description: "Creates a snapshot of all current data into snapshots/<datetime>-<rand> under the TSDB's data directory and returns the directory name. Accepts query parameters via PUT request.",
		Tags:        []string{"admin"},
		Middlewares: huma.Middlewares{adminMiddleware},
	}, func(ctx context.Context, input *SnapshotInput) (*SnapshotOutput, error) {
		return executeSnapshot(ctx, input)
	})
}

type CleanTombstonesOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (api *API) registerCleanTombstones() {
	adminMiddleware := func(ctx huma.Context, next func(huma.Context)) {
		if !api.enableAdmin {
			huma.WriteErr(api.humaAPI, ctx, http.StatusServiceUnavailable, "admin APIs disabled")
			return
		}
		next(ctx)
	}

	executeCleanTombstones := func(ctx context.Context, input *struct{}) (*CleanTombstonesOutput, error) {
		if err := api.db.CleanTombstones(); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "error cleaning tombstones", err)
		}

		out := &CleanTombstonesOutput{}
		out.Body.Status = "success"
		return out, nil
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "cleanTombstonesPost",
		Method:      http.MethodPost,
		Path:        "/admin/tsdb/clean_tombstones",
		Summary:     "Clean tombstones in the TSDB",
		Description: "Removes deleted data from disk and cleans up the existing tombstones. Triggered via POST request.",
		Tags:        []string{"admin"},
		Middlewares: huma.Middlewares{adminMiddleware},
	}, func(ctx context.Context, input *struct{}) (*CleanTombstonesOutput, error) {
		return executeCleanTombstones(ctx, input)
	})

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "cleanTombstonesPut",
		Method:      http.MethodPut,
		Path:        "/admin/tsdb/clean_tombstones",
		Summary:     "Clean tombstones in the TSDB",
		Description: "Removes deleted data from disk and cleans up the existing tombstones. Triggered via PUT request.",
		Tags:        []string{"admin"},
		Middlewares: huma.Middlewares{adminMiddleware},
	}, func(ctx context.Context, input *struct{}) (*CleanTombstonesOutput, error) {
		return executeCleanTombstones(ctx, input)
	})
}

func (api *API) registerDropSeries() {
	huma.Delete(api.humaAPI, "/series", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, huma.NewError(http.StatusInternalServerError, "not implemented")
	})
}

// registerRemoteRead registers the POST /read endpoint for remote read.
func (api *API) registerRemoteRead() {
	// Use a middleware to intercept and handle the raw HTTP request/response.
	middleware := func(ctx huma.Context, next func(huma.Context)) {
		// Unwrap to get raw HTTP request and response writer.
		r, w := humago.Unwrap(ctx)

		// This is only really for tests - this will never be nil IRL.
		if api.remoteReadHandler != nil {
			api.remoteReadHandler.ServeHTTP(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
		// Don't call next - we've fully handled the request.
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "remoteRead",
		Method:      http.MethodPost,
		Path:        "/read",
		Summary:     "Remote read endpoint",
		Description: "Prometheus remote read endpoint for federated queries and remote storage. Uses Protocol Buffers encoding.",
		Tags:        []string{"remote"},
		Middlewares: huma.Middlewares{middleware},
	}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
		// This won't be called because middleware handles everything.
		return nil, nil
	})
}

// registerRemoteWrite registers the POST /write endpoint for remote write.
func (api *API) registerRemoteWrite() {
	middleware := func(ctx huma.Context, next func(huma.Context)) {
		r, w := humago.Unwrap(ctx)

		if api.remoteWriteHandler != nil {
			api.remoteWriteHandler.ServeHTTP(w, r)
		} else {
			http.Error(w, "remote write receiver needs to be enabled with --web.enable-remote-write-receiver", http.StatusNotFound)
		}
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "remoteWrite",
		Method:      http.MethodPost,
		Path:        "/write",
		Summary:     "Remote write endpoint",
		Description: "Prometheus remote write endpoint for sending metrics to Prometheus. Uses Protocol Buffers encoding.",
		Tags:        []string{"remote"},
		Middlewares: huma.Middlewares{middleware},
	}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})
}

// registerOTLPWrite registers the POST /otlp/v1/metrics endpoint for OTLP write.
func (api *API) registerOTLPWrite() {
	middleware := func(ctx huma.Context, next func(huma.Context)) {
		r, w := humago.Unwrap(ctx)

		if api.otlpWriteHandler != nil {
			api.otlpWriteHandler.ServeHTTP(w, r)
		} else {
			http.Error(w, "otlp write receiver needs to be enabled with --web.enable-otlp-receiver", http.StatusNotFound)
		}
	}

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "otlpWrite",
		Method:      http.MethodPost,
		Path:        "/otlp/v1/metrics",
		Summary:     "OTLP metrics write endpoint",
		Description: "OpenTelemetry Protocol (OTLP) metrics ingestion endpoint. Uses Protocol Buffers encoding.",
		Tags:        []string{"otlp"},
		Middlewares: huma.Middlewares{middleware},
	}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})
}
