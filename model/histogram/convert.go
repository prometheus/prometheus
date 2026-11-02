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
	"slices"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
)

// Suffixes identifying which classic series a caller wants out of
// ConvertNHCBToClassic. Passing "" emits all three (buckets, count and sum).
const (
	ClassicSuffixBucket = "_bucket"
	ClassicSuffixCount  = "_count"
	ClassicSuffixSum    = "_sum"
)

// ClassicSeriesCache holds precomputed label sets and scratch buffers across
// repeated ConvertNHCBToClassic calls.
type ClassicSeriesCache struct {
	lset         labels.Labels
	baseName     string
	bucketName   string
	countName    string
	sumName      string
	customValues []float64
	leStrings    []string

	bucketLabels    []labels.Labels // len(customValues)+1; last entry is the +Inf bucket.
	haveBuckets     bool
	countLabels     labels.Labels
	haveCount       bool
	sumLabels       labels.Labels
	haveSum         bool
	positiveBuckets []float64
}

// prepare updates the cache for lset and baseName, preserving reusable
// customValues, formatted leStrings, metric suffix names, and slice capacities
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

// customValuesMatch reports whether customValues equals the cached customValues.
func (c *ClassicSeriesCache) customValuesMatch(customValues []float64) bool {
	if len(c.customValues) != len(customValues) || len(c.leStrings) != len(customValues) {
		return false
	}
	for i, v := range customValues {
		if c.customValues[i] != v {
			return false
		}
	}
	return true
}

// bucketsMatch reports whether the cached bucket label sets were built for
// the current series and these exact custom bucket bounds.
func (c *ClassicSeriesCache) bucketsMatch(customValues []float64) bool {
	return c.haveBuckets && c.customValuesMatch(customValues)
}

// bucketLabelsFor returns the label set for bucket index idx (len(customValues)
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

// ClassicBucketCounts returns the cumulative bucket counts of a valid NHCB, one per
// custom bound followed by +Inf, along with its count and sum. It reuses buf.
// Every returned value of a stale marker is a stale marker.
func ClassicBucketCounts(nhcb any, buf []float64) (buckets []float64, count, sum float64, err error) {
	var (
		customValues    []float64
		positiveBuckets []float64
		idx             int // This index is to track buckets in Classic Histogram
		currIdx         int // This index is to track buckets in Native Histogram
	)

	switch h := nhcb.(type) {
	case *Histogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return nil, 0, 0, errors.New("unsupported histogram schema, not a NHCB")
		}

		// Validate the histogram before conversion.
		// The caller must ensure that the provided histogram is valid NHCB.
		if h.Validate() != nil {
			return nil, 0, 0, errors.New(h.Validate().Error())
		}

		customValues = h.CustomValues
		positiveBuckets = slices.Grow(buf[:0], len(customValues)+1)[:len(customValues)+1]
		clear(positiveBuckets)

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
		count = float64(h.Count)
		sum = h.Sum
	case *FloatHistogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return nil, 0, 0, errors.New("unsupported histogram schema, not a NHCB")
		}

		// Validate the histogram before conversion.
		// The caller must ensure that the provided histogram is valid NHCB.
		if h.Validate() != nil {
			return nil, 0, 0, errors.New(h.Validate().Error())
		}
		customValues = h.CustomValues
		positiveBuckets = slices.Grow(buf[:0], len(customValues)+1)[:len(customValues)+1]
		clear(positiveBuckets)

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
		count = h.Count
		sum = h.Sum
	default:
		return nil, 0, 0, fmt.Errorf("unsupported histogram type: %T", h)
	}

	// Classic buckets are cumulative, and every value of a stale marker is stale.
	isStale := value.IsStaleNaN(sum)
	currCount := float64(0)
	for i := range positiveBuckets {
		currCount += positiveBuckets[i]
		positiveBuckets[i] = currCount
		if isStale {
			positiveBuckets[i] = sum
		}
	}
	if isStale {
		count = sum
	}
	return positiveBuckets, count, sum, nil
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

	var buf []float64
	if cache != nil {
		buf = cache.positiveBuckets
	}
	buckets, count, sum, err := ClassicBucketCounts(nhcb, buf)
	if err != nil {
		return err
	}
	if cache != nil {
		cache.positiveBuckets = buckets
	}
	var customValues []float64
	switch h := nhcb.(type) {
	case *Histogram:
		customValues = h.CustomValues
	case *FloatHistogram:
		customValues = h.CustomValues
	}

	if wantBuckets {
		if cache != nil && !cache.bucketsMatch(customValues) {
			if !cache.customValuesMatch(customValues) {
				cache.customValues = append(cache.customValues[:0], customValues...)
				if cap(cache.leStrings) < len(customValues) {
					cache.leStrings = make([]string, len(customValues))
				} else {
					cache.leStrings = cache.leStrings[:len(customValues)]
				}
				for i, val := range customValues {
					cache.leStrings[i] = labels.FormatOpenMetricsFloat(val)
				}
			}
			nBuckets := len(customValues) + 1
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
			cache.bucketLabels[len(customValues)] = lsetBuilder.Labels()
			cache.haveBuckets = true
		}

		for i, val := range customValues {
			bucketLabels := bucketLabelsFor(cache, i, lsetBuilder, lset, baseName, val)
			if err := emitSeriesFn(bucketLabels, buckets[i]); err != nil {
				return err
			}
		}

		infLabels := bucketLabelsFor(cache, len(customValues), lsetBuilder, lset, baseName, math.Inf(1))
		if err := emitSeriesFn(infLabels, buckets[len(customValues)]); err != nil {
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
