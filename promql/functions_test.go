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
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	storage2 "github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestDeriv(t *testing.T) {
	// https://github.com/prometheus/prometheus/issues/2674#issuecomment-315439393
	// This requires more precision than the usual test system offers,
	// so we test it by hand.
	storage := teststorage.New(t)

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10000,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	a := storage.Appender(context.Background())

	var start, interval, i int64
	metric := labels.FromStrings("__name__", "foo")
	start = 1493712816939
	interval = 30 * 1000
	// Introduce some timestamp jitter to test 0 slope case.
	// https://github.com/prometheus/prometheus/issues/7180
	for i = range int64(15) {
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

func TestStartTimestampOutputWhenUseStartTimestampIsDisabled(t *testing.T) {
	storage := teststorage.New(t, func(opts *tsdb.Options) {
		opts.XOR2EncodingAllowed = true
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
	})

	a := storage.AppenderV2(t.Context())

	for i := range int64(5) {
		inputLabel := labels.FromStrings(model.MetricNameLabel, "some_series", "case", strconv.Itoa(int(i)))
		var (
			ts = i * 1000
			st = ts - i*100
		)
		_, err := a.Append(0, inputLabel, st, ts, 0, nil, nil, storage2.AppendV2Options{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())

	tests := []struct {
		name               string
		useStartTimestamps bool
		expected           []float64
	}{
		{
			name:               "use-start-timestamps enabled",
			useStartTimestamps: true,
			expected:           []float64{0, 0.9, 1.8, 2.7, 3.6},
		},
		{
			name:               "use-start-timestamps disabled",
			useStartTimestamps: false,
			expected:           []float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := promql.EngineOpts{
				MaxSamples:         10000,
				Timeout:            10 * time.Second,
				UseStartTimestamps: tt.useStartTimestamps,
				Parser:             parser.NewParser(promqltest.TestParserOpts),
			}
			engine := promqltest.NewTestEngineWithOpts(t, opts)

			query, err := engine.NewInstantQuery(t.Context(), storage, nil, "start_timestamp(some_series)", timestamp.Time(5000))
			require.NoError(t, err)

			result := query.Exec(t.Context())
			require.NoError(t, result.Err)

			vec, _ := result.Vector()
			require.Len(t, vec, len(tt.expected), "Unexpected number of results, got %d", len(vec))
			for i := range tt.expected {
				require.Equal(t, tt.expected[i], vec[i].F, "At index %d", i)
			}
		})
	}
}

func TestIgnoreStartTimesFunction(t *testing.T) {
	storage := teststorage.New(t, func(opts *tsdb.Options) {
		opts.XOR2EncodingAllowed = true
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
	})

	a := storage.AppenderV2(t.Context())

	// Insert data with start timestamps
	for i := range int64(5) {
		inputLabel := labels.FromStrings(model.MetricNameLabel, "some_series", "case", strconv.Itoa(int(i)))
		var (
			ts = i * 1000
			st = ts - i*100
		)
		_, err := a.Append(0, inputLabel, st, ts, float64(i*10), nil, nil, storage2.AppendV2Options{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())

	opts := promql.EngineOpts{
		MaxSamples:         10000,
		Timeout:            10 * time.Second,
		UseStartTimestamps: true, // Enable start timestamps
		Parser:             parser.NewParser(promqltest.TestParserOpts),
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	// ignore_start_times should return raw samples without start timestamp processing
	// Use a range query since the function expects a Matrix (range vector)
	query, err := engine.NewRangeQuery(t.Context(), storage, nil, "ignore_start_times(some_series[1m])", timestamp.Time(0), timestamp.Time(5000), time.Second)
	require.NoError(t, err)

	result := query.Exec(t.Context())
	require.NoError(t, result.Err)

	matrix, _ := result.Matrix()
	require.Len(t, matrix, 5, "Expected 5 series, got %d", len(matrix))
	// Values should be 0, 10, 20, 30, 40 (raw values, not start timestamps)
	for i := range 5 {
		require.Len(t, matrix[i].Floats, 1, "Expected 1 sample per series")
		require.Equal(t, float64(i*10), matrix[i].Floats[0].F, "At index %d", i)
	}
}

func TestIgnoreStartTimesWithRate(t *testing.T) {
	storage := teststorage.New(t, func(opts *tsdb.Options) {
		opts.XOR2EncodingAllowed = true
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
	})

	a := storage.AppenderV2(t.Context())

	// Insert cumulative counter with start timestamps that indicate resets
	for i := range int64(10) {
		inputLabel := labels.FromStrings(model.MetricNameLabel, "counter")
		var (
			ts  = i * 1000
			st  = ts - 500 // ST indicates reset every sample
			val = float64(i * 100)
		)
		_, err := a.Append(0, inputLabel, st, ts, val, nil, nil, storage2.AppendV2Options{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())

	// Test with use-start-timestamps enabled but ignore_start_times wrapper
	opts := promql.EngineOpts{
		MaxSamples:         10000,
		Timeout:            10 * time.Second,
		UseStartTimestamps: true,
		Parser:             parser.NewParser(promqltest.TestParserOpts),
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	// With ignore_start_times, rate should behave as if start timestamps are disabled
	query, err := engine.NewRangeQuery(t.Context(), nil, nil, "rate(ignore_start_times(counter[5m]))", timestamp.Time(0), timestamp.Time(9000), time.Second)
	require.NoError(t, err)

	result := query.Exec(t.Context())
	require.NoError(t, result.Err)

	matrix, _ := result.Matrix()
	require.Len(t, matrix, 1, "Expected 1 series")

	// With start timestamps disabled, rate should be constant (100 per second = 1)
	// Each step is 1s, values increase by 100, so rate = 100/1000 = 0.1 per ms = 100 per second
	// Actually with 1s steps and values 0,100,200,300... the increase per step is 100, over 1s = 100/s
	for _, sample := range matrix[0].Floats {
		require.InDelta(t, 100.0, sample.F, 1.0, "Rate should be ~100 when start timestamps are ignored")
	}
}
