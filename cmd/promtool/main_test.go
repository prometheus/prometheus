// Copyright 2018 The Prometheus Authors
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

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestQueryRange(t *testing.T) {
	s, getURL := mockServer(200, `{"status": "success", "data": {"resultType": "matrix", "result": []}}`)
	defer s.Close()

	p := &promqlPrinter{}
	exitCode := QueryRange(s.URL, "up", "0", "300", 0, p)
	expectedPath := "/api/v1/query_range"
	if getURL().Path != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", getURL().Path, expectedPath)
	}
	actual := getURL().Query().Get("query")
	if actual != "up" {
		t.Errorf("unexpected value %s for query", actual)
	}
	actual = getURL().Query().Get("step")
	if actual != "1.000" {
		t.Errorf("unexpected value %s for step", actual)
	}
	if exitCode > 0 {
		t.Error()
	}

	exitCode = QueryRange(s.URL, "up", "0", "300", 10*time.Millisecond, p)
	if getURL().Path != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", getURL().Path, expectedPath)
	}
	actual = getURL().Query().Get("query")
	if actual != "up" {
		t.Errorf("unexpected value %s for query", actual)
	}
	actual = getURL().Query().Get("step")
	if actual != "0.010" {
		t.Errorf("unexpected value %s for step", actual)
	}
	if exitCode > 0 {
		t.Error()
	}
}

func TestQueryLabels(t *testing.T) {
	s, getURL := mockServer(200, `{"status": "success", "data": []}`)
	defer s.Close()

	p := &promqlPrinter{}
	u, _ := url.Parse(s.URL)
	exitCode := QueryLabels(u, "job", p)
	expectedPath := "/api/v1/label/job/values"
	if getURL().Path != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", getURL().Path, expectedPath)
	}
	if exitCode > 0 {
		t.Error()
	}
}

func TestQueryInstant(t *testing.T) {
	s, getURL := mockServer(200, `{"status": "success", "data": {"resultType": "matrix", "result": []}}`)
	defer s.Close()

	p := &promqlPrinter{}
	exitCode := QueryInstant(s.URL, "up", p)
	expectedPath := "/api/v1/query"
	if getURL().Path != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", getURL().Path, expectedPath)
	}
	actual := getURL().Query().Get("query")
	if actual != "up" {
		t.Errorf("unexpected value %s for query", actual)
	}
	if exitCode > 0 {
		t.Error()
	}
}

func TestQuerySeries(t *testing.T) {
	s, getURL := mockServer(200, `{"status": "success", "data": []}`)
	p := &promqlPrinter{}
	u, _ := url.Parse(s.URL)
	exitCode := QuerySeries(u, []string{"up"}, "0", "300", p)
	expectedPath := "/api/v1/series"
	if getURL().Path != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", getURL().Path, expectedPath)
	}
	if exitCode > 0 {
		t.Error()
	}
	actual := getURL().Query().Get("match[]")
	if actual != "up" {
		t.Errorf("unexpected value %s for query", actual)
	}
}

func mockServer(code int, body string) (*httptest.Server, func() *url.URL) {
	var u *url.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u = r.URL
		w.WriteHeader(code)
		fmt.Fprintln(w, body)
	}))

	f := func() *url.URL {
		return u
	}
	return server, f
}
