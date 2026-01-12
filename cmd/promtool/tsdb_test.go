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

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/tsdb"
)

func TestGenerateBucket(t *testing.T) {
	t.Parallel()
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
func getDumpedSamples(t *testing.T, databasePath, sandboxDirRoot string, mint, maxt int64, match []string, formatter SeriesSetFormatter) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := dumpTSDBData(
		context.Background(),
		databasePath,
		sandboxDirRoot,
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
	storage := promqltest.LoadedStorage(t, `
		load 1m
			metric{foo="bar", baz="abc"} 1 2 3 4 5
			heavy_metric{foo="bar"} 5 4 3 2 1
			heavy_metric{foo="foo"} 5 4 3 2 1
	`)
	t.Cleanup(func() { storage.Close() })

	tests := []struct {
		name           string
		mint           int64
		maxt           int64
		sandboxDirRoot string
		match          []string
		expectedDump   string
		expectedSeries string
	}{
		{
			name:           "default match",
			mint:           math.MinInt64,
			maxt:           math.MaxInt64,
			match:          []string{"{__name__=~'(?s:.*)'}"},
			expectedDump:   "testdata/dump-test-1.prom",
			expectedSeries: "testdata/dump-series-1.prom",
		},
		{
			name:           "default match with sandbox dir root set",
			mint:           math.MinInt64,
			maxt:           math.MaxInt64,
			sandboxDirRoot: t.TempDir(),
			match:          []string{"{__name__=~'(?s:.*)'}"},
			expectedDump:   "testdata/dump-test-1.prom",
			expectedSeries: "testdata/dump-series-1.prom",
		},
		{
			name:           "same matcher twice",
			mint:           math.MinInt64,
			maxt:           math.MaxInt64,
			match:          []string{"{foo=~'.+'}", "{foo=~'.+'}"},
			expectedDump:   "testdata/dump-test-1.prom",
			expectedSeries: "testdata/dump-series-1.prom",
		},
		{
			name:           "no duplication",
			mint:           math.MinInt64,
			maxt:           math.MaxInt64,
			match:          []string{"{__name__=~'(?s:.*)'}", "{baz='abc'}"},
			expectedDump:   "testdata/dump-test-1.prom",
			expectedSeries: "testdata/dump-series-1.prom",
		},
		{
			name:           "well merged",
			mint:           math.MinInt64,
			maxt:           math.MaxInt64,
			match:          []string{"{__name__='heavy_metric'}", "{baz='abc'}"},
			expectedDump:   "testdata/dump-test-1.prom",
			expectedSeries: "testdata/dump-series-1.prom",
		},
		{
			name:           "multi matchers",
			mint:           math.MinInt64,
			maxt:           math.MaxInt64,
			match:          []string{"{__name__='heavy_metric',foo='foo'}", "{__name__='metric'}"},
			expectedDump:   "testdata/dump-test-2.prom",
			expectedSeries: "testdata/dump-series-2.prom",
		},
		{
			name:           "with reduced mint and maxt",
			mint:           int64(60000),
			maxt:           int64(120000),
			match:          []string{"{__name__='metric'}"},
			expectedDump:   "testdata/dump-test-3.prom",
			expectedSeries: "testdata/dump-series-3.prom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dumpedMetrics := getDumpedSamples(t, storage.Dir(), tt.sandboxDirRoot, tt.mint, tt.maxt, tt.match, formatSeriesSet)
			expectedMetrics, err := os.ReadFile(tt.expectedDump)
			require.NoError(t, err)
			expectedMetrics = normalizeNewLine(expectedMetrics)
			// Sort both, because Prometheus does not guarantee the output order.
			require.Equal(t, sortLines(string(expectedMetrics)), sortLines(dumpedMetrics))

			dumpedSeries := getDumpedSamples(t, storage.Dir(), tt.sandboxDirRoot, tt.mint, tt.maxt, tt.match, formatSeriesSetLabelsToJSON)
			expectedSeries, err := os.ReadFile(tt.expectedSeries)
			require.NoError(t, err)
			expectedSeries = normalizeNewLine(expectedSeries)
			require.Equal(t, sortLines(string(expectedSeries)), sortLines(dumpedSeries))
		})
	}
}

func sortLines(buf string) string {
	lines := strings.Split(buf, "\n")
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

func TestTSDBDumpOpenMetrics(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			my_counter{foo="bar", baz="abc"} 1 2 3 4 5
			my_gauge{bar="foo", abc="baz"} 9 8 0 4 7
	`)
	t.Cleanup(func() { storage.Close() })

	tests := []struct {
		name           string
		sandboxDirRoot string
	}{
		{
			name: "default match",
		},
		{
			name:           "default match with sandbox dir root set",
			sandboxDirRoot: t.TempDir(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedMetrics, err := os.ReadFile("testdata/dump-openmetrics-test.prom")
			require.NoError(t, err)
			expectedMetrics = normalizeNewLine(expectedMetrics)
			dumpedMetrics := getDumpedSamples(t, storage.Dir(), tt.sandboxDirRoot, math.MinInt64, math.MaxInt64, []string{"{__name__=~'(?s:.*)'}"}, formatSeriesSetOpenMetrics)
			require.Equal(t, sortLines(string(expectedMetrics)), sortLines(dumpedMetrics))
		})
	}
}

func TestTSDBDumpOpenMetricsRoundTrip(t *testing.T) {
	initialMetrics, err := os.ReadFile("testdata/dump-openmetrics-roundtrip-test.prom")
	require.NoError(t, err)
	initialMetrics = normalizeNewLine(initialMetrics)

	dbDir := t.TempDir()
	// Import samples from OM format
	err = backfill(5000, initialMetrics, dbDir, false, false, 2*time.Hour, map[string]string{})
	require.NoError(t, err)
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

func TestPromtoolAnalyze(t *testing.T) {
	metrics := "test_1{} 1 2 3 4 5\n"
	for i := 1; i < 30; i++ {
		metrics += fmt.Sprintf("test_%d{index=\"0\"} 1 2 3 4 5\n", i)
	}
	storage := promqltest.LoadedStorage(t, "load 1m\n"+metrics)
	t.Cleanup(func() { storage.Close() })

	storage.CompactHead(tsdb.NewRangeHead(storage.Head(), 0, 10))

	output, err := Promtool("tsdb", "analyze", "--extended", storage.Dir())
	require.NoError(t, err)

	fmt.Println("Output of `promtool tsdb analyze --extended`:")
	fmt.Println(output)
	require.Contains(t, output, strings.TrimSpace(`
Top 20 metric names by disk usage:
          34 bytes (  6.67%) test_1
          17 bytes (  3.33%) test_10
          17 bytes (  3.33%) test_11
          17 bytes (  3.33%) test_12
          17 bytes (  3.33%) test_13
          17 bytes (  3.33%) test_14
          17 bytes (  3.33%) test_15
          17 bytes (  3.33%) test_16
          17 bytes (  3.33%) test_17
          17 bytes (  3.33%) test_18
          17 bytes (  3.33%) test_19
          17 bytes (  3.33%) test_2
          17 bytes (  3.33%) test_20
          17 bytes (  3.33%) test_21
          17 bytes (  3.33%) test_22
          17 bytes (  3.33%) test_23
          17 bytes (  3.33%) test_24
          17 bytes (  3.33%) test_25
          17 bytes (  3.33%) test_26
          17 bytes (  3.33%) test_27
  total (top       20):          357 bytes (70.00%)
  total (all       29):          510 bytes
	`), "The output of `promtool tsdb analyze --extended` does not contain the metric disk usage info.")
}
