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

package testutil

import (
	"net"
	"slices"
	"sync"
	"testing"
)

var (
	mu        sync.Mutex
	usedPorts []int
)

// RandomUnprivilegedPort returns valid unprivileged random port number which can be used for testing.
func RandomUnprivilegedPort(t *testing.T) int {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()

	port, err := getPort()
	if err != nil {
		t.Fatal(err)
	}

	for portWasUsed(port) {
		port, err = getPort()
		if err != nil {
			t.Fatal(err)
		}
	}

	usedPorts = append(usedPorts, port)

	return port
}

func portWasUsed(port int) bool {
	return slices.Contains(usedPorts, port)
}

func getPort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}

	if err := listener.Close(); err != nil {
		return 0, err
	}

	return listener.Addr().(*net.TCPAddr).Port, nil
}
