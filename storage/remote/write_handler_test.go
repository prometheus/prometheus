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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

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
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, Version1)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	i := 0
	j := 0
	k := 0
	for _, ts := range writeRequestFixture.Timeseries {
		ls := labelProtosToLabels(ts.Labels)
		for _, s := range ts.Samples {
			require.Equal(t, mockSample{ls, s.Timestamp, s.Value}, appendable.samples[i])
			i++
		}

		for _, e := range ts.Exemplars {
			exemplarLabels := labelProtosToLabels(e.Labels)
			require.Equal(t, mockExemplar{ls, exemplarLabels, e.Timestamp, e.Value}, appendable.exemplars[j])
			j++
		}

		for _, hp := range ts.Histograms {
			if hp.IsFloatHistogram() {
				fh := FloatHistogramProtoToFloatHistogram(hp)
				require.Equal(t, mockHistogram{ls, hp.Timestamp, nil, fh}, appendable.histograms[k])
			} else {
				h := HistogramProtoToHistogram(hp)
				require.Equal(t, mockHistogram{ls, hp.Timestamp, h, nil}, appendable.histograms[k])
			}

			k++
		}
	}
}

func TestRemoteWriteHandlerMinimizedFormat(t *testing.T) {
	buf, _, err := buildMinimizedWriteRequestStr(writeRequestMinimizedFixture.Timeseries, writeRequestMinimizedFixture.Symbols, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion11HeaderValue)
	require.NoError(t, err)

	appendable := &mockAppendable{}
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(nil, nil, appendable, Version2)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	i := 0
	j := 0
	k := 0
	// the reduced write request is equivalent to the write request fixture.
	// we can use it for
	for _, ts := range writeRequestMinimizedFixture.Timeseries {
		ls := labelProtosV2ToLabels(ts.LabelsRefs, writeRequestMinimizedFixture.Symbols)
		for _, s := range ts.Samples {
			require.Equal(t, mockSample{ls, s.Timestamp, s.Value}, appendable.samples[i])
			i++
		}

		for _, e := range ts.Exemplars {
			exemplarLabels := labelProtosV2ToLabels(e.LabelsRefs, writeRequestMinimizedFixture.Symbols)
			require.Equal(t, mockExemplar{ls, exemplarLabels, e.Timestamp, e.Value}, appendable.exemplars[j])
			j++
		}

		for _, hp := range ts.Histograms {
			if hp.IsFloatHistogram() {
				fh := FloatHistogramProtoV2ToFloatHistogram(hp)
				require.Equal(t, mockHistogram{ls, hp.Timestamp, nil, fh}, appendable.histograms[k])
			} else {
				h := HistogramProtoV2ToHistogram(hp)
				require.Equal(t, mockHistogram{ls, hp.Timestamp, h, nil}, appendable.histograms[k])
			}

			k++
		}

		// todo: check for metadata
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
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, Version1)

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
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, Version1)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// TODO: update to require.Equal(t, http.StatusConflict, resp.StatusCode) once exemplar storage is not experimental.
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestOutOfOrderHistogram(t *testing.T) {
	buf, _, err := buildWriteRequest([]prompb.TimeSeries{{
		Labels:     []prompb.Label{{Name: "__name__", Value: "test_metric"}},
		Histograms: []prompb.Histogram{HistogramToHistogramProto(0, &testHistogram), FloatHistogramToHistogramProto(1, testHistogram.ToFloat(nil))},
	}}, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{
		latestHistogram: 100,
	}
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, Version1)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func BenchmarkRemoteWritehandler(b *testing.B) {
	const labelValue = "abcdefg'hijlmn234!@#$%^&*()_+~`\"{}[],./<>?hello0123hiOlá你好Dzieńdobry9Zd8ra765v4stvuyte"
	var reqs []*http.Request
	for i := 0; i < b.N; i++ {
		num := strings.Repeat(strconv.Itoa(i), 16)
		buf, _, err := buildWriteRequest([]prompb.TimeSeries{{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric"},
				{Name: "test_label_name_" + num, Value: labelValue + num},
			},
			Histograms: []prompb.Histogram{HistogramToHistogramProto(0, &testHistogram)},
		}}, nil, nil, nil)
		require.NoError(b, err)
		req, err := http.NewRequest("", "", bytes.NewReader(buf))
		require.NoError(b, err)
		reqs = append(reqs, req)
	}

	appendable := &mockAppendable{}
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, Version1)
	recorder := httptest.NewRecorder()

	b.ResetTimer()
	for _, req := range reqs {
		handler.ServeHTTP(recorder, req)
	}
}

func TestCommitErr(t *testing.T) {
	buf, _, err := buildWriteRequest(writeRequestFixture.Timeseries, nil, nil, nil)
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)

	appendable := &mockAppendable{
		commitErr: fmt.Errorf("commit error"),
	}
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, Version1)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "commit error\n", string(body))
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
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, db.Head(), Version1)

	buf, _, err := buildWriteRequest(genSeriesWithSample(1000, 200*time.Minute.Milliseconds()), nil, nil, nil)
	require.NoError(b, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(b, err)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	require.Equal(b, http.StatusNoContent, recorder.Code)
	require.Equal(b, uint64(1000), db.Head().NumSeries())

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
		require.Equal(b, uint64(1000), db.Head().NumSeries())
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

func (m *mockAppendable) AppendHistogram(_ storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if t < m.latestHistogram {
		return 0, storage.ErrOutOfOrderSample
	}

	m.latestHistogram = t
	m.histograms = append(m.histograms, mockHistogram{l, t, h, fh})
	return 0, nil
}

func (m *mockAppendable) UpdateMetadata(_ storage.SeriesRef, l labels.Labels, mp metadata.Metadata) (storage.SeriesRef, error) {
	// TODO: Wire metadata in a mockAppendable field when we get around to handling metadata in remote_write.
	// UpdateMetadata is no-op for remote write (where mockAppendable is being used to test) for now.
	return 0, nil
}

func (m *mockAppendable) AppendCTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, _ int64) (storage.SeriesRef, error) {
	// AppendCTZeroSample is no-op for remote-write for now.
	return 0, nil
}
