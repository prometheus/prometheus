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

package main

import (
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"
)

func TestGenerateBucket(t *testing.T) {
	tcs := []struct {
		min, max         int
		start, end, step int
	}{
		{
			min:   101,
			max:   141,
			start: 100,
			end:   150,
			step:  10,
		},
	}

	for _, tc := range tcs {
		start, end, step := generateBucket(tc.min, tc.max)

		require.Equal(t, tc.start, start)
		require.Equal(t, tc.end, end)
		require.Equal(t, tc.step, step)
	}
}

// getDumpedSamples dumps samples and returns them.
func getDumpedSamples(t *testing.T, path string, mint, maxt int64, match []string, formatter SeriesSetFormatter) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := dumpSamples(
		context.Background(),
		path,
		mint,
		maxt,
		match,
		formatter,
	)
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func normalizeNewLine(b []byte) []byte {
	if strings.Contains(runtime.GOOS, "windows") {
		// We use "/n" while dumping on windows as well.
		return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	}
	return b
}

func TestTSDBDump(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			metric{foo="bar", baz="abc"} 1 2 3 4 5
			heavy_metric{foo="bar"} 5 4 3 2 1
			heavy_metric{foo="foo"} 5 4 3 2 1
	`)

	tests := []struct {
		name         string
		mint         int64
		maxt         int64
		match        []string
		expectedDump string
	}{
		{
			name:         "default match",
			mint:         math.MinInt64,
			maxt:         math.MaxInt64,
			match:        []string{"{__name__=~'(?s:.*)'}"},
			expectedDump: "testdata/dump-test-1.prom",
		},
		{
			name:         "same matcher twice",
			mint:         math.MinInt64,
			maxt:         math.MaxInt64,
			match:        []string{"{foo=~'.+'}", "{foo=~'.+'}"},
			expectedDump: "testdata/dump-test-1.prom",
		},
		{
			name:         "no duplication",
			mint:         math.MinInt64,
			maxt:         math.MaxInt64,
			match:        []string{"{__name__=~'(?s:.*)'}", "{baz='abc'}"},
			expectedDump: "testdata/dump-test-1.prom",
		},
		{
			name:         "well merged",
			mint:         math.MinInt64,
			maxt:         math.MaxInt64,
			match:        []string{"{__name__='heavy_metric'}", "{baz='abc'}"},
			expectedDump: "testdata/dump-test-1.prom",
		},
		{
			name:         "multi matchers",
			mint:         math.MinInt64,
			maxt:         math.MaxInt64,
			match:        []string{"{__name__='heavy_metric',foo='foo'}", "{__name__='metric'}"},
			expectedDump: "testdata/dump-test-2.prom",
		},
		{
			name:         "with reduced mint and maxt",
			mint:         int64(60000),
			maxt:         int64(120000),
			match:        []string{"{__name__='metric'}"},
			expectedDump: "testdata/dump-test-3.prom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dumpedMetrics := getDumpedSamples(t, storage.Dir(), tt.mint, tt.maxt, tt.match, formatSeriesSet)
			expectedMetrics, err := os.ReadFile(tt.expectedDump)
			require.NoError(t, err)
			expectedMetrics = normalizeNewLine(expectedMetrics)
			// even though in case of one matcher samples are not sorted, the order in the cases above should stay the same.
			require.Equal(t, string(expectedMetrics), dumpedMetrics)
		})
	}
}

func TestTSDBDumpOpenMetrics(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			my_counter{foo="bar", baz="abc"} 1 2 3 4 5
			my_gauge{bar="foo", abc="baz"} 9 8 0 4 7
	`)

	expectedMetrics, err := os.ReadFile("testdata/dump-openmetrics-test.prom")
	require.NoError(t, err)
	expectedMetrics = normalizeNewLine(expectedMetrics)
	dumpedMetrics := getDumpedSamples(t, storage.Dir(), math.MinInt64, math.MaxInt64, []string{"{__name__=~'(?s:.*)'}"}, formatSeriesSetOpenMetrics)
	require.Equal(t, string(expectedMetrics), dumpedMetrics)
}

func TestTSDBDumpOpenMetricsRoundTrip(t *testing.T) {
	initialMetrics, err := os.ReadFile("testdata/dump-openmetrics-roundtrip-test.prom")
	require.NoError(t, err)
	initialMetrics = normalizeNewLine(initialMetrics)

	dbDir := t.TempDir()
	// Import samples from OM format
	err = backfill(5000, initialMetrics, dbDir, false, false, 2*time.Hour)
	require.NoError(t, err)
	db, err := tsdb.Open(dbDir, nil, nil, tsdb.DefaultOptions(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Dump the blocks into OM format
	dumpedMetrics := getDumpedSamples(t, dbDir, math.MinInt64, math.MaxInt64, []string{"{__name__=~'(?s:.*)'}"}, formatSeriesSetOpenMetrics)

	// Should get back the initial metrics.
	require.Equal(t, string(initialMetrics), dumpedMetrics)
}
