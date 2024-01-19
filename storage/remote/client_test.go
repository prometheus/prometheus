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
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
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
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		err = c.Store(context.Background(), []byte{}, 0)
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
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			err = c.Store(context.Background(), []byte{}, 0)
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

func TestClientHeaders(t *testing.T) {
	headersToSend := map[string]string{"Foo": "Bar", "Baz": "qux"}

	var called bool
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	err = c.Store(context.Background(), []byte{}, 0)
	require.NoError(t, err)

	require.True(t, called, "The remote server wasn't called")
}

func TestReadClient(t *testing.T) {
	tests := []struct {
		name                  string
		query                 *prompb.Query
		httpHandler           http.HandlerFunc
		expectedLabels        []map[string]string
		expectedSamples       [][]model.SamplePair
		expectedErrorContains string
		sortSeries            bool
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
			httpHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			httpHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "foobar")
			}),
			expectedErrorContains: "unsupported content type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.httpHandler)
			defer server.Close()

			u, err := url.Parse(server.URL)
			require.NoError(t, err)

			conf := &ClientConfig{
				URL:              &config_util.URL{URL: u},
				Timeout:          model.Duration(5 * time.Second),
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
	return func(w http.ResponseWriter, r *http.Request) {
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
