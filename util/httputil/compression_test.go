// Copyright 2016 The Prometheus Authors
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

package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

var (
	mux      *http.ServeMux
	server   *httptest.Server
	respBody = strings.Repeat("Hello World!", 500)
)

func setup() func() {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	return func() {
		server.CloseClientConnections()
		server.Close()
	}
}

func getCompressionHandlerFunc() CompressionHandler {
	hf := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(respBody))
	}
	return CompressionHandler{
		Handler: http.HandlerFunc(hf),
	}
}

func TestCompressionHandler_PlainText(t *testing.T) {
	tearDown := setup()
	defer tearDown()

	ch := getCompressionHandlerFunc()
	mux.Handle("/foo_endpoint", ch)

	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	resp, err := client.Get(server.URL + "/foo_endpoint")
	require.NoError(t, err, "client get failed with unexpected error")
	defer resp.Body.Close()
	contents, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "unexpected error while creating the response body reader")

	actual := string(contents)
	require.Equal(t, respBody, actual, "expected response with content")
}

func TestCompressionHandler_Gzip(t *testing.T) {
	tearDown := setup()
	defer tearDown()

	ch := getCompressionHandlerFunc()
	mux.Handle("/foo_endpoint", ch)

	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	req, _ := http.NewRequest("GET", server.URL+"/foo_endpoint", nil)
	req.Header.Set(acceptEncodingHeader, gzipEncoding)

	resp, err := client.Do(req)
	require.NoError(t, err, "client get failed with unexpected error")
	defer resp.Body.Close()

	actualHeader := resp.Header.Get(contentEncodingHeader)
	require.Equal(t, gzipEncoding, actualHeader, "unexpected encoding header in response")

	var buf bytes.Buffer
	zr, err := gzip.NewReader(resp.Body)
	require.NoError(t, err, "unexpected error while creating the response body reader")

	_, err = buf.ReadFrom(zr)
	require.NoError(t, err, "unexpected error while reading the response body")

	actual := buf.String()
	require.Equal(t, respBody, actual, "unexpected response content")
}

func TestCompressionHandler_Deflate(t *testing.T) {
	tearDown := setup()
	defer tearDown()

	ch := getCompressionHandlerFunc()
	mux.Handle("/foo_endpoint", ch)

	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	req, _ := http.NewRequest("GET", server.URL+"/foo_endpoint", nil)
	req.Header.Set(acceptEncodingHeader, deflateEncoding)

	resp, err := client.Do(req)
	require.NoError(t, err, "client get failed with unexpected error")
	defer resp.Body.Close()

	actualHeader := resp.Header.Get(contentEncodingHeader)
	require.Equal(t, deflateEncoding, actualHeader, "expected response with encoding header")

	var buf bytes.Buffer
	dr, err := zlib.NewReader(resp.Body)
	require.NoError(t, err, "unexpected error while creating the response body reader")

	_, err = buf.ReadFrom(dr)
	require.NoError(t, err, "unexpected error while reading the response body")

	actual := buf.String()
	require.Equal(t, respBody, actual, "expected response with content")
}

func Benchmark_compression(b *testing.B) {
	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	cases := map[string]struct {
		enc            string
		numberOfLabels int
	}{
		"gzip-10-labels": {
			enc:            gzipEncoding,
			numberOfLabels: 10,
		},
		"gzip-100-labels": {
			enc:            gzipEncoding,
			numberOfLabels: 100,
		},
		"gzip-1K-labels": {
			enc:            gzipEncoding,
			numberOfLabels: 1000,
		},
		"gzip-10K-labels": {
			enc:            gzipEncoding,
			numberOfLabels: 10000,
		},
		"gzip-100K-labels": {
			enc:            gzipEncoding,
			numberOfLabels: 100000,
		},
		"gzip-1M-labels": {
			enc:            gzipEncoding,
			numberOfLabels: 1000000,
		},
	}

	for name, tc := range cases {
		b.Run(name, func(b *testing.B) {
			tearDown := setup()
			defer tearDown()
			labels := labels.ScratchBuilder{}

			for i := 0; i < tc.numberOfLabels; i++ {
				labels.Add(fmt.Sprintf("Name%v", i), fmt.Sprintf("Value%v", i))
			}

			respBody, err := json.Marshal(labels.Labels())
			require.NoError(b, err)

			hf := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write(respBody)
			}
			h := CompressionHandler{
				Handler: http.HandlerFunc(hf),
			}

			mux.Handle("/foo_endpoint", h)

			req, _ := http.NewRequest("GET", server.URL+"/foo_endpoint", nil)
			req.Header.Set(acceptEncodingHeader, tc.enc)

			b.ReportAllocs()
			b.ResetTimer()

			// Reusing the array to read the body and avoid allocation on the test
			encRespBody := make([]byte, len(respBody))

			for i := 0; i < b.N; i++ {
				resp, err := client.Do(req)

				require.NoError(b, err)

				require.NoError(b, err, "client get failed with unexpected error")
				responseBodySize := 0
				for {
					n, err := resp.Body.Read(encRespBody)
					responseBodySize += n
					if err == io.EOF {
						break
					}
				}

				b.ReportMetric(float64(responseBodySize), "ContentLength")
				resp.Body.Close()
			}

			client.CloseIdleConnections()
		})
	}
}
