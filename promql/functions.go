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
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
)

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	Name       string
	ArgTypes   []ValueType
	Variadic   int
	ReturnType ValueType
	Call       func(ev *evaluator, args Expressions) Value
}

// === time() float64 ===
func funcTime(ev *evaluator, args Expressions) Value {
	return Scalar{
		V: float64(ev.Timestamp) / 1000,
		T: ev.Timestamp,
	}
}

// extrapolatedRate is a utility function for rate/increase/delta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// extrapolates if the first/last sample is close to the boundary, and returns
// the result as either per-second (if isRate is true) or overall.
func extrapolatedRate(ev *evaluator, arg Expr, isCounter bool, isRate bool) Value {
	ms := arg.(*MatrixSelector)

	var (
		matrix       = ev.evalMatrix(ms)
		rangeStart   = ev.Timestamp - durationMilliseconds(ms.Range+ms.Offset)
		rangeEnd     = ev.Timestamp - durationMilliseconds(ms.Offset)
		resultVector = make(Vector, 0, len(matrix))
	)

	for _, samples := range matrix {
		// No sense in trying to compute a rate without at least two points. Drop
		// this Vector element.
		if len(samples.Points) < 2 {
			continue
		}
		var (
			counterCorrection float64
			lastValue         float64
		)
		for _, sample := range samples.Points {
			if isCounter && sample.V < lastValue {
				counterCorrection += lastValue
			}
			lastValue = sample.V
		}
		resultValue := lastValue - samples.Points[0].V + counterCorrection

		// Duration between first/last samples and boundary of range.
		durationToStart := float64(samples.Points[0].T-rangeStart) / 1000
		durationToEnd := float64(rangeEnd-samples.Points[len(samples.Points)-1].T) / 1000

		sampledInterval := float64(samples.Points[len(samples.Points)-1].T-samples.Points[0].T) / 1000
		averageDurationBetweenSamples := sampledInterval / float64(len(samples.Points)-1)

		if isCounter && resultValue > 0 && samples.Points[0].V >= 0 {
			// Counters cannot be negative. If we have any slope at
			// all (i.e. resultValue went up), we can extrapolate
			// the zero point of the counter. If the duration to the
			// zero point is shorter than the durationToStart, we
			// take the zero point as the start of the series,
			// thereby avoiding extrapolation to negative counter
			// values.
			durationToZero := sampledInterval * (samples.Points[0].V / resultValue)
			if durationToZero < durationToStart {
				durationToStart = durationToZero
			}
		}

		// If the first/last samples are close to the boundaries of the range,
		// extrapolate the result. This is as we expect that another sample
		// will exist given the spacing between samples we've seen thus far,
		// with an allowance for noise.
		extrapolationThreshold := averageDurationBetweenSamples * 1.1
		extrapolateToInterval := sampledInterval

		if durationToStart < extrapolationThreshold {
			extrapolateToInterval += durationToStart
		} else {
			extrapolateToInterval += averageDurationBetweenSamples / 2
		}
		if durationToEnd < extrapolationThreshold {
			extrapolateToInterval += durationToEnd
		} else {
			extrapolateToInterval += averageDurationBetweenSamples / 2
		}
		resultValue = resultValue * (extrapolateToInterval / sampledInterval)
		if isRate {
			resultValue = resultValue / ms.Range.Seconds()
		}

		resultVector = append(resultVector, Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: resultValue, T: ev.Timestamp},
		})
	}
	return resultVector
}

// === delta(Matrix ValueTypeMatrix) Vector ===
func funcDelta(ev *evaluator, args Expressions) Value {
	return extrapolatedRate(ev, args[0], false, false)
}

// === rate(node ValueTypeMatrix) Vector ===
func funcRate(ev *evaluator, args Expressions) Value {
	return extrapolatedRate(ev, args[0], true, true)
}

// === increase(node ValueTypeMatrix) Vector ===
func funcIncrease(ev *evaluator, args Expressions) Value {
	return extrapolatedRate(ev, args[0], true, false)
}

// === irate(node ValueTypeMatrix) Vector ===
func funcIrate(ev *evaluator, args Expressions) Value {
	return instantValue(ev, args[0], true)
}

// === idelta(node model.ValMatric) Vector ===
func funcIdelta(ev *evaluator, args Expressions) Value {
	return instantValue(ev, args[0], false)
}

func instantValue(ev *evaluator, arg Expr, isRate bool) Value {
	resultVector := Vector{}
	for _, samples := range ev.evalMatrix(arg) {
		// No sense in trying to compute a rate without at least two points. Drop
		// this Vector element.
		if len(samples.Points) < 2 {
			continue
		}

		lastSample := samples.Points[len(samples.Points)-1]
		previousSample := samples.Points[len(samples.Points)-2]

		var resultValue float64
		if isRate && lastSample.V < previousSample.V {
			// Counter reset.
			resultValue = lastSample.V
		} else {
			resultValue = lastSample.V - previousSample.V
		}

		sampledInterval := lastSample.T - previousSample.T
		if sampledInterval == 0 {
			// Avoid dividing by 0.
			continue
		}

		if isRate {
			// Convert to per-second.
			resultValue /= float64(sampledInterval) / 1000
		}

		resultVector = append(resultVector, Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: resultValue, T: ev.Timestamp},
		})
	}
	return resultVector
}

// Calculate the trend value at the given index i in raw data d.
// This is somewhat analogous to the slope of the trend at the given index.
// The argument "s" is the set of computed smoothed values.
// The argument "b" is the set of computed trend factors.
// The argument "d" is the set of raw input values.
func calcTrendValue(i int, sf, tf float64, s, b, d []float64) float64 {
	if i == 0 {
		return b[0]
	}

	x := tf * (s[i] - s[i-1])
	y := (1 - tf) * b[i-1]

	// Cache the computed value.
	b[i] = x + y

	return b[i]
}

// Holt-Winters is similar to a weighted moving average, where historical data has exponentially less influence on the current data.
// Holt-Winter also accounts for trends in data. The smoothing factor (0 < sf < 1) affects how historical data will affect the current
// data. A lower smoothing factor increases the influence of historical data. The trend factor (0 < tf < 1) affects
// how trends in historical data will affect the current data. A higher trend factor increases the influence.
// of trends. Algorithm taken from https://en.wikipedia.org/wiki/Exponential_smoothing titled: "Double exponential smoothing".
func funcHoltWinters(ev *evaluator, args Expressions) Value {
	mat := ev.evalMatrix(args[0])

	// The smoothing factor argument.
	sf := ev.evalFloat(args[1])

	// The trend factor argument.
	tf := ev.evalFloat(args[2])

	// Sanity check the input.
	if sf <= 0 || sf >= 1 {
		ev.errorf("invalid smoothing factor. Expected: 0 < sf < 1 goT: %f", sf)
	}
	if tf <= 0 || tf >= 1 {
		ev.errorf("invalid trend factor. Expected: 0 < tf < 1 goT: %f", sf)
	}

	// Make an output Vector large enough to hold the entire result.
	resultVector := make(Vector, 0, len(mat))

	// Create scratch values.
	var s, b, d []float64

	var l int
	for _, samples := range mat {
		l = len(samples.Points)

		// Can't do the smoothing operation with less than two points.
		if l < 2 {
			continue
		}

		// Resize scratch values.
		if l != len(s) {
			s = make([]float64, l)
			b = make([]float64, l)
			d = make([]float64, l)
		}

		// Fill in the d values with the raw values from the input.
		for i, v := range samples.Points {
			d[i] = v.V
		}

		// Set initial values.
		s[0] = d[0]
		b[0] = d[1] - d[0]

		// Run the smoothing operation.
		var x, y float64
		for i := 1; i < len(d); i++ {

			// Scale the raw value against the smoothing factor.
			x = sf * d[i]

			// Scale the last smoothed value with the trend at this point.
			y = (1 - sf) * (s[i-1] + calcTrendValue(i-1, sf, tf, s, b, d))

			s[i] = x + y
		}

		resultVector = append(resultVector, Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: s[len(s)-1], T: ev.Timestamp}, // The last value in the Vector is the smoothed result.
		})
	}

	return resultVector
}

// === sort(node ValueTypeVector) Vector ===
func funcSort(ev *evaluator, args Expressions) Value {
	// NaN should sort to the bottom, so take descending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByReverseValueHeap(ev.evalVector(args[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter)
}

// === sortDesc(node ValueTypeVector) Vector ===
func funcSortDesc(ev *evaluator, args Expressions) Value {
	// NaN should sort to the bottom, so take ascending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByValueHeap(ev.evalVector(args[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter)
}

// === clamp_max(Vector ValueTypeVector, max Scalar) Vector ===
func funcClampMax(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	max := ev.evalFloat(args[1])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Min(max, float64(el.V))
	}
	return vec
}

// === clamp_min(Vector ValueTypeVector, min Scalar) Vector ===
func funcClampMin(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	min := ev.evalFloat(args[1])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Max(min, float64(el.V))
	}
	return vec
}

// === round(Vector ValueTypeVector, toNearest=1 Scalar) Vector ===
func funcRound(ev *evaluator, args Expressions) Value {
	// round returns a number rounded to toNearest.
	// Ties are solved by rounding up.
	toNearest := float64(1)
	if len(args) >= 2 {
		toNearest = ev.evalFloat(args[1])
	}
	// Invert as it seems to cause fewer floating point accuracy issues.
	toNearestInverse := 1.0 / toNearest

	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Floor(float64(el.V)*toNearestInverse+0.5) / toNearestInverse
	}
	return vec
}

// === Scalar(node ValueTypeVector) Scalar ===
func funcScalar(ev *evaluator, args Expressions) Value {
	v := ev.evalVector(args[0])
	if len(v) != 1 {
		return Scalar{
			V: math.NaN(),
			T: ev.Timestamp,
		}
	}
	return Scalar{
		V: v[0].V,
		T: ev.Timestamp,
	}
}

func aggrOverTime(ev *evaluator, args Expressions, aggrFn func([]Point) float64) Value {
	mat := ev.evalMatrix(args[0])
	resultVector := Vector{}

	for _, el := range mat {
		if len(el.Points) == 0 {
			continue
		}

		resultVector = append(resultVector, Sample{
			Metric: dropMetricName(el.Metric),
			Point:  Point{V: aggrFn(el.Points), T: ev.Timestamp},
		})
	}
	return resultVector
}

// === avg_over_time(Matrix ValueTypeMatrix) Vector ===
func funcAvgOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		var sum float64
		for _, v := range values {
			sum += v.V
		}
		return sum / float64(len(values))
	})
}

// === count_over_time(Matrix ValueTypeMatrix) Vector ===
func funcCountOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		return float64(len(values))
	})
}

// === floor(Vector ValueTypeVector) Vector ===
func funcFloor(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Floor(float64(el.V))
	}
	return vec
}

// === max_over_time(Matrix ValueTypeMatrix) Vector ===
func funcMaxOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		max := math.Inf(-1)
		for _, v := range values {
			max = math.Max(max, float64(v.V))
		}
		return max
	})
}

// === min_over_time(Matrix ValueTypeMatrix) Vector ===
func funcMinOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		min := math.Inf(1)
		for _, v := range values {
			min = math.Min(min, float64(v.V))
		}
		return min
	})
}

// === sum_over_time(Matrix ValueTypeMatrix) Vector ===
func funcSumOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		var sum float64
		for _, v := range values {
			sum += v.V
		}
		return sum
	})
}

// === quantile_over_time(Matrix ValueTypeMatrix) Vector ===
func funcQuantileOverTime(ev *evaluator, args Expressions) Value {
	q := ev.evalFloat(args[0])
	mat := ev.evalMatrix(args[1])
	resultVector := Vector{}

	for _, el := range mat {
		if len(el.Points) == 0 {
			continue
		}

		el.Metric = dropMetricName(el.Metric)
		values := make(vectorByValueHeap, 0, len(el.Points))
		for _, v := range el.Points {
			values = append(values, Sample{Point: Point{V: v.V}})
		}
		resultVector = append(resultVector, Sample{
			Metric: el.Metric,
			Point:  Point{V: quantile(q, values), T: ev.Timestamp},
		})
	}
	return resultVector
}

// === stddev_over_time(Matrix ValueTypeMatrix) Vector ===
func funcStddevOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		var sum, squaredSum, count float64
		for _, v := range values {
			sum += v.V
			squaredSum += v.V * v.V
			count++
		}
		avg := sum / count
		return math.Sqrt(float64(squaredSum/count - avg*avg))
	})
}

// === stdvar_over_time(Matrix ValueTypeMatrix) Vector ===
func funcStdvarOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values []Point) float64 {
		var sum, squaredSum, count float64
		for _, v := range values {
			sum += v.V
			squaredSum += v.V * v.V
			count++
		}
		avg := sum / count
		return squaredSum/count - avg*avg
	})
}

// === abs(Vector ValueTypeVector) Vector ===
func funcAbs(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Abs(float64(el.V))
	}
	return vec
}

// === absent(Vector ValueTypeVector) Vector ===
func funcAbsent(ev *evaluator, args Expressions) Value {
	if len(ev.evalVector(args[0])) > 0 {
		return Vector{}
	}
	m := []labels.Label{}

	if vs, ok := args[0].(*VectorSelector); ok {
		for _, ma := range vs.LabelMatchers {
			if ma.Type == labels.MatchEqual && ma.Name != labels.MetricName {
				m = append(m, labels.Label{Name: ma.Name, Value: ma.Value})
			}
		}
	}
	return Vector{
		Sample{
			Metric: labels.New(m...),
			Point:  Point{V: 1, T: ev.Timestamp},
		},
	}
}

// === ceil(Vector ValueTypeVector) Vector ===
func funcCeil(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Ceil(float64(el.V))
	}
	return vec
}

// === exp(Vector ValueTypeVector) Vector ===
func funcExp(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Exp(float64(el.V))
	}
	return vec
}

// === sqrt(Vector VectorNode) Vector ===
func funcSqrt(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Sqrt(float64(el.V))
	}
	return vec
}

// === ln(Vector ValueTypeVector) Vector ===
func funcLn(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Log(float64(el.V))
	}
	return vec
}

// === log2(Vector ValueTypeVector) Vector ===
func funcLog2(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Log2(float64(el.V))
	}
	return vec
}

// === log10(Vector ValueTypeVector) Vector ===
func funcLog10(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = math.Log10(float64(el.V))
	}
	return vec
}

// === timestamp(Vector ValueTypeVector) Vector ===
func funcTimestamp(ev *evaluator, args Expressions) Value {
	vec := ev.evalVector(args[0])
	for i := range vec {
		el := &vec[i]

		el.Metric = dropMetricName(el.Metric)
		el.V = float64(el.T) / 1000.0
	}
	return vec
}

// linearRegression performs a least-square linear regression analysis on the
// provided SamplePairs. It returns the slope, and the intercept value at the
// provided time.
func linearRegression(samples []Point, interceptTime int64) (slope, intercept float64) {
	var (
		n            float64
		sumX, sumY   float64
		sumXY, sumX2 float64
	)
	for _, sample := range samples {
		x := float64(sample.T-interceptTime) / 1e3
		n += 1.0
		sumY += sample.V
		sumX += x
		sumXY += x * sample.V
		sumX2 += x * x
	}
	covXY := sumXY - sumX*sumY/n
	varX := sumX2 - sumX*sumX/n

	slope = covXY / varX
	intercept = sumY/n - slope*sumX/n
	return slope, intercept
}

// === deriv(node ValueTypeMatrix) Vector ===
func funcDeriv(ev *evaluator, args Expressions) Value {
	mat := ev.evalMatrix(args[0])
	resultVector := make(Vector, 0, len(mat))

	for _, samples := range mat {
		// No sense in trying to compute a derivative without at least two points.
		// Drop this Vector element.
		if len(samples.Points) < 2 {
			continue
		}

		// We pass in an arbitrary timestamp that is near the values in use
		// to avoid floating point accuracy issues, see
		// https://github.com/prometheus/prometheus/issues/2674
		slope, _ := linearRegression(samples.Points, samples.Points[0].T)
		resultSample := Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: slope, T: ev.Timestamp},
		}

		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === predict_linear(node ValueTypeMatrix, k ValueTypeScalar) Vector ===
func funcPredictLinear(ev *evaluator, args Expressions) Value {
	mat := ev.evalMatrix(args[0])
	resultVector := make(Vector, 0, len(mat))
	duration := ev.evalFloat(args[1])

	for _, samples := range mat {
		// No sense in trying to predict anything without at least two points.
		// Drop this Vector element.
		if len(samples.Points) < 2 {
			continue
		}
		slope, intercept := linearRegression(samples.Points, ev.Timestamp)

		resultVector = append(resultVector, Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: slope*duration + intercept, T: ev.Timestamp},
		})
	}
	return resultVector
}

// === histogram_quantile(k ValueTypeScalar, Vector ValueTypeVector) Vector ===
func funcHistogramQuantile(ev *evaluator, args Expressions) Value {
	q := ev.evalFloat(args[0])
	inVec := ev.evalVector(args[1])

	outVec := Vector{}
	signatureToMetricWithBuckets := map[uint64]*metricWithBuckets{}
	for _, el := range inVec {
		upperBound, err := strconv.ParseFloat(
			el.Metric.Get(model.BucketLabel), 64,
		)
		if err != nil {
			// Oops, no bucket label or malformed label value. Skip.
			// TODO(beorn7): Issue a warning somehow.
			continue
		}
		hash := hashWithoutLabels(el.Metric, excludedLabels...)

		mb, ok := signatureToMetricWithBuckets[hash]
		if !ok {
			el.Metric = labels.NewBuilder(el.Metric).
				Del(labels.BucketLabel, labels.MetricName).
				Labels()

			mb = &metricWithBuckets{el.Metric, nil}
			signatureToMetricWithBuckets[hash] = mb
		}
		mb.buckets = append(mb.buckets, bucket{upperBound, el.V})
	}

	for _, mb := range signatureToMetricWithBuckets {
		outVec = append(outVec, Sample{
			Metric: mb.metric,
			Point:  Point{V: bucketQuantile(q, mb.buckets), T: ev.Timestamp},
		})
	}

	return outVec
}

// === resets(Matrix ValueTypeMatrix) Vector ===
func funcResets(ev *evaluator, args Expressions) Value {
	in := ev.evalMatrix(args[0])
	out := make(Vector, 0, len(in))

	for _, samples := range in {
		resets := 0
		prev := samples.Points[0].V
		for _, sample := range samples.Points[1:] {
			current := sample.V
			if current < prev {
				resets++
			}
			prev = current
		}

		out = append(out, Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: float64(resets), T: ev.Timestamp},
		})
	}
	return out
}

// === changes(Matrix ValueTypeMatrix) Vector ===
func funcChanges(ev *evaluator, args Expressions) Value {
	in := ev.evalMatrix(args[0])
	out := make(Vector, 0, len(in))

	for _, samples := range in {
		changes := 0
		prev := samples.Points[0].V
		for _, sample := range samples.Points[1:] {
			current := sample.V
			if current != prev && !(math.IsNaN(float64(current)) && math.IsNaN(float64(prev))) {
				changes++
			}
			prev = current
		}

		out = append(out, Sample{
			Metric: dropMetricName(samples.Metric),
			Point:  Point{V: float64(changes), T: ev.Timestamp},
		})
	}
	return out
}

// === label_replace(Vector ValueTypeVector, dst_label, replacement, src_labelname, regex ValueTypeString) Vector ===
func funcLabelReplace(ev *evaluator, args Expressions) Value {
	var (
		vector   = ev.evalVector(args[0])
		dst      = ev.evalString(args[1]).V
		repl     = ev.evalString(args[2]).V
		src      = ev.evalString(args[3]).V
		regexStr = ev.evalString(args[4]).V
	)

	regex, err := regexp.Compile("^(?:" + regexStr + ")$")
	if err != nil {
		ev.errorf("invalid regular expression in label_replace(): %s", regexStr)
	}
	if !model.LabelNameRE.MatchString(string(dst)) {
		ev.errorf("invalid destination label name in label_replace(): %s", dst)
	}

	outSet := make(map[uint64]struct{}, len(vector))
	for i := range vector {
		el := &vector[i]

		srcVal := el.Metric.Get(src)
		indexes := regex.FindStringSubmatchIndex(srcVal)
		// If there is no match, no replacement should take place.
		if indexes == nil {
			continue
		}
		res := regex.ExpandString([]byte{}, repl, srcVal, indexes)

		lb := labels.NewBuilder(el.Metric).Del(dst)
		if len(res) > 0 {
			lb.Set(dst, string(res))
		}
		el.Metric = lb.Labels()

		h := el.Metric.Hash()
		if _, ok := outSet[h]; ok {
			ev.errorf("duplicated label set in output of label_replace(): %s", el.Metric)
		} else {
			outSet[h] = struct{}{}
		}
	}

	return vector
}

// === Vector(s Scalar) Vector ===
func funcVector(ev *evaluator, args Expressions) Value {
	return Vector{
		Sample{
			Metric: labels.Labels{},
			Point:  Point{V: ev.evalFloat(args[0]), T: ev.Timestamp},
		},
	}
}

// === label_join(vector model.ValVector, dest_labelname, separator, src_labelname...) Vector ===
func funcLabelJoin(ev *evaluator, args Expressions) Value {
	var (
		vector    = ev.evalVector(args[0])
		dst       = ev.evalString(args[1]).V
		sep       = ev.evalString(args[2]).V
		srcLabels = make([]string, len(args)-3)
	)
	for i := 3; i < len(args); i++ {
		src := ev.evalString(args[i]).V
		if !model.LabelName(src).IsValid() {
			ev.errorf("invalid source label name in label_join(): %s", src)
		}
		srcLabels[i-3] = src
	}

	if !model.LabelName(dst).IsValid() {
		ev.errorf("invalid destination label name in label_join(): %s", dst)
	}

	outSet := make(map[uint64]struct{}, len(vector))
	for i := range vector {
		el := &vector[i]

		srcVals := make([]string, len(srcLabels))
		for i, src := range srcLabels {
			srcVals[i] = el.Metric.Get(src)
		}

		lb := labels.NewBuilder(el.Metric)

		strval := strings.Join(srcVals, sep)
		if strval == "" {
			lb.Del(dst)
		} else {
			lb.Set(dst, strval)
		}

		el.Metric = lb.Labels()
		h := el.Metric.Hash()

		if _, exists := outSet[h]; exists {
			ev.errorf("duplicated label set in output of label_join(): %s", el.Metric)
		} else {
			outSet[h] = struct{}{}
		}
	}
	return vector
}

// Common code for date related functions.
func dateWrapper(ev *evaluator, args Expressions, f func(time.Time) float64) Value {
	var v Vector
	if len(args) == 0 {
		v = Vector{
			Sample{
				Metric: labels.Labels{},
				Point:  Point{V: float64(ev.Timestamp) / 1000, T: ev.Timestamp},
			},
		}
	} else {
		v = ev.evalVector(args[0])
	}
	for i := range v {
		el := &v[i]

		el.Metric = dropMetricName(el.Metric)
		t := time.Unix(int64(el.V), 0).UTC()
		el.V = f(t)
	}
	return v
}

// === days_in_month(v Vector) Scalar ===
func funcDaysInMonth(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(32 - time.Date(t.Year(), t.Month(), 32, 0, 0, 0, 0, time.UTC).Day())
	})
}

// === day_of_month(v Vector) Scalar ===
func funcDayOfMonth(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(t.Day())
	})
}

// === day_of_week(v Vector) Scalar ===
func funcDayOfWeek(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(t.Weekday())
	})
}

// === hour(v Vector) Scalar ===
func funcHour(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(t.Hour())
	})
}

// === minute(v Vector) Scalar ===
func funcMinute(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(t.Minute())
	})
}

// === month(v Vector) Scalar ===
func funcMonth(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(t.Month())
	})
}

// === year(v Vector) Scalar ===
func funcYear(ev *evaluator, args Expressions) Value {
	return dateWrapper(ev, args, func(t time.Time) float64 {
		return float64(t.Year())
	})
}

var functions = map[string]*Function{
	"abs": {
		Name:       "abs",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcAbs,
	},
	"absent": {
		Name:       "absent",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcAbsent,
	},
	"avg_over_time": {
		Name:       "avg_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcAvgOverTime,
	},
	"ceil": {
		Name:       "ceil",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcCeil,
	},
	"changes": {
		Name:       "changes",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcChanges,
	},
	"clamp_max": {
		Name:       "clamp_max",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcClampMax,
	},
	"clamp_min": {
		Name:       "clamp_min",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcClampMin,
	},
	"count_over_time": {
		Name:       "count_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcCountOverTime,
	},
	"days_in_month": {
		Name:       "days_in_month",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDaysInMonth,
	},
	"day_of_month": {
		Name:       "day_of_month",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDayOfMonth,
	},
	"day_of_week": {
		Name:       "day_of_week",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDayOfWeek,
	},
	"delta": {
		Name:       "delta",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcDelta,
	},
	"deriv": {
		Name:       "deriv",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcDeriv,
	},
	"exp": {
		Name:       "exp",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcExp,
	},
	"floor": {
		Name:       "floor",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcFloor,
	},
	"histogram_quantile": {
		Name:       "histogram_quantile",
		ArgTypes:   []ValueType{ValueTypeScalar, ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcHistogramQuantile,
	},
	"holt_winters": {
		Name:       "holt_winters",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcHoltWinters,
	},
	"hour": {
		Name:       "hour",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcHour,
	},
	"idelta": {
		Name:       "idelta",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcIdelta,
	},
	"increase": {
		Name:       "increase",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcIncrease,
	},
	"irate": {
		Name:       "irate",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcIrate,
	},
	"label_replace": {
		Name:       "label_replace",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString, ValueTypeString, ValueTypeString},
		ReturnType: ValueTypeVector,
		Call:       funcLabelReplace,
	},
	"label_join": {
		Name:       "label_join",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString, ValueTypeString},
		Variadic:   -1,
		ReturnType: ValueTypeVector,
		Call:       funcLabelJoin,
	},
	"ln": {
		Name:       "ln",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcLn,
	},
	"log10": {
		Name:       "log10",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcLog10,
	},
	"log2": {
		Name:       "log2",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcLog2,
	},
	"max_over_time": {
		Name:       "max_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcMaxOverTime,
	},
	"min_over_time": {
		Name:       "min_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcMinOverTime,
	},
	"minute": {
		Name:       "minute",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcMinute,
	},
	"month": {
		Name:       "month",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcMonth,
	},
	"predict_linear": {
		Name:       "predict_linear",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcPredictLinear,
	},
	"quantile_over_time": {
		Name:       "quantile_over_time",
		ArgTypes:   []ValueType{ValueTypeScalar, ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcQuantileOverTime,
	},
	"rate": {
		Name:       "rate",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRate,
	},
	"resets": {
		Name:       "resets",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcResets,
	},
	"round": {
		Name:       "round",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcRound,
	},
	"scalar": {
		Name:       "scalar",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeScalar,
		Call:       funcScalar,
	},
	"sort": {
		Name:       "sort",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSort,
	},
	"sort_desc": {
		Name:       "sort_desc",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSortDesc,
	},
	"sqrt": {
		Name:       "sqrt",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSqrt,
	},
	"stddev_over_time": {
		Name:       "stddev_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcStddevOverTime,
	},
	"stdvar_over_time": {
		Name:       "stdvar_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcStdvarOverTime,
	},
	"sum_over_time": {
		Name:       "sum_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcSumOverTime,
	},
	"time": {
		Name:       "time",
		ArgTypes:   []ValueType{},
		ReturnType: ValueTypeScalar,
		Call:       funcTime,
	},
	"timestamp": {
		Name:       "timestamp",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcTimestamp,
	},
	"vector": {
		Name:       "vector",
		ArgTypes:   []ValueType{ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcVector,
	},
	"year": {
		Name:       "year",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcYear,
	},
}

// getFunction returns a predefined Function object for the given name.
func getFunction(name string) (*Function, bool) {
	function, ok := functions[name]
	return function, ok
}

type vectorByValueHeap Vector

func (s vectorByValueHeap) Len() int {
	return len(s)
}

func (s vectorByValueHeap) Less(i, j int) bool {
	if math.IsNaN(float64(s[i].V)) {
		return true
	}
	return s[i].V < s[j].V
}

func (s vectorByValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByValueHeap) Push(x interface{}) {
	*s = append(*s, *(x.(*Sample)))
}

func (s *vectorByValueHeap) Pop() interface{} {
	old := *s
	n := len(old)
	el := old[n-1]
	*s = old[0 : n-1]
	return el
}

type vectorByReverseValueHeap Vector

func (s vectorByReverseValueHeap) Len() int {
	return len(s)
}

func (s vectorByReverseValueHeap) Less(i, j int) bool {
	if math.IsNaN(float64(s[i].V)) {
		return true
	}
	return s[i].V > s[j].V
}

func (s vectorByReverseValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByReverseValueHeap) Push(x interface{}) {
	*s = append(*s, *(x.(*Sample)))
}

func (s *vectorByReverseValueHeap) Pop() interface{} {
	old := *s
	n := len(old)
	el := old[n-1]
	*s = old[0 : n-1]
	return el
}
