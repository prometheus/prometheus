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

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
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
			h := HistogramProtoToHistogram(hp)
			require.Equal(t, mockHistogram{labels, hp.Timestamp, h}, appendable.histograms[k])
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
		Histograms: []prompb.Histogram{HistogramToHistogramProto(0, &testHistogram)},
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
	l labels.Labels
	t int64
	h *histogram.Histogram
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

func (m *mockAppendable) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram) (storage.SeriesRef, error) {
	if t < m.latestHistogram {
		return 0, storage.ErrOutOfOrderSample
	}

	m.latestHistogram = t
	m.histograms = append(m.histograms, mockHistogram{l, t, h})
	return 0, nil
}
