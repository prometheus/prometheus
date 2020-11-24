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

type backfillSample struct {
	Timestamp int64
	Value     float64
	Labels    labels.Labels
}

func createTemporaryOpenMetricsFile(t *testing.T, text string) string {
	newf, err := ioutil.TempFile("", "")
	require.NoError(t, err)

	_, err = newf.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, newf.Close())

	return newf.Name()
}

func sortSamples(samples []backfillSample) {
	sort.Slice(samples, func(x, y int) bool {
		sx, sy := samples[x], samples[y]
		if sx.Timestamp != sy.Timestamp {
			return sx.Timestamp < sy.Timestamp
		}
		return sx.Value < sy.Value
	})
}

func queryblock(t testing.TB, q storage.Querier, lbls labels.Labels, matchers ...*labels.Matcher) ([]backfillSample, error) {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()
	samples := []backfillSample{}
	for ss.Next() {
		series := ss.At()
		it := series.Iterator()
		require.NoError(t, it.Err())
		for it.Next() {
			ts, v := it.At()
			samples = append(samples, backfillSample{Timestamp: ts, Value: v, Labels: series.Labels()})
		}
		if len(samples) == 0 {
			continue
		}
	}
	return samples, nil
}

func testBlocks(t *testing.T, blocks []*tsdb.Block, expectedMinTime, expectedMaxTime int64, expectedSamples []backfillSample, metricLabels []string, expectedNumBlocks int) {
	require.Equal(t, expectedNumBlocks, len(blocks))

	allSamples := make([]backfillSample, 0)

	var maxt, mint int64 = math.MinInt64, math.MaxInt64

	for _, block := range blocks {
		index, err := block.Index()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, index.Close())
		}()

		if maxt < block.Meta().MaxTime {
			maxt = block.Meta().MaxTime
		}
		if mint > block.Meta().MinTime {
			mint = block.Meta().MinTime
		}

		q, err := tsdb.NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		series, err := queryblock(t, q, labels.FromStrings(metricLabels...), labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
		require.NoError(t, err)

		allSamples = append(allSamples, series...)
	}

	sortSamples(allSamples)
	sortSamples(expectedSamples)
	require.Equal(t, expectedSamples, allSamples)

	require.Equal(t, expectedMinTime, mint)
	require.Equal(t, expectedMaxTime, maxt)
}

func TestBackfill(t *testing.T) {
	tests := []struct {
		ToParse              string
		IsOk                 bool
		MetricLabels         []string
		Description          string
		MaxSamplesInAppender int64
		Expected             struct {
			MinTime   int64
			MaxTime   int64
			NumBlocks int
			Samples   []backfillSample
		}
	}{
		{
			ToParse:              `# EOF`,
			IsOk:                 true,
			Description:          "Empty file.",
			MaxSamplesInAppender: 5000,
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   math.MaxInt64,
				MaxTime:   math.MinInt64,
				NumBlocks: 0,
				Samples:   []backfillSample{},
			},
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713.989
http_requests_total{code="400"} 1 1565133713.990
# EOF
`,
			IsOk:                 true,
			Description:          "Multiple samples with different timestamp for different series.",
			MaxSamplesInAppender: 5000,
			MetricLabels:         []string{"__name__", "http_requests_total"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565133713991,
				NumBlocks: 1,
				Samples: []backfillSample{
					{
						Timestamp: 1565133713989,
						Value:     1021,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565133713990,
						Value:     1,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
				},
			},
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713.989
http_requests_total{code="200"} 1 1565133714.989
http_requests_total{code="400"} 2 1565133715.989
# EOF
`,
			IsOk:                 true,
			Description:          "Multiple samples with different timestamp for the same series.",
			MaxSamplesInAppender: 5000,
			MetricLabels:         []string{"__name__", "http_requests_total"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565133715990,
				NumBlocks: 1,
				Samples: []backfillSample{
					{
						Timestamp: 1565133713989,
						Value:     1021,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565133714989,
						Value:     1,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565133715989,
						Value:     2,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
				},
			},
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713.989
http_requests_total{code="200"} 1022 1565144513.989
http_requests_total{code="400"} 2 1565155313.989
http_requests_total{code="400"} 1 1565166113.989
# EOF
`,
			IsOk:                 true,
			Description:          "Multiple samples that end up in different blocks.",
			MaxSamplesInAppender: 5000,
			MetricLabels:         []string{"__name__", "http_requests_total"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565166113990,
				NumBlocks: 4,
				Samples: []backfillSample{
					{
						Timestamp: 1565133713989,
						Value:     1021,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565144513989,
						Value:     1022,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565155313989,
						Value:     2,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
					{
						Timestamp: 1565166113989,
						Value:     1,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
				},
			},
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713.989
http_requests_total{code="200"} 1022 1565133714
http_requests_total{code="200"} 1023 1565133716
http_requests_total{code="200"} 1022 1565144513.989
http_requests_total{code="400"} 2 1565155313.989
http_requests_total{code="400"} 3 1565155314
http_requests_total{code="400"} 1 1565166113.989
# EOF
`,
			IsOk:                 true,
			Description:          "Number of samples are greater than the sample batch size.",
			MaxSamplesInAppender: 2,
			MetricLabels:         []string{"__name__", "http_requests_total"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565166113990,
				NumBlocks: 4,
				Samples: []backfillSample{
					{
						Timestamp: 1565133713989,
						Value:     1021,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565133714000,
						Value:     1022,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565133716000,
						Value:     1023,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565144513989,
						Value:     1022,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565155313989,
						Value:     2,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
					{
						Timestamp: 1565155314000,
						Value:     3,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
					{
						Timestamp: 1565166113989,
						Value:     1,
						Labels:    labels.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
				},
			},
		},
		{
			ToParse: `no_help_no_type{foo="bar"} 42 6900
# EOF
`,
			IsOk:                 true,
			Description:          "Sample with no #HELP or #TYPE keyword.",
			MaxSamplesInAppender: 5000,
			MetricLabels:         []string{"__name__", "no_help_no_type"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   6900000,
				MaxTime:   6900001,
				NumBlocks: 1,
				Samples: []backfillSample{
					{
						Timestamp: 6900000,
						Value:     42,
						Labels:    labels.FromStrings("__name__", "no_help_no_type", "foo", "bar"),
					},
				},
			},
		},
		{
			ToParse: `bare_metric 42.24 1001
# EOF
`,
			IsOk:                 true,
			Description:          "Bare sample.",
			MaxSamplesInAppender: 5000,
			MetricLabels:         []string{"__name__", "bare_metric"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1001000,
				MaxTime:   1001001,
				NumBlocks: 1,
				Samples: []backfillSample{
					{
						Timestamp: 1001000,
						Value:     42.24,
						Labels:    labels.FromStrings("__name__", "bare_metric"),
					},
				},
			},
		},
		{
			ToParse: `# HELP rpc_duration_seconds A summary of the RPC duration in seconds.
# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.01"} 3102
rpc_duration_seconds{quantile="0.05"} 3272
# EOF
`,
			IsOk:        false,
			Description: "Does not have timestamp.",
		},
		{
			ToParse: `# HELP bad_metric This a bad metric
# TYPE bad_metric bad_type
bad_metric{type="has a bad type information"} 0.0 111
# EOF
`,
			IsOk:        false,
			Description: "Has a bad type information.",
		},
		{
			ToParse: `# HELP no_nl This test has no newline so will fail
# TYPE no_nl gauge
no_nl{type="no newline"}
# EOF
`,
			IsOk:        false,
			Description: "No newline.",
		},
	}
	for _, test := range tests {

		t.Logf("Test:%s", test.Description)

		openMetricsFile := createTemporaryOpenMetricsFile(t, test.ToParse)

		input, errOpen := os.Open(openMetricsFile)
		require.NoError(t, errOpen)
		defer func() {
			require.NoError(t, input.Close())
		}()

		outputDir, errd := ioutil.TempDir("", "myDir")
		require.NoError(t, errd)
		defer func() {
			require.NoError(t, os.RemoveAll(outputDir))
		}()

		errb := backfill(test.MaxSamplesInAppender, input, outputDir)

		if !test.IsOk {
			require.Error(t, errb, test.Description)
			continue
		}

		opts := tsdb.DefaultOptions()

		db, err := tsdb.Open(outputDir, nil, nil, opts)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, db.Close())
		}()

		blocks := db.Blocks()
		testBlocks(t, blocks, test.Expected.MinTime, test.Expected.MaxTime, test.Expected.Samples, test.MetricLabels, test.Expected.NumBlocks)

	}
}
