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
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

// TestToHash pins the serialisation the remote write and read names are derived
// from. When a config has no explicit name, the first 6 characters of this hash
// become the remote_name label of the prometheus_remote_storage_* metrics and the
// name used in logs (see the `name` field documentation), so a change in the YAML
// serialisation is visible to users and needs a CHANGELOG entry.
//
// This is also why toHash keeps using YAML rather than JSON: several fields of the
// remote write and read configs have a yaml tag but no json tag, so a JSON based
// hash would not tell them apart and distinct configs would be rejected as
// duplicates.
func TestToHash(t *testing.T) {
	u, err := url.Parse("http://localhost:9090/api/v1/write")
	require.NoError(t, err)

	base := config.RemoteWriteConfig{
		URL:             &common_config.URL{URL: u},
		RemoteTimeout:   model.Duration(30 * time.Second),
		ProtobufMessage: "prometheus.WriteRequest",
	}

	withRelabel := base
	withRelabel.WriteRelabelConfigs = []*relabel.Config{{
		SourceLabels: model.LabelNames{"__name__"},
		Separator:    ";",
		Regex:        relabel.MustNewRegexp("foo.*"),
		Replacement:  "$1",
		Action:       relabel.Keep,
	}}

	withNHCB := base
	withNHCB.ConvertNHCBToClassic = true

	withLogging := base
	withLogging.FailedRequestLogging = true

	readBase := config.RemoteReadConfig{
		URL:           &common_config.URL{URL: u},
		RemoteTimeout: model.Duration(time.Minute),
	}
	withMatchers := readBase
	withMatchers.RequiredMatchers = model.LabelSet{"env": "prod"}

	tests := []struct {
		name     string
		cfg      config.RemoteWriteConfig
		wantHash string
		wantName string
	}{
		{
			name:     "minimal config",
			cfg:      base,
			wantHash: "1dfab5ef980ec150f4665a359ea365a2",
			wantName: "1dfab5",
		},
		{
			name:     "config with write_relabel_configs",
			cfg:      withRelabel,
			wantHash: "72a35bac778859714e04e7c28ca5b136",
			wantName: "72a35b",
		},
	}

	hashes := map[string]string{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hash, err := toHash(&test.cfg)
			require.NoError(t, err)
			require.Equal(t, test.wantHash, hash)
			require.Equal(t, test.wantName, hash[:6])
			hashes[hash] = test.name
		})
	}

	// Remote read configs are hashed the same way and their hash also ends up in
	// the remote_name label of the remote read metrics.
	readHash, err := toHash(&readBase)
	require.NoError(t, err)
	require.Equal(t, "2fc477417ccd0d8205d43a44be9fd1b4", readHash)
	matchersHash, err := toHash(&withMatchers)
	require.NoError(t, err)
	require.NotEqual(t, readHash, matchersHash)

	// Configs that only differ in a field without a json tag must not collide,
	// otherwise they would be rejected as duplicate remote write configs.
	for _, cfg := range []config.RemoteWriteConfig{withNHCB, withLogging} {
		hash, err := toHash(&cfg)
		require.NoError(t, err)
		require.NotEqual(t, base, cfg)
		require.NotEqual(t, "1dfab5ef980ec150f4665a359ea365a2", hash)
		require.NotContains(t, hashes, hash)
	}
}

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
