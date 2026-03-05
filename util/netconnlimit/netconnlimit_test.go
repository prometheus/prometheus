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
package netconnlimit

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSharedLimitListenerConcurrency(t *testing.T) {
	testCases := []struct {
		name        string
		semCapacity int
		connCount   int
		expected    int // Expected number of connections processed simultaneously.
	}{
		{
			name:        "Single connection allowed",
			semCapacity: 1,
			connCount:   3,
			expected:    1,
		},
		{
			name:        "Two connections allowed",
			semCapacity: 2,
			connCount:   3,
			expected:    2,
		},
		{
			name:        "Three connections allowed",
			semCapacity: 3,
			connCount:   3,
			expected:    3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sem := NewSharedSemaphore(tc.semCapacity)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err, "failed to create listener")
			defer listener.Close()

			limitedListener := SharedLimitListener(listener, sem)

			var wg sync.WaitGroup
			var activeConnCount int64
			var mu sync.Mutex

			wg.Add(tc.connCount)

			// Accept connections.
			for i := 0; i < tc.connCount; i++ {
				go func() {
					defer wg.Done()

					conn, err := limitedListener.Accept()
					require.NoError(t, err, "failed to accept connection")
					defer conn.Close()

					// Simulate work and track the active connection count.
					mu.Lock()
					activeConnCount++
					require.LessOrEqual(t, activeConnCount, int64(tc.expected), "too many simultaneous connections")
					mu.Unlock()

					time.Sleep(100 * time.Millisecond)

					mu.Lock()
					activeConnCount--
					mu.Unlock()
				}()
			}

			// Create clients that attempt to connect to the listener.
			for i := 0; i < tc.connCount; i++ {
				go func() {
					conn, err := net.Dial("tcp", listener.Addr().String())
					require.NoError(t, err, "failed to connect to listener")
					defer conn.Close()
					_, _ = io.WriteString(conn, "hello")
				}()
			}

			wg.Wait()

			// Ensure all connections are released and semaphore is empty.
			require.Empty(t, sem)
		})
	}
}

func TestSharedLimitListenerClose(t *testing.T) {
	sem := NewSharedSemaphore(2)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to create listener")

	limitedListener := SharedLimitListener(listener, sem)

	// Close the listener and ensure it does not accept new connections.
	err = limitedListener.Close()
	require.NoError(t, err, "failed to close listener")

	conn, err := limitedListener.Accept()
	require.Error(t, err, "expected error on accept after listener closed")
	if conn != nil {
		conn.Close()
	}
}
