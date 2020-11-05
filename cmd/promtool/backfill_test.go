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
	"github.com/prometheus/prometheus/tsdb/importer/openmetrics"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

type sample1 struct {
	t int64
	v float64
}

func (s sample1) T() int64   { return s.t }
func (s sample1) V() float64 { return s.v }

func createTemporaryOpenmetricsFile(text string, omFile string) error {
	f, err := os.Create(omFile)
	if err != nil {
		return err
	}
	_, errW := f.WriteString(text)
	if errW != nil {
		f.Close()
		return errW
	}
	err = f.Close()
	if err != nil {
		return err
	}
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
				Series:  map[string][]tsdbutil.Sample{"{http_requests_total=\"200\"}": []tsdbutil.Sample{sample1{1565133713989, 1021}}, "{http_requests_total=\"400\"}": []tsdbutil.Sample{sample1{1565133713990, 1}}},
			},
		},
	}
	for _, test := range tests {
		omFile := "backfill_test.om"
		testutil.Ok(t, createTemporaryOpenmetricsFile(test.ToParse, omFile))
		input, errOpen := os.Open(omFile)
		testutil.Ok(t, errOpen)
		outputDir := "./tmpDir"
		testutil.Ok(t, backfill(input, outputDir))
		_, errReset := input.Seek(0, 0)
		testutil.Ok(t, errReset)
		p := openmetrics.NewParser(input)
		maxt, mint, errTs := getMinAndMaxTimestamps(p)
		testutil.Equals(t, test.Expected.MinTime, mint)
		testutil.Equals(t, test.Expected.MaxTime, maxt)
		if test.IsOk {
			blocks, _ := ioutil.ReadDir(outputDir)
			for _, block := range blocks {
				blockpath := filepath.Join(outputDir, block.Name())
				testutil.Ok(t, os.MkdirAll(path.Join(blockpath, "wal"), 0777))
				testBlocks(t, blockpath, mint, maxt, test.Expected.Series)
			}
		} else {
			testutil.NotOk(t, errTs)
		}
		testutil.Ok(t, os.RemoveAll("backfill_test.om"))
		testutil.Ok(t, os.RemoveAll(outputDir))
	}
}
