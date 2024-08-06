// Copyright 2015 The Prometheus Authors
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

package promql_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestDeriv(t *testing.T) {
	// https://github.com/prometheus/prometheus/issues/2674#issuecomment-315439393
	// This requires more precision than the usual test system offers,
	// so we test it by hand.
	storage := teststorage.New(t)
	defer storage.Close()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10000,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(opts)

	a := storage.Appender(context.Background())

	var start, interval, i int64
	metric := labels.FromStrings("__name__", "foo")
	start = 1493712816939
	interval = 30 * 1000
	// Introduce some timestamp jitter to test 0 slope case.
	// https://github.com/prometheus/prometheus/issues/7180
	for i = 0; i < 15; i++ {
		jitter := 12 * i % 2
		a.Append(0, metric, start+interval*i+jitter, 1)
	}

	require.NoError(t, a.Commit())

	ctx := context.Background()
	query, err := engine.NewInstantQuery(ctx, storage, nil, "deriv(foo[30m])", timestamp.Time(1493712846939))
	require.NoError(t, err)

	result := query.Exec(ctx)
	require.NoError(t, result.Err)

	vec, _ := result.Vector()
	require.Len(t, vec, 1, "Expected 1 result, got %d", len(vec))
	require.Equal(t, 0.0, vec[0].F, "Expected 0.0 as value, got %f", vec[0].F)
}

func TestFunctionList(t *testing.T) {
	// Test that Functions and parser.Functions list the same functions.
	for i := range promql.FunctionCalls {
		_, ok := parser.Functions[i]
		require.True(t, ok, "function %s exists in promql package, but not in parser package", i)
	}

	for i := range parser.Functions {
		_, ok := promql.FunctionCalls[i]
		require.True(t, ok, "function %s exists in parser package, but not in promql package", i)
	}
}

func TestInfo(t *testing.T) {
	// Initialize and defer the closing of the test storage.
	testStorage := teststorage.New(t)
	defer testStorage.Close()

	// Define the time range and step interval for queries.
	start := time.Unix(0, 0)
	end := start.Add(2 * time.Hour)
	step := 30 * time.Second

	// Generate test series data.
	if err := generateInfoFunctionTestSeries(testStorage, 100, 2000, 3600); err != nil {
		t.Fatalf("failed to generate test series: %v", err)
	}

	// Set up PromQL engine options.
	opts := promql.EngineOpts{
		Logger:               nil,
		Reg:                  nil,
		MaxSamples:           50000000,
		Timeout:              100 * time.Second,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	}

	// Create a new PromQL engine.
	engine := promql.NewEngine(opts)

	// Define test cases for queries.
	testCases := []struct {
		joinQuery string
		infoQuery string
	}{
		{
			joinQuery: "rate(http_server_request_duration_seconds_count[2m]) * on (job, instance) group_left (k8s_cluster_name) target_info{k8s_cluster_name=\"us-east\"}",
			infoQuery: "info(rate(http_server_request_duration_seconds_count[2m]),{k8s_cluster_name=\"us-east\"})",
		},
		{
			joinQuery: "sum by (k8s_cluster_name, http_status_code) (rate(http_server_request_duration_seconds_count[2m]) * on (job, instance) group_left (k8s_cluster_name) target_info)",
			infoQuery: "sum by (k8s_cluster_name, http_status_code) (info(rate(http_server_request_duration_seconds_count[2m])))",
		},
	}

	// Iterate over test cases and execute queries.
	for _, tc := range testCases {
		// Execute join query.
		joinQueryResult, err := executeRangeQuery(engine, testStorage, tc.joinQuery, start, end, step)
		if err != nil {
			t.Fatalf("error executing join query '%s': %v", tc.joinQuery, err)
		}

		// Execute info query.
		infoQueryResult, err := executeRangeQuery(engine, testStorage, tc.infoQuery, start, end, step)
		if err != nil {
			t.Fatalf("error executing info query '%s': %v", tc.infoQuery, err)
		}

		// Compare results.
		require.Equal(t, joinQueryResult, infoQueryResult, "results for queries did not match")
	}
}

// Helper function to execute a query and return the result.
func executeRangeQuery(engine *promql.Engine, storage *teststorage.TestStorage, query string, start, end time.Time, step time.Duration) (promql.Result, error) {
	qry, err := engine.NewRangeQuery(context.Background(), storage, nil, query, start, end, step)
	if err != nil {
		return promql.Result{}, err
	}
	result := qry.Exec(context.Background())
	return *result, result.Err
}

// Helper function to generate test target_info and http_server_request_duration_seconds_count for the info function.
func generateInfoFunctionTestSeries(stor *teststorage.TestStorage, infoSeriesNum, interval, numIntervals int) error {
	ctx := context.Background()
	statusCodes := []string{"200", "400", "500"}

	// Generate target_info metrics with instance and job labels, and k8s_cluster_name label.
	// Generate http_server_request_duration_seconds_count metrics with instance and job labels, and http_status_code label.
	// the classic target_info metrics is gauge type.
	metrics := make([]labels.Labels, 0, infoSeriesNum+len(statusCodes))
	for j := 0; j < infoSeriesNum; j++ {
		clusterName := "us-east"
		if j >= infoSeriesNum/2 {
			clusterName = "eu-south"
		}
		metrics = append(metrics, labels.FromStrings(
			"__name__", "target_info",
			"instance", "instance"+strconv.Itoa(j),
			"job", "job"+strconv.Itoa(j),
			"k8s_cluster_name", clusterName,
		))
	}

	for _, statusCode := range statusCodes {
		metrics = append(metrics, labels.FromStrings(
			"__name__", "http_server_request_duration_seconds_count",
			"instance", "instance0",
			"job", "job0",
			"http_status_code", statusCode,
		))
	}

	// Append the generated metrics and samples to the storage.
	refs := make([]storage.SeriesRef, len(metrics))

	for s := 0; s < numIntervals; s++ {
		a := stor.Appender(context.Background())
		ts := int64(s * interval)
		for i, metric := range metrics[:infoSeriesNum] {
			ref, _ := a.Append(refs[i], metric, ts, 1)
			refs[i] = ref
		}

		for i, metric := range metrics[infoSeriesNum:] {
			ref, _ := a.Append(refs[i+infoSeriesNum], metric, ts, float64(s))
			refs[i+infoSeriesNum] = ref
		}

		if err := a.Commit(); err != nil {
			return err
		}
	}

	stor.DB.ForceHeadMMap() // Ensure we have at most one head chunk for every series.
	stor.DB.Compact(ctx)
	return nil
}
