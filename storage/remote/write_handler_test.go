// Copyright 2021 The Prometheus Authors
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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

func TestRemoteWriteHandler(t *testing.T) {
	buf, _, err := buildWriteRequest(writeRequestFixture.Timeseries, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{}
	handler := NewWriteHandler(nil, appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	i := 0
	j := 0
	k := 0
	for _, ts := range writeRequestFixture.Timeseries {
		labels := labelProtosToLabels(ts.Labels)
		for _, s := range ts.Samples {
			require.Equal(t, mockSample{labels, s.Timestamp, s.Value}, appendable.samples[i])
			i++
		}

		for _, e := range ts.Exemplars {
			exemplarLabels := labelProtosToLabels(e.Labels)
			require.Equal(t, mockExemplar{labels, exemplarLabels, e.Timestamp, e.Value}, appendable.exemplars[j])
			j++
		}

		for _, hp := range ts.Histograms {
			if hp.GetCountFloat() > 0 || hp.GetZeroCountFloat() > 0 { // It is a float histogram.
				fh := HistogramProtoToFloatHistogram(hp)
				require.Equal(t, mockHistogram{labels, hp.Timestamp, nil, fh}, appendable.histograms[k])
			} else {
				h := HistogramProtoToHistogram(hp)
				require.Equal(t, mockHistogram{labels, hp.Timestamp, h, nil}, appendable.histograms[k])
			}

			k++
		}
	}
}

func TestOutOfOrderSample(t *testing.T) {
	buf, _, err := buildWriteRequest([]prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "test_metric"}},
		Samples: []prompb.Sample{{Value: 1, Timestamp: 0}},
	}}, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{
		latestSample: 100,
	}
	handler := NewWriteHandler(log.NewNopLogger(), appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// This test case currently aims to verify that the WriteHandler endpoint
// don't fail on ingestion errors since the exemplar storage is
// still experimental.
func TestOutOfOrderExemplar(t *testing.T) {
	buf, _, err := buildWriteRequest([]prompb.TimeSeries{{
		Labels:    []prompb.Label{{Name: "__name__", Value: "test_metric"}},
		Exemplars: []prompb.Exemplar{{Labels: []prompb.Label{{Name: "foo", Value: "bar"}}, Value: 1, Timestamp: 0}},
	}}, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{
		latestExemplar: 100,
	}
	handler := NewWriteHandler(log.NewNopLogger(), appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// TODO: update to require.Equal(t, http.StatusConflict, resp.StatusCode) once exemplar storage is not experimental.
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestOutOfOrderHistogram(t *testing.T) {
	buf, _, err := buildWriteRequest([]prompb.TimeSeries{{
		Labels:     []prompb.Label{{Name: "__name__", Value: "test_metric"}},
		Histograms: []prompb.Histogram{HistogramToHistogramProto(0, &testHistogram), FloatHistogramToHistogramProto(1, testHistogram.ToFloat())},
	}}, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{
		latestHistogram: 100,
	}
	handler := NewWriteHandler(log.NewNopLogger(), appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCommitErr(t *testing.T) {
	buf, _, err := buildWriteRequest(writeRequestFixture.Timeseries, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{
		commitErr: fmt.Errorf("commit error"),
	}
	handler := NewWriteHandler(log.NewNopLogger(), appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "commit error\n", string(body))
}

func TestOTLPWriteHandler(t *testing.T) {
	exportRequest := generateOTLPWriteRequest(t)

	buf, err := exportRequest.MarshalProto()
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")

	appendable := &mockAppendable{}
	handler := NewOTLPWriteHandler(nil, appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	require.Equal(t, 12, len(appendable.samples))   // 1 (counter) + 1 (gauge) + 1 (target_info) + 7 (hist_bucket) + 2 (hist_sum, hist_count)
	require.Equal(t, 1, len(appendable.histograms)) // 1 (exponential histogram)
	require.Equal(t, 1, len(appendable.exemplars))  // 1 (exemplar)
}

func BenchmarkRemoteWriteOOOSamples(b *testing.B) {
	dir := b.TempDir()

	opts := tsdb.DefaultOptions()
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 120 * time.Minute.Milliseconds()

	db, err := tsdb.Open(dir, nil, nil, opts, nil)
	require.NoError(b, err)

	b.Cleanup(func() {
		require.NoError(b, db.Close())
	})

	handler := NewWriteHandler(log.NewNopLogger(), db.Head())

	buf, _, err := buildWriteRequest(genSeriesWithSample(1000, 200*time.Minute.Milliseconds()), nil, nil, nil)
	require.NoError(b, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(b, err)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	require.Equal(b, http.StatusNoContent, recorder.Code)
	require.Equal(b, db.Head().NumSeries(), uint64(1000))

	var bufRequests [][]byte
	for i := 0; i < 100; i++ {
		buf, _, err = buildWriteRequest(genSeriesWithSample(1000, int64(80+i)*time.Minute.Milliseconds()), nil, nil, nil)
		require.NoError(b, err)
		bufRequests = append(bufRequests, buf)
	}

	b.ResetTimer()
	for i := 0; i < 100; i++ {
		req, err = http.NewRequest("", "", bytes.NewReader(bufRequests[i]))
		require.NoError(b, err)

		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		require.Equal(b, http.StatusNoContent, recorder.Code)
		require.Equal(b, db.Head().NumSeries(), uint64(1000))
	}
}

func genSeriesWithSample(numSeries int, ts int64) []prompb.TimeSeries {
	var series []prompb.TimeSeries
	for i := 0; i < numSeries; i++ {
		s := prompb.TimeSeries{
			Labels:  []prompb.Label{{Name: "__name__", Value: fmt.Sprintf("test_metric_%d", i)}},
			Samples: []prompb.Sample{{Value: float64(i), Timestamp: ts}},
		}
		series = append(series, s)
	}

	return series
}

type mockAppendable struct {
	latestSample    int64
	samples         []mockSample
	latestExemplar  int64
	exemplars       []mockExemplar
	latestHistogram int64
	histograms      []mockHistogram
	commitErr       error
}

type mockSample struct {
	l labels.Labels
	t int64
	v float64
}

type mockExemplar struct {
	l  labels.Labels
	el labels.Labels
	t  int64
	v  float64
}

type mockHistogram struct {
	l  labels.Labels
	t  int64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func (m *mockAppendable) Appender(_ context.Context) storage.Appender {
	return m
}

func (m *mockAppendable) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t < m.latestSample {
		return 0, storage.ErrOutOfOrderSample
	}

	m.latestSample = t
	m.samples = append(m.samples, mockSample{l, t, v})
	return 0, nil
}

func (m *mockAppendable) Commit() error {
	return m.commitErr
}

func (*mockAppendable) Rollback() error {
	return fmt.Errorf("not implemented")
}

func (m *mockAppendable) AppendExemplar(_ storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	if e.Ts < m.latestExemplar {
		return 0, storage.ErrOutOfOrderExemplar
	}

	m.latestExemplar = e.Ts
	m.exemplars = append(m.exemplars, mockExemplar{l, e.Labels, e.Ts, e.Value})
	return 0, nil
}

func (m *mockAppendable) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if t < m.latestHistogram {
		return 0, storage.ErrOutOfOrderSample
	}

	m.latestHistogram = t
	m.histograms = append(m.histograms, mockHistogram{l, t, h, fh})
	return 0, nil
}

func (m *mockAppendable) UpdateMetadata(_ storage.SeriesRef, _ labels.Labels, _ metadata.Metadata) (storage.SeriesRef, error) {
	// TODO: Wire metadata in a mockAppendable field when we get around to handling metadata in remote_write.
	// UpdateMetadata is no-op for remote write (where mockAppendable is being used to test) for now.
	return 0, nil
}

func generateOTLPWriteRequest(t *testing.T) pmetricotlp.ExportRequest {
	d := pmetric.NewMetrics()

	// Generate One Counter, One Gauge, One Histogram, One Exponential-Histogram
	// with resource attributes: service.name="test-service", service.instance.id="test-instance", host.name="test-host"
	// with metric attibute: foo.bar="baz"

	timestamp := time.Now()

	resourceMetric := d.ResourceMetrics().AppendEmpty()
	resourceMetric.Resource().Attributes().PutStr("service.name", "test-service")
	resourceMetric.Resource().Attributes().PutStr("service.instance.id", "test-instance")
	resourceMetric.Resource().Attributes().PutStr("host.name", "test-host")

	scopeMetric := resourceMetric.ScopeMetrics().AppendEmpty()

	// Generate One Counter
	counterMetric := scopeMetric.Metrics().AppendEmpty()
	counterMetric.SetName("test-counter")
	counterMetric.SetDescription("test-counter-description")
	counterMetric.SetEmptySum()
	counterMetric.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	counterMetric.Sum().SetIsMonotonic(true)

	counterDataPoint := counterMetric.Sum().DataPoints().AppendEmpty()
	counterDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	counterDataPoint.SetDoubleValue(10.0)
	counterDataPoint.Attributes().PutStr("foo.bar", "baz")

	counterExemplar := counterDataPoint.Exemplars().AppendEmpty()
	counterExemplar.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	counterExemplar.SetDoubleValue(10.0)
	counterExemplar.SetSpanID(pcommon.SpanID{0, 1, 2, 3, 4, 5, 6, 7})
	counterExemplar.SetTraceID(pcommon.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})

	// Generate One Gauge
	gaugeMetric := scopeMetric.Metrics().AppendEmpty()
	gaugeMetric.SetName("test-gauge")
	gaugeMetric.SetDescription("test-gauge-description")
	gaugeMetric.SetEmptyGauge()

	gaugeDataPoint := gaugeMetric.Gauge().DataPoints().AppendEmpty()
	gaugeDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	gaugeDataPoint.SetDoubleValue(10.0)
	gaugeDataPoint.Attributes().PutStr("foo.bar", "baz")

	// Generate One Histogram
	histogramMetric := scopeMetric.Metrics().AppendEmpty()
	histogramMetric.SetName("test-histogram")
	histogramMetric.SetDescription("test-histogram-description")
	histogramMetric.SetEmptyHistogram()
	histogramMetric.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	histogramDataPoint := histogramMetric.Histogram().DataPoints().AppendEmpty()
	histogramDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	histogramDataPoint.ExplicitBounds().FromRaw([]float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0})
	histogramDataPoint.BucketCounts().FromRaw([]uint64{2, 2, 2, 2, 2, 2})
	histogramDataPoint.SetCount(10)
	histogramDataPoint.SetSum(30.0)
	histogramDataPoint.Attributes().PutStr("foo.bar", "baz")

	// Generate One Exponential-Histogram
	exponentialHistogramMetric := scopeMetric.Metrics().AppendEmpty()
	exponentialHistogramMetric.SetName("test-exponential-histogram")
	exponentialHistogramMetric.SetDescription("test-exponential-histogram-description")
	exponentialHistogramMetric.SetEmptyExponentialHistogram()
	exponentialHistogramMetric.ExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	exponentialHistogramDataPoint := exponentialHistogramMetric.ExponentialHistogram().DataPoints().AppendEmpty()
	exponentialHistogramDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	exponentialHistogramDataPoint.SetScale(2.0)
	exponentialHistogramDataPoint.Positive().BucketCounts().FromRaw([]uint64{2, 2, 2, 2, 2})
	exponentialHistogramDataPoint.SetZeroCount(2)
	exponentialHistogramDataPoint.SetCount(10)
	exponentialHistogramDataPoint.SetSum(30.0)
	exponentialHistogramDataPoint.Attributes().PutStr("foo.bar", "baz")

	return pmetricotlp.NewExportRequestFromMetrics(d)
}
