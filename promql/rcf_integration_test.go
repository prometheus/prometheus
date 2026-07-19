// Copyright The Prometheus Authors
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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/teststorage"
)

func newRCFTestEngine(t *testing.T) *promql.Engine {
	t.Helper()
	return promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		MaxSamples: 50000,
		Timeout:    30 * time.Second,
	})
}

// TestFuncRCFStatefulDetection demonstrates that the RCF model learns normal
// behaviour over time and assigns a higher anomaly score to a sudden spike:
//
//  1. Warm-up: 120 samples of a repeating 0–9 pattern → model learns "normal".
//  2. Score the last normal sample → low score.
//  3. Append a spike (value 1000) → score must be strictly higher.
//  4. rcf_attribution at the spike → exactly 6 output series, all non-negative.
//
// Uses a dedicated metric name to isolate from the global rcfModels store.
func TestFuncRCFStatefulDetection(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()

	metric := labels.FromStrings("__name__", "test_rcf_stateful", "job", "stateful")
	app := storage.Appender(context.Background())
	var lastNormalTS int64
	for i := range 120 {
		lastNormalTS = int64(i) * 15_000
		_, err := app.Append(0, metric, lastNormalTS, float64(i%10))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	engine := newRCFTestEngine(t)
	ctx := context.Background()

	q, err := engine.NewInstantQuery(ctx, storage, nil,
		`rcf(test_rcf_stateful{job="stateful"}[15m], 10, 32)`,
		time.UnixMilli(lastNormalTS))
	require.NoError(t, err)
	res := q.Exec(ctx)
	require.NoError(t, res.Err)
	vec, err := res.Vector()
	require.NoError(t, err)
	require.Len(t, vec, 1)
	normalScore := vec[0].F

	spikeTS := lastNormalTS + 15_000
	app2 := storage.Appender(ctx)
	_, err = app2.Append(0, metric, spikeTS, 1000.0)
	require.NoError(t, err)
	require.NoError(t, app2.Commit())

	q2, err := engine.NewInstantQuery(ctx, storage, nil,
		`rcf(test_rcf_stateful{job="stateful"}[15m], 10, 32)`,
		time.UnixMilli(spikeTS))
	require.NoError(t, err)
	res2 := q2.Exec(ctx)
	require.NoError(t, res2.Err)
	vec2, err := res2.Vector()
	require.NoError(t, err)
	require.Len(t, vec2, 1)
	spikeScore := vec2[0].F

	require.Greater(t, spikeScore, normalScore,
		"spike score (%v) should exceed normal score (%v) after warm-up", spikeScore, normalScore)

	// rcf_attribution must return exactly 6 series (one per feature dimension),
	// each with a non-negative value.
	q3, err := engine.NewInstantQuery(ctx, storage, nil,
		`rcf_attribution(test_rcf_stateful{job="stateful"}[15m], 10, 32)`,
		time.UnixMilli(spikeTS))
	require.NoError(t, err)
	res3 := q3.Exec(ctx)
	require.NoError(t, res3.Err)
	vec3, err := res3.Vector()
	require.NoError(t, err)
	require.Len(t, vec3, 6, "expected one output series per feature dimension")
	for _, s := range vec3 {
		require.GreaterOrEqual(t, s.F, 0.0, "attribution for dim %q must be non-negative", s.Metric.Get("rcf_dim"))
	}
}

// TestFuncRCFInvalidArgs verifies that invalid parameters produce empty results.
func TestFuncRCFInvalidArgs(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()

	app := storage.Appender(context.Background())
	metric := labels.FromStrings("__name__", "test_rcf_invalid", "job", "test")
	var lastTS int64
	for i := range 60 {
		lastTS = int64(i) * 15_000
		_, err := app.Append(0, metric, lastTS, float64(i%10))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	engine := newRCFTestEngine(t)
	ctx := context.Background()
	evalTime := time.UnixMilli(lastTS)

	for _, expr := range []string{
		`rcf(test_rcf_invalid{job="test"}[15m], 0)`,    // trees < 1
		`rcf(test_rcf_invalid{job="test"}[15m], 5, 1)`, // sample_size < 2
	} {
		q, err := engine.NewInstantQuery(ctx, storage, nil, expr, evalTime)
		require.NoError(t, err)
		res := q.Exec(ctx)
		require.NoError(t, res.Err)
		vec, _ := res.Vector()
		require.Empty(t, vec)
	}
}
