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

//go:build !windows

package main

import (
	"bytes"
	"io"
	"math"
	"os"
	"path"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb"
)

func TestTSDBDumpOpenMetricsRoundTripPipe(t *testing.T) {
	initialMetrics, err := os.ReadFile("testdata/dump-openmetrics-roundtrip-test.prom")
	require.NoError(t, err)
	initialMetrics = normalizeNewLine(initialMetrics)

	pipeDir := t.TempDir()
	dbDir := t.TempDir()

	// create pipe
	pipe := path.Join(pipeDir, "pipe")
	err = syscall.Mkfifo(pipe, 0o666)
	require.NoError(t, err)

	go func() {
		// open pipe to write
		in, err := os.OpenFile(pipe, os.O_WRONLY, os.ModeNamedPipe)
		require.NoError(t, err)
		defer func() { require.NoError(t, in.Close()) }()
		_, err = io.Copy(in, bytes.NewReader(initialMetrics))
		require.NoError(t, err)
	}()

	// Import samples from OM format
	code := backfillOpenMetrics(pipe, dbDir, false, false, 2*time.Hour, map[string]string{})
	require.Equal(t, 0, code)
	db, err := tsdb.Open(dbDir, nil, nil, tsdb.DefaultOptions(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Dump the blocks into OM format
	dumpedMetrics := getDumpedSamples(t, dbDir, "", math.MinInt64, math.MaxInt64, []string{"{__name__=~'(?s:.*)'}"}, formatSeriesSetOpenMetrics)

	// Should get back the initial metrics.
	require.Equal(t, string(initialMetrics), dumpedMetrics)
}
