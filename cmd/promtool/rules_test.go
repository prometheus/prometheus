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
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/require"
)

type mockQueryRangeAPI struct {
	samples model.Matrix
}

func (mockAPI mockQueryRangeAPI) QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, v1.Warnings, error) {
	return mockAPI.samples, v1.Warnings{}, nil
}

// TestBackfillRuleIntegration is an integration test that runs all the rule importer code to confirm the parts work together.
func TestBackfillRuleIntegration(t *testing.T) {
	const (
		testMaxSampleCount = 50
		testValue          = 123
		testValue2         = 98
	)
	var (
		start     = time.Date(2009, time.November, 10, 6, 34, 0, 0, time.UTC)
		testTime  = model.Time(start.Add(-9 * time.Hour).Unix())
		testTime2 = model.Time(start.Add(-8 * time.Hour).Unix())
	)

	var testCases = []struct {
		name                string
		runcount            int
		expectedBlockCount  int
		expectedSeriesCount int
		expectedSampleCount int
		samples             []*model.SampleStream
	}{
		{"no samples", 1, 0, 0, 0, []*model.SampleStream{}},
		{"run importer once", 1, 8, 4, 4, []*model.SampleStream{{Metric: model.Metric{"name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: testValue}}}}},
		{"one importer twice", 2, 8, 4, 8, []*model.SampleStream{{Metric: model.Metric{"name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: testValue}, {Timestamp: testTime2, Value: testValue2}}}}},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", "backfilldata")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(tmpDir))
			}()
			ctx := context.Background()

			// Execute the test more than once to simulate running the rule importer twice with the same data.
			// We expect duplicate blocks with the same series are created when run more than once.
			for i := 0; i < tt.runcount; i++ {
				ruleImporter, err := newTestRuleImporter(ctx, start, tmpDir, tt.samples)
				require.NoError(t, err)
				path1 := filepath.Join(tmpDir, "test.file")
				require.NoError(t, createSingleRuleTestFiles(path1))
				path2 := filepath.Join(tmpDir, "test2.file")
				require.NoError(t, createMultiRuleTestFiles(path2))

				// Confirm that the rule files were loaded in correctly.
				errs := ruleImporter.loadGroups(ctx, []string{path1, path2})
				for _, err := range errs {
					require.NoError(t, err)
				}
				require.Equal(t, 3, len(ruleImporter.groups))
				group1 := ruleImporter.groups[path1+";group0"]
				require.NotNil(t, group1)
				const defaultInterval = 60
				require.Equal(t, time.Duration(defaultInterval*time.Second), group1.Interval())
				gRules := group1.Rules()
				require.Equal(t, 1, len(gRules))
				require.Equal(t, "rule1", gRules[0].Name())
				require.Equal(t, "ruleExpr", gRules[0].Query().String())
				require.Equal(t, 1, len(gRules[0].Labels()))

				group2 := ruleImporter.groups[path2+";group2"]
				require.NotNil(t, group2)
				require.Equal(t, time.Duration(defaultInterval*time.Second), group2.Interval())
				g2Rules := group2.Rules()
				require.Equal(t, 2, len(g2Rules))
				require.Equal(t, "grp2_rule1", g2Rules[0].Name())
				require.Equal(t, "grp2_rule1_expr", g2Rules[0].Query().String())
				require.Equal(t, 0, len(g2Rules[0].Labels()))

				// Backfill all recording rules then check the blocks to confirm the correct data was created.
				errs = ruleImporter.importAll(ctx)
				for _, err := range errs {
					require.NoError(t, err)
				}

				opts := tsdb.DefaultOptions()
				opts.AllowOverlappingBlocks = true
				db, err := tsdb.Open(tmpDir, nil, nil, opts, nil)
				require.NoError(t, err)

				blocks := db.Blocks()
				require.Equal(t, (i+1)*tt.expectedBlockCount, len(blocks))

				q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
				require.NoError(t, err)

				selectedSeries := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
				var seriesCount, samplesCount int
				for selectedSeries.Next() {
					seriesCount++
					series := selectedSeries.At()
					if len(series.Labels()) != 3 {
						require.Equal(t, 2, len(series.Labels()))
						x := labels.Labels{
							labels.Label{Name: "__name__", Value: "grp2_rule1"},
							labels.Label{Name: "name1", Value: "val1"},
						}
						require.Equal(t, x, series.Labels())
					} else {
						require.Equal(t, 3, len(series.Labels()))
					}
					it := series.Iterator()
					for it.Next() {
						samplesCount++
						ts, v := it.At()
						if v == testValue {
							require.Equal(t, int64(testTime), ts)
						} else {
							require.Equal(t, int64(testTime2), ts)
						}
					}
					require.NoError(t, it.Err())
				}
				require.NoError(t, selectedSeries.Err())
				require.Equal(t, tt.expectedSeriesCount, seriesCount)
				require.Equal(t, tt.expectedSampleCount, samplesCount)
				require.NoError(t, q.Close())
				require.NoError(t, db.Close())
			}
		})
	}
}

func newTestRuleImporter(ctx context.Context, start time.Time, tmpDir string, testSamples model.Matrix) (*ruleImporter, error) {
	logger := log.NewNopLogger()
	cfg := ruleImporterConfig{
		outputDir:    tmpDir,
		start:        start.Add(-10 * time.Hour),
		end:          start.Add(-7 * time.Hour),
		evalInterval: 60 * time.Second,
	}

	return newRuleImporter(logger, cfg, mockQueryRangeAPI{
		samples: testSamples,
	}), nil
}

func createSingleRuleTestFiles(path string) error {
	recordingRules := `groups:
- name: group0
  rules:
  - record: rule1
    expr:  ruleExpr
    labels:
        testlabel11: testlabelvalue11
`
	return ioutil.WriteFile(path, []byte(recordingRules), 0777)
}

func createMultiRuleTestFiles(path string) error {
	recordingRules := `groups:
- name: group1
  rules:
  - record: grp1_rule1
    expr: grp1_rule1_expr
    labels:
        testlabel11: testlabelvalue11
- name: group2
  rules:
  - record: grp2_rule1
    expr: grp2_rule1_expr
  - record: grp2_rule2
    expr: grp2_rule2_expr
    labels:
        testlabel11: testlabelvalue11
`
	return ioutil.WriteFile(path, []byte(recordingRules), 0777)
}
