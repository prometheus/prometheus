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

package httputil

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/stretchr/testify/require"
)

var (
	mux    *http.ServeMux
	server *httptest.Server
)

func setup() func() {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	return func() {
		server.Close()
	}
}

func getCompressionHandlerFunc() CompressionHandler {
	hf := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello World!"))
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

	expected := "Hello World!"
	actual := string(contents)
	require.Equal(t, expected, actual, "expected response with content")
}

func BenchmarkNewCompressionHandler_MaliciousAcceptEncoding(b *testing.B) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	req.Header.Set("Accept-Encoding", strings.Repeat(",", http.DefaultMaxHeaderBytes))
	b.ReportAllocs()

	for b.Loop() {
		newCompressedResponseWriter(rec, req)
	}
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

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/foo_endpoint", nil)
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
	expected := "Hello World!"
	require.Equal(t, expected, actual, "unexpected response content")
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

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/foo_endpoint", nil)
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
	expected := "Hello World!"
	require.Equal(t, expected, actual, "expected response with content")
}
