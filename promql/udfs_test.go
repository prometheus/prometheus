// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/util/teststorage"
)

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

			queryString = fmt.Sprintf(`udf_mad_over_time(%s[%dm])`, seriesName, len(c.series))
			queryAndCheck(queryString, []Sample{{T: ts, F: c.expectedRes, Metric: labels.EmptyLabels()}})
		})
	}
}

func compareAggsOverTime(t *testing.T, aggName string) {
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("iteration %d", i), func(t *testing.T) {
			engine := newTestEngine()
			storage := teststorage.New(t)
			t.Cleanup(func() { storage.Close() })

			seriesName := "float_series"

			ts := int64(0)
			app := storage.Appender(context.Background())
			lbls := labels.FromStrings("__name__", seriesName)
			n := 100
			var err error
			for j := 0; j < n; j++ {
				num := rand.Float64() * 1000
				_, err = app.Append(0, lbls, ts, num)
				require.NoError(t, err)
				ts += int64(1 * time.Minute / time.Millisecond)
			}
			require.NoError(t, app.Commit())

			queryAndCheck := func(queryString, queryString2 string) {
				qry, err := engine.NewInstantQuery(context.Background(), storage, nil, queryString, timestamp.Time(ts))
				require.NoError(t, err)

				res := qry.Exec(context.Background())
				require.NoError(t, res.Err)

				vector, err := res.Vector()
				require.NoError(t, err)

				qry, err = engine.NewInstantQuery(context.Background(), storage, nil, queryString2, timestamp.Time(ts))
				require.NoError(t, err)

				res = qry.Exec(context.Background())
				require.NoError(t, res.Err)

				vector2, err := res.Vector()
				require.NoError(t, err)

				require.Equal(t, vector[0].F, vector2[0].F)
				// require.InEpsilon(t, vector[0].F, vector2[0].F, 1e-12)
			}

			queryString := fmt.Sprintf(`%s_over_time(%s[%dm])`, aggName, seriesName, n)
			queryString2 := fmt.Sprintf(`udf_%s_over_time(%s[%dm])`, aggName, seriesName, n)
			queryAndCheck(queryString, queryString2)
		})
	}
}

func TestMadOverTimeComparison(t *testing.T) {
	compareAggsOverTime(t, "mad")
}

func TestStdDevOverTimeComparison(t *testing.T) {
	compareAggsOverTime(t, "std_dev")
}

func benchmarkAggOverTime(b *testing.B, aggName string) {
	engine := newTestEngine()
	storage := teststorage.New(b)
	b.Cleanup(func() { storage.Close() })

	seriesName := "float_series"

	ts := int64(0)
	app := storage.Appender(context.Background())
	lbls := labels.FromStrings("__name__", seriesName)
	n := 100
	var err error
	for j := 0; j < n; j++ {
		num := rand.Float64() * 1000
		_, err = app.Append(0, lbls, ts, num)
		require.NoError(b, err)
		ts += int64(1 * time.Minute / time.Millisecond)
	}
	require.NoError(b, app.Commit())

	query := func(queryString string) {
		qry, _ := engine.NewInstantQuery(context.Background(), storage, nil, queryString, timestamp.Time(ts))
		res := qry.Exec(context.Background())
		res.Vector()
	}

	queryString := fmt.Sprintf(`%s_over_time(%s[%dm])`, aggName, seriesName, n)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query(queryString)
	}
}

func BenchmarkMadOverTime(b *testing.B) {
	benchmarkAggOverTime(b, "mad")
}

func BenchmarkStdDevOverTime(b *testing.B) {
	benchmarkAggOverTime(b, "std_dev")
}
