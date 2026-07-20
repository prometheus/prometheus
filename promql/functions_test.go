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
				require.EqualValues(t, tt.expected[i], vec[i].F, "At index %d", i)
			}
		})
	}
}
