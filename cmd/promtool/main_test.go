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

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/stretchr/testify/require"
)

func TestQueryRange(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "matrix", "result": []}}`)
	defer s.Close()

	urlObject, err := url.Parse(s.URL)
	require.Equal(t, nil, err)

	p := &promqlPrinter{}
	exitCode := QueryRange(urlObject, map[string]string{}, "up", "0", "300", 0, p)
	require.Equal(t, "/api/v1/query_range", getRequest().URL.Path)
	form := getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "1", form.Get("step"))
	require.Equal(t, 0, exitCode)

	exitCode = QueryRange(urlObject, map[string]string{}, "up", "0", "300", 10*time.Millisecond, p)
	require.Equal(t, "/api/v1/query_range", getRequest().URL.Path)
	form = getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "0.01", form.Get("step"))
	require.Equal(t, 0, exitCode)
}

func TestQueryInstant(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "vector", "result": []}}`)
	defer s.Close()

	urlObject, err := url.Parse(s.URL)
	require.Equal(t, nil, err)

	p := &promqlPrinter{}
	exitCode := QueryInstant(urlObject, "up", "300", p)
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

func TestCheckSDFile(t *testing.T) {
	cases := []struct {
		name string
		file string
		err  string
	}{
		{
			name: "good .yml",
			file: "./testdata/good-sd-file.yml",
		},
		{
			name: "good .yaml",
			file: "./testdata/good-sd-file.yaml",
		},
		{
			name: "good .json",
			file: "./testdata/good-sd-file.json",
		},
		{
			name: "bad file extension",
			file: "./testdata/bad-sd-file-extension.nonexistant",
			err:  "invalid file extension: \".nonexistant\"",
		},
		{
			name: "bad format",
			file: "./testdata/bad-sd-file-format.yml",
			err:  "yaml: unmarshal errors:\n  line 1: field targats not found in type struct { Targets []string \"yaml:\\\"targets\\\"\"; Labels model.LabelSet \"yaml:\\\"labels\\\"\" }",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := checkSDFile(test.file)
			if test.err != "" {
				require.Equalf(t, test.err, err.Error(), "Expected error %q, got %q", test.err, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckDuplicates(t *testing.T) {
	cases := []struct {
		name         string
		ruleFile     string
		expectedDups []compareRuleType
	}{
		{
			name:     "no duplicates",
			ruleFile: "./testdata/rules.yml",
		},
		{
			name:     "duplicate in other group",
			ruleFile: "./testdata/rules_duplicates.yml",
			expectedDups: []compareRuleType{
				{
					metric: "job:test:count_over_time1m",
					label:  labels.New(),
				},
			},
		},
	}

	for _, test := range cases {
		c := test
		t.Run(c.name, func(t *testing.T) {
			rgs, err := rulefmt.ParseFile(c.ruleFile)
			require.Empty(t, err)
			dups := checkDuplicates(rgs.Groups)
			require.Equal(t, c.expectedDups, dups)
		})
	}
}

func BenchmarkCheckDuplicates(b *testing.B) {
	rgs, err := rulefmt.ParseFile("./testdata/rules_large.yml")
	require.Empty(b, err)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		checkDuplicates(rgs.Groups)
	}
}
