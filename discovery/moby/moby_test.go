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

package moby

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// The refresh interval doubles as the request timeout, so it is kept well
// below the deadlines a refresh is measured against.
const (
	refreshInterval    = 100 * time.Millisecond
	maxRefreshDuration = 2 * time.Second
	hangDeadline       = 10 * time.Second
)

// refresher is implemented by both Docker and Docker Swarm discovery.
type refresher interface {
	refresh(ctx context.Context) ([]*targetgroup.Group, error)
}

// TestRefreshTimesOutOnUnresponsiveDaemon asserts that a Docker daemon which
// accepts the connection but never answers cannot block discovery forever. The
// http case also guards the option ordering the timeout relies on.
func TestRefreshTimesOutOnUnresponsiveDaemon(t *testing.T) {
	for _, host := range []struct {
		name string
		host func(t *testing.T) string
	}{
		{
			name: "unix",
			host: func(t *testing.T) string {
				if runtime.GOOS == "windows" {
					t.Skip("Docker is not reached over a unix socket on Windows.")
				}
				// The socket path must stay under the 104 byte cap on unix
				// addresses, so it is not derived from this test's name.
				dir, err := os.MkdirTemp("", "moby")
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })

				return "unix://" + unresponsiveDaemon(t, "unix", filepath.Join(dir, "d.sock"))
			},
		},
		{
			name: "http",
			host: func(t *testing.T) string {
				return "http://" + unresponsiveDaemon(t, "tcp", "127.0.0.1:0")
			},
		},
	} {
		for _, sd := range []struct {
			name         string
			newDiscovery func(t *testing.T, host string) refresher
		}{
			{
				name: "docker",
				newDiscovery: func(t *testing.T, host string) refresher {
					var cfg DockerSDConfig
					require.NoError(t, yaml.Unmarshal(fmt.Appendf(nil, `
---
host: %s
refresh_interval: %s
`, host, refreshInterval), &cfg))

					d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
						Logger:  promslog.NewNopLogger(),
						Metrics: newDiscovererMetrics(t, &cfg),
					})
					require.NoError(t, err)
					return d
				},
			},
			{
				name: "dockerswarm",
				newDiscovery: func(t *testing.T, host string) refresher {
					var cfg DockerSwarmSDConfig
					require.NoError(t, yaml.Unmarshal(fmt.Appendf(nil, `
---
host: %s
role: nodes
refresh_interval: %s
`, host, refreshInterval), &cfg))

					d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
						Logger:  promslog.NewNopLogger(),
						Metrics: newDiscovererMetrics(t, &cfg),
					})
					require.NoError(t, err)
					return d
				},
			},
		} {
			t.Run(sd.name+"/"+host.name, func(t *testing.T) {
				d := sd.newDiscovery(t, host.host(t))

				// The context deliberately carries no deadline, so that a
				// refresh returning at all proves the client bounded it.
				errCh := make(chan error, 1)
				start := time.Now()
				go func() {
					_, err := d.refresh(context.Background())
					errCh <- err
				}()

				select {
				case err := <-errCh:
					require.Error(t, err)
					var netErr net.Error
					require.ErrorAs(t, err, &netErr)
					require.True(t, netErr.Timeout(), "expected a timeout error, got: %s", err)
					require.Less(t, time.Since(start), maxRefreshDuration,
						"refresh returned, but too late for the timeout to still be derived from refresh_interval")
				case <-time.After(hangDeadline):
					t.Fatal("refresh did not return: the Docker client has no request timeout on this transport")
				}
			})
		}
	}
}

// unresponsiveDaemon accepts connections on the given address without ever
// writing to them, simulating a wedged Docker daemon, and returns the address
// it listens on. Connections are held open until the test ends, so that a
// client without a request timeout blocks rather than seeing the socket close.
func unresponsiveDaemon(t *testing.T, network, address string) string {
	t.Helper()

	l, err := net.Listen(network, address)
	require.NoError(t, err)

	var (
		mtx    sync.Mutex
		conns  []net.Conn
		closed bool
	)
	t.Cleanup(func() {
		require.NoError(t, l.Close())
		mtx.Lock()
		defer mtx.Unlock()
		closed = true
		for _, c := range conns {
			c.Close()
		}
	})

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mtx.Lock()
			// Accepting races with the cleanup, which must not leave a
			// connection behind.
			if closed {
				c.Close()
			} else {
				conns = append(conns, c)
			}
			mtx.Unlock()
		}
	}()

	if network == "unix" {
		return address
	}
	return l.Addr().String()
}

func newDiscovererMetrics(t *testing.T, cfg discovery.Config) discovery.DiscovererMetrics {
	t.Helper()

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	t.Cleanup(metrics.Unregister)
	t.Cleanup(refreshMetrics.Unregister)

	return metrics
}
