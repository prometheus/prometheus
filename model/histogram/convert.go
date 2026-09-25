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

// ClassicSeriesCache holds precomputed label sets for one native histogram
// series across repeated ConvertNHCBToClassic or ConvertExponentialToClassic
// calls.
type ClassicSeriesCache struct {
	baseName     string
	customValues []float64

	bucketLabels []labels.Labels // len(customValues)+1; last entry is the +Inf bucket.
	haveBuckets  bool
	countLabels  labels.Labels
	haveCount    bool
	sumLabels    labels.Labels
	haveSum      bool

	// exponentialBucketLabels holds the _bucket label sets emitted by
	// ConvertExponentialToClassic, keyed by bucket boundary, as the set of
	// boundaries of an exponential histogram may change from sample to sample.
	exponentialBucketLabels map[float64]labels.Labels
}

// invalidateIfNameChanged drops every cached label set once the cache is
// reused for a differently-named series.
func (c *ClassicSeriesCache) invalidateIfNameChanged(baseName string) {
	if c.baseName != baseName {
		*c = ClassicSeriesCache{baseName: baseName}
	}
}

// bucketsMatch reports whether the cached bucket label sets were built for
// these exact custom bucket.
func (c *ClassicSeriesCache) bucketsMatch(customValues []float64) bool {
	if !c.haveBuckets || len(c.customValues) != len(customValues) {
		return false
	}
	for i, v := range customValues {
		if c.customValues[i] != v {
			return false
		}
	}
	return true
}

// bucketLabelsFor returns the label set for bucket index idx (len(customValues)
// for the +Inf bucket). If cache is non-nil, the caller must have already
// populated cache.bucketLabels.
func bucketLabelsFor(cache *ClassicSeriesCache, idx int, lsetBuilder *labels.Builder, lset labels.Labels, baseName string, boundary float64) labels.Labels {
	if cache != nil {
		return cache.bucketLabels[idx]
	}
	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+"_bucket")
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
	if cache != nil {
		cache.invalidateIfNameChanged(baseName)
	}

	// We preserve original labels and restore them after conversion.
	// This is to ensure that no modifications are made to the original labels
	// that the queue_manager relies on.
	oldLabels := lsetBuilder.Labels()
	defer lsetBuilder.Reset(oldLabels)

	wantBuckets := onlySuffix == "" || onlySuffix == ClassicSuffixBucket

	var (
		customValues    []float64
		positiveBuckets []float64
		count, sum      float64
		idx             int // This index is to track buckets in Classic Histogram
		currIdx         int // This index is to track buckets in Native Histogram
	)

	switch h := nhcb.(type) {
	case *Histogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not a NHCB")
		}

		// Validate the histogram before conversion.
		// The caller must ensure that the provided histogram is valid NHCB.
		if h.Validate() != nil {
			return errors.New(h.Validate().Error())
		}

		if wantBuckets {
			customValues = h.CustomValues
			positiveBuckets = make([]float64, len(customValues)+1)

			// Histograms are in delta format so we first bring them to absolute format.
			acc := int64(0)
			for _, s := range h.PositiveSpans {
				// Skipped buckets are empty, so leave them at zero.
				idx += int(s.Offset)
				for i := 0; i < int(s.Length); i++ {
					acc += h.PositiveBuckets[currIdx]
					positiveBuckets[idx] = float64(acc)
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
		if h.Validate() != nil {
			return errors.New(h.Validate().Error())
		}

		if wantBuckets {
			customValues = h.CustomValues
			positiveBuckets = make([]float64, len(customValues)+1)

			for _, span := range h.PositiveSpans {
				// Since Float Histogram is already in absolute format we should
				// keep the sparse buckets empty so we jump and go to next filled
				// bucket index.
				idx += int(span.Offset)
				for i := 0; i < int(span.Length); i++ {
					positiveBuckets[idx] = h.PositiveBuckets[currIdx]
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
		if cache != nil && !cache.bucketsMatch(customValues) {
			cache.customValues = append(cache.customValues[:0], customValues...)
			cache.bucketLabels = make([]labels.Labels, len(customValues)+1)
			for i, val := range customValues {
				lsetBuilder.Reset(lset)
				lsetBuilder.Set(model.MetricNameLabel, baseName+"_bucket")
				lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(val))
				cache.bucketLabels[i] = lsetBuilder.Labels()
			}
			lsetBuilder.Reset(lset)
			lsetBuilder.Set(model.MetricNameLabel, baseName+"_bucket")
			lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(math.Inf(1)))
			cache.bucketLabels[len(customValues)] = lsetBuilder.Labels()
			cache.haveBuckets = true
		}

		currCount := float64(0)
		for i, val := range customValues {
			currCount += positiveBuckets[i]
			bucketLabels := bucketLabelsFor(cache, i, lsetBuilder, lset, baseName, val)
			if err := emitSeriesFn(bucketLabels, currCount); err != nil {
				return err
			}
		}

		currCount += positiveBuckets[len(positiveBuckets)-1]

		infLabels := bucketLabelsFor(cache, len(customValues), lsetBuilder, lset, baseName, math.Inf(1))
		if err := emitSeriesFn(infLabels, currCount); err != nil {
			return err
		}
	}

	return emitCountAndSum(count, sum, lset, lsetBuilder, baseName, onlySuffix, cache, emitSeriesFn)
}

// emitCountAndSum emits the _count and the _sum series of a histogram
// converted to classic histogram series, skipping the one not matching
// onlySuffix (if set). See ConvertNHCBToClassic for the other arguments.
func emitCountAndSum(count, sum float64, lset labels.Labels, lsetBuilder *labels.Builder, baseName, onlySuffix string, cache *ClassicSeriesCache, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	if onlySuffix == "" || onlySuffix == ClassicSuffixCount {
		if cache != nil {
			if !cache.haveCount {
				lsetBuilder.Reset(lset)
				lsetBuilder.Set(model.MetricNameLabel, baseName+"_count")
				cache.countLabels = lsetBuilder.Labels()
				cache.haveCount = true
			}
			if err := emitSeriesFn(cache.countLabels, count); err != nil {
				return err
			}
		} else {
			lsetBuilder.Reset(lset)
			lsetBuilder.Set(model.MetricNameLabel, baseName+"_count")
			if err := emitSeriesFn(lsetBuilder.Labels(), count); err != nil {
				return err
			}
		}
	}

	if onlySuffix == "" || onlySuffix == ClassicSuffixSum {
		if cache != nil {
			if !cache.haveSum {
				lsetBuilder.Reset(lset)
				lsetBuilder.Set(model.MetricNameLabel, baseName+"_sum")
				cache.sumLabels = lsetBuilder.Labels()
				cache.haveSum = true
			}
			if err := emitSeriesFn(cache.sumLabels, sum); err != nil {
				return err
			}
		} else {
			lsetBuilder.Reset(lset)
			lsetBuilder.Set(model.MetricNameLabel, baseName+"_sum")
			if err := emitSeriesFn(lsetBuilder.Labels(), sum); err != nil {
				return err
			}
		}
	}

	return nil
}

// ConvertExponentialToClassic converts a native histogram with an exponential
// schema to classic histogram series. It is the counterpart of
// ConvertNHCBToClassic, with the same order of emitted series: the buckets in
// ascending order, then the count and the sum.
//
// Unlike NHCB, an exponential histogram has no fixed set of bucket boundaries,
// so the caller picks them: a _bucket series is emitted for every boundary in
// boundaries, which must be finite and strictly ascending, followed by the +Inf
// bucket. The value of a _bucket series is the number of observations in the
// buckets of nh, including the zero bucket, whose upper boundary is less than
// or equal to the boundary. That is exact for the bucket boundaries of nh, and
// hence for the bucket boundaries of nh reduced to a lower schema, too.
//
// AppendClassicBoundaries returns the boundaries representing a single
// histogram best. Classic histograms converted with different boundaries cannot
// be aggregated by the le label, though, e.g. with sum by (le) or over time. To
// be able to do so, convert all of them with the same boundaries, e.g. the
// union of the boundaries of all of them reduced to the lowest schema amongst
// them.
//
// When calling this function, caller must ensure that the provided histogram is
// a valid exponential native histogram.
func ConvertExponentialToClassic(nh any, boundaries []float64, lset labels.Labels, lsetBuilder *labels.Builder, onlySuffix string, cache *ClassicSeriesCache, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	baseName := lset.Get(model.MetricNameLabel)
	if baseName == "" {
		return errors.New("metric name label '__name__' is missing")
	}

	var fh *FloatHistogram
	switch h := nh.(type) {
	case *Histogram:
		if !IsExponentialSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not an exponential native histogram")
		}
		if err := h.Validate(); err != nil {
			return err
		}
		fh = h.ToFloat(nil)
	case *FloatHistogram:
		if !IsExponentialSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not an exponential native histogram")
		}
		if err := h.Validate(); err != nil {
			return err
		}
		fh = h
	default:
		return fmt.Errorf("unsupported histogram type: %T", h)
	}

	wantBuckets := onlySuffix == "" || onlySuffix == ClassicSuffixBucket
	if wantBuckets {
		for i, le := range boundaries {
			if math.IsNaN(le) || math.IsInf(le, 0) {
				return fmt.Errorf("classic bucket boundaries must be finite, got %g", le)
			}
			if i > 0 && le <= boundaries[i-1] {
				return fmt.Errorf("classic bucket boundaries must be strictly ascending, got %g after %g", le, boundaries[i-1])
			}
		}
	}

	if cache != nil {
		cache.invalidateIfNameChanged(baseName)
	}
	// Preserve the original labels of the builder, see ConvertNHCBToClassic.
	oldLabels := lsetBuilder.Labels()
	defer lsetBuilder.Reset(oldLabels)

	if wantBuckets {
		var (
			cumulativeCount float64
			it              = fh.AllBucketIterator()
			more            = it.Next()
		)
		for _, le := range boundaries {
			// The buckets are iterated in ascending order.
			for ; more && it.At().Upper <= le; more = it.Next() {
				cumulativeCount += it.At().Count
			}
			if err := emitSeriesFn(exponentialBucketLabels(cache, lsetBuilder, lset, baseName, le), cumulativeCount); err != nil {
				return err
			}
		}
		for ; more; more = it.Next() {
			cumulativeCount += it.At().Count
		}
		if err := emitSeriesFn(exponentialBucketLabels(cache, lsetBuilder, lset, baseName, math.Inf(1)), cumulativeCount); err != nil {
			return err
		}
	}

	return emitCountAndSum(fh.Count, fh.Sum, lset, lsetBuilder, baseName, onlySuffix, cache, emitSeriesFn)
}

// AppendClassicBoundaries appends the bucket boundaries of the classic
// histogram representing the exponential native histogram fh best to dst, and
// returns the extended slice. The appended boundaries are strictly ascending
// and do not include +Inf, see ConvertExponentialToClassic.
//
// Those are the upper boundaries of all buckets of fh, plus the lower boundary
// of every bucket that does not directly follow the previous one, i.e. of the
// lowest bucket and of the first bucket after a gap. The latter keep the linear
// interpolation of a classic histogram_quantile() within the buckets the
// observations are in, rather than spreading it across a gap or, for the lowest
// bucket, down to zero. The lower boundary of a zero bucket that is the lowest
// bucket is not included, as histogram_quantile() already assumes zero as the
// lower boundary in that case.
func AppendClassicBoundaries(dst []float64, fh *FloatHistogram) []float64 {
	var (
		prevUpper float64
		lowest    = true
	)
	for it := fh.AllBucketIterator(); it.Next(); {
		b := it.At()
		if lowerBoundaryNeeded(b, lowest, prevUpper) {
			dst = append(dst, b.Lower)
		}
		lowest = false
		if math.IsInf(b.Upper, +1) {
			// Only the bucket for +Inf observations has an infinite upper
			// boundary, it is represented by the +Inf bucket.
			break
		}
		dst = append(dst, b.Upper)
		prevUpper = b.Upper
	}
	return dst
}

// lowerBoundaryNeeded reports whether AppendClassicBoundaries includes the
// lower boundary of bucket b, given whether b is the lowest bucket, and the
// upper boundary of the previous bucket if it is not.
func lowerBoundaryNeeded(b Bucket[float64], lowest bool, prevUpper float64) bool {
	switch {
	case math.IsInf(b.Lower, -1), b.Lower == b.Upper:
		// Nothing is below -Inf, and the zero bucket of a histogram with a
		// zero threshold of 0 has a single boundary.
		return false
	case lowest:
		// Only the zero bucket contains 0, which is the implied lower
		// boundary of the lowest bucket.
		return b.Lower > 0 || b.Upper < 0
	default:
		return b.Lower != prevUpper
	}
}

// exponentialBucketLabels returns the label set of the _bucket series with the
// given boundary. The label set is cached in cache if it is not nil.
func exponentialBucketLabels(cache *ClassicSeriesCache, lsetBuilder *labels.Builder, lset labels.Labels, baseName string, le float64) labels.Labels {
	if cache != nil {
		if l, ok := cache.exponentialBucketLabels[le]; ok {
			return l
		}
	}
	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+ClassicSuffixBucket)
	lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(le))
	l := lsetBuilder.Labels()
	if cache != nil {
		if cache.exponentialBucketLabels == nil {
			cache.exponentialBucketLabels = make(map[float64]labels.Labels)
		}
		cache.exponentialBucketLabels[le] = l
	}
	return l
}
