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
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/require"
)

const (
	testMaxSampleCount = 50
	testValue          = 123
)

var testTime = model.Time(time.Now().Add(-20 * time.Minute).Unix())

type mockQueryRangeAPI struct{}

func (mockAPI mockQueryRangeAPI) QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, v1.Warnings, error) {
	var testMatrix model.Matrix = []*model.SampleStream{
		{
			Metric: model.Metric{
				"name1": "val1",
			},
			Values: []model.SamplePair{
				{
					Timestamp: testTime,
					Value:     testValue,
				},
			},
		},
	}
	return testMatrix, v1.Warnings{}, nil
}

// TestBackfillRuleIntegration is an integration test that runs all the rule importer code to confirm the parts work together.
func TestBackfillRuleIntegration(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "backfilldata")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpDir))
	}()
	start := time.Now().UTC()
	ctx := context.Background()

	const (
		groupName  = "test_group_name"
		ruleName1  = "test_rule1_name"
		ruleExpr   = "test_expr"
		ruleLabels = "test_label_name: test_label_value"
	)

	// Execute test two times in a row to simulate running the rule importer twice with the same data.
	// We expect that duplicate blocks with the same series are created when run more than once.
	for i := 0; i < 2; i++ {

		ruleImporter, err := newTestRuleImporter(ctx, start, tmpDir)
		require.NoError(t, err)
		path := tmpDir + "/test.file"
		require.NoError(t, createTestFiles(groupName, ruleName1, ruleExpr, ruleLabels, path))

		// After loading/parsing the recording rule files make sure the parsing was correct.
		errs := ruleImporter.loadGroups(ctx, []string{path})
		for _, err := range errs {
			require.NoError(t, err)
		}
		const groupCount = 1
		require.Equal(t, groupCount, len(ruleImporter.groups))
		groupNameWithPath := path + ";" + groupName
		group1 := ruleImporter.groups[groupNameWithPath]
		require.NotNil(t, group1)
		const defaultInterval = 60
		require.Equal(t, time.Duration(defaultInterval*time.Second), group1.Interval())
		gRules := group1.Rules()
		const ruleCount = 1
		require.Equal(t, ruleCount, len(gRules))
		require.Equal(t, ruleName1, gRules[0].Name())
		require.Equal(t, ruleExpr, gRules[0].Query().String())
		require.Equal(t, 1, len(gRules[0].Labels()))

		// Backfill all recording rules then check the blocks to confirm the right
		// data was created.
		errs = ruleImporter.importAll(ctx)
		for _, err := range errs {
			require.NoError(t, err)
		}

		opts := tsdb.DefaultOptions()
		opts.AllowOverlappingBlocks = true
		db, err := tsdb.Open(tmpDir, nil, nil, opts)
		require.NoError(t, err)

		blocks := db.Blocks()
		require.Equal(t, i+1, len(blocks))

		q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		selectedSeries := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
		var seriesCount, samplesCount int
		for selectedSeries.Next() {
			seriesCount++
			series := selectedSeries.At()
			require.Equal(t, 3, len(series.Labels()))
			it := series.Iterator()
			for it.Next() {
				samplesCount++
				ts, v := it.At()
				require.Equal(t, float64(testValue), v)
				require.Equal(t, int64(testTime), ts)
			}
			require.NoError(t, it.Err())
		}
		require.NoError(t, selectedSeries.Err())
		require.Equal(t, 1, seriesCount)
		require.Equal(t, 1, samplesCount)
		require.NoError(t, q.Close())
		require.NoError(t, db.Close())
	}

}

func newTestRuleImporter(ctx context.Context, start time.Time, tmpDir string) (*ruleImporter, error) {
	logger := log.NewNopLogger()
	cfg := ruleImporterConfig{
		Start:        start.Add(-1 * time.Hour),
		End:          start,
		EvalInterval: 60 * time.Second,
	}
	writer, err := tsdb.NewBlockWriter(logger,
		tmpDir,
		tsdb.DefaultBlockDuration,
	)
	if err != nil {
		return nil, err
	}

	app := newMultipleAppender(ctx, testMaxSampleCount, writer)
	return newRuleImporter(logger, cfg, mockQueryRangeAPI{}, app), nil
}

func createTestFiles(groupName, ruleName, ruleExpr, ruleLabels, path string) error {
	recordingRules := fmt.Sprintf(`groups:
- name: %s
  rules:
  - record: %s
    expr: %s
    labels:
      %s
`,
		groupName, ruleName, ruleExpr, ruleLabels,
	)
	return ioutil.WriteFile(path, []byte(recordingRules), 0777)
}
