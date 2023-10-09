// DO NOT EDIT. COPIED AS-IS. SEE README.md

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite // import "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"

import (
	"fmt"
	"math"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const defaultZeroThreshold = 1e-128

func addSingleExponentialHistogramDataPoint(
	metric string,
	pt pmetric.ExponentialHistogramDataPoint,
	resource pcommon.Resource,
	settings Settings,
	series map[string]*prompb.TimeSeries,
) error {
	labels := createAttributes(
		resource,
		pt.Attributes(),
		settings.ExternalLabels,
		model.MetricNameLabel, metric,
	)

	sig := timeSeriesSignature(
		pmetric.MetricTypeExponentialHistogram.String(),
		&labels,
	)
	ts, ok := series[sig]
	if !ok {
		ts = &prompb.TimeSeries{
			Labels: labels,
		}
		series[sig] = ts
	}

	histogram, err := exponentialToNativeHistogram(pt)
	if err != nil {
		return err
	}
	ts.Histograms = append(ts.Histograms, histogram)

	exemplars := getPromExemplars[pmetric.ExponentialHistogramDataPoint](pt)
	ts.Exemplars = append(ts.Exemplars, exemplars...)

	return nil
}

// exponentialToNativeHistogram  translates OTel Exponential Histogram data point
// to Prometheus Native Histogram.
func exponentialToNativeHistogram(p pmetric.ExponentialHistogramDataPoint) (prompb.Histogram, error) {
	scale := p.Scale()
	if scale < -4 || scale > 8 {
		return prompb.Histogram{},
			fmt.Errorf("cannot convert exponential to native histogram."+
				" Scale must be <= 8 and >= -4, was %d", scale)
		// TODO: downscale to 8 if scale > 8
	}

	pSpans, pDeltas := convertBucketsLayout(p.Positive())
	nSpans, nDeltas := convertBucketsLayout(p.Negative())

	h := prompb.Histogram{
		Schema: scale,

		ZeroCount: &prompb.Histogram_ZeroCountInt{ZeroCountInt: p.ZeroCount()},
		// TODO use zero_threshold, if set, see
		// https://github.com/open-telemetry/opentelemetry-proto/pull/441
		ZeroThreshold: defaultZeroThreshold,

		PositiveSpans:  pSpans,
		PositiveDeltas: pDeltas,
		NegativeSpans:  nSpans,
		NegativeDeltas: nDeltas,

		Timestamp: convertTimeStamp(p.Timestamp()),
	}

	if p.Flags().NoRecordedValue() {
		h.Sum = math.Float64frombits(value.StaleNaN)
		h.Count = &prompb.Histogram_CountInt{CountInt: value.StaleNaN}
	} else {
		if p.HasSum() {
			h.Sum = p.Sum()
		}
		h.Count = &prompb.Histogram_CountInt{CountInt: p.Count()}
	}
	return h, nil
}

// convertBucketsLayout translates OTel Exponential Histogram dense buckets
// representation to Prometheus Native Histogram sparse bucket representation.
//
// The translation logic is taken from the client_golang `histogram.go#makeBuckets`
// function, see `makeBuckets` https://github.com/prometheus/client_golang/blob/main/prometheus/histogram.go
// The bucket indexes conversion was adjusted, since OTel exp. histogram bucket
// index 0 corresponds to the range (1, base] while Prometheus bucket index 0
// to the range (base 1].
func convertBucketsLayout(buckets pmetric.ExponentialHistogramDataPointBuckets) ([]prompb.BucketSpan, []int64) {
	bucketCounts := buckets.BucketCounts()
	if bucketCounts.Len() == 0 {
		return nil, nil
	}

	var (
		spans         []prompb.BucketSpan
		deltas        []int64
		prevCount     int64
		nextBucketIdx int32
	)

	appendDelta := func(count int64) {
		spans[len(spans)-1].Length++
		deltas = append(deltas, count-prevCount)
		prevCount = count
	}

	for i := 0; i < bucketCounts.Len(); i++ {
		count := int64(bucketCounts.At(i))
		if count == 0 {
			continue
		}

		// The offset is adjusted by 1 as described above.
		bucketIdx := int32(i) + buckets.Offset() + 1
		delta := bucketIdx - nextBucketIdx
		if i == 0 || delta > 2 {
			// We have to create a new span, either because we are
			// at the very beginning, or because we have found a gap
			// of more than two buckets. The constant 2 is copied from the logic in
			// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
			spans = append(spans, prompb.BucketSpan{
				Offset: delta,
				Length: 0,
			})
		} else {
			// We have found a small gap (or no gap at all).
			// Insert empty buckets as needed.
			for j := int32(0); j < delta; j++ {
				appendDelta(0)
			}
		}
		appendDelta(count)
		nextBucketIdx = bucketIdx + 1
	}

	return spans, deltas
}
