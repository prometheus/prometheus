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

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestConfig(t *testing.T) {
	c := Config{}
	if c.roundTripper() != DefaultRoundTripper {
		t.Fatalf("expected default roundtripper for nil RoundTripper field")
	}
}

func TestClientURL(t *testing.T) {
	tests := []struct {
		address  string
		endpoint string
		args     map[string]string
		expected string
	}{
		{
			address:  "http://localhost:9090",
			endpoint: "/test",
			expected: "http://localhost:9090/test",
		},
		{
			address:  "http://localhost",
			endpoint: "/test",
			expected: "http://localhost/test",
		},
		{
			address:  "http://localhost:9090",
			endpoint: "test",
			expected: "http://localhost:9090/test",
		},
		{
			address:  "http://localhost:9090/prefix",
			endpoint: "/test",
			expected: "http://localhost:9090/prefix/test",
		},
		{
			address:  "https://localhost:9090/",
			endpoint: "/test/",
			expected: "https://localhost:9090/test",
		},
		{
			address:  "http://localhost:9090",
			endpoint: "/test/:param",
			args: map[string]string{
				"param": "content",
			},
			expected: "http://localhost:9090/test/content",
		},
		{
			address:  "http://localhost:9090",
			endpoint: "/test/:param/more/:param",
			args: map[string]string{
				"param": "content",
			},
			expected: "http://localhost:9090/test/content/more/content",
		},
		{
			address:  "http://localhost:9090",
			endpoint: "/test/:param/more/:foo",
			args: map[string]string{
				"param": "content",
				"foo":   "bar",
			},
			expected: "http://localhost:9090/test/content/more/bar",
		},
		{
			address:  "http://localhost:9090",
			endpoint: "/test/:param",
			args: map[string]string{
				"nonexistent": "content",
			},
			expected: "http://localhost:9090/test/:param",
		},
	}

	for _, test := range tests {
		ep, err := url.Parse(test.address)
		if err != nil {
			t.Fatal(err)
		}

		hclient := &httpClient{
			endpoint: ep,
			client:   http.Client{Transport: DefaultRoundTripper},
		}

		u := hclient.URL(test.endpoint, test.args)
		if u.String() != test.expected {
			t.Errorf("unexpected result: got %s, want %s", u, test.expected)
			continue
		}
	}
}

func TestDoGetFallback(t *testing.T) {
	v := url.Values{"a": []string{"1", "2"}}

	type testResponse struct {
		Values string
		Method string
	}

	// Start a local HTTP server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		r := &testResponse{
			Values: req.Form.Encode(),
			Method: req.Method,
		}

		body, _ := json.Marshal(r)

		if req.Method == http.MethodPost {
			if req.URL.Path == "/blockPost" {
				http.Error(w, string(body), http.StatusMethodNotAllowed)
				return
			}
		}

		w.Write(body)
	}))
	// Close the server when test finishes.
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &httpClient{client: *(server.Client())}

	// Do a post, and ensure that the post succeeds.
	_, b, _, err := DoGetFallback(client, context.TODO(), u, v)
	if err != nil {
		t.Fatalf("Error doing local request: %v", err)
	}
	resp := &testResponse{}
	if err := json.Unmarshal(b, resp); err != nil {
		t.Fatal(err)
	}
	if resp.Method != http.MethodPost {
		t.Fatalf("Mismatch method")
	}
	if resp.Values != v.Encode() {
		t.Fatalf("Mismatch in values")
	}

	// Do a fallbcak to a get.
	u.Path = "/blockPost"
	_, b, _, err = DoGetFallback(client, context.TODO(), u, v)
	if err != nil {
		t.Fatalf("Error doing local request: %v", err)
	}
	if err := json.Unmarshal(b, resp); err != nil {
		t.Fatal(err)
	}
	if resp.Method != http.MethodGet {
		t.Fatalf("Mismatch method")
	}
	if resp.Values != v.Encode() {
		t.Fatalf("Mismatch in values")
	}
}
