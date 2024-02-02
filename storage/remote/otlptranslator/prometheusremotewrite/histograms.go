// DO NOT EDIT. COPIED AS-IS. SEE ../README.md

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
	if scale < -4 {
		return prompb.Histogram{},
			fmt.Errorf("cannot convert exponential to native histogram."+
				" Scale must be >= -4, was %d", scale)
	}

	var scaleDown int32
	if scale > 8 {
		scaleDown = scale - 8
		scale = 8
	}

	pSpans, pDeltas := convertBucketsLayout(p.Positive(), scaleDown)
	nSpans, nDeltas := convertBucketsLayout(p.Negative(), scaleDown)

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
//
// scaleDown is the factor by which the buckets are scaled down. In other words 2^scaleDown buckets will be merged into one.
func convertBucketsLayout(buckets pmetric.ExponentialHistogramDataPointBuckets, scaleDown int32) ([]prompb.BucketSpan, []int64) {
	bucketCounts := buckets.BucketCounts()
	if bucketCounts.Len() == 0 {
		return nil, nil
	}

	var (
		spans     []prompb.BucketSpan
		deltas    []int64
		count     int64
		prevCount int64
	)

	appendDelta := func(count int64) {
		spans[len(spans)-1].Length++
		deltas = append(deltas, count-prevCount)
		prevCount = count
	}

	// Let the compiler figure out that this is const during this function by
	// moving it into a local variable.
	numBuckets := bucketCounts.Len()

	// The offset is scaled and adjusted by 1 as described above.
	bucketIdx := buckets.Offset()>>scaleDown + 1
	spans = append(spans, prompb.BucketSpan{
		Offset: bucketIdx,
		Length: 0,
	})

	for i := 0; i < numBuckets; i++ {
		// The offset is scaled and adjusted by 1 as described above.
		nextBucketIdx := (int32(i)+buckets.Offset())>>scaleDown + 1
		if bucketIdx == nextBucketIdx { // We have not collected enough buckets to merge yet.
			count += int64(bucketCounts.At(i))
			continue
		}
		if count == 0 {
			count = int64(bucketCounts.At(i))
			continue
		}

		gap := nextBucketIdx - bucketIdx - 1
		if gap > 2 {
			// We have to create a new span, because we have found a gap
			// of more than two buckets. The constant 2 is copied from the logic in
			// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
			spans = append(spans, prompb.BucketSpan{
				Offset: gap,
				Length: 0,
			})
		} else {
			// We have found a small gap (or no gap at all).
			// Insert empty buckets as needed.
			for j := int32(0); j < gap; j++ {
				appendDelta(0)
			}
		}
		appendDelta(count)
		count = int64(bucketCounts.At(i))
		bucketIdx = nextBucketIdx
	}
	// Need to use the last item's index. The offset is scaled and adjusted by 1 as described above.
	gap := (int32(numBuckets)+buckets.Offset()-1)>>scaleDown + 1 - bucketIdx
	if gap > 2 {
		// We have to create a new span, because we have found a gap
		// of more than two buckets. The constant 2 is copied from the logic in
		// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
		spans = append(spans, prompb.BucketSpan{
			Offset: gap,
			Length: 0,
		})
	} else {
		// We have found a small gap (or no gap at all).
		// Insert empty buckets as needed.
		for j := int32(0); j < gap; j++ {
			appendDelta(0)
		}
	}
	appendDelta(count)

	return spans, deltas
}
