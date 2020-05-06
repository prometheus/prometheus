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
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	// On linux with a global proxy the tests will fail as the go client(http,grpc) tries to connect through the proxy.
	os.Setenv("no_proxy", "localhost,127.0.0.1,0.0.0.0,:")
	os.Exit(m.Run())
}

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

	for _, test := range tests {
		inURL, err := url.Parse(test.inURL)

		testutil.Ok(t, err)

		globalURL := tmplFuncs("", opts)["globalURL"].(func(u *url.URL) *url.URL)
		outURL := globalURL(inURL)

		testutil.Equals(t, test.outURL, outURL.String())
	}
}

type dbAdapter struct {
	*tsdb.DB
}

func (a *dbAdapter) Stats(statsByLabelName string) (*tsdb.Stats, error) {
	return a.Head().Stats(statsByLabelName), nil
}

func TestReadyAndHealthy(t *testing.T) {
	t.Parallel()

	dbDir, err := ioutil.TempDir("", "tsdb-ready")
	testutil.Ok(t, err)
	defer testutil.Ok(t, os.RemoveAll(dbDir))

	db, err := tsdb.Open(dbDir, nil, nil, nil)
	testutil.Ok(t, err)

	opts := &Options{
		ListenAddress:  ":9090",
		ReadTimeout:    30 * time.Second,
		MaxConnections: 512,
		Context:        nil,
		Storage:        nil,
		LocalStorage:   &dbAdapter{db},
		TSDBDir:        dbDir,
		QueryEngine:    nil,
		ScrapeManager:  &scrape.Manager{},
		RuleManager:    &rules.Manager{},
		Notifier:       nil,
		RoutePrefix:    "/",
		EnableAdminAPI: true,
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost:9090",
			Path:   "/",
		},
		Version:  &PrometheusVersion{},
		Gatherer: prometheus.DefaultGatherer,
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)

	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}

	go func() {
		err := webHandler.Run(context.Background())
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
	}()

	// TODO(bwplotka): Those tests create tons of new connection and memory that is never cleaned.
	// Close and exhaust all response bodies.

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	resp, err := http.Get("http://localhost:9090/-/healthy")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/-/ready")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/version")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/graph")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Post("http://localhost:9090/api/v2/admin/tsdb/snapshot", "", strings.NewReader(""))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Post("http://localhost:9090/api/v2/admin/tsdb/delete_series", "", strings.NewReader("{}"))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/graph")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/alerts")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/flags")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/rules")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/service-discovery")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/targets")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/config")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/status")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	// Set to ready.
	webHandler.Ready()

	resp, err = http.Get("http://localhost:9090/-/healthy")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/-/ready")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/version")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/graph")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Post("http://localhost:9090/api/v2/admin/tsdb/snapshot", "", strings.NewReader(""))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Post("http://localhost:9090/api/v2/admin/tsdb/delete_series", "", strings.NewReader("{}"))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/alerts")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/flags")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/rules")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/service-discovery")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/targets")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/config")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9090/status")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)
}

func TestRoutePrefix(t *testing.T) {
	t.Parallel()
	dbDir, err := ioutil.TempDir("", "tsdb-ready")
	testutil.Ok(t, err)
	defer testutil.Ok(t, os.RemoveAll(dbDir))

	db, err := tsdb.Open(dbDir, nil, nil, nil)
	testutil.Ok(t, err)

	opts := &Options{
		ListenAddress:  ":9091",
		ReadTimeout:    30 * time.Second,
		MaxConnections: 512,
		Context:        nil,
		TSDBDir:        dbDir,
		LocalStorage:   &dbAdapter{db},
		Storage:        nil,
		QueryEngine:    nil,
		ScrapeManager:  nil,
		RuleManager:    nil,
		Notifier:       nil,
		RoutePrefix:    "/prometheus",
		EnableAdminAPI: true,
		ExternalURL: &url.URL{
			Host:   "localhost.localdomain:9090",
			Scheme: "http",
		},
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)
	go func() {
		err := webHandler.Run(context.Background())
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
	}()

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	resp, err := http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/healthy")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/ready")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/version")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v2/admin/tsdb/snapshot", "", strings.NewReader(""))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v2/admin/tsdb/delete_series", "", strings.NewReader("{}"))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusServiceUnavailable, resp.StatusCode)

	// Set to ready.
	webHandler.Ready()

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/healthy")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/ready")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/version")

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v2/admin/tsdb/snapshot", "", strings.NewReader(""))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v2/admin/tsdb/delete_series", "", strings.NewReader("{}"))

	testutil.Ok(t, err)
	testutil.Equals(t, http.StatusOK, resp.StatusCode)
}

func TestDebugHandler(t *testing.T) {
	for _, tc := range []struct {
		prefix, url string
		code        int
	}{
		{"/", "/debug/pprof/cmdline", 200},
		{"/foo", "/foo/debug/pprof/cmdline", 200},

		{"/", "/debug/pprof/goroutine", 200},
		{"/foo", "/foo/debug/pprof/goroutine", 200},

		{"/", "/debug/pprof/foo", 404},
		{"/foo", "/bar/debug/pprof/goroutine", 404},
	} {
		opts := &Options{
			RoutePrefix:   tc.prefix,
			ListenAddress: "somehost:9090",
			ExternalURL: &url.URL{
				Host:   "localhost.localdomain:9090",
				Scheme: "http",
			},
		}
		handler := New(nil, opts)
		handler.Ready()

		w := httptest.NewRecorder()

		req, err := http.NewRequest("GET", tc.url, nil)

		testutil.Ok(t, err)

		handler.router.ServeHTTP(w, req)

		testutil.Equals(t, tc.code, w.Code)
	}
}

func TestHTTPMetrics(t *testing.T) {
	t.Parallel()
	handler := New(nil, &Options{
		RoutePrefix:   "/",
		ListenAddress: "somehost:9090",
		ExternalURL: &url.URL{
			Host:   "localhost.localdomain:9090",
			Scheme: "http",
		},
	})
	getReady := func() int {
		t.Helper()
		w := httptest.NewRecorder()

		req, err := http.NewRequest("GET", "/-/ready", nil)
		testutil.Ok(t, err)

		handler.router.ServeHTTP(w, req)
		return w.Code
	}

	code := getReady()
	testutil.Equals(t, http.StatusServiceUnavailable, code)
	counter := handler.metrics.requestCounter
	testutil.Equals(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))

	handler.Ready()
	for range [2]int{} {
		code = getReady()
		testutil.Equals(t, http.StatusOK, code)
	}
	testutil.Equals(t, 2, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusOK)))))
	testutil.Equals(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))
}
