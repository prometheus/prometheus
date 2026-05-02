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
	"fmt"
	"math/rand/v2"
	"net"
	"slices"
	"sync"
	"testing"
)

var (
	mu        sync.Mutex
	usedPorts []int
)

// Port range used by getPort. Chosen to live entirely below Linux's
// default ephemeral allocation range (32768–60999), so a concurrent
// net.Listen(":0") elsewhere on the host will never be assigned a port
// from here. Adjust if the runtime host has a non-default
// /proc/sys/net/ipv4/ip_local_port_range.
const (
	testPortLo = 20000
	testPortHi = 30000
)

// RandomUnprivilegedPort returns a random TCP port number that is currently
// free, drawn from a range outside the kernel's ephemeral allocation range.
//
// Drawing from a non-ephemeral range eliminates the dominant
// "bind: address already in use" race in subprocess tests: the kernel will
// not hand a port from this range to anyone calling net.Listen(":0"), so
// other tests that pass --web.listen-address=0.0.0.0:0 cannot grab the
// port between this helper's Close and the caller's subprocess bind.
func RandomUnprivilegedPort(t *testing.T) int {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()

	port := getPort()
	for portWasUsed(port) {
		port = getPort()
	}
	usedPorts = append(usedPorts, port)
	return port
}

func portWasUsed(port int) bool {
	return slices.Contains(usedPorts, port)
}

func getPort() int {
	for {
		port := testPortLo + rand.IntN(testPortHi-testPortLo)
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		listener.Close()
		return port
	}
}
