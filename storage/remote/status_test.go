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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
)

func TestRemoteWriteStatusNoQueues(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	status := s.RemoteWriteStatus()
	require.Empty(t, status.Queues)
}

func TestRemoteWriteStatusWithQueues(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(nil, prometheus.NewRegistry(), nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			baseRemoteWriteConfig("http://test-storage.com"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))

	status := s.RemoteWriteStatus()
	require.Len(t, status.Queues, 1)

	q := status.Queues[0]
	require.NotEmpty(t, q.Name)
	require.Contains(t, q.Endpoint, "test-storage.com")
	require.Equal(t, 0, q.SamplesSent)
	require.Equal(t, 0, q.SamplesFailed)
}

func TestRemoteWriteStatusMultipleQueues(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(nil, prometheus.NewRegistry(), nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			baseRemoteWriteConfig("http://remote-1.com"),
			baseRemoteWriteConfig("http://remote-2.com"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))

	status := s.RemoteWriteStatus()
	require.Len(t, status.Queues, 2)

	endpoints := make(map[string]bool)
	for _, q := range status.Queues {
		endpoints[q.Endpoint] = true
	}
	require.True(t, endpoints["http://remote-1.com"])
	require.True(t, endpoints["http://remote-2.com"])
}

func TestReadGauge(t *testing.T) {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_gauge",
	})
	g.Set(42.5)
	require.Equal(t, 42.5, readGauge(g))
}

func TestReadCounter(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
	})
	c.Add(100)
	require.Equal(t, 100.0, readCounter(c))
}
