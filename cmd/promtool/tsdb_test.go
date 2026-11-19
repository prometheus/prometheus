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
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
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
		parser.NewParser(parser.Options{}),
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

func TestTSDBDumpNativeHistogram(t *testing.T) {
	testStorage := promqltest.LoadedStorage(t, `
		load 1m
			mixed_hist{job="test"} 1 {{sum:2 count:2}} 3
	`)

	app := testStorage.AppenderV2(context.Background())
	for i := range 3 {
		_, err := app.Append(
			0,
			labels.FromStrings(labels.MetricName, "integer_hist", "job", "test"),
			0,
			int64(i)*int64(time.Minute/time.Millisecond),
			0,
			&histogram.Histogram{
				Count:         uint64(i + 1),
				Sum:           float64(i + 1),
				ZeroCount:     uint64(i + 1),
				ZeroThreshold: 0.001,
			},
			nil,
			storage.AppendV2Options{},
		)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	dumpedMetrics := getDumpedSamples(t, testStorage.Dir(), "", math.MinInt64, math.MaxInt64, []string{"{__name__=~'(?s:.*)'}"}, formatSeriesSet)
	expected := `
{__name__="integer_hist", job="test"} {count:1, sum:1, [-0.001,0.001]:1} 0
{__name__="integer_hist", job="test"} {count:2, sum:2, [-0.001,0.001]:2} 60000
{__name__="integer_hist", job="test"} {count:3, sum:3, [-0.001,0.001]:3} 120000
{__name__="mixed_hist", job="test"} 1 0
{__name__="mixed_hist", job="test"} {count:2, sum:2} 60000
{__name__="mixed_hist", job="test"} 3 120000
`
	require.Equal(t, sortLines(strings.TrimSpace(expected)), sortLines(strings.TrimSpace(dumpedMetrics)))
}

func TestFormatSeriesSetRejectsUnsupportedSampleType(t *testing.T) {
	tests := []struct {
		formatName  string
		format      func(storage.SeriesSet) error
		valType     chunkenc.ValueType
		expectedErr string
	}{
		{
			formatName:  "prom rejects unknown",
			format:      formatSeriesSet,
			valType:     chunkenc.ValueType(255),
			expectedErr: "unknown sample type unknown",
		},
		{
			formatName:  "openmetrics rejects unknown",
			format:      formatSeriesSetOpenMetrics,
			valType:     chunkenc.ValueType(255),
			expectedErr: "unknown sample type unknown",
		},
		{
			formatName:  "openmetrics rejects integer histogram",
			format:      formatSeriesSetOpenMetrics,
			valType:     chunkenc.ValHistogram,
			expectedErr: "unsupported sample type histogram",
		},
		{
			formatName:  "openmetrics rejects float histogram",
			format:      formatSeriesSetOpenMetrics,
			valType:     chunkenc.ValFloatHistogram,
			expectedErr: "unsupported sample type floathistogram",
		},
	}

	for _, tt := range tests {
		t.Run(tt.formatName, func(t *testing.T) {
			ss := &singleSeriesSet{
				series: &storage.SeriesEntry{
					Lset: labels.FromStrings(labels.MetricName, "test"),
					SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
						return &valueTypeIterator{valType: tt.valType}
					},
				},
			}

			require.EqualError(t, tt.format(ss), tt.expectedErr)
		})
	}
}

func TestFormatSeriesSetOpenMetricsRejectsNativeHistogramInMixedSeries(t *testing.T) {
	ss := &singleSeriesSet{
		series: &storage.SeriesEntry{
			Lset: labels.FromStrings(labels.MetricName, "mixed"),
			SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
				return &mixedIterator{
					types: []chunkenc.ValueType{chunkenc.ValFloat, chunkenc.ValHistogram},
				}
			},
		},
	}

	var err error
	out := captureStdout(t, func() {
		err = formatSeriesSetOpenMetrics(ss)
	})
	require.EqualError(t, err, "unsupported sample type histogram")
	require.Contains(t, out, "mixed{} 0 0.000")
	require.NotContains(t, out, "# EOF")
}

func TestFormatSeriesSetReturnsSeriesSetError(t *testing.T) {
	for name, format := range map[string]func(storage.SeriesSet) error{
		"prom":        formatSeriesSet,
		"json":        formatSeriesSetLabelsToJSON,
		"openmetrics": formatSeriesSetOpenMetrics,
	} {
		t.Run(name, func(t *testing.T) {
			ss := &singleSeriesSet{
				err: errors.New("series set error"),
			}

			var err error
			out := captureStdout(t, func() {
				err = format(ss)
			})
			require.EqualError(t, err, "series set error")
			require.NotContains(t, out, "# EOF")
		})
	}
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestFormatSeriesSetReturnsIteratorError(t *testing.T) {
	for name, format := range map[string]func(storage.SeriesSet) error{
		"prom":        formatSeriesSet,
		"openmetrics": formatSeriesSetOpenMetrics,
	} {
		t.Run(name, func(t *testing.T) {
			ss := &singleSeriesSet{
				series: &storage.SeriesEntry{
					Lset: labels.FromStrings(labels.MetricName, "corrupt"),
					SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
						return &errIterator{err: errors.New("checksum mismatch")}
					},
				},
			}

			require.EqualError(t, format(ss), "checksum mismatch")
		})
	}
}

type singleSeriesSet struct {
	series storage.Series
	done   bool
	err    error
}

func (s *singleSeriesSet) Next() bool {
	if s.err != nil || s.done {
		return false
	}
	s.done = true
	return true
}

func (s *singleSeriesSet) At() storage.Series              { return s.series }
func (s *singleSeriesSet) Err() error                      { return s.err }
func (*singleSeriesSet) Warnings() annotations.Annotations { return nil }

type valueTypeIterator struct {
	chunkenc.Iterator
	valType chunkenc.ValueType
	done    bool
}

func (it *valueTypeIterator) Next() chunkenc.ValueType {
	if it.done {
		return chunkenc.ValNone
	}
	it.done = true
	return it.valType
}

func (*valueTypeIterator) Err() error { return nil }

type mixedIterator struct {
	chunkenc.Iterator
	types []chunkenc.ValueType
	idx   int
}

func (it *mixedIterator) Next() chunkenc.ValueType {
	if it.idx >= len(it.types) {
		return chunkenc.ValNone
	}
	vt := it.types[it.idx]
	it.idx++
	return vt
}

func (*mixedIterator) At() (int64, float64) {
	return 0, 0
}

func (*mixedIterator) Err() error { return nil }

type errIterator struct {
	chunkenc.Iterator
	err error
}

func (*errIterator) Next() chunkenc.ValueType { return chunkenc.ValNone }
func (it *errIterator) Err() error            { return it.err }

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
	var sb strings.Builder
	sb.WriteString("test_1{} 1 2 3 4 5\n")
	for i := 1; i < 30; i++ {
		fmt.Fprintf(&sb, "test_%d{index=\"0\"} 1 2 3 4 5\n", i)
	}
	storage := promqltest.LoadedStorage(t, "load 1m\n"+sb.String())

	require.NoError(t, storage.CompactHead(tsdb.NewRangeHead(storage.Head(), 0, 10)))

	cmd := exec.Command(promtoolPath, "-test.main", "tsdb", "analyze", "--extended", storage.Dir())
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	require.NoError(t, cmd.Run())
	output := stdout.String()

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
