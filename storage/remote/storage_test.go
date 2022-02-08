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
	"net/url"
	"testing"

	common_config "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

func TestStorageLifecycle(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&config.DefaultRemoteWriteConfig,
		},
		RemoteReadConfigs: []*config.RemoteReadConfig{
			&config.DefaultRemoteReadConfig,
		},
	}
	// We need to set URL's so that metric creation doesn't panic.
	conf.RemoteWriteConfigs[0].URL = &common_config.URL{
		URL: &url.URL{
			Host: "http://test-storage.com",
		},
	}
	conf.RemoteReadConfigs[0].URL = &common_config.URL{
		URL: &url.URL{
			Host: "http://test-storage.com",
		},
	}

	require.NoError(t, s.ApplyConfig(conf))

	// make sure remote write has a queue.
	require.Equal(t, 1, len(s.rws.queues))

	// make sure remote write has a queue.
	require.Equal(t, 1, len(s.queryables))

	err := s.Close()
	require.NoError(t, err)
}

func TestUpdateRemoteReadConfigs(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, 0, len(s.queryables))

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		&config.DefaultRemoteReadConfig,
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, 1, len(s.queryables))

	err := s.Close()
	require.NoError(t, err)
}

func TestFilterExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ExternalLabels: labels.Labels{labels.Label{Name: "foo", Value: "bar"}},
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, 0, len(s.queryables))

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		&config.DefaultRemoteReadConfig,
	}

	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, 1, len(s.queryables))
	require.Equal(t, 1, len(s.queryables[0].(*sampleAndChunkQueryableClient).externalLabels))

	err := s.Close()
	require.NoError(t, err)
}

func TestIgnoreExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ExternalLabels: labels.Labels{labels.Label{Name: "foo", Value: "bar"}},
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, 0, len(s.queryables))

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		&config.DefaultRemoteReadConfig,
	}

	conf.RemoteReadConfigs[0].FilterExternalLabels = false

	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, 1, len(s.queryables))
	require.Equal(t, 0, len(s.queryables[0].(*sampleAndChunkQueryableClient).externalLabels))

	err := s.Close()
	require.NoError(t, err)
}
