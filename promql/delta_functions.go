package promql

import (
	"errors"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/annotations"
)

func funcDirectDeltaRate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return deltaRate(vals, args, enh, true, "direct")
}
func funcDirectDeltaIncrease(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return deltaRate(vals, args, enh, false, "direct")
}

func funcExtrapolatedDeltaRate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return deltaRate(vals, args, enh, true, "extrapolated")
}
func funcExtrapolatedDeltaIncrease(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return deltaRate(vals, args, enh, false, "extrapolated")
}

func deltaRate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper, isRate bool, impl string) (Vector, annotations.Annotations) {
	ms := args[0].(*parser.MatrixSelector)

	switch impl {
	case "direct":
		res, _ := funcSumOverTime(vals, args, enh)
		if !isRate {
			return res, nil
		}
		for i, val := range res {
			if val.H == nil {
				res[i].F = val.F / ms.Range.Seconds()
			} else {
				res[i].H = val.H.Div(ms.Range.Seconds())
			}
		}
		// TODO: annotations
		return res, nil
	case "extrapolated":
		return extrapolatedDeltaRate(vals, args, enh, isRate)
	}

	panic("unknown method")
}
func extrapolatedDeltaRate(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper, isRate bool) (Vector, annotations.Annotations) {
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
		resultHistogram, newAnnos = histogramDeltaRate(samples.Histograms, metricName, args[0].PositionRange())
		annos.Merge(newAnnos)
		if resultHistogram == nil {
			// The histograms are not compatible with each other.
			return enh.Out, annos
		}
	case len(samples.Floats) > 1:
		numSamplesMinusOne = len(samples.Floats) - 1
		firstT = samples.Floats[0].T
		lastT = samples.Floats[numSamplesMinusOne].T
		resultFloat = 0
		for _, currPoint := range samples.Floats[1:] {
			resultFloat += currPoint.F
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

	// If samples are close enough to the (lower or upper) boundary of the
	// range, we extrapolate the rate all the way to the boundary in
	// question. "Close enough" is defined as "up to 10% more than the
	// average duration between samples within the range", see
	// extrapolationThreshold below. Essentially, we are assuming a more or
	// less regular spacing between samples, and if we don't see a sample
	// where we would expect one, we assume the series does not cover the
	// whole range, but starts and/or ends within the range.

	extrapolationThreshold := averageDurationBetweenSamples * 1.1
	extrapolateToInterval := sampledInterval

	// In the cumulative version the start/end in the range is extrapolated by half the average interval
	// this is from the cumulative extrapolatedRate() func
	// I don't think this is needed in the delta case - assumption is that delta systems will push all their data
	// vs the cumulative pull case where the scraper can miss the exact start and end values.

	if durationToStart < extrapolationThreshold {
		extrapolateToInterval += durationToStart
	}

	if durationToEnd < extrapolationThreshold {
		extrapolateToInterval += durationToEnd
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

func histogramDeltaRate(points []HPoint, metricName string, pos posrange.PositionRange) (*histogram.FloatHistogram, annotations.Annotations) {
	if len(points) < 2 {
		return nil, nil //TODO: return range too short
	}

	// TODO: gauge type errors - currently is marked as gauge histogram hint as not to cit spurious chunks

	// ignore first sample
	sum := points[1].H.Copy()
	for _, h := range points[2:] {
		_, err := sum.Add(h.H)
		if err != nil {
			if errors.Is(err, histogram.ErrHistogramsIncompatibleSchema) {
				return nil, annotations.New().Add(annotations.NewMixedExponentialCustomHistogramsWarning(metricName, pos))
			} else if errors.Is(err, histogram.ErrHistogramsIncompatibleBounds) {
				return nil, annotations.New().Add(annotations.NewIncompatibleCustomBucketsHistogramsWarning(metricName, pos))
			}
			//TODO: error at this point? histogramRate() just keeps going
		}
	}

	sum.CounterResetHint = histogram.GaugeType
	return sum.Compact(0), nil
}
