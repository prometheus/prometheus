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

package histogram

import (
	"errors"
	"fmt"
	"math"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

// Suffixes identifying which classic series a caller wants out of
// ConvertNHCBToClassic. Passing "" emits all three (buckets, count and sum).
const (
	ClassicSuffixBucket = "_bucket"
	ClassicSuffixCount  = "_count"
	ClassicSuffixSum    = "_sum"
)

// ClassicSeriesCache holds precomputed label sets and scratch buffers across
// repeated ConvertNHCBToClassic and ConvertExponentialToClassic calls.
type ClassicSeriesCache struct {
	lset       labels.Labels
	baseName   string
	bucketName string
	countName  string
	sumName    string
	bounds     []float64 // Classic upper bounds the cached bucketLabels were built for.
	leStrings  []string

	bucketLabels []labels.Labels // len(bounds)+1; last entry is the +Inf bucket.
	haveBuckets  bool
	countLabels  labels.Labels
	haveCount    bool
	sumLabels    labels.Labels
	haveSum      bool
	cumulative   []float64
}

// prepare updates the cache for lset and baseName, preserving reusable
// bounds, formatted leStrings, metric suffix names, and slice capacities
// when switching between series of the same metric.
func (c *ClassicSeriesCache) prepare(lset labels.Labels, baseName string) {
	if !labels.Equal(c.lset, lset) {
		c.lset = lset
		c.haveBuckets = false
		c.haveCount = false
		c.haveSum = false
	}
	if c.baseName != baseName {
		c.baseName = baseName
		c.bucketName = baseName + ClassicSuffixBucket
		c.countName = baseName + ClassicSuffixCount
		c.sumName = baseName + ClassicSuffixSum
	}
}

// boundsMatch reports whether bounds equals the cached bounds.
func (c *ClassicSeriesCache) boundsMatch(bounds []float64) bool {
	if len(c.bounds) != len(bounds) || len(c.leStrings) != len(bounds) {
		return false
	}
	for i, v := range bounds {
		if c.bounds[i] != v {
			return false
		}
	}
	return true
}

// bucketsMatch reports whether the cached bucket label sets were built for
// the current series and these exact classic bounds.
func (c *ClassicSeriesCache) bucketsMatch(bounds []float64) bool {
	return c.haveBuckets && c.boundsMatch(bounds)
}

func (c *ClassicSeriesCache) allocCumulative(n int) []float64 {
	if cap(c.cumulative) < n {
		c.cumulative = make([]float64, n)
	} else {
		c.cumulative = c.cumulative[:n]
		clear(c.cumulative)
	}
	return c.cumulative
}

func allocCumulative(cache *ClassicSeriesCache, n int) []float64 {
	if cache != nil {
		return cache.allocCumulative(n)
	}
	return make([]float64, n)
}

// bucketLabelsFor returns the label set for bucket index idx (len(bounds)
// for the +Inf bucket). If cache is non-nil, the caller must have already
// populated cache.bucketLabels.
func bucketLabelsFor(cache *ClassicSeriesCache, idx int, lsetBuilder *labels.Builder, lset labels.Labels, baseName string, boundary float64) labels.Labels {
	if cache != nil {
		return cache.bucketLabels[idx]
	}
	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+ClassicSuffixBucket)
	lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(boundary))
	return lsetBuilder.Labels()
}

// ConvertNHCBToClassic converts Native Histogram Custom Buckets (NHCB) to classic histogram series.
// This conversion is needed in various scenarios where users need to get NHCB back to classic histogram format,
// such as Remote Write v1 for external system compatibility and migration use cases.
//
// onlySuffix restricts emission to one of ClassicSuffixBucket, ClassicSuffixCount
// or ClassicSuffixSum, skipping the work of building and emitting the other two
// series kinds. Pass "" to emit all three, as before.
//
// cache, if non-nil, is used to avoid rebuilding label sets that are
// identical to a previous call for the same series (see ClassicSeriesCache).
//
// When calling this function, caller must ensure that provided nhcb is valid NHCB histogram.
func ConvertNHCBToClassic(nhcb any, lset labels.Labels, lsetBuilder *labels.Builder, onlySuffix string, cache *ClassicSeriesCache, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	baseName := lset.Get(model.MetricNameLabel)
	if baseName == "" {
		return errors.New("metric name label '__name__' is missing")
	}

	wantBuckets := onlySuffix == "" || onlySuffix == ClassicSuffixBucket

	var (
		customValues []float64
		cumulative   []float64
		count, sum   float64
		idx          int // This index is to track buckets in Classic Histogram
		currIdx      int // This index is to track buckets in Native Histogram
	)

	switch h := nhcb.(type) {
	case *Histogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not a NHCB")
		}

		// Validate the histogram before conversion.
		// The caller must ensure that the provided histogram is valid NHCB.
		if err := h.Validate(); err != nil {
			return err
		}

		if wantBuckets {
			customValues = h.CustomValues
			cumulative = allocCumulative(cache, len(customValues)+1)

			// Histograms are in delta format so we first bring them to absolute format.
			acc := int64(0)
			for _, s := range h.PositiveSpans {
				// Skipped buckets are empty, so leave them at zero.
				idx += int(s.Offset)
				for i := 0; i < int(s.Length); i++ {
					acc += h.PositiveBuckets[currIdx]
					cumulative[idx] = float64(acc)
					idx++
					currIdx++
				}
			}
		}
		count = float64(h.Count)
		sum = h.Sum
	case *FloatHistogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not a NHCB")
		}

		// Validate the histogram before conversion.
		// The caller must ensure that the provided histogram is valid NHCB.
		if err := h.Validate(); err != nil {
			return err
		}

		if wantBuckets {
			customValues = h.CustomValues
			cumulative = allocCumulative(cache, len(customValues)+1)

			for _, span := range h.PositiveSpans {
				// Since Float Histogram is already in absolute format we should
				// keep the sparse buckets empty so we jump and go to next filled
				// bucket index.
				idx += int(span.Offset)
				for i := 0; i < int(span.Length); i++ {
					cumulative[idx] = h.PositiveBuckets[currIdx]
					idx++
					currIdx++
				}
			}
		}
		count = h.Count
		sum = h.Sum
	default:
		return fmt.Errorf("unsupported histogram type: %T", h)
	}

	if wantBuckets {
		// Turn per-bucket counts into cumulative counts. The +Inf bucket is the
		// sum of all buckets, matching the classic exposition for NHCB.
		for i := 1; i < len(cumulative); i++ {
			cumulative[i] += cumulative[i-1]
		}
	}
	return emitClassicSeries(lset, baseName, lsetBuilder, onlySuffix, cache, customValues, cumulative, count, sum, emitSeriesFn)
}

// ConvertExponentialToClassic converts a standard (exponential schema) native
// histogram to classic histogram series, evaluating the cumulative count at
// each of the given classic upper bounds. Bounds must be finite, sorted in
// ascending order and deduplicated; the +Inf bucket is always emitted in
// addition. Unlike ConvertNHCBToClassic, only float histograms are accepted:
// convert integer histograms with Histogram.ToFloat first.
//
// Exponential bucket boundaries rarely coincide with the requested bounds, so
// the observations of a bucket straddling a bound are interpolated the same
// way histogram_fraction does for native histograms: exponentially for regular
// buckets and linearly for the zero bucket (see
// FloatHistogram.InterpolationBounds). When a bound coincides with an
// exponential bucket boundary the result is exact.
//
// Use AppendExponentialBounds to derive bounds from the populated buckets of
// the histograms being converted.
//
// See ConvertNHCBToClassic for onlySuffix, cache and emitSeriesFn.
func ConvertExponentialToClassic(h *FloatHistogram, bounds []float64, lset labels.Labels, lsetBuilder *labels.Builder, onlySuffix string, cache *ClassicSeriesCache, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	baseName := lset.Get(model.MetricNameLabel)
	if baseName == "" {
		return errors.New("metric name label '__name__' is missing")
	}
	if !IsExponentialSchema(h.Schema) {
		return errors.New("unsupported histogram schema, not an exponential native histogram")
	}
	if err := h.Validate(); err != nil {
		return err
	}

	var cumulative []float64
	if onlySuffix == "" || onlySuffix == ClassicSuffixBucket {
		cumulative = allocCumulative(cache, len(bounds)+1)
		total := exponentialCumulativeCounts(h, bounds, cumulative)
		// The +Inf bucket is h.Count, which unlike the buckets includes NaN
		// observations. Validate does not require Count >= sum of buckets for
		// float histograms though, so take the bucket sum when it is larger
		// to keep the classic buckets monotonic.
		cumulative[len(bounds)] = max(h.Count, total)
	}
	return emitClassicSeries(lset, baseName, lsetBuilder, onlySuffix, cache, bounds, cumulative, h.Count, h.Sum, emitSeriesFn)
}

// exponentialCumulativeCounts fills cumulative[i] with the (estimated) number
// of observations in h that are less than or equal to bounds[i], and returns
// the total number of observations in all buckets. Bounds must be sorted in
// ascending order.
func exponentialCumulativeCounts(h *FloatHistogram, bounds, cumulative []float64) float64 {
	var (
		it     = h.AllBucketIterator()
		rank   float64 // Observations in all buckets fully below the current one.
		b      Bucket[float64]
		linear bool
		have   = it.Next()
	)
	if have {
		b, linear = h.InterpolationBounds(it.At())
	}
	for i, v := range bounds {
		for have && b.Upper <= v {
			rank += b.Count
			if have = it.Next(); have {
				b, linear = h.InterpolationBounds(it.At())
			}
		}
		c := rank
		// Here v < b.Upper, so v is strictly inside the bucket if it is
		// above its (possibly adjusted) lower bound.
		if have && b.Count > 0 && b.Lower < v {
			c += b.Count * b.FractionBelow(v, linear)
		}
		cumulative[i] = c
	}
	for have {
		rank += b.Count
		if have = it.Next(); have {
			b = it.At()
		}
	}
	return rank
}

// AppendExponentialBounds appends the lower and upper bound of every populated
// bucket of the exponential histogram h to dst, after reducing the resolution
// to at most maxSchema (which must be a valid exponential schema, i.e. within
// [ExponentialSchemaMin, ExponentialSchemaMax]), and returns the extended
// slice. Both bounds are needed so that classic quantile estimation
// interpolates within the populated bucket instead of all the way from the
// previous populated bucket. The result is neither sorted nor deduplicated and
// may contain +Inf for the overflow bucket. dst is returned unchanged if h does
// not use an exponential schema.
func AppendExponentialBounds(dst []float64, h *FloatHistogram, maxSchema int32) []float64 {
	if !IsExponentialSchema(h.Schema) {
		return dst
	}
	schema := min(h.Schema, maxSchema)
	for it := h.PositiveBucketIterator(); it.Next(); {
		b := it.At()
		if b.Count == 0 {
			continue
		}
		idx := b.Index
		if h.Schema > schema {
			idx = targetIdx(idx, h.Schema, schema)
		}
		dst = append(dst, getBoundExponential(idx-1, schema), getBoundExponential(idx, schema))
	}
	for it := h.NegativeBucketIterator(); it.Next(); {
		b := it.At()
		if b.Count == 0 {
			continue
		}
		idx := b.Index
		if h.Schema > schema {
			idx = targetIdx(idx, h.Schema, schema)
		}
		dst = append(dst, -getBoundExponential(idx, schema), -getBoundExponential(idx-1, schema))
	}
	if h.ZeroCount > 0 {
		dst = append(dst, h.ZeroThreshold)
		if len(h.NegativeBuckets) > 0 {
			dst = append(dst, -h.ZeroThreshold)
		}
	}
	return dst
}

// emitClassicSeries emits the classic bucket (using bounds and cumulative,
// where cumulative has one more element than bounds for the +Inf bucket),
// count and sum series selected by onlySuffix.
func emitClassicSeries(lset labels.Labels, baseName string, lsetBuilder *labels.Builder, onlySuffix string, cache *ClassicSeriesCache, bounds, cumulative []float64, count, sum float64, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	if cache != nil {
		cache.prepare(lset, baseName)
	}

	// We preserve original labels and restore them after conversion.
	// This is to ensure that no modifications are made to the original labels
	// that the queue_manager relies on.
	oldLabels := lsetBuilder.Labels()
	defer lsetBuilder.Reset(oldLabels)

	wantBuckets := onlySuffix == "" || onlySuffix == ClassicSuffixBucket
	wantCount := onlySuffix == "" || onlySuffix == ClassicSuffixCount
	wantSum := onlySuffix == "" || onlySuffix == ClassicSuffixSum

	if wantBuckets {
		if cache != nil && !cache.bucketsMatch(bounds) {
			if !cache.boundsMatch(bounds) {
				cache.bounds = append(cache.bounds[:0], bounds...)
				if cap(cache.leStrings) < len(bounds) {
					cache.leStrings = make([]string, len(bounds))
				} else {
					cache.leStrings = cache.leStrings[:len(bounds)]
				}
				for i, val := range bounds {
					cache.leStrings[i] = labels.FormatOpenMetricsFloat(val)
				}
			}
			nBuckets := len(bounds) + 1
			if cap(cache.bucketLabels) < nBuckets {
				cache.bucketLabels = make([]labels.Labels, nBuckets)
			} else {
				cache.bucketLabels = cache.bucketLabels[:nBuckets]
			}
			lsetBuilder.Reset(lset)
			lsetBuilder.Set(model.MetricNameLabel, cache.bucketName)
			for i, leStr := range cache.leStrings {
				lsetBuilder.Set(model.BucketLabel, leStr)
				cache.bucketLabels[i] = lsetBuilder.Labels()
			}
			lsetBuilder.Set(model.BucketLabel, "+Inf")
			cache.bucketLabels[len(bounds)] = lsetBuilder.Labels()
			cache.haveBuckets = true
		}

		for i, val := range bounds {
			bucketLabels := bucketLabelsFor(cache, i, lsetBuilder, lset, baseName, val)
			if err := emitSeriesFn(bucketLabels, cumulative[i]); err != nil {
				return err
			}
		}
		infLabels := bucketLabelsFor(cache, len(bounds), lsetBuilder, lset, baseName, math.Inf(1))
		if err := emitSeriesFn(infLabels, cumulative[len(bounds)]); err != nil {
			return err
		}
	}

	if wantCount {
		if cache != nil {
			if !cache.haveCount {
				lsetBuilder.Reset(lset)
				lsetBuilder.Set(model.MetricNameLabel, cache.countName)
				cache.countLabels = lsetBuilder.Labels()
				cache.haveCount = true
			}
			if err := emitSeriesFn(cache.countLabels, count); err != nil {
				return err
			}
		} else {
			lsetBuilder.Reset(lset)
			lsetBuilder.Set(model.MetricNameLabel, baseName+ClassicSuffixCount)
			if err := emitSeriesFn(lsetBuilder.Labels(), count); err != nil {
				return err
			}
		}
	}

	if wantSum {
		if cache != nil {
			if !cache.haveSum {
				lsetBuilder.Reset(lset)
				lsetBuilder.Set(model.MetricNameLabel, cache.sumName)
				cache.sumLabels = lsetBuilder.Labels()
				cache.haveSum = true
			}
			if err := emitSeriesFn(cache.sumLabels, sum); err != nil {
				return err
			}
		} else {
			lsetBuilder.Reset(lset)
			lsetBuilder.Set(model.MetricNameLabel, baseName+ClassicSuffixSum)
			if err := emitSeriesFn(lsetBuilder.Labels(), sum); err != nil {
				return err
			}
		}
	}

	return nil
}
