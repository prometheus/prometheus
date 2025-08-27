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

package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func TestDedupe(t *testing.T) {
	var buf bytes.Buffer
	d := Dedupe(promslog.New(&promslog.Config{Writer: &buf}), 100*time.Millisecond)
	dlog := slog.New(d)
	defer d.Stop()

	// Log 10 times quickly, ensure they are deduped.
	for range 10 {
		dlog.Info("test", "hello", "world")
	}

	// Trim empty lines
	lines := []string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	require.Len(t, lines, 1)

	// Wait, then log again, make sure it is logged.
	time.Sleep(200 * time.Millisecond)
	dlog.Info("test", "hello", "world")
	// Trim empty lines
	lines = []string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	require.Len(t, lines, 2)
}

func TestDedupeConcurrent(t *testing.T) {
	d := Dedupe(promslog.New(&promslog.Config{}), 250*time.Millisecond)
	dlog := slog.New(d)
	defer d.Stop()

	concurrentWriteFunc := func() {
		go func() {
			dlog1 := dlog.With("writer", 1)
			for range 10 {
				dlog1.With("foo", "bar").Info("test", "hello", "world")
			}
		}()

		go func() {
			dlog2 := dlog.With("writer", 2)
			for range 10 {
				dlog2.With("foo", "bar").Info("test", "hello", "world")
			}
		}()
	}

	require.NotPanics(t, func() { concurrentWriteFunc() })
}
