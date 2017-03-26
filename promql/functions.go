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
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	Name         string
	ArgTypes     []model.ValueType
	OptionalArgs int
	ReturnType   model.ValueType
	Call         func(ev *evaluator, args Expressions) model.Value
}

// === time() model.SampleValue ===
func funcTime(ev *evaluator, args Expressions) model.Value {
	return &model.Scalar{
		Value:     model.SampleValue(ev.Timestamp.Unix()),
		Timestamp: ev.Timestamp,
	}
}

// extrapolatedRate is a utility function for rate/increase/delta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// extrapolates if the first/last sample is close to the boundary, and returns
// the result as either per-second (if isRate is true) or overall.
func extrapolatedRate(ev *evaluator, arg Expr, isCounter bool, isRate bool) model.Value {
	ms := arg.(*MatrixSelector)

	rangeStart := ev.Timestamp.Add(-ms.Range - ms.Offset)
	rangeEnd := ev.Timestamp.Add(-ms.Offset)

	resultVector := vector{}

	matrixValue := ev.evalMatrix(ms)
	for _, samples := range matrixValue {
		// No sense in trying to compute a rate without at least two points. Drop
		// this vector element.
		if len(samples.Values) < 2 {
			continue
		}
		var (
			counterCorrection model.SampleValue
			lastValue         model.SampleValue
		)
		for _, sample := range samples.Values {
			currentValue := sample.Value
			if isCounter && currentValue < lastValue {
				counterCorrection += lastValue
			}
			lastValue = currentValue
		}
		resultValue := lastValue - samples.Values[0].Value + counterCorrection

		// Duration between first/last samples and boundary of range.
		durationToStart := samples.Values[0].Timestamp.Sub(rangeStart).Seconds()
		durationToEnd := rangeEnd.Sub(samples.Values[len(samples.Values)-1].Timestamp).Seconds()

		sampledInterval := samples.Values[len(samples.Values)-1].Timestamp.Sub(samples.Values[0].Timestamp).Seconds()
		averageDurationBetweenSamples := sampledInterval / float64(len(samples.Values)-1)

		if isCounter && resultValue > 0 && samples.Values[0].Value >= 0 {
			// Counters cannot be negative. If we have any slope at
			// all (i.e. resultValue went up), we can extrapolate
			// the zero point of the counter. If the duration to the
			// zero point is shorter than the durationToStart, we
			// take the zero point as the start of the series,
			// thereby avoiding extrapolation to negative counter
			// values.
			durationToZero := sampledInterval * float64(samples.Values[0].Value/resultValue)
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
		resultValue = resultValue * model.SampleValue(extrapolateToInterval/sampledInterval)
		if isRate {
			resultValue = resultValue / model.SampleValue(ms.Range.Seconds())
		}

		resultSample := &sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: ev.Timestamp,
		}
		resultSample.Metric.Del(model.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === delta(matrix model.ValMatrix) Vector ===
func funcDelta(ev *evaluator, args Expressions) model.Value {
	return extrapolatedRate(ev, args[0], false, false)
}

// === rate(node model.ValMatrix) Vector ===
func funcRate(ev *evaluator, args Expressions) model.Value {
	return extrapolatedRate(ev, args[0], true, true)
}

// === increase(node model.ValMatrix) Vector ===
func funcIncrease(ev *evaluator, args Expressions) model.Value {
	return extrapolatedRate(ev, args[0], true, false)
}

// === irate(node model.ValMatrix) Vector ===
func funcIrate(ev *evaluator, args Expressions) model.Value {
	return instantValue(ev, args[0], true)
}

// === idelta(node model.ValMatric) Vector ===
func funcIdelta(ev *evaluator, args Expressions) model.Value {
	return instantValue(ev, args[0], false)
}

func instantValue(ev *evaluator, arg Expr, isRate bool) model.Value {
	resultVector := vector{}
	for _, samples := range ev.evalMatrix(arg) {
		// No sense in trying to compute a rate without at least two points. Drop
		// this vector element.
		if len(samples.Values) < 2 {
			continue
		}

		lastSample := samples.Values[len(samples.Values)-1]
		previousSample := samples.Values[len(samples.Values)-2]

		var resultValue model.SampleValue
		if isRate && lastSample.Value < previousSample.Value {
			// Counter reset.
			resultValue = lastSample.Value
		} else {
			resultValue = lastSample.Value - previousSample.Value
		}

		sampledInterval := lastSample.Timestamp.Sub(previousSample.Timestamp)
		if sampledInterval == 0 {
			// Avoid dividing by 0.
			continue
		}

		if isRate {
			// Convert to per-second.
			resultValue /= model.SampleValue(sampledInterval.Seconds())
		}

		resultSample := &sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: ev.Timestamp,
		}
		resultSample.Metric.Del(model.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
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
func funcHoltWinters(ev *evaluator, args Expressions) model.Value {
	mat := ev.evalMatrix(args[0])

	// The smoothing factor argument.
	sf := ev.evalFloat(args[1])

	// The trend factor argument.
	tf := ev.evalFloat(args[2])

	// Sanity check the input.
	if sf <= 0 || sf >= 1 {
		ev.errorf("invalid smoothing factor. Expected: 0 < sf < 1 got: %f", sf)
	}
	if tf <= 0 || tf >= 1 {
		ev.errorf("invalid trend factor. Expected: 0 < tf < 1 got: %f", sf)
	}

	// Make an output vector large enough to hold the entire result.
	resultVector := make(vector, 0, len(mat))

	// Create scratch values.
	var s, b, d []float64

	var l int
	for _, samples := range mat {
		l = len(samples.Values)

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
		for i, v := range samples.Values {
			d[i] = float64(v.Value)
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

		samples.Metric.Del(model.MetricNameLabel)
		resultVector = append(resultVector, &sample{
			Metric:    samples.Metric,
			Value:     model.SampleValue(s[len(s)-1]), // The last value in the vector is the smoothed result.
			Timestamp: ev.Timestamp,
		})
	}

	return resultVector
}

// === sort(node model.ValVector) Vector ===
func funcSort(ev *evaluator, args Expressions) model.Value {
	// NaN should sort to the bottom, so take descending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByReverseValueHeap(ev.evalVector(args[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return vector(byValueSorter)
}

// === sortDesc(node model.ValVector) Vector ===
func funcSortDesc(ev *evaluator, args Expressions) model.Value {
	// NaN should sort to the bottom, so take ascending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByValueHeap(ev.evalVector(args[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return vector(byValueSorter)
}

// === clamp_max(vector model.ValVector, max Scalar) Vector ===
func funcClampMax(ev *evaluator, args Expressions) model.Value {
	vec := ev.evalVector(args[0])
	max := ev.evalFloat(args[1])
	for _, el := range vec {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Min(max, float64(el.Value)))
	}
	return vec
}

// === clamp_min(vector model.ValVector, min Scalar) Vector ===
func funcClampMin(ev *evaluator, args Expressions) model.Value {
	vec := ev.evalVector(args[0])
	min := ev.evalFloat(args[1])
	for _, el := range vec {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Max(min, float64(el.Value)))
	}
	return vec
}

// === drop_common_labels(node model.ValVector) Vector ===
func funcDropCommonLabels(ev *evaluator, args Expressions) model.Value {
	vec := ev.evalVector(args[0])
	if len(vec) < 1 {
		return vector{}
	}
	common := model.LabelSet{}
	for k, v := range vec[0].Metric.Metric {
		// TODO(julius): Should we also drop common metric names?
		if k == model.MetricNameLabel {
			continue
		}
		common[k] = v
	}

	for _, el := range vec[1:] {
		for k, v := range common {
			if el.Metric.Metric[k] != v {
				// Deletion of map entries while iterating over them is safe.
				// From http://golang.org/ref/spec#For_statements:
				// "If map entries that have not yet been reached are deleted during
				// iteration, the corresponding iteration values will not be produced."
				delete(common, k)
			}
		}
	}

	for _, el := range vec {
		for k := range el.Metric.Metric {
			if _, ok := common[k]; ok {
				el.Metric.Del(k)
			}
		}
	}
	return vec
}

// === round(vector model.ValVector, toNearest=1 Scalar) Vector ===
func funcRound(ev *evaluator, args Expressions) model.Value {
	// round returns a number rounded to toNearest.
	// Ties are solved by rounding up.
	toNearest := float64(1)
	if len(args) >= 2 {
		toNearest = ev.evalFloat(args[1])
	}
	// Invert as it seems to cause fewer floating point accuracy issues.
	toNearestInverse := 1.0 / toNearest

	vec := ev.evalVector(args[0])
	for _, el := range vec {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Floor(float64(el.Value)*toNearestInverse+0.5) / toNearestInverse)
	}
	return vec
}

// === scalar(node model.ValVector) Scalar ===
func funcScalar(ev *evaluator, args Expressions) model.Value {
	v := ev.evalVector(args[0])
	if len(v) != 1 {
		return &model.Scalar{
			Value:     model.SampleValue(math.NaN()),
			Timestamp: ev.Timestamp,
		}
	}
	return &model.Scalar{
		Value:     model.SampleValue(v[0].Value),
		Timestamp: ev.Timestamp,
	}
}

// === count_scalar(vector model.ValVector) model.SampleValue ===
func funcCountScalar(ev *evaluator, args Expressions) model.Value {
	return &model.Scalar{
		Value:     model.SampleValue(len(ev.evalVector(args[0]))),
		Timestamp: ev.Timestamp,
	}
}

func aggrOverTime(ev *evaluator, args Expressions, aggrFn func([]model.SamplePair) model.SampleValue) model.Value {
	mat := ev.evalMatrix(args[0])
	resultVector := vector{}

	for _, el := range mat {
		if len(el.Values) == 0 {
			continue
		}

		el.Metric.Del(model.MetricNameLabel)
		resultVector = append(resultVector, &sample{
			Metric:    el.Metric,
			Value:     aggrFn(el.Values),
			Timestamp: ev.Timestamp,
		})
	}
	return resultVector
}

// === avg_over_time(matrix model.ValMatrix) Vector ===
func funcAvgOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		var sum model.SampleValue
		for _, v := range values {
			sum += v.Value
		}
		return sum / model.SampleValue(len(values))
	})
}

// === count_over_time(matrix model.ValMatrix) Vector ===
func funcCountOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		return model.SampleValue(len(values))
	})
}

// === floor(vector model.ValVector) Vector ===
func funcFloor(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Floor(float64(el.Value)))
	}
	return vector
}

// === max_over_time(matrix model.ValMatrix) Vector ===
func funcMaxOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		max := math.Inf(-1)
		for _, v := range values {
			max = math.Max(max, float64(v.Value))
		}
		return model.SampleValue(max)
	})
}

// === min_over_time(matrix model.ValMatrix) Vector ===
func funcMinOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		min := math.Inf(1)
		for _, v := range values {
			min = math.Min(min, float64(v.Value))
		}
		return model.SampleValue(min)
	})
}

// === sum_over_time(matrix model.ValMatrix) Vector ===
func funcSumOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		var sum model.SampleValue
		for _, v := range values {
			sum += v.Value
		}
		return sum
	})
}

// === quantile_over_time(matrix model.ValMatrix) Vector ===
func funcQuantileOverTime(ev *evaluator, args Expressions) model.Value {
	q := ev.evalFloat(args[0])
	mat := ev.evalMatrix(args[1])
	resultVector := vector{}

	for _, el := range mat {
		if len(el.Values) == 0 {
			continue
		}

		el.Metric.Del(model.MetricNameLabel)
		values := make(vectorByValueHeap, 0, len(el.Values))
		for _, v := range el.Values {
			values = append(values, &sample{Value: v.Value})
		}
		resultVector = append(resultVector, &sample{
			Metric:    el.Metric,
			Value:     model.SampleValue(quantile(q, values)),
			Timestamp: ev.Timestamp,
		})
	}
	return resultVector
}

// === stddev_over_time(matrix model.ValMatrix) Vector ===
func funcStddevOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		var sum, squaredSum, count model.SampleValue
		for _, v := range values {
			sum += v.Value
			squaredSum += v.Value * v.Value
			count++
		}
		avg := sum / count
		return model.SampleValue(math.Sqrt(float64(squaredSum/count - avg*avg)))
	})
}

// === stdvar_over_time(matrix model.ValMatrix) Vector ===
func funcStdvarOverTime(ev *evaluator, args Expressions) model.Value {
	return aggrOverTime(ev, args, func(values []model.SamplePair) model.SampleValue {
		var sum, squaredSum, count model.SampleValue
		for _, v := range values {
			sum += v.Value
			squaredSum += v.Value * v.Value
			count++
		}
		avg := sum / count
		return squaredSum/count - avg*avg
	})
}

// === abs(vector model.ValVector) Vector ===
func funcAbs(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Abs(float64(el.Value)))
	}
	return vector
}

// === absent(vector model.ValVector) Vector ===
func funcAbsent(ev *evaluator, args Expressions) model.Value {
	if len(ev.evalVector(args[0])) > 0 {
		return vector{}
	}
	m := model.Metric{}
	if vs, ok := args[0].(*VectorSelector); ok {
		for _, matcher := range vs.LabelMatchers {
			if matcher.Type == metric.Equal && matcher.Name != model.MetricNameLabel {
				m[matcher.Name] = matcher.Value
			}
		}
	}
	return vector{
		&sample{
			Metric: metric.Metric{
				Metric: m,
				Copied: true,
			},
			Value:     1,
			Timestamp: ev.Timestamp,
		},
	}
}

// === ceil(vector model.ValVector) Vector ===
func funcCeil(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Ceil(float64(el.Value)))
	}
	return vector
}

// === exp(vector model.ValVector) Vector ===
func funcExp(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Exp(float64(el.Value)))
	}
	return vector
}

// === sqrt(vector VectorNode) Vector ===
func funcSqrt(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Sqrt(float64(el.Value)))
	}
	return vector
}

// === ln(vector model.ValVector) Vector ===
func funcLn(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Log(float64(el.Value)))
	}
	return vector
}

// === log2(vector model.ValVector) Vector ===
func funcLog2(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Log2(float64(el.Value)))
	}
	return vector
}

// === log10(vector model.ValVector) Vector ===
func funcLog10(ev *evaluator, args Expressions) model.Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Del(model.MetricNameLabel)
		el.Value = model.SampleValue(math.Log10(float64(el.Value)))
	}
	return vector
}

// linearRegression performs a least-square linear regression analysis on the
// provided SamplePairs. It returns the slope, and the intercept value at the
// provided time.
func linearRegression(samples []model.SamplePair, interceptTime model.Time) (slope, intercept model.SampleValue) {
	var (
		n            model.SampleValue
		sumX, sumY   model.SampleValue
		sumXY, sumX2 model.SampleValue
	)
	for _, sample := range samples {
		x := model.SampleValue(
			model.Time(sample.Timestamp-interceptTime).UnixNano(),
		) / 1e9
		n += 1.0
		sumY += sample.Value
		sumX += x
		sumXY += x * sample.Value
		sumX2 += x * x
	}
	covXY := sumXY - sumX*sumY/n
	varX := sumX2 - sumX*sumX/n

	slope = covXY / varX
	intercept = sumY/n - slope*sumX/n
	return slope, intercept
}

// === deriv(node model.ValMatrix) Vector ===
func funcDeriv(ev *evaluator, args Expressions) model.Value {
	mat := ev.evalMatrix(args[0])
	resultVector := make(vector, 0, len(mat))

	for _, samples := range mat {
		// No sense in trying to compute a derivative without at least two points.
		// Drop this vector element.
		if len(samples.Values) < 2 {
			continue
		}
		slope, _ := linearRegression(samples.Values, 0)
		resultSample := &sample{
			Metric:    samples.Metric,
			Value:     slope,
			Timestamp: ev.Timestamp,
		}
		resultSample.Metric.Del(model.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === predict_linear(node model.ValMatrix, k model.ValScalar) Vector ===
func funcPredictLinear(ev *evaluator, args Expressions) model.Value {
	mat := ev.evalMatrix(args[0])
	resultVector := make(vector, 0, len(mat))
	duration := model.SampleValue(ev.evalFloat(args[1]))

	for _, samples := range mat {
		// No sense in trying to predict anything without at least two points.
		// Drop this vector element.
		if len(samples.Values) < 2 {
			continue
		}
		slope, intercept := linearRegression(samples.Values, ev.Timestamp)
		resultSample := &sample{
			Metric:    samples.Metric,
			Value:     slope*duration + intercept,
			Timestamp: ev.Timestamp,
		}
		resultSample.Metric.Del(model.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === histogram_quantile(k model.ValScalar, vector model.ValVector) Vector ===
func funcHistogramQuantile(ev *evaluator, args Expressions) model.Value {
	q := model.SampleValue(ev.evalFloat(args[0]))
	inVec := ev.evalVector(args[1])

	outVec := vector{}
	signatureToMetricWithBuckets := map[uint64]*metricWithBuckets{}
	for _, el := range inVec {
		upperBound, err := strconv.ParseFloat(
			string(el.Metric.Metric[model.BucketLabel]), 64,
		)
		if err != nil {
			// Oops, no bucket label or malformed label value. Skip.
			// TODO(beorn7): Issue a warning somehow.
			continue
		}
		signature := model.SignatureWithoutLabels(el.Metric.Metric, excludedLabels)
		mb, ok := signatureToMetricWithBuckets[signature]
		if !ok {
			el.Metric.Del(model.BucketLabel)
			el.Metric.Del(model.MetricNameLabel)
			mb = &metricWithBuckets{el.Metric, nil}
			signatureToMetricWithBuckets[signature] = mb
		}
		mb.buckets = append(mb.buckets, bucket{upperBound, el.Value})
	}

	for _, mb := range signatureToMetricWithBuckets {
		outVec = append(outVec, &sample{
			Metric:    mb.metric,
			Value:     model.SampleValue(bucketQuantile(q, mb.buckets)),
			Timestamp: ev.Timestamp,
		})
	}

	return outVec
}

// === resets(matrix model.ValMatrix) Vector ===
func funcResets(ev *evaluator, args Expressions) model.Value {
	in := ev.evalMatrix(args[0])
	out := make(vector, 0, len(in))

	for _, samples := range in {
		resets := 0
		prev := model.SampleValue(samples.Values[0].Value)
		for _, sample := range samples.Values[1:] {
			current := sample.Value
			if current < prev {
				resets++
			}
			prev = current
		}

		rs := &sample{
			Metric:    samples.Metric,
			Value:     model.SampleValue(resets),
			Timestamp: ev.Timestamp,
		}
		rs.Metric.Del(model.MetricNameLabel)
		out = append(out, rs)
	}
	return out
}

// === changes(matrix model.ValMatrix) Vector ===
func funcChanges(ev *evaluator, args Expressions) model.Value {
	in := ev.evalMatrix(args[0])
	out := make(vector, 0, len(in))

	for _, samples := range in {
		changes := 0
		prev := model.SampleValue(samples.Values[0].Value)
		for _, sample := range samples.Values[1:] {
			current := sample.Value
			if current != prev && !(math.IsNaN(float64(current)) && math.IsNaN(float64(prev))) {
				changes++
			}
			prev = current
		}

		rs := &sample{
			Metric:    samples.Metric,
			Value:     model.SampleValue(changes),
			Timestamp: ev.Timestamp,
		}
		rs.Metric.Del(model.MetricNameLabel)
		out = append(out, rs)
	}
	return out
}

// === label_replace(vector model.ValVector, dst_label, replacement, src_labelname, regex model.ValString) Vector ===
func funcLabelReplace(ev *evaluator, args Expressions) model.Value {
	var (
		vector   = ev.evalVector(args[0])
		dst      = model.LabelName(ev.evalString(args[1]).Value)
		repl     = ev.evalString(args[2]).Value
		src      = model.LabelName(ev.evalString(args[3]).Value)
		regexStr = ev.evalString(args[4]).Value
	)

	regex, err := regexp.Compile("^(?:" + regexStr + ")$")
	if err != nil {
		ev.errorf("invalid regular expression in label_replace(): %s", regexStr)
	}
	if !model.LabelNameRE.MatchString(string(dst)) {
		ev.errorf("invalid destination label name in label_replace(): %s", dst)
	}

	outSet := make(map[model.Fingerprint]struct{}, len(vector))
	for _, el := range vector {
		srcVal := string(el.Metric.Metric[src])
		indexes := regex.FindStringSubmatchIndex(srcVal)
		// If there is no match, no replacement should take place.
		if indexes == nil {
			continue
		}
		res := regex.ExpandString([]byte{}, repl, srcVal, indexes)
		if len(res) == 0 {
			el.Metric.Del(dst)
		} else {
			el.Metric.Set(dst, model.LabelValue(res))
		}

		fp := el.Metric.Metric.Fingerprint()
		if _, exists := outSet[fp]; exists {
			ev.errorf("duplicated label set in output of label_replace(): %s", el.Metric.Metric)
		} else {
			outSet[fp] = struct{}{}
		}
	}

	return vector
}

// === vector(s scalar) Vector ===
func funcVector(ev *evaluator, args Expressions) model.Value {
	return vector{
		&sample{
			Metric:    metric.Metric{},
			Value:     model.SampleValue(ev.evalFloat(args[0])),
			Timestamp: ev.Timestamp,
		},
	}
}

// Common code for date related functions.
func dateWrapper(ev *evaluator, args Expressions, f func(time.Time) model.SampleValue) model.Value {
	var v vector
	if len(args) == 0 {
		v = vector{
			&sample{
				Metric: metric.Metric{},
				Value:  model.SampleValue(ev.Timestamp.Unix()),
			},
		}
	} else {
		v = ev.evalVector(args[0])
	}
	for _, el := range v {
		el.Metric.Del(model.MetricNameLabel)
		t := time.Unix(int64(el.Value), 0).UTC()
		el.Value = f(t)
	}
	return v
}

// === days_in_month(v vector) scalar ===
func funcDaysInMonth(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(32 - time.Date(t.Year(), t.Month(), 32, 0, 0, 0, 0, time.UTC).Day())
	})
}

// === day_of_month(v vector) scalar ===
func funcDayOfMonth(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(t.Day())
	})
}

// === day_of_week(v vector) scalar ===
func funcDayOfWeek(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(t.Weekday())
	})
}

// === hour(v vector) scalar ===
func funcHour(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(t.Hour())
	})
}

// === minute(v vector) scalar ===
func funcMinute(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(t.Minute())
	})
}

// === month(v vector) scalar ===
func funcMonth(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(t.Month())
	})
}

// === year(v vector) scalar ===
func funcYear(ev *evaluator, args Expressions) model.Value {
	return dateWrapper(ev, args, func(t time.Time) model.SampleValue {
		return model.SampleValue(t.Year())
	})
}

var functions = map[string]*Function{
	"abs": {
		Name:       "abs",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcAbs,
	},
	"absent": {
		Name:       "absent",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcAbsent,
	},
	"avg_over_time": {
		Name:       "avg_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcAvgOverTime,
	},
	"ceil": {
		Name:       "ceil",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcCeil,
	},
	"changes": {
		Name:       "changes",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcChanges,
	},
	"clamp_max": {
		Name:       "clamp_max",
		ArgTypes:   []model.ValueType{model.ValVector, model.ValScalar},
		ReturnType: model.ValVector,
		Call:       funcClampMax,
	},
	"clamp_min": {
		Name:       "clamp_min",
		ArgTypes:   []model.ValueType{model.ValVector, model.ValScalar},
		ReturnType: model.ValVector,
		Call:       funcClampMin,
	},
	"count_over_time": {
		Name:       "count_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcCountOverTime,
	},
	"count_scalar": {
		Name:       "count_scalar",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValScalar,
		Call:       funcCountScalar,
	},
	"days_in_month": {
		Name:         "days_in_month",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcDaysInMonth,
	},
	"day_of_month": {
		Name:         "day_of_month",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcDayOfMonth,
	},
	"day_of_week": {
		Name:         "day_of_week",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcDayOfWeek,
	},
	"delta": {
		Name:       "delta",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcDelta,
	},
	"deriv": {
		Name:       "deriv",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcDeriv,
	},
	"drop_common_labels": {
		Name:       "drop_common_labels",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcDropCommonLabels,
	},
	"exp": {
		Name:       "exp",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcExp,
	},
	"floor": {
		Name:       "floor",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcFloor,
	},
	"histogram_quantile": {
		Name:       "histogram_quantile",
		ArgTypes:   []model.ValueType{model.ValScalar, model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcHistogramQuantile,
	},
	"holt_winters": {
		Name:       "holt_winters",
		ArgTypes:   []model.ValueType{model.ValMatrix, model.ValScalar, model.ValScalar},
		ReturnType: model.ValVector,
		Call:       funcHoltWinters,
	},
	"hour": {
		Name:         "hour",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcHour,
	},
	"idelta": {
		Name:       "idelta",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcIdelta,
	},
	"increase": {
		Name:       "increase",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcIncrease,
	},
	"irate": {
		Name:       "irate",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcIrate,
	},
	"label_replace": {
		Name:       "label_replace",
		ArgTypes:   []model.ValueType{model.ValVector, model.ValString, model.ValString, model.ValString, model.ValString},
		ReturnType: model.ValVector,
		Call:       funcLabelReplace,
	},
	"ln": {
		Name:       "ln",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcLn,
	},
	"log10": {
		Name:       "log10",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcLog10,
	},
	"log2": {
		Name:       "log2",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcLog2,
	},
	"max_over_time": {
		Name:       "max_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcMaxOverTime,
	},
	"min_over_time": {
		Name:       "min_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcMinOverTime,
	},
	"minute": {
		Name:         "minute",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcMinute,
	},
	"month": {
		Name:         "month",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcMonth,
	},
	"predict_linear": {
		Name:       "predict_linear",
		ArgTypes:   []model.ValueType{model.ValMatrix, model.ValScalar},
		ReturnType: model.ValVector,
		Call:       funcPredictLinear,
	},
	"quantile_over_time": {
		Name:       "quantile_over_time",
		ArgTypes:   []model.ValueType{model.ValScalar, model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcQuantileOverTime,
	},
	"rate": {
		Name:       "rate",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcRate,
	},
	"resets": {
		Name:       "resets",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcResets,
	},
	"round": {
		Name:         "round",
		ArgTypes:     []model.ValueType{model.ValVector, model.ValScalar},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcRound,
	},
	"scalar": {
		Name:       "scalar",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValScalar,
		Call:       funcScalar,
	},
	"sort": {
		Name:       "sort",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcSort,
	},
	"sort_desc": {
		Name:       "sort_desc",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcSortDesc,
	},
	"sqrt": {
		Name:       "sqrt",
		ArgTypes:   []model.ValueType{model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcSqrt,
	},
	"stddev_over_time": {
		Name:       "stddev_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcStddevOverTime,
	},
	"stdvar_over_time": {
		Name:       "stdvar_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcStdvarOverTime,
	},
	"sum_over_time": {
		Name:       "sum_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcSumOverTime,
	},
	"time": {
		Name:       "time",
		ArgTypes:   []model.ValueType{},
		ReturnType: model.ValScalar,
		Call:       funcTime,
	},
	"vector": {
		Name:       "vector",
		ArgTypes:   []model.ValueType{model.ValScalar},
		ReturnType: model.ValVector,
		Call:       funcVector,
	},
	"year": {
		Name:         "year",
		ArgTypes:     []model.ValueType{model.ValVector},
		OptionalArgs: 1,
		ReturnType:   model.ValVector,
		Call:         funcYear,
	},
}

// getFunction returns a predefined Function object for the given name.
func getFunction(name string) (*Function, bool) {
	function, ok := functions[name]
	return function, ok
}

type vectorByValueHeap vector

func (s vectorByValueHeap) Len() int {
	return len(s)
}

func (s vectorByValueHeap) Less(i, j int) bool {
	if math.IsNaN(float64(s[i].Value)) {
		return true
	}
	return s[i].Value < s[j].Value
}

func (s vectorByValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByValueHeap) Push(x interface{}) {
	*s = append(*s, x.(*sample))
}

func (s *vectorByValueHeap) Pop() interface{} {
	old := *s
	n := len(old)
	el := old[n-1]
	*s = old[0 : n-1]
	return el
}

type vectorByReverseValueHeap vector

func (s vectorByReverseValueHeap) Len() int {
	return len(s)
}

func (s vectorByReverseValueHeap) Less(i, j int) bool {
	if math.IsNaN(float64(s[i].Value)) {
		return true
	}
	return s[i].Value > s[j].Value
}

func (s vectorByReverseValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByReverseValueHeap) Push(x interface{}) {
	*s = append(*s, x.(*sample))
}

func (s *vectorByReverseValueHeap) Pop() interface{} {
	old := *s
	n := len(old)
	el := old[n-1]
	*s = old[0 : n-1]
	return el
}
