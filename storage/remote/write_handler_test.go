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
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/promslog"
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
	"github.com/prometheus/prometheus/util/compression"
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
				"Content-Encoding":       compression.Snappy,
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "missing remote write version",
			reqHeaders: map[string]string{
				"Content-Type":     remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV1],
				"Content-Encoding": compression.Snappy,
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
				"Content-Encoding":       compression.Snappy,
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
				"Content-Encoding":       compression.Snappy,
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "wrong content-type2",
			reqHeaders: map[string]string{
				"Content-Type":           appProtoContentType + ";proto=yolo",
				"Content-Encoding":       compression.Snappy,
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
			handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

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
	payload, _, _, err := buildV2WriteRequest(promslog.NewNopLogger(), writeV2RequestFixture.Timeseries, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
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
				"Content-Encoding":       compression.Snappy,
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "missing remote write version",
			reqHeaders: map[string]string{
				"Content-Type":     remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2],
				"Content-Encoding": compression.Snappy,
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
				"Content-Encoding":       compression.Snappy,
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
				"Content-Encoding":       compression.Snappy,
				RemoteWriteVersionHeader: RemoteWriteVersion20HeaderValue,
			},
			expectedCode: http.StatusUnsupportedMediaType,
		},
		{
			name: "wrong content-type2",
			reqHeaders: map[string]string{
				"Content-Type":           appProtoContentType + ";proto=yolo",
				"Content-Encoding":       compression.Snappy,
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
			handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2}, false)

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
	handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

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

func expectHeaderValue(t testing.TB, expected int, got string) {
	t.Helper()

	require.NotEmpty(t, got)
	i, err := strconv.Atoi(got)
	require.NoError(t, err)
	require.Equal(t, expected, i)
}

func TestRemoteWriteHandler_V2Message(t *testing.T) {
	// V2 supports partial writes for non-retriable errors, so test them.
	for _, tc := range []struct {
		desc             string
		input            []writev2.TimeSeries
		expectedCode     int
		expectedRespBody string

		commitErr             error
		appendSampleErr       error
		appendCTZeroSampleErr error
		appendHistogramErr    error
		appendExemplarErr     error
		updateMetadataErr     error

		ingestCTZeroSample bool
	}{
		{
			desc:               "All timeseries accepted/ct_enabled",
			input:              writeV2RequestFixture.Timeseries,
			expectedCode:       http.StatusNoContent,
			ingestCTZeroSample: true,
		},
		{
			desc:         "All timeseries accepted/ct_disabled",
			input:        writeV2RequestFixture.Timeseries,
			expectedCode: http.StatusNoContent,
		},
		{
			desc: "Partial write; first series with invalid labels (no metric name)",
			input: append(
				// Series with test_metric1="test_metric1" labels.
				[]writev2.TimeSeries{{LabelsRefs: []uint32{2, 2}, Samples: []writev2.Sample{{Value: 1, Timestamp: 1}}}},
				writeV2RequestFixture.Timeseries...),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "invalid metric name or labels, got {test_metric1=\"test_metric1\"}\n",
		},
		{
			desc: "Partial write; first series with invalid labels (empty metric name)",
			input: append(
				// Series with __name__="" labels.
				[]writev2.TimeSeries{{LabelsRefs: []uint32{1, 0}, Samples: []writev2.Sample{{Value: 1, Timestamp: 1}}}},
				writeV2RequestFixture.Timeseries...),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "invalid metric name or labels, got {__name__=\"\"}\n",
		},
		{
			desc: "Partial write; first series with duplicate labels",
			input: append(
				// Series with __name__="test_metric1",test_metric1="test_metric1",test_metric1="test_metric1" labels.
				[]writev2.TimeSeries{{LabelsRefs: []uint32{1, 2, 2, 2, 2, 2}, Samples: []writev2.Sample{{Value: 1, Timestamp: 1}}}},
				writeV2RequestFixture.Timeseries...),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "invalid labels for series, labels {__name__=\"test_metric1\", test_metric1=\"test_metric1\", test_metric1=\"test_metric1\"}, duplicated label test_metric1\n",
		},
		{
			desc: "Partial write; first series with one OOO sample",
			input: func() []writev2.TimeSeries {
				f := proto.Clone(writeV2RequestFixture).(*writev2.Request)
				f.Timeseries[0].Samples = append(f.Timeseries[0].Samples, writev2.Sample{Value: 2, Timestamp: 0})
				return f.Timeseries
			}(),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "out of order sample for series {__name__=\"test_metric1\", b=\"c\", baz=\"qux\", d=\"e\", foo=\"bar\"}\n",
		},
		{
			desc: "Partial write; first series with one dup sample",
			input: func() []writev2.TimeSeries {
				f := proto.Clone(writeV2RequestFixture).(*writev2.Request)
				f.Timeseries[0].Samples = append(f.Timeseries[0].Samples, f.Timeseries[0].Samples[0])
				return f.Timeseries
			}(),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "duplicate sample for timestamp for series {__name__=\"test_metric1\", b=\"c\", baz=\"qux\", d=\"e\", foo=\"bar\"}\n",
		},
		{
			desc: "Partial write; first series with one OOO histogram sample",
			input: func() []writev2.TimeSeries {
				f := proto.Clone(writeV2RequestFixture).(*writev2.Request)
				f.Timeseries[0].Histograms = append(f.Timeseries[0].Histograms, writev2.FromFloatHistogram(1, testHistogram.ToFloat(nil)))
				return f.Timeseries
			}(),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "out of order sample for series {__name__=\"test_metric1\", b=\"c\", baz=\"qux\", d=\"e\", foo=\"bar\"}\n",
		},
		{
			desc: "Partial write; first series with one dup histogram sample",
			input: func() []writev2.TimeSeries {
				f := proto.Clone(writeV2RequestFixture).(*writev2.Request)
				f.Timeseries[0].Histograms = append(f.Timeseries[0].Histograms, f.Timeseries[0].Histograms[1])
				return f.Timeseries
			}(),
			expectedCode:     http.StatusBadRequest,
			expectedRespBody: "duplicate sample for timestamp for series {__name__=\"test_metric1\", b=\"c\", baz=\"qux\", d=\"e\", foo=\"bar\"}\n",
		},
		// Non retriable errors from various parts.
		{
			desc:            "Internal sample append error; rollback triggered",
			input:           writeV2RequestFixture.Timeseries,
			appendSampleErr: errors.New("some sample internal append error"),

			expectedCode:     http.StatusInternalServerError,
			expectedRespBody: "some sample internal append error\n",
		},
		{
			desc:               "Internal histogram sample append error; rollback triggered",
			input:              writeV2RequestFixture.Timeseries,
			appendHistogramErr: errors.New("some histogram sample internal append error"),

			expectedCode:     http.StatusInternalServerError,
			expectedRespBody: "some histogram sample internal append error\n",
		},
		{
			desc:              "Partial write; skipped exemplar; exemplar storage errs are noop",
			input:             writeV2RequestFixture.Timeseries,
			appendExemplarErr: errors.New("some exemplar internal append error"),

			expectedCode: http.StatusNoContent,
		},
		{
			desc:              "Partial write; skipped metadata; metadata storage errs are noop",
			input:             writeV2RequestFixture.Timeseries,
			updateMetadataErr: errors.New("some metadata update error"),

			expectedCode: http.StatusNoContent,
		},
		{
			desc:      "Internal commit error; rollback triggered",
			input:     writeV2RequestFixture.Timeseries,
			commitErr: errors.New("storage error"),

			expectedCode:     http.StatusInternalServerError,
			expectedRespBody: "storage error\n",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			payload, _, _, err := buildV2WriteRequest(promslog.NewNopLogger(), tc.input, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewReader(payload))
			require.NoError(t, err)

			req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
			req.Header.Set("Content-Encoding", compression.Snappy)
			req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

			appendable := &mockAppendable{
				commitErr:             tc.commitErr,
				appendSampleErr:       tc.appendSampleErr,
				appendCTZeroSampleErr: tc.appendCTZeroSampleErr,
				appendHistogramErr:    tc.appendHistogramErr,
				appendExemplarErr:     tc.appendExemplarErr,
				updateMetadataErr:     tc.updateMetadataErr,
			}
			handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2}, tc.ingestCTZeroSample)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			require.Equal(t, tc.expectedCode, resp.StatusCode)
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.expectedRespBody, string(respBody))

			if tc.expectedCode == http.StatusInternalServerError {
				// We don't expect writes for partial writes with retry-able code.
				expectHeaderValue(t, 0, resp.Header.Get(rw20WrittenSamplesHeader))
				expectHeaderValue(t, 0, resp.Header.Get(rw20WrittenHistogramsHeader))
				expectHeaderValue(t, 0, resp.Header.Get(rw20WrittenExemplarsHeader))

				require.Empty(t, appendable.samples)
				require.Empty(t, appendable.histograms)
				require.Empty(t, appendable.exemplars)
				require.Empty(t, appendable.metadata)
				return
			}

			// Double check mandatory 2.0 stats.
			// writeV2RequestFixture has 2 series with 1 sample, 2 histograms, 1 exemplar each.
			expectHeaderValue(t, 2, resp.Header.Get(rw20WrittenSamplesHeader))
			expectHeaderValue(t, 4, resp.Header.Get(rw20WrittenHistogramsHeader))
			if tc.appendExemplarErr != nil {
				expectHeaderValue(t, 0, resp.Header.Get(rw20WrittenExemplarsHeader))
			} else {
				expectHeaderValue(t, 2, resp.Header.Get(rw20WrittenExemplarsHeader))
			}

			// Double check what was actually appended.
			var (
				b          = labels.NewScratchBuilder(0)
				i, j, k, m int
			)
			for _, ts := range writeV2RequestFixture.Timeseries {
				ls := ts.ToLabels(&b, writeV2RequestFixture.Symbols)

				for _, s := range ts.Samples {
					if ts.CreatedTimestamp != 0 && tc.ingestCTZeroSample {
						requireEqual(t, mockSample{ls, ts.CreatedTimestamp, 0}, appendable.samples[i])
						i++
					}
					requireEqual(t, mockSample{ls, s.Timestamp, s.Value}, appendable.samples[i])
					i++
				}
				for _, hp := range ts.Histograms {
					if hp.IsFloatHistogram() {
						fh := hp.ToFloatHistogram()
						if ts.CreatedTimestamp != 0 && tc.ingestCTZeroSample {
							requireEqual(t, mockHistogram{ls, ts.CreatedTimestamp, nil, &histogram.FloatHistogram{}}, appendable.histograms[k])
							k++
						}
						requireEqual(t, mockHistogram{ls, hp.Timestamp, nil, fh}, appendable.histograms[k])
					} else {
						h := hp.ToIntHistogram()
						if ts.CreatedTimestamp != 0 && tc.ingestCTZeroSample {
							requireEqual(t, mockHistogram{ls, ts.CreatedTimestamp, &histogram.Histogram{}, nil}, appendable.histograms[k])
							k++
						}
						requireEqual(t, mockHistogram{ls, hp.Timestamp, h, nil}, appendable.histograms[k])
					}
					k++
				}
				if tc.appendExemplarErr == nil {
					for _, e := range ts.Exemplars {
						exemplarLabels := e.ToExemplar(&b, writeV2RequestFixture.Symbols).Labels
						requireEqual(t, mockExemplar{ls, exemplarLabels, e.Timestamp, e.Value}, appendable.exemplars[j])
						j++
					}
				}
				if tc.updateMetadataErr == nil {
					expectedMeta := ts.ToMetadata(writeV2RequestFixture.Symbols)
					requireEqual(t, mockMetadata{ls, expectedMeta}, appendable.metadata[m])
					m++
				}
			}
		})
	}
}

// NOTE: V2 Message is tested in TestRemoteWriteHandler_V2Message.
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

			appendable := &mockAppendable{latestSample: map[uint64]int64{labels.FromStrings("__name__", "test_metric").Hash(): 100}}
			handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

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
// NOTE: V2 Message is tested in TestRemoteWriteHandler_V2Message.
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

			appendable := &mockAppendable{latestSample: map[uint64]int64{labels.FromStrings("__name__", "test_metric").Hash(): 100}}
			handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			// TODO: update to require.Equal(t, http.StatusConflict, resp.StatusCode) once exemplar storage is not experimental.
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	}
}

// NOTE: V2 Message is tested in TestRemoteWriteHandler_V2Message.
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

			appendable := &mockAppendable{latestSample: map[uint64]int64{labels.FromStrings("__name__", "test_metric").Hash(): 100}}
			handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

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
	handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)
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

	appendable := &mockAppendable{commitErr: errors.New("commit error")}
	handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "commit error\n", string(body))
}

func TestCommitErr_V2Message(t *testing.T) {
	payload, _, _, err := buildV2WriteRequest(promslog.NewNopLogger(), writeV2RequestFixture.Timeseries, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(payload))
	require.NoError(t, err)

	req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[config.RemoteWriteProtoMsgV2])
	req.Header.Set("Content-Encoding", compression.Snappy)
	req.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion20HeaderValue)

	appendable := &mockAppendable{commitErr: errors.New("commit error")}
	handler := NewWriteHandler(promslog.NewNopLogger(), nil, appendable, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV2}, false)

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
	handler := NewWriteHandler(promslog.NewNopLogger(), nil, db.Head(), []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1}, false)

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
	latestSample    map[uint64]int64
	samples         []mockSample
	latestExemplar  map[uint64]int64
	exemplars       []mockExemplar
	latestHistogram map[uint64]int64
	latestFloatHist map[uint64]int64
	histograms      []mockHistogram
	metadata        []mockMetadata

	// optional errors to inject.
	commitErr             error
	appendSampleErr       error
	appendCTZeroSampleErr error
	appendHistogramErr    error
	appendExemplarErr     error
	updateMetadataErr     error
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
	if m.latestSample == nil {
		m.latestSample = map[uint64]int64{}
	}
	if m.latestHistogram == nil {
		m.latestHistogram = map[uint64]int64{}
	}
	if m.latestFloatHist == nil {
		m.latestFloatHist = map[uint64]int64{}
	}
	if m.latestExemplar == nil {
		m.latestExemplar = map[uint64]int64{}
	}
	return m
}

func (m *mockAppendable) SetOptions(_ *storage.AppendOptions) {
	panic("unimplemented")
}

func (m *mockAppendable) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if m.appendSampleErr != nil {
		return 0, m.appendSampleErr
	}

	latestTs := m.latestSample[l.Hash()]
	if t < latestTs {
		return 0, storage.ErrOutOfOrderSample
	}
	if t == latestTs {
		return 0, storage.ErrDuplicateSampleForTimestamp
	}

	if l.IsEmpty() {
		return 0, tsdb.ErrInvalidSample
	}
	if _, hasDuplicates := l.HasDuplicateLabelNames(); hasDuplicates {
		return 0, tsdb.ErrInvalidSample
	}

	m.latestSample[l.Hash()] = t
	m.samples = append(m.samples, mockSample{l, t, v})
	return 0, nil
}

func (m *mockAppendable) Commit() error {
	if m.commitErr != nil {
		_ = m.Rollback() // As per Commit method contract.
	}
	return m.commitErr
}

func (m *mockAppendable) Rollback() error {
	m.samples = m.samples[:0]
	m.exemplars = m.exemplars[:0]
	m.histograms = m.histograms[:0]
	m.metadata = m.metadata[:0]
	return nil
}

func (m *mockAppendable) AppendExemplar(_ storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	if m.appendExemplarErr != nil {
		return 0, m.appendExemplarErr
	}

	latestTs := m.latestExemplar[l.Hash()]
	if e.Ts < latestTs {
		return 0, storage.ErrOutOfOrderExemplar
	}
	if e.Ts == latestTs {
		return 0, storage.ErrDuplicateExemplar
	}

	m.latestExemplar[l.Hash()] = e.Ts
	m.exemplars = append(m.exemplars, mockExemplar{l, e.Labels, e.Ts, e.Value})
	return 0, nil
}

func (m *mockAppendable) AppendHistogram(_ storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if m.appendHistogramErr != nil {
		return 0, m.appendHistogramErr
	}

	var latestTs int64
	if h != nil {
		latestTs = m.latestHistogram[l.Hash()]
	} else {
		latestTs = m.latestFloatHist[l.Hash()]
	}
	if t < latestTs {
		return 0, storage.ErrOutOfOrderSample
	}
	if t == latestTs {
		return 0, storage.ErrDuplicateSampleForTimestamp
	}

	if l.IsEmpty() {
		return 0, tsdb.ErrInvalidSample
	}
	if _, hasDuplicates := l.HasDuplicateLabelNames(); hasDuplicates {
		return 0, tsdb.ErrInvalidSample
	}

	if h != nil {
		m.latestHistogram[l.Hash()] = t
	} else {
		m.latestFloatHist[l.Hash()] = t
	}
	m.histograms = append(m.histograms, mockHistogram{l, t, h, fh})
	return 0, nil
}

func (m *mockAppendable) AppendHistogramCTZeroSample(_ storage.SeriesRef, l labels.Labels, t, ct int64, h *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if m.appendCTZeroSampleErr != nil {
		return 0, m.appendCTZeroSampleErr
	}

	// Created Timestamp can't be higher than the original sample's timestamp.
	if ct > t {
		return 0, storage.ErrOutOfOrderSample
	}

	var latestTs int64
	if h != nil {
		latestTs = m.latestHistogram[l.Hash()]
	} else {
		latestTs = m.latestFloatHist[l.Hash()]
	}
	if ct < latestTs {
		return 0, storage.ErrOutOfOrderSample
	}
	if ct == latestTs {
		return 0, storage.ErrDuplicateSampleForTimestamp
	}

	if l.IsEmpty() {
		return 0, tsdb.ErrInvalidSample
	}

	if _, hasDuplicates := l.HasDuplicateLabelNames(); hasDuplicates {
		return 0, tsdb.ErrInvalidSample
	}

	if h != nil {
		m.latestHistogram[l.Hash()] = ct
		m.histograms = append(m.histograms, mockHistogram{l, ct, &histogram.Histogram{}, nil})
	} else {
		m.latestFloatHist[l.Hash()] = ct
		m.histograms = append(m.histograms, mockHistogram{l, ct, nil, &histogram.FloatHistogram{}})
	}
	return 0, nil
}

func (m *mockAppendable) UpdateMetadata(_ storage.SeriesRef, l labels.Labels, mp metadata.Metadata) (storage.SeriesRef, error) {
	if m.updateMetadataErr != nil {
		return 0, m.updateMetadataErr
	}

	m.metadata = append(m.metadata, mockMetadata{l: l, m: mp})
	return 0, nil
}

func (m *mockAppendable) AppendCTZeroSample(_ storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	if m.appendCTZeroSampleErr != nil {
		return 0, m.appendCTZeroSampleErr
	}

	// Created Timestamp can't be higher than the original sample's timestamp.
	if ct > t {
		return 0, storage.ErrOutOfOrderSample
	}

	latestTs := m.latestSample[l.Hash()]
	if ct < latestTs {
		return 0, storage.ErrOutOfOrderSample
	}
	if ct == latestTs {
		return 0, storage.ErrDuplicateSampleForTimestamp
	}

	if l.IsEmpty() {
		return 0, tsdb.ErrInvalidSample
	}
	if _, hasDuplicates := l.HasDuplicateLabelNames(); hasDuplicates {
		return 0, tsdb.ErrInvalidSample
	}

	m.latestSample[l.Hash()] = ct
	m.samples = append(m.samples, mockSample{l, ct, 0})
	return 0, nil
}
