// Copyright 2021 The Prometheus Authors
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
	"testing"
)

// RandomUnprivilegedPort returns valid unprivileged random port number which can be used for testing.
func RandomUnprivilegedPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Listening on random port: %v", err)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("Closing listener: %v", err)
	}

	return listener.Addr().(*net.TCPAddr).Port
}
