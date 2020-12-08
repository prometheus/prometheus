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
	"context"
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

func sortSamples(samples []backfillSample) {
	sort.Slice(samples, func(x, y int) bool {
		sx, sy := samples[x], samples[y]
		if sx.Timestamp != sy.Timestamp {
			return sx.Timestamp < sy.Timestamp
		}
		return sx.Value < sy.Value
	})
}

func queryAllSeries(t testing.TB, q storage.Querier, expectedMinTime, expectedMaxTime int64) []backfillSample {
	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
	samples := []backfillSample{}
	for ss.Next() {
		series := ss.At()
		it := series.Iterator()
		require.NoError(t, it.Err())
		for it.Next() {
			ts, v := it.At()
			samples = append(samples, backfillSample{Timestamp: ts, Value: v, Labels: series.Labels()})
		}
	}
	return samples
}

func testBlocks(t *testing.T, db *tsdb.DB, expectedMinTime, expectedMaxTime int64, expectedSamples []backfillSample, expectedNumBlocks int) {
	blocks := db.Blocks()
	require.Equal(t, expectedNumBlocks, len(blocks))

	for _, block := range blocks {
		require.Equal(t, true, block.MinTime()/tsdb.DefaultBlockDuration == (block.MaxTime()-1)/tsdb.DefaultBlockDuration)
	}

	q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, q.Close())
	}()

	allSamples := queryAllSeries(t, q, expectedMinTime, expectedMaxTime)
	sortSamples(allSamples)
	sortSamples(expectedSamples)
	require.Equal(t, expectedSamples, allSamples)

	if len(allSamples) > 0 {
		require.Equal(t, expectedMinTime, allSamples[0].Timestamp)
		require.Equal(t, expectedMaxTime, allSamples[len(allSamples)-1].Timestamp)
	}
}

func TestBackfill(t *testing.T) {
	tests := []struct {
		ToParse              string
		IsOk                 bool
		Description          string
		MaxSamplesInAppender int
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
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565133713990,
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
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565133715989,
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
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565166113989,
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
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565166113989,
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
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   6900000,
				MaxTime:   6900000,
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
			ToParse: `no_newline_after_eof 42 6900
# EOF`,
			IsOk:                 true,
			Description:          "Sample without newline after # EOF.",
			MaxSamplesInAppender: 5000,
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   6900000,
				MaxTime:   6900000,
				NumBlocks: 1,
				Samples: []backfillSample{
					{
						Timestamp: 6900000,
						Value:     42,
						Labels:    labels.FromStrings("__name__", "no_newline_after_eof"),
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
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Samples   []backfillSample
			}{
				MinTime:   1001000,
				MaxTime:   1001000,
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
		{
			ToParse: `# HELP no_eof This test has no EOF so will fail
# TYPE no_eof gauge
no_eof 1 1
`,
			IsOk:        false,
			Description: "No EOF.",
		},
		{
			ToParse: `# HELP after_eof There is data after EOF.
# TYPE after_eof gauge
after_eof 1 1
# EOF
after_eof 1 2
`,
			IsOk:        false,
			Description: "Data after EOF.",
		},
	}
	for _, test := range tests {
		t.Logf("Test:%s", test.Description)

		outputDir, err := ioutil.TempDir("", "myDir")
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.RemoveAll(outputDir))
		}()

		err = backfill(test.MaxSamplesInAppender, []byte(test.ToParse), outputDir)

		if !test.IsOk {
			require.Error(t, err, test.Description)
			continue
		}

		require.NoError(t, err)
		db, err := tsdb.Open(outputDir, nil, nil, tsdb.DefaultOptions())
		require.NoError(t, err)
		defer func() {
			require.NoError(t, db.Close())
		}()

		testBlocks(t, db, test.Expected.MinTime, test.Expected.MaxTime, test.Expected.Samples, test.Expected.NumBlocks)
	}
}
