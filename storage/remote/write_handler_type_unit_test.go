// Copyright 2025 The Prometheus Authors
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

package remote

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// buildExpectedLabels constructs labels the same way as the write handler implementation
// to ensure consistency across different labels implementations (stringlabels, dedupelabels).
func buildExpectedLabels(metricName, labelName, labelValue string, metricType writev2.Metadata_MetricType, unit string) labels.Labels {
	// Start with base labels from the incoming request.
	baseLabels := labels.FromStrings("__name__", metricName, labelName, labelValue)

	// If no metadata, return base labels as-is.
	if metricType == writev2.Metadata_METRIC_TYPE_UNSPECIFIED && unit == "" {
		return baseLabels
	}

	// Add metadata labels using Builder (same as implementation).
	lb := labels.NewBuilder(baseLabels)
	if metricType != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
		// Convert to model.MetricType string.
		var typeStr string
		switch metricType {
		case writev2.Metadata_METRIC_TYPE_COUNTER:
			typeStr = "counter"
		case writev2.Metadata_METRIC_TYPE_GAUGE:
			typeStr = "gauge"
		case writev2.Metadata_METRIC_TYPE_HISTOGRAM:
			typeStr = "histogram"
		case writev2.Metadata_METRIC_TYPE_GAUGEHISTOGRAM:
			typeStr = "gaugehistogram"
		case writev2.Metadata_METRIC_TYPE_SUMMARY:
			typeStr = "summary"
		case writev2.Metadata_METRIC_TYPE_INFO:
			typeStr = "info"
		case writev2.Metadata_METRIC_TYPE_STATESET:
			typeStr = "stateset"
		}
		if typeStr != "" {
			lb.Set("__type__", typeStr)
		}
	}
	if unit != "" {
		lb.Set("__unit__", unit)
	}
	return lb.Labels()
}

func TestRemoteWriteHandler_TypeAndUnitLabels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                    string
		enableTypeAndUnitLabels bool
		metricType              writev2.Metadata_MetricType
		unit                    string
		expectedLabels          labels.Labels
	}{
		{
			name:                    "type and unit labels enabled with counter and bytes unit",
			enableTypeAndUnitLabels: true,
			metricType:              writev2.Metadata_METRIC_TYPE_COUNTER,
			unit:                    "bytes",
			expectedLabels:          buildExpectedLabels("test_metric", "foo", "bar", writev2.Metadata_METRIC_TYPE_COUNTER, "bytes"),
		},
		{
			name:                    "type and unit labels enabled with gauge and seconds unit",
			enableTypeAndUnitLabels: true,
			metricType:              writev2.Metadata_METRIC_TYPE_GAUGE,
			unit:                    "seconds",
			expectedLabels:          buildExpectedLabels("test_metric", "foo", "bar", writev2.Metadata_METRIC_TYPE_GAUGE, "seconds"),
		},
		{
			name:                    "type and unit labels disabled - no metadata labels",
			enableTypeAndUnitLabels: false,
			metricType:              writev2.Metadata_METRIC_TYPE_COUNTER,
			unit:                    "bytes",
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"foo", "bar",
			),
		},
		{
			name:                    "type and unit labels enabled but no metadata",
			enableTypeAndUnitLabels: true,
			metricType:              writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			unit:                    "",
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"foo", "bar",
			),
		},
		{
			name:                    "type and unit labels enabled with only unit (no type)",
			enableTypeAndUnitLabels: true,
			metricType:              writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			unit:                    "milliseconds",
			expectedLabels:          buildExpectedLabels("test_metric", "foo", "bar", writev2.Metadata_METRIC_TYPE_UNSPECIFIED, "milliseconds"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			appendable := &mockAppendable{}
			handler := NewWriteHandler(
				slog.Default(),
				nil,
				appendable,
				[]config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2},
				false, // ingestCTZeroSample
				tc.enableTypeAndUnitLabels,
			)

			// Build a v2 write request with metadata
			symbolTable := writev2.NewSymbolTable()
			labelRefs := symbolTable.SymbolizeLabels(
				labels.FromStrings("__name__", "test_metric", "foo", "bar"),
				nil,
			)

			var unitRef uint32
			if tc.unit != "" {
				unitRef = symbolTable.Symbolize(tc.unit)
			}

			req := &writev2.Request{
				Symbols: symbolTable.Symbols(),
				Timeseries: []writev2.TimeSeries{
					{
						LabelsRefs: labelRefs,
						Metadata: writev2.Metadata{
							Type:    tc.metricType,
							UnitRef: unitRef,
						},
						Samples: []writev2.Sample{
							{Value: 1.0, Timestamp: 1000},
						},
					},
				},
			}

			// Marshal the request
			data, _, _, err := buildV2WriteRequest(
				slog.Default(),
				req.Timeseries,
				req.Symbols,
				nil,
				nil,
				nil,
				"snappy",
			)
			require.NoError(t, err)

			// Create HTTP request
			httpReq, err := http.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(data))
			require.NoError(t, err)
			httpReq.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
			httpReq.Header.Set("Content-Encoding", "snappy")
			httpReq.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httpReq)

			resp := recorder.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusNoContent, resp.StatusCode, "response body: %s", string(body))

			// Verify the labels
			require.Len(t, appendable.samples, 1)
			require.Equal(t, tc.expectedLabels, appendable.samples[0].l)
		})
	}
}

func TestRemoteWriteHandler_V1DoesNotAddTypeAndUnitLabels(t *testing.T) {
	t.Parallel()

	// Even with type and unit labels enabled, v1 should not add them
	// since v1 doesn't have metadata in the TimeSeries
	appendable := &mockAppendable{}
	handler := NewWriteHandler(
		slog.Default(),
		nil,
		appendable,
		[]config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1},
		false, // ingestCTZeroSample
		true,  // enableTypeAndUnitLabels - should have no effect on v1
	)

	req := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric"},
					{Name: "foo", Value: "bar"},
				},
				Samples: []prompb.Sample{
					{Value: 1.0, Timestamp: 1000},
				},
			},
		},
	}

	data, _, _, err := buildWriteRequest(nil, req.Timeseries, nil, nil, nil, nil, "snappy")
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(data))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "snappy")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify no type/unit labels were added
	require.Len(t, appendable.samples, 1)
	expectedLabels := labels.FromStrings("__name__", "test_metric", "foo", "bar")
	require.Equal(t, expectedLabels, appendable.samples[0].l)
}
