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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"
)

type HumaOptions struct {
	Enabled     bool
	ExternalURL *url.URL
}

// Time wraps time.Time to support Prometheus time parsing (Unix timestamp or RFC3339)
type Time struct {
	time.Time
}

// UnmarshalText implements the encoding.TextUnmarshaler interface for custom time parsing
func (t *Time) UnmarshalText(text []byte) error {
	s := string(text)

	// Try parsing as Unix timestamp (float seconds)
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		sec, nsec := math.Modf(v)
		nsec = math.Round(nsec*1000) / 1000
		t.Time = time.Unix(int64(sec), int64(nsec*float64(time.Second))).UTC()
		return nil
	}

	// Try parsing as RFC3339
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

// UnmarshalText implements the encoding.TextUnmarshaler interface for custom duration parsing
func (d *Duration) UnmarshalText(text []byte) error {
	s := string(text)

	// Try parsing as float (seconds)
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		ts := v * float64(time.Second)
		if ts > float64(math.MaxInt64) || ts < float64(math.MinInt64) {
			return fmt.Errorf("cannot parse %q to a valid duration. It overflows int64", s)
		}
		d.Duration = time.Duration(ts)
		return nil
	}

	// Try parsing as Prometheus duration (e.g., "1h", "5m")
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
				encoder := json.NewEncoder(w)
				return encoder.Encode(v)
			},
			Unmarshal: func(data []byte, v any) error {
				return json.Unmarshal(data, v)
			},
		},
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
	api.registerQueryRange()

	return func(http.HandlerFunc) http.HandlerFunc { return api.humaAPI.Adapter().ServeHTTP }
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

// Schema implements huma.SchemaProvider for QueryData to properly describe the result field
func (q QueryData) Schema(r huma.Registry) *huma.Schema {
	minItems2 := 2
	maxItems2 := 2

	return &huma.Schema{
		Type: huma.TypeObject,
		Properties: map[string]*huma.Schema{
			"resultType": {
				Type:        huma.TypeString,
				Enum:        []any{"matrix", "vector", "scalar", "string"},
				Description: "The type of the result",
			},
			"result": {
				Description: "Query result. The structure depends on the resultType field.",
				OneOf: []*huma.Schema{
					{
						Type:        huma.TypeArray,
						Description: "Matrix result: array of time series, each with a metric and array of [timestamp, value] points",
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
									Description: "Array of histogram data points (optional)",
								},
							},
							Required:             []string{"metric", "values"},
							AdditionalProperties: false,
						},
					},
					{
						Type:        huma.TypeArray,
						Description: "Vector result: array of instant samples, each with a metric and a single [timestamp, value]",
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
									Description: "Histogram data (optional)",
								},
							},
							Required:             []string{"metric", "value"},
							AdditionalProperties: false,
						},
					},
					{
						Type:        huma.TypeArray,
						Description: "Scalar result: [timestamp, value]",
						MinItems:    &minItems2,
						MaxItems:    &maxItems2,
						Items: &huma.Schema{
							OneOf: []*huma.Schema{
								{Type: huma.TypeNumber},
								{Type: huma.TypeString},
							},
						},
					},
					{
						Type:        huma.TypeArray,
						Description: "String result: [timestamp, string_value]",
						MinItems:    &minItems2,
						MaxItems:    &maxItems2,
						Items: &huma.Schema{
							OneOf: []*huma.Schema{
								{Type: huma.TypeNumber},
								{Type: huma.TypeString},
							},
						},
					},
				},
			},
			"stats": {
				Type:        huma.TypeObject,
				Description: "Query statistics (optional)",
			},
		},
		Required: []string{"resultType", "result"},
	}
}

type QueryRangeInput struct {
	Limit int      `required:"false" default:"-1" query:"limit" doc:"The maximum number of metrics to return." example:"100"`
	Start Time     `required:"true" query:"start" doc:"The start time of the query." example:"2021-01-01T00:00:00Z"`
	End   Time     `required:"true" query:"end" doc:"The end time of the query." example:"2021-01-01T00:00:00Z"`
	Step  Duration `required:"true" query:"step" doc:"The step size of the query." example:"1h"`
	Query string   `required:"true" query:"query" doc:"The query to execute." example:"up"`
}

type QueryRangeOutput struct {
	Body struct {
		Status string     `json:"status"`
		Data   *QueryData `json:"data"`
	}
}

func (api *API) registerQueryRange() {
	handler := func(ctx context.Context, input *QueryRangeInput) (*QueryRangeOutput, error) {
		start := input.Start.Time
		end := input.End.Time
		step := input.Step.Duration
		limit := input.Limit

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
		qry, err := api.QueryEngine.NewRangeQuery(ctx, api.Queryable, opts, input.Query, start, end, step)
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

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-range",
		Method:      http.MethodGet,
		Path:        "/query_range",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, handler)

	huma.Register(api.humaAPI, huma.Operation{
		OperationID: "query-range-post",
		Method:      http.MethodPost,
		Path:        "/query_range",
		Middlewares: huma.Middlewares{api.agentMiddleware()},
	}, handler)
}
