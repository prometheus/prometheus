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
	"os"
	"testing"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

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

	s := NewWriteStorage(nil, dir, defaultFlushDeadline)

	externalLabels := labels.FromStrings("external", "true")
	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			&config.DefaultRemoteWriteConfig,
		},
	}
	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	testutil.Equals(t, labels.Labels(nil), s.queues[0].externalLabels)

	conf.GlobalConfig.ExternalLabels = externalLabels
	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	testutil.Equals(t, externalLabels, s.queues[0].externalLabels)

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
	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	queue := s.queues[0]

	s.ApplyConfig(conf)
	testutil.Equals(t, 1, len(s.queues))
	testutil.Assert(t, queue == s.queues[0], "Queue pointer should have remained the same")

	err = s.Close()
	testutil.Ok(t, err)
}
