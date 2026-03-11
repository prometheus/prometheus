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

package remote

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

type sample = teststorage.Sample

func TestOTLPWriteHandler(t *testing.T) {
	ts := time.Now()
	st := ts.Add(-1 * time.Millisecond)

	// Expected samples passed via OTLP request without details (labels for now) that
	// depend on translation or type and unit labels options.
	expectedSamplesWithoutLabelsFn := func() []sample {
		return []sample{
			{
				M: metadata.Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "test-counter-description"},
				V: 10.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts), ES: []exemplar.Exemplar{
					{
						Labels: labels.FromStrings("span_id", "0001020304050607", "trace_id", "000102030405060708090a0b0c0d0e0f"),
						Value:  10, Ts: timestamp.FromTime(ts), HasTs: true,
					},
				},
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeGauge, Unit: "bytes", Help: "test-gauge-description"},
				V: 10.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 30.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 12.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 2.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 4.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 6.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 8.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 10.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 12.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-histogram-description"},
				V: 12.0, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "bytes", Help: "test-exponential-histogram-description"},
				H: &histogram.Histogram{
					Count:           10,
					Sum:             30.0,
					Schema:          2,
					ZeroThreshold:   1e-128,
					ZeroCount:       2,
					PositiveSpans:   []histogram.Span{{Offset: 1, Length: 5}},
					PositiveBuckets: []int64{2, 0, 0, 0, 0},
				}, ST: timestamp.FromTime(st), T: timestamp.FromTime(ts),
			},
			{
				M: metadata.Metadata{Type: model.MetricTypeGauge, Unit: "", Help: "Target metadata"}, V: 1, T: timestamp.FromTime(ts),
			},
		}
	}

	exportRequest := generateOTLPWriteRequest(ts, st)
	for _, testCase := range []struct {
		name                 string
		otlpCfg              config.OTLPConfig
		typeAndUnitLabels    bool
		expectedLabelsAndMFs []sample
	}{
		{
			name: "NoTranslation/NoTypeAndUnitLabels",
			otlpCfg: config.OTLPConfig{
				TranslationStrategy: otlptranslator.NoTranslation,
			},
			expectedLabelsAndMFs: []sample{
				{MF: "test.counter", L: labels.FromStrings(model.MetricNameLabel, "test.counter", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.gauge", L: labels.FromStrings(model.MetricNameLabel, "test.gauge", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_sum", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_count", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "0")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "1")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "2")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "3")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "4")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "5")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "+Inf")},
				{MF: "test.exponential.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.exponential.histogram", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "target_info", L: labels.FromStrings(model.MetricNameLabel, "target_info", "host.name", "test-host", "instance", "test-instance", "job", "test-service")},
			},
		},
		{
			name: "NoTranslation/WithTypeAndUnitLabels",
			otlpCfg: config.OTLPConfig{
				TranslationStrategy: otlptranslator.NoTranslation,
			},
			typeAndUnitLabels: true,
			expectedLabelsAndMFs: []sample{
				{MF: "test.counter", L: labels.FromStrings(model.MetricNameLabel, "test.counter", "__type__", "counter", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.gauge", L: labels.FromStrings(model.MetricNameLabel, "test.gauge", "__type__", "gauge", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_sum", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_count", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "0")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "1")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "2")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "3")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "4")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "5")},
				{MF: "test.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bucket", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "+Inf")},
				{MF: "test.exponential.histogram", L: labels.FromStrings(model.MetricNameLabel, "test.exponential.histogram", "__type__", "histogram", "__unit__", "bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "target_info", L: labels.FromStrings(model.MetricNameLabel, "target_info", "host.name", "test-host", "instance", "test-instance", "job", "test-service")},
			},
		},
		// For the following cases, skip type and unit cases, it has nothing todo with translation.
		{
			name: "UnderscoreEscapingWithSuffixes",
			otlpCfg: config.OTLPConfig{
				TranslationStrategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			},
			expectedLabelsAndMFs: []sample{
				{MF: "test_counter_bytes_total", L: labels.FromStrings(model.MetricNameLabel, "test_counter_bytes_total", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_gauge_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_gauge_bytes", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_sum", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_count", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "0")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "1")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "2")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "3")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "4")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "5")},
				{MF: "test_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bytes_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "+Inf")},
				{MF: "test_exponential_histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test_exponential_histogram_bytes", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "target_info", L: labels.FromStrings(model.MetricNameLabel, "target_info", "host_name", "test-host", "instance", "test-instance", "job", "test-service")},
			},
		},
		{
			name: "UnderscoreEscapingWithoutSuffixes",
			otlpCfg: config.OTLPConfig{
				TranslationStrategy: otlptranslator.UnderscoreEscapingWithoutSuffixes,
			},
			expectedLabelsAndMFs: []sample{
				{MF: "test_counter", L: labels.FromStrings(model.MetricNameLabel, "test_counter", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_gauge", L: labels.FromStrings(model.MetricNameLabel, "test_gauge", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_sum", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_count", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "0")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "1")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "2")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "3")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "4")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "5")},
				{MF: "test_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_histogram_bucket", "foo_bar", "baz", "instance", "test-instance", "job", "test-service", "le", "+Inf")},
				{MF: "test_exponential_histogram", L: labels.FromStrings(model.MetricNameLabel, "test_exponential_histogram", "foo_bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "target_info", L: labels.FromStrings(model.MetricNameLabel, "target_info", "host_name", "test-host", "instance", "test-instance", "job", "test-service")},
			},
		},
		{
			name: "NoUTF8EscapingWithSuffixes",
			otlpCfg: config.OTLPConfig{
				TranslationStrategy: otlptranslator.NoUTF8EscapingWithSuffixes,
			},
			expectedLabelsAndMFs: []sample{
				// TODO: Counter MF name looks likea bug. Uncovered in unrelated refactor. fix it.
				{MF: "test.counter_bytes_total", L: labels.FromStrings(model.MetricNameLabel, "test.counter_bytes_total", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.gauge_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.gauge_bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_sum", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_count", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "0")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "1")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "2")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "3")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "4")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "5")},
				{MF: "test.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.histogram_bytes_bucket", "foo.bar", "baz", "instance", "test-instance", "job", "test-service", "le", "+Inf")},
				{MF: "test.exponential.histogram_bytes", L: labels.FromStrings(model.MetricNameLabel, "test.exponential.histogram_bytes", "foo.bar", "baz", "instance", "test-instance", "job", "test-service")},
				{MF: "target_info", L: labels.FromStrings(model.MetricNameLabel, "target_info", "host.name", "test-host", "instance", "test-instance", "job", "test-service")},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			otlpOpts := OTLPOptions{
				EnableTypeAndUnitLabels: testCase.typeAndUnitLabels,
			}
			appendable := handleOTLP(t, exportRequest, testCase.otlpCfg, otlpOpts)

			// Compile final expected samples.
			expectedSamples := expectedSamplesWithoutLabelsFn()
			for i, s := range testCase.expectedLabelsAndMFs {
				expectedSamples[i].L = s.L
				expectedSamples[i].MF = s.MF
			}
			teststorage.RequireEqual(t, expectedSamples, appendable.ResultSamples())
		})
	}
}

func handleOTLP(t *testing.T, exportRequest pmetricotlp.ExportRequest, otlpCfg config.OTLPConfig, otlpOpts OTLPOptions) *teststorage.Appendable {
	t.Helper()

	buf, err := exportRequest.MarshalProto()
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	appendable := teststorage.NewAppendable()
	handler := NewOTLPWriteHandler(log, nil, appendable, func() config.Config {
		return config.Config{
			OTLPConfig: otlpCfg,
		}
	}, otlpOpts)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	return appendable
}

func generateOTLPWriteRequest(timestamp, startTime time.Time) pmetricotlp.ExportRequest {
	d := pmetric.NewMetrics()

	// Generate One Counter, One Gauge, One Histogram, One Exponential-Histogram
	// with resource attributes: service.name="test-service", service.instance.id="test-instance", host.name="test-host"
	// with metric attribute: foo.bar="baz"

	resourceMetric := d.ResourceMetrics().AppendEmpty()
	resourceMetric.Resource().Attributes().PutStr("service.name", "test-service")
	resourceMetric.Resource().Attributes().PutStr("service.instance.id", "test-instance")
	resourceMetric.Resource().Attributes().PutStr("host.name", "test-host")

	scopeMetric := resourceMetric.ScopeMetrics().AppendEmpty()

	// Generate One Counter
	counterMetric := scopeMetric.Metrics().AppendEmpty()
	counterMetric.SetName("test.counter")
	counterMetric.SetDescription("test-counter-description")
	counterMetric.SetUnit("By")
	counterMetric.SetEmptySum()
	counterMetric.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	counterMetric.Sum().SetIsMonotonic(true)

	counterDataPoint := counterMetric.Sum().DataPoints().AppendEmpty()
	counterDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	counterDataPoint.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	counterDataPoint.SetDoubleValue(10.0)
	counterDataPoint.Attributes().PutStr("foo.bar", "baz")

	counterExemplar := counterDataPoint.Exemplars().AppendEmpty()

	counterExemplar.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	counterExemplar.SetDoubleValue(10.0)
	counterExemplar.SetSpanID(pcommon.SpanID{0, 1, 2, 3, 4, 5, 6, 7})
	counterExemplar.SetTraceID(pcommon.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})

	// Generate One Gauge
	gaugeMetric := scopeMetric.Metrics().AppendEmpty()
	gaugeMetric.SetName("test.gauge")
	gaugeMetric.SetDescription("test-gauge-description")
	gaugeMetric.SetUnit("By")
	gaugeMetric.SetEmptyGauge()

	gaugeDataPoint := gaugeMetric.Gauge().DataPoints().AppendEmpty()
	gaugeDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	gaugeDataPoint.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	gaugeDataPoint.SetDoubleValue(10.0)
	gaugeDataPoint.Attributes().PutStr("foo.bar", "baz")

	// Generate One Histogram
	histogramMetric := scopeMetric.Metrics().AppendEmpty()
	histogramMetric.SetName("test.histogram")
	histogramMetric.SetDescription("test-histogram-description")
	histogramMetric.SetUnit("By")
	histogramMetric.SetEmptyHistogram()
	histogramMetric.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	histogramDataPoint := histogramMetric.Histogram().DataPoints().AppendEmpty()
	histogramDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	histogramDataPoint.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	histogramDataPoint.ExplicitBounds().FromRaw([]float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0})
	histogramDataPoint.BucketCounts().FromRaw([]uint64{2, 2, 2, 2, 2, 2})
	histogramDataPoint.SetCount(12)
	histogramDataPoint.SetSum(30.0)
	histogramDataPoint.Attributes().PutStr("foo.bar", "baz")

	// Generate One Exponential-Histogram
	exponentialHistogramMetric := scopeMetric.Metrics().AppendEmpty()
	exponentialHistogramMetric.SetName("test.exponential.histogram")
	exponentialHistogramMetric.SetDescription("test-exponential-histogram-description")
	exponentialHistogramMetric.SetUnit("By")
	exponentialHistogramMetric.SetEmptyExponentialHistogram()
	exponentialHistogramMetric.ExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	exponentialHistogramDataPoint := exponentialHistogramMetric.ExponentialHistogram().DataPoints().AppendEmpty()
	exponentialHistogramDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	exponentialHistogramDataPoint.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	exponentialHistogramDataPoint.SetScale(2.0)
	exponentialHistogramDataPoint.Positive().BucketCounts().FromRaw([]uint64{2, 2, 2, 2, 2})
	exponentialHistogramDataPoint.SetZeroCount(2)
	exponentialHistogramDataPoint.SetCount(10)
	exponentialHistogramDataPoint.SetSum(30.0)
	exponentialHistogramDataPoint.Attributes().PutStr("foo.bar", "baz")

	return pmetricotlp.NewExportRequestFromMetrics(d)
}

func TestOTLPDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	appendable := teststorage.NewAppendable()
	cfg := func() config.Config {
		return config.Config{OTLPConfig: config.DefaultOTLPConfig}
	}
	handler := NewOTLPWriteHandler(log, nil, appendable, cfg, OTLPOptions{ConvertDelta: true})

	md := pmetric.NewMetrics()
	ms := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()

	m := ms.AppendEmpty()
	m.SetName("some.delta.total")

	sum := m.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	ts := time.Date(2000, 1, 2, 3, 4, 0, 0, time.UTC)
	for i := range 3 {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(i))
		dp.SetTimestamp(pcommon.NewTimestampFromTime(ts.Add(time.Duration(i) * time.Second)))
	}

	proto, err := pmetricotlp.NewExportRequestFromMetrics(md).MarshalProto()
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(proto))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Result().StatusCode)

	ls := labels.FromStrings("__name__", "some_delta_total")
	milli := func(sec int) int64 {
		return time.Date(2000, 1, 2, 3, 4, sec, 0, time.UTC).UnixMilli()
	}

	want := []sample{
		{MF: "some_delta_total", M: metadata.Metadata{Type: model.MetricTypeGauge}, T: milli(0), L: ls, V: 0}, // +0
		{MF: "some_delta_total", M: metadata.Metadata{Type: model.MetricTypeGauge}, T: milli(1), L: ls, V: 1}, // +1
		{MF: "some_delta_total", M: metadata.Metadata{Type: model.MetricTypeGauge}, T: milli(2), L: ls, V: 3}, // +2
	}
	if diff := cmp.Diff(want, appendable.ResultSamples(), cmp.Exporter(func(reflect.Type) bool { return true })); diff != "" {
		t.Fatal(diff)
	}
}

func BenchmarkOTLP(b *testing.B) {
	start := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)

	type Type struct {
		name string
		data func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric
	}
	types := []Type{{
		name: "sum",
		data: func() func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
			cumul := make(map[int]float64)
			return func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
				m := pmetric.NewMetric()
				sum := m.SetEmptySum()
				sum.SetAggregationTemporality(mode)
				dps := sum.DataPoints()
				for id := range dpc {
					dp := dps.AppendEmpty()
					dp.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
					dp.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(epoch) * time.Minute)))
					dp.Attributes().PutStr("id", strconv.Itoa(id))
					v := float64(rand.IntN(100)) / 10
					switch mode {
					case pmetric.AggregationTemporalityDelta:
						dp.SetDoubleValue(v)
					case pmetric.AggregationTemporalityCumulative:
						cumul[id] += v
						dp.SetDoubleValue(cumul[id])
					}
				}
				return []pmetric.Metric{m}
			}
		}(),
	}, {
		name: "histogram",
		data: func() func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
			bounds := [4]float64{1, 10, 100, 1000}
			type state struct {
				counts [4]uint64
				count  uint64
				sum    float64
			}
			var cumul []state
			return func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
				if cumul == nil {
					cumul = make([]state, dpc)
				}
				m := pmetric.NewMetric()
				hist := m.SetEmptyHistogram()
				hist.SetAggregationTemporality(mode)
				dps := hist.DataPoints()
				for id := range dpc {
					dp := dps.AppendEmpty()
					dp.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
					dp.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(epoch) * time.Minute)))
					dp.Attributes().PutStr("id", strconv.Itoa(id))
					dp.ExplicitBounds().FromRaw(bounds[:])

					var obs *state
					switch mode {
					case pmetric.AggregationTemporalityDelta:
						obs = new(state)
					case pmetric.AggregationTemporalityCumulative:
						obs = &cumul[id]
					}

					for i := range obs.counts {
						v := uint64(rand.IntN(10))
						obs.counts[i] += v
						obs.count++
						obs.sum += float64(v)
					}

					dp.SetCount(obs.count)
					dp.SetSum(obs.sum)
					dp.BucketCounts().FromRaw(obs.counts[:])
				}
				return []pmetric.Metric{m}
			}
		}(),
	}, {
		name: "exponential",
		data: func() func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
			type state struct {
				counts [4]uint64
				count  uint64
				sum    float64
			}
			var cumul []state
			return func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
				if cumul == nil {
					cumul = make([]state, dpc)
				}
				m := pmetric.NewMetric()
				ex := m.SetEmptyExponentialHistogram()
				ex.SetAggregationTemporality(mode)
				dps := ex.DataPoints()
				for id := range dpc {
					dp := dps.AppendEmpty()
					dp.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
					dp.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(epoch) * time.Minute)))
					dp.Attributes().PutStr("id", strconv.Itoa(id))
					dp.SetScale(2)

					var obs *state
					switch mode {
					case pmetric.AggregationTemporalityDelta:
						obs = new(state)
					case pmetric.AggregationTemporalityCumulative:
						obs = &cumul[id]
					}

					for i := range obs.counts {
						v := uint64(rand.IntN(10))
						obs.counts[i] += v
						obs.count++
						obs.sum += float64(v)
					}

					dp.Positive().BucketCounts().FromRaw(obs.counts[:])
					dp.SetCount(obs.count)
					dp.SetSum(obs.sum)
				}

				return []pmetric.Metric{m}
			}
		}(),
	}}

	modes := []struct {
		name string
		data func(func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, int) []pmetric.Metric
	}{{
		name: "cumulative",
		data: func(data func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, epoch int) []pmetric.Metric {
			return data(pmetric.AggregationTemporalityCumulative, 10, epoch)
		},
	}, {
		name: "delta",
		data: func(data func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, epoch int) []pmetric.Metric {
			return data(pmetric.AggregationTemporalityDelta, 10, epoch)
		},
	}, {
		name: "mixed",
		data: func(data func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, epoch int) []pmetric.Metric {
			cumul := data(pmetric.AggregationTemporalityCumulative, 5, epoch)
			delta := data(pmetric.AggregationTemporalityDelta, 5, epoch)
			out := append(cumul, delta...)
			rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
			return out
		},
	}}

	configs := []struct {
		name string
		opts OTLPOptions
	}{
		{name: "default"},
		{name: "convert", opts: OTLPOptions{ConvertDelta: true}},
	}

	Workers := runtime.GOMAXPROCS(0)
	for _, cs := range types {
		for _, mode := range modes {
			for _, cfg := range configs {
				b.Run(fmt.Sprintf("type=%s/temporality=%s/cfg=%s", cs.name, mode.name, cfg.name), func(b *testing.B) {
					if !cfg.opts.ConvertDelta && (mode.name == "delta" || mode.name == "mixed") {
						b.Skip("not possible")
					}

					var total int

					// reqs is a [b.N]*http.Request, divided across the workers.
					// deltatocumulative requires timestamps to be strictly in
					// order on a per-series basis. to ensure this, each reqs[k]
					// contains samples of differently named series, sorted
					// strictly in time order
					reqs := make([][]*http.Request, Workers)
					for n := range b.N {
						k := n % Workers

						md := pmetric.NewMetrics()
						ms := md.ResourceMetrics().AppendEmpty().
							ScopeMetrics().AppendEmpty().
							Metrics()

						for i, m := range mode.data(cs.data, n) {
							m.SetName(fmt.Sprintf("benchmark_%d_%d", k, i))
							m.MoveTo(ms.AppendEmpty())
						}

						total += sampleCount(md)

						ex := pmetricotlp.NewExportRequestFromMetrics(md)
						data, err := ex.MarshalProto()
						require.NoError(b, err)

						req, err := http.NewRequest("", "", bytes.NewReader(data))
						require.NoError(b, err)
						req.Header.Set("Content-Type", "application/x-protobuf")

						reqs[k] = append(reqs[k], req)
					}

					log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

					appendable := teststorage.NewAppendable()
					cfgfn := func() config.Config {
						return config.Config{OTLPConfig: config.DefaultOTLPConfig}
					}
					handler := NewOTLPWriteHandler(log, nil, appendable, cfgfn, cfg.opts)

					fail := make(chan struct{})
					done := make(chan struct{})

					b.ResetTimer()
					b.ReportAllocs()

					// we use multiple workers to mimic a real-world scenario
					// where multiple OTel collectors are sending their
					// time-series in parallel.
					// this is necessary to exercise potential lock-contention
					// in this benchmark
					for k := range Workers {
						go func() {
							rec := httptest.NewRecorder()
							for _, req := range reqs[k] {
								handler.ServeHTTP(rec, req)
								if rec.Result().StatusCode != http.StatusOK {
									fail <- struct{}{}
									return
								}
							}
							done <- struct{}{}
						}()
					}

					for range Workers {
						select {
						case <-fail:
							b.FailNow()
						case <-done:
						}
					}

					require.Len(b, appendable.ResultSamples(), total)
				})
			}
		}
	}
}

func sampleCount(md pmetric.Metrics) int {
	var total int
	rms := md.ResourceMetrics()
	for i := range rms.Len() {
		sms := rms.At(i).ScopeMetrics()
		for i := range sms.Len() {
			ms := sms.At(i).Metrics()
			for i := range ms.Len() {
				m := ms.At(i)
				switch m.Type() {
				case pmetric.MetricTypeSum:
					total += m.Sum().DataPoints().Len()
				case pmetric.MetricTypeGauge:
					total += m.Gauge().DataPoints().Len()
				case pmetric.MetricTypeHistogram:
					dps := m.Histogram().DataPoints()
					for i := range dps.Len() {
						total += dps.At(i).BucketCounts().Len()
						total++ // le=+Inf series
						total++ // _sum series
						total++ // _count series
					}
				case pmetric.MetricTypeExponentialHistogram:
					total += m.ExponentialHistogram().DataPoints().Len()
				case pmetric.MetricTypeSummary:
					total += m.Summary().DataPoints().Len()
				}
			}
		}
	}
	return total
}

func TestOTLPInstrumentedAppendable(t *testing.T) {
	t.Run("no problems", func(t *testing.T) {
		appTest := teststorage.NewAppendable()
		oa := newOTLPInstrumentedAppendable(prometheus.NewRegistry(), appTest)

		require.Equal(t, 0.0, testutil.ToFloat64(oa.outOfOrderExemplars))
		require.Equal(t, 0.0, testutil.ToFloat64(oa.samplesAppendedWithoutMetadata))

		app := oa.AppenderV2(t.Context())
		_, err := app.Append(0, labels.EmptyLabels(), -1, 1, 2, nil, nil, storage.AOptions{Metadata: metadata.Metadata{Help: "yo"}})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.Len(t, appTest.ResultSamples(), 1)

		require.Equal(t, 0.0, testutil.ToFloat64(oa.outOfOrderExemplars))
		require.Equal(t, 0.0, testutil.ToFloat64(oa.samplesAppendedWithoutMetadata))
	})
	t.Run("without metadata", func(t *testing.T) {
		appTest := teststorage.NewAppendable()
		oa := newOTLPInstrumentedAppendable(prometheus.NewRegistry(), appTest)

		require.Equal(t, 0.0, testutil.ToFloat64(oa.outOfOrderExemplars))
		require.Equal(t, 0.0, testutil.ToFloat64(oa.samplesAppendedWithoutMetadata))

		app := oa.AppenderV2(t.Context())
		_, err := app.Append(0, labels.EmptyLabels(), -1, 1, 2, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.Len(t, appTest.ResultSamples(), 1)

		require.Equal(t, 0.0, testutil.ToFloat64(oa.outOfOrderExemplars))
		require.Equal(t, 1.0, testutil.ToFloat64(oa.samplesAppendedWithoutMetadata))
	})
	t.Run("without metadata; 2 exemplar OOO errors", func(t *testing.T) {
		appTest := teststorage.NewAppendable().WithErrs(nil, errors.New("exemplar error"), nil)
		oa := newOTLPInstrumentedAppendable(prometheus.NewRegistry(), appTest)

		require.Equal(t, 0.0, testutil.ToFloat64(oa.outOfOrderExemplars))
		require.Equal(t, 0.0, testutil.ToFloat64(oa.samplesAppendedWithoutMetadata))

		app := oa.AppenderV2(t.Context())
		_, err := app.Append(0, labels.EmptyLabels(), -1, 1, 2, nil, nil, storage.AOptions{Exemplars: []exemplar.Exemplar{{}, {}}})
		// Partial errors should be handled in the middleware, OTLP converter does not handle it.
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.Len(t, appTest.ResultSamples(), 1)

		require.Equal(t, 2.0, testutil.ToFloat64(oa.outOfOrderExemplars))
		require.Equal(t, 1.0, testutil.ToFloat64(oa.samplesAppendedWithoutMetadata))
	})
}
