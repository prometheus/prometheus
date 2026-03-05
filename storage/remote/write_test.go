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
	"errors"
	"net/url"
	"testing"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

func testRemoteWriteConfig() *config.RemoteWriteConfig {
	return &config.RemoteWriteConfig{
		Name: "dev",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
}

func TestWriteStorageApplyConfig_NoDuplicateWriteConfigs(t *testing.T) {
	dir := t.TempDir()

	cfg1 := config.RemoteWriteConfig{
		Name: "write-1",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
	cfg2 := config.RemoteWriteConfig{
		Name: "write-2",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
	cfg3 := config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}

	for _, tc := range []struct {
		cfgs        []*config.RemoteWriteConfig
		expectedErr error
	}{
		{ // Two duplicates, we should get an error.
			cfgs:        []*config.RemoteWriteConfig{&cfg1, &cfg1},
			expectedErr: errors.New("duplicate remote write configs are not allowed, found duplicate for URL: http://localhost"),
		},
		{ // Duplicates but with different names, we should not get an error.
			cfgs: []*config.RemoteWriteConfig{&cfg1, &cfg2},
		},
		{ // Duplicates but one with no name, we should not get an error.
			cfgs: []*config.RemoteWriteConfig{&cfg1, &cfg3},
		},
		{ // Duplicates both with no name, we should get an error.
			cfgs:        []*config.RemoteWriteConfig{&cfg3, &cfg3},
			expectedErr: errors.New("duplicate remote write configs are not allowed, found duplicate for URL: http://localhost"),
		},
	} {
		t.Run("", func(t *testing.T) {
			s := NewWriteStorage(nil, nil, dir, time.Millisecond, nil, false)
			conf := &config.Config{
				GlobalConfig:       config.DefaultGlobalConfig,
				RemoteWriteConfigs: tc.cfgs,
			}
			err := s.ApplyConfig(conf)
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, tc.expectedErr, err)
			}

			require.NoError(t, s.Close())
		})
	}
}

func TestWriteStorageApplyConfig_RestartOnNameChange(t *testing.T) {
	dir := t.TempDir()

	cfg := testRemoteWriteConfig()

	hash, err := toHash(cfg)
	require.NoError(t, err)

	s := NewWriteStorage(nil, nil, dir, time.Millisecond, nil, false)

	conf := &config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, s.queues[hash].client().Name(), cfg.Name)

	// Change the queues name, ensure the queue has been restarted.
	conf.RemoteWriteConfigs[0].Name = "dev-2"
	require.NoError(t, s.ApplyConfig(conf))
	hash, err = toHash(cfg)
	require.NoError(t, err)
	require.Equal(t, s.queues[hash].client().Name(), conf.RemoteWriteConfigs[0].Name)

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_UpdateWithRegisterer(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, prometheus.NewRegistry(), dir, time.Millisecond, nil, false)
	c1 := &config.RemoteWriteConfig{
		Name: "named",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
	c2 := &config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
	conf := &config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c1, c2},
	}
	require.NoError(t, s.ApplyConfig(conf))

	c1.QueueConfig.MaxShards = 10
	c2.QueueConfig.MaxShards = 10
	require.NoError(t, s.ApplyConfig(conf))
	for _, queue := range s.queues {
		require.Equal(t, 10, queue.cfg.MaxShards)
	}

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_Lifecycle(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			baseRemoteWriteConfig("http://test-storage.com"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_UpdateExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, prometheus.NewRegistry(), dir, time.Second, nil, false)

	externalLabels := labels.FromStrings("external", "true")
	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			testRemoteWriteConfig(),
		},
	}
	hash, err := toHash(conf.RemoteWriteConfigs[0])
	require.NoError(t, err)
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)
	require.Empty(t, s.queues[hash].externalLabels)

	conf.GlobalConfig.ExternalLabels = externalLabels
	hash, err = toHash(conf.RemoteWriteConfigs[0])
	require.NoError(t, err)
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)
	require.Equal(t, []labels.Label{{Name: "external", Value: "true"}}, s.queues[hash].externalLabels)

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_Idempotent(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false)
	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			baseRemoteWriteConfig("http://test-storage.com"),
		},
	}
	hash, err := toHash(conf.RemoteWriteConfigs[0])
	require.NoError(t, err)

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)
	_, hashExists := s.queues[hash]
	require.True(t, hashExists, "Queue pointer should have remained the same")

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_PartialUpdate(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false)

	c0 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(10 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
		WriteRelabelConfigs: []*relabel.Config{
			{
				Regex:                relabel.MustNewRegexp(".+"),
				NameValidationScheme: model.UTF8Validation,
			},
		},
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
	c1 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(20 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
		HTTPClientConfig: common_config.HTTPClientConfig{
			BearerToken: "foo",
		},
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
	c2 := &config.RemoteWriteConfig{
		RemoteTimeout:   model.Duration(30 * time.Second),
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}

	conf := &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c0, c1, c2},
	}
	// We need to set URL's so that metric creation doesn't panic.
	for i := range conf.RemoteWriteConfigs {
		conf.RemoteWriteConfigs[i].URL = &common_config.URL{
			URL: mustURLParse("http://test-storage.com"),
		}
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 3)

	hashes := make([]string, len(conf.RemoteWriteConfigs))
	queues := make([]*QueueManager, len(conf.RemoteWriteConfigs))
	storeHashes := func() {
		for i := range conf.RemoteWriteConfigs {
			hash, err := toHash(conf.RemoteWriteConfigs[i])
			require.NoError(t, err)
			hashes[i] = hash
			queues[i] = s.queues[hash]
		}
	}

	storeHashes()
	// Update c0 and c2.
	c0.WriteRelabelConfigs[0] = &relabel.Config{
		Regex:                relabel.MustNewRegexp("foo"),
		NameValidationScheme: model.UTF8Validation,
	}
	c2.RemoteTimeout = model.Duration(50 * time.Second)
	conf = &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c0, c1, c2},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 3)

	_, hashExists := s.queues[hashes[0]]
	require.False(t, hashExists, "The queue for the first remote write configuration should have been restarted because the relabel configuration has changed.")
	q, hashExists := s.queues[hashes[1]]
	require.True(t, hashExists, "Hash of unchanged queue should have remained the same")
	require.Equal(t, q, queues[1], "Pointer of unchanged queue should have remained the same")
	_, hashExists = s.queues[hashes[2]]
	require.False(t, hashExists, "The queue for the third remote write configuration should have been restarted because the timeout has changed.")

	storeHashes()
	secondClient := s.queues[hashes[1]].client()
	// Update c1.
	c1.HTTPClientConfig.BearerToken = "bar"
	err := s.ApplyConfig(conf)
	require.NoError(t, err)
	require.Len(t, s.queues, 3)

	_, hashExists = s.queues[hashes[0]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")
	q, hashExists = s.queues[hashes[1]]
	require.True(t, hashExists, "Hash of queue with secret change should have remained the same")
	require.NotEqual(t, secondClient, q.client(), "Pointer of a client with a secret change should not be the same")
	_, hashExists = s.queues[hashes[2]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")

	storeHashes()
	// Delete c0.
	conf = &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c1, c2},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 2)

	_, hashExists = s.queues[hashes[0]]
	require.False(t, hashExists, "If a config is removed, the queue should be stopped and recreated.")
	_, hashExists = s.queues[hashes[1]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")
	_, hashExists = s.queues[hashes[2]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")

	require.NoError(t, s.Close())
}

func TestWriteStorage_CanRegisterMetricsAfterClosing(t *testing.T) {
	dir := t.TempDir()
	reg := prometheus.NewPedanticRegistry()

	s := NewWriteStorage(nil, reg, dir, time.Millisecond, nil, false)
	require.NoError(t, s.Close())
	require.NotPanics(t, func() { NewWriteStorage(nil, reg, dir, time.Millisecond, nil, false) })
}
