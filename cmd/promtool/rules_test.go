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
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb"
)

type mockQueryRangeAPI struct {
	samples model.Matrix
}

func (mockAPI mockQueryRangeAPI) QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, v1.Warnings, error) {
	return mockAPI.samples, v1.Warnings{}, nil
}

func createMockMetricsSingleSeries(ts time.Time) []*model.SampleStream {
	return []*model.SampleStream{
		{
			Metric: model.Metric{"metric1": "metric1val1"},
			Values: []model.SamplePair{
				{
					Timestamp: model.TimeFromUnixNano(ts.UnixNano()),
					Value:     1,
				},
				{
					Timestamp: model.TimeFromUnixNano(ts.Add(1 * evalInterval).UnixNano()),
					Value:     2,
				},
			},
		},
	}
}

const (
	defaultBlockDuration = time.Duration(tsdb.DefaultBlockDuration) * time.Millisecond
	evalInterval         = 60 * time.Second
)

// TestBackfillRuleIntegration is an integration test that runs all the rule importer code to confirm the parts work together.
func TestBackfillRuleIntegration(t *testing.T) {
	var (
		start                     = time.Date(2009, time.November, 10, 6, 34, 0, 0, time.UTC)
		testTime                  = start.Add(-9 * time.Hour)
		testTime2                 = start.Add(-8 * time.Hour)
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
		{"run importer once", 1, defaultBlockDuration, 12, 4, 12, createMockMetricsSingleSeries(testTime)},
		{"run importer twice", 2, defaultBlockDuration, 12, 4, 12, createMockMetricsSingleSeries(testTime2)},
		{"run importer once with larger blocks", 1, twentyFourHourDuration, 8, 4, 12, createMockMetricsSingleSeries(testTime)},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			ctx := context.Background()

			// Execute the test more than once to simulate running the rule importer twice with the same data.
			// We expect duplicate blocks with the same series are created when run more than once.
			for i := 0; i < tt.runcount; i++ {
				cfg := ruleImporterConfig{
					outputDir:        tmpDir,
					start:            start.Add(-10 * time.Hour),
					end:              start.Add(-7 * time.Hour),
					evalInterval:     evalInterval,
					maxBlockDuration: tt.maxBlockDuration,
				}
				ruleImporter := newRuleImporter(log.NewNopLogger(), cfg, mockQueryRangeAPI{
					samples: tt.samples,
				})
				path1 := filepath.Join(tmpDir, "test.file")
				require.NoError(t, createSingleRuleTestFiles(path1))
				path2 := filepath.Join(tmpDir, "test2.file")
				require.NoError(t, createMultiRuleTestFiles(path2))

				errs := ruleImporter.loadGroups(ctx, []string{path1, path2})
				for _, err := range errs {
					require.NoError(t, err)
				}

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
					it := series.Iterator()
					for it.Next() {
						samplesCount++
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
func TestBackfillParseRules(t *testing.T) {
	var (
		start1                    = time.Date(2009, time.November, 6, 1, 34, 0, 0, time.UTC)
		ttestTime                 = start1.Add(1 * time.Hour)
		ttestTime2                = start1.Add(2 * time.Hour)
		start                     = time.Date(2009, time.November, 10, 6, 34, 0, 0, time.UTC)
		testTime                  = model.Time(start.Add(-9 * time.Hour).Unix())
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
		{"run importer once", 1, defaultBlockDuration, 8, 4, 8, createMockMetricsSingleSeries(ttestTime)},
		{"run importer twice", 2, defaultBlockDuration, 8, 4, 8, createMockMetricsSingleSeries(ttestTime2)},
		{"run importer once with larger blocks", 1, twentyFourHourDuration, 4, 4, 4, []*model.SampleStream{{Metric: model.Metric{"name1": "val1"}, Values: []model.SamplePair{{Timestamp: testTime, Value: 2}}}}},
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
				cfg := ruleImporterConfig{
					outputDir:        tmpDir,
					start:            start.Add(-10 * time.Hour),
					end:              start.Add(-7 * time.Hour),
					evalInterval:     evalInterval,
					maxBlockDuration: tt.maxBlockDuration,
				}
				ruleImporter := newRuleImporter(log.NewNopLogger(), cfg, mockQueryRangeAPI{
					samples: tt.samples,
				})
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
				require.Equal(t, "grp0_rule1", gRules[0].Name())
				require.Equal(t, "ruleExpr", gRules[0].Query().String())
				require.Equal(t, 1, len(gRules[0].Labels()))

				group2 := ruleImporter.groups[path2+";group2"]
				require.NotNil(t, group2)
				require.Equal(t, defaultInterval*time.Second, group2.Interval())
				g2Rules := group2.Rules()
				require.Equal(t, 2, len(g2Rules))
				require.Equal(t, "grp2_rule1", g2Rules[0].Name())
				require.Equal(t, "grp2_rule1_expr", g2Rules[0].Query().String())
				require.Equal(t, 0, len(g2Rules[0].Labels()))
			}
		})
	}
}

func createSingleRuleTestFiles(path string) error {
	recordingRules := `groups:
- name: group0
  rules:
  - record: grp0_rule1
    expr:  ruleExpr
    labels:
        testlabel11: testlabelvalue11
`
	return ioutil.WriteFile(path, []byte(recordingRules), 0o777)
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
	return ioutil.WriteFile(path, []byte(recordingRules), 0o777)
}

func createMockStaleMetricsSingleSeries(ts time.Time) []*model.SampleStream {
	return []*model.SampleStream{
		{
			Metric: model.Metric{"metric1": "metric1val1"},
			Values: []model.SamplePair{
				{
					Timestamp: model.TimeFromUnixNano(ts.UnixNano()),
					Value:     1,
				},
				{
					Timestamp: model.TimeFromUnixNano(ts.Add(1 * evalInterval).UnixNano()),
					Value:     2,
				},
				// stale marker should be inserted between these 2 because the next sample is more than 1 step after
				{
					Timestamp: model.TimeFromUnixNano(ts.Add(5 * evalInterval).UnixNano()),
					Value:     3,
				},
				{
					Timestamp: model.TimeFromUnixNano(ts.Add(6 * evalInterval).UnixNano()),
					Value:     4,
				},
				{
					Timestamp: model.TimeFromUnixNano(ts.Add(7 * evalInterval).UnixNano()),
					Value:     5,
				},
				{
					// There should be a stale marker inserted before and after this sample because its in a new block more than 1 step away
					// and because it is the last sample and it occurs before the backfilling end time.
					Timestamp: model.TimeFromUnixNano(ts.Add(time.Duration(tsdb.DefaultBlockDuration)*time.Millisecond + 8*evalInterval).UnixNano()),
					Value:     6,
				},
			},
		},
	}
}

func TestBackfillRuleImporterStale(t *testing.T) {
	var (
		start    = time.Date(2009, time.November, 6, 1, 34, 0, 0, time.UTC)
		testTime = start.Add(1 * time.Hour)
	)

	var testCases = []struct {
		name            string
		wantBlockCount  int
		wantSeriesCount int
		wantSampleCount int
		wantStaleCount  int
		wantStaleOrder  []int
		samples         []*model.SampleStream
	}{
		{
			name:            "stale within 1 series",
			wantBlockCount:  2,
			wantSeriesCount: 1,
			wantSampleCount: 9,
			wantStaleCount:  3,
			wantStaleOrder:  []int{0, 0, 1, 0, 0, 0, 1, 0, 1},
			samples:         createMockStaleMetricsSingleSeries(testTime),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// setup test
			tmpDir, err := ioutil.TempDir("", "backfilldata")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(tmpDir))
			}()
			ctx := context.Background()

			cfg := ruleImporterConfig{
				outputDir:        tmpDir,
				start:            start,
				end:              start.Add(4 * time.Hour),
				evalInterval:     evalInterval,
				maxBlockDuration: defaultBlockDuration,
			}
			ruleImporter := newRuleImporter(log.NewNopLogger(), cfg, mockQueryRangeAPI{
				samples: tt.samples,
			})
			require.NoError(t, err)
			path1 := filepath.Join(tmpDir, "test.file")
			require.NoError(t, createSingleRuleTestFiles(path1))

			errs := ruleImporter.loadGroups(ctx, []string{path1})
			for _, err := range errs {
				require.NoError(t, err)
			}

			// Backfill recording rules then check the blocks to confirm the correct stale markers were created.
			errs = ruleImporter.importAll(ctx)
			for _, err := range errs {
				require.NoError(t, err)
			}

			opts := tsdb.DefaultOptions()
			opts.AllowOverlappingBlocks = true
			db, err := tsdb.Open(tmpDir, nil, nil, opts, nil)
			require.NoError(t, err)

			blocks := db.Blocks()
			require.GreaterOrEqual(t, len(blocks), tt.wantBlockCount)

			q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
			require.NoError(t, err)

			// Check that the correct staleNaNs are present
			selectedSeries := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
			var seriesCount, samplesCount, staleCount int
			for selectedSeries.Next() {
				seriesCount++
				series := selectedSeries.At()

				it := series.Iterator()
				actualStaleOrder := []int{}
				for it.Next() {
					samplesCount++
					_, v := it.At()
					if value.IsStaleNaN(v) {
						staleCount++
						actualStaleOrder = append(actualStaleOrder, 1)
					} else {
						actualStaleOrder = append(actualStaleOrder, 0)
					}
				}
				require.NoError(t, it.Err())
				require.NoError(t, selectedSeries.Err())
				require.Equal(t, tt.wantSeriesCount, seriesCount)
				require.Equal(t, tt.wantSampleCount, samplesCount)
				require.Equal(t, tt.wantStaleCount, staleCount)
				if len(tt.wantStaleOrder) != len(actualStaleOrder) {
					t.Fatalf("expected %d, actual %d", len(tt.wantStaleOrder), len(actualStaleOrder))
				}
				for i, expected := range tt.wantStaleOrder {
					if actualStaleOrder[i] != expected {
						t.Fatalf("expected %d, actual %d at position %d", expected, actualStaleOrder[i], i)
					}
				}
				require.NoError(t, q.Close())
				require.NoError(t, db.Close())
			}
		})
	}
}

// TestBackfillBuildLabels confirms that the labels in the rule file override the labels from the metrics
// received from Prometheus Query API, including the __name__ label.
func TestBackfillBuildLabels(t *testing.T) {
	queryMetric := model.Metric{"label1": "override-me", "__name__": "override-me-too", "label2": "dont-override"}
	ruleLabels := labels.Labels{
		labels.Label{Name: "__name__", Value: "rule-name"},
		labels.Label{Name: "label1", Value: "rule-label-override"},
		labels.Label{Name: "foo", Value: "bar"},
	}
	ruleName := "test"

	wantLabels := labels.Labels{
		labels.Label{Name: "__name__", Value: ruleName},
		labels.Label{Name: "foo", Value: "bar"},
		labels.Label{Name: "label1", Value: "rule-label-override"},
		labels.Label{Name: "label2", Value: "dont-override"},
	}
	gotLabels := buildLabels(queryMetric, ruleLabels, ruleName)

	if len(wantLabels) != len(gotLabels) {
		t.Fatalf("wanted %d, got %d", len(wantLabels), len(gotLabels))
	}

	for i, got := range gotLabels {
		if got != wantLabels[i] {
			t.Fatalf("wanted %v, got %v", wantLabels[i], got)
		}
	}
}
