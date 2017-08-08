// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package web

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestGlobalURL(t *testing.T) {
	opts := &Options{
		ListenAddress: ":9090",
		ExternalURL: &url.URL{
			Scheme: "https",
			Host:   "externalhost:80",
			Path:   "/path/prefix",
		},
	}

	tests := []struct {
		inURL  string
		outURL string
	}{
		{
			// Nothing should change if the input URL is not on localhost, even if the port is our listening port.
			inURL:  "http://somehost:9090/metrics",
			outURL: "http://somehost:9090/metrics",
		},
		{
			// Port and host should change if target is on localhost and port is our listening port.
			inURL:  "http://localhost:9090/metrics",
			outURL: "https://externalhost:80/metrics",
		},
		{
			// Only the host should change if the port is not our listening port, but the host is localhost.
			inURL:  "http://localhost:8000/metrics",
			outURL: "http://externalhost:8000/metrics",
		},
		{
			// Alternative localhost representations should also work.
			inURL:  "http://127.0.0.1:9090/metrics",
			outURL: "https://externalhost:80/metrics",
		},
	}

	for i, test := range tests {
		inURL, err := url.Parse(test.inURL)
		if err != nil {
			t.Fatalf("%d. Error parsing input URL: %s", i, err)
		}
		globalURL := tmplFuncs("", opts)["globalURL"].(func(u *url.URL) *url.URL)
		outURL := globalURL(inURL)

		if outURL.String() != test.outURL {
			t.Fatalf("%d. got %s, want %s", i, outURL.String(), test.outURL)
		}
	}
}

func TestReadyAndHealthy(t *testing.T) {
	opts := &Options{
		ListenAddress:  ":9090",
		ReadTimeout:    30 * time.Second,
		MaxConnections: 512,
		Context:        nil,
		Storage:        nil,
		QueryEngine:    nil,
		TargetManager:  nil,
		RuleManager:    nil,
		Notifier:       nil,
		RoutePrefix:    "/",
		MetricsPath:    "/metrics/",
	}

	opts.Flags = map[string]string{}

	webHandler := New(opts)
	go webHandler.Run()

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	resp, err := http.Get("http://localhost:9090/-/healthy")
	if err != nil {
		t.Fatalf("Unexpected HTTP error %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Path /-/healthy with server unready test, Expected status 200 got: %s", resp.Status)
	}

	resp, err = http.Get("http://localhost:9090/-/ready")
	if err != nil {
		t.Fatalf("Unexpected HTTP error %s", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Path /-/ready with server unready test, Expected status 503 got: %s", resp.Status)
	}

	resp, err = http.Get("http://localhost:9090/version")
	if err != nil {
		t.Fatalf("Unexpected HTTP error %s", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Path /version with server unready test, Expected status 503 got: %s", resp.Status)
	}

	// Set to ready.
	webHandler.Ready()

	resp, err = http.Get("http://localhost:9090/-/healthy")
	if err != nil {
		t.Fatalf("Unexpected HTTP error %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Path /-/healthy with server ready test, Expected status 200 got: %s", resp.Status)
	}

	resp, err = http.Get("http://localhost:9090/-/ready")
	if err != nil {
		t.Fatalf("Unexpected HTTP error %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Path /-/ready with server ready test, Expected status 200 got: %s", resp.Status)
	}

	resp, err = http.Get("http://localhost:9090/version")
	if err != nil {
		t.Fatalf("Unexpected HTTP error %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Path /version with server ready test, Expected status 200 got: %s", resp.Status)
	}

}
