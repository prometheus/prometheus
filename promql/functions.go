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

package promql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/facette/natsort"
	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/kahansum"
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
type FunctionCall func(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations)

// === time() float64 ===
func funcTime(_ []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return Vector{Sample{
		F: float64(enh.Ts) / 1000,
	}}, nil
}

// pickOrInterpolateLeft returns the value at the left boundary of the range.
// If interpolation is needed (when smoothed is true and the first sample is before the range start),
// it returns the interpolated value at the left boundary; otherwise, it returns the first sample's value.
func pickOrInterpolateLeft(floats []FPoint, first int, rangeStart int64, smoothed, isCounter bool) float64 {
	if smoothed && floats[first].T < rangeStart {
		return interpolate(floats[first], floats[first+1], rangeStart, isCounter)
	}
	return floats[first].F
}

// pickOrInterpolateRight returns the value at the right boundary of the range.
// If interpolation is needed (when smoothed is true and the last sample is after the range end),
// it returns the interpolated value at the right boundary; otherwise, it returns the last sample's value.
func pickOrInterpolateRight(floats []FPoint, last int, rangeEnd int64, smoothed, isCounter bool) float64 {
	if smoothed && last > 0 && floats[last].T > rangeEnd {
		return interpolate(floats[last-1], floats[last], rangeEnd, isCounter)
	}
	return floats[last].F
}

// interpolate performs linear interpolation between two points.
// If isCounter is true and there is a counter reset, it models the counter
// as starting from 0 (post-reset) by setting y1 to 0.
// It then calculates the interpolated value at the given timestamp.
func interpolate(p1, p2 FPoint, t int64, isCounter bool) float64 {
	y1 := p1.F
	y2 := p2.F
	if isCounter && y2 < y1 {
		y1 = 0
	}

	return y1 + (y2-y1)*float64(t-p1.T)/float64(p2.T-p1.T)
}

// interpolateHistograms performs linear interpolation between two histogram points.
// If isCounter is true and there is a counter reset, it models the counter
// as starting from 0 (post-reset) by returning h2 scaled by the fraction.
// It returns an error if the histograms are incompatible (e.g. mixing
// exponential and custom-bucket schemas). NHCB bucket-bounds reconciliation
// warnings are collected into annos.
func interpolateHistograms(h1 *histogram.FloatHistogram, t1 int64, h2 *histogram.FloatHistogram, t2, t int64, isCounter bool, annos *annotations.Annotations, pos posrange.PositionRange) (*histogram.FloatHistogram, error) {
	if t == t1 {
		return h1.Copy(), nil
	}
	if t == t2 {
		return h2.Copy(), nil
	}
	fraction := float64(t-t1) / float64(t2-t1)

	if isCounter && h2.DetectReset(h1) {
		// Counter reset: model as starting from zero.
		return h2.Copy().Mul(fraction), nil
	}

	// Result = H1 + (H2 - H1) * fraction.
	result := h2.Copy()
	_, _, nhcbReconciled, err := result.Sub(h1)
	if err != nil {
		return nil, err
	}
	if nhcbReconciled {
		annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(pos, annotations.HistogramSub))
	}
	result.Mul(fraction)
	_, _, nhcbReconciled, err = result.Add(h1)
	if err != nil {
		return nil, err
	}
	if nhcbReconciled {
		annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(pos, annotations.HistogramAdd))
	}
	return result, nil
}

// pickOrInterpolateLeftHistogram returns the histogram at the left boundary of the range.
// If interpolation is needed (when smoothed is true and the first sample is before the range start),
// it returns the interpolated histogram at the left boundary; otherwise, it returns a copy of the first sample's histogram.
func pickOrInterpolateLeftHistogram(hists []HPoint, first int, rangeStart int64, smoothed, isCounter bool, annos *annotations.Annotations, pos posrange.PositionRange) (*histogram.FloatHistogram, error) {
	if smoothed && hists[first].T < rangeStart {
		return interpolateHistograms(hists[first].H, hists[first].T, hists[first+1].H, hists[first+1].T, rangeStart, isCounter, annos, pos)
	}
	return hists[first].H.Copy(), nil
}

// pickOrInterpolateRightHistogram returns the histogram at the right boundary of the range.
// If interpolation is needed (when smoothed is true and the last sample is after the range end),
// it returns the interpolated histogram at the right boundary; otherwise, it returns a copy of the last sample's histogram.
func pickOrInterpolateRightHistogram(hists []HPoint, last int, rangeEnd int64, smoothed, isCounter bool, annos *annotations.Annotations, pos posrange.PositionRange) (*histogram.FloatHistogram, error) {
	if smoothed && last > 0 && hists[last].T > rangeEnd {
		return interpolateHistograms(hists[last-1].H, hists[last-1].T, hists[last].H, hists[last].T, rangeEnd, isCounter, annos, pos)
	}
	return hists[last].H.Copy(), nil
}

// correctForCounterResets calculates the correction for counter resets.
// This function is only used for extendedRate functions with smoothed or anchored rates.
func correctForCounterResets(left, right float64, points []FPoint) float64 {
	var correction float64
	prev := left
	for _, p := range points {
		if p.F < prev {
			correction += prev
		}
		prev = p.F
	}
	if right < prev {
		correction += prev
	}
	return correction
}

// annosFromInterpolationError classifies an error returned by
// interpolateHistograms (via pickOrInterpolate*Histogram) into the appropriate
// annotation. If the error is not recognised, annos is left unchanged.
func annosFromInterpolationError(annos *annotations.Annotations, err error, metricName string, pos posrange.PositionRange) {
	if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
		annos.Add(annotations.NewMixedExponentialCustomHistogramsWarning(metricName, pos))
	}
}

// addHistogramWithAnnotations adds other into base in place, translating
// histogram errors and bucket-bounds reconciliations into annotations.
// It returns false if the operation failed.
func addHistogramWithAnnotations(base, other *histogram.FloatHistogram, annos *annotations.Annotations, metricName string, pos posrange.PositionRange) bool {
	_, _, nhcbBoundsReconciled, err := base.Add(other)
	if err != nil {
		if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
			annos.Add(annotations.NewMixedExponentialCustomHistogramsWarning(metricName, pos))
		}
		return false
	}
	if nhcbBoundsReconciled {
		annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(pos, annotations.HistogramAdd))
	}
	return true
}

// subHistogramWithAnnotations subtracts other from base in place, translating
// histogram errors and bucket-bounds reconciliations into annotations.
// It returns false if the operation failed.
func subHistogramWithAnnotations(base, other *histogram.FloatHistogram, annos *annotations.Annotations, metricName string, pos posrange.PositionRange) bool {
	_, _, nhcbBoundsReconciled, err := base.Sub(other)
	if err != nil {
		if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
			annos.Add(annotations.NewMixedExponentialCustomHistogramsWarning(metricName, pos))
		}
		return false
	}
	if nhcbBoundsReconciled {
		annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(pos, annotations.HistogramSub))
	}
	return true
}

// validateHistogramRange checks all histogram samples in h for schema
// consistency and counter-type hints. It returns false (and adds an annotation
// to annos) when exponential and custom buckets are mixed. It adds a
// NativeHistogramNotCounterWarning for any sample carrying a gauge hint while
// isCounter is true.
func validateHistogramRange(h []HPoint, isCounter bool, annos *annotations.Annotations, metricName string, pos posrange.PositionRange) bool {
	usingCustomBuckets := h[0].H.UsesCustomBuckets()
	for _, p := range h {
		if p.H.UsesCustomBuckets() != usingCustomBuckets {
			annos.Add(annotations.NewMixedExponentialCustomHistogramsWarning(metricName, pos))
			return false
		}
		if isCounter && p.H.CounterResetHint == histogram.GaugeType {
			annos.Add(annotations.NewNativeHistogramNotCounterWarning(metricName, pos))
		}
	}
	return true
}

// correctForCounterResetsHistogram calculates the histogram correction for
// counter resets between firstSampleIndex and lastSampleIndex in h, using
// left and right as the boundary values. It mirrors correctForCounterResets
// for float samples. It returns the accumulated correction (nil if none),
// any annotations, and false if combining histograms failed.
func correctForCounterResetsHistogram(h []HPoint, firstSampleIndex, lastSampleIndex int, left, right *histogram.FloatHistogram, rangeStart int64, smoothed bool, metricName string, pos posrange.PositionRange) (*histogram.FloatHistogram, annotations.Annotations, bool) {
	// firstSampleIndex is represented by left, so the loop starts one beyond.
	first := firstSampleIndex + 1
	prev := left
	if smoothed && h[firstSampleIndex].T < rangeStart && h[firstSampleIndex+1].H.DetectReset(h[firstSampleIndex].H) {
		// The left interpolation spanned the reset between h[firstSampleIndex]
		// and h[firstSampleIndex+1]. That reset is already accounted for, so
		// skip h[firstSampleIndex+1] from the loop and use it as the comparison
		// anchor for any reset that immediately follows.
		prev = h[firstSampleIndex+1].H
		first++
	}
	// lastSampleIndex is always excluded: right is either a direct copy of
	// h[lastSampleIndex] or an interpolation that inherits its CounterResetHint.
	// Including h[lastSampleIndex] in the loop would make right.DetectReset
	// self-detect on the same hint. The final right.DetectReset(prev) below
	// handles the right-boundary reset safely once h[lastSampleIndex] is not prev.
	last := lastSampleIndex - 1

	// first > last+1 when there is nothing between the two boundary samples to
	// check. This happens when firstSampleIndex == lastSampleIndex (single-sample
	// window), or when the smoothed left interpolation already spanned the only
	// reset interval (lastSampleIndex == firstSampleIndex+1 and first was
	// incremented above). Both boundaries were interpolated from the same reset
	// segment, so there is nothing more to correct.
	if first > last+1 {
		return nil, nil, true
	}

	var (
		correction *histogram.FloatHistogram
		annos      annotations.Annotations
	)

	addCorrection := func(h *histogram.FloatHistogram) bool {
		if correction == nil {
			correction = h.Copy()
			return true
		}
		return addHistogramWithAnnotations(correction, h, &annos, metricName, pos)
	}

	for _, p := range h[first : last+1] {
		if p.H.DetectReset(prev) {
			if !addCorrection(prev) {
				return nil, annos, false
			}
		}
		prev = p.H
	}
	if right.DetectReset(prev) {
		if !addCorrection(prev) {
			return nil, annos, false
		}
	}
	return correction, annos, true
}

// extendedRate is a utility function for anchored/smoothed rate/increase/delta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// interpolates at the first/last sample boundary if needed, and returns
// the result as either per-second (if isRate is true) or overall.
func extendedRate(vals Matrix, args parser.Expressions, enh *EvalNodeHelper, isCounter, isRate bool) (Vector, annotations.Annotations) {
	var (
		ms              = args[0].(*parser.MatrixSelector)
		vs              = ms.VectorSelector.(*parser.VectorSelector)
		samples         = vals[0]
		f               = samples.Floats
		lastSampleIndex = len(f) - 1
		rangeStart      = enh.Ts - durationMilliseconds(ms.Range+vs.Offset)
		rangeEnd        = enh.Ts - durationMilliseconds(vs.Offset)
		annos           annotations.Annotations
		smoothed        = vs.Smoothed
	)

	firstSampleIndex := max(0, sort.Search(lastSampleIndex, func(i int) bool { return f[i].T > rangeStart })-1)
	if smoothed {
		lastSampleIndex = sort.Search(lastSampleIndex, func(i int) bool { return f[i].T >= rangeEnd })
	}

	if f[lastSampleIndex].T <= rangeStart {
		return enh.Out, annos
	}
	if smoothed && f[firstSampleIndex].T > rangeEnd {
		return enh.Out, annos
	}

	left := pickOrInterpolateLeft(f, firstSampleIndex, rangeStart, smoothed, isCounter)
	right := pickOrInterpolateRight(f, lastSampleIndex, rangeEnd, smoothed, isCounter)

	resultFloat := right - left

	if isCounter {
		// We only need to consider samples exactly within the range
		// for counter resets correction, as pickOrInterpolateLeft and
		// pickOrInterpolateRight already handle the resets at boundaries.
		if f[firstSampleIndex].T <= rangeStart {
			firstSampleIndex++
		}
		if f[lastSampleIndex].T >= rangeEnd {
			lastSampleIndex--
		}

		resultFloat += correctForCounterResets(left, right, f[firstSampleIndex:lastSampleIndex+1])
	}
	if isRate {
		resultFloat /= ms.Range.Seconds()
	}

	return append(enh.Out, Sample{F: resultFloat}), annos
}

// extendedHistogramRate is a utility function for anchored/smoothed rate/increase/delta
// with native histograms. It calculates the rate (allowing for counter resets if
// isCounter is true), interpolates at the range boundaries if needed, and returns
// the result as either per-second (if isRate is true) or overall.
//
// TODO: histogramRate pre-computes the minimum exponential schema across all
// samples and reduces every sample to that schema before doing arithmetic, so
// the schema is reduced at most once. extendedHistogramRate reduces schema on
// the fly during pairwise Sub/Add operations. In the common constant-schema
// case both produce identical results. In mixed-schema cases the final schema
// is also the same (global minimum wins either way), but intermediate values
// may briefly sit at a higher resolution before being pulled down when the
// correction is added. Aligning the two approaches requires a min-schema
// pre-scan followed by CopyToSchema on the interpolated boundaries, which is
// a non-trivial change and warrants its own PR.
func extendedHistogramRate(vals Matrix, args parser.Expressions, enh *EvalNodeHelper, isCounter, isRate bool) (Vector, annotations.Annotations) {
	var (
		ms              = args[0].(*parser.MatrixSelector)
		vs              = ms.VectorSelector.(*parser.VectorSelector)
		samples         = vals[0]
		h               = samples.Histograms
		lastSampleIndex = len(h) - 1
		rangeStart      = enh.Ts - durationMilliseconds(ms.Range+vs.Offset)
		rangeEnd        = enh.Ts - durationMilliseconds(vs.Offset)
		annos           annotations.Annotations
		metricName      = getMetricName(samples.Metric)
		pos             = args[0].PositionRange()
		smoothed        = vs.Smoothed
	)

	firstSampleIndex := max(0, sort.Search(lastSampleIndex, func(i int) bool { return h[i].T > rangeStart })-1)
	if smoothed {
		lastSampleIndex = sort.Search(lastSampleIndex, func(i int) bool { return h[i].T >= rangeEnd })
	}

	if h[lastSampleIndex].T <= rangeStart {
		return enh.Out, annos
	}
	if smoothed && h[firstSampleIndex].T > rangeEnd {
		return enh.Out, annos
	}

	if !validateHistogramRange(h[firstSampleIndex:lastSampleIndex+1], isCounter, &annos, metricName, pos) {
		return enh.Out, annos
	}

	left, err := pickOrInterpolateLeftHistogram(h, firstSampleIndex, rangeStart, smoothed, isCounter, &annos, pos)
	if err != nil {
		annosFromInterpolationError(&annos, err, metricName, pos)
		return enh.Out, annos
	}
	right, err := pickOrInterpolateRightHistogram(h, lastSampleIndex, rangeEnd, smoothed, isCounter, &annos, pos)
	if err != nil {
		annosFromInterpolationError(&annos, err, metricName, pos)
		return enh.Out, annos
	}

	if !isCounter && (left.CounterResetHint != histogram.GaugeType || right.CounterResetHint != histogram.GaugeType) {
		annos.Add(annotations.NewNativeHistogramNotGaugeWarning(metricName, pos))
	}

	// Copy right so that correctForCounterResetsHistogram can still call
	// right.DetectReset without observing mutations from subHistogramWithAnnotations.
	resultHistogram := right.Copy()
	if !subHistogramWithAnnotations(resultHistogram, left, &annos, metricName, pos) {
		return enh.Out, annos
	}

	if isCounter {
		correction, newAnnos, ok := correctForCounterResetsHistogram(h, firstSampleIndex, lastSampleIndex, left, right, rangeStart, smoothed, metricName, pos)
		annos.Merge(newAnnos)
		if !ok {
			return enh.Out, annos
		}
		if correction != nil && !addHistogramWithAnnotations(resultHistogram, correction, &annos, metricName, pos) {
			return enh.Out, annos
		}
	}
	if isRate {
		resultHistogram.Div(ms.Range.Seconds())
	}

	resultHistogram.CounterResetHint = histogram.GaugeType
	return append(enh.Out, Sample{H: resultHistogram.Compact(0)}), annos
}

// extrapolatedRate is a utility function for rate/increase/delta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// extrapolates if the first/last sample is close to the boundary, and returns
// the result as either per-second (if isRate is true) or overall.
//
// Note: If the vector selector is smoothed or anchored, it will use the
// extendedRate or extendedHistogramRate function instead.
func extrapolatedRate(vals Matrix, args parser.Expressions, enh *EvalNodeHelper, isCounter, isRate bool) (Vector, annotations.Annotations) {
	ms := args[0].(*parser.MatrixSelector)
	vs := ms.VectorSelector.(*parser.VectorSelector)
	if vs.Anchored || vs.Smoothed {
		samples := vals[0]
		if len(samples.Histograms) > 0 && len(samples.Floats) > 0 {
			var annos annotations.Annotations
			return enh.Out, annos.Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(samples.Metric), args[0].PositionRange()))
		}
		if len(samples.Histograms) > 0 {
			return extendedHistogramRate(vals, args, enh, isCounter, isRate)
		}
		if len(samples.Floats) > 0 {
			return extendedRate(vals, args, enh, isCounter, isRate)
		}
		// Not enough samples.
		return enh.Out, nil
	}

	var (
		samples            = vals[0]
		startTimestamps    []int64
		rangeStart         = enh.Ts - durationMilliseconds(ms.Range+vs.Offset)
		rangeEnd           = enh.Ts - durationMilliseconds(vs.Offset)
		resultFloat        float64
		resultHistogram    *histogram.FloatHistogram
		firstT, lastT      int64
		numSamplesMinusOne int
		annos              annotations.Annotations
	)

	// Drop this vector element and return a warning if this rate window contains mixed float and histogram samples.
	if len(samples.Histograms) > 0 && len(samples.Floats) > 0 {
		return enh.Out, annos.Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(samples.Metric), args[0].PositionRange()))
	}

	// To calculate a rate, we normally need at least two float or two histogram samples. However,
	// we can calculate rate with a single sample as long as there's a start timestamp reset inside the window.
	switch {
	case len(samples.Histograms) > 0:
		numSamplesMinusOne = len(samples.Histograms) - 1
		firstT = samples.Histograms[0].T
		lastT = samples.Histograms[numSamplesMinusOne].T
		var newAnnos annotations.Annotations
		if enh.StartTimestamps != nil {
			startTimestamps = enh.StartTimestamps.Histograms
		}
		if len(samples.Histograms) > 1 {
			resultHistogram, newAnnos = histogramRate(samples.Histograms, startTimestamps, isCounter,
				samples.Metric, args[0].PositionRange())
			annos.Merge(newAnnos)
			if resultHistogram == nil {
				// The histograms are not compatible with each other.
				return enh.Out, annos
			}
		}
	case len(samples.Floats) > 0:
		numSamplesMinusOne = len(samples.Floats) - 1
		firstT = samples.Floats[0].T
		lastT = samples.Floats[numSamplesMinusOne].T
		resultFloat = samples.Floats[numSamplesMinusOne].F - samples.Floats[0].F
		if !isCounter {
			break
		}
		// Handle counter resets:
		if enh.StartTimestamps != nil {
			startTimestamps = enh.StartTimestamps.Floats
		}
		var overlapDetected bool
		for i, currPoint := range samples.Floats[1:] {
			prevPoint := samples.Floats[i]
			// Warn once per series if start timestamp overlap detected.
			if !overlapDetected && i+1 < len(startTimestamps) {
				if checkStartTimeOverlap(startTimestamps[i], prevPoint.T, startTimestamps[i+1]) {
					// Extract metric name only when needed.
					annos.Add(annotations.NewStartTimeOverlapWarning(getMetricName(samples.Metric), args[0].PositionRange()))
					overlapDetected = true
				}
			}
			if currPoint.F < prevPoint.F || (i+1 < len(startTimestamps) && isStartTimestampReset(startTimestamps[i], prevPoint.T, startTimestamps[i+1], currPoint.T)) {
				resultFloat += prevPoint.F
			}
		}
	default:
		// TODO: add RangeTooShortWarning
		return enh.Out, annos
	}

	// Duration between first/last samples and boundary of range.
	durationToStart := float64(firstT-rangeStart) / 1000
	durationToEnd := float64(rangeEnd-lastT) / 1000

	sampledInterval := float64(lastT-firstT) / 1000
	var averageDurationBetweenSamples float64
	if numSamplesMinusOne > 0 {
		averageDurationBetweenSamples = sampledInterval / float64(numSamplesMinusOne)
	}
	extrapolationThreshold := averageDurationBetweenSamples * 1.1

	if sts := startTimestamps; isCounter && len(sts) > 0 && sts[0] != 0 && sts[0] > rangeStart && sts[0] < firstT {
		// Take the first sample in the range and check whether its ST points inside the range
		// (while also having a sensible value). If yes, we assume that there is a zero-value sample
		// at the time of ST, and use that instead of extrapolating towards left side.
		//
		// Note that the rangeStart is exclusive, thus ST=rangeStart would be outside the range.
		//
		// Also note that when there's a single sample, we lose extrapolation to the right, because
		// we need at least two samples to calculate average duration between samples, which leads
		// to extrapolation threshold being 0.
		durationToStart = 0
		sampledInterval = float64(lastT-sts[0]) / 1000
		if len(samples.Floats) > 0 {
			resultFloat += samples.Floats[0].F
		} else if len(samples.Histograms) > 0 {
			if resultHistogram == nil {
				resultHistogram = samples.Histograms[0].H.Copy()
			} else if !addHistogramWithAnnotations(resultHistogram, samples.Histograms[0].H, &annos, getMetricName(samples.Metric), args[0].PositionRange()) {
				return enh.Out, annos
			}
		}
	} else if numSamplesMinusOne == 0 {
		// There's a single sample, and we do not have suitable ST to calculate the increase. Return nothing.
		return enh.Out, annos
	} else {
		// If samples are close enough to the (lower or upper) boundary of the
		// range, we extrapolate the rate all the way to the boundary in
		// question. "Close enough" is defined as "up to 10% more than the
		// average duration between samples within the range", see
		// extrapolationThreshold below. Essentially, we are assuming a more or
		// less regular spacing between samples, and if we don't see a sample
		// where we would expect one, we assume the series does not cover the
		// whole range, but starts and/or ends within the range. We still
		// extrapolate the rate in this case, but not all the way to the
		// boundary, but only by half of the average duration between samples
		// (which is our guess for where the series actually starts or ends).

		if durationToStart >= extrapolationThreshold {
			durationToStart = averageDurationBetweenSamples / 2
		}
		if isCounter {
			// Counters cannot be negative. If we have any slope at all
			// (i.e. resultFloat went up), we can extrapolate the zero point
			// of the counter. If the duration to the zero point is shorter
			// than the durationToStart, we take the zero point as the start
			// of the series, thereby avoiding extrapolation to negative
			// counter values.
			durationToZero := durationToStart
			if resultFloat > 0 &&
				len(samples.Floats) > 0 &&
				samples.Floats[0].F >= 0 {
				durationToZero = sampledInterval * (samples.Floats[0].F / resultFloat)
			} else if resultHistogram != nil &&
				resultHistogram.Count > 0 &&
				len(samples.Histograms) > 0 &&
				samples.Histograms[0].H.Count >= 0 {
				durationToZero = sampledInterval * (samples.Histograms[0].H.Count / resultHistogram.Count)
			}
			if durationToZero < durationToStart {
				durationToStart = durationToZero
			}
		}
	}

	if durationToEnd >= extrapolationThreshold {
		durationToEnd = averageDurationBetweenSamples / 2
	}

	factor := 1.0
	if sampledInterval != 0 {
		factor = (sampledInterval + durationToStart + durationToEnd) / sampledInterval
	}
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
// not a histogram or there are incompatibilities between histograms, and a warning
// wrapped in an annotation in that case. Otherwise, it returns the calculated histogram,
// and potentially some annotations.
func histogramRate(
	points []HPoint,
	startTimestamps []int64,
	isCounter bool,
	labels labels.Labels,
	pos posrange.PositionRange,
) (*histogram.FloatHistogram, annotations.Annotations) {
	var (
		prev               = points[0].H
		usingCustomBuckets = prev.UsesCustomBuckets()
		last               = points[len(points)-1].H
		annos              annotations.Annotations
	)

	if last == nil {
		return nil, annos.Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(labels), pos))
	}

	// We check for gauge type histograms in the loop below, but the loop
	// below does not run on the first and last point, so check the first
	// and last point now.
	if isCounter && (prev.CounterResetHint == histogram.GaugeType || last.CounterResetHint == histogram.GaugeType) {
		// TODO(start-timestamps): for delta histograms, we plan to use Gauge counter reset hint,
		// while the reset will be indicated via a start timestamp. This will be an expected usage pattern,
		// thus we should not be returning the following warning. When addressing this, also check
		// other places where this warning is being emitted.
		annos.Add(annotations.NewNativeHistogramNotCounterWarning(getMetricName(labels), pos))
	}

	// Null out the 1st sample if there is a counter reset between the 1st
	// and 2nd. In this case, we want to ignore any incompatibility in the
	// bucket layout of the 1st sample because we do not need to look at it.
	if isCounter {
		second := points[1].H
		if second != nil && (len(startTimestamps) > 1 && isStartTimestampReset(startTimestamps[0], points[0].T, startTimestamps[1], points[1].T) || second.DetectReset(prev)) {
			prev = &histogram.FloatHistogram{}
			prev.Schema = second.Schema
			prev.CustomValues = second.CustomValues
			usingCustomBuckets = second.UsesCustomBuckets()
		}
	}

	if last.UsesCustomBuckets() != usingCustomBuckets {
		return nil, annos.Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(labels), pos))
	}

	// First iteration to find out two things:
	// - What's the smallest relevant schema?
	// - Are all data points histograms?
	minSchema := min(last.Schema, prev.Schema)
	for _, currPoint := range points[1 : len(points)-1] {
		curr := currPoint.H
		if curr == nil {
			return nil, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(labels), pos))
		}
		if !isCounter {
			continue
		}
		if curr.CounterResetHint == histogram.GaugeType {
			annos.Add(annotations.NewNativeHistogramNotCounterWarning(getMetricName(labels), pos))
		}
		if curr.Schema < minSchema {
			minSchema = curr.Schema
		}
		if curr.UsesCustomBuckets() != usingCustomBuckets {
			return nil, annotations.New().Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(labels), pos))
		}
	}

	h := last.CopyToSchema(minSchema)
	// This subtraction may deliberately include conflicting counter resets.
	// Counter resets are treated explicitly in this function, so the
	// information about conflicting counter resets is ignored here.
	_, _, nhcbBoundsReconciled, err := h.Sub(prev)
	if err != nil {
		if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
			return nil, annotations.New().Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(labels), pos))
		}
	}
	if nhcbBoundsReconciled {
		annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(pos, annotations.HistogramSub))
	}

	if isCounter {
		// Second iteration to deal with counter resets.
		var overlapDetected bool
		for i, currPoint := range points[1:] {
			curr := currPoint.H
			// Warn once per series if start timestamp overlap detected.
			if !overlapDetected && i+1 < len(startTimestamps) {
				if checkStartTimeOverlap(startTimestamps[i], points[i].T, startTimestamps[i+1]) {
					// Extract metric name only when needed.
					annos.Add(annotations.NewStartTimeOverlapWarning(getMetricName(labels), pos))
					overlapDetected = true
				}
			}
			// Check start timestamps first since it's potentially cheaper.
			if i+1 < len(startTimestamps) && isStartTimestampReset(startTimestamps[i], points[i].T, startTimestamps[i+1], currPoint.T) || curr.DetectReset(prev) {
				// Counter reset conflict ignored here for the same reason as above.
				_, _, nhcbBoundsReconciled, err := h.Add(prev)
				if err != nil {
					if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
						return nil, annotations.New().Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(labels), pos))
					}
				}
				if nhcbBoundsReconciled {
					annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(pos, annotations.HistogramAdd))
				}
			}
			prev = curr
		}
	} else if points[0].H.CounterResetHint != histogram.GaugeType || points[len(points)-1].H.CounterResetHint != histogram.GaugeType {
		annos.Add(annotations.NewNativeHistogramNotGaugeWarning(getMetricName(labels), pos))
	}

	h.CounterResetHint = histogram.GaugeType
	return h.Compact(0), annos
}

// isStartTimestampReset tells whether there was a counter reset by checking the start timestamp value.
func isStartTimestampReset(prevStartTimestamp, prevTimestamp, currStartTimestamp, currTimestamp int64) bool {
	if currStartTimestamp == 0 || currStartTimestamp >= currTimestamp {
		// No reset if start timestamp is not set (value is 0), if it is clearly invalid
		// (ST > T), or if it is OTel's unknown start time (ST == T).
		return false
	}

	if currStartTimestamp < prevTimestamp {
		return false
	}

	if currStartTimestamp > prevTimestamp {
		return true
	}

	// If this place is reached, then it means that the current datapoint start timestamp is pointing
	// to a previous datapoint. In OTel, this should only happen in two cases:
	// * this is a delta series,
	// * or this is a cumulative series with unknown start timestamp.
	//
	// This should be treated as a reset for deltas, but it is not a reset for cumulative series
	// with unknown start timestamp. Thus we have to check whether the start timestamp
	// of the previous datapoint is known.
	//
	// A previous start timestamp greater than the previous sample timestamp is invalid; treat it
	// as unknown to avoid a spurious reset.
	if prevStartTimestamp > prevTimestamp {
		return false
	}
	return prevStartTimestamp != 0 && prevStartTimestamp != prevTimestamp
}

// checkStartTimeOverlap detects when a sample's start timestamp overlaps with a
// previous sample's timestamp, indicating potential data quality issues.
// Works for both delta and cumulative counter metrics.
//
// Returns true when: currST != 0 && currST < prevT && currST != prevST
// This correctly handles:
//   - Valid deltas: currST = prevT (no overlap detected)
//   - Valid cumulative: currST = prevST (no overlap detected)
//   - Invalid overlap: currST < prevT && currST != prevST (overlap detected)
func checkStartTimeOverlap(prevStartTimestamp, prevTimestamp, currStartTimestamp int64) bool {
	return currStartTimestamp != 0 && currStartTimestamp < prevTimestamp && currStartTimestamp != prevStartTimestamp
}

// === delta(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcDelta(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extrapolatedRate(matrixVals, args, enh, false, false)
}

// === rate(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcRate(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extrapolatedRate(matrixVals, args, enh, true, true)
}

// === increase(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcIncrease(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extrapolatedRate(matrixVals, args, enh, true, false)
}

// === irate(node parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcIrate(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return instantValue(matrixVals, args, enh, true)
}

// === idelta(node model.ValMatrix) (Vector, Annotations) ===
func funcIdelta(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return instantValue(matrixVals, args, enh, false)
}

func instantValue(vals Matrix, args parser.Expressions, enh *EvalNodeHelper, isRate bool) (Vector, annotations.Annotations) {
	type sampleWithSt struct {
		Sample

		ST int64
	}

	var (
		samples = vals[0]
		out     = enh.Out
		ss      = make([]sampleWithSt, 0, 2)
		annos   annotations.Annotations
	)

	// No sense in trying to compute a rate without at least two points. Drop
	// this Vector element.
	// TODO: add RangeTooShortWarning
	if len(samples.Floats)+len(samples.Histograms) < 2 {
		return out, nil
	}

	// Add the last 2 float samples if they exist.
	for i := max(0, len(samples.Floats)-2); i < len(samples.Floats); i++ {
		s := sampleWithSt{
			Sample: Sample{
				F: samples.Floats[i].F,
				T: samples.Floats[i].T,
			},
		}
		if sts := enh.StartTimestamps; sts != nil && i < len(sts.Floats) {
			s.ST = sts.Floats[i]
		}
		ss = append(ss, s)
	}

	// Add the last 2 histogram samples into their correct position if they exist.
	for i := max(0, len(samples.Histograms)-2); i < len(samples.Histograms); i++ {
		s := sampleWithSt{
			Sample: Sample{
				H: samples.Histograms[i].H,
				T: samples.Histograms[i].T,
			},
		}
		if sts := enh.StartTimestamps; sts != nil && i < len(sts.Histograms) {
			s.ST = sts.Histograms[i]
		}
		switch {
		case len(ss) == 0:
			ss = append(ss, s)
		case len(ss) == 1:
			if s.T < ss[0].T {
				ss = append([]sampleWithSt{s}, ss...)
			} else {
				ss = append(ss, s)
			}
		case s.T < ss[0].T:
			// s is older than 1st, so discard it.
		case s.T > ss[1].T:
			// s is newest, so add it as 2nd and make the old 2nd the new 1st.
			ss[0] = ss[1]
			ss[1] = s
		default:
			// In all other cases, we just make s the new 1st.
			// This establishes a correct order, even in the (irregular)
			// case of equal timestamps.
			ss[0] = s
		}
	}

	resultSample := ss[1]
	sampledInterval := ss[1].T - ss[0].T
	if sampledInterval == 0 {
		// Avoid dividing by 0.
		return out, nil
	}
	switch {
	case ss[1].H == nil && ss[0].H == nil:
		if !isRate || !(ss[1].F < ss[0].F || isStartTimestampReset(ss[0].ST, ss[0].T, ss[1].ST, ss[1].T)) {
			// Gauge, or counter without reset, or counter with NaN value.
			resultSample.F = ss[1].F - ss[0].F
		}

		// In case of a counter reset, we leave resultSample at
		// its current value, which is already ss[1].
	case ss[1].H != nil && ss[0].H != nil:
		resultSample.H = ss[1].H.Copy()
		// irate should only be applied to counters.
		if isRate && (ss[1].H.CounterResetHint == histogram.GaugeType || ss[0].H.CounterResetHint == histogram.GaugeType) {
			annos.Add(annotations.NewNativeHistogramNotCounterWarning(getMetricName(samples.Metric), args.PositionRange()))
		}
		// idelta should only be applied to gauges.
		if !isRate && (ss[1].H.CounterResetHint != histogram.GaugeType || ss[0].H.CounterResetHint != histogram.GaugeType) {
			annos.Add(annotations.NewNativeHistogramNotGaugeWarning(getMetricName(samples.Metric), args.PositionRange()))
		}
		if !isRate || (!isStartTimestampReset(ss[0].ST, ss[0].T, ss[1].ST, ss[1].T) && !ss[1].H.DetectReset(ss[0].H)) {
			// This subtraction may deliberately include conflicting
			// counter resets. Counter resets are treated explicitly
			// in this function, so the information about
			// conflicting counter resets is ignored here.
			_, _, nhcbBoundsReconciled, err := resultSample.H.Sub(ss[0].H)
			if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
				return out, annos.Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(samples.Metric), args.PositionRange()))
			}
			if nhcbBoundsReconciled {
				annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(args.PositionRange(), annotations.HistogramSub))
			}
		}
		resultSample.H.CounterResetHint = histogram.GaugeType
		resultSample.H.Compact(0)
	default:
		// Mix of a float and a histogram.
		return out, annos.Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(samples.Metric), args.PositionRange()))
	}

	if isRate {
		// Convert to per-second.
		if resultSample.H == nil {
			resultSample.F /= float64(sampledInterval) / 1000
		} else {
			resultSample.H.Div(float64(sampledInterval) / 1000)
		}
	}

	return append(out, resultSample.Sample), annos
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

// Double exponential smoothing is similar to a weighted moving average, where
// historical data has exponentially less influence on the current data. It also
// accounts for trends in data. The smoothing factor (0 < sf < 1) affects how
// historical data will affect the current data. A lower smoothing factor
// increases the influence of historical data. The trend factor (0 < tf < 1)
// affects how trends in historical data will affect the current data. A higher
// trend factor increases the influence. of trends. Algorithm taken from
// https://en.wikipedia.org/wiki/Exponential_smoothing .
func funcDoubleExponentialSmoothing(vectorVals []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vectorVals) < 2 || len(vectorVals[0]) == 0 || len(vectorVals[1]) == 0 || len(matrixVal) == 0 {
		return enh.Out, nil
	}
	samples := matrixVal[0]
	// The smoothing factor argument.
	sf := vectorVals[0][0].F

	// The trend factor argument.
	tf := vectorVals[1][0].F

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
		// Annotate mix of float and histogram.
		if l == 1 && len(samples.Histograms) > 0 {
			return enh.Out, annotations.New().Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
		}
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
	if len(samples.Histograms) > 0 {
		return append(enh.Out, Sample{F: s1}), annotations.New().Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
	}
	return append(enh.Out, Sample{F: s1}), nil
}

// filterFloats filters out histogram samples from the vector in-place.
func filterFloats(v Vector) Vector {
	floats := v[:0]
	for _, s := range v {
		if s.H == nil {
			floats = append(floats, s)
		}
	}
	return floats
}

// === sort(node parser.ValueTypeVector) (Vector, Annotations) ===
func funcSort(vectorVals []Vector, _ Matrix, _ parser.Expressions, _ *EvalNodeHelper) (Vector, annotations.Annotations) {
	// NaN should sort to the bottom, so take descending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByReverseValueHeap(filterFloats(vectorVals[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter), nil
}

// === sortDesc(node parser.ValueTypeVector) (Vector, Annotations) ===
func funcSortDesc(vectorVals []Vector, _ Matrix, _ parser.Expressions, _ *EvalNodeHelper) (Vector, annotations.Annotations) {
	// NaN should sort to the bottom, so take ascending sort with NaN first and
	// reverse it.
	byValueSorter := vectorByValueHeap(filterFloats(vectorVals[0]))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter), nil
}

// === sort_by_label(vector parser.ValueTypeVector, label parser.ValueTypeString...) (Vector, Annotations) ===
func funcSortByLabel(vectorVals []Vector, _ Matrix, args parser.Expressions, _ *EvalNodeHelper) (Vector, annotations.Annotations) {
	lbls := stringSliceFromArgs(args[1:])
	slices.SortFunc(vectorVals[0], func(a, b Sample) int {
		for _, label := range lbls {
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

		// If all labels provided as arguments were equal, sort by the full label set. This ensures a consistent ordering.
		return labels.Compare(a.Metric, b.Metric)
	})

	return vectorVals[0], nil
}

// === sort_by_label_desc(vector parser.ValueTypeVector, label parser.ValueTypeString...) (Vector, Annotations) ===
func funcSortByLabelDesc(vectorVals []Vector, _ Matrix, args parser.Expressions, _ *EvalNodeHelper) (Vector, annotations.Annotations) {
	lbls := stringSliceFromArgs(args[1:])
	slices.SortFunc(vectorVals[0], func(a, b Sample) int {
		for _, label := range lbls {
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

		// If all labels provided as arguments were equal, sort by the full label set. This ensures a consistent ordering.
		return -labels.Compare(a.Metric, b.Metric)
	})

	return vectorVals[0], nil
}

func clamp(vec Vector, minVal, maxVal float64, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if maxVal < minVal {
		return enh.Out, nil
	}
	for _, el := range vec {
		if el.H != nil {
			// Process only float samples.
			continue
		}
		if !enh.enableDelayedNameRemoval {
			el.Metric = el.Metric.DropReserved(schema.IsMetadataLabel)
		}
		enh.Out = append(enh.Out, Sample{
			Metric:   el.Metric,
			F:        math.Max(minVal, math.Min(maxVal, el.F)),
			DropName: true,
		})
	}
	return enh.Out, nil
}

// === clamp(Vector parser.ValueTypeVector, min, max Scalar) (Vector, Annotations) ===
func funcClamp(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vectorVals[0]
	minVal := vectorVals[1][0].F
	maxVal := vectorVals[2][0].F
	return clamp(vec, minVal, maxVal, enh)
}

// === clamp_max(Vector parser.ValueTypeVector, max Scalar) (Vector, Annotations) ===
func funcClampMax(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vectorVals[0]
	maxVal := vectorVals[1][0].F
	return clamp(vec, math.Inf(-1), maxVal, enh)
}

// === clamp_min(Vector parser.ValueTypeVector, min Scalar) (Vector, Annotations) ===
func funcClampMin(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vectorVals[0]
	minVal := vectorVals[1][0].F
	return clamp(vec, minVal, math.Inf(+1), enh)
}

// === round(Vector parser.ValueTypeVector, toNearest=1 Scalar) (Vector, Annotations) ===
func funcRound(vectorVals []Vector, _ Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	// round returns a number rounded to toNearest.
	// Ties are solved by rounding up.
	toNearest := float64(1)
	if len(args) >= 2 {
		toNearest = vectorVals[1][0].F
	}
	// Invert as it seems to cause fewer floating point accuracy issues.
	toNearestInverse := 1.0 / toNearest
	return simpleFloatFunc(vectorVals, enh, func(f float64) float64 {
		return math.Floor(f*toNearestInverse+0.5) / toNearestInverse
	}), nil
}

// === Scalar(node parser.ValueTypeVector) Scalar ===
func funcScalar(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	var (
		v     = vectorVals[0]
		value float64
		found bool
	)

	for _, s := range v {
		if s.H == nil {
			if found {
				// More than one float found, return NaN.
				return append(enh.Out, Sample{F: math.NaN()}), nil
			}
			found = true
			value = s.F
		}
	}
	// Return the single float if found, otherwise return NaN.
	if !found {
		return append(enh.Out, Sample{F: math.NaN()}), nil
	}
	return append(enh.Out, Sample{F: value}), nil
}

func aggrOverTime(matrixVal Matrix, enh *EvalNodeHelper, aggrFn func(Series) float64) Vector {
	if len(matrixVal) == 0 {
		return enh.Out
	}
	el := matrixVal[0]

	return append(enh.Out, Sample{F: aggrFn(el)})
}

func aggrHistOverTime(matrixVal Matrix, enh *EvalNodeHelper, aggrFn func(Series) (*histogram.FloatHistogram, error)) (Vector, error) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	el := matrixVal[0]
	res, err := aggrFn(el)

	return append(enh.Out, Sample{H: res}), err
}

// === avg_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations)  ===
func funcAvgOverTime(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	firstSeries := matrixVal[0]
	if len(firstSeries.Floats) > 0 && len(firstSeries.Histograms) > 0 {
		return enh.Out, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(firstSeries.Metric), args[0].PositionRange()))
	}
	// We improve the accuracy with the help of Kahan summation.
	// For a while, we assumed that incremental mean calculation combined
	// with Kahan summation (see
	// https://stackoverflow.com/questions/61665473/is-it-beneficial-for-precision-to-calculate-the-incremental-mean-average
	// for inspiration) is generally the preferred solution. However, it
	// then turned out that direct mean calculation (still in combination
	// with Kahan summation) is often more accurate. See discussion in
	// https://github.com/prometheus/prometheus/issues/16714 . The problem
	// with the direct mean calculation is that it can overflow float64 for
	// inputs on which the incremental mean calculation works just fine. Our
	// current approach is therefore to use direct mean calculation as long
	// as we do not overflow (or underflow) the running sum. Once the latter
	// would happen, we switch to incremental mean calculation. This seems
	// to work reasonably well, but note that a deeper understanding would
	// be needed to find out if maybe an earlier switch to incremental mean
	// calculation would be better in terms of accuracy. Also, we could
	// apply a number of additional means to improve the accuracy, like
	// processing the values in a particular order. For now, we decided that
	// the current implementation is accurate enough for practical purposes.
	if len(firstSeries.Floats) == 0 {
		// The passed values only contain histograms.
		var annos annotations.Annotations
		vec, err := aggrHistOverTime(matrixVal, enh, func(s Series) (*histogram.FloatHistogram, error) {
			var counterResetSeen, notCounterResetSeen, nhcbBoundsReconciledSeen bool

			trackCounterReset := func(h *histogram.FloatHistogram) {
				switch h.CounterResetHint {
				case histogram.CounterReset:
					counterResetSeen = true
				case histogram.NotCounterReset:
					notCounterResetSeen = true
				}
			}

			defer func() {
				if counterResetSeen && notCounterResetSeen {
					annos.Add(annotations.NewHistogramCounterResetCollisionWarning(args[0].PositionRange(), annotations.HistogramAgg))
				}
				if nhcbBoundsReconciledSeen {
					annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(args[0].PositionRange(), annotations.HistogramAgg))
				}
			}()

			var (
				sum                  = s.Histograms[0].H.Copy()
				mean, kahanC         *histogram.FloatHistogram
				count                = 1.
				incrementalMean      bool
				nhcbBoundsReconciled bool
				err                  error
			)
			trackCounterReset(sum)
			for i, h := range s.Histograms[1:] {
				trackCounterReset(h.H)
				count = float64(i + 2)
				if !incrementalMean {
					sumCopy := sum.Copy()
					var cCopy *histogram.FloatHistogram
					if kahanC != nil {
						cCopy = kahanC.Copy()
					}
					cCopy, _, nhcbBoundsReconciled, err = sumCopy.KahanAdd(h.H, cCopy)
					if err != nil {
						return sumCopy.Div(count), err
					}
					if nhcbBoundsReconciled {
						nhcbBoundsReconciledSeen = true
					}
					if !sumCopy.HasOverflow() {
						sum, kahanC = sumCopy, cCopy
						continue
					}
					incrementalMean = true
					mean = sum.Copy().Div(count - 1)
					if kahanC != nil {
						kahanC.Div(count - 1)
					}
				}
				q := (count - 1) / count
				if kahanC != nil {
					kahanC.Mul(q)
				}
				toAdd := h.H.Copy().Div(count)
				kahanC, _, nhcbBoundsReconciled, err = mean.Mul(q).KahanAdd(toAdd, kahanC)
				if err != nil {
					return mean, err
				}
				if nhcbBoundsReconciled {
					nhcbBoundsReconciledSeen = true
				}
			}
			if incrementalMean {
				if kahanC != nil {
					_, _, _, err := mean.Add(kahanC)
					return mean, err
				}
				return mean, nil
			}
			if kahanC != nil {
				_, _, _, err := sum.Div(count).Add(kahanC.Div(count))
				return sum, err
			}
			return sum.Div(count), nil
		})
		if err != nil {
			if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
				return enh.Out, annotations.New().Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(firstSeries.Metric), args[0].PositionRange()))
			}
		}
		return vec, annos
	}
	return aggrOverTime(matrixVal, enh, func(s Series) float64 {
		var (
			// Pre-set the 1st sample to start the loop with the 2nd.
			sum, count      = s.Floats[0].F, 1.
			mean, kahanC    float64
			incrementalMean bool
		)
		for i, f := range s.Floats[1:] {
			count = float64(i + 2)
			if !incrementalMean {
				newSum, newC := kahansum.Inc(f.F, sum, kahanC)
				// Perform regular mean calculation as long as
				// the sum doesn't overflow.
				if !math.IsInf(newSum, 0) {
					sum, kahanC = newSum, newC
					continue
				}
				// Handle overflow by reverting to incremental
				// calculation of the mean value.
				incrementalMean = true
				mean = sum / (count - 1)
				kahanC /= (count - 1)
			}
			q := (count - 1) / count
			mean, kahanC = kahansum.Inc(f.F/count, q*mean, q*kahanC)
		}
		if incrementalMean {
			return mean + kahanC
		}
		return sum/count + kahanC/count
	}), nil
}

// === count_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcCountOverTime(_ []Vector, matrixVals Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return aggrOverTime(matrixVals, enh, func(s Series) float64 {
		return float64(len(s.Floats) + len(s.Histograms))
	}), nil
}

// === first_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcFirstOverTime(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	el := matrixVal[0]

	var f FPoint
	if len(el.Floats) > 0 {
		f = el.Floats[0]
	}

	var h HPoint
	if len(el.Histograms) > 0 {
		h = el.Histograms[0]
	}

	// If a float data point exists and is older than any histogram data
	// points, return it.
	if h.H == nil || (len(el.Floats) > 0 && f.T < h.T) {
		return append(enh.Out, Sample{
			Metric: el.Metric,
			F:      f.F,
		}), nil
	}
	return append(enh.Out, Sample{
		Metric: el.Metric,
		H:      h.H.Copy(),
	}), nil
}

// === last_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcLastOverTime(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	el := matrixVal[0]

	var f FPoint
	if len(el.Floats) > 0 {
		f = el.Floats[len(el.Floats)-1]
	}

	var h HPoint
	if len(el.Histograms) > 0 {
		h = el.Histograms[len(el.Histograms)-1]
	}

	if h.H == nil || (len(el.Floats) > 0 && h.T < f.T) {
		return append(enh.Out, Sample{
			Metric: el.Metric,
			F:      f.F,
		}), nil
	}
	return append(enh.Out, Sample{
		Metric: el.Metric,
		H:      h.H.Copy(),
	}), nil
}

// === mad_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMadOverTime(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	samples := matrixVal[0]
	var annos annotations.Annotations
	if len(samples.Floats) == 0 {
		return enh.Out, nil
	}
	if len(samples.Histograms) > 0 {
		annos.Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
	}
	return aggrOverTime(matrixVal, enh, func(s Series) float64 {
		values := make(vectorByValueHeap, 0, len(s.Floats))
		for _, f := range s.Floats {
			// A NaN sample makes the median, and therefore the deviation,
			// undefined, so propagate NaN rather than silently dropping it.
			if math.IsNaN(f.F) {
				return math.NaN()
			}
			values = append(values, Sample{F: f.F})
		}
		median := quantile(0.5, values)
		for i := range values {
			values[i].F = math.Abs(values[i].F - median)
		}
		return quantile(0.5, values)
	}), annos
}

// === ts_of_first_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcTsOfFirstOverTime(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	el := matrixVal[0]

	var tf int64 = math.MaxInt64
	if len(el.Floats) > 0 {
		tf = el.Floats[0].T
	}

	var th int64 = math.MaxInt64
	if len(el.Histograms) > 0 {
		th = el.Histograms[0].T
	}

	return append(enh.Out, Sample{
		Metric: el.Metric,
		F:      float64(min(tf, th)) / 1000,
	}), nil
}

// === ts_of_last_over_time(Matrix parser.ValueTypeMatrix) (Vector, Notes)  ===
func funcTsOfLastOverTime(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	el := matrixVal[0]

	var tf int64
	if len(el.Floats) > 0 {
		tf = el.Floats[len(el.Floats)-1].T
	}

	var th int64
	if len(el.Histograms) > 0 {
		th = el.Histograms[len(el.Histograms)-1].T
	}

	return append(enh.Out, Sample{
		Metric: el.Metric,
		F:      float64(max(tf, th)) / 1000,
	}), nil
}

// === ts_of_max_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcTsOfMaxOverTime(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return compareOverTime(matrixVal, args, enh, func(cur, maxVal float64) bool {
		return (cur >= maxVal) || math.IsNaN(maxVal)
	}, true)
}

// === ts_of_min_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcTsOfMinOverTime(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return compareOverTime(matrixVals, args, enh, func(cur, maxVal float64) bool {
		return (cur <= maxVal) || math.IsNaN(maxVal)
	}, true)
}

// compareOverTime is a helper used by funcMaxOverTime and funcMinOverTime.
func compareOverTime(matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper, compareFn func(float64, float64) bool, returnTimestamp bool) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	samples := matrixVal[0]
	var annos annotations.Annotations
	if len(samples.Floats) == 0 {
		return enh.Out, nil
	}
	if len(samples.Histograms) > 0 {
		annos.Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
	}
	return aggrOverTime(matrixVal, enh, func(s Series) float64 {
		maxVal := s.Floats[0].F
		tsOfMax := s.Floats[0].T
		for _, f := range s.Floats {
			if compareFn(f.F, maxVal) {
				maxVal = f.F
				tsOfMax = f.T
			}
		}
		if returnTimestamp {
			return float64(tsOfMax) / 1000
		}
		return maxVal
	}), annos
}

// === max_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMaxOverTime(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return compareOverTime(matrixVals, args, enh, func(cur, maxVal float64) bool {
		return (cur > maxVal) || math.IsNaN(maxVal)
	}, false)
}

// === min_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMinOverTime(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return compareOverTime(matrixVals, args, enh, func(cur, maxVal float64) bool {
		return (cur < maxVal) || math.IsNaN(maxVal)
	}, false)
}

// === sum_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcSumOverTime(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	firstSeries := matrixVal[0]
	if len(firstSeries.Floats) > 0 && len(firstSeries.Histograms) > 0 {
		return enh.Out, annotations.New().Add(annotations.NewMixedFloatsHistogramsWarning(getMetricName(firstSeries.Metric), args[0].PositionRange()))
	}
	if len(firstSeries.Floats) == 0 {
		// The passed values only contain histograms.
		var annos annotations.Annotations
		vec, err := aggrHistOverTime(matrixVal, enh, func(s Series) (*histogram.FloatHistogram, error) {
			var counterResetSeen, notCounterResetSeen, nhcbBoundsReconciledSeen bool

			trackCounterReset := func(h *histogram.FloatHistogram) {
				switch h.CounterResetHint {
				case histogram.CounterReset:
					counterResetSeen = true
				case histogram.NotCounterReset:
					notCounterResetSeen = true
				}
			}

			defer func() {
				if counterResetSeen && notCounterResetSeen {
					annos.Add(annotations.NewHistogramCounterResetCollisionWarning(args[0].PositionRange(), annotations.HistogramAgg))
				}
				if nhcbBoundsReconciledSeen {
					annos.Add(annotations.NewMismatchedCustomBucketsHistogramsInfo(args[0].PositionRange(), annotations.HistogramAgg))
				}
			}()

			sum := s.Histograms[0].H.Copy()
			trackCounterReset(sum)
			var (
				comp                 *histogram.FloatHistogram
				nhcbBoundsReconciled bool
				err                  error
			)
			for _, h := range s.Histograms[1:] {
				trackCounterReset(h.H)
				comp, _, nhcbBoundsReconciled, err = sum.KahanAdd(h.H, comp)
				if err != nil {
					return sum, err
				}
				if nhcbBoundsReconciled {
					nhcbBoundsReconciledSeen = true
				}
			}
			if comp != nil {
				sum, _, nhcbBoundsReconciled, err = sum.Add(comp)
				if err != nil {
					return sum, err
				}
				if nhcbBoundsReconciled {
					nhcbBoundsReconciledSeen = true
				}
			}
			return sum, err
		})
		if err != nil {
			if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
				return enh.Out, annotations.New().Add(annotations.NewMixedExponentialCustomHistogramsWarning(getMetricName(firstSeries.Metric), args[0].PositionRange()))
			}
		}
		return vec, annos
	}
	return aggrOverTime(matrixVal, enh, func(s Series) float64 {
		var sum, c float64
		for _, f := range s.Floats {
			sum, c = kahansum.Inc(f.F, sum, c)
		}
		if math.IsInf(sum, 0) {
			return sum
		}
		return sum + c
	}), nil
}

// === quantile_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcQuantileOverTime(vectorVals []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vectorVals) == 0 || len(vectorVals[0]) == 0 || len(matrixVal) == 0 {
		return enh.Out, nil
	}
	q := vectorVals[0][0].F
	el := matrixVal[0]
	if len(el.Floats) == 0 {
		return enh.Out, nil
	}

	var annos annotations.Annotations
	if math.IsNaN(q) || q < 0 || q > 1 {
		annos.Add(annotations.NewInvalidQuantileWarning(q, args[0].PositionRange()))
	}
	if len(el.Histograms) > 0 {
		annos.Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(el.Metric), args[0].PositionRange()))
	}
	values := make(vectorByValueHeap, 0, len(el.Floats))
	for _, f := range el.Floats {
		values = append(values, Sample{F: f.F})
	}
	return append(enh.Out, Sample{F: quantile(q, values)}), annos
}

func varianceOverTime(matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper, varianceToResult func(float64) float64) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	samples := matrixVal[0]
	var annos annotations.Annotations
	if len(samples.Floats) == 0 {
		return enh.Out, nil
	}
	if len(samples.Histograms) > 0 {
		annos.Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
	}
	return aggrOverTime(matrixVal, enh, func(s Series) float64 {
		var count float64
		var mean, cMean float64
		var aux, cAux float64
		for _, f := range s.Floats {
			count++
			delta := f.F - (mean + cMean)
			mean, cMean = kahansum.Inc(delta/count, mean, cMean)
			aux, cAux = kahansum.Inc(delta*(f.F-(mean+cMean)), aux, cAux)
		}
		variance := (aux + cAux) / count
		if varianceToResult == nil {
			return variance
		}
		return varianceToResult(variance)
	}), annos
}

// === stddev_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcStddevOverTime(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return varianceOverTime(matrixVals, args, enh, math.Sqrt)
}

// === stdvar_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcStdvarOverTime(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return varianceOverTime(matrixVals, args, enh, nil)
}

// === absent(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAbsent(vectorVals []Vector, _ Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vectorVals[0]) > 0 {
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
func funcAbsentOverTime(_ []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return append(enh.Out, Sample{F: 1}), nil
}

// === present_over_time(Vector parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcPresentOverTime(_ []Vector, matrixVals Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return aggrOverTime(matrixVals, enh, func(Series) float64 {
		return 1
	}), nil
}

func simpleFloatFunc(vectorVals []Vector, enh *EvalNodeHelper, f func(float64) float64) Vector {
	for _, el := range vectorVals[0] {
		if el.H == nil { // Process only float samples.
			if !enh.enableDelayedNameRemoval {
				el.Metric = el.Metric.DropReserved(schema.IsMetadataLabel)
			}
			enh.Out = append(enh.Out, Sample{
				Metric:   el.Metric,
				F:        f(el.F),
				DropName: true,
			})
		}
	}
	return enh.Out
}

// === abs(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAbs(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Abs), nil
}

// === ceil(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcCeil(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Ceil), nil
}

// === floor(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcFloor(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Floor), nil
}

// === exp(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcExp(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Exp), nil
}

// === sqrt(Vector VectorNode) (Vector, Annotations) ===
func funcSqrt(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Sqrt), nil
}

// === max_of(a, b parser.ValueTypeScalar) Scalar ===
func funcMaxOf(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return append(enh.Out, Sample{F: math.Max(vectorVals[0][0].F, vectorVals[1][0].F)}), nil
}

// === min_of(a, b parser.ValueTypeScalar) Scalar ===
func funcMinOf(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return append(enh.Out, Sample{F: math.Min(vectorVals[0][0].F, vectorVals[1][0].F)}), nil
}

// === ln(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcLn(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Log), nil
}

// === log2(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcLog2(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Log2), nil
}

// === log10(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcLog10(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Log10), nil
}

// === sin(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcSin(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Sin), nil
}

// === cos(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcCos(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Cos), nil
}

// === tan(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcTan(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Tan), nil
}

// === asin(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAsin(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Asin), nil
}

// === acos(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAcos(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Acos), nil
}

// === atan(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAtan(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Atan), nil
}

// === sinh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcSinh(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Sinh), nil
}

// === cosh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcCosh(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Cosh), nil
}

// === tanh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcTanh(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Tanh), nil
}

// === asinh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAsinh(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Asinh), nil
}

// === acosh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAcosh(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Acosh), nil
}

// === atanh(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcAtanh(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, math.Atanh), nil
}

// === rad(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcRad(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, func(v float64) float64 {
		return v * math.Pi / 180
	}), nil
}

// === deg(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcDeg(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, func(v float64) float64 {
		return v * 180 / math.Pi
	}), nil
}

// === pi() Scalar ===
func funcPi([]Vector, Matrix, parser.Expressions, *EvalNodeHelper) (Vector, annotations.Annotations) {
	return Vector{Sample{F: math.Pi}}, nil
}

// === sgn(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcSgn(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleFloatFunc(vectorVals, enh, func(v float64) float64 {
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
func funcTimestamp(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vectorVals[0]
	for _, el := range vec {
		if !enh.enableDelayedNameRemoval {
			el.Metric = el.Metric.DropReserved(schema.IsMetadataLabel)
		}
		enh.Out = append(enh.Out, Sample{
			Metric:   el.Metric,
			F:        float64(el.T) / 1000,
			DropName: true,
		})
	}
	return enh.Out, nil
}

// === start_timestamp(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcStartTimestamp(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	vec := vectorVals[0]
	var sts []int64
	if enh.StartTimestamps != nil {
		sts = enh.StartTimestamps.Floats
	}
	for i, el := range vec {
		if !enh.enableDelayedNameRemoval {
			el.Metric = el.Metric.DropReserved(schema.IsMetadataLabel)
		}

		if i >= len(sts) {
			// Only return results if start timestamps slice is populated. This means that the output is empty
			// when `use-start-timestamps` is disabled or when this function is called on an expression.
			continue
		}

		enh.Out = append(enh.Out, Sample{
			Metric:   el.Metric,
			F:        float64(sts[i]) / 1000,
			DropName: true,
		})
	}
	return enh.Out, nil
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
		sumX, cX = kahansum.Inc(x, sumX, cX)
		sumY, cY = kahansum.Inc(sample.F, sumY, cY)
		sumXY, cXY = kahansum.Inc(x*sample.F, sumXY, cXY)
		sumX2, cX2 = kahansum.Inc(x*x, sumX2, cX2)
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
func funcDeriv(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	samples := matrixVal[0]

	// No sense in trying to compute a derivative without at least two float points.
	// Drop this Vector element.
	if len(samples.Floats) < 2 {
		// Annotate mix of float and histogram.
		if len(samples.Floats) == 1 && len(samples.Histograms) > 0 {
			return enh.Out, annotations.New().Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
		}
		return enh.Out, nil
	}

	// We pass in an arbitrary timestamp that is near the values in use
	// to avoid floating point accuracy issues, see
	// https://github.com/prometheus/prometheus/issues/2674
	slope, _ := linearRegression(samples.Floats, samples.Floats[0].T)
	if len(samples.Histograms) > 0 {
		return append(enh.Out, Sample{F: slope}), annotations.New().Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
	}
	return append(enh.Out, Sample{F: slope}), nil
}

// === predict_linear(node parser.ValueTypeMatrix, k parser.ValueTypeScalar) (Vector, Annotations) ===
func funcPredictLinear(vectorVals []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vectorVals) == 0 || len(vectorVals[0]) == 0 || len(matrixVal) == 0 {
		return enh.Out, nil
	}
	samples := matrixVal[0]
	duration := vectorVals[0][0].F

	// No sense in trying to predict anything without at least two float points.
	// Drop this Vector element.
	if len(samples.Floats) < 2 {
		// Annotate mix of float and histogram.
		if len(samples.Floats) == 1 && len(samples.Histograms) > 0 {
			return enh.Out, annotations.New().Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
		}
		return enh.Out, nil
	}

	slope, intercept := linearRegression(samples.Floats, enh.Ts)
	if len(samples.Histograms) > 0 {
		return append(enh.Out, Sample{F: slope*duration + intercept}), annotations.New().Add(annotations.NewHistogramIgnoredInMixedRangeInfo(getMetricName(samples.Metric), args[0].PositionRange()))
	}
	return append(enh.Out, Sample{F: slope*duration + intercept}), nil
}

func simpleHistogramFunc(vectorVals []Vector, enh *EvalNodeHelper, f func(h *histogram.FloatHistogram) float64) Vector {
	for _, el := range vectorVals[0] {
		if el.H != nil { // Process only histogram samples.
			if !enh.enableDelayedNameRemoval {
				el.Metric = el.Metric.DropReserved(func(n string) bool { return n == labels.MetricName })
			}
			enh.Out = append(enh.Out, Sample{
				Metric:   el.Metric,
				F:        f(el.H),
				DropName: true,
			})
		}
	}
	return enh.Out
}

// === histogram_count(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramCount(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleHistogramFunc(vectorVals, enh, func(h *histogram.FloatHistogram) float64 {
		return h.Count
	}), nil
}

// === histogram_sum(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramSum(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleHistogramFunc(vectorVals, enh, func(h *histogram.FloatHistogram) float64 {
		return h.Sum
	}), nil
}

// === histogram_avg(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramAvg(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return simpleHistogramFunc(vectorVals, enh, func(h *histogram.FloatHistogram) float64 {
		return h.Sum / h.Count
	}), nil
}

func histogramVariance(vectorVals []Vector, enh *EvalNodeHelper, varianceToResult func(float64) float64) (Vector, annotations.Annotations) {
	return simpleHistogramFunc(vectorVals, enh, func(h *histogram.FloatHistogram) float64 {
		mean := h.Sum / h.Count
		var variance, cVariance float64
		it := h.AllBucketIterator()
		for it.Next() {
			bucket := it.At()
			if bucket.Count == 0 {
				continue
			}
			var val float64
			switch {
			case h.UsesCustomBuckets():
				// Use arithmetic mean in case of custom buckets.
				val = (bucket.Upper + bucket.Lower) / 2.0
			case bucket.Lower <= 0 && bucket.Upper >= 0:
				// Use zero (effectively the arithmetic mean) in the zero bucket of a standard exponential histogram.
				val = 0
			default:
				// Use geometric mean in case of standard exponential buckets.
				val = math.Sqrt(bucket.Upper * bucket.Lower)
				if bucket.Upper < 0 {
					val = -val
				}
			}
			delta := val - mean
			variance, cVariance = kahansum.Inc(bucket.Count*delta*delta, variance, cVariance)
		}
		variance += cVariance
		variance /= h.Count
		if varianceToResult != nil {
			variance = varianceToResult(variance)
		}
		return variance
	}), nil
}

// === histogram_stddev(Vector parser.ValueTypeVector) (Vector, Annotations)  ===
func funcHistogramStdDev(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return histogramVariance(vectorVals, enh, math.Sqrt)
}

// === histogram_stdvar(Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramStdVar(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return histogramVariance(vectorVals, enh, nil)
}

// === histogram_fraction(lower, upper parser.ValueTypeScalar, Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramFraction(vectorVals []Vector, _ Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vectorVals) < 3 || len(vectorVals[0]) == 0 || len(vectorVals[1]) == 0 {
		return enh.Out, nil
	}
	lower := vectorVals[0][0].F
	upper := vectorVals[1][0].F
	inVec := vectorVals[2]

	annos := enh.resetHistograms(inVec, args[2])

	// Deal with the native histograms.
	for _, sample := range enh.nativeHistogramSamples {
		if sample.H == nil {
			// Native histogram conflicts with classic histogram at the same timestamp, ignore.
			continue
		}
		if !enh.enableDelayedNameRemoval {
			sample.Metric = sample.Metric.DropReserved(schema.IsMetadataLabel)
		}
		hf, hfAnnos := HistogramFraction(lower, upper, sample.H, getMetricName(sample.Metric), args[0].PositionRange())
		annos.Merge(hfAnnos)
		enh.Out = append(enh.Out, Sample{
			Metric:   sample.Metric,
			F:        hf,
			DropName: true,
		})
	}

	// Deal with classic histograms that have already been filtered for conflicting native histograms.
	for _, mb := range enh.signatureToMetricWithBuckets {
		if len(mb.buckets) == 0 {
			continue
		}
		if !enh.enableDelayedNameRemoval {
			mb.metric = mb.metric.DropReserved(schema.IsMetadataLabel)
		}

		enh.Out = append(enh.Out, Sample{
			Metric:   mb.metric,
			F:        BucketFraction(lower, upper, mb.buckets),
			DropName: true,
		})
	}

	return enh.Out, annos
}

// === histogram_quantile(k parser.ValueTypeScalar, Vector parser.ValueTypeVector) (Vector, Annotations) ===
func funcHistogramQuantile(vectorVals []Vector, _ Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vectorVals) < 2 || len(vectorVals[0]) == 0 {
		return enh.Out, nil
	}
	q := vectorVals[0][0].F
	inVec := vectorVals[1]
	var annos annotations.Annotations

	if err := validateQuantile(q, args[0]); err != nil {
		annos.Add(err)
	}
	annos.Merge(enh.resetHistograms(inVec, args[1]))

	// Deal with the native histograms.
	for _, sample := range enh.nativeHistogramSamples {
		if sample.H == nil {
			// Native histogram conflicts with classic histogram at the same timestamp, ignore.
			continue
		}
		if !enh.enableDelayedNameRemoval {
			sample.Metric = sample.Metric.DropReserved(schema.IsMetadataLabel)
		}
		hq, hqAnnos := HistogramQuantile(q, sample.H, getMetricName(sample.Metric), args[0].PositionRange())
		annos.Merge(hqAnnos)
		enh.Out = append(enh.Out, Sample{
			Metric:   sample.Metric,
			F:        hq,
			DropName: true,
		})
	}

	// Deal with classic histograms that have already been filtered for conflicting native histograms.
	for _, mb := range enh.signatureToMetricWithBuckets {
		if len(mb.buckets) > 0 {
			quantile, forcedMonotonicity, _, minBucket, maxBucket, maxDiff := BucketQuantile(q, mb.buckets)
			if forcedMonotonicity {
				metricName := ""
				if enh.enableDelayedNameRemoval {
					metricName = getMetricName(mb.metric)
				}
				annos.Add(annotations.NewHistogramQuantileForcedMonotonicityInfo(metricName, args[1].PositionRange(), enh.Ts, minBucket, maxBucket, maxDiff))
			}

			if !enh.enableDelayedNameRemoval {
				mb.metric = mb.metric.DropReserved(schema.IsMetadataLabel)
			}

			enh.Out = append(enh.Out, Sample{
				Metric:   mb.metric,
				F:        quantile,
				DropName: true,
			})
		}
	}

	return enh.Out, annos
}

func validateQuantile(q float64, arg parser.Expr) error {
	if math.IsNaN(q) || q < 0 || q > 1 {
		return annotations.NewInvalidQuantileWarning(q, arg.PositionRange())
	}
	return nil
}

// === histogram_quantiles(Vector parser.ValueTypeVector, label parser.ValueTypeString, q0 parser.ValueTypeScalar, qs parser.ValueTypeScalar...) (Vector, Annotations) ===
func funcHistogramQuantiles(vectorVals []Vector, _ Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	var (
		inVec         = vectorVals[0]
		quantileLabel = args[1].(*parser.StringLiteral).Val
		numQuantiles  = len(vectorVals[2:])
		qs            = make([]float64, 0, numQuantiles)

		annos annotations.Annotations
	)

	if enh.quantileStrs == nil {
		enh.quantileStrs = make(map[float64]string, numQuantiles)
	}
	for i := 2; i < len(vectorVals); i++ {
		q := vectorVals[i][0].F

		if err := validateQuantile(q, args[i]); err != nil {
			annos.Add(err)
		}

		if _, ok := enh.quantileStrs[q]; !ok {
			enh.quantileStrs[q] = labels.FormatOpenMetricsFloat(q)
		}
		qs = append(qs, q)
	}

	annos.Merge(enh.resetHistograms(inVec, args[0]))

	for _, q := range qs {
		// Deal with the native histograms.
		for _, sample := range enh.nativeHistogramSamples {
			if sample.H == nil {
				// Native histogram conflicts with classic histogram at the same timestamp, ignore.
				continue
			}
			if !enh.enableDelayedNameRemoval {
				sample.Metric = sample.Metric.DropReserved(schema.IsMetadataLabel)
			}
			hq, hqAnnos := HistogramQuantile(q, sample.H, sample.Metric.Get(model.MetricNameLabel), args[0].PositionRange())
			annos.Merge(hqAnnos)
			enh.Out = append(enh.Out, Sample{
				Metric:   enh.getOrCreateLblsWithQuantile(sample.Metric, quantileLabel, q),
				F:        hq,
				DropName: true,
			})
		}

		// Deal with classic histograms that have already been filtered for conflicting native histograms.
		for _, mb := range enh.signatureToMetricWithBuckets {
			if len(mb.buckets) > 0 {
				hq, forcedMonotonicity, _, minBucket, maxBucket, maxDiff := BucketQuantile(q, mb.buckets)
				if forcedMonotonicity {
					metricName := ""
					if enh.enableDelayedNameRemoval {
						metricName = getMetricName(mb.metric)
					}
					annos.Add(annotations.NewHistogramQuantileForcedMonotonicityInfo(metricName, args[0].PositionRange(), enh.Ts, minBucket, maxBucket, maxDiff))
				}

				if !enh.enableDelayedNameRemoval {
					mb.metric = mb.metric.DropReserved(schema.IsMetadataLabel)
				}

				enh.Out = append(enh.Out, Sample{
					Metric:   enh.getOrCreateLblsWithQuantile(mb.metric, quantileLabel, q),
					F:        hq,
					DropName: true,
				})
			}
		}
	}

	return enh.Out, annos
}

// pickFirstSampleIndices returns the start indices into the floats and
// histograms slices for anchored range processing. The anchor is the single
// most recent sample at or before the range start, regardless of type; it is
// retained as the baseline together with every sample after the range start,
// while earlier samples of either type are skipped.
// If the vector selector is not anchored, it always returns 0, 0, true.
// found is false when no sample lies strictly after the range start, in which
// case there is nothing to measure.
func pickFirstSampleIndices(floats []FPoint, histograms []HPoint, args parser.Expressions, enh *EvalNodeHelper) (firstFloat, firstHistogram int, found bool) {
	ms := args[0].(*parser.MatrixSelector)
	vs := ms.VectorSelector.(*parser.VectorSelector)
	if !vs.Anchored {
		return 0, 0, true
	}
	rangeStart := enh.Ts - durationMilliseconds(ms.Range+vs.Offset)

	// Index of the last float / histogram at or before the range start, or -1.
	lastFloatLE := sort.Search(len(floats), func(i int) bool { return floats[i].T > rangeStart }) - 1
	lastHistLE := sort.Search(len(histograms), func(i int) bool { return histograms[i].T > rangeStart }) - 1

	// Without a sample strictly after the range start there is nothing to measure.
	if lastFloatLE+1 >= len(floats) && lastHistLE+1 >= len(histograms) {
		return 0, 0, false
	}

	switch {
	case lastFloatLE < 0 && lastHistLE < 0:
		// No anchor; every sample is after the range start, so include all.
		return 0, 0, true
	case lastHistLE < 0 || (lastFloatLE >= 0 && floats[lastFloatLE].T >= histograms[lastHistLE].T):
		// The anchor is a float; histograms at or before the range start precede
		// it and are skipped.
		return lastFloatLE, lastHistLE + 1, true
	default:
		// The anchor is a histogram; floats at or before the range start precede
		// it and are skipped.
		return lastFloatLE + 1, lastHistLE, true
	}
}

// === resets(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcResets(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	floats := matrixVal[0].Floats
	histograms := matrixVal[0].Histograms
	resets := 0
	if len(floats) == 0 && len(histograms) == 0 {
		return enh.Out, nil
	}

	var (
		prevSample, curSample  Sample
		prevST                 int64
		floatSTs, histogramSTs []int64
	)
	if sts := enh.StartTimestamps; sts != nil {
		floatSTs = sts.Floats
		histogramSTs = sts.Histograms
	}
	firstFloat, firstHistogram, found := pickFirstSampleIndices(floats, histograms, args, enh)
	if !found {
		return enh.Out, nil
	}
	for iFloat, iHistogram := firstFloat, firstHistogram; iFloat < len(floats) || iHistogram < len(histograms); {
		var curST int64
		switch {
		// Process a float sample if no histogram sample remains or its timestamp is earlier.
		// Process a histogram sample if no float sample remains or its timestamp is earlier.
		case iHistogram >= len(histograms) || iFloat < len(floats) && floats[iFloat].T < histograms[iHistogram].T:
			curSample.T = floats[iFloat].T
			curSample.F = floats[iFloat].F
			curSample.H = nil
			if iFloat < len(floatSTs) {
				curST = floatSTs[iFloat]
			}
			iFloat++
		case iFloat >= len(floats) || iHistogram < len(histograms) && floats[iFloat].T > histograms[iHistogram].T:
			curSample.T = histograms[iHistogram].T
			curSample.H = histograms[iHistogram].H
			if iHistogram < len(histogramSTs) {
				curST = histogramSTs[iHistogram]
			}
			iHistogram++
		}
		// Skip the comparison for the first sample, just initialize prevSample.
		if iFloat+iHistogram == 1+firstFloat+firstHistogram {
			prevSample = curSample
			prevST = curST
			continue
		}

		switch {
		case prevSample.H == nil && curSample.H == nil:
			if curSample.F < prevSample.F || isStartTimestampReset(prevST, prevSample.T, curST, curSample.T) {
				resets++
			}
		case prevSample.H != nil && curSample.H == nil, prevSample.H == nil && curSample.H != nil:
			resets++
		case prevSample.H != nil && curSample.H != nil:
			if isStartTimestampReset(prevST, prevSample.T, curST, curSample.T) || curSample.H.DetectReset(prevSample.H) {
				resets++
			}
		}
		prevSample = curSample
		prevST = curST
	}

	return append(enh.Out, Sample{F: float64(resets)}), nil
}

// === changes(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcChanges(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	floats := matrixVal[0].Floats
	histograms := matrixVal[0].Histograms
	changes := 0
	if len(floats) == 0 && len(histograms) == 0 {
		return enh.Out, nil
	}

	var prevSample, curSample Sample
	firstFloat, firstHistogram, found := pickFirstSampleIndices(floats, histograms, args, enh)
	if !found {
		return enh.Out, nil
	}
	for iFloat, iHistogram := firstFloat, firstHistogram; iFloat < len(floats) || iHistogram < len(histograms); {
		switch {
		// Process a float sample if no histogram sample remains or its timestamp is earlier.
		// Process a histogram sample if no float sample remains or its timestamp is earlier.
		case iHistogram >= len(histograms) || iFloat < len(floats) && floats[iFloat].T < histograms[iHistogram].T:
			curSample.F = floats[iFloat].F
			curSample.H = nil
			iFloat++
		case iFloat >= len(floats) || iHistogram < len(histograms) && floats[iFloat].T > histograms[iHistogram].T:
			curSample.H = histograms[iHistogram].H
			iHistogram++
		}
		// Skip the comparison for the first sample, just initialize prevSample.
		if iFloat+iHistogram == 1+firstFloat+firstHistogram {
			prevSample = curSample
			continue
		}
		switch {
		case prevSample.H == nil && curSample.H == nil:
			if curSample.F != prevSample.F && !(math.IsNaN(curSample.F) && math.IsNaN(prevSample.F)) {
				changes++
			}
		case prevSample.H != nil && curSample.H == nil, prevSample.H == nil && curSample.H != nil:
			changes++
		case prevSample.H != nil && curSample.H != nil:
			if !curSample.H.Equals(prevSample.H) {
				changes++
			}
		}
		prevSample = curSample
	}

	return append(enh.Out, Sample{F: float64(changes)}), nil
}

// label_replace function operates only on series; does not look at timestamps or values.
func (ev *evaluator) evalLabelReplace(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	var (
		dst      = stringFromArg(args[1])
		repl     = stringFromArg(args[2])
		src      = stringFromArg(args[3])
		regexStr = stringFromArg(args[4])
	)

	regex, err := regexp.Compile("^(?s:" + regexStr + ")$")
	if err != nil {
		panic(fmt.Errorf("invalid regular expression in label_replace(): %s", regexStr))
	}
	if !model.UTF8Validation.IsValidLabelName(dst) {
		panic(fmt.Errorf("invalid destination label name in label_replace(): %s", dst))
	}

	val, ws := ev.eval(ctx, args[0])
	matrix := val.(Matrix)
	lb := labels.NewBuilder(labels.EmptyLabels())

	for i, el := range matrix {
		srcVal := el.Metric.Get(src)
		indexes := regex.FindStringSubmatchIndex(srcVal)
		if indexes != nil { // Only replace when regexp matches.
			res := regex.ExpandString([]byte{}, repl, srcVal, indexes)
			lb.Reset(el.Metric)
			lb.Set(dst, string(res))
			matrix[i].Metric = lb.Labels()
			if dst == model.MetricNameLabel {
				matrix[i].DropName = false
			} else {
				matrix[i].DropName = el.DropName
			}
		}
	}

	return ev.mergeSeriesWithSameLabelset(matrix), ws
}

// === Vector(s Scalar) (Vector, Annotations) ===
func funcVector(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return append(enh.Out,
		Sample{
			Metric: labels.Labels{},
			F:      vectorVals[0][0].F,
		}), nil
}

// label_join function operates only on series; does not look at timestamps or values.
func (ev *evaluator) evalLabelJoin(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	var (
		dst       = stringFromArg(args[1])
		sep       = stringFromArg(args[2])
		srcLabels = make([]string, len(args)-3)
	)
	for i := 3; i < len(args); i++ {
		src := stringFromArg(args[i])
		if !model.UTF8Validation.IsValidLabelName(src) {
			panic(fmt.Errorf("invalid source label name in label_join(): %s", src))
		}
		srcLabels[i-3] = src
	}
	if !model.UTF8Validation.IsValidLabelName(dst) {
		panic(fmt.Errorf("invalid destination label name in label_join(): %s", dst))
	}

	val, ws := ev.eval(ctx, args[0])
	matrix := val.(Matrix)
	srcVals := make([]string, len(srcLabels))
	lb := labels.NewBuilder(labels.EmptyLabels())

	for i, el := range matrix {
		for i, src := range srcLabels {
			srcVals[i] = el.Metric.Get(src)
		}
		strval := strings.Join(srcVals, sep)
		lb.Reset(el.Metric)
		lb.Set(dst, strval)
		matrix[i].Metric = lb.Labels()

		if dst == model.MetricNameLabel {
			matrix[i].DropName = false
		} else {
			matrix[i].DropName = el.DropName
		}
	}

	return ev.mergeSeriesWithSameLabelset(matrix), ws
}

// Common code for date related functions.
func dateWrapper(vectorVals []Vector, enh *EvalNodeHelper, f func(time.Time) float64) Vector {
	if len(vectorVals) == 0 {
		return append(enh.Out,
			Sample{
				Metric: labels.Labels{},
				F:      f(time.Unix(enh.Ts/1000, 0).UTC()),
			})
	}

	for _, el := range vectorVals[0] {
		if el.H != nil {
			// Ignore histogram sample.
			continue
		}
		t := time.Unix(int64(el.F), 0).UTC()
		if !enh.enableDelayedNameRemoval {
			el.Metric = el.Metric.DropReserved(schema.IsMetadataLabel)
		}
		enh.Out = append(enh.Out, Sample{
			Metric:   el.Metric,
			F:        f(t),
			DropName: true,
		})
	}
	return enh.Out
}

// === days_in_month(v Vector) Scalar ===
func funcDaysInMonth(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(32 - time.Date(t.Year(), t.Month(), 32, 0, 0, 0, 0, time.UTC).Day())
	}), nil
}

// === day_of_month(v Vector) Scalar ===
func funcDayOfMonth(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.Day())
	}), nil
}

// === day_of_week(v Vector) Scalar ===
func funcDayOfWeek(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.Weekday())
	}), nil
}

// === day_of_year(v Vector) Scalar ===
func funcDayOfYear(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.YearDay())
	}), nil
}

// === hour(v Vector) Scalar ===
func funcHour(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.Hour())
	}), nil
}

// === minute(v Vector) Scalar ===
func funcMinute(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.Minute())
	}), nil
}

// === month(v Vector) Scalar ===
func funcMonth(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.Month())
	}), nil
}

// === year(v Vector) Scalar ===
func funcYear(vectorVals []Vector, _ Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return dateWrapper(vectorVals, enh, func(t time.Time) float64 {
		return float64(t.Year())
	}), nil
}

// FunctionCalls is a list of all functions supported by PromQL, including their types.
var FunctionCalls = map[string]FunctionCall{
	"abs":                          funcAbs,
	"absent":                       funcAbsent,
	"absent_over_time":             funcAbsentOverTime,
	"acos":                         funcAcos,
	"acosh":                        funcAcosh,
	"asin":                         funcAsin,
	"asinh":                        funcAsinh,
	"atan":                         funcAtan,
	"atanh":                        funcAtanh,
	"avg_over_time":                funcAvgOverTime,
	"burst_score":                  funcBurstScore,
	"ceil":                         funcCeil,
	"changepoint":                  funcChangepoint,
	"changes":                      funcChanges,
	"clamp":                        funcClamp,
	"clamp_max":                    funcClampMax,
	"clamp_min":                    funcClampMin,
	"cos":                          funcCos,
	"cosh":                         funcCosh,
	"count_over_time":              funcCountOverTime,
	"days_in_month":                funcDaysInMonth,
	"day_of_month":                 funcDayOfMonth,
	"day_of_week":                  funcDayOfWeek,
	"day_of_year":                  funcDayOfYear,
	"deg":                          funcDeg,
	"delta":                        funcDelta,
	"deriv":                        funcDeriv,
	"end":                          nil, // Folded into NumberLiteral by foldQueryContextFunctions.
	"entropy":                      funcEntropy,
	"ewma":                         funcEWMA,
	"exp":                          funcExp,
	"first_over_time":              funcFirstOverTime,
	"floor":                        funcFloor,
	"histogram_avg":                funcHistogramAvg,
	"histogram_count":              funcHistogramCount,
	"histogram_fraction":           funcHistogramFraction,
	"histogram_quantile":           funcHistogramQuantile,
	"histogram_quantiles":          funcHistogramQuantiles,
	"histogram_sum":                funcHistogramSum,
	"histogram_stddev":             funcHistogramStdDev,
	"histogram_stdvar":             funcHistogramStdVar,
	"double_exponential_smoothing": funcDoubleExponentialSmoothing,
	"hst":                          funcHST,
	"hour":                         funcHour,
	"hw":                           funcHW,
	"idelta":                       funcIdelta,
	"increase":                     funcIncrease,
	"info":                         nil,
	"irate":                        funcIrate,
	"isf":                          funcIsolationForest,
	"max_of":                       funcMaxOf,
	"label_replace":                nil, // evalLabelReplace not called via this map.
	"label_join":                   nil, // evalLabelJoin not called via this map.
	"min_of":                       funcMinOf,
	"ln":                           funcLn,
	"log10":                        funcLog10,
	"log2":                         funcLog2,
	"last_over_time":               funcLastOverTime,
	"mad":                          funcMAD,
	"mad_over_time":                funcMadOverTime,
	"max_over_time":                funcMaxOverTime,
	"min_over_time":                funcMinOverTime,
	"ts_of_first_over_time":        funcTsOfFirstOverTime,
	"ts_of_last_over_time":         funcTsOfLastOverTime,
	"ts_of_max_over_time":          funcTsOfMaxOverTime,
	"ts_of_min_over_time":          funcTsOfMinOverTime,
	"minute":                       funcMinute,
	"month":                        funcMonth,
	"pi":                           funcPi,
	"predict_linear":               funcPredictLinear,
	"present_over_time":            funcPresentOverTime,
	"qscore":                       funcQScore,
	"quantile_over_time":           funcQuantileOverTime,
	"rad":                          funcRad,
	"range":                        nil, // Folded into NumberLiteral by foldQueryContextFunctions.
	"rate":                         funcRate,
	"random_cut_score":             funcRandomCutScore,
	"rcf":                          funcRCF,
	"rcf_attribution":              nil, // evalRCFAttribution not called via this map.
	"resets":                       funcResets,
	"round":                        funcRound,
	"scalar":                       funcScalar,
	"seasonal":                     funcSeasonal,
	"sgn":                          funcSgn,
	"sin":                          funcSin,
	"sinh":                         funcSinh,
	"sort":                         funcSort,
	"sort_desc":                    funcSortDesc,
	"sort_by_label":                funcSortByLabel,
	"sort_by_label_desc":           funcSortByLabelDesc,
	"start":                        nil, // Folded into NumberLiteral by foldQueryContextFunctions.
	"start_timestamp":              funcStartTimestamp,
	"step":                         nil, // Folded into NumberLiteral by foldQueryContextFunctions.
	"sqrt":                         funcSqrt,
	"stddev_over_time":             funcStddevOverTime,
	"stdvar_over_time":             funcStdvarOverTime,
	"sum_over_time":                funcSumOverTime,
	"tan":                          funcTan,
	"tanh":                         funcTanh,
	"time":                         funcTime,
	"timestamp":                    funcTimestamp,
	"trend_score":                  funcTrendScore,
	"vector":                       funcVector,
	"year":                         funcYear,
	"zscore":                       funcZScore,
}

// AtModifierUnsafeFunctions are the functions whose result
// can vary if evaluation time is changed when the arguments are
// step invariant. It also includes functions that use the timestamps
// of the passed instant vector argument to calculate a result since
// that can also change with change in eval time.
var AtModifierUnsafeFunctions = map[string]struct{}{
	// Step invariant functions.
	"days_in_month": {}, "day_of_month": {}, "day_of_week": {}, "day_of_year": {},
	"end": {}, "hour": {}, "minute": {}, "month": {}, "year": {},
	"predict_linear": {}, "range": {}, "start": {}, "step": {}, "time": {},
	// Uses timestamp of the argument for the result,
	// hence unsafe to use with @ modifier.
	"timestamp": {},
}

// AnchoredSafeFunctions are the functions that can be used with the anchored
// modifier.  Anchored modifier returns matrices with samples outside of the
// boundaries, so not every function can be used with it.
var AnchoredSafeFunctions = map[string]struct{}{
	"resets":   {},
	"changes":  {},
	"rate":     {},
	"increase": {},
	"delta":    {},
}

// SmoothedSafeFunctions are the functions that can be used with the smoothed
// modifier.  Smoothed modifier returns matrices with samples outside of the
// boundaries, so not every function can be used with it.
var SmoothedSafeFunctions = map[string]struct{}{
	"rate":     {},
	"increase": {},
	"delta":    {},
}

type vectorByValueHeap Vector

func (s vectorByValueHeap) Len() int {
	return len(s)
}

func (s vectorByValueHeap) Less(i, j int) bool {
	vi, vj := s[i].F, s[j].F
	if math.IsNaN(vi) {
		return true
	}
	return vi < vj
}

func (s vectorByValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByValueHeap) Push(x any) {
	*s = append(*s, *(x.(*Sample)))
}

func (s *vectorByValueHeap) Pop() any {
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
	vi, vj := s[i].F, s[j].F
	if math.IsNaN(vi) {
		return true
	}
	return vi > vj
}

func (s vectorByReverseValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByReverseValueHeap) Push(x any) {
	*s = append(*s, *(x.(*Sample)))
}

func (s *vectorByReverseValueHeap) Pop() any {
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
	return e.(*parser.StringLiteral).Val
}

func stringSliceFromArgs(args parser.Expressions) []string {
	tmp := make([]string, len(args))
	for i := range args {
		tmp[i] = stringFromArg(args[i])
	}
	return tmp
}

func getMetricName(metric labels.Labels) string {
	return metric.Get(model.MetricNameLabel)
}

// anomalySample is a (timestamp, scalar-value) pair extracted from either a
// float sample or a native histogram sample (represented as sum/count = avg).
type anomalySample struct {
	T int64
	V float64
}

// extractAnomalySamples converts a Series into a flat []anomalySample.
// For float series it uses the raw float value.
// For histogram series it uses sum/count (the histogram average).
// Mixed float+histogram series return a MixedFloatsHistogramsWarning and nil.
func extractAnomalySamples(series Series, metricName string, pos posrange.PositionRange) ([]anomalySample, annotations.Annotations) {
	if len(series.Floats) > 0 && len(series.Histograms) > 0 {
		var annos annotations.Annotations
		return nil, annos.Add(annotations.NewMixedFloatsHistogramsWarning(metricName, pos))
	}
	if len(series.Histograms) > 0 {
		out := make([]anomalySample, 0, len(series.Histograms))
		for _, p := range series.Histograms {
			if p.H == nil || p.H.Count <= 0 {
				continue
			}
			out = append(out, anomalySample{T: p.T, V: p.H.Sum / p.H.Count})
		}
		return out, nil
	}
	out := make([]anomalySample, len(series.Floats))
	for i, p := range series.Floats {
		out[i] = anomalySample{T: p.T, V: p.F}
	}
	return out, nil
}

// clampScore clamps a score to [0, 1].
func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

// forEachAnomalySeries iterates over matrixVal, extracts anomaly samples for
// each series, skips series with fewer than minSamples, and calls scoreFn to
// compute a score. The result is appended to enh.Out.
// scoreFn returns (score, ok): if ok is false the series is skipped.
// Returns early with an annotation if extractAnomalySamples reports one.
func forEachAnomalySeries(matrixVal Matrix, enh *EvalNodeHelper, minSamples int, scoreFn func(series Series, samples []anomalySample) (float64, bool)) (Vector, annotations.Annotations) {
	for _, series := range matrixVal {
		samples, annos := extractAnomalySamples(series, getMetricName(series.Metric), posrange.PositionRange{})
		if annos != nil {
			return enh.Out, annos
		}
		if len(samples) < minSamples {
			continue
		}
		score, ok := scoreFn(series, samples)
		if !ok {
			continue
		}
		enh.Out = append(enh.Out, Sample{
			Metric: series.Metric,
			F:      clampScore(score),
			T:      samples[len(samples)-1].T,
		})
	}
	return enh.Out, nil
}

// buildFeatures converts a slice of anomaly samples into the 6-dimensional
// feature vectors used by RCF, HST, and ISF. It uses Welford online statistics
// (O(n)) instead of O(n²) sum-over-features approach.
func buildFeatures(samples []anomalySample) [][6]float64 {
	features := make([][6]float64, 0, len(samples))
	var (
		stats        onlineStats
		lastVal      float64
		lastVelocity float64
		ewmaVal      float64
		lastT        int64
	)
	for i, s := range samples {
		if i == 0 {
			lastVal = s.V
			ewmaVal = s.V
			lastT = s.T
			stats.add(s.V)
			features = append(features, [6]float64{})
			continue
		}
		stats.add(s.V)
		mean := stats.mean()
		std := stats.stdDev()
		if std < 1e-9 {
			std = 1
		}
		delta := s.V - lastVal
		dt := float64(s.T-lastT) / 1000.0
		if dt <= 0 {
			dt = 1
		}
		velocity := delta / dt
		acceleration := velocity - lastVelocity
		features = append(features, [6]float64{
			(s.V - mean) / std,
			delta / std,
			velocity / std,
			acceleration / std,
			(s.V - ewmaVal) / std,
			stats.stdDev() / math.Max(math.Abs(mean), 1e-9),
		})
		lastVal = s.V
		lastVelocity = velocity
		lastT = s.T
		ewmaVal = 0.2*s.V + 0.8*ewmaVal
	}
	return features
}

func funcEWMA(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	alpha := 0.2
	if len(args) > 1 {
		alpha = args[1].(*parser.NumberLiteral).Val
	}
	if alpha <= 0 || alpha > 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("EWMA alpha must be in (0,1], got %g", alpha))
	}
	return forEachAnomalySeries(matrixVal, enh, 2, func(_ Series, samples []anomalySample) (float64, bool) {
		baseline := samples[0].V
		variance := 0.0
		for _, s := range samples[1:] {
			delta := s.V - baseline
			baseline += alpha * delta
			variance = (1 - alpha) * (variance + alpha*delta*delta)
		}
		lastVal := samples[len(samples)-1].V
		delta := lastVal - baseline
		std := math.Sqrt(math.Max(variance, 0))
		if std > 1e-9 {
			return 1 - math.Exp(-math.Abs(delta)/(3*std)), true
		}
		return 0, true
	})
}

// funcRandomCutScore computes a stateless random-cut anomaly score.
// It repeatedly partitions the history with random axis-aligned cuts until the
// query point is isolated, returning a normalised depth score in [0, 1].
// This is NOT a streaming Random Cut Forest: no model is persisted between
// evaluations and the cost is O(trees × N log N) per call.
func funcRandomCutScore(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	trees := 100
	if len(args) > 1 {
		trees = int(args[1].(*parser.NumberLiteral).Val)
	}
	if trees < 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("random_cut_score trees must be positive, got %d", trees))
	}
	return forEachAnomalySeries(matrixVal, enh, 32, func(series Series, samples []anomalySample) (float64, bool) {
		features := buildFeatures(samples)
		if len(features) < 32 {
			return 0, false
		}
		seed := rcfSeedFromString(getMetricName(series.Metric))
		return randomCutScoreInMemory(features[:len(features)-1], features[len(features)-1], trees, seed), true
	})
}

func rcfSeedFromString(name string) uint64 {
	// FNV-1a: fast, no allocation, good avalanche.
	const (
		offset64 uint64 = 14695981039346656037
		prime64  uint64 = 1099511628211
	)
	h := offset64
	for i := range len(name) {
		h ^= uint64(name[i])
		h *= prime64
	}
	return h
}

// randomCutScoreInMemory is the stateless scoring kernel for funcRandomCutScore.
// It isolates point from points using random axis-aligned cuts and returns a
// normalised depth score. It is NOT a persistent Random Cut Forest.
func randomCutScoreInMemory(points [][6]float64, point [6]float64, trees int, seed uint64) float64 {
	if len(points) < 2 || trees == 0 {
		return 0
	}
	maxDepth := int(math.Ceil(math.Log2(float64(len(points))))) + 1
	maxDepth = max(maxDepth, 1)
	indices := make([]int, len(points))
	var total float64
	for range trees {
		for i := range points {
			indices[i] = i
		}
		count, depth := len(indices), 0
		seed = nextRandomInMemory(seed)
		for depth < maxDepth && count > 1 {
			seed = nextRandomInMemory(seed)
			dimension := int(seed % 6)
			minimum, maximum := points[indices[0]][dimension], points[indices[0]][dimension]
			for i := 1; i < count; i++ {
				value := points[indices[i]][dimension]
				if value < minimum {
					minimum = value
				}
				if value > maximum {
					maximum = value
				}
			}
			if maximum-minimum < 1e-12 {
				depth++
				continue
			}
			seed = nextRandomInMemory(seed)
			cut := minimum + (maximum-minimum)*float64(seed>>11)/float64(uint64(1)<<53)
			goesLeft := point[dimension] < cut
			nextCount := 0
			for i := 0; i < count; i++ {
				candidate := indices[i]
				if (points[candidate][dimension] < cut) == goesLeft {
					indices[nextCount] = candidate
					nextCount++
				}
			}
			count, depth = nextCount, depth+1
		}
		total += 1 - float64(depth)/float64(maxDepth)
	}
	score := total / float64(trees)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func nextRandomInMemory(value uint64) uint64 {
	value ^= value << 13
	value ^= value >> 7
	value ^= value << 17
	return value
}

func funcZScore(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	threshold := 3.0
	if len(args) > 1 {
		threshold = args[1].(*parser.NumberLiteral).Val
	}
	if threshold <= 0 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("ZScore threshold must be positive, got %g", threshold))
	}
	return forEachAnomalySeries(matrixVal, enh, 2, func(_ Series, samples []anomalySample) (float64, bool) {
		var stats onlineStats
		for _, s := range samples[:len(samples)-1] {
			stats.add(s.V)
		}
		mean := stats.mean()
		stddev := stats.stdDev()
		lastVal := samples[len(samples)-1].V
		if stddev > 1e-9 {
			return math.Abs(lastVal-mean) / stddev / threshold, true
		} else if math.Abs(lastVal-mean) > 1e-9 {
			return 1.0, true
		}
		return 0, true
	})
}

func funcSeasonal(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	period := int64(86400)
	alpha := 0.2
	if len(args) > 1 {
		period = int64(args[1].(*parser.NumberLiteral).Val)
	}
	if len(args) > 2 {
		alpha = args[2].(*parser.NumberLiteral).Val
	}
	if period <= 0 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("seasonal period must be positive, got %d", period))
	}
	if alpha <= 0 || alpha > 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("seasonal alpha must be in (0,1], got %g", alpha))
	}
	return forEachAnomalySeries(matrixVal, enh, 2, func(_ Series, samples []anomalySample) (float64, bool) {
		slotBuckets := make(map[int64][]anomalySample)
		for _, s := range samples {
			slotBuckets[s.T/1000%period] = append(slotBuckets[s.T/1000%period], s)
		}
		last := samples[len(samples)-1]
		slotPoints := slotBuckets[last.T/1000%period]
		if len(slotPoints) < 2 {
			return 0, false
		}
		baseline := slotPoints[0].V
		variance := 0.0
		for _, s := range slotPoints[1:] {
			delta := s.V - baseline
			baseline += alpha * delta
			variance = (1 - alpha) * (variance + alpha*delta*delta)
		}
		std := math.Sqrt(math.Max(variance, 0))
		if std > 1e-9 {
			return 1 - math.Exp(-math.Abs(last.V-baseline)/(3*std)), true
		}
		return 0, true
	})
}

func funcMAD(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	threshold := 3.0
	if len(args) > 1 {
		threshold = args[1].(*parser.NumberLiteral).Val
	}
	if threshold <= 0 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("MAD threshold must be positive, got %g", threshold))
	}
	return forEachAnomalySeries(matrixVal, enh, 3, func(_ Series, samples []anomalySample) (float64, bool) {
		vals := make([]float64, len(samples))
		for i, s := range samples {
			vals[i] = s.V
		}
		median := quickSelectMedian(vals)
		devs := vals
		for i, v := range vals {
			devs[i] = math.Abs(v - median)
		}
		madVal := quickSelectMedian(devs)
		lastVal := samples[len(samples)-1].V
		denom := 1.4826 * madVal
		if denom > 1e-9 {
			return math.Abs(lastVal-median) / denom / threshold, true
		} else if math.Abs(lastVal-median) > 1e-9 {
			return 1.0, true
		}
		return 0, true
	})
}

// quickSelectMedian returns the median of vals using quickselect (O(n) average).
// It partially reorders vals in place — callers must not rely on order after this call.
func quickSelectMedian(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	mid := n / 2
	v := quickSelect(vals, mid)
	if n%2 == 1 {
		return v
	}
	// Even length: average of two middle elements.
	return (v + quickSelect(vals, mid-1)) / 2
}

// quickSelect partially sorts data so that data[k] is the k-th smallest element.
func quickSelect(data []float64, k int) float64 {
	lo, hi := 0, len(data)-1
outer:
	for lo < hi {
		pivot := data[(lo+hi)/2]
		i, j := lo, hi
		for i <= j {
			for data[i] < pivot {
				i++
			}
			for data[j] > pivot {
				j--
			}
			if i <= j {
				data[i], data[j] = data[j], data[i]
				i++
				j--
			}
		}
		switch {
		case k <= j:
			hi = j
		case k >= i:
			lo = i
		default:
			break outer
		}
	}
	return data[k]
}

func funcQScore(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	lower := 0.05
	upper := 0.95
	if len(args) > 1 {
		lower = args[1].(*parser.NumberLiteral).Val
	}
	if len(args) > 2 {
		upper = args[2].(*parser.NumberLiteral).Val
	}
	if lower < 0 || lower >= upper || upper > 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("quantile lower and upper must satisfy 0 <= lower < upper <= 1, got lower=%g upper=%g", lower, upper))
	}
	return forEachAnomalySeries(matrixVal, enh, 10, func(_ Series, samples []anomalySample) (float64, bool) {
		work := make([]float64, len(samples))
		for i, s := range samples {
			work[i] = s.V
		}
		n := float64(len(work))
		minVal := quickSelect(work, 0)
		maxVal := quickSelect(work, len(work)-1)
		qLower := quickSelect(work, int(math.Floor(lower*(n-1))))
		qUpper := quickSelect(work, int(math.Ceil(upper*(n-1))))
		lastVal := samples[len(samples)-1].V
		if lastVal > qUpper {
			denom := maxVal - qUpper
			if denom > 1e-9 {
				return (lastVal - qUpper) / denom, true
			}
			return 1.0, true
		} else if lastVal < qLower {
			denom := qLower - minVal
			if denom > 1e-9 {
				return (qLower - lastVal) / denom, true
			}
			return 1.0, true
		}
		return 0, true
	})
}

func funcHW(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	alpha := 0.2
	beta := 0.1
	if len(args) > 1 {
		alpha = args[1].(*parser.NumberLiteral).Val
	}
	if len(args) > 2 {
		beta = args[2].(*parser.NumberLiteral).Val
	}
	if alpha <= 0 || alpha > 1 || beta <= 0 || beta > 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("Holt-Winters alpha and beta must be in (0,1], got alpha=%g beta=%g", alpha, beta))
	}
	return forEachAnomalySeries(matrixVal, enh, 3, func(_ Series, samples []anomalySample) (float64, bool) {
		level := samples[0].V
		trend := samples[1].V - samples[0].V
		varianceError := 0.0
		for _, s := range samples[2:] {
			forecast := level + trend
			errorVal := s.V - forecast
			prevLevel := level
			level = alpha*s.V + (1-alpha)*(level+trend)
			trend = beta*(level-prevLevel) + (1-beta)*trend
			varianceError = (1-alpha)*varianceError + alpha*errorVal*errorVal
		}
		forecast := level + trend
		errorVal := samples[len(samples)-1].V - forecast
		stdErr := math.Sqrt(math.Max(varianceError, 0))
		if stdErr > 1e-9 {
			return math.Abs(errorVal) / (3.0 * stdErr), true
		}
		return 0, true
	})
}

type onlineStats struct {
	count int
	mu    float64
	m2    float64
}

func (s *onlineStats) add(v float64) {
	s.count++
	delta := v - s.mu
	s.mu += delta / float64(s.count)
	delta2 := v - s.mu
	s.m2 += delta * delta2
}

func (s *onlineStats) mean() float64 {
	return s.mu
}

func (s *onlineStats) variance() float64 {
	if s.count < 2 {
		return 0
	}

	return s.m2 / float64(s.count-1)
}

func (s *onlineStats) stdDev() float64 {
	return math.Sqrt(math.Max(s.variance(), 0))
}

func funcHST(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	trees := 100
	depth := 8
	if len(args) > 1 {
		trees = int(args[1].(*parser.NumberLiteral).Val)
	}
	if len(args) > 2 {
		depth = int(args[2].(*parser.NumberLiteral).Val)
	}
	if trees < 1 || depth < 1 {
		return enh.Out, annotations.NewInvalidParamError(
			fmt.Errorf("HST trees and depth must be positive, got trees=%d depth=%d", trees, depth),
		)
	}
	return forEachAnomalySeries(matrixVal, enh, 32, func(series Series, samples []anomalySample) (float64, bool) {
		features := buildFeatures(samples)
		if len(features) < 32 {
			return 0, false
		}
		return hstScore(features, trees, depth, getMetricName(series.Metric)), true
	})
}

func hstScore(features [][6]float64, trees, depth int, metric string) float64 {
	target := features[len(features)-1]
	history := features[:len(features)-1]
	seed := rcfSeedFromString(metric)
	var avgDensity float64
	for tree := range trees {
		treeSeed := nextRandomInMemory(seed ^ uint64(tree))
		leafCounts := make(map[uint64]int, len(history))
		for _, feature := range history {
			path := hstPathKey(feature, depth, treeSeed)
			leafCounts[path]++
		}
		targetPath := hstPathKey(target, depth, treeSeed)
		avgDensity += float64(leafCounts[targetPath])
	}
	avgDensity /= float64(trees)
	return clampScore(1.0 - avgDensity/float64(len(features)))
}

// hstPathKey encodes the HST traversal path as a uint64 bitmask (0=L, 1=R per bit).
// Supports up to 64 depth levels; depth is capped at 63 in practice.
func hstPathKey(point [6]float64, depth int, seed uint64) uint64 {
	minVals := [6]float64{-8, -8, -8, -8, -8, -8}
	maxVals := [6]float64{8, 8, 8, 8, 8, 8}
	var key uint64
	for i := range depth {
		seed = nextRandomInMemory(seed)
		dim := int(seed % 6)
		mid := (minVals[dim] + maxVals[dim]) / 2
		if point[dim] < mid {
			maxVals[dim] = mid
		} else {
			key |= 1 << i
			minVals[dim] = mid
		}
	}
	return key
}

func funcIsolationForest(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	trees := 100
	sampleSize := 256
	if len(args) > 1 {
		trees = int(args[1].(*parser.NumberLiteral).Val)
	}
	if len(args) > 2 {
		sampleSize = int(args[2].(*parser.NumberLiteral).Val)
	}
	if trees < 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("isolation Forest trees must be positive, got %d", trees))
	}
	if sampleSize < 2 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("isolation Forest sample_size must be at least 2, got %d", sampleSize))
	}
	return forEachAnomalySeries(matrixVal, enh, 32, func(series Series, samples []anomalySample) (float64, bool) {
		features := buildFeatures(samples)
		if len(features) < 32 {
			return 0, false
		}
		history := features[:len(features)-1]
		if len(history) > sampleSize {
			history = history[len(history)-sampleSize:]
		}
		seed := rcfSeedFromString(getMetricName(series.Metric))
		return isolationScoreInMemory(history, features[len(features)-1], trees, seed), true
	})
}

// isoNode is a node in a built isolation tree.
type isoNode struct {
	// leaf when left == nil
	left, right *isoNode
	dimension   int
	splitValue  float64
	size        int // number of training points that reached this node
}

// buildIsoTree builds a single isolation tree from points[indices] up to maxDepth.
func buildIsoTree(points [][6]float64, indices []int, depth, maxDepth int, seed *uint64) *isoNode {
	n := len(indices)
	node := &isoNode{size: n}
	if n <= 1 || depth >= maxDepth {
		return node
	}
	*seed = nextRandomInMemory(*seed)
	dim := int(*seed % 6)
	minV, maxV := points[indices[0]][dim], points[indices[0]][dim]
	for _, idx := range indices[1:] {
		v := points[idx][dim]
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if maxV-minV < 1e-12 {
		return node
	}
	*seed = nextRandomInMemory(*seed)
	split := minV + (maxV-minV)*float64(*seed>>11)/float64(uint64(1)<<53)
	node.dimension = dim
	node.splitValue = split
	left := make([]int, 0, n/2+1)
	right := make([]int, 0, n/2+1)
	for _, idx := range indices {
		if points[idx][dim] < split {
			left = append(left, idx)
		} else {
			right = append(right, idx)
		}
	}
	node.left = buildIsoTree(points, left, depth+1, maxDepth, seed)
	node.right = buildIsoTree(points, right, depth+1, maxDepth, seed)
	return node
}

// pathLength returns the path length for point in the tree, adding c(n) at leaves.
func pathLength(node *isoNode, point [6]float64, depth int) float64 {
	if node.left == nil {
		return float64(depth) + cInMemory(node.size)
	}
	if point[node.dimension] < node.splitValue {
		return pathLength(node.left, point, depth+1)
	}
	return pathLength(node.right, point, depth+1)
}

func isolationScoreInMemory(points [][6]float64, point [6]float64, trees int, seed uint64) float64 {
	n := len(points)
	if n < 2 {
		return 0
	}
	cN := cInMemory(n)
	if cN == 0 {
		return 0
	}
	maxDepth := int(math.Ceil(math.Log2(float64(n)))) + 1
	indices := make([]int, n)
	for i := range points {
		indices[i] = i
	}
	var totalPath float64
	for range trees {
		seed = nextRandomInMemory(seed)
		root := buildIsoTree(points, indices, 0, maxDepth, &seed)
		totalPath += pathLength(root, point, 0)
	}
	return clampScore(math.Pow(2, -totalPath/float64(trees)/cN))
}

func cInMemory(n int) float64 {
	if n <= 1 {
		return 0
	}
	if n == 2 {
		return 1
	}
	fn := float64(n)
	return 2*(math.Log(fn-1)+0.5772156649) - 2*(fn-1)/fn
}

// === changepoint(v range-vector) float ===
// Detects sudden baseline shifts using CUSUM. O(n).
func funcChangepoint(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return forEachAnomalySeries(matrixVal, enh, 4, func(_ Series, samples []anomalySample) (float64, bool) {
		var count int
		var mean, m2 float64
		for _, s := range samples[:len(samples)-1] {
			count++
			delta := s.V - mean
			mean += delta / float64(count)
			m2 += delta * (s.V - mean)
		}
		var std float64
		if count >= 2 {
			std = math.Sqrt(math.Max(m2/float64(count-1), 0))
		}
		if std < 1e-9 {
			return 0, true
		}
		var cusumPos, cusumNeg, maxCusum float64
		for _, s := range samples {
			z := (s.V - mean) / std
			cusumPos = math.Max(0, cusumPos+z-0.5)
			cusumNeg = math.Max(0, cusumNeg-z-0.5)
			if c := math.Max(cusumPos, cusumNeg); c > maxCusum {
				maxCusum = c
			}
		}
		return maxCusum / 5.0, true
	})
}

// === trend_score(v range-vector) float ===
// Detects abnormal slope via linear-regression residual. O(n).
func funcTrendScore(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return forEachAnomalySeries(matrixVal, enh, 3, func(_ Series, samples []anomalySample) (float64, bool) {
		n := float64(len(samples))
		t0 := samples[0].T
		var sumT, sumV, sumTT, sumTV float64
		for _, s := range samples {
			t := float64(s.T-t0) / 1000.0
			sumT += t
			sumV += s.V
			sumTT += t * t
			sumTV += t * s.V
		}
		denom := n*sumTT - sumT*sumT
		if math.Abs(denom) < 1e-12 {
			return 0, true
		}
		slope := (n*sumTV - sumT*sumV) / denom
		intercept := (sumV - slope*sumT) / n
		var resCount int
		var resMean, resM2 float64
		for _, s := range samples {
			t := float64(s.T-t0) / 1000.0
			res := s.V - (slope*t + intercept)
			resCount++
			d := res - resMean
			resMean += d / float64(resCount)
			resM2 += d * (res - resMean)
		}
		var resStd float64
		if resCount >= 2 {
			resStd = math.Sqrt(math.Max(resM2/float64(resCount-1), 0))
		}
		if resStd < 1e-9 {
			return 0, true
		}
		last := samples[len(samples)-1]
		t := float64(last.T-t0) / 1000.0
		return math.Abs(last.V-(slope*t+intercept)) / resStd / 3.0, true
	})
}

// === burst_score(v range-vector) float ===
// Detects sudden spikes using EWMA + variance. O(n).
func funcBurstScore(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(matrixVal) == 0 {
		return enh.Out, nil
	}
	alpha := 0.1
	if len(args) > 1 {
		alpha = args[1].(*parser.NumberLiteral).Val
	}
	if alpha <= 0 || alpha > 1 {
		return enh.Out, annotations.NewInvalidParamError(fmt.Errorf("burst_score alpha must be in (0,1], got %g", alpha))
	}
	return forEachAnomalySeries(matrixVal, enh, 2, func(_ Series, samples []anomalySample) (float64, bool) {
		baseline := samples[0].V
		variance := 0.0
		for _, s := range samples[1:] {
			delta := s.V - baseline
			baseline += alpha * delta
			variance = (1-alpha)*variance + alpha*delta*delta
		}
		std := math.Sqrt(math.Max(variance, 0))
		if std > 1e-9 {
			return math.Abs(samples[len(samples)-1].V-baseline) / (3.0 * std), true
		}
		return 0, true
	})
}

// === entropy(v range-vector) float ===
// Computes normalised Shannon entropy of the value distribution. O(n).
// Uses histogram binning (Sturges rule). Works on floats and histogram-avg values.
func funcEntropy(_ []Vector, matrixVal Matrix, _ parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return forEachAnomalySeries(matrixVal, enh, 2, func(_ Series, samples []anomalySample) (float64, bool) {
		n := len(samples)
		nBins := min(max(int(math.Ceil(math.Log2(float64(n))))+1, 2), 32)
		minV, maxV := samples[0].V, samples[0].V
		for _, s := range samples[1:] {
			if s.V < minV {
				minV = s.V
			}
			if s.V > maxV {
				maxV = s.V
			}
		}
		if maxV-minV < 1e-12 {
			return 0, true
		}
		binWidth := (maxV - minV) / float64(nBins)
		var counts [32]int
		for _, s := range samples {
			idx := int((s.V - minV) / binWidth)
			if idx >= nBins {
				idx = nBins - 1
			}
			counts[idx]++
		}
		var entropy float64
		for _, cnt := range counts {
			if cnt == 0 {
				continue
			}
			p := float64(cnt) / float64(n)
			entropy -= p * math.Log2(p)
		}
		return entropy / math.Log2(float64(nBins)), true
	})
}

// funcRCF implements rcf(v range-vector [, trees [, sample_size]]).
//
// It maintains a per-series streaming Random Cut Forest that persists across
// PromQL evaluations. New samples are inserted incrementally; old samples are
// evicted via bounded reservoir sampling. The anomaly score is the collusive
// displacement of the latest point, normalised to [0, 1].
func funcRCF(_ []Vector, matrixVal Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	numTrees, sampleSize, annos := rcfParseArgs(args)
	if annos != nil {
		return enh.Out, annos
	}
	for _, series := range matrixVal {
		samples, annos := extractAnomalySamples(series, getMetricName(series.Metric), posrange.PositionRange{})
		if annos != nil {
			return enh.Out, annos
		}
		if len(samples) < 2 {
			continue
		}
		f := enh.RCFStore.forest(series.Metric.Hash(), numTrees, sampleSize)
		f.mu.Lock()
		rcfIngest(f.forest, samples)
		score := f.forest.score(buildFeatures(samples)[len(samples)-1])
		f.mu.Unlock()
		enh.RCFStore.markDirty(series.Metric.Hash())
		enh.Out = append(enh.Out, Sample{
			Metric: series.Metric,
			F:      score,
			T:      samples[len(samples)-1].T,
		})
	}
	return enh.Out, nil
}

// evalRCFAttribution implements rcf_attribution(v range-vector [, trees [, sample_size]]).
//
// It is special-cased in the engine (like label_replace) because it produces
// rcfDims output series per input series — one per feature dimension — which
// the standard range-vector evalCall path cannot support (it only takes outVec[0]).
//
// Each output series carries rcf_dim=<name> and the fractional displacement
// contribution of that dimension to the anomaly score.
func (ev *evaluator) evalRCFAttribution(ctx context.Context, args parser.Expressions) (parser.Value, annotations.Annotations) {
	numTrees, sampleSize, annos := rcfParseArgs(args)
	if annos != nil {
		return Matrix{}, annos
	}

	sel := args[0].(*parser.MatrixSelector)

	// dimSeries accumulates FPoints per output label-set across all steps.
	type dimKey struct {
		hash uint64
		dim  int
	}
	type dimSeries struct {
		metric labels.Labels
		points []FPoint
	}
	seriesMap := make(map[dimKey]*dimSeries)

	dimNames := [rcfDims]string{"value", "delta", "velocity", "acceleration", "ewma_dev", "cv"}
	lb := labels.NewBuilder(labels.EmptyLabels())
	var ws annotations.Annotations

	// Iterate over every evaluation step, evaluating the matrix window at
	// each step via matrixSelector (which handles windowing correctly).
	for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
		// Temporarily set the evaluator timestamps to this single step so
		// matrixSelector fetches the correct [ts-range, ts] window.
		origStart, origEnd := ev.startTimestamp, ev.endTimestamp
		ev.startTimestamp, ev.endTimestamp = ts, ts
		matrix, stepWS := ev.matrixSelector(ctx, sel)
		ev.startTimestamp, ev.endTimestamp = origStart, origEnd
		ws.Merge(stepWS)

		for _, series := range matrix {
			samples, seriesAnnos := extractAnomalySamples(series, getMetricName(series.Metric), posrange.PositionRange{})
			ws.Merge(seriesAnnos)
			if len(samples) < 2 {
				continue
			}
			f := ev.rcfStore.forest(series.Metric.Hash(), numTrees, sampleSize)
			f.mu.Lock()
			rcfIngest(f.forest, samples)
			attr := f.forest.attribution(buildFeatures(samples)[len(samples)-1])
			f.mu.Unlock()
			ev.rcfStore.markDirty(series.Metric.Hash())

			base := series.Metric.DropReserved(func(n string) bool { return n == labels.MetricName })
			seriesHash := base.Hash()
			for d := range rcfDims {
				key := dimKey{seriesHash, d}
				ds, ok := seriesMap[key]
				if !ok {
					lb.Reset(base)
					lb.Set("rcf_dim", dimNames[d])
					ds = &dimSeries{metric: lb.Labels()}
					seriesMap[key] = ds
				}
				ds.points = append(ds.points, FPoint{T: ts, F: attr[d]})
			}
		}
	}

	out := make(Matrix, 0, len(seriesMap))
	for _, ds := range seriesMap {
		out = append(out, Series{Metric: ds.metric, Floats: ds.points})
	}
	return ev.mergeSeriesWithSameLabelset(out), ws
}

// rcfParseArgs extracts and validates the optional trees and sample_size args.
func rcfParseArgs(args parser.Expressions) (numTrees, sampleSize int, annos annotations.Annotations) {
	numTrees, sampleSize = rcfDefaultTrees, rcfDefaultSampleSize
	if len(args) > 1 {
		numTrees = int(args[1].(*parser.NumberLiteral).Val)
	}
	if len(args) > 2 {
		sampleSize = int(args[2].(*parser.NumberLiteral).Val)
	}
	if numTrees < 1 {
		return 0, 0, annotations.NewInvalidParamError(fmt.Errorf("rcf trees must be positive, got %d", numTrees))
	}
	if sampleSize < 2 {
		return 0, 0, annotations.NewInvalidParamError(fmt.Errorf("rcf sample_size must be at least 2, got %d", sampleSize))
	}
	return numTrees, sampleSize, nil
}

// rcfIngest inserts all samples newer than forest.LastTS into the forest.
func rcfIngest(f *rcfForest, samples []anomalySample) {
	features := buildFeatures(samples)
	for i, s := range samples {
		if s.T <= f.LastTS {
			continue
		}
		f.update(s.T, features[i])
	}
}
