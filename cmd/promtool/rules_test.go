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
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type mockQueryRangeAPI struct {
	samples model.Matrix
}

func (mockAPI mockQueryRangeAPI) QueryRange(_ context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error) { // nolint:revive
	return mockAPI.samples, v1.Warnings{}, nil
}

const defaultBlockDuration = time.Duration(tsdb.DefaultBlockDuration) * time.Millisecond

// TestBackfillRuleIntegration is an integration test that runs all the rule importer code to confirm the parts work together.
func TestBackfillRuleIntegration(t *testing.T) {
	const (
		testMaxSampleCount = 50
		testValue          = 123
		testValue2         = 98
	)
	var (
		start                     = time.Date(2009, time.November, 10, 6, 34, 0, 0, time.UTC)
		testTime                  = model.Time(start.Add(-9 * time.Hour).Unix())
		testTime2                 = model.Time(start.Add(-8 * time.Hour).Unix())
		twentyFourHourDuration, _ = time.ParseDuration("24h")
	)

	testCases := []struct {
		name                string
		runcount            int
		maxBlockDuration    time.Duration
		expectedBlockCount  int
		expectedSeriesCount int
		expectedSampleCount int
		samples             []*model.SampleStream
	}{
		{"no samples", 1, defaultBlockDuration, 0, 0, 0, []*model.SampleStream{}},
		{"run importer once", 1, defaultBlockDuration, 8, 4, 4, []*model.SampleStream{{Metric: model.Metric{"name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: testValue}}}}},
		{"run importer with dup name label", 1, defaultBlockDuration, 8, 4, 4, []*model.SampleStream{{Metric: model.Metric{"__name__": "val1", "name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: testValue}}}}},
		{"one importer twice", 2, defaultBlockDuration, 8, 4, 8, []*model.SampleStream{{Metric: model.Metric{"name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: testValue}, {Timestamp: testTime2, Value: testValue2}}}}},
		{"run importer once with larger blocks", 1, twentyFourHourDuration, 4, 4, 4, []*model.SampleStream{{Metric: model.Metric{"name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: testValue}}}}},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			ctx := context.Background()

			// Execute the test more than once to simulate running the rule importer twice with the same data.
			// We expect duplicate blocks with the same series are created when run more than once.
			for i := 0; i < tt.runcount; i++ {

				ruleImporter, err := newTestRuleImporter(ctx, start, tmpDir, tt.samples, tt.maxBlockDuration)
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
				require.Equal(t, defaultInterval*time.Second, group1.Interval())
				gRules := group1.Rules()
				require.Equal(t, 1, len(gRules))
				require.Equal(t, "rule1", gRules[0].Name())
				require.Equal(t, "ruleExpr", gRules[0].Query().String())
				require.Equal(t, 1, gRules[0].Labels().Len())

				group2 := ruleImporter.groups[path2+";group2"]
				require.NotNil(t, group2)
				require.Equal(t, defaultInterval*time.Second, group2.Interval())
				g2Rules := group2.Rules()
				require.Equal(t, 2, len(g2Rules))
				require.Equal(t, "grp2_rule1", g2Rules[0].Name())
				require.Equal(t, "grp2_rule1_expr", g2Rules[0].Query().String())
				require.Equal(t, 0, g2Rules[0].Labels().Len())

				// Backfill all recording rules then check the blocks to confirm the correct data was created.
				errs = ruleImporter.importAll(ctx)
				for _, err := range errs {
					require.NoError(t, err)
				}

				opts := tsdb.DefaultOptions()
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
					if series.Labels().Len() != 3 {
						require.Equal(t, 2, series.Labels().Len())
						x := labels.FromStrings("__name__", "grp2_rule1", "name1", "val1")
						require.Equal(t, x, series.Labels())
					} else {
						require.Equal(t, 3, series.Labels().Len())
					}
					it := series.Iterator(nil)
					for it.Next() == chunkenc.ValFloat {
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

func newTestRuleImporter(_ context.Context, start time.Time, tmpDir string, testSamples model.Matrix, maxBlockDuration time.Duration) (*ruleImporter, error) {
	logger := log.NewNopLogger()
	cfg := ruleImporterConfig{
		outputDir:        tmpDir,
		start:            start.Add(-10 * time.Hour),
		end:              start.Add(-7 * time.Hour),
		evalInterval:     60 * time.Second,
		maxBlockDuration: maxBlockDuration,
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
	return os.WriteFile(path, []byte(recordingRules), 0o777)
}

func createMultiRuleTestFiles(path string) error {
	recordingRules := `groups:
- name: group1
  rules:
  - record: grp1_rule1
    expr: grp1_rule1_expr
    labels:
        testlabel11: testlabelvalue12
- name: group2
  rules:
  - record: grp2_rule1
    expr: grp2_rule1_expr
  - record: grp2_rule2
    expr: grp2_rule2_expr
    labels:
        testlabel11: testlabelvalue13
`
	return os.WriteFile(path, []byte(recordingRules), 0o777)
}

// TestBackfillLabels confirms that the labels in the rule file override the labels from the metrics
// received from Prometheus Query API, including the __name__ label.
func TestBackfillLabels(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	start := time.Date(2009, time.November, 10, 6, 34, 0, 0, time.UTC)
	mockAPISamples := []*model.SampleStream{
		{
			Metric: model.Metric{"name1": "override-me", "__name__": "override-me-too"},
			Values: []model.SamplePair{{Timestamp: model.TimeFromUnixNano(start.UnixNano()), Value: 123}},
		},
	}
	ruleImporter, err := newTestRuleImporter(ctx, start, tmpDir, mockAPISamples, defaultBlockDuration)
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "test.file")
	recordingRules := `groups:
- name: group0
  rules:
  - record: rulename
    expr:  ruleExpr
    labels:
        name1: value-from-rule
`
	require.NoError(t, os.WriteFile(path, []byte(recordingRules), 0o777))
	errs := ruleImporter.loadGroups(ctx, []string{path})
	for _, err := range errs {
		require.NoError(t, err)
	}

	errs = ruleImporter.importAll(ctx)
	for _, err := range errs {
		require.NoError(t, err)
	}

	opts := tsdb.DefaultOptions()
	db, err := tsdb.Open(tmpDir, nil, nil, opts, nil)
	require.NoError(t, err)

	q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	t.Run("correct-labels", func(t *testing.T) {
		selectedSeries := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
		for selectedSeries.Next() {
			series := selectedSeries.At()
			expectedLabels := labels.FromStrings("__name__", "rulename", "name1", "value-from-rule")
			require.Equal(t, expectedLabels, series.Labels())
		}
		require.NoError(t, selectedSeries.Err())
		require.NoError(t, q.Close())
		require.NoError(t, db.Close())
	})
}
