// Copyright 2015 The Prometheus Authors
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

import "testing"

func TestURLJoin(t *testing.T) {

	testCases := []struct {
		inputHost string
		inputPath string
		expected  string
	}{
		{"http://host", "path", "http://host/path"},
		{"http://host", "path/", "http://host/path"},
		{"http://host", "/path", "http://host/path"},
		{"http://host", "/path/", "http://host/path"},

		{"http://host/", "path", "http://host/path"},
		{"http://host/", "path/", "http://host/path"},
		{"http://host/", "/path", "http://host/path"},
		{"http://host/", "/path/", "http://host/path"},

		{"https://host", "path", "https://host/path"},
		{"https://host", "path/", "https://host/path"},
		{"https://host", "/path", "https://host/path"},
		{"https://host", "/path/", "https://host/path"},

		{"https://host/", "path", "https://host/path"},
		{"https://host/", "path/", "https://host/path"},
		{"https://host/", "/path", "https://host/path"},
		{"https://host/", "/path/", "https://host/path"},
	}
	for i, c := range testCases {
		client, err := newPrometheusHTTPClient(c.inputHost)
		if err != nil {
			panic(err)
		}
		actual := client.urlJoin(c.inputPath)
		if actual != c.expected {
			t.Errorf("Error on case %d: %v(actual) != %v(expected)", i, actual, c.expected)
		}
		t.Logf("Case %d: %v(actual) == %v(expected)", i, actual, c.expected)
	}
}
