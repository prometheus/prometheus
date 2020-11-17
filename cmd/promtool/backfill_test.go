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

package main

import (
	"io/ioutil"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/require"
)

type mySample struct {
	Timestamp int64
	Value     float64
	Labels    labels.Labels
}

type backfillSample struct {
	t int64
	v float64
}

func (s backfillSample) T() int64   { return s.t }
func (s backfillSample) V() float64 { return s.v }

func createTemporaryOpenmetricsFile(t *testing.T, omFile string, text string) error {
	f, err := os.Create(omFile)
	require.NoError(t, err)
	_, errW := f.WriteString(text)
	require.NoError(t, errW)
	require.NoError(t, f.Close())
	return nil
}

func queryblock(t testing.TB, q storage.Querier, lbls labels.Labels, matchers ...*labels.Matcher) ([]mySample, error) {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()
	samples := []mySample{}
	for ss.Next() {
		series := ss.At()
		it := series.Iterator()
		for it.Next() {
			ts, v := it.At()
			samples = append(samples, mySample{Timestamp: ts, Value: v, Labels: lbls})
		}
		require.NoError(t, it.Err())
		if len(samples) == 0 {
			continue
		}
	}
	return samples, nil
}

func testBlocks(t *testing.T, blocks []tsdb.BlockReader, expectedSamples []mySample, metricLabels []string, expectedSymbols []string, expectedNumBlocks int) {
	require.Equal(t, expectedNumBlocks, len(blocks))
	allSymbols := make(map[string]struct{})
	allSamples := make([]mySample, 0)
	for _, block := range blocks {
		index, err := block.Index()
		require.NoError(t, err)
		symbols := index.Symbols()
		for symbols.Next() {
			key := symbols.At()
			if _, ok := allSymbols[key]; !ok {
				allSymbols[key] = struct{}{}
			}
		}
		q, err := tsdb.NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series, err := queryblock(t, q, labels.FromStrings(metricLabels...), labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
		require.NoError(t, err)
		allSamples = append(allSamples, series[0])
		_ = index.Close()
	}
	allSymbolsSlice := make([]string, 0)
	for key := range allSymbols {
		allSymbolsSlice = append(allSymbolsSlice, key)
	}
	sort.Strings(allSymbolsSlice)
	sort.Strings(expectedSymbols)
	require.Equal(t, expectedSymbols, allSymbolsSlice)
	// sortSamples expected and allSamples and compare
	// check if dB time ranges are correct
}

func TestBackfill(t *testing.T) {
	tests := []struct {
		ToParse      string
		IsOk         bool
		MetricLabels []string
		Expected     struct {
			MinTime   int64
			MaxTime   int64
			Symbols   []string
			NumBlocks int
			Samples   []mySample
		}
	}{
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713989
http_requests_total{code="400"} 1 1565144513989
# EOF
`,
			IsOk:         true,
			MetricLabels: []string{"__name__", "http_requests_total"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				Symbols   []string
				NumBlocks int
				Samples   []mySample
			}{
				MinTime:   1565133713989000,
				MaxTime:   1565144513989000,
				Symbols:   []string{"http_requests_total", "code", "200", "400", "__name__"},
				NumBlocks: 2,
				Samples: []mySample{
					{
						Timestamp: 1565133713989000,
						Value:     1021,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565144513989000,
						Value:     1,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
				},
			},
		},
	}
	for _, test := range tests {
		omFile := "backfill_test.om"
		require.NoError(t, createTemporaryOpenmetricsFile(t, omFile, test.ToParse))
		defer os.RemoveAll("backfill_test.om")
		input, errOpen := os.Open(omFile)
		require.NoError(t, errOpen)
		outputDir, errd := ioutil.TempDir("", "data")
		require.NoError(t, errd)
		// outputDir := "./data" + fmt.Sprint(testID)
		errb := backfill(input, outputDir)
		// ranges
		defer os.RemoveAll(outputDir)
		if test.IsOk {
			require.NoError(t, errb)
			if len(test.Expected.Symbols) > 0 {
				db, err := tsdb.OpenDBReadOnly(outputDir, nil) // change
				require.NoError(t, err)
				blocks, err := db.Blocks()
				testBlocks(t, blocks, test.Expected.Samples, test.MetricLabels, test.Expected.Symbols, test.Expected.NumBlocks)
			}
			// _, errReset := input.Seek(0, 0)
			// require.NoError(t, errReset)
			// p := NewParser(input)
			// maxt, mint, errTs := getMinAndMaxTimestamps(p)
			// require.NoError(t, errTs)
			// require.Equal(t, test.Expected.MinTime, mint)
			// require.Equal(t, test.Expected.MaxTime, maxt)
			// require.NoError(t, input.Close())
			// blocks, _ := ioutil.ReadDir(outputDir)
			// require.Equal(t, test.Expected.NumBlocks, len(blocks))
			// for _, block := range blocks {
			// 	blockpath := filepath.Join(outputDir, block.Name())
			// 	require.NoError(t, os.MkdirAll(path.Join(blockpath, "wal"), 0777))
			// 	// testBlocks(t, blockpath, mint, maxt)
			//}
		} else {
			require.Error(t, errb)
		}
	}
}

// func TestBackfill(t *testing.T) {
// 	tests := []struct {
// 		ToParse  string
// 		IsOk     bool
// 		Expected struct {
// 			MinTime int64
// 			MaxTime int64
// 			Series  map[string][]tsdbutil.Sample
// 		}
// 	}{
// 		{
// 			ToParse: `# EOF`,
// 			IsOk:    true,
// 		},
// 		{
// 			ToParse: `# HELP http_requests_total The total number of HTTP requests.
// # TYPE http_requests_total counter
// http_requests_total{code="200"} 1021 1565133713989
// http_requests_total{code="400"} 1 1565133713990
// # EOF
// `,
// 			IsOk: true,
// 			Expected: struct {
// 				MinTime int64
// 				MaxTime int64
// 				Series  map[string][]tsdbutil.Sample
// 			}{
// 				MinTime: 1565133713989000,
// 				MaxTime: 1565133713990000,
// 				Series:  map[string][]tsdbutil.Sample{"{__name__=\"http_requests_total\", code=\"200\"}": []tsdbutil.Sample{backfillSample{t: 1565133713989000, v: 1021}}, "{__name__=\"http_requests_total\", code=\"400\"}": []tsdbutil.Sample{backfillSample{t: 1565133713990000, v: 1}}},
// 			},
// 		},
// 		{
// 			ToParse: `# HELP http_requests_total The total number of HTTP requests.
// # TYPE http_requests_total counter
// http_requests_total{code="200"} 1022 1565133713989
// http_requests_total{code="400"} 2 1565133713990
// # EOF
// `,
// 			IsOk: true,
// 			Expected: struct {
// 				MinTime int64
// 				MaxTime int64
// 				Series  map[string][]tsdbutil.Sample
// 			}{
// 				MinTime: 1565133713989000,
// 				MaxTime: 1565133713990000,
// 				Series:  map[string][]tsdbutil.Sample{"{__name__=\"http_requests_total\", code=\"200\"}": []tsdbutil.Sample{backfillSample{t: 1565133713989000, v: 1022}}, "{__name__=\"http_requests_total\", code=\"400\"}": []tsdbutil.Sample{backfillSample{t: 1565133713990000, v: 2}}},
// 			},
// 		},
// 		{
// 			ToParse: `# HELP http_requests_total The total number of HTTP requests.
// # TYPE http_requests_total counter
// http_requests_total{code="200"} 1023 1395066363000
// http_requests_total{code="400"} 3 1395066363000
// # EOF
// `,
// 			IsOk: true,
// 			Expected: struct {
// 				MinTime int64
// 				MaxTime int64
// 				Series  map[string][]tsdbutil.Sample
// 			}{
// 				MinTime: 1395066363000000,
// 				MaxTime: 1395066363000000,
// 				Series:  map[string][]tsdbutil.Sample{"{__name__=\"http_requests_total\", code=\"200\"}": []tsdbutil.Sample{backfillSample{t: 1395066363000000, v: 1023}}, "{__name__=\"http_requests_total\", code=\"400\"}": []tsdbutil.Sample{backfillSample{t: 1395066363000000, v: 3}}},
// 			},
// 		},
// 		{
// 			ToParse: `# HELP rpc_duration_seconds A summary of the RPC duration in seconds.
// 		# TYPE rpc_duration_seconds summary
// 		rpc_duration_seconds{quantile="0.01"} 3102
// 		rpc_duration_seconds{quantile="0.05"} 3272
// 		# EOF
// 		`,
// 			IsOk: false,
// 		},
// 		{
// 			ToParse: `# HELP bad_ts This is a metric with an extreme timestamp
// 		# TYPE bad_ts gauge
// 		bad_ts{type="bad_timestamp"} 420 -1e99
// 		# EOF
// 		`,
// 			IsOk: false,
// 		},
// 		{
// 			ToParse: `# HELP bad_ts This is a metric with an extreme timestamp
// 		# TYPE bad_ts gauge
// 		bad_ts{type="bad_timestamp"} 420 1e99
// 		# EOF
// 		`,
// 			IsOk: false,
// 		},
// 		{
// 			ToParse: `no_help_no_type{foo="bar"} 42 6900
// # EOF
// `,
// 			IsOk: true,
// 			Expected: struct {
// 				MinTime int64
// 				MaxTime int64
// 				Series  map[string][]tsdbutil.Sample
// 			}{
// 				MinTime: 6900000,
// 				MaxTime: 6900000,
// 				Series:  map[string][]tsdbutil.Sample{"{__name__=\"no_help_no_type\", foo=\"bar\"}": []tsdbutil.Sample{backfillSample{t: 6900000, v: 42}}},
// 			},
// 		},
// 		{
// 			ToParse: `bare_metric 42.24 1001
// # EOF
// `,
// 			IsOk: true,
// 			Expected: struct {
// 				MinTime int64
// 				MaxTime int64
// 				Series  map[string][]tsdbutil.Sample
// 			}{
// 				MinTime: 1001000,
// 				MaxTime: 1001000,
// 				Series:  map[string][]tsdbutil.Sample{"{__name__=\"bare_metric\"}": []tsdbutil.Sample{backfillSample{t: 1001000, v: 42.24}}},
// 			},
// 		},
// 		{
// 			ToParse: `# HELP bad_metric This a bad metric
// 		# TYPE bad_metric bad_type
// 		bad_metric{type="has a bad type information"} 0.0 111
// 		# EOF
// 		`,
// 			IsOk: false,
// 		},
// 		{
// 			ToParse: `# HELP no_nl This test has no newline so will fail
// 		# TYPE no_nl gauge
// 		no_nl{type="no newline"}
// 		# EOF`,
// 			IsOk: false,
// 		},
// 	}
// 	for testID, test := range tests {
// 		omFile := "backfill_test.om"
// 		require.NoError(t, createTemporaryOpenmetricsFile(t, omFile, test.ToParse))
// 		defer os.RemoveAll("backfill_test.om")
// 		input, errOpen := os.Open(omFile)
// 		require.NoError(t, errOpen)
// 		outputDir := "./data" + fmt.Sprint(testID)
// 		errb := backfill(input, outputDir)
// 		defer os.RemoveAll(outputDir)
// 		if test.IsOk {
// 			_, errReset := input.Seek(0, 0)
// 			require.NoError(t, errReset)
// 			p := NewParser(input)
// 			maxt, mint, errTs := getMinAndMaxTimestamps(p)
// 			require.NoError(t, errTs)
// 			require.Equal(t, test.Expected.MinTime, mint)
// 			require.Equal(t, test.Expected.MaxTime, maxt)
// 			require.NoError(t, input.Close())
// 			blocks, _ := ioutil.ReadDir(outputDir)
// 			for _, block := range blocks {
// 				blockpath := filepath.Join(outputDir, block.Name())
// 				require.NoError(t, os.MkdirAll(path.Join(blockpath, "wal"), 0777))
// 				testBlocks(t, blockpath, mint, maxt, test.Expected.Series)
// 			}
// 		} else {
// 			require.Error(t, errb)
// 		}
// 	}
// }
