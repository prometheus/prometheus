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
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/stretchr/testify/assert"
)

type backfillSample struct {
	t int64
	v float64
}

func (s backfillSample) T() int64   { return s.t }
func (s backfillSample) V() float64 { return s.v }

func createTemporaryOpenmetricsFile(t *testing.T, omFile string, text string) error {
	f, err := os.Create(omFile)
	assert.NoError(t, err)
	_, errW := f.WriteString(text)
	assert.NoError(t, errW)
	assert.NoError(t, f.Close())
	return nil
}

// queryblock runs a matcher query against the querier and fully expands its data.
// This function is same as query from tsdb/db_test.go If you change either, please make sure to change at both places.
func queryblock(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]tsdbutil.Sample {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		assert.NoError(t, q.Close())
	}()

	result := map[string][]tsdbutil.Sample{}
	for ss.Next() {
		series := ss.At()

		samples := []tsdbutil.Sample{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			samples = append(samples, backfillSample{t: t, v: v})
		}
		assert.NoError(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	assert.NoError(t, ss.Err())
	assert.Equal(t, 0, len(ss.Warnings()))

	return result
}

func testBlocks(t *testing.T, blockpath string, mint, maxt int64, expectedSeries map[string][]tsdbutil.Sample) {
	b, err := tsdb.OpenBlock(nil, blockpath, nil)
	assert.NoError(t, err)
	q, err := tsdb.NewBlockQuerier(b, math.MinInt64, math.MaxInt64)
	assert.NoError(t, err)
	series := queryblock(t, q, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
	assert.Equal(t, expectedSeries, series)
}
func TestBackfill(t *testing.T) {
	tests := []struct {
		ToParse  string
		IsOk     bool
		Expected struct {
			MinTime int64
			MaxTime int64
			Series  map[string][]tsdbutil.Sample
		}
	}{
		{
			ToParse: `# EOF`,
			IsOk:    true,
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713989
http_requests_total{code="400"} 1 1565133713990
# EOF
`,
			IsOk: true,
			Expected: struct {
				MinTime int64
				MaxTime int64
				Series  map[string][]tsdbutil.Sample
			}{
				MinTime: 1565133713989000,
				MaxTime: 1565133713990000,
				Series:  map[string][]tsdbutil.Sample{"{__name__=\"http_requests_total\", code=\"200\"}": []tsdbutil.Sample{backfillSample{t: 1565133713989000, v: 1021}}, "{__name__=\"http_requests_total\", code=\"400\"}": []tsdbutil.Sample{backfillSample{t: 1565133713990000, v: 1}}},
			},
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1022 1565133713989
http_requests_total{code="400"} 2 1565133713990
# EOF
`,
			IsOk: true,
			Expected: struct {
				MinTime int64
				MaxTime int64
				Series  map[string][]tsdbutil.Sample
			}{
				MinTime: 1565133713989000,
				MaxTime: 1565133713990000,
				Series:  map[string][]tsdbutil.Sample{"{__name__=\"http_requests_total\", code=\"200\"}": []tsdbutil.Sample{backfillSample{t: 1565133713989000, v: 1022}}, "{__name__=\"http_requests_total\", code=\"400\"}": []tsdbutil.Sample{backfillSample{t: 1565133713990000, v: 2}}},
			},
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1023 1395066363000
http_requests_total{code="400"} 3 1395066363000
# EOF
`,
			IsOk: true,
			Expected: struct {
				MinTime int64
				MaxTime int64
				Series  map[string][]tsdbutil.Sample
			}{
				MinTime: 1395066363000000,
				MaxTime: 1395066363000000,
				Series:  map[string][]tsdbutil.Sample{"{__name__=\"http_requests_total\", code=\"200\"}": []tsdbutil.Sample{backfillSample{t: 1395066363000000, v: 1023}}, "{__name__=\"http_requests_total\", code=\"400\"}": []tsdbutil.Sample{backfillSample{t: 1395066363000000, v: 3}}},
			},
		},
		{
			ToParse: `# HELP something_weird Something weird
		# TYPE something_weird gauge
		something_weird{problem="infinite timestamp"} +Inf -3982045
		# EOF
		`,
			IsOk: false,
		},
		{
			ToParse: `# HELP rpc_duration_seconds A summary of the RPC duration in seconds.
		# TYPE rpc_duration_seconds summary
		rpc_duration_seconds{quantile="0.01"} 3102
		rpc_duration_seconds{quantile="0.05"} 3272
		# EOF
		`,
			IsOk: false,
		},
		{
			ToParse: `# HELP bad_ts This is a metric with an extreme timestamp
		# TYPE bad_ts gauge
		bad_ts{type="bad_timestamp"} 420 -1e99
		# EOF
		`,
			IsOk: false,
		},
		{
			ToParse: `# HELP bad_ts This is a metric with an extreme timestamp
		# TYPE bad_ts gauge
		bad_ts{type="bad_timestamp"} 420 1e99
		# EOF
		`,
			IsOk: false,
		},
		{
			ToParse: `no_help_no_type{foo="bar"} 42 6900
# EOF
`,
			IsOk: true,
			Expected: struct {
				MinTime int64
				MaxTime int64
				Series  map[string][]tsdbutil.Sample
			}{
				MinTime: 6900000,
				MaxTime: 6900000,
				Series:  map[string][]tsdbutil.Sample{"{__name__=\"no_help_no_type\", foo=\"bar\"}": []tsdbutil.Sample{backfillSample{t: 6900000, v: 42}}},
			},
		},
		{
			ToParse: `bare_metric 42.24 1001
# EOF
`,
			IsOk: true,
			Expected: struct {
				MinTime int64
				MaxTime int64
				Series  map[string][]tsdbutil.Sample
			}{
				MinTime: 1001000,
				MaxTime: 1001000,
				Series:  map[string][]tsdbutil.Sample{"{__name__=\"bare_metric\"}": []tsdbutil.Sample{backfillSample{t: 1001000, v: 42.24}}},
			},
		},
		{
			ToParse: `# HELP bad_metric This a bad metric
		# TYPE bad_metric bad_type
		bad_metric{type="has no type information"} 0.0 111
		# EOF
		`,
			IsOk: false,
		},
		{
			ToParse: `# HELP no_nl This test has no newline so will fail
		# TYPE no_nl gauge
		no_nl{type="no newline"}
		# EOF`,
			IsOk: false,
		},
	}
	for testID, test := range tests {
		omFile := "backfill_test.om"
		assert.NoError(t, createTemporaryOpenmetricsFile(t, omFile, test.ToParse))
		defer os.RemoveAll("backfill_test.om")
		input, errOpen := os.Open(omFile)
		assert.NoError(t, errOpen)
		outputDir := "./data" + fmt.Sprint(testID)
		errb := backfill(input, outputDir)
		defer os.RemoveAll(outputDir)
		if test.IsOk {
			_, errReset := input.Seek(0, 0)
			assert.NoError(t, errReset)
			p := NewParser(input)
			maxt, mint, errTs := getMinAndMaxTimestamps(p)
			assert.NoError(t, errTs)
			assert.Equal(t, test.Expected.MinTime, mint)
			assert.Equal(t, test.Expected.MaxTime, maxt)
			assert.NoError(t, input.Close())
			blocks, _ := ioutil.ReadDir(outputDir)
			for _, block := range blocks {
				blockpath := filepath.Join(outputDir, block.Name())
				assert.NoError(t, os.MkdirAll(path.Join(blockpath, "wal"), 0777))
				testBlocks(t, blockpath, mint, maxt, test.Expected.Series)
			}
		} else {
			assert.Error(t, errb)
		}
	}
}
