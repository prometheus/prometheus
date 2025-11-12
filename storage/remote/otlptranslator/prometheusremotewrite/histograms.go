// Copyright 2024 The Prometheus Authors
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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/histograms.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"context"
	"fmt"
	"math"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/util/annotations"
)

const defaultZeroThreshold = 1e-128

// addExponentialHistogramDataPoints adds OTel exponential histogram data points to the corresponding time series
// as native histogram samples.
func (c *PrometheusConverter) addExponentialHistogramDataPoints(ctx context.Context, dataPoints pmetric.ExponentialHistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, temporality pmetric.AggregationTemporality,
	scope scope, meta Metadata,
) (annotations.Annotations, error) {
	var annots annotations.Annotations
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return annots, err
		}

		pt := dataPoints.At(x)

		hp, ws, err := exponentialToNativeHistogram(pt, temporality)
		annots.Merge(ws)
		if err != nil {
			return annots, err
		}

		lbls, err := c.createAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
			meta,
			model.MetricNameLabel,
			meta.MetricFamilyName,
		)
		if err != nil {
			return annots, err
		}
		ts := convertTimeStamp(pt.Timestamp())
		st := convertTimeStamp(pt.StartTimestamp())
		exemplars, err := c.getPromExemplars(ctx, pt.Exemplars())
		if err != nil {
			return annots, err
		}
		// OTel exponential histograms are always Int Histograms.
		if err = c.appender.AppendHistogram(lbls, meta, st, ts, hp, exemplars); err != nil {
			return annots, err
		}
	}

	return annots, nil
}

// exponentialToNativeHistogram translates an OTel Exponential Histogram data point
// to a Prometheus Native Histogram.
func exponentialToNativeHistogram(p pmetric.ExponentialHistogramDataPoint, temporality pmetric.AggregationTemporality) (*histogram.Histogram, annotations.Annotations, error) {
	var annots annotations.Annotations
	scale := p.Scale()
	if scale < histogram.ExponentialSchemaMin {
		return nil, annots,
			fmt.Errorf("cannot convert exponential to native histogram."+
				" Scale must be >= %d, was %d", histogram.ExponentialSchemaMin, scale)
	}

	var scaleDown int32
	if scale > histogram.ExponentialSchemaMax {
		scaleDown = scale - histogram.ExponentialSchemaMax
		scale = histogram.ExponentialSchemaMax
	}

	pSpans, pDeltas := convertBucketsLayout(p.Positive().BucketCounts().AsRaw(), p.Positive().Offset(), scaleDown, true)
	nSpans, nDeltas := convertBucketsLayout(p.Negative().BucketCounts().AsRaw(), p.Negative().Offset(), scaleDown, true)

	// The counter reset detection must be compatible with Prometheus to
	// safely set ResetHint to NO. This is not ensured currently.
	// Sending a sample that triggers counter reset but with ResetHint==NO
	// would lead to Prometheus panic as it does not double check the hint.
	// Thus we're explicitly saying UNKNOWN here, which is always safe.
	// TODO: using created time stamp should be accurate, but we
	// need to know here if it was used for the detection.
	// Ref: https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/28663#issuecomment-1810577303
	// Counter reset detection in Prometheus: https://github.com/prometheus/prometheus/blob/f997c72f294c0f18ca13fa06d51889af04135195/tsdb/chunkenc/histogram.go#L232
	resetHint := histogram.UnknownCounterReset

	if temporality == pmetric.AggregationTemporalityDelta {
		// If the histogram has delta temporality, set the reset hint to gauge to avoid unnecessary chunk cutting.
		// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/).
		// This might be changed to a different hint name as gauge type might be misleading for samples that should be
		// summed over time.
		resetHint = histogram.GaugeType
	}
	h := &histogram.Histogram{
		CounterResetHint: resetHint,
		Schema:           scale,
		// TODO use zero_threshold, if set, see
		// https://github.com/open-telemetry/opentelemetry-proto/pull/441
		ZeroThreshold:   defaultZeroThreshold,
		ZeroCount:       p.ZeroCount(),
		PositiveSpans:   pSpans,
		PositiveBuckets: pDeltas,
		NegativeSpans:   nSpans,
		NegativeBuckets: nDeltas,
	}

	if p.Flags().NoRecordedValue() {
		h.Sum = math.Float64frombits(value.StaleNaN)
		h.Count = value.StaleNaN
	} else {
		if p.HasSum() {
			h.Sum = p.Sum()
		}
		h.Count = p.Count()
		if p.Count() == 0 && h.Sum != 0 {
			annots.Add(fmt.Errorf("exponential histogram data point has zero count, but non-zero sum: %f", h.Sum))
		}
	}
	return h, annots, nil
}

// convertBucketsLayout translates OTel Explicit or Exponential Histogram dense buckets
// representation to Prometheus Native Histogram sparse bucket representation. This is used
// for translating Exponential Histograms into Native Histograms, and Explicit Histograms
// into Native Histograms with Custom Buckets.
//
// The translation logic is taken from the client_golang `histogram.go#makeBuckets`
// function, see `makeBuckets` https://github.com/prometheus/client_golang/blob/main/prometheus/histogram.go
//
// scaleDown is the factor by which the buckets are scaled down. In other words 2^scaleDown buckets will be merged into one.
//
// When converting from OTel Exponential Histograms to Native Histograms, the
// bucket indexes conversion is adjusted, since OTel exp. histogram bucket
// index 0 corresponds to the range (1, base] while Prometheus bucket index 0
// to the range (base 1].
//
// When converting from OTel Explicit Histograms to Native Histograms with Custom Buckets,
// the bucket indexes are not scaled, and the indices are not adjusted by 1.
func convertBucketsLayout(bucketCounts []uint64, offset, scaleDown int32, adjustOffset bool) ([]histogram.Span, []int64) {
	if len(bucketCounts) == 0 {
		return nil, nil
	}

	var (
		spans     []histogram.Span
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
	numBuckets := len(bucketCounts)

	bucketIdx := offset>>scaleDown + 1

	initialOffset := offset
	if adjustOffset {
		initialOffset = initialOffset>>scaleDown + 1
	}

	spans = append(spans, histogram.Span{
		Offset: initialOffset,
		Length: 0,
	})

	for i := range numBuckets {
		nextBucketIdx := (int32(i)+offset)>>scaleDown + 1
		if bucketIdx == nextBucketIdx { // We have not collected enough buckets to merge yet.
			count += int64(bucketCounts[i])
			continue
		}
		if count == 0 {
			count = int64(bucketCounts[i])
			continue
		}

		gap := nextBucketIdx - bucketIdx - 1
		if gap > 2 {
			// We have to create a new span, because we have found a gap
			// of more than two buckets. The constant 2 is copied from the logic in
			// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
			spans = append(spans, histogram.Span{
				Offset: gap,
				Length: 0,
			})
		} else {
			// We have found a small gap (or no gap at all).
			// Insert empty buckets as needed.
			for range gap {
				appendDelta(0)
			}
		}
		appendDelta(count)
		count = int64(bucketCounts[i])
		bucketIdx = nextBucketIdx
	}

	// Need to use the last item's index. The offset is scaled and adjusted by 1 as described above.
	gap := (int32(numBuckets)+offset-1)>>scaleDown + 1 - bucketIdx
	if gap > 2 {
		// We have to create a new span, because we have found a gap
		// of more than two buckets. The constant 2 is copied from the logic in
		// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
		spans = append(spans, histogram.Span{
			Offset: gap,
			Length: 0,
		})
	} else {
		// We have found a small gap (or no gap at all).
		// Insert empty buckets as needed.
		for range gap {
			appendDelta(0)
		}
	}
	appendDelta(count)

	return spans, deltas
}

func (c *PrometheusConverter) addCustomBucketsHistogramDataPoints(ctx context.Context, dataPoints pmetric.HistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, temporality pmetric.AggregationTemporality,
	scope scope, meta Metadata,
) (annotations.Annotations, error) {
	var annots annotations.Annotations

	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return annots, err
		}

		pt := dataPoints.At(x)

		hp, ws, err := explicitHistogramToCustomBucketsHistogram(pt, temporality)
		annots.Merge(ws)
		if err != nil {
			return annots, err
		}

		lbls, err := c.createAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
			meta,
			model.MetricNameLabel,
			meta.MetricFamilyName,
		)
		if err != nil {
			return annots, err
		}
		ts := convertTimeStamp(pt.Timestamp())
		st := convertTimeStamp(pt.StartTimestamp())
		exemplars, err := c.getPromExemplars(ctx, pt.Exemplars())
		if err != nil {
			return annots, err
		}
		if err = c.appender.AppendHistogram(lbls, meta, st, ts, hp, exemplars); err != nil {
			return annots, err
		}
	}

	return annots, nil
}

func explicitHistogramToCustomBucketsHistogram(p pmetric.HistogramDataPoint, temporality pmetric.AggregationTemporality) (*histogram.Histogram, annotations.Annotations, error) {
	var annots annotations.Annotations

	buckets := p.BucketCounts().AsRaw()
	offset := getBucketOffset(buckets)
	bucketCounts := buckets[offset:]
	positiveSpans, positiveDeltas := convertBucketsLayout(bucketCounts, int32(offset), 0, false)

	// The counter reset detection must be compatible with Prometheus to
	// safely set ResetHint to NO. This is not ensured currently.
	// Sending a sample that triggers counter reset but with ResetHint==NO
	// would lead to Prometheus panic as it does not double check the hint.
	// Thus we're explicitly saying UNKNOWN here, which is always safe.
	// TODO: using created time stamp should be accurate, but we
	// need to know here if it was used for the detection.
	// Ref: https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/28663#issuecomment-1810577303
	// Counter reset detection in Prometheus: https://github.com/prometheus/prometheus/blob/f997c72f294c0f18ca13fa06d51889af04135195/tsdb/chunkenc/histogram.go#L232
	resetHint := histogram.UnknownCounterReset

	if temporality == pmetric.AggregationTemporalityDelta {
		// If the histogram has delta temporality, set the reset hint to gauge to avoid unnecessary chunk cutting.
		// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/).
		// This might be changed to a different hint name as gauge type might be misleading for samples that should be
		// summed over time.
		resetHint = histogram.GaugeType
	}

	// TODO(carrieedwards): Add setting to limit maximum bucket count
	h := &histogram.Histogram{
		CounterResetHint: resetHint,
		Schema:           histogram.CustomBucketsSchema,
		PositiveSpans:    positiveSpans,
		PositiveBuckets:  positiveDeltas,
		// Note: OTel explicit histograms have an implicit +Inf bucket, which has a lower bound
		// of the last element in the explicit_bounds array.
		// This is similar to the custom_values array in native histograms with custom buckets.
		// Because of this shared property, the OTel explicit histogram's explicit_bounds array
		// can be mapped directly to the custom_values array.
		// See: https://github.com/open-telemetry/opentelemetry-proto/blob/d7770822d70c7bd47a6891fc9faacc66fc4af3d3/opentelemetry/proto/metrics/v1/metrics.proto#L469
		CustomValues: p.ExplicitBounds().AsRaw(),
	}

	if p.Flags().NoRecordedValue() {
		h.Sum = math.Float64frombits(value.StaleNaN)
		h.Count = value.StaleNaN
	} else {
		if p.HasSum() {
			h.Sum = p.Sum()
		}
		h.Count = p.Count()
		if p.Count() == 0 && h.Sum != 0 {
			annots.Add(fmt.Errorf("histogram data point has zero count, but non-zero sum: %f", h.Sum))
		}
	}
	return h, annots, nil
}

func getBucketOffset(buckets []uint64) (offset int) {
	for offset < len(buckets) && buckets[offset] == 0 {
		offset++
	}
	return offset
}
