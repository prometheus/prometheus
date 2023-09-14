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
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

var srcMap = map[string]string{
	"udf_mad_over_time": `
		// fmt := import("fmt")
		math := import("math")
		util := import("util")

		mad := func(seq) {
			median := util.median(seq)
			if is_error(median) {
				return median
			}
			deltas := []
			for x in seq {
				deltas = append(deltas, math.abs(x-median))
			}
			// fmt.println(deltas)
			return util.median(deltas)
		}

		output := mad(input0)
	`,
	// "udf_std_dev_simple_over_time": `
	// 	math := import("math")

	// 	std := func(seq) {
	// 		n := len(seq)
	// 		if n == 0 {
	// 			return error("cannot calculate mean of empty array")
	// 		}
	// 		sum := 0
	// 		for x in seq {
	// 			sum += x
	// 		}
	// 		mean := sum / n
	// 		variance := 0
	// 		for x in seq {
	// 			variance += math.pow(x-mean, 2)
	// 		}
	// 		variance /= n
	// 		return math.sqrt(variance)
	// 	}

	// 	output := std(input0)
	// `,
	"udf_std_dev_over_time": `
		math := import("math")

		std := func(seq) {
			count := 0
			mean := 0
			aux := 0
			for x in seq {
				count++
				delta := x - mean
				mean += delta/count
				aux += delta*(x-mean)
			}
			if count == 0 {
				return error("cannot calculate mean of empty array")
			}
			return math.sqrt(aux / count)
		}

		output := std(input0)
	`,
}

func init() {
	for name, src := range srcMap {
		inputTypes := []parser.ValueType{parser.ValueTypeMatrix}
		if err := config.ValidateUDFInputTypes(inputTypes); err != nil {
			panic(err)
		}
		err := addUDF(name, src, []string{"math", "rand"}, true, inputTypes)
		if err != nil {
			panic(err)
		}
	}
	for _, name := range []string{"mad_over_time", "std_dev_over_time"} {
		parser.AddFunction(name, []parser.ValueType{parser.ValueTypeMatrix})
	}
	FunctionCalls["mad_over_time"] = funcMadOverTime
	FunctionCalls["std_dev_over_time"] = funcStdDevOverTime
}

// duplicate of funcStddevOverTime that doesn't use kahan sum so it is comparable to funcUdfStdDevOverTime
// === std_dev_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcStdDevOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		var count float64
		var mean float64
		var aux float64
		for _, f := range s.Floats {
			count++
			delta := f.F - mean
			mean += delta / count
			aux += delta * (f.F - mean)
		}
		return math.Sqrt(aux / count)
	}), nil
}

// === mad_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMadOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		values := make(vectorByValueHeap, 0, len(s.Floats))
		for _, f := range s.Floats {
			values = append(values, Sample{F: f.F})
		}
		median := quantile(0.5, values)
		values = make(vectorByValueHeap, 0, len(s.Floats))
		for _, f := range s.Floats {
			values = append(values, Sample{F: math.Abs(f.F - median)})
		}
		return quantile(0.5, values)
	}), nil
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
