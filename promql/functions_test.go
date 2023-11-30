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

package promql

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestDeriv(t *testing.T) {
	// https://github.com/prometheus/prometheus/issues/2674#issuecomment-315439393
	// This requires more precision than the usual test system offers,
	// so we test it by hand.
	storage := teststorage.New(t)
	defer storage.Close()
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10000,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)

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
	require.Equal(t, 1, len(vec), "Expected 1 result, got %d", len(vec))
	require.Equal(t, 0.0, vec[0].F, "Expected 0.0 as value, got %f", vec[0].F)
}

func TestFunctionList(t *testing.T) {
	// Test that Functions and parser.Functions list the same functions.
	for i := range FunctionCalls {
		_, ok := parser.Functions[i]
		require.True(t, ok, "function %s exists in promql package, but not in parser package", i)
	}

	for i := range parser.Functions {
		_, ok := FunctionCalls[i]
		require.True(t, ok, "function %s exists in parser package, but not in promql package", i)
	}
}

func TestKahanSum(t *testing.T) {
	vals := []float64{1.0, math.Pow(10, 100), 1.0, -1 * math.Pow(10, 100)}
	expected := 2.0
	require.Equal(t, expected, kahanSum(vals))
}

func TestMadOverTime(t *testing.T) {
	cases := []struct {
		series      []int
		expectedRes float64
	}{
		{
			series:      []int{4, 6, 2, 1, 999, 1, 2},
			expectedRes: 1,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			engine := newTestEngine()
			storage := teststorage.New(t)
			t.Cleanup(func() { storage.Close() })

			seriesName := "float_series"

			ts := int64(0)
			app := storage.Appender(context.Background())
			lbls := labels.FromStrings("__name__", seriesName)
			var err error
			for _, num := range c.series {
				_, err = app.Append(0, lbls, ts, float64(num))
				require.NoError(t, err)
				ts += int64(1 * time.Minute / time.Millisecond)
			}
			require.NoError(t, app.Commit())

			queryAndCheck := func(queryString string, exp Vector) {
				qry, err := engine.NewInstantQuery(context.Background(), storage, nil, queryString, timestamp.Time(ts))
				require.NoError(t, err)

				res := qry.Exec(context.Background())
				require.NoError(t, res.Err)

				vector, err := res.Vector()
				require.NoError(t, err)

				require.Equal(t, exp, vector)
			}

			queryString := fmt.Sprintf(`mad_over_time(%s[%dm])`, seriesName, len(c.series))
			queryAndCheck(queryString, []Sample{{T: ts, F: c.expectedRes, Metric: labels.EmptyLabels()}})
		})
	}
}
