// Copyright 2020 The Prometheus Authors
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
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"
)

func getLogLines(t *testing.T, name string) []string {
	content, err := os.ReadFile(name)
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			lines = append(lines[:i], lines[i+1:]...)
		}
	}
	return lines
}

func TestJSONFileLogger_basic(t *testing.T) {
	f, err := os.CreateTemp("", "logging")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
		require.NoError(t, os.Remove(f.Name()))
	}()

	logHandler, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	require.NotNil(t, logHandler, "logger handler can't be nil")

	logger := slog.New(logHandler)
	logger.Info("test", "hello", "world")

	r := getLogLines(t, f.Name())
	require.Len(t, r, 1, "expected 1 log line")
	result, err := regexp.Match(`^{"time":"[^"]+","level":"INFO","source":"\w+.go:\d+","msg":"test","hello":"world"}`, []byte(r[0]))
	require.NoError(t, err)
	require.True(t, result, "unexpected content: %s", r)

	err = logHandler.Close()
	require.NoError(t, err)

	err = logHandler.file.Close()
	require.Error(t, err)
	require.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")
}

func TestJSONFileLogger_parallel(t *testing.T) {
	f, err := os.CreateTemp("", "logging")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
		require.NoError(t, os.Remove(f.Name()))
	}()

	logHandler, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	require.NotNil(t, logHandler, "logger handler can't be nil")

	logger := slog.New(logHandler)
	logger.Info("test", "hello", "world")

	logHandler2, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	require.NotNil(t, logHandler2, "logger handler can't be nil")

	logger2 := slog.New(logHandler2)
	logger2.Info("test", "hello", "world")

	err = logHandler.Close()
	require.NoError(t, err)

	err = logHandler.file.Close()
	require.Error(t, err)
	require.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")

	err = logHandler2.Close()
	require.NoError(t, err)

	err = logHandler2.file.Close()
	require.Error(t, err)
	require.True(t, strings.HasSuffix(err.Error(), os.ErrClosed.Error()), "file not closed")
}
