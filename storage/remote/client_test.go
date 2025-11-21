// Copyright 2017 The Prometheus Authors
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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/testutil"
)

var longErrMessage = strings.Repeat("error message", maxErrMsgLen)

func TestStoreHTTPErrorHandling(t *testing.T) {
	tests := []struct {
		code int
		err  error
	}{
		{
			code: 200,
			err:  nil,
		},
		{
			code: 300,
			err:  errors.New("server returned HTTP status 300 Multiple Choices: " + longErrMessage[:maxErrMsgLen]),
		},
		{
			code: 404,
			err:  errors.New("server returned HTTP status 404 Not Found: " + longErrMessage[:maxErrMsgLen]),
		},
		{
			code: 500,
			err:  RecoverableError{errors.New("server returned HTTP status 500 Internal Server Error: " + longErrMessage[:maxErrMsgLen]), defaultBackoff},
		},
	}

	for _, test := range tests {
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, longErrMessage, test.code)
			}),
		)

		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)

		conf := &ClientConfig{
			URL:     &config_util.URL{URL: serverURL},
			Timeout: model.Duration(time.Second),
		}

		hash, err := toHash(conf)
		require.NoError(t, err)
		c, err := NewWriteClient(hash, conf)
		require.NoError(t, err)

		_, err = c.Store(context.Background(), []byte{}, 0)
		if test.err != nil {
			require.EqualError(t, err, test.err.Error())
		} else {
			require.NoError(t, err)
		}

		server.Close()
	}
}

func TestClientRetryAfter(t *testing.T) {
	setupServer := func(statusCode int) *httptest.Server {
		return httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "5")
				http.Error(w, longErrMessage, statusCode)
			}),
		)
	}

	getClientConfig := func(serverURL *url.URL, retryOnRateLimit bool) *ClientConfig {
		return &ClientConfig{
			URL:              &config_util.URL{URL: serverURL},
			Timeout:          model.Duration(time.Second),
			RetryOnRateLimit: retryOnRateLimit,
		}
	}

	getClient := func(conf *ClientConfig) WriteClient {
		hash, err := toHash(conf)
		require.NoError(t, err)
		c, err := NewWriteClient(hash, conf)
		require.NoError(t, err)
		return c
	}

	testCases := []struct {
		name                string
		statusCode          int
		retryOnRateLimit    bool
		expectedRecoverable bool
		expectedRetryAfter  model.Duration
	}{
		{"TooManyRequests - No Retry", http.StatusTooManyRequests, false, false, 0},
		{"TooManyRequests - With Retry", http.StatusTooManyRequests, true, true, 5 * model.Duration(time.Second)},
		{"InternalServerError", http.StatusInternalServerError, false, true, 5 * model.Duration(time.Second)}, // HTTP 5xx errors do not depend on retryOnRateLimit.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := setupServer(tc.statusCode)
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			c := getClient(getClientConfig(serverURL, tc.retryOnRateLimit))

			var recErr RecoverableError
			_, err = c.Store(context.Background(), []byte{}, 0)
			require.Equal(t, tc.expectedRecoverable, errors.As(err, &recErr), "Mismatch in expected recoverable error status.")
			if tc.expectedRecoverable {
				require.Equal(t, tc.expectedRetryAfter, recErr.retryAfter)
			}
		})
	}
}

func TestRetryAfterDuration(t *testing.T) {
	tc := []struct {
		name     string
		tInput   string
		expected model.Duration
	}{
		{
			name:     "seconds",
			tInput:   "120",
			expected: model.Duration(time.Second * 120),
		},
		{
			name:     "date-time default",
			tInput:   time.RFC1123, // Expected layout is http.TimeFormat, hence an error.
			expected: defaultBackoff,
		},
		{
			name:     "retry-after not provided",
			tInput:   "", // Expected layout is http.TimeFormat, hence an error.
			expected: defaultBackoff,
		},
	}
	for _, c := range tc {
		require.Equal(t, c.expected, retryAfterDuration(c.tInput), c.name)
	}
}

func TestClientCustomHeaders(t *testing.T) {
	headersToSend := map[string]string{"Foo": "Bar", "Baz": "qux"}

	var called bool
	server := httptest.NewServer(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			receivedHeaders := r.Header
			for name, value := range headersToSend {
				require.Equal(
					t,
					[]string{value},
					receivedHeaders.Values(name),
					"expected %v to be part of the received headers %v",
					headersToSend,
					receivedHeaders,
				)
			}
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	conf := &ClientConfig{
		URL:     &config_util.URL{URL: serverURL},
		Timeout: model.Duration(time.Second),
		Headers: headersToSend,
	}

	c, err := NewWriteClient("c", conf)
	require.NoError(t, err)

	_, err = c.Store(context.Background(), []byte{}, 0)
	require.NoError(t, err)

	require.True(t, called, "The remote server wasn't called")
}

func TestReadClient(t *testing.T) {
	tests := []struct {
		name                  string
		query                 *prompb.Query
		httpHandler           http.HandlerFunc
		timeout               time.Duration
		expectedLabels        []map[string]string
		expectedSamples       [][]model.SamplePair
		expectedErrorContains string
		sortSeries            bool
		unwrap                bool
	}{
		{
			name:        "sorted sampled response",
			httpHandler: sampledResponseHTTPHandler(t),
			expectedLabels: []map[string]string{
				{"foo1": "bar"},
				{"foo2": "bar"},
			},
			expectedSamples: [][]model.SamplePair{
				{
					{Timestamp: model.Time(0), Value: model.SampleValue(3)},
					{Timestamp: model.Time(5), Value: model.SampleValue(4)},
				},
				{
					{Timestamp: model.Time(0), Value: model.SampleValue(1)},
					{Timestamp: model.Time(5), Value: model.SampleValue(2)},
				},
			},
			expectedErrorContains: "",
			sortSeries:            true,
		},
		{
			name:        "unsorted sampled response",
			httpHandler: sampledResponseHTTPHandler(t),
			expectedLabels: []map[string]string{
				{"foo2": "bar"},
				{"foo1": "bar"},
			},
			expectedSamples: [][]model.SamplePair{
				{
					{Timestamp: model.Time(0), Value: model.SampleValue(1)},
					{Timestamp: model.Time(5), Value: model.SampleValue(2)},
				},
				{
					{Timestamp: model.Time(0), Value: model.SampleValue(3)},
					{Timestamp: model.Time(5), Value: model.SampleValue(4)},
				},
			},
			expectedErrorContains: "",
			sortSeries:            false,
		},
		{
			name: "chunked response",
			query: &prompb.Query{
				StartTimestampMs: 4000,
				EndTimestampMs:   12000,
			},
			httpHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse")

				flusher, ok := w.(http.Flusher)
				require.True(t, ok)

				cw := NewChunkedWriter(w, flusher)
				l := []prompb.Label{
					{Name: "foo", Value: "bar"},
				}

				chunks := buildTestChunks(t)
				for i, c := range chunks {
					cSeries := prompb.ChunkedSeries{Labels: l, Chunks: []prompb.Chunk{c}}
					readResp := prompb.ChunkedReadResponse{
						ChunkedSeries: []*prompb.ChunkedSeries{&cSeries},
						QueryIndex:    int64(i),
					}

					b, err := proto.Marshal(&readResp)
					require.NoError(t, err)

					_, err = cw.Write(b)
					require.NoError(t, err)
				}
			}),
			expectedLabels: []map[string]string{
				{"foo": "bar"},
				{"foo": "bar"},
				{"foo": "bar"},
			},
			// This is the output of buildTestChunks minus the samples outside the query range.
			expectedSamples: [][]model.SamplePair{
				{
					{Timestamp: model.Time(4000), Value: model.SampleValue(4)},
				},
				{
					{Timestamp: model.Time(5000), Value: model.SampleValue(1)},
					{Timestamp: model.Time(6000), Value: model.SampleValue(2)},
					{Timestamp: model.Time(7000), Value: model.SampleValue(3)},
					{Timestamp: model.Time(8000), Value: model.SampleValue(4)},
					{Timestamp: model.Time(9000), Value: model.SampleValue(5)},
				},
				{
					{Timestamp: model.Time(10000), Value: model.SampleValue(2)},
					{Timestamp: model.Time(11000), Value: model.SampleValue(3)},
					{Timestamp: model.Time(12000), Value: model.SampleValue(4)},
				},
			},
			expectedErrorContains: "",
		},
		{
			name: "unsupported content type",
			httpHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "foobar")
			}),
			expectedErrorContains: "unsupported content type",
		},
		{
			name:                  "timeout",
			httpHandler:           delayedResponseHTTPHandler(t, 15*time.Millisecond),
			timeout:               5 * time.Millisecond,
			expectedErrorContains: "context deadline exceeded: request timed out after 5ms",
		},
		{
			name: "unwrap error",
			httpHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "test error", http.StatusBadRequest)
			}),
			expectedErrorContains: "test error",
			unwrap:                true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.httpHandler)
			defer server.Close()

			u, err := url.Parse(server.URL)
			require.NoError(t, err)

			if test.timeout == 0 {
				test.timeout = 5 * time.Second
			}

			conf := &ClientConfig{
				URL:              &config_util.URL{URL: u},
				Timeout:          model.Duration(test.timeout),
				ChunkedReadLimit: config.DefaultChunkedReadLimit,
			}
			c, err := NewReadClient("test", conf)
			require.NoError(t, err)

			query := &prompb.Query{}
			if test.query != nil {
				query = test.query
			}

			ss, err := c.Read(context.Background(), query, test.sortSeries)
			if test.expectedErrorContains != "" {
				require.ErrorContains(t, err, test.expectedErrorContains)
				if test.unwrap {
					err = errors.Unwrap(err)
					require.EqualError(t, err, test.expectedErrorContains)
				}
				return
			}

			require.NoError(t, err)

			i := 0

			for ss.Next() {
				require.NoError(t, ss.Err())
				s := ss.At()

				l := s.Labels()
				require.Len(t, test.expectedLabels[i], l.Len())
				for k, v := range test.expectedLabels[i] {
					require.True(t, l.Has(k))
					require.Equal(t, v, l.Get(k))
				}

				it := s.Iterator(nil)
				j := 0

				for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
					require.NoError(t, it.Err())

					ts, v := it.At()
					expectedSample := test.expectedSamples[i][j]

					require.Equal(t, int64(expectedSample.Timestamp), ts)
					require.Equal(t, float64(expectedSample.Value), v)

					j++
				}

				require.Len(t, test.expectedSamples[i], j)

				i++
			}

			require.NoError(t, ss.Err())
		})
	}
}

func sampledResponseHTTPHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")

		resp := prompb.ReadResponse{
			Results: []*prompb.QueryResult{
				{
					Timeseries: []*prompb.TimeSeries{
						{
							Labels: []prompb.Label{
								{Name: "foo2", Value: "bar"},
							},
							Samples: []prompb.Sample{
								{Value: float64(1), Timestamp: int64(0)},
								{Value: float64(2), Timestamp: int64(5)},
							},
							Exemplars: []prompb.Exemplar{},
						},
						{
							Labels: []prompb.Label{
								{Name: "foo1", Value: "bar"},
							},
							Samples: []prompb.Sample{
								{Value: float64(3), Timestamp: int64(0)},
								{Value: float64(4), Timestamp: int64(5)},
							},
							Exemplars: []prompb.Exemplar{},
						},
					},
				},
			},
		}
		b, err := proto.Marshal(&resp)
		require.NoError(t, err)

		_, err = w.Write(snappy.Encode(nil, b))
		require.NoError(t, err)
	}
}

func delayedResponseHTTPHandler(t *testing.T, delay time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)

		w.Header().Set("Content-Type", "application/x-protobuf")
		b, err := proto.Marshal(&prompb.ReadResponse{})
		require.NoError(t, err)

		_, err = w.Write(snappy.Encode(nil, b))
		require.NoError(t, err)
	}
}

func TestReadMultipleErrorHandling(t *testing.T) {
	m := &mockedRemoteClient{
		store: []*prompb.TimeSeries{
			{Labels: []prompb.Label{{Name: "job", Value: "prometheus"}}},
		},
		b: labels.NewScratchBuilder(0),
	}

	// Test with invalid matcher - should return error
	queries := []*prompb.Query{
		{
			StartTimestampMs: 1000,
			EndTimestampMs:   2000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_Type(999), Name: "job", Value: "prometheus"}, // invalid matcher type
			},
		},
	}

	result, err := m.ReadMultiple(context.Background(), queries, true)
	require.Error(t, err)
	require.Nil(t, result)
}

func TestReadMultiple(t *testing.T) {
	const sampleIntervalMs = 250

	// Helper function to calculate series multiplier based on labels
	getSeriesMultiplier := func(labels []prompb.Label) uint64 {
		// Create a simple hash from labels to generate unique values per series
		labelHash := uint64(0)
		for _, label := range labels {
			for _, b := range label.Name + label.Value {
				labelHash = labelHash*31 + uint64(b)
			}
		}
		return labelHash % sampleIntervalMs
	}

	// Helper function to generate a complete time series with samples at 250ms intervals
	// Each series gets different sample values based on a hash of their labels
	generateSeries := func(labels []prompb.Label, startMs, endMs int64) *prompb.TimeSeries {
		seriesMultiplier := getSeriesMultiplier(labels)

		var samples []prompb.Sample
		for ts := startMs; ts <= endMs; ts += sampleIntervalMs {
			samples = append(samples, prompb.Sample{
				Timestamp: ts,
				Value:     float64(ts + int64(seriesMultiplier)), // Unique value per series
			})
		}

		return &prompb.TimeSeries{
			Labels:  labels,
			Samples: samples,
		}
	}

	m := &mockedRemoteClient{
		store: []*prompb.TimeSeries{
			generateSeries([]prompb.Label{{Name: "job", Value: "prometheus"}}, 0, 10000),
			generateSeries([]prompb.Label{{Name: "job", Value: "node_exporter"}}, 0, 10000),
			generateSeries([]prompb.Label{{Name: "job", Value: "cadvisor"}, {Name: "region", Value: "us"}}, 0, 10000),
			generateSeries([]prompb.Label{{Name: "instance", Value: "localhost:9090"}}, 0, 10000),
		},
		b: labels.NewScratchBuilder(0),
	}

	testCases := []struct {
		name            string
		queries         []*prompb.Query
		expectedResults []*prompb.TimeSeries
	}{
		{
			name: "single query",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
			},
			expectedResults: []*prompb.TimeSeries{
				generateSeries([]prompb.Label{{Name: "job", Value: "prometheus"}}, 1000, 2000),
			},
		},
		{
			name: "multiple queries - different matchers",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "node_exporter"},
					},
				},
			},
			expectedResults: []*prompb.TimeSeries{
				generateSeries([]prompb.Label{{Name: "job", Value: "node_exporter"}}, 1500, 2500),
				generateSeries([]prompb.Label{{Name: "job", Value: "prometheus"}}, 1000, 2000),
			},
		},
		{
			name: "multiple queries - overlapping results",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "job", Value: "prometheus|node_exporter"},
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
					},
				},
			},
			expectedResults: []*prompb.TimeSeries{
				generateSeries([]prompb.Label{{Name: "job", Value: "cadvisor"}, {Name: "region", Value: "us"}}, 1500, 2500),
				generateSeries([]prompb.Label{{Name: "job", Value: "node_exporter"}}, 1000, 2000),
				generateSeries([]prompb.Label{{Name: "job", Value: "prometheus"}}, 1000, 2000),
			},
		},
		{
			name: "query with no results",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "nonexistent"},
					},
				},
			},
			expectedResults: nil, // empty result
		},
		{
			name:            "empty query list",
			queries:         []*prompb.Query{},
			expectedResults: nil,
		},
		{
			name: "three queries with mixed results",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "nonexistent"},
					},
				},
				{
					StartTimestampMs: 2000,
					EndTimestampMs:   3000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "instance", Value: "localhost:9090"},
					},
				},
			},
			expectedResults: []*prompb.TimeSeries{
				generateSeries([]prompb.Label{{Name: "instance", Value: "localhost:9090"}}, 2000, 3000),
				generateSeries([]prompb.Label{{Name: "job", Value: "prometheus"}}, 1000, 2000),
			},
		},
		{
			name: "same matchers with overlapping time ranges",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   5000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
					},
				},
				{
					StartTimestampMs: 3000,
					EndTimestampMs:   8000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "region", Value: "us"},
					},
				},
			},
			expectedResults: []*prompb.TimeSeries{
				generateSeries([]prompb.Label{{Name: "job", Value: "cadvisor"}, {Name: "region", Value: "us"}}, 1000, 8000),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m.reset()

			result, err := m.ReadMultiple(context.Background(), tc.queries, true)
			require.NoError(t, err)

			// Verify the queries were stored correctly
			require.Equal(t, tc.queries, m.gotMultiple)

			// Verify the combined result matches expected
			var got []*prompb.TimeSeries
			for result.Next() {
				series := result.At()
				var samples []prompb.Sample
				iterator := series.Iterator(nil)

				// Collect actual samples
				for iterator.Next() != chunkenc.ValNone {
					ts, value := iterator.At()
					samples = append(samples, prompb.Sample{
						Timestamp: ts,
						Value:     value,
					})
				}
				require.NoError(t, iterator.Err())

				got = append(got, &prompb.TimeSeries{
					Labels:  prompb.FromLabels(series.Labels(), nil),
					Samples: samples,
				})
			}
			require.NoError(t, result.Err())

			require.ElementsMatch(t, tc.expectedResults, got)
		})
	}
}

func TestReadMultipleSorting(t *testing.T) {
	// Test data with labels designed to test sorting behavior
	// When sorted: aaa < bbb < ccc
	// When unsorted: order depends on processing order
	m := &mockedRemoteClient{
		store: []*prompb.TimeSeries{
			{Labels: []prompb.Label{{Name: "series", Value: "ccc"}}}, // Will be returned by query 1
			{Labels: []prompb.Label{{Name: "series", Value: "aaa"}}}, // Will be returned by query 2
			{Labels: []prompb.Label{{Name: "series", Value: "bbb"}}}, // Will be returned by both queries (overlapping)
		},
		b: labels.NewScratchBuilder(0),
	}

	testCases := []struct {
		name          string
		queries       []*prompb.Query
		sortSeries    bool
		expectedOrder []string
	}{
		{
			name: "multiple queries with sortSeries=true - should be sorted",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "ccc|bbb"}, // Returns: ccc, bbb (unsorted in store)
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "aaa|bbb"}, // Returns: aaa, bbb (unsorted in store)
					},
				},
			},
			sortSeries:    true,
			expectedOrder: []string{"aaa", "bbb", "ccc"}, // Should be sorted after merge
		},
		{
			name: "multiple queries with sortSeries=false - concatenates without deduplication",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   2000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "ccc|bbb"}, // Returns: ccc, bbb (unsorted)
					},
				},
				{
					StartTimestampMs: 1500,
					EndTimestampMs:   2500,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "series", Value: "aaa|bbb"}, // Returns: aaa, bbb (unsorted in store)
					},
				},
			},
			sortSeries:    false,
			expectedOrder: []string{"ccc", "bbb", "aaa", "bbb"}, // Concatenated results - duplicates included
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m.reset()

			result, err := m.ReadMultiple(context.Background(), tc.queries, tc.sortSeries)
			require.NoError(t, err)

			// Collect the actual results
			var actualOrder []string
			for result.Next() {
				series := result.At()
				seriesValue := series.Labels().Get("series")
				actualOrder = append(actualOrder, seriesValue)
			}
			require.NoError(t, result.Err())

			// Verify the expected order matches actual order
			// For sortSeries=true: results should be in sorted order
			// For sortSeries=false: results should be in concatenated order (with duplicates)
			testutil.RequireEqual(t, tc.expectedOrder, actualOrder)
		})
	}
}

func TestReadMultipleWithChunks(t *testing.T) {
	tests := []struct {
		name                 string
		queries              []*prompb.Query
		responseType         string
		mockHandler          func(*testing.T, []*prompb.Query) http.HandlerFunc
		expectedSeriesCount  int
		validateSampleCounts []int // expected samples per series
	}{
		{
			name: "multiple queries with chunked responses",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   5000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
				{
					StartTimestampMs: 6000,
					EndTimestampMs:   10000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "node_exporter"},
					},
				},
			},
			responseType:         "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse",
			mockHandler:          createChunkedResponseHandler,
			expectedSeriesCount:  6, // 3 chunks per query (2 queries * 3 series per query)
			validateSampleCounts: []int{4, 5, 1, 4, 5, 1},
		},
		{
			name: "sampled response multiple queries",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   3000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "prometheus"},
					},
				},
				{
					StartTimestampMs: 4000,
					EndTimestampMs:   6000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "node_exporter"},
					},
				},
			},
			responseType:         "application/x-protobuf",
			mockHandler:          createSampledResponseHandler,
			expectedSeriesCount:  4, // 2 series per query * 2 queries
			validateSampleCounts: []int{2, 2, 2, 2},
		},
		{
			name: "single query with multiple chunks",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 0,
					EndTimestampMs:   15000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
					},
				},
			},
			responseType:         "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse",
			mockHandler:          createChunkedResponseHandler,
			expectedSeriesCount:  3,
			validateSampleCounts: []int{5, 5, 5},
		},
		{
			name: "overlapping series from multiple queries",
			queries: []*prompb.Query{
				{
					StartTimestampMs: 1000,
					EndTimestampMs:   5000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
					},
				},
				{
					StartTimestampMs: 3000,
					EndTimestampMs:   7000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
					},
				},
			},
			responseType:         "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse",
			mockHandler:          createOverlappingSeriesHandler,
			expectedSeriesCount:  2,           // Each query creates a separate series entry
			validateSampleCounts: []int{4, 4}, // Actual samples returned by handler
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.mockHandler(t, tc.queries))
			defer server.Close()

			u, err := url.Parse(server.URL)
			require.NoError(t, err)

			cfg := &ClientConfig{
				URL:              &config_util.URL{URL: u},
				Timeout:          model.Duration(5 * time.Second),
				ChunkedReadLimit: config.DefaultChunkedReadLimit,
			}

			client, err := NewReadClient("test", cfg)
			require.NoError(t, err)

			// Test ReadMultiple
			result, err := client.ReadMultiple(context.Background(), tc.queries, false)

			require.NoError(t, err)

			// Collect all series and validate
			var allSeries []storage.Series
			var totalSamples int
			for result.Next() {
				series := result.At()
				allSeries = append(allSeries, series)

				// Verify we have some labels
				require.Positive(t, series.Labels().Len())

				// Count samples in this series
				it := series.Iterator(nil)
				var sampleCount int
				for it.Next() != chunkenc.ValNone {
					sampleCount++
				}
				require.NoError(t, it.Err())
				totalSamples += sampleCount

				require.Equalf(t, tc.validateSampleCounts[len(allSeries)-1], sampleCount, "Series %d sample count mismatch", len(allSeries))
			}
			require.NoError(t, result.Err())

			// Validate total counts
			require.Len(t, allSeries, tc.expectedSeriesCount, "Series count mismatch")
		})
	}
}

// createChunkedResponseHandler creates a mock handler for chunked responses.
func createChunkedResponseHandler(t *testing.T, queries []*prompb.Query) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse")

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		cw := NewChunkedWriter(w, flusher)

		// For each query, simulate multiple chunks
		for queryIndex := range queries {
			chunks := buildTestChunks(t) // Creates 3 chunks with 5 samples each
			for chunkIndex, chunk := range chunks {
				// Create unique labels for each series in each query
				var labels []prompb.Label
				if queryIndex == 0 {
					labels = []prompb.Label{
						{Name: "job", Value: "prometheus"},
						{Name: "instance", Value: fmt.Sprintf("localhost:%d", 9090+chunkIndex)},
					}
				} else {
					labels = []prompb.Label{
						{Name: "job", Value: "node_exporter"},
						{Name: "instance", Value: fmt.Sprintf("localhost:%d", 9100+chunkIndex)},
					}
				}

				cSeries := prompb.ChunkedSeries{
					Labels: labels,
					Chunks: []prompb.Chunk{chunk},
				}

				readResp := prompb.ChunkedReadResponse{
					ChunkedSeries: []*prompb.ChunkedSeries{&cSeries},
					QueryIndex:    int64(queryIndex),
				}

				b, err := proto.Marshal(&readResp)
				require.NoError(t, err)

				_, err = cw.Write(b)
				require.NoError(t, err)
			}
		}
	})
}

// createSampledResponseHandler creates a mock handler for sampled responses.
func createSampledResponseHandler(t *testing.T, queries []*prompb.Query) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")

		var results []*prompb.QueryResult
		for queryIndex, query := range queries {
			var timeseries []*prompb.TimeSeries

			// Create 2 series per query
			for seriesIndex := range 2 {
				var labels []prompb.Label
				if queryIndex == 0 {
					labels = []prompb.Label{
						{Name: "job", Value: "prometheus"},
						{Name: "instance", Value: fmt.Sprintf("localhost:%d", 9090+seriesIndex)},
					}
				} else {
					labels = []prompb.Label{
						{Name: "job", Value: "node_exporter"},
						{Name: "instance", Value: fmt.Sprintf("localhost:%d", 9100+seriesIndex)},
					}
				}

				// Create 2 samples per series within query time range
				samples := []prompb.Sample{
					{Timestamp: query.StartTimestampMs, Value: float64(queryIndex*10 + seriesIndex)},
					{Timestamp: query.EndTimestampMs, Value: float64(queryIndex*10 + seriesIndex + 1)},
				}

				timeseries = append(timeseries, &prompb.TimeSeries{
					Labels:  labels,
					Samples: samples,
				})
			}

			results = append(results, &prompb.QueryResult{Timeseries: timeseries})
		}

		resp := &prompb.ReadResponse{Results: results}
		data, err := proto.Marshal(resp)
		require.NoError(t, err)

		compressed := snappy.Encode(nil, data)
		_, err = w.Write(compressed)
		require.NoError(t, err)
	})
}

// createOverlappingSeriesHandler creates responses with same series from multiple queries.
func createOverlappingSeriesHandler(t *testing.T, queries []*prompb.Query) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse")

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		cw := NewChunkedWriter(w, flusher)

		// Same series labels for both queries (will be merged)
		commonLabels := []prompb.Label{
			{Name: "__name__", Value: "up"},
			{Name: "job", Value: "prometheus"},
		}

		// Send response for each query with the same series
		for queryIndex := range queries {
			chunk := buildTestChunks(t)[0] // Use first chunk with 5 samples

			cSeries := prompb.ChunkedSeries{
				Labels: commonLabels,
				Chunks: []prompb.Chunk{chunk},
			}

			readResp := prompb.ChunkedReadResponse{
				ChunkedSeries: []*prompb.ChunkedSeries{&cSeries},
				QueryIndex:    int64(queryIndex),
			}

			b, err := proto.Marshal(&readResp)
			require.NoError(t, err)

			_, err = cw.Write(b)
			require.NoError(t, err)
		}
	})
}
