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
	"container/heap"
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

// === delta(matrix model.ValMatrix, isCounter=0 model.ValScalar) Vector ===
func funcDelta(ev *evaluator, args Expressions) model.Value {
	isCounter := len(args) >= 2 && ev.evalInt(args[1]) > 0
	resultVector := vector{}

	// If we treat these metrics as counters, we need to fetch all values
	// in the interval to find breaks in the timeseries' monotonicity.
	// I.e. if a counter resets, we want to ignore that reset.
	var matrixValue matrix
	if isCounter {
		matrixValue = ev.evalMatrix(args[0])
	} else {
		matrixValue = ev.evalMatrixBounds(args[0])
	}
	for _, samples := range matrixValue {
		// No sense in trying to compute a delta without at least two points. Drop
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
				counterCorrection += lastValue - currentValue
			}
			lastValue = currentValue
		}
		resultValue := lastValue - samples.Values[0].Value + counterCorrection

		targetInterval := args[0].(*MatrixSelector).Range
		sampledInterval := samples.Values[len(samples.Values)-1].Timestamp.Sub(samples.Values[0].Timestamp)
		if sampledInterval == 0 {
			// Only found one sample. Cannot compute a rate from this.
			continue
		}
		// Correct for differences in target vs. actual delta interval.
		//
		// Above, we didn't actually calculate the delta for the specified target
		// interval, but for an interval between the first and last found samples
		// under the target interval, which will usually have less time between
		// them. Depending on how many samples are found under a target interval,
		// the delta results are distorted and temporal aliasing occurs (ugly
		// bumps). This effect is corrected for below.
		intervalCorrection := model.SampleValue(targetInterval) / model.SampleValue(sampledInterval)
		resultValue *= intervalCorrection

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

// === rate(node model.ValMatrix) Vector ===
func funcRate(ev *evaluator, args Expressions) model.Value {
	args = append(args, &NumberLiteral{1})
	vector := funcDelta(ev, args).(vector)

	// TODO: could be other type of model.ValMatrix in the future (right now, only
	// MatrixSelector exists). Find a better way of getting the duration of a
	// matrix, such as looking at the samples themselves.
	interval := args[0].(*MatrixSelector).Range
	for i := range vector {
		vector[i].Value /= model.SampleValue(interval / time.Second)
	}
	return vector
}

// === increase(node model.ValMatrix) Vector ===
func funcIncrease(ev *evaluator, args Expressions) model.Value {
	args = append(args, &NumberLiteral{1})
	return funcDelta(ev, args).(vector)
}

// === sort(node model.ValVector) Vector ===
func funcSort(ev *evaluator, args Expressions) model.Value {
	byValueSorter := vectorByValueHeap(ev.evalVector(args[0]))
	sort.Sort(byValueSorter)
	return vector(byValueSorter)
}

// === sortDesc(node model.ValVector) Vector ===
func funcSortDesc(ev *evaluator, args Expressions) model.Value {
	byValueSorter := vectorByValueHeap(ev.evalVector(args[0]))
	sort.Sort(sort.Reverse(byValueSorter))

	return vector(byValueSorter)
}

// === topk(k model.ValScalar, node model.ValVector) Vector ===
func funcTopk(ev *evaluator, args Expressions) model.Value {
	k := ev.evalInt(args[0])
	if k < 1 {
		return vector{}
	}
	vec := ev.evalVector(args[1])

	topk := make(vectorByValueHeap, 0, k)

	for _, el := range vec {
		if len(topk) < k || topk[0].Value < el.Value {
			if len(topk) == k {
				heap.Pop(&topk)
			}
			heap.Push(&topk, el)
		}
	}
	sort.Sort(sort.Reverse(topk))
	return vector(topk)
}

// === bottomk(k model.ValScalar, node model.ValVector) Vector ===
func funcBottomk(ev *evaluator, args Expressions) model.Value {
	k := ev.evalInt(args[0])
	if k < 1 {
		return vector{}
	}
	vec := ev.evalVector(args[1])

	bottomk := make(vectorByValueHeap, 0, k)
	bkHeap := reverseHeap{Interface: &bottomk}

	for _, el := range vec {
		if len(bottomk) < k || bottomk[0].Value > el.Value {
			if len(bottomk) == k {
				heap.Pop(&bkHeap)
			}
			heap.Push(&bkHeap, el)
		}
	}
	sort.Sort(bottomk)
	return vector(bottomk)
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
		return &model.Scalar{model.SampleValue(math.NaN()), ev.Timestamp}
	}
	return &model.Scalar{model.SampleValue(v[0].Value), ev.Timestamp}
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

// === deriv(node model.ValMatrix) Vector ===
func funcDeriv(ev *evaluator, args Expressions) model.Value {
	resultVector := vector{}
	mat := ev.evalMatrix(args[0])

	for _, samples := range mat {
		// No sense in trying to compute a derivative without at least two points.
		// Drop this vector element.
		if len(samples.Values) < 2 {
			continue
		}

		// Least squares.
		var (
			n            model.SampleValue
			sumX, sumY   model.SampleValue
			sumXY, sumX2 model.SampleValue
		)
		for _, sample := range samples.Values {
			x := model.SampleValue(sample.Timestamp.UnixNano() / 1e9)
			n += 1.0
			sumY += sample.Value
			sumX += x
			sumXY += x * sample.Value
			sumX2 += x * x
		}
		numerator := sumXY - sumX*sumY/n
		denominator := sumX2 - (sumX*sumX)/n

		resultValue := numerator / denominator

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

// === predict_linear(node model.ValMatrix, k model.ValScalar) Vector ===
func funcPredictLinear(ev *evaluator, args Expressions) model.Value {
	vec := funcDeriv(ev, args[0:1]).(vector)
	duration := model.SampleValue(model.SampleValue(ev.evalFloat(args[1])))

	excludedLabels := map[model.LabelName]struct{}{
		model.MetricNameLabel: {},
	}

	// Calculate predicted delta over the duration.
	signatureToDelta := map[uint64]model.SampleValue{}
	for _, el := range vec {
		signature := model.SignatureWithoutLabels(el.Metric.Metric, excludedLabels)
		signatureToDelta[signature] = el.Value * duration
	}

	// add predicted delta to last value.
	matrixBounds := ev.evalMatrixBounds(args[0])
	outVec := make(vector, 0, len(signatureToDelta))
	for _, samples := range matrixBounds {
		if len(samples.Values) < 2 {
			continue
		}
		signature := model.SignatureWithoutLabels(samples.Metric.Metric, excludedLabels)
		delta, ok := signatureToDelta[signature]
		if ok {
			samples.Metric.Del(model.MetricNameLabel)
			outVec = append(outVec, &sample{
				Metric:    samples.Metric,
				Value:     delta + samples.Values[1].Value,
				Timestamp: ev.Timestamp,
			})
		}
	}
	return outVec
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
			Value:     model.SampleValue(quantile(q, mb.buckets)),
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
			if current != prev {
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

// === vector(s scalar, vector model.ValVectora={}) Vector ===
func funcVector(ev *evaluator, args Expressions) model.Value {
	m := model.Metric{}
	if len(args) >= 2 {
		if vs, ok := args[1].(*VectorSelector); ok {
			for _, matcher := range vs.LabelMatchers {
				if matcher.Type == metric.Equal && matcher.Name != model.MetricNameLabel {
					m[matcher.Name] = matcher.Value
				}
			}
		}
	}
	return vector{
		&sample{
			Metric: metric.Metric{
				Metric: m,
				Copied: true,
			},
			Value:     model.SampleValue(ev.evalFloat(args[0])),
			Timestamp: ev.Timestamp,
		},
	}
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
	"increase": {
		Name:       "increase",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcIncrease,
	},
	"avg_over_time": {
		Name:       "avg_over_time",
		ArgTypes:   []model.ValueType{model.ValMatrix},
		ReturnType: model.ValVector,
		Call:       funcAvgOverTime,
	},
	"bottomk": {
		Name:       "bottomk",
		ArgTypes:   []model.ValueType{model.ValScalar, model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcBottomk,
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
	"delta": {
		Name:         "delta",
		ArgTypes:     []model.ValueType{model.ValMatrix, model.ValScalar},
		OptionalArgs: 1, // The 2nd argument is deprecated.
		ReturnType:   model.ValVector,
		Call:         funcDelta,
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
	"predict_linear": {
		Name:       "predict_linear",
		ArgTypes:   []model.ValueType{model.ValMatrix, model.ValScalar},
		ReturnType: model.ValVector,
		Call:       funcPredictLinear,
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
	"topk": {
		Name:       "topk",
		ArgTypes:   []model.ValueType{model.ValScalar, model.ValVector},
		ReturnType: model.ValVector,
		Call:       funcTopk,
	},
	"vector": {
		Name:         "vector",
		ArgTypes:     []model.ValueType{model.ValScalar, model.ValVector},
		ReturnType:   model.ValVector,
		OptionalArgs: 1,
		Call:         funcVector,
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

type reverseHeap struct {
	heap.Interface
}

func (s reverseHeap) Less(i, j int) bool {
	return s.Interface.Less(j, i)
}
