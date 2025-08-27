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
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	// On linux with a global proxy the tests will fail as the go client(http,grpc) tries to connect through the proxy.
	os.Setenv("no_proxy", "localhost,127.0.0.1,0.0.0.0,:")
	os.Exit(m.Run())
}

type dbAdapter struct {
	*tsdb.DB
}

func (a *dbAdapter) BlockMetas() ([]tsdb.BlockMeta, error) {
	return a.DB.BlockMetas(), nil
}

func (a *dbAdapter) Stats(statsByLabelName string, limit int) (*tsdb.Stats, error) {
	return a.Head().Stats(statsByLabelName, limit), nil
}

func (*dbAdapter) WALReplayStatus() (tsdb.WALReplayStatus, error) {
	return tsdb.WALReplayStatus{}, nil
}

func TestReadyAndHealthy(t *testing.T) {
	t.Parallel()

	dbDir := t.TempDir()

	db, err := tsdb.Open(dbDir, nil, nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	opts := &Options{
		ListenAddresses: []string{port},
		ReadTimeout:     30 * time.Second,
		MaxConnections:  512,
		Context:         nil,
		Storage:         nil,
		LocalStorage:    &dbAdapter{db},
		TSDBDir:         dbDir,
		QueryEngine:     nil,
		ScrapeManager:   &scrape.Manager{},
		RuleManager:     &rules.Manager{},
		Notifier:        nil,
		RoutePrefix:     "/",
		EnableAdminAPI:  true,
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost" + port,
			Path:   "/",
		},
		Version:  &PrometheusVersion{},
		Gatherer: prometheus.DefaultGatherer,
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)

	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}
	l, err := webHandler.Listeners()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listeners: %s", err))
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

	baseURL := "http://localhost" + port

	resp, err := http.Get(baseURL + "/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Head(baseURL + "/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	for _, u := range []string{
		baseURL + "/-/ready",
	} {
		resp, err = http.Get(u)
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		cleanupTestResponse(t, resp)

		resp, err = http.Head(u)
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		cleanupTestResponse(t, resp)
	}

	resp, err = http.Post(baseURL+"/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post(baseURL+"/api/v1/admin/tsdb/delete_series", "", strings.NewReader("{}"))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	// Set to ready.
	webHandler.SetReady(Ready)

	for _, u := range []string{
		baseURL + "/-/healthy",
		baseURL + "/-/ready",
	} {
		resp, err = http.Get(u)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		cleanupTestResponse(t, resp)

		resp, err = http.Head(u)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		cleanupTestResponse(t, resp)
	}

	resp, err = http.Post(baseURL+"/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupSnapshot(t, dbDir, resp)
	cleanupTestResponse(t, resp)

	resp, err = http.Post(baseURL+"/api/v1/admin/tsdb/delete_series?match[]=up", "", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	cleanupTestResponse(t, resp)
}

func TestRoutePrefix(t *testing.T) {
	t.Parallel()
	dbDir := t.TempDir()

	db, err := tsdb.Open(dbDir, nil, nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	opts := &Options{
		ListenAddresses: []string{port},
		ReadTimeout:     30 * time.Second,
		MaxConnections:  512,
		Context:         nil,
		TSDBDir:         dbDir,
		LocalStorage:    &dbAdapter{db},
		Storage:         nil,
		QueryEngine:     nil,
		ScrapeManager:   nil,
		RuleManager:     nil,
		Notifier:        nil,
		RoutePrefix:     "/prometheus",
		EnableAdminAPI:  true,
		ExternalURL: &url.URL{
			Host:   "localhost.localdomain" + port,
			Scheme: "http",
		},
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)
	l, err := webHandler.Listeners()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listeners: %s", err))
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

	baseURL := "http://localhost" + port

	resp, err := http.Get(baseURL + opts.RoutePrefix + "/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Get(baseURL + opts.RoutePrefix + "/-/ready")
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post(baseURL+opts.RoutePrefix+"/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post(baseURL+opts.RoutePrefix+"/api/v1/admin/tsdb/delete_series", "", strings.NewReader("{}"))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	cleanupTestResponse(t, resp)

	// Set to ready.
	webHandler.SetReady(Ready)

	resp, err = http.Get(baseURL + opts.RoutePrefix + "/-/healthy")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Get(baseURL + opts.RoutePrefix + "/-/ready")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupTestResponse(t, resp)

	resp, err = http.Post(baseURL+opts.RoutePrefix+"/api/v1/admin/tsdb/snapshot", "", strings.NewReader(""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cleanupSnapshot(t, dbDir, resp)
	cleanupTestResponse(t, resp)

	resp, err = http.Post(baseURL+opts.RoutePrefix+"/api/v1/admin/tsdb/delete_series?match[]=up", "", nil)
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
			RoutePrefix:     tc.prefix,
			ListenAddresses: []string{"somehost:9090"},
			ExternalURL: &url.URL{
				Host:   "localhost.localdomain:9090",
				Scheme: "http",
			},
		}
		handler := New(nil, opts)
		handler.SetReady(Ready)

		w := httptest.NewRecorder()

		req, err := http.NewRequest(http.MethodGet, tc.url, nil)

		require.NoError(t, err)

		handler.router.ServeHTTP(w, req)

		require.Equal(t, tc.code, w.Code)
	}
}

func TestHTTPMetrics(t *testing.T) {
	t.Parallel()
	handler := New(nil, &Options{
		RoutePrefix:     "/",
		ListenAddresses: []string{"somehost:9090"},
		ExternalURL: &url.URL{
			Host:   "localhost.localdomain:9090",
			Scheme: "http",
		},
	})
	getReady := func() int {
		t.Helper()
		w := httptest.NewRecorder()

		req, err := http.NewRequest(http.MethodGet, "/-/ready", nil)
		require.NoError(t, err)

		handler.router.ServeHTTP(w, req)
		return w.Code
	}

	code := getReady()
	require.Equal(t, http.StatusServiceUnavailable, code)
	ready := handler.metrics.readyStatus
	require.Equal(t, 0, int(prom_testutil.ToFloat64(ready)))
	counter := handler.metrics.requestCounter
	require.Equal(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))

	handler.SetReady(Ready)
	for range [2]int{} {
		code = getReady()
		require.Equal(t, http.StatusOK, code)
	}
	require.Equal(t, 1, int(prom_testutil.ToFloat64(ready)))
	require.Equal(t, 2, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusOK)))))
	require.Equal(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))

	handler.SetReady(NotReady)
	for range [2]int{} {
		code = getReady()
		require.Equal(t, http.StatusServiceUnavailable, code)
	}
	require.Equal(t, 0, int(prom_testutil.ToFloat64(ready)))
	require.Equal(t, 2, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusOK)))))
	require.Equal(t, 3, int(prom_testutil.ToFloat64(counter.WithLabelValues("/-/ready", strconv.Itoa(http.StatusServiceUnavailable)))))
}

func TestShutdownWithStaleConnection(t *testing.T) {
	dbDir := t.TempDir()

	db, err := tsdb.Open(dbDir, nil, nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	timeout := 10 * time.Second

	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	opts := &Options{
		ListenAddresses: []string{port},
		ReadTimeout:     timeout,
		MaxConnections:  512,
		Context:         nil,
		Storage:         nil,
		LocalStorage:    &dbAdapter{db},
		TSDBDir:         dbDir,
		QueryEngine:     nil,
		ScrapeManager:   &scrape.Manager{},
		RuleManager:     &rules.Manager{},
		Notifier:        nil,
		RoutePrefix:     "/",
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost" + port,
			Path:   "/",
		},
		Version:  &PrometheusVersion{},
		Gatherer: prometheus.DefaultGatherer,
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)

	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}
	l, err := webHandler.Listeners()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listeners: %s", err))
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
	c, err := net.Dial("tcp", opts.ExternalURL.Host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	// Stop the web handler.
	cancel()

	select {
	case <-closed:
	case <-time.After(timeout + 5*time.Second):
		require.FailNow(t, "Server still running after read timeout.")
	}
}

func TestHandleMultipleQuitRequests(t *testing.T) {
	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	opts := &Options{
		ListenAddresses: []string{port},
		MaxConnections:  512,
		EnableLifecycle: true,
		RoutePrefix:     "/",
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost" + port,
			Path:   "/",
		},
	}
	webHandler := New(nil, opts)
	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}
	l, err := webHandler.Listeners()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listeners: %s", err))
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

	baseURL := opts.ExternalURL.Scheme + "://" + opts.ExternalURL.Host

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := http.Post(baseURL+"/-/quit", "", strings.NewReader(""))
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
		require.FailNow(t, "Server still running after 5 seconds.")
	}
}

// Test for availability of API endpoints in Prometheus Agent mode.
func TestAgentAPIEndPoints(t *testing.T) {
	t.Parallel()

	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	opts := &Options{
		ListenAddresses: []string{port},
		ReadTimeout:     30 * time.Second,
		MaxConnections:  512,
		Context:         nil,
		Storage:         nil,
		QueryEngine:     nil,
		ScrapeManager:   &scrape.Manager{},
		RuleManager:     &rules.Manager{},
		Notifier:        nil,
		RoutePrefix:     "/",
		EnableAdminAPI:  true,
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost" + port,
			Path:   "/",
		},
		Version:  &PrometheusVersion{},
		Gatherer: prometheus.DefaultGatherer,
		IsAgent:  true,
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)
	webHandler.SetReady(Ready)
	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}
	l, err := webHandler.Listeners()
	if err != nil {
		panic(fmt.Sprintf("Unable to start web listeners: %s", err))
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
	baseURL := "http://localhost" + port + "/api/v1"

	// Test for non-available endpoints in the Agent mode.
	for path, methods := range map[string][]string{
		"/labels":                      {http.MethodGet, http.MethodPost},
		"/label/:name/values":          {http.MethodGet},
		"/series":                      {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/alertmanagers":               {http.MethodGet},
		"/query":                       {http.MethodGet, http.MethodPost},
		"/query_range":                 {http.MethodGet, http.MethodPost},
		"/query_exemplars":             {http.MethodGet, http.MethodPost},
		"/status/tsdb":                 {http.MethodGet},
		"/status/tsdb/blocks":          {http.MethodGet},
		"/alerts":                      {http.MethodGet},
		"/rules":                       {http.MethodGet},
		"/admin/tsdb/delete_series":    {http.MethodPost, http.MethodPut},
		"/admin/tsdb/clean_tombstones": {http.MethodPost, http.MethodPut},
		"/admin/tsdb/snapshot":         {http.MethodPost, http.MethodPut},
	} {
		for _, m := range methods {
			req, err := http.NewRequest(m, baseURL+path, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
			t.Cleanup(func() {
				require.NoError(t, resp.Body.Close())
			})
		}
	}

	// Test for some of available endpoints in the Agent mode.
	for path, methods := range map[string][]string{
		"/targets":            {http.MethodGet},
		"/targets/metadata":   {http.MethodGet},
		"/metadata":           {http.MethodGet},
		"/status/config":      {http.MethodGet},
		"/status/runtimeinfo": {http.MethodGet},
		"/status/flags":       {http.MethodGet},
	} {
		for _, m := range methods {
			req, err := http.NewRequest(m, baseURL+path, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			t.Cleanup(func() {
				require.NoError(t, resp.Body.Close())
			})
		}
	}
}

func cleanupTestResponse(t *testing.T, resp *http.Response) {
	_, err := io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func cleanupSnapshot(t *testing.T, dbDir string, resp *http.Response) {
	snapshot := &struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}{}
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, snapshot))
	require.NotEmpty(t, snapshot.Data.Name, "snapshot directory not returned")
	require.NoError(t, os.Remove(filepath.Join(dbDir, "snapshots", snapshot.Data.Name)))
	require.NoError(t, os.Remove(filepath.Join(dbDir, "snapshots")))
}

func TestMultipleListenAddresses(t *testing.T) {
	t.Parallel()

	dbDir := t.TempDir()

	db, err := tsdb.Open(dbDir, nil, nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Create multiple ports for testing multiple ListenAddresses
	port1 := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))
	port2 := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	opts := &Options{
		ListenAddresses: []string{port1, port2},
		ReadTimeout:     30 * time.Second,
		MaxConnections:  512,
		Context:         nil,
		Storage:         nil,
		LocalStorage:    &dbAdapter{db},
		TSDBDir:         dbDir,
		QueryEngine:     nil,
		ScrapeManager:   &scrape.Manager{},
		RuleManager:     &rules.Manager{},
		Notifier:        nil,
		RoutePrefix:     "/",
		EnableAdminAPI:  true,
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   "localhost" + port1,
			Path:   "/",
		},
		Version:  &PrometheusVersion{},
		Gatherer: prometheus.DefaultGatherer,
	}

	opts.Flags = map[string]string{}

	webHandler := New(nil, opts)

	webHandler.config = &config.Config{}
	webHandler.notifier = &notifier.Manager{}
	l, err := webHandler.Listeners()
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

	// Set to ready.
	webHandler.SetReady(Ready)

	for _, port := range []string{port1, port2} {
		baseURL := "http://localhost" + port

		resp, err := http.Get(baseURL + "/-/healthy")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		cleanupTestResponse(t, resp)

		resp, err = http.Get(baseURL + "/-/ready")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		cleanupTestResponse(t, resp)
	}
}
