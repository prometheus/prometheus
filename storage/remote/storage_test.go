// Copyright 2019 The Prometheus Authors
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
	"fmt"
	"net/url"
	"sync"
	"testing"

	common_config "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

func TestStorageLifecycle(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
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

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

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

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

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

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

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
	s := NewStorage(nil, nil, nil, t.TempDir(), defaultFlushDeadline, nil, false)

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
