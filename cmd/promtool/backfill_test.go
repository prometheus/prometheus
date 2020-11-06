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
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

type backfillSample struct {
	t int64
	v float64
}

func (s backfillSample) T() int64   { return s.t }
func (s backfillSample) V() float64 { return s.v }

func createTemporaryOpenmetricsFile(t *testing.T, omFile string, text string) error {
	f, err := os.Create(omFile)
	testutil.Ok(t, err)
	_, errW := f.WriteString(text)
	testutil.Ok(t, errW)
	testutil.Ok(t, f.Close())
	return nil
}
func testBlocks(t *testing.T, blockpath string, mint, maxt int64, expectedSeries map[string][]tsdbutil.Sample) {
	db, err := tsdb.OpenDBReadOnly(blockpath, nil)
	testutil.Ok(t, err)
	q, err := db.Querier(context.TODO(), mint, maxt)
	testutil.Ok(t, err)
	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
	for ss.Next() {
		series := ss.At()
		testutil.Equals(t, expectedSeries, series)
	}
	testutil.Ok(t, db.Close())
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
		// Handle empty file
		// {
		// 	ToParse: `# EOF`,
		// 	IsOk:    true,
		// },
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
				Series:  map[string][]tsdbutil.Sample{"{http_requests_total=\"200\"}": []tsdbutil.Sample{backfillSample{1565133713989, 1021}}, "{http_requests_total=\"400\"}": []tsdbutil.Sample{backfillSample{1565133713990, 1}}},
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
				Series:  map[string][]tsdbutil.Sample{"{http_requests_total=\"200\"}": []tsdbutil.Sample{backfillSample{1565133713989, 1022}}, "{http_requests_total=\"400\"}": []tsdbutil.Sample{backfillSample{1565133713990, 2}}},
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
				Series:  map[string][]tsdbutil.Sample{"{http_requests_total=\"200\"}": []tsdbutil.Sample{backfillSample{1395066363000000, 1023}}, "{http_requests_total=\"400\"}": []tsdbutil.Sample{backfillSample{1395066363000000, 3}}},
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
		// {
		// 	ToParse: `# HELP rpc_duration_seconds A summary of the RPC duration in seconds.
		// # TYPE rpc_duration_seconds summary
		// rpc_duration_seconds{quantile="0.01"} 3102
		// rpc_duration_seconds{quantile="0.05"} 3272
		// # EOF
		// `,
		// 	IsOk: false,
		// },
		// 		{
		// 			ToParse: `# HELP bad_ts This is a metric with an extreme timestamp
		// # TYPE bad_ts gauge
		// bad_ts{type="bad_timestamp"} 420 1e99
		// # EOF
		// `,
		// 			IsOk: false,
		// 		},
	}
	for _, test := range tests {
		omFile := "backfill_test.om"
		testutil.Ok(t, createTemporaryOpenmetricsFile(t, omFile, test.ToParse))
		defer os.RemoveAll("backfill_test.om")
		input, errOpen := os.Open(omFile)
		testutil.Ok(t, errOpen)
		outputDir := "./data"
		errb := backfill(input, outputDir)
		defer os.RemoveAll(outputDir)
		if test.IsOk {
			_, errReset := input.Seek(0, 0)
			testutil.Ok(t, errReset)
			p := NewParser(input)
			maxt, mint, errTs := getMinAndMaxTimestamps(p)
			testutil.Ok(t, errTs)
			testutil.Equals(t, test.Expected.MinTime, mint)
			testutil.Equals(t, test.Expected.MaxTime, maxt)
			testutil.Ok(t, input.Close())
			blocks, _ := ioutil.ReadDir(outputDir)
			for _, block := range blocks {
				blockpath := filepath.Join(outputDir, block.Name())
				testutil.Ok(t, os.MkdirAll(path.Join(blockpath, "wal"), 0777))
				testBlocks(t, blockpath, mint, maxt, test.Expected.Series)
			}
		} else {
			testutil.NotOk(t, errb)
		}
	}
}
