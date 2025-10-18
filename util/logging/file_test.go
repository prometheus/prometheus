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
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/querylog"
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

func TestJSONFileLogger_ReadQueryLogs(t *testing.T) {
	f, err := os.CreateTemp("", "querylog")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
		require.NoError(t, os.Remove(f.Name()))
	}()

	// Write valid query log entries
	validLog := `{"params":{"query":"up","start":"123","end":"456","step":1},"stats":{"timings":{"evalTotalTime":0.1,"execQueueTime":0.01,"execTotalTime":0.11,"innerEvalTime":0.05,"queryPreparationTime":0.02,"resultSortTime":0.03},"samples":{"totalQueryableSamples":100,"peakSamples":50}},"ts":"2025-01-01T00:00:00Z"}
`
	_, err = f.WriteString(validLog)
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	logger, err := NewJSONFileLogger(f.Name())
	require.NoError(t, err)
	defer logger.Close()

	// Test successful read
	ctx := context.Background()
	var logs []querylog.QueryLog
	logs, err = logger.ReadQueryLogs(ctx)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "up", logs[0].Params.Query)
	require.Equal(t, "123", logs[0].Params.Start)
	require.Equal(t, uint64(100), logs[0].Stats.Samples.TotalQueryableSamples)
}
