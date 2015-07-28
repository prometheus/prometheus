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
	"sort"
	"strconv"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	Name         string
	ArgTypes     []ExprType
	OptionalArgs int
	ReturnType   ExprType
	Call         func(ev *evaluator, args Expressions) Value
}

// === time() clientmodel.SampleValue ===
func funcTime(ev *evaluator, args Expressions) Value {
	return &Scalar{
		Value:     clientmodel.SampleValue(ev.Timestamp.Unix()),
		Timestamp: ev.Timestamp,
	}
}

// === delta(matrix ExprMatrix, isCounter=0 ExprScalar) Vector ===
func funcDelta(ev *evaluator, args Expressions) Value {
	isCounter := len(args) >= 2 && ev.evalInt(args[1]) > 0
	resultVector := Vector{}

	// If we treat these metrics as counters, we need to fetch all values
	// in the interval to find breaks in the timeseries' monotonicity.
	// I.e. if a counter resets, we want to ignore that reset.
	var matrixValue Matrix
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

		counterCorrection := clientmodel.SampleValue(0)
		lastValue := clientmodel.SampleValue(0)
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
		intervalCorrection := clientmodel.SampleValue(targetInterval) / clientmodel.SampleValue(sampledInterval)
		resultValue *= intervalCorrection

		resultSample := &Sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: ev.Timestamp,
		}
		resultSample.Metric.Delete(clientmodel.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === rate(node ExprMatrix) Vector ===
func funcRate(ev *evaluator, args Expressions) Value {
	args = append(args, &NumberLiteral{1})
	vector := funcDelta(ev, args).(Vector)

	// TODO: could be other type of ExprMatrix in the future (right now, only
	// MatrixSelector exists). Find a better way of getting the duration of a
	// matrix, such as looking at the samples themselves.
	interval := args[0].(*MatrixSelector).Range
	for i := range vector {
		vector[i].Value /= clientmodel.SampleValue(interval / time.Second)
	}
	return vector
}

// === increase(node ExprMatrix) Vector ===
func funcIncrease(ev *evaluator, args Expressions) Value {
	args = append(args, &NumberLiteral{1})
	vector := funcDelta(ev, args).(Vector)
	return vector
}

// === sort(node ExprVector) Vector ===
func funcSort(ev *evaluator, args Expressions) Value {
	byValueSorter := vectorByValueHeap(ev.evalVector(args[0]))
	sort.Sort(byValueSorter)
	return Vector(byValueSorter)
}

// === sortDesc(node ExprVector) Vector ===
func funcSortDesc(ev *evaluator, args Expressions) Value {
	byValueSorter := vectorByValueHeap(ev.evalVector(args[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter)
}

// === topk(k ExprScalar, node ExprVector) Vector ===
func funcTopk(ev *evaluator, args Expressions) Value {
	k := ev.evalInt(args[0])
	if k < 1 {
		return Vector{}
	}
	vector := ev.evalVector(args[1])

	topk := make(vectorByValueHeap, 0, k)

	for _, el := range vector {
		if len(topk) < k || topk[0].Value < el.Value {
			if len(topk) == k {
				heap.Pop(&topk)
			}
			heap.Push(&topk, el)
		}
	}
	sort.Sort(sort.Reverse(topk))
	return Vector(topk)
}

// === bottomk(k ExprScalar, node ExprVector) Vector ===
func funcBottomk(ev *evaluator, args Expressions) Value {
	k := ev.evalInt(args[0])
	if k < 1 {
		return Vector{}
	}
	vector := ev.evalVector(args[1])

	bottomk := make(vectorByValueHeap, 0, k)
	bkHeap := reverseHeap{Interface: &bottomk}

	for _, el := range vector {
		if len(bottomk) < k || bottomk[0].Value > el.Value {
			if len(bottomk) == k {
				heap.Pop(&bkHeap)
			}
			heap.Push(&bkHeap, el)
		}
	}
	sort.Sort(bottomk)
	return Vector(bottomk)
}

// === drop_common_labels(node ExprVector) Vector ===
func funcDropCommonLabels(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	if len(vector) < 1 {
		return Vector{}
	}
	common := clientmodel.LabelSet{}
	for k, v := range vector[0].Metric.Metric {
		// TODO(julius): Should we also drop common metric names?
		if k == clientmodel.MetricNameLabel {
			continue
		}
		common[k] = v
	}

	for _, el := range vector[1:] {
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

	for _, el := range vector {
		for k := range el.Metric.Metric {
			if _, ok := common[k]; ok {
				el.Metric.Delete(k)
			}
		}
	}
	return vector
}

// === round(vector ExprVector, toNearest=1 Scalar) Vector ===
func funcRound(ev *evaluator, args Expressions) Value {
	// round returns a number rounded to toNearest.
	// Ties are solved by rounding up.
	toNearest := float64(1)
	if len(args) >= 2 {
		toNearest = ev.evalFloat(args[1])
	}
	// Invert as it seems to cause fewer floating point accuracy issues.
	toNearestInverse := 1.0 / toNearest

	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Floor(float64(el.Value)*toNearestInverse+0.5) / toNearestInverse)
	}
	return vector
}

// === scalar(node ExprVector) Scalar ===
func funcScalar(ev *evaluator, args Expressions) Value {
	v := ev.evalVector(args[0])
	if len(v) != 1 {
		return &Scalar{clientmodel.SampleValue(math.NaN()), ev.Timestamp}
	}
	return &Scalar{clientmodel.SampleValue(v[0].Value), ev.Timestamp}
}

// === count_scalar(vector ExprVector) model.SampleValue ===
func funcCountScalar(ev *evaluator, args Expressions) Value {
	return &Scalar{
		Value:     clientmodel.SampleValue(len(ev.evalVector(args[0]))),
		Timestamp: ev.Timestamp,
	}
}

func aggrOverTime(ev *evaluator, args Expressions, aggrFn func(metric.Values) clientmodel.SampleValue) Value {
	matrix := ev.evalMatrix(args[0])
	resultVector := Vector{}

	for _, el := range matrix {
		if len(el.Values) == 0 {
			continue
		}

		el.Metric.Delete(clientmodel.MetricNameLabel)
		resultVector = append(resultVector, &Sample{
			Metric:    el.Metric,
			Value:     aggrFn(el.Values),
			Timestamp: ev.Timestamp,
		})
	}
	return resultVector
}

// === avg_over_time(matrix ExprMatrix) Vector ===
func funcAvgOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values metric.Values) clientmodel.SampleValue {
		var sum clientmodel.SampleValue
		for _, v := range values {
			sum += v.Value
		}
		return sum / clientmodel.SampleValue(len(values))
	})
}

// === count_over_time(matrix ExprMatrix) Vector ===
func funcCountOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values metric.Values) clientmodel.SampleValue {
		return clientmodel.SampleValue(len(values))
	})
}

// === floor(vector ExprVector) Vector ===
func funcFloor(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Floor(float64(el.Value)))
	}
	return vector
}

// === max_over_time(matrix ExprMatrix) Vector ===
func funcMaxOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values metric.Values) clientmodel.SampleValue {
		max := math.Inf(-1)
		for _, v := range values {
			max = math.Max(max, float64(v.Value))
		}
		return clientmodel.SampleValue(max)
	})
}

// === min_over_time(matrix ExprMatrix) Vector ===
func funcMinOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values metric.Values) clientmodel.SampleValue {
		min := math.Inf(1)
		for _, v := range values {
			min = math.Min(min, float64(v.Value))
		}
		return clientmodel.SampleValue(min)
	})
}

// === sum_over_time(matrix ExprMatrix) Vector ===
func funcSumOverTime(ev *evaluator, args Expressions) Value {
	return aggrOverTime(ev, args, func(values metric.Values) clientmodel.SampleValue {
		var sum clientmodel.SampleValue
		for _, v := range values {
			sum += v.Value
		}
		return sum
	})
}

// === abs(vector ExprVector) Vector ===
func funcAbs(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Abs(float64(el.Value)))
	}
	return vector
}

// === absent(vector ExprVector) Vector ===
func funcAbsent(ev *evaluator, args Expressions) Value {
	if len(ev.evalVector(args[0])) > 0 {
		return Vector{}
	}
	m := clientmodel.Metric{}
	if vs, ok := args[0].(*VectorSelector); ok {
		for _, matcher := range vs.LabelMatchers {
			if matcher.Type == metric.Equal && matcher.Name != clientmodel.MetricNameLabel {
				m[matcher.Name] = matcher.Value
			}
		}
	}
	return Vector{
		&Sample{
			Metric: clientmodel.COWMetric{
				Metric: m,
				Copied: true,
			},
			Value:     1,
			Timestamp: ev.Timestamp,
		},
	}
}

// === ceil(vector ExprVector) Vector ===
func funcCeil(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Ceil(float64(el.Value)))
	}
	return vector
}

// === exp(vector ExprVector) Vector ===
func funcExp(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Exp(float64(el.Value)))
	}
	return vector
}

// === sqrt(vector VectorNode) Vector ===
func funcSqrt(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Sqrt(float64(el.Value)))
	}
	return vector
}

// === ln(vector ExprVector) Vector ===
func funcLn(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Log(float64(el.Value)))
	}
	return vector
}

// === log2(vector ExprVector) Vector ===
func funcLog2(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Log2(float64(el.Value)))
	}
	return vector
}

// === log10(vector ExprVector) Vector ===
func funcLog10(ev *evaluator, args Expressions) Value {
	vector := ev.evalVector(args[0])
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Log10(float64(el.Value)))
	}
	return vector
}

// === deriv(node ExprMatrix) Vector ===
func funcDeriv(ev *evaluator, args Expressions) Value {
	resultVector := Vector{}
	matrix := ev.evalMatrix(args[0])

	for _, samples := range matrix {
		// No sense in trying to compute a derivative without at least two points.
		// Drop this vector element.
		if len(samples.Values) < 2 {
			continue
		}

		// Least squares.
		n := clientmodel.SampleValue(0)
		sumY := clientmodel.SampleValue(0)
		sumX := clientmodel.SampleValue(0)
		sumXY := clientmodel.SampleValue(0)
		sumX2 := clientmodel.SampleValue(0)
		for _, sample := range samples.Values {
			x := clientmodel.SampleValue(sample.Timestamp.UnixNano() / 1e9)
			n += 1.0
			sumY += sample.Value
			sumX += x
			sumXY += x * sample.Value
			sumX2 += x * x
		}
		numerator := sumXY - sumX*sumY/n
		denominator := sumX2 - (sumX*sumX)/n

		resultValue := numerator / denominator

		resultSample := &Sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: ev.Timestamp,
		}
		resultSample.Metric.Delete(clientmodel.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === predict_linear(node ExprMatrix, k ExprScalar) Vector ===
func funcPredictLinear(ev *evaluator, args Expressions) Value {
	vector := funcDeriv(ev, args[0:1]).(Vector)
	duration := clientmodel.SampleValue(clientmodel.SampleValue(ev.evalFloat(args[1])))

	excludedLabels := map[clientmodel.LabelName]struct{}{
		clientmodel.MetricNameLabel: {},
	}

	// Calculate predicted delta over the duration.
	signatureToDelta := map[uint64]clientmodel.SampleValue{}
	for _, el := range vector {
		signature := clientmodel.SignatureWithoutLabels(el.Metric.Metric, excludedLabels)
		signatureToDelta[signature] = el.Value * duration
	}

	// add predicted delta to last value.
	matrixBounds := ev.evalMatrixBounds(args[0])
	outVec := make(Vector, 0, len(signatureToDelta))
	for _, samples := range matrixBounds {
		if len(samples.Values) < 2 {
			continue
		}
		signature := clientmodel.SignatureWithoutLabels(samples.Metric.Metric, excludedLabels)
		delta, ok := signatureToDelta[signature]
		if ok {
			samples.Metric.Delete(clientmodel.MetricNameLabel)
			outVec = append(outVec, &Sample{
				Metric:    samples.Metric,
				Value:     delta + samples.Values[1].Value,
				Timestamp: ev.Timestamp,
			})
		}
	}
	return outVec
}

// === histogram_quantile(k ExprScalar, vector ExprVector) Vector ===
func funcHistogramQuantile(ev *evaluator, args Expressions) Value {
	q := clientmodel.SampleValue(ev.evalFloat(args[0]))
	inVec := ev.evalVector(args[1])

	outVec := Vector{}
	signatureToMetricWithBuckets := map[uint64]*metricWithBuckets{}
	for _, el := range inVec {
		upperBound, err := strconv.ParseFloat(
			string(el.Metric.Metric[clientmodel.BucketLabel]), 64,
		)
		if err != nil {
			// Oops, no bucket label or malformed label value. Skip.
			// TODO(beorn7): Issue a warning somehow.
			continue
		}
		signature := clientmodel.SignatureWithoutLabels(el.Metric.Metric, excludedLabels)
		mb, ok := signatureToMetricWithBuckets[signature]
		if !ok {
			el.Metric.Delete(clientmodel.BucketLabel)
			el.Metric.Delete(clientmodel.MetricNameLabel)
			mb = &metricWithBuckets{el.Metric, nil}
			signatureToMetricWithBuckets[signature] = mb
		}
		mb.buckets = append(mb.buckets, bucket{upperBound, el.Value})
	}

	for _, mb := range signatureToMetricWithBuckets {
		outVec = append(outVec, &Sample{
			Metric:    mb.metric,
			Value:     clientmodel.SampleValue(quantile(q, mb.buckets)),
			Timestamp: ev.Timestamp,
		})
	}

	return outVec
}

// === resets(matrix ExprMatrix) Vector ===
func funcResets(ev *evaluator, args Expressions) Value {
	in := ev.evalMatrix(args[0])
	out := make(Vector, 0, len(in))

	for _, samples := range in {
		resets := 0
		prev := clientmodel.SampleValue(samples.Values[0].Value)
		for _, sample := range samples.Values[1:] {
			current := sample.Value
			if current < prev {
				resets++
			}
			prev = current
		}

		rs := &Sample{
			Metric:    samples.Metric,
			Value:     clientmodel.SampleValue(resets),
			Timestamp: ev.Timestamp,
		}
		rs.Metric.Delete(clientmodel.MetricNameLabel)
		out = append(out, rs)
	}
	return out
}

// === changes(matrix ExprMatrix) Vector ===
func funcChanges(ev *evaluator, args Expressions) Value {
	in := ev.evalMatrix(args[0])
	out := make(Vector, 0, len(in))

	for _, samples := range in {
		changes := 0
		prev := clientmodel.SampleValue(samples.Values[0].Value)
		for _, sample := range samples.Values[1:] {
			current := sample.Value
			if current != prev {
				changes++
			}
			prev = current
		}

		rs := &Sample{
			Metric:    samples.Metric,
			Value:     clientmodel.SampleValue(changes),
			Timestamp: ev.Timestamp,
		}
		rs.Metric.Delete(clientmodel.MetricNameLabel)
		out = append(out, rs)
	}
	return out
}

var functions = map[string]*Function{
	"abs": {
		Name:       "abs",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcAbs,
	},
	"absent": {
		Name:       "absent",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcAbsent,
	},
	"increase": {
		Name:       "increase",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcIncrease,
	},
	"avg_over_time": {
		Name:       "avg_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcAvgOverTime,
	},
	"bottomk": {
		Name:       "bottomk",
		ArgTypes:   []ExprType{ExprScalar, ExprVector},
		ReturnType: ExprVector,
		Call:       funcBottomk,
	},
	"ceil": {
		Name:       "ceil",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcCeil,
	},
	"changes": {
		Name:       "changes",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcChanges,
	},
	"count_over_time": {
		Name:       "count_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcCountOverTime,
	},
	"count_scalar": {
		Name:       "count_scalar",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprScalar,
		Call:       funcCountScalar,
	},
	"delta": {
		Name:         "delta",
		ArgTypes:     []ExprType{ExprMatrix, ExprScalar},
		OptionalArgs: 1, // The 2nd argument is deprecated.
		ReturnType:   ExprVector,
		Call:         funcDelta,
	},
	"deriv": {
		Name:       "deriv",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcDeriv,
	},
	"drop_common_labels": {
		Name:       "drop_common_labels",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcDropCommonLabels,
	},
	"exp": {
		Name:       "exp",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcExp,
	},
	"floor": {
		Name:       "floor",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcFloor,
	},
	"histogram_quantile": {
		Name:       "histogram_quantile",
		ArgTypes:   []ExprType{ExprScalar, ExprVector},
		ReturnType: ExprVector,
		Call:       funcHistogramQuantile,
	},
	"ln": {
		Name:       "ln",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcLn,
	},
	"log10": {
		Name:       "log10",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcLog10,
	},
	"log2": {
		Name:       "log2",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcLog2,
	},
	"max_over_time": {
		Name:       "max_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcMaxOverTime,
	},
	"min_over_time": {
		Name:       "min_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcMinOverTime,
	},
	"predict_linear": {
		Name:       "predict_linear",
		ArgTypes:   []ExprType{ExprMatrix, ExprScalar},
		ReturnType: ExprVector,
		Call:       funcPredictLinear,
	},
	"rate": {
		Name:       "rate",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcRate,
	},
	"resets": {
		Name:       "resets",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcResets,
	},
	"round": {
		Name:         "round",
		ArgTypes:     []ExprType{ExprVector, ExprScalar},
		OptionalArgs: 1,
		ReturnType:   ExprVector,
		Call:         funcRound,
	},
	"scalar": {
		Name:       "scalar",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprScalar,
		Call:       funcScalar,
	},
	"sort": {
		Name:       "sort",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcSort,
	},
	"sort_desc": {
		Name:       "sort_desc",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcSortDesc,
	},
	"sqrt": {
		Name:       "sqrt",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       funcSqrt,
	},
	"sum_over_time": {
		Name:       "sum_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       funcSumOverTime,
	},
	"time": {
		Name:       "time",
		ArgTypes:   []ExprType{},
		ReturnType: ExprScalar,
		Call:       funcTime,
	},
	"topk": {
		Name:       "topk",
		ArgTypes:   []ExprType{ExprScalar, ExprVector},
		ReturnType: ExprVector,
		Call:       funcTopk,
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
	if math.IsNaN(float64(s[i].Value)) {
		return true
	}
	return s[i].Value < s[j].Value
}

func (s vectorByValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByValueHeap) Push(x interface{}) {
	*s = append(*s, x.(*Sample))
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
