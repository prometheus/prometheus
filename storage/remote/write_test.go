// Copyright 2017 The Prometheus Authors
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
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"

	common_config "github.com/prometheus/common/config"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

var cfg = config.RemoteWriteConfig{
	Name: "dev",
	URL: &config_util.URL{
		URL: &url.URL{
			Scheme: "http",
			Host:   "localhost",
		},
	},
	QueueConfig: config.DefaultQueueConfig,
}

func TestNoDuplicateWriteConfigs(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestNoDuplicateWriteConfigs")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	// Test that configs that are identical in all but name are considered identical.
	cfg1 := config.RemoteWriteConfig{
		Name: "write-1",
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
	}
	cfg2 := config.RemoteWriteConfig{
		Name: "write-2",
		URL: &config_util.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
	}

	s := NewWriteStorage(nil, dir, defaultFlushDeadline)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&cfg1,
			&cfg2,
		},
	}
	testutil.NotOk(t, s.ApplyConfig(conf))

	err = s.Close()
	testutil.Ok(t, err)
}

func TestRestartOnNameChange(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestRestartOnNameChange")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	hash, err := cfg.ToHash()
	testutil.Ok(t, err)

	s := NewWriteStorage(nil, dir, time.Millisecond)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&cfg,
		},
	}
	testutil.Ok(t, s.ApplyConfig(conf))
	testutil.Equals(t, s.queues[hash].client.Name(), cfg.Name)

	// Change the queues name, ensure the queue has been restarted.
	conf.RemoteWriteConfigs[0].Name = "dev-2"
	testutil.Ok(t, s.ApplyConfig(conf))
	testutil.Equals(t, s.queues[hash].client.Name(), cfg.Name)

	err = s.Close()
	testutil.Ok(t, err)
}

func TestWriteStorageLifecycle(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestWriteStorageLifecycle")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	s := NewWriteStorage(nil, dir, defaultFlushDeadline)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&config.DefaultRemoteWriteConfig,
		},
	}
	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))

	err = s.Close()
	testutil.Ok(t, err)
}

func TestUpdateExternalLabels(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestUpdateExternalLabels")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	s := NewWriteStorage(nil, dir, time.Second)

	externalLabels := labels.FromStrings("external", "true")
	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&cfg,
		},
	}
	hash, err := conf.RemoteWriteConfigs[0].ToHash()
	testutil.Ok(t, err)
	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	testutil.Equals(t, labels.Labels(nil), s.queues[hash].externalLabels)

	conf.GlobalConfig.ExternalLabels = externalLabels
	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	testutil.Equals(t, externalLabels, s.queues[hash].externalLabels)

	err = s.Close()
	testutil.Ok(t, err)
}

func TestWriteStorageApplyConfigsIdempotent(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestWriteStorageApplyConfigsIdempotent")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	s := NewWriteStorage(nil, dir, defaultFlushDeadline)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&config.DefaultRemoteWriteConfig,
		},
	}
	// We need to set URL's so that metric creation doesn't panic.
	conf.RemoteWriteConfigs[0].URL = &common_config.URL{
		URL: &url.URL{
			Host: "http://test-storage.com",
		},
	}
	hash, err := toHash(conf.RemoteWriteConfigs[0])
	testutil.Ok(t, err)

	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))

	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	_, hashExists := s.queues[hash]
	testutil.Assert(t, hashExists, "Queue pointer should have remained the same")

	err = s.Close()
	testutil.Ok(t, err)
}

func TestWriteStorageApplyConfigsPartialUpdate(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestWriteStorageApplyConfigsPartialUpdate")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	s := NewWriteStorage(nil, dir, defaultFlushDeadline)

	c0 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(10 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
	}
	c1 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(20 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
	}
	c2 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(30 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
	}

	conf := &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c0, c1, c2},
	}
	// We need to set URL's so that metric creation doesn't panic.
	conf.RemoteWriteConfigs[0].URL = &common_config.URL{
		URL: &url.URL{
			Host: "http://test-storage.com",
		},
	}
	conf.RemoteWriteConfigs[1].URL = &common_config.URL{
		URL: &url.URL{
			Host: "http://test-storage.com",
		},
	}
	conf.RemoteWriteConfigs[2].URL = &common_config.URL{
		URL: &url.URL{
			Host: "http://test-storage.com",
		},
	}
	s.ApplyConfig(conf)
	testutil.Equals(t, 3, len(s.queues))

	c0Hash, err := toHash(c0)
	testutil.Ok(t, err)
	c1Hash, err := toHash(c1)
	testutil.Ok(t, err)
	// q := s.queues[1]

	// Update c0 and c2.
	c0.RemoteTimeout = model.Duration(40 * time.Second)
	c2.RemoteTimeout = model.Duration(50 * time.Second)
	conf = &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c0, c1, c2},
	}
	s.ApplyConfig(conf)
	testutil.Equals(t, 3, len(s.queues))

	_, hashExists := s.queues[c1Hash]
	testutil.Assert(t, hashExists, "Pointer of unchanged queue should have remained the same")

	// Delete c0.
	conf = &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c1, c2},
	}
	s.ApplyConfig(conf)
	testutil.Equals(t, 2, len(s.queues))

	_, hashExists = s.queues[c0Hash]
	testutil.Assert(t, !hashExists, "If the index changed, the queue should be stopped and recreated.")

	err = s.Close()
	testutil.Ok(t, err)
}
