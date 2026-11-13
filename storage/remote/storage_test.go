// Copyright The Prometheus Authors
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

package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	common_config "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestStorageLifecycle(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false, false, false)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			// We need to set URL's so that metric creation doesn't panic.
			baseRemoteWriteConfig("http://test-storage.com"),
		},
		RemoteReadConfigs: []*config.RemoteReadConfig{
			baseRemoteReadConfig("http://test-storage.com"),
		},
	}

	require.NoError(t, s.ApplyConfig(conf))

	// make sure remote write has a queue.
	require.Len(t, s.rws.queues, 1)

	// make sure remote write has a queue.
	require.Len(t, s.queryables, 1)

	err := s.Close()
	require.NoError(t, err)
}

func TestUpdateRemoteReadConfigs(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false, false, false)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Empty(t, s.queryables)

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		baseRemoteReadConfig("http://test-storage.com"),
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queryables, 1)

	err := s.Close()
	require.NoError(t, err)
}

func TestFilterExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false, false, false)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ExternalLabels: labels.FromStrings("foo", "bar"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Empty(t, s.queryables)

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		baseRemoteReadConfig("http://test-storage.com"),
	}

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queryables, 1)
	require.Equal(t, 1, s.queryables[0].(*sampleAndChunkQueryableClient).externalLabels.Len())

	err := s.Close()
	require.NoError(t, err)
}

func TestIgnoreExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false, false, false)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ExternalLabels: labels.FromStrings("foo", "bar"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Empty(t, s.queryables)

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		baseRemoteReadConfig("http://test-storage.com"),
	}

	conf.RemoteReadConfigs[0].FilterExternalLabels = false

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queryables, 1)
	require.Equal(t, 0, s.queryables[0].(*sampleAndChunkQueryableClient).externalLabels.Len())

	err := s.Close()
	require.NoError(t, err)
}

func TestRequiredRemoteRead(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "remote read is down", http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)

	for _, required := range []bool{false, true} {
		t.Run(fmt.Sprintf("required=%t", required), func(t *testing.T) {
			s := NewStorage(nil, nil, nil, t.TempDir(), defaultFlushDeadline, nil, false, false, false)
			t.Cleanup(func() { require.NoError(t, s.Close()) })

			rrConf := baseRemoteReadConfig(failing.URL)
			rrConf.ReadRecent = true
			rrConf.Required = required
			require.NoError(t, s.ApplyConfig(&config.Config{
				GlobalConfig:      config.DefaultGlobalConfig,
				RemoteReadConfigs: []*config.RemoteReadConfig{rrConf},
			}))

			req, bestEffort, err := s.SplitQueriers(0, 1000)
			require.NoError(t, err)
			if required {
				require.Len(t, req, 1)
				require.Empty(t, bestEffort)
			} else {
				require.Empty(t, req)
				require.Len(t, bestEffort, 1)
			}
			for _, q := range append(req, bestEffort...) {
				require.NoError(t, q.Close())
			}

			fanout := storage.NewFanout(nil, teststorage.New(t), s)
			q, err := fanout.Querier(0, 1000)
			require.NoError(t, err)
			defer q.Close()

			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "up"))
			for ss.Next() {
				ss.At()
			}
			if required {
				require.ErrorContains(t, ss.Err(), "remote read is down")
			} else {
				require.NoError(t, ss.Err())
				require.NotEmpty(t, ss.Warnings())
			}
		})
	}
}

// mustURLParse parses a URL and panics on error.
func mustURLParse(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(fmt.Sprintf("failed to parse URL %q: %v", rawURL, err))
	}
	return u
}

// baseRemoteWriteConfig copy values from global Default Write config
// to avoid change global state and cross impact test execution.
func baseRemoteWriteConfig(host string) *config.RemoteWriteConfig {
	cfg := config.DefaultRemoteWriteConfig
	cfg.URL = &common_config.URL{
		URL: mustURLParse(host),
	}
	return &cfg
}

// baseRemoteReadConfig copy values from global Default Read config
// to avoid change global state and cross impact test execution.
func baseRemoteReadConfig(host string) *config.RemoteReadConfig {
	cfg := config.DefaultRemoteReadConfig
	cfg.URL = &common_config.URL{
		URL: mustURLParse(host),
	}
	return &cfg
}

// TestWriteStorageApplyConfigsDuringCommit helps detecting races when
// ApplyConfig runs concurrently with Notify
// See https://github.com/prometheus/prometheus/issues/12747
func TestWriteStorageApplyConfigsDuringCommit(t *testing.T) {
	s := NewStorage(nil, nil, nil, t.TempDir(), defaultFlushDeadline, nil, false, false, false)

	var wg sync.WaitGroup
	wg.Add(2000)

	start := make(chan struct{})
	for i := range 1000 {
		go func(i int) {
			<-start
			conf := &config.Config{
				GlobalConfig: config.DefaultGlobalConfig,
				RemoteWriteConfigs: []*config.RemoteWriteConfig{
					baseRemoteWriteConfig(fmt.Sprintf("http://test-%d.com", i)),
				},
			}
			require.NoError(t, s.ApplyConfig(conf))
			wg.Done()
		}(i)
	}

	for range 1000 {
		go func() {
			<-start
			s.Notify()
			wg.Done()
		}()
	}

	close(start)
	wg.Wait()
}
