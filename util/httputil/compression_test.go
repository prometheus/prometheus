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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
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
	hf := func(w http.ResponseWriter, r *http.Request) {
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

	if err != nil {
		t.Error("client get failed with unexpected error")
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		t.Errorf("unexpected error while reading the response body: %s", err.Error())
	}

	expected := "Hello World!"
	actual := string(contents)
	if expected != actual {
		t.Errorf("expected response with content %s, but got %s", expected, actual)
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

	req, _ := http.NewRequest("GET", server.URL+"/foo_endpoint", nil)
	req.Header.Set(acceptEncodingHeader, gzipEncoding)

	resp, err := client.Do(req)
	if err != nil {
		t.Error("client get failed with unexpected error")
	}
	defer resp.Body.Close()

	if err != nil {
		t.Errorf("unexpected error while reading the response body: %s", err.Error())
	}

	actualHeader := resp.Header.Get(contentEncodingHeader)

	if actualHeader != gzipEncoding {
		t.Errorf("expected response with encoding header %s, but got %s", gzipEncoding, actualHeader)
	}

	var buf bytes.Buffer
	zr, _ := gzip.NewReader(resp.Body)

	_, err = buf.ReadFrom(zr)
	if err != nil {
		t.Error("unexpected error while reading from response body")
	}

	actual := buf.String()
	expected := "Hello World!"
	if expected != actual {
		t.Errorf("expected response with content %s, but got %s", expected, actual)
	}
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
	if err != nil {
		t.Error("client get failed with unexpected error")
	}
	defer resp.Body.Close()

	if err != nil {
		t.Errorf("unexpected error while reading the response body: %s", err.Error())
	}

	actualHeader := resp.Header.Get(contentEncodingHeader)

	if actualHeader != deflateEncoding {
		t.Errorf("expected response with encoding header %s, but got %s", deflateEncoding, actualHeader)
	}

	var buf bytes.Buffer
	dr, err := zlib.NewReader(resp.Body)
	if err != nil {
		t.Error("unexpected error while reading from response body")
	}

	_, err = buf.ReadFrom(dr)
	if err != nil {
		t.Error("unexpected error while reading from response body")
	}

	actual := buf.String()
	expected := "Hello World!"
	if expected != actual {
		t.Errorf("expected response with content %s, but got %s", expected, actual)
	}
}
