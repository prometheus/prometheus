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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb"
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

		require.NoError(t, err)

		globalURL := tmplFuncs("", opts)["globalURL"].(func(u *url.URL) *url.URL)
		outURL := globalURL(inURL)

		require.Equal(t, test.outURL, outURL.String())
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
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(dbDir)) }()

	db, err := tsdb.Open(dbDir, nil, nil, nil)
	require.NoError(t, err)

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
	l, err := webHandler.Listener()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listener: %s", err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		err := webHandler.Run(ctx, l, "")
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
	}()

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	resp, err := http.Get("http://localhost:9090/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	for _, u := range []string{
		"http://localhost:9090/-/ready",
		"http://localhost:9090/classic/graph",
		"http://localhost:9090/classic/flags",
		"http://localhost:9090/classic/rules",
		"http://localhost:9090/classic/service-discovery",
		"http://localhost:9090/classic/targets",
		"http://localhost:9090/classic/status",
		"http://localhost:9090/classic/config",
	} {
		resp, err = http.Get(u)
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		cleanupTestResponse(t, resp)
	}

	resp, err = http.Post("http://localhost:9090/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post("http://localhost:9090/api/v1/admin/tsdb/delete_series", "", strings.NewReader("{}"))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	// Set to ready.
	webHandler.Ready()

	for _, u := range []string{
		"http://localhost:9090/-/healthy",
		"http://localhost:9090/-/ready",
		"http://localhost:9090/classic/graph",
		"http://localhost:9090/classic/flags",
		"http://localhost:9090/classic/rules",
		"http://localhost:9090/classic/service-discovery",
		"http://localhost:9090/classic/targets",
		"http://localhost:9090/classic/status",
		"http://localhost:9090/classic/config",
	} {
		resp, err = http.Get(u)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		cleanupTestResponse(t, resp)
	}

	resp, err = http.Post("http://localhost:9090/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupSnapshot(t, dbDir, resp)
	cleanupTestResponse(t, resp)

	resp, err = http.Post("http://localhost:9090/api/v1/admin/tsdb/delete_series?match[]=up", "", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	cleanupTestResponse(t, resp)
}

func TestRoutePrefix(t *testing.T) {
	t.Parallel()
	dbDir, err := ioutil.TempDir("", "tsdb-ready")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(dbDir)) }()

	db, err := tsdb.Open(dbDir, nil, nil, nil)
	require.NoError(t, err)

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
	l, err := webHandler.Listener()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listener: %s", err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		err := webHandler.Run(ctx, l, "")
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
	}()

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	resp, err := http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/ready")
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v1/admin/tsdb/delete_series", "", strings.NewReader("{}"))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	// Set to ready.
	webHandler.Ready()

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Get("http://localhost:9091" + opts.RoutePrefix + "/-/ready")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupSnapshot(t, dbDir, resp)
	cleanupTestResponse(t, resp)

	resp, err = http.Post("http://localhost:9091"+opts.RoutePrefix+"/api/v1/admin/tsdb/delete_series?match[]=up", "", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	cleanupTestResponse(t, resp)
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

		require.NoError(t, err)

		handler.router.ServeHTTP(w, req)

		require.Equal(t, tc.code, w.Code)
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
		require.NoError(t, err)

		handler.router.ServeHTTP(w, req)
		return w.Code
	}

	code := getReady()
	require.Equal(t, http.StatusServiceUnavailable, code)
	counter := handler.metrics.requestCounter
	require.Equal(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))

	handler.Ready()
	for range [2]int{} {
		code = getReady()
		require.Equal(t, http.StatusOK, code)
	}
	require.Equal(t, 2, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusOK)))))
	require.Equal(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))
}

func TestShutdownWithStaleConnection(t *testing.T) {
	dbDir, err := ioutil.TempDir("", "tsdb-ready")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(dbDir)) }()

	db, err := tsdb.Open(dbDir, nil, nil, nil)
	require.NoError(t, err)

	timeout := 10 * time.Second

	opts := &Options{
		ListenAddress:  ":9090",
		ReadTimeout:    timeout,
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
	l, err := webHandler.Listener()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listener: %s", err))
	}

	closed := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := webHandler.Run(ctx, l, "")
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
		close(closed)
	}()

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	// Open a socket, and don't use it. This connection should then be closed
	// after the ReadTimeout.
	c, err := net.Dial("tcp", "localhost:9090")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	// Stop the web handler.
	cancel()

	select {
	case <-closed:
	case <-time.After(timeout + 5*time.Second):
		t.Fatalf("Server still running after read timeout.")
	}
}

func TestHandleMultipleQuitRequests(t *testing.T) {
	opts := &Options{
		ListenAddress:   ":9090",
		MaxConnections:  512,
		EnableLifecycle: true,
		RoutePrefix:     "/",
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost:9090",
			Path:   "/",
		},
	}
	webHandler := New(nil, opts)
	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}
	l, err := webHandler.Listener()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listener: %s", err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	closed := make(chan struct{})
	go func() {
		err := webHandler.Run(ctx, l, "")
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
		close(closed)
	}()

	// Give some time for the web goroutine to run since we need the server
	// to be up before starting tests.
	time.Sleep(5 * time.Second)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := http.Post("http://localhost:9090/-/quit", "", strings.NewReader(""))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		}()
	}
	close(start)
	wg.Wait()

	// Stop the web handler.
	cancel()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatalf("Server still running after 5 seconds.")
	}
}

func cleanupTestResponse(t *testing.T, resp *http.Response) {
	_, err := io.Copy(ioutil.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func cleanupSnapshot(t *testing.T, dbDir string, resp *http.Response) {
	snapshot := &struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}{}
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, snapshot))
	require.NotZero(t, snapshot.Data.Name, "snapshot directory not returned")
	require.NoError(t, os.Remove(filepath.Join(dbDir, "snapshots", snapshot.Data.Name)))
	require.NoError(t, os.Remove(filepath.Join(dbDir, "snapshots")))
}
