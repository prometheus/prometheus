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
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/facette/natsort"
	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"golang.org/x/exp/slices"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/annotations"
)

// FunctionCall is the type of a PromQL function implementation
//
// vals is a list of the evaluated arguments for the function call.
//
// For range vectors it will be a Matrix with one series, instant vectors a
// Vector, scalars a Vector with one series whose value is the scalar
// value,and nil for strings.
//
// args are the original arguments to the function, where you can access
// matrixSelectors, vectorSelectors, and StringLiterals.
//
// enh.Out is a pre-allocated empty vector that you may use to accumulate
// output before returning it. The vectors in vals should not be returned.a
//
// Range vector functions need only return a vector with the right value,
// the metric and timestamp are not needed.
//
// Instant vector functions need only return a vector with the right values and
// metrics, the timestamp are not needed.
//
// Scalar results should be returned as the value of a sample in a Vector.
type FunctionCall func(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations)

// === time() float64 ===
func funcTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return Vector{Sample{
		F: float64(enh.Ts) / 1000,
	}}, nil
}

// extrapolatedRate is a utility function for rate/increase/delta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// extrapolates if the first/last sample is close to the boundary, and returns
// the result as either per-second (if isRate is true) or overall.
func extrapolatedRate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper, isCounter, isRate bool) (Vector, annotations.Annotations) {
	ms := args[0].(*parser.MatrixSelector)
	vs := ms.VectorSelector.(*parser.VectorSelector)
	var (
		samples            = vals[0].(Matrix)[0]
		rangeStart         = enh.Ts - durationMilliseconds(ms.Range+vs.Offset)
		rangeEnd           = enh.Ts - durationMilliseconds(vs.Offset)
		resultFloat        float64
		resultHistogram    *histogram.FloatHistogram
		firstT, lastT      int64
		numSamplesMinusOne int
		annos              annotations.Annotations
	)

	// We need either at least two Histograms and no Floats, or at least two
	// Floats and no Histograms to calculate a rate. Otherwise, drop this
	// Vector element.
	metricName := samples.Metric.Get(labels.MetricName)
	if len(samples.Histograms) > 0 && len(samples.Floats) > 0 {
		return enh.Out, annos.Add(annotations.NewMixedFloatsHistogramsWarning(metricName, args[0].PositionRange()))
	}

	switch {
	case len(samples.Histograms) > 1:
		numSamplesMinusOne = len(samples.Histograms) - 1
		firstT = samples.Histograms[0].T
		lastT = samples.Histograms[numSamplesMinusOne].T
		var newAnnos annotations.Annotations
		resultHistogram, newAnnos = histogramRate(samples.Histograms, isCounter, metricName, args[0].PositionRange())
		if resultHistogram == nil {
			// The histograms are not compatible with each other.
			return enh.Out, annos.Merge(newAnnos)
		}
	case len(samples.Floats) > 1:
		numSamplesMinusOne = len(samples.Floats) - 1
		firstT = samples.Floats[0].T
		lastT = samples.Floats[numSamplesMinusOne].T
		resultFloat = samples.Floats[numSamplesMinusOne].F - samples.Floats[0].F
		if !isCounter {
			break
		}
		// Handle counter resets:
		prevValue := samples.Floats[0].F
		for _, currPoint := range samples.Floats[1:] {
			if currPoint.F < prevValue {
				resultFloat += prevValue
			}
			prevValue = currPoint.F
		}
	default:
		// TODO: add RangeTooShortWarning
		return enh.Out, annos
	}

	// Duration between first/last samples and boundary of range.
	durationToStart := float64(firstT-rangeStart) / 1000
	durationToEnd := float64(rangeEnd-lastT) / 1000

	sampledInterval := float64(lastT-firstT) / 1000
	averageDurationBetweenSamples := sampledInterval / float64(numSamplesMinusOne)

	// TODO(beorn7): Do this for histograms, too.
	if isCounter && resultFloat > 0 && len(samples.Floats) > 0 && samples.Floats[0].F >= 0 {
		// Counters cannot be negative. If we have any slope at all
		// (i.e. resultFloat went up), we can extrapolate the zero point
		// of the counter. If the duration to the zero point is shorter
		// than the durationToStart, we take the zero point as the start
		// of the series, thereby avoiding extrapolation to negative
		// counter values.
		durationToZero := sampledInterval * (samples.Floats[0].F / resultFloat)
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
	factor := extrapolateToInterval / sampledInterval
	if isRate {
		factor /= ms.Range.Seconds()
	}
	if resultHistogram == nil {
		resultFloat *= factor
	} else {
		resultHistogram.Mul(factor)
	}

	return append(enh.Out, Sample{F: resultFloat, H: resultHistogram}), annos
}

// histogramRate is a helper function for extrapolatedRate. It requires
// points[0] to be a histogram. It returns nil if any other Point in points is
// not a histogram, and a warning wrapped in an annotation in that case.
// Otherwise, it returns the calculated histogram and an empty annotation.
func histogramRate(points []HPoint, isCounter bool, metricName string, pos posrange.PositionRange) (*histogram.FloatHistogram, annotations.Annotations) {
	prev := points[0].H
	last := points[len(points)-1].H
	if last == nil {
		return nil, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(metricName, pos))
	}
	minSchema := prev.Schema
	if last.Schema < minSchema {
		minSchema = last.Schema
	}

	var annos annotations.Annotations

	// First iteration to find out two things:
	// - What's the smallest relevant schema?
	// - Are all data points histograms?
	//   TODO(beorn7): Find a way to check that earlier, e.g. by handing in a
	//   []FloatPoint and a []HistogramPoint separately.
	for _, currPoint := range points[1 : len(points)-1] {
		curr := currPoint.H
		if curr == nil {
			return nil, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(metricName, pos))
		}
		if !isCounter {
			continue
		}
		if curr.CounterResetHint == histogram.GaugeType {
			annos.Add(annotations.NewNativeHistogramNotCounterWarning(metricName, pos))
		}
		if curr.Schema < minSchema {
			minSchema = curr.Schema
		}
	}

	h := last.CopyToSchema(minSchema)
	h.Sub(prev)

	if isCounter {
		// Second iteration to deal with counter resets.
		for _, currPoint := range points[1:] {
			curr := currPoint.H
			if curr.DetectReset(prev) {
				h.Add(prev)
			}
			prev = curr
		}
	} else if points[0].H.CounterResetHint != histogram.GaugeType || points[len(points)-1].H.CounterResetHint != histogram.GaugeType {
		annos.Add(annotations.NewNativeHistogramNotGaugeWarning(metricName, pos))
	}

	h.CounterResetHint = histogram.GaugeType
	return h.Compact(0), nil
}

// === delta(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcDelta(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extrapolatedRate(vals, args, enh, false, false)
}

// === rate(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcRate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extrapolatedRate(vals, args, enh, true, true)
}

// === increase(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcIncrease(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extrapolatedRate(vals, args, enh, true, false)
}

// === irate(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcIrate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return instantValue(vals, enh.Out, true)
}

// === idelta(node model.ValMatrix) (Vector, Annotations) ===
func funcIdelta(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return instantValue(vals, enh.Out, false)
}

func instantValue(vals []parser.Value, out Vector, isRate bool) (Vector, annotations.Annotations) {
	samples := vals[0].(Matrix)[0]
	// No sense in trying to compute a rate without at least two points. Drop
	// this Vector element.
	// TODO: add RangeTooShortWarning
	if len(samples.Floats) < 2 {
		return out, nil
	}

	lastSample := samples.Floats[len(samples.Floats)-1]
	previousSample := samples.Floats[len(samples.Floats)-2]

	var resultValue float64
	if isRate && lastSample.F < previousSample.F {
		// Counter reset.
		resultValue = lastSample.F
	} else {
		resultValue = lastSample.F - previousSample.F
	}

	sampledInterval := lastSample.T - previousSample.T
	if sampledInterval == 0 {
		// Avoid dividing by 0.
		return out, nil
	}

	if isRate {
		// Convert to per-second.
		resultValue /= float64(sampledInterval) / 1000
	}

	return append(out, Sample{F: resultValue}), nil
}

// Calculate the trend value at the given index i in raw data d.
// This is somewhat analogous to the slope of the trend at the given index.
// The argument "tf" is the trend factor.
// The argument "s0" is the computed smoothed value.
// The argument "s1" is the computed trend factor.
// The argument "b" is the raw input value.
func calcTrendValue(i int, tf, s0, s1, b float64) float64 {
	if i == 0 {
		return b
	}

	x := tf * (s1 - s0)
	y := (1 - tf) * b

	return x + y
}

// Holt-Winters is similar to a weighted moving average, where historical data has exponentially less influence on the current data.
// Holt-Winter also accounts for trends in data. The smoothing factor (0 < sf < 1) affects how historical data will affect the current
// data. A lower smoothing factor increases the influence of historical data. The trend factor (0 < tf < 1) affects
// how trends in historical data will affect the current data. A higher trend factor increases the influence.
// of trends. Algorithm taken from https://en.wikipedia.org/wiki/Exponential_smoothing titled: "Double exponential smoothing".
func funcHoltWinters(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	samples := vals[0].(Matrix)[0]

	// The smoothing factor argument.
	sf := vals[1].(Vector)[0].F

	// The trend factor argument.
	tf := vals[2].(Vector)[0].F

	// Check that the input parameters are valid.
	if sf <= 0 || sf >= 1 {
		panic(fmt.Errorf("invalid smoothing factor. Expected: 0 < sf < 1, got: %f", sf))
	}
	if tf <= 0 || tf >= 1 {
		panic(fmt.Errorf("invalid trend factor. Expected: 0 < tf < 1, got: %f", tf))
	}

	l := len(samples.Floats)

	// Can't do the smoothing operation with less than two points.
	if l < 2 {
		return enh.Out, nil
	}

	var s0, s1, b float64
	// Set initial values.
	s1 = samples.Floats[0].F
	b = samples.Floats[1].F - samples.Floats[0].F

	// Run the smoothing operation.
	var x, y float64
	for i := 1; i < l; i++ {

		// Scale the raw value against the smoothing factor.
		x = sf * samples.Floats[i].F

		// Scale the last smoothed value with the trend at this point.
		b = calcTrendValue(i-1, tf, s0, s1, b)
		y = (1 - sf) * (s1 + b)

		s0, s1 = s1, x+y
	}

	return append(enh.Out, Sample{F: s1}), nil
}

// === sort(node parser.ValueTypeVector) (Vector, Annotations) ===
func funcSort(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	// NaN should sort to the bottom, so take descending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByReverseValueHeap(vals[0].(Vector))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter), nil
}

// === sortDesc(node parser.ValueTypeVector) (Vector, Annotations) ===
func funcSortDesc(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	// NaN should sort to the bottom, so take ascending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByValueHeap(vals[0].(Vector))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter), nil
}

// === sort_by_label(vector parser.ValueTypeVector, label parser.ValueTypeString...) (Vector, Annotations) ===
func funcSortByLabel(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	// In case the labels are the same, NaN should sort to the bottom, so take
	// ascending sort with NaN first and reverse it.
	var anno annotations.Annotations
	vals[0], anno = funcSort(vals, args, enh)
	labels := stringSliceFromArgs(args[1:])
	slices.SortFunc(vals[0].(Vector), func(a, b Sample) int {
		// Iterate over each given label
		for _, label := range labels {
			lv1 := a.Metric.Get(label)
			lv2 := b.Metric.Get(label)

			if lv1 == lv2 {
				continue
			}

			if natsort.Compare(lv1, lv2) {
				return -1
			}

			return +1
		}

		return 0
	})

	return vals[0].(Vector), anno
}

// === sort_by_label_desc(vector parser.ValueTypeVector, label parser.ValueTypeString...) (Vector, Annotations) ===
func funcSortByLabelDesc(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	// In case the labels are the same, NaN should sort to the bottom, so take
	// ascending sort with NaN first and reverse it.
	var anno annotations.Annotations
	vals[0], anno = funcSortDesc(vals, args, enh)
	labels := stringSliceFromArgs(args[1:])
	slices.SortFunc(vals[0].(Vector), func(a, b Sample) int {
		// Iterate over each given label
		for _, label := range labels {
			lv1 := a.Metric.Get(label)
			lv2 := b.Metric.Get(label)

			if lv1 == lv2 {
				continue
			}

			if natsort.Compare(lv1, lv2) {
				return +1
			}

			return -1
		}

		return 0
	})

	return vals[0].(Vector), anno
}

// === clamp(Vector parser.ValueTypeVector, min, max Scalar) (Vector, Annotations) ===
func funcClamp(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vals[0].(Vector)
	min := vals[1].(Vector)[0].F
	max := vals[2].(Vector)[0].F
	if max < min {
		return enh.Out, nil
	}
	for _, el := range vec {
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(el.Metric),
			F:      math.Max(min, math.Min(max, el.F)),
		})
	}
	return enh.Out, nil
}

// === clamp_max(Vector parser.ValueTypeVector, max Scalar) (Vector, Annotations) ===
func funcClampMax(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vals[0].(Vector)
	max := vals[1].(Vector)[0].F
	for _, el := range vec {
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(el.Metric),
			F:      math.Min(max, el.F),
		})
	}
	return enh.Out, nil
}

// === clamp_min(Vector parser.ValueTypeVector, min Scalar) (Vector, Annotations) ===
func funcClampMin(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vals[0].(Vector)
	min := vals[1].(Vector)[0].F
	for _, el := range vec {
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(el.Metric),
			F:      math.Max(min, el.F),
		})
	}
	return enh.Out, nil
}

// === round(Vector parser.ValueTypeVector, toNearest=1 Scalar) (Vector, Annotations) ===
func funcRound(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vals[0].(Vector)
	// round returns a number rounded to toNearest.
	// Ties are solved by rounding up.
	toNearest := float64(1)
	if len(args) >= 2 {
		toNearest = vals[1].(Vector)[0].F
	}
	// Invert as it seems to cause fewer floating point accuracy issues.
	toNearestInverse := 1.0 / toNearest

	for _, el := range vec {
		f := math.Floor(el.F*toNearestInverse+0.5) / toNearestInverse
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(el.Metric),
			F:      f,
		})
	}
	return enh.Out, nil
}

// === Scalar(node parser.ValueTypeVector) Scalar ===
func funcScalar(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	v := vals[0].(Vector)
	if len(v) != 1 {
		return append(enh.Out, Sample{F: math.NaN()}), nil
	}
	return append(enh.Out, Sample{F: v[0].F}), nil
}

func aggrOverTime(vals []parser.Value, enh *EvalNodeHelper, aggrFn func(Series) float64) Vector {
	el := vals[0].(Matrix)[0]

	return append(enh.Out, Sample{F: aggrFn(el)})
}

func aggrHistOverTime(vals []parser.Value, enh *EvalNodeHelper, aggrFn func(Series) *histogram.FloatHistogram) Vector {
	el := vals[0].(Matrix)[0]

	return append(enh.Out, Sample{H: aggrFn(el)})
}

// === avg_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations)  ===
func funcAvgOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	firstSeries := vals[0].(Matrix)[0]
	if len(firstSeries.Floats) > 0 && len(firstSeries.Histograms) > 0 {
		metricName := firstSeries.Metric.Get(labels.MetricName)
		return enh.Out, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(metricName, args[0].PositionRange()))
	}
	if len(firstSeries.Floats) == 0 {
		// The passed values only contain histograms.
		return aggrHistOverTime(vals, enh, func(s Series) *histogram.FloatHistogram {
			count := 1
			mean := s.Histograms[0].H.Copy()
			for _, h := range s.Histograms[1:] {
				count++
				left := h.H.Copy().Div(float64(count))
				right := mean.Copy().Div(float64(count))
				toAdd := left.Sub(right)
				mean.Add(toAdd)
			}
			return mean
		}), nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		var mean, count, c float64
		for _, f := range s.Floats {
			count++
			if math.IsInf(mean, 0) {
				if math.IsInf(f.F, 0) && (mean > 0) == (f.F > 0) {
					// The `mean` and `f.F` values are `Inf` of the same sign.  They
					// can't be subtracted, but the value of `mean` is correct
					// already.
					continue
				}
				if !math.IsInf(f.F, 0) && !math.IsNaN(f.F) {
					// At this stage, the mean is an infinite. If the added
					// value is neither an Inf or a Nan, we can keep that mean
					// value.
					// This is required because our calculation below removes
					// the mean value, which would look like Inf += x - Inf and
					// end up as a NaN.
					continue
				}
			}
			mean, c = kahanSumInc(f.F/count-mean/count, mean, c)
		}

		if math.IsInf(mean, 0) {
			return mean
		}
		return mean + c
	}), nil
}

// === count_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcCountOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return aggrOverTime(vals, enh, func(s Series) float64 {
		return float64(len(s.Floats) + len(s.Histograms))
	}), nil
}

// === last_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcLastOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	el := vals[0].(Matrix)[0]

	var f FPoint
	if len(el.Floats) > 0 {
		f = el.Floats[len(el.Floats)-1]
	}

	var h HPoint
	if len(el.Histograms) > 0 {
		h = el.Histograms[len(el.Histograms)-1]
	}

	if h.H == nil || h.T < f.T {
		return append(enh.Out, Sample{
			Metric: el.Metric,
			F:      f.F,
		}), nil
	}
	return append(enh.Out, Sample{
		Metric: el.Metric,
		H:      h.H,
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

// === max_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMaxOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. max_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		max := s.Floats[0].F
		for _, f := range s.Floats {
			if f.F > max || math.IsNaN(max) {
				max = f.F
			}
		}
		return max
	}), nil
}

// === min_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMinOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. min_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		min := s.Floats[0].F
		for _, f := range s.Floats {
			if f.F < min || math.IsNaN(min) {
				min = f.F
			}
		}
		return min
	}), nil
}

// === sum_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcSumOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	firstSeries := vals[0].(Matrix)[0]
	if len(firstSeries.Floats) > 0 && len(firstSeries.Histograms) > 0 {
		metricName := firstSeries.Metric.Get(labels.MetricName)
		return enh.Out, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(metricName, args[0].PositionRange()))
	}
	if len(firstSeries.Floats) == 0 {
		// The passed values only contain histograms.
		return aggrHistOverTime(vals, enh, func(s Series) *histogram.FloatHistogram {
			sum := s.Histograms[0].H.Copy()
			for _, h := range s.Histograms[1:] {
				sum.Add(h.H)
			}
			return sum
		}), nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		var sum, c float64
		for _, f := range s.Floats {
			sum, c = kahanSumInc(f.F, sum, c)
		}
		if math.IsInf(sum, 0) {
			return sum
		}
		return sum + c
	}), nil
}

// === quantile_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcQuantileOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	q := vals[0].(Vector)[0].F
	el := vals[1].(Matrix)[0]
	if len(el.Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. quantile_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}

	var annos annotations.Annotations
	if math.IsNaN(q) || q < 0 || q > 1 {
		annos.Add(annotations.NewInvalidQuantileWarning(q, args[0].PositionRange()))
	}

	values := make(vectorByValueHeap, 0, len(el.Floats))
	for _, f := range el.Floats {
		values = append(values, Sample{F: f.F})
	}
	return append(enh.Out, Sample{F: quantile(q, values)}), annos
}

// === stddev_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcStddevOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. stddev_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		var count float64
		var mean, cMean float64
		var aux, cAux float64
		for _, f := range s.Floats {
			count++
			delta := f.F - (mean + cMean)
			mean, cMean = kahanSumInc(delta/count, mean, cMean)
			aux, cAux = kahanSumInc(delta*(f.F-(mean+cMean)), aux, cAux)
		}
		return math.Sqrt((aux + cAux) / count)
	}), nil
}

// === stdvar_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcStdvarOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. stdvar_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		var count float64
		var mean, cMean float64
		var aux, cAux float64
		for _, f := range s.Floats {
			count++
			delta := f.F - (mean + cMean)
			mean, cMean = kahanSumInc(delta/count, mean, cMean)
			aux, cAux = kahanSumInc(delta*(f.F-(mean+cMean)), aux, cAux)
		}
		return (aux + cAux) / count
	}), nil
}

// === absent(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAbsent(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Vector)) > 0 {
		return enh.Out, nil
	}
	return append(enh.Out,
		Sample{
			Metric: createLabelsForAbsentFunction(args[0]),
			F:      1,
		}), nil
}

// === absent_over_time(Vector parser.ValueTypeMatrix) (Vector, Annotations) ===
// As this function has a matrix as argument, it does not get all the Series.
// This function will return 1 if the matrix has at least one element.
// Due to engine optimization, this function is only called when this condition is true.
// Then, the engine post-processes the results to get the expected output.
func funcAbsentOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return append(enh.Out, Sample{F: 1}), nil
}

// === present_over_time(Vector parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcPresentOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return aggrOverTime(vals, enh, func(s Series) float64 {
		return 1
	}), nil
}

func simpleFunc(vals []parser.Value, enh *EvalNodeHelper, f func(float64) float64) Vector {
	for _, el := range vals[0].(Vector) {
		if el.H == nil { // Process only float samples.
			enh.Out = append(enh.Out, Sample{
				Metric: enh.DropMetricName(el.Metric),
				F:      f(el.F),
			})
		}
	}
	return enh.Out
}

// === abs(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAbs(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Abs), nil
}

// === ceil(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcCeil(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Ceil), nil
}

// === floor(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcFloor(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Floor), nil
}

// === exp(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcExp(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Exp), nil
}

// === sqrt(Vector VectorNode) (Vector, Annotations) ===
func funcSqrt(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Sqrt), nil
}

// === ln(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcLn(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Log), nil
}

// === log2(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcLog2(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Log2), nil
}

// === log10(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcLog10(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Log10), nil
}

// === sin(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcSin(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Sin), nil
}

// === cos(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcCos(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Cos), nil
}

// === tan(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcTan(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Tan), nil
}

// === asin(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAsin(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Asin), nil
}

// === acos(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAcos(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Acos), nil
}

// === atan(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAtan(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Atan), nil
}

// === sinh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcSinh(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Sinh), nil
}

// === cosh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcCosh(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Cosh), nil
}

// === tanh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcTanh(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Tanh), nil
}

// === asinh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAsinh(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Asinh), nil
}

// === acosh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAcosh(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Acosh), nil
}

// === atanh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAtanh(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, math.Atanh), nil
}

// === rad(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcRad(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, func(v float64) float64 {
		return v * math.Pi / 180
	}), nil
}

// === deg(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcDeg(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, func(v float64) float64 {
		return v * 180 / math.Pi
	}), nil
}

// === pi() Scalar ===
func funcPi(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return Vector{Sample{F: math.Pi}}, nil
}

// === sgn(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcSgn(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFunc(vals, enh, func(v float64) float64 {
		switch {
		case v < 0:
			return -1
		case v > 0:
			return 1
		default:
			return v
		}
	}), nil
}

// === timestamp(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcTimestamp(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vals[0].(Vector)
	for _, el := range vec {
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(el.Metric),
			F:      float64(el.T) / 1000,
		})
	}
	return enh.Out, nil
}

func kahanSum(samples []float64) float64 {
	var sum, c float64

	for _, v := range samples {
		sum, c = kahanSumInc(v, sum, c)
	}
	return sum + c
}

func kahanSumInc(inc, sum, c float64) (newSum, newC float64) {
	t := sum + inc
	// Using Neumaier improvement, swap if next term larger than sum.
	if math.Abs(sum) >= math.Abs(inc) {
		c += (sum - t) + inc
	} else {
		c += (inc - t) + sum
	}
	return t, c
}

// linearRegression performs a least-square linear regression analysis on the
// provided SamplePairs. It returns the slope, and the intercept value at the
// provided time.
func linearRegression(samples []FPoint, interceptTime int64) (slope, intercept float64) {
	var (
		n          float64
		sumX, cX   float64
		sumY, cY   float64
		sumXY, cXY float64
		sumX2, cX2 float64
		initY      float64
		constY     bool
	)
	initY = samples[0].F
	constY = true
	for i, sample := range samples {
		// Set constY to false if any new y values are encountered.
		if constY && i > 0 && sample.F != initY {
			constY = false
		}
		n += 1.0
		x := float64(sample.T-interceptTime) / 1e3
		sumX, cX = kahanSumInc(x, sumX, cX)
		sumY, cY = kahanSumInc(sample.F, sumY, cY)
		sumXY, cXY = kahanSumInc(x*sample.F, sumXY, cXY)
		sumX2, cX2 = kahanSumInc(x*x, sumX2, cX2)
	}
	if constY {
		if math.IsInf(initY, 0) {
			return math.NaN(), math.NaN()
		}
		return 0, initY
	}
	sumX += cX
	sumY += cY
	sumXY += cXY
	sumX2 += cX2

	covXY := sumXY - sumX*sumY/n
	varX := sumX2 - sumX*sumX/n

	slope = covXY / varX
	intercept = sumY/n - slope*sumX/n
	return slope, intercept
}

// === deriv(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcDeriv(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	samples := vals[0].(Matrix)[0]

	// No sense in trying to compute a derivative without at least two points.
	// Drop this Vector element.
	if len(samples.Floats) < 2 {
		return enh.Out, nil
	}

	// We pass in an arbitrary timestamp that is near the values in use
	// to avoid floating point accuracy issues, see
	// https://github.com/prometheus/prometheus/issues/2674
	slope, _ := linearRegression(samples.Floats, samples.Floats[0].T)
	return append(enh.Out, Sample{F: slope}), nil
}

// === predict_linear(node parser.ValueTypeMatrix, k parser.ValueTypeScalar) (Vector, Annotations) ===
func funcPredictLinear(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	samples := vals[0].(Matrix)[0]
	duration := vals[1].(Vector)[0].F
	// No sense in trying to predict anything without at least two points.
	// Drop this Vector element.
	if len(samples.Floats) < 2 {
		return enh.Out, nil
	}
	slope, intercept := linearRegression(samples.Floats, enh.Ts)

	return append(enh.Out, Sample{F: slope*duration + intercept}), nil
}

// === histogram_count(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramCount(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	inVec := vals[0].(Vector)

	for _, sample := range inVec {
		// Skip non-histogram samples.
		if sample.H == nil {
			continue
		}
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(sample.Metric),
			F:      sample.H.Count,
		})
	}
	return enh.Out, nil
}

// === histogram_sum(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramSum(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	inVec := vals[0].(Vector)

	for _, sample := range inVec {
		// Skip non-histogram samples.
		if sample.H == nil {
			continue
		}
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(sample.Metric),
			F:      sample.H.Sum,
		})
	}
	return enh.Out, nil
}

// === histogram_stddev(Vector parser.ValueTypeVector) (Vector, Annotations)  ===
func funcHistogramStdDev(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	inVec := vals[0].(Vector)

	for _, sample := range inVec {
		// Skip non-histogram samples.
		if sample.H == nil {
			continue
		}
		mean := sample.H.Sum / sample.H.Count
		var variance, cVariance float64
		it := sample.H.AllBucketIterator()
		for it.Next() {
			bucket := it.At()
			var val float64
			if bucket.Lower <= 0 && 0 <= bucket.Upper {
				val = 0
			} else {
				val = math.Sqrt(bucket.Upper * bucket.Lower)
			}
			delta := val - mean
			variance, cVariance = kahanSumInc(bucket.Count*delta*delta, variance, cVariance)
		}
		variance += cVariance
		variance /= sample.H.Count
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(sample.Metric),
			F:      math.Sqrt(variance),
		})
	}
	return enh.Out, nil
}

// === histogram_stdvar(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramStdVar(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	inVec := vals[0].(Vector)

	for _, sample := range inVec {
		// Skip non-histogram samples.
		if sample.H == nil {
			continue
		}
		mean := sample.H.Sum / sample.H.Count
		var variance, cVariance float64
		it := sample.H.AllBucketIterator()
		for it.Next() {
			bucket := it.At()
			var val float64
			if bucket.Lower <= 0 && 0 <= bucket.Upper {
				val = 0
			} else {
				val = math.Sqrt(bucket.Upper * bucket.Lower)
			}
			delta := val - mean
			variance, cVariance = kahanSumInc(bucket.Count*delta*delta, variance, cVariance)
		}
		variance += cVariance
		variance /= sample.H.Count
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(sample.Metric),
			F:      variance,
		})
	}
	return enh.Out, nil
}

// === histogram_fraction(lower, upper parser.ValueTypeScalar, Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramFraction(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	lower := vals[0].(Vector)[0].F
	upper := vals[1].(Vector)[0].F
	inVec := vals[2].(Vector)

	for _, sample := range inVec {
		// Skip non-histogram samples.
		if sample.H == nil {
			continue
		}
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(sample.Metric),
			F:      histogramFraction(lower, upper, sample.H),
		})
	}
	return enh.Out, nil
}

// === histogram_quantile(k parser.ValueTypeScalar, Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramQuantile(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	q := vals[0].(Vector)[0].F
	inVec := vals[1].(Vector)
	var annos annotations.Annotations

	if math.IsNaN(q) || q < 0 || q > 1 {
		annos.Add(annotations.NewInvalidQuantileWarning(q, args[0].PositionRange()))
	}

	if enh.signatureToMetricWithBuckets == nil {
		enh.signatureToMetricWithBuckets = map[string]*metricWithBuckets{}
	} else {
		for _, v := range enh.signatureToMetricWithBuckets {
			v.buckets = v.buckets[:0]
		}
	}

	var histogramSamples []Sample

	for _, sample := range inVec {
		// We are only looking for classic buckets here. Remember
		// the histograms for later treatment.
		if sample.H != nil {
			histogramSamples = append(histogramSamples, sample)
			continue
		}

		upperBound, err := strconv.ParseFloat(
			sample.Metric.Get(model.BucketLabel), 64,
		)
		if err != nil {
			annos.Add(annotations.NewBadBucketLabelWarning(sample.Metric.Get(labels.MetricName), sample.Metric.Get(model.BucketLabel), args[1].PositionRange()))
			continue
		}
		enh.lblBuf = sample.Metric.BytesWithoutLabels(enh.lblBuf, labels.BucketLabel)
		mb, ok := enh.signatureToMetricWithBuckets[string(enh.lblBuf)]
		if !ok {
			sample.Metric = labels.NewBuilder(sample.Metric).
				Del(excludedLabels...).
				Labels()

			mb = &metricWithBuckets{sample.Metric, nil}
			enh.signatureToMetricWithBuckets[string(enh.lblBuf)] = mb
		}
		mb.buckets = append(mb.buckets, bucket{upperBound, sample.F})

	}

	// Now deal with the histograms.
	for _, sample := range histogramSamples {
		// We have to reconstruct the exact same signature as above for
		// a classic histogram, just ignoring any le label.
		enh.lblBuf = sample.Metric.Bytes(enh.lblBuf)
		if mb, ok := enh.signatureToMetricWithBuckets[string(enh.lblBuf)]; ok && len(mb.buckets) > 0 {
			// At this data point, we have classic histogram
			// buckets and a native histogram with the same name and
			// labels. Do not evaluate anything.
			annos.Add(annotations.NewMixedClassicNativeHistogramsWarning(sample.Metric.Get(labels.MetricName), args[1].PositionRange()))
			delete(enh.signatureToMetricWithBuckets, string(enh.lblBuf))
			continue
		}

		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(sample.Metric),
			F:      histogramQuantile(q, sample.H),
		})
	}

	for _, mb := range enh.signatureToMetricWithBuckets {
		if len(mb.buckets) > 0 {
			res, forcedMonotonicity, _ := bucketQuantile(q, mb.buckets)
			enh.Out = append(enh.Out, Sample{
				Metric: mb.metric,
				F:      res,
			})
			if forcedMonotonicity {
				annos.Add(annotations.NewHistogramQuantileForcedMonotonicityInfo(mb.metric.Get(labels.MetricName), args[1].PositionRange()))
			}
		}
	}

	return enh.Out, annos
}

// === resets(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcResets(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	floats := vals[0].(Matrix)[0].Floats
	histograms := vals[0].(Matrix)[0].Histograms
	resets := 0

	if len(floats) > 1 {
		prev := floats[0].F
		for _, sample := range floats[1:] {
			current := sample.F
			if current < prev {
				resets++
			}
			prev = current
		}
	}

	if len(histograms) > 1 {
		prev := histograms[0].H
		for _, sample := range histograms[1:] {
			current := sample.H
			if current.DetectReset(prev) {
				resets++
			}
			prev = current
		}
	}

	return append(enh.Out, Sample{F: float64(resets)}), nil
}

// === changes(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcChanges(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	floats := vals[0].(Matrix)[0].Floats
	changes := 0

	if len(floats) == 0 {
		// TODO(beorn7): Only histogram values, still need to add support.
		return enh.Out, nil
	}

	prev := floats[0].F
	for _, sample := range floats[1:] {
		current := sample.F
		if current != prev && !(math.IsNaN(current) && math.IsNaN(prev)) {
			changes++
		}
		prev = current
	}

	return append(enh.Out, Sample{F: float64(changes)}), nil
}

// === label_replace(Vector parser.ValueTypeVector, dst_label, replacement, src_labelname, regex parser.ValueTypeString) (Vector, Annotations) ===
func funcLabelReplace(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	var (
		vector   = vals[0].(Vector)
		dst      = stringFromArg(args[1])
		repl     = stringFromArg(args[2])
		src      = stringFromArg(args[3])
		regexStr = stringFromArg(args[4])
	)

	if enh.regex == nil {
		var err error
		enh.regex, err = regexp.Compile("^(?:" + regexStr + ")$")
		if err != nil {
			panic(fmt.Errorf("invalid regular expression in label_replace(): %s", regexStr))
		}
		if !model.LabelNameRE.MatchString(dst) {
			panic(fmt.Errorf("invalid destination label name in label_replace(): %s", dst))
		}
		enh.Dmn = make(map[uint64]labels.Labels, len(enh.Out))
	}

	for _, el := range vector {
		h := el.Metric.Hash()
		var outMetric labels.Labels
		if l, ok := enh.Dmn[h]; ok {
			outMetric = l
		} else {
			srcVal := el.Metric.Get(src)
			indexes := enh.regex.FindStringSubmatchIndex(srcVal)
			if indexes == nil {
				// If there is no match, no replacement should take place.
				outMetric = el.Metric
				enh.Dmn[h] = outMetric
			} else {
				res := enh.regex.ExpandString([]byte{}, repl, srcVal, indexes)

				lb := labels.NewBuilder(el.Metric).Del(dst)
				if len(res) > 0 {
					lb.Set(dst, string(res))
				}
				outMetric = lb.Labels()
				enh.Dmn[h] = outMetric
			}
		}

		enh.Out = append(enh.Out, Sample{
			Metric: outMetric,
			F:      el.F,
			H:      el.H,
		})
	}
	return enh.Out, nil
}

// === Vector(s Scalar) (Vector, Annotations) ===
func funcVector(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return append(enh.Out,
		Sample{
			Metric: labels.Labels{},
			F:      vals[0].(Vector)[0].F,
		}), nil
}

// === label_join(vector model.ValVector, dest_labelname, separator, src_labelname...) (Vector, Annotations) ===
func funcLabelJoin(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	var (
		vector    = vals[0].(Vector)
		dst       = stringFromArg(args[1])
		sep       = stringFromArg(args[2])
		srcLabels = make([]string, len(args)-3)
	)

	if enh.Dmn == nil {
		enh.Dmn = make(map[uint64]labels.Labels, len(enh.Out))
	}

	for i := 3; i < len(args); i++ {
		src := stringFromArg(args[i])
		if !model.LabelName(src).IsValid() {
			panic(fmt.Errorf("invalid source label name in label_join(): %s", src))
		}
		srcLabels[i-3] = src
	}

	if !model.LabelName(dst).IsValid() {
		panic(fmt.Errorf("invalid destination label name in label_join(): %s", dst))
	}

	srcVals := make([]string, len(srcLabels))
	for _, el := range vector {
		h := el.Metric.Hash()
		var outMetric labels.Labels
		if l, ok := enh.Dmn[h]; ok {
			outMetric = l
		} else {

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

			outMetric = lb.Labels()
			enh.Dmn[h] = outMetric
		}

		enh.Out = append(enh.Out, Sample{
			Metric: outMetric,
			F:      el.F,
			H:      el.H,
		})
	}
	return enh.Out, nil
}

// Common code for date related functions.
func dateWrapper(vals []parser.Value, enh *EvalNodeHelper, f func(time.Time) float64) Vector {
	if len(vals) == 0 {
		return append(enh.Out,
			Sample{
				Metric: labels.Labels{},
				F:      f(time.Unix(enh.Ts/1000, 0).UTC()),
			})
	}

	for _, el := range vals[0].(Vector) {
		t := time.Unix(int64(el.F), 0).UTC()
		enh.Out = append(enh.Out, Sample{
			Metric: enh.DropMetricName(el.Metric),
			F:      f(t),
		})
	}
	return enh.Out
}

// === days_in_month(v Vector) Scalar ===
func funcDaysInMonth(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(32 - time.Date(t.Year(), t.Month(), 32, 0, 0, 0, 0, time.UTC).Day())
	}), nil
}

// === day_of_month(v Vector) Scalar ===
func funcDayOfMonth(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.Day())
	}), nil
}

// === day_of_week(v Vector) Scalar ===
func funcDayOfWeek(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.Weekday())
	}), nil
}

// === day_of_year(v Vector) Scalar ===
func funcDayOfYear(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.YearDay())
	}), nil
}

// === hour(v Vector) Scalar ===
func funcHour(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.Hour())
	}), nil
}

// === minute(v Vector) Scalar ===
func funcMinute(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.Minute())
	}), nil
}

// === month(v Vector) Scalar ===
func funcMonth(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.Month())
	}), nil
}

// === year(v Vector) Scalar ===
func funcYear(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vals, enh, func(t time.Time) float64 {
		return float64(t.Year())
	}), nil
}

// FunctionCalls is a list of all functions supported by PromQL, including their types.
var FunctionCalls = map[string]FunctionCall{
	"abs":                funcAbs,
	"absent":             funcAbsent,
	"absent_over_time":   funcAbsentOverTime,
	"acos":               funcAcos,
	"acosh":              funcAcosh,
	"asin":               funcAsin,
	"asinh":              funcAsinh,
	"atan":               funcAtan,
	"atanh":              funcAtanh,
	"avg_over_time":      funcAvgOverTime,
	"ceil":               funcCeil,
	"changes":            funcChanges,
	"clamp":              funcClamp,
	"clamp_max":          funcClampMax,
	"clamp_min":          funcClampMin,
	"cos":                funcCos,
	"cosh":               funcCosh,
	"count_over_time":    funcCountOverTime,
	"days_in_month":      funcDaysInMonth,
	"day_of_month":       funcDayOfMonth,
	"day_of_week":        funcDayOfWeek,
	"day_of_year":        funcDayOfYear,
	"deg":                funcDeg,
	"delta":              funcDelta,
	"deriv":              funcDeriv,
	"exp":                funcExp,
	"floor":              funcFloor,
	"histogram_count":    funcHistogramCount,
	"histogram_fraction": funcHistogramFraction,
	"histogram_quantile": funcHistogramQuantile,
	"histogram_sum":      funcHistogramSum,
	"histogram_stddev":   funcHistogramStdDev,
	"histogram_stdvar":   funcHistogramStdVar,
	"holt_winters":       funcHoltWinters,
	"hour":               funcHour,
	"idelta":             funcIdelta,
	"increase":           funcIncrease,
	"irate":              funcIrate,
	"label_replace":      funcLabelReplace,
	"label_join":         funcLabelJoin,
	"ln":                 funcLn,
	"log10":              funcLog10,
	"log2":               funcLog2,
	"last_over_time":     funcLastOverTime,
	"mad_over_time":      funcMadOverTime,
	"max_over_time":      funcMaxOverTime,
	"min_over_time":      funcMinOverTime,
	"minute":             funcMinute,
	"month":              funcMonth,
	"pi":                 funcPi,
	"predict_linear":     funcPredictLinear,
	"present_over_time":  funcPresentOverTime,
	"quantile_over_time": funcQuantileOverTime,
	"rad":                funcRad,
	"rate":               funcRate,
	"resets":             funcResets,
	"round":              funcRound,
	"scalar":             funcScalar,
	"sgn":                funcSgn,
	"sin":                funcSin,
	"sinh":               funcSinh,
	"sort":               funcSort,
	"sort_desc":          funcSortDesc,
	"sort_by_label":      funcSortByLabel,
	"sort_by_label_desc": funcSortByLabelDesc,
	"sqrt":               funcSqrt,
	"stddev_over_time":   funcStddevOverTime,
	"stdvar_over_time":   funcStdvarOverTime,
	"sum_over_time":      funcSumOverTime,
	"tan":                funcTan,
	"tanh":               funcTanh,
	"time":               funcTime,
	"timestamp":          funcTimestamp,
	"vector":             funcVector,
	"year":               funcYear,
}

// AtModifierUnsafeFunctions are the functions whose result
// can vary if evaluation time is changed when the arguments are
// step invariant. It also includes functions that use the timestamps
// of the passed instant vector argument to calculate a result since
// that can also change with change in eval time.
var AtModifierUnsafeFunctions = map[string]struct{}{
	// Step invariant functions.
	"days_in_month": {}, "day_of_month": {}, "day_of_week": {}, "day_of_year": {},
	"hour": {}, "minute": {}, "month": {}, "year": {},
	"predict_linear": {}, "time": {},
	// Uses timestamp of the argument for the result,
	// hence unsafe to use with @ modifier.
	"timestamp": {},
}

type vectorByValueHeap Vector

func (s vectorByValueHeap) Len() int {
	return len(s)
}

func (s vectorByValueHeap) Less(i, j int) bool {
	// We compare histograms based on their sum of observations.
	// TODO(beorn7): Is that what we want?
	vi, vj := s[i].F, s[j].F
	if s[i].H != nil {
		vi = s[i].H.Sum
	}
	if s[j].H != nil {
		vj = s[j].H.Sum
	}

	if math.IsNaN(vi) {
		return true
	}
	return vi < vj
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
	// We compare histograms based on their sum of observations.
	// TODO(beorn7): Is that what we want?
	vi, vj := s[i].F, s[j].F
	if s[i].H != nil {
		vi = s[i].H.Sum
	}
	if s[j].H != nil {
		vj = s[j].H.Sum
	}

	if math.IsNaN(vi) {
		return true
	}
	return vi > vj
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

// createLabelsForAbsentFunction returns the labels that are uniquely and exactly matched
// in a given expression. It is used in the absent functions.
func createLabelsForAbsentFunction(expr parser.Expr) labels.Labels {
	b := labels.NewBuilder(labels.EmptyLabels())

	var lm []*labels.Matcher
	switch n := expr.(type) {
	case *parser.VectorSelector:
		lm = n.LabelMatchers
	case *parser.MatrixSelector:
		lm = n.VectorSelector.(*parser.VectorSelector).LabelMatchers
	default:
		return labels.EmptyLabels()
	}

	// The 'has' map implements backwards-compatibility for historic behaviour:
	// e.g. in `absent(x{job="a",job="b",foo="bar"})` then `job` is removed from the output.
	// Note this gives arguably wrong behaviour for `absent(x{job="a",job="a",foo="bar"})`.
	has := make(map[string]bool, len(lm))
	for _, ma := range lm {
		if ma.Name == labels.MetricName {
			continue
		}
		if ma.Type == labels.MatchEqual && !has[ma.Name] {
			b.Set(ma.Name, ma.Value)
			has[ma.Name] = true
		} else {
			b.Del(ma.Name)
		}
	}

	return b.Labels()
}

func stringFromArg(e parser.Expr) string {
	tmp := unwrapStepInvariantExpr(e) // Unwrap StepInvariant
	unwrapParenExpr(&tmp)             // Optionally unwrap ParenExpr
	return tmp.(*parser.StringLiteral).Val
}

func stringSliceFromArgs(args parser.Expressions) []string {
	tmp := make([]string, len(args))
	for i := 0; i < len(args); i++ {
		tmp[i] = stringFromArg(args[i])
	}
	return tmp
}
