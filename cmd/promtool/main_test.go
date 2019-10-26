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
	"os"
	"testing"
	"time"
)

func TestQueryRange(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "matrix", "result": []}}`)
	defer s.Close()

	p := &promqlPrinter{}
	exitCode := QueryRange(s.URL, map[string]string{}, "up", "0", "300", 0, p)
	expectedPath := "/api/v1/query_range"
	gotPath := getRequest().URL.Path
	if gotPath != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", gotPath, expectedPath)
	}
	form := getRequest().Form
	actual := form.Get("query")
	if actual != "up" {
		t.Errorf("unexpected value %s for query", actual)
	}
	actual = form.Get("step")
	if actual != "1" {
		t.Errorf("unexpected value %s for step", actual)
	}
	if exitCode > 0 {
		t.Error()
	}

	exitCode = QueryRange(s.URL, map[string]string{}, "up", "0", "300", 10*time.Millisecond, p)
	gotPath = getRequest().URL.Path
	if gotPath != expectedPath {
		t.Errorf("unexpected URL path %s (wanted %s)", gotPath, expectedPath)
	}
	form = getRequest().Form
	actual = form.Get("query")
	if actual != "up" {
		t.Errorf("unexpected value %s for query", actual)
	}
	actual = form.Get("step")
	if actual != "0.01" {
		t.Errorf("unexpected value %s for step", actual)
	}
	if exitCode > 0 {
		t.Error()
	}
}

func mockServer(code int, body string) (*httptest.Server, func() *http.Request) {
	var req *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		req = r
		w.WriteHeader(code)
		fmt.Fprintln(w, body)
	}))

	f := func() *http.Request {
		return req
	}
	return server, f
}

func TestCheckExpression(t *testing.T) {
	// Temporarily move error output from CheckExpression calls
	// to /dev/null, only return value is relevant
	stderr := os.Stderr
	defer func() { os.Stderr = stderr }()
	os.Stderr = os.NewFile(0, os.DevNull)

	if CheckExpression("test{query=\"hello\"}") != 0 {
		t.Error()
	}

	if CheckExpression("i{am=\"incomplete\"") == 0 {
		t.Error()
	}
}
