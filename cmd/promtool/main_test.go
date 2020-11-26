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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueryRange(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "matrix", "result": []}}`)
	defer s.Close()

	p := &promqlPrinter{}
	exitCode := QueryRange(s.URL, map[string]string{}, "up", "0", "300", 0, p)
	require.Equal(t, "/api/v1/query_range", getRequest().URL.Path)
	form := getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "1", form.Get("step"))
	require.Equal(t, 0, exitCode)

	exitCode = QueryRange(s.URL, map[string]string{}, "up", "0", "300", 10*time.Millisecond, p)
	require.Equal(t, "/api/v1/query_range", getRequest().URL.Path)
	form = getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "0.01", form.Get("step"))
	require.Equal(t, 0, exitCode)
}

func TestQueryInstant(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "vector", "result": []}}`)
	defer s.Close()

	p := &promqlPrinter{}
	exitCode := QueryInstant(s.URL, "up", "300", p)
	require.Equal(t, "/api/v1/query", getRequest().URL.Path)
	form := getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "300", form.Get("time"))
	require.Equal(t, 0, exitCode)
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
