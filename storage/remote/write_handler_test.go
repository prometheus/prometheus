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
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRemoteWriteHandlerHeadersHandling_V1Message(t *testing.T) {
	payload, _, _, err := buildWriteRequest(nil, writeRequestFixture.Timeseries, nil, nil, nil, nil, "snappy")
	require.NoError(t, err)

	for _, tc := range []struct {
		name         string
		reqHeaders   map[string]string
		expectedCode int
	}{
		// Generally Prometheus 1.0 Receiver never checked for existence of the headers, so
		// we keep things permissive.
		{
			name: "correct PRW 1.0 headers",
			reqHeaders: map[string]string{
				"Content-Type":           remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV1],
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "missing remote write version",
			reqHeaders: map[string]string{
				"Content-Type":     remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV1],
				"Content-Encoding": string(SnappyBlockCompression),
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:         "no headers",
			reqHeaders:   map[string]string{},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "missing content-type",
			reqHeaders: map[string]string{
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "missing content-encoding",
			reqHeaders: map[string]string{
				"Content-Type":           remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV1],
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "wrong content-type",
			reqHeaders: map[string]string{
				"Content-Type":           "yolo",
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "wrong content-type2",
			reqHeaders: map[string]string{
				"Content-Type":           appProtoContentType + ";proto=yolo",
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "not supported content-encoding",
			reqHeaders: map[string]string{
				"Content-Type":           remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV1],
				"Content-Encoding":       "zstd",
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)
			for k, v := range tc.reqHeaders {
				req.Header.Set(k, v)
			}

			appendable := &mockAppendable{}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			out, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, tc.expectedCode, resp.StatusCode, string(out))
		})
	}
}

func TestRemoteWriteHandlerHeadersHandling_V2Message(t *testing.T) {
	payload, _, _, err := buildV2WriteRequest(log.NewNopLogger(), writeV2RequestFixture.Timeseries, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
	require.NoError(t, err)

	for _, tc := range []struct {
		name         string
		reqHeaders   map[string]string
		expectedCode int
	}{
		{
			name: "correct PRW 2.0 headers",
			reqHeaders: map[string]string{
				"Content-Type":           remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2],
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "missing remote write version",
			reqHeaders: map[string]string{
				"Content-Type":     remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2],
				"Content-Encoding": string(SnappyBlockCompression),
			},
			expectedCode: http.StatusNoContent, // We don't check for now.
		},
		{
			name:         "no headers",
			reqHeaders:   map[string]string{},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "missing content-type",
			reqHeaders: map[string]string{
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			// This only gives 415, because we explicitly only support 2.0. If we supported both
			// (default) it would be empty message parsed and ok response.
			// This is perhaps better, than 415 for previously working 1.0 flow with
			// no content-type.
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "missing content-encoding",
			reqHeaders: map[string]string{
				"Content-Type":           remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2],
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent, // Similar to 1.0 impl, we default to Snappy, so it works.
		},
		{
			name: "wrong content-type",
			reqHeaders: map[string]string{
				"Content-Type":           "yolo",
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "wrong content-type2",
			reqHeaders: map[string]string{
				"Content-Type":           appProtoContentType + ";proto=yolo",
				"Content-Encoding":       string(SnappyBlockCompression),
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "not supported content-encoding",
			reqHeaders: map[string]string{
				"Content-Type":           remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2],
				"Content-Encoding":       "zstd",
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)
			for k, v := range tc.reqHeaders {
				req.Header.Set(k, v)
			}

			appendable := &mockAppendable{}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			out, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, tc.expectedCode, resp.StatusCode, string(out))
		})
	}
}

func TestRemoteWriteHandler_V1Message(t *testing.T) {
	payload, _, _, err := buildWriteRequest(nil, writeRequestFixture.Timeseries, nil, nil, nil, nil, "snappy")
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(payload))
	require.NoError(t, err)

	// NOTE: Strictly speaking, even for 1.0 we require headers, but we never verified those
	// in Prometheus, so keeping like this to not break existing 1.0 clients.

	appendable := &mockAppendable{}
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	b := labels.NewScratchBuilder(0)
	i := 0
	j := 0
	k := 0
	for _, ts := range writeRequestFixture.Timeseries {
		labels := ts.ToLabels(&b, nil)
		for _, s := range ts.Samples {
			requireEqual(t, mockSample{labels, s.Timestamp, s.Value}, appendable.samples[i])
			i++
		}
		for _, e := range ts.Exemplars {
			exemplarLabels := e.ToExemplar(&b, nil).Labels
			requireEqual(t, mockExemplar{labels, exemplarLabels, e.Timestamp, e.Value}, appendable.exemplars[j])
			j++
		}
		for _, hp := range ts.Histograms {
			if hp.IsFloatHistogram() {
				fh := hp.ToFloatHistogram()
				requireEqual(t, mockHistogram{labels, hp.Timestamp, nil, fh}, appendable.histograms[k])
			} else {
				h := hp.ToIntHistogram()
				requireEqual(t, mockHistogram{labels, hp.Timestamp, h, nil}, appendable.histograms[k])
			}

			k++
		}
	}
}

func TestRemoteWriteHandler_V2Message(t *testing.T) {
	payload, _, _, err := buildV2WriteRequest(log.NewNopLogger(), writeV2RequestFixture.Timeseries, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(payload))
	require.NoError(t, err)

	req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
	req.Header.Set("Content-Encoding", string(SnappyBlockCompression))
	req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

	appendable := &mockAppendable{}
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	b := labels.NewScratchBuilder(0)
	i := 0
	j := 0
	k := 0
	for _, ts := range writeV2RequestFixture.Timeseries {
		ls := ts.ToLabels(&b, writeV2RequestFixture.Symbols)

		for _, s := range ts.Samples {
			requireEqual(t, mockSample{ls, s.Timestamp, s.Value}, appendable.samples[i])

			switch i {
			case 0:
				requireEqual(t, mockMetadata{ls, writeV2RequestSeries1Metadata}, appendable.metadata[i])
			case 1:
				requireEqual(t, mockMetadata{ls, writeV2RequestSeries2Metadata}, appendable.metadata[i])
			default:
				t.Fatal("more series/samples then expected")
			}
			i++
		}
		for _, e := range ts.Exemplars {
			exemplarLabels := e.ToExemplar(&b, writeV2RequestFixture.Symbols).Labels
			requireEqual(t, mockExemplar{ls, exemplarLabels, e.Timestamp, e.Value}, appendable.exemplars[j])
			j++
		}
		for _, hp := range ts.Histograms {
			if hp.IsFloatHistogram() {
				fh := hp.ToFloatHistogram()
				requireEqual(t, mockHistogram{ls, hp.Timestamp, nil, fh}, appendable.histograms[k])
			} else {
				h := hp.ToIntHistogram()
				requireEqual(t, mockHistogram{ls, hp.Timestamp, h, nil}, appendable.histograms[k])
			}
			k++
		}
	}
}

func TestOutOfOrderSample_V1Message(t *testing.T) {
	for _, tc := range []struct {
		Name      string
		Timestamp int64
	}{
		{
			Name:      "historic",
			Timestamp: 0,
		},
		{
			Name:      "future",
			Timestamp: math.MaxInt64,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			payload, _, _, err := buildWriteRequest(nil, []prompb.TimeSeries{{
				Labels:  []prompb.Label{{Name: "__name__", Value: "test_metric"}},
				Samples: []prompb.Sample{{Value: 1, Timestamp: tc.Timestamp}},
			}}, nil, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			appendable := &mockAppendable{latestSample: 100}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestOutOfOrderSample_V2Message(t *testing.T) {
	for _, tc := range []struct {
		Name      string
		Timestamp int64
	}{
		{
			Name:      "historic",
			Timestamp: 0,
		},
		{
			Name:      "future",
			Timestamp: math.MaxInt64,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			payload, _, _, err := buildV2WriteRequest(nil, []writev2.TimeSeries{{
				LabelsRefs: []uint32{1, 2},
				Samples:    []writev2.Sample{{Value: 1, Timestamp: tc.Timestamp}},
			}}, []string{"", "__name__", "metric1"}, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
			req.Header.Set("Content-Encoding", string(SnappyBlockCompression))
			req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

			appendable := &mockAppendable{latestSample: 100}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

// This test case currently aims to verify that the WriteHandler endpoint
// don't fail on exemplar ingestion errors since the exemplar storage is
// still experimental.
func TestOutOfOrderExemplar_V1Message(t *testing.T) {
	tests := []struct {
		Name      string
		Timestamp int64
	}{
		{
			Name:      "historic",
			Timestamp: 0,
		},
		{
			Name:      "future",
			Timestamp: math.MaxInt64,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			payload, _, _, err := buildWriteRequest(nil, []prompb.TimeSeries{{
				Labels:    []prompb.Label{{Name: "__name__", Value: "test_metric"}},
				Exemplars: []prompb.Exemplar{{Labels: []prompb.Label{{Name: "foo", Value: "bar"}}, Value: 1, Timestamp: tc.Timestamp}},
			}}, nil, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			appendable := &mockAppendable{latestExemplar: 100}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			// TODO: update to require.Equal(t, http.StatusConflict, resp.StatusCode) once exemplar storage is not experimental.
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	}
}

func TestOutOfOrderExemplar_V2Message(t *testing.T) {
	tests := []struct {
		Name      string
		Timestamp int64
	}{
		{
			Name:      "historic",
			Timestamp: 0,
		},
		{
			Name:      "future",
			Timestamp: math.MaxInt64,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			payload, _, _, err := buildV2WriteRequest(nil, []writev2.TimeSeries{{
				LabelsRefs: []uint32{1, 2},
				Exemplars:  []writev2.Exemplar{{LabelsRefs: []uint32{3, 4}, Value: 1, Timestamp: tc.Timestamp}},
			}}, []string{"", "__name__", "metric1", "foo", "bar"}, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
			req.Header.Set("Content-Encoding", string(SnappyBlockCompression))
			req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

			appendable := &mockAppendable{latestExemplar: 100}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			// TODO: update to require.Equal(t, http.StatusConflict, resp.StatusCode) once exemplar storage is not experimental.
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	}
}

func TestOutOfOrderHistogram_V1Message(t *testing.T) {
	for _, tc := range []struct {
		Name      string
		Timestamp int64
	}{
		{
			Name:      "historic",
			Timestamp: 0,
		},
		{
			Name:      "future",
			Timestamp: math.MaxInt64,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			payload, _, _, err := buildWriteRequest(nil, []prompb.TimeSeries{{
				Labels:     []prompb.Label{{Name: "__name__", Value: "test_metric"}},
				Histograms: []prompb.Histogram{prompb.FromIntHistogram(tc.Timestamp, &testHistogram), prompb.FromFloatHistogram(1, testHistogram.ToFloat(nil))},
			}}, nil, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			appendable := &mockAppendable{latestHistogram: 100}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestOutOfOrderHistogram_V2Message(t *testing.T) {
	for _, tc := range []struct {
		Name      string
		Timestamp int64
	}{
		{
			Name:      "historic",
			Timestamp: 0,
		},
		{
			Name:      "future",
			Timestamp: math.MaxInt64,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			payload, _, _, err := buildV2WriteRequest(nil, []writev2.TimeSeries{{
				LabelsRefs: []uint32{0, 1},
				Histograms: []writev2.Histogram{writev2.FromIntHistogram(0, &testHistogram), writev2.FromFloatHistogram(1, testHistogram.ToFloat(nil))},
			}}, []string{"__name__", "metric1"}, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
			req.Header.Set("Content-Encoding", string(SnappyBlockCompression))
			req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

			appendable := &mockAppendable{latestHistogram: 100}
			handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func BenchmarkRemoteWriteHandler(b *testing.B) {
	const labelValue = "abcdefg'hijlmn234!@#$%^&*()_+~`\"{}[],./<>?hello0123hiOlá你好Dzieńdobry9Zd8ra765v4stvuyte"
	var reqs []*http.Request
	for i := 0; i < b.N; i++ {
		num := strings.Repeat(strconv.Itoa(i), 16)
		buf, _, _, err := buildWriteRequest(nil, []prompb.TimeSeries{{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric"},
				{Name: "test_label_name_" + num, Value: labelValue + num},
			},
			Histograms: []prompb.Histogram{prompb.FromIntHistogram(0, &testHistogram)},
		}}, nil, nil, nil, nil, "snappy")
		require.NoError(b, err)
		req, err := http.NewRequest("", "", bytes.NewReader(buf))
		require.NoError(b, err)
		reqs = append(reqs, req)
	}

	appendable := &mockAppendable{}
	// TODO: test with other proto format(s)
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})
	recorder := httptest.NewRecorder()

	b.ResetTimer()
	for _, req := range reqs {
		handler.ServeHTTP(recorder, req)
	}
}

func TestCommitErr_V1Message(t *testing.T) {
	payload, _, _, err := buildWriteRequest(nil, writeRequestFixture.Timeseries, nil, nil, nil, nil, "snappy")
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(payload))
	require.NoError(t, err)

	appendable := &mockAppendable{commitErr: fmt.Errorf("commit error")}
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "commit error\n", string(body))
}

func TestCommitErr_V2Message(t *testing.T) {
	payload, _, _, err := buildV2WriteRequest(log.NewNopLogger(), writeV2RequestFixture.Timeseries, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(payload))
	require.NoError(t, err)

	req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
	req.Header.Set("Content-Encoding", string(SnappyBlockCompression))
	req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

	appendable := &mockAppendable{commitErr: fmt.Errorf("commit error")}
	handler := NewWriteHandler(log.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "commit error\n", string(body))
}

func BenchmarkRemoteWriteOOOSamples(b *testing.B) {
	b.Skip("Not a valid benchmark (does not count to b.N)")
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
	handler := NewWriteHandler(log.NewNopLogger(), nil, db.Head(), []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})

	buf, _, _, err := buildWriteRequest(nil, genSeriesWithSample(1000, 200*time.Minute.Milliseconds()), nil, nil, nil, nil, "snappy")
	require.NoError(b, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(b, err)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	require.Equal(b, http.StatusNoContent, recorder.Code)
	require.Equal(b, uint64(1000), db.Head().NumSeries())

	var bufRequests [][]byte
	for i := 0; i < 100; i++ {
		buf, _, _, err = buildWriteRequest(nil, genSeriesWithSample(1000, int64(80+i)*time.Minute.Milliseconds()), nil, nil, nil, nil, "snappy")
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
	metadata        []mockMetadata

	commitErr error
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

type mockMetadata struct {
	l labels.Labels
	m metadata.Metadata
}

// Wrapper to instruct go-cmp package to compare a list of structs with unexported fields.
func requireEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()

	testutil.RequireEqualWithOptions(t, expected, actual,
		[]cmp.Option{cmp.AllowUnexported(mockSample{}), cmp.AllowUnexported(mockExemplar{}), cmp.AllowUnexported(mockHistogram{}), cmp.AllowUnexported(mockMetadata{})},
		msgAndArgs...)
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
	m.metadata = append(m.metadata, mockMetadata{l: l, m: mp})
	return 0, nil
}

func (m *mockAppendable) AppendCTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, _ int64) (storage.SeriesRef, error) {
	// AppendCTZeroSample is no-op for remote-write for now.
	// TODO(bwplotka): Add support for PRW 2.0 for CT zero feature (but also we might
	// replace this with in-metadata CT storage, see https://github.com/prometheus/prometheus/issues/14218).
	return 0, nil
}
