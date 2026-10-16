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

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
)

// ConvertNHCBToClassic converts Native Histogram Custom Buckets (NHCB) to classic histogram series.
// This conversion is needed in various scenarios where users need to get NHCB back to classic histogram format,
// such as Remote Write v1 for external system compatibility and migration use cases.
//
// When calling this function, caller must ensure that provided nhcb is valid NHCB histogram.
func ConvertNHCBToClassic(nhcb any, lset labels.Labels, lsetBuilder *labels.Builder, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	baseName := lset.Get(model.MetricNameLabel)
	if baseName == "" {
		return errors.New("metric name label '__name__' is missing")
	}

	// We preserve original labels and restore them after conversion.
	// This is to ensure that no modifications are made to the original labels
	// that the queue_manager relies on.
	oldLabels := lsetBuilder.Labels()
	defer lsetBuilder.Reset(oldLabels)

	var (
		customValues   []float64
		spans          []Span
		intBuckets     []int64
		floatBuckets   []float64
		isIntHistogram bool
		count, sum     float64
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

		customValues = h.CustomValues
		spans = h.PositiveSpans
		intBuckets = h.PositiveBuckets
		isIntHistogram = true
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
		customValues = h.CustomValues
		spans = h.PositiveSpans
		floatBuckets = h.PositiveBuckets
		count = h.Count
		sum = h.Sum
	default:
		return fmt.Errorf("unsupported histogram type: %T", h)
	}

	var (
		spanIdx   int
		nextIdx   int
		remInSpan int
		currIdx   int
		intAcc    int64
		currCount float64
	)
	stepBucket := func(i int) {
		for remInSpan == 0 && spanIdx < len(spans) {
			nextIdx += int(spans[spanIdx].Offset)
			remInSpan = int(spans[spanIdx].Length)
			spanIdx++
		}
		if remInSpan > 0 && i == nextIdx {
			if isIntHistogram {
				intAcc += intBuckets[currIdx]
				currCount += float64(intAcc)
			} else {
				currCount += floatBuckets[currIdx]
			}
			currIdx++
			nextIdx++
			remInSpan--
		}
	}

	isStale := value.IsStaleNaN(sum)
	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+"_bucket")
	for i, val := range customValues {
		stepBucket(i)
		bucketVal := currCount
		if isStale {
			bucketVal = sum
		}
		lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(val))
		if err := emitSeriesFn(lsetBuilder.Labels(), bucketVal); err != nil {
			return err
		}
	}

	stepBucket(len(customValues))
	infVal := currCount
	if isStale {
		infVal = sum
		count = sum
	}

	lsetBuilder.Set(model.BucketLabel, "+Inf")
	if err := emitSeriesFn(lsetBuilder.Labels(), infVal); err != nil {
		return err
	}

	lsetBuilder.Del(model.BucketLabel)
	lsetBuilder.Set(model.MetricNameLabel, baseName+"_count")
	if err := emitSeriesFn(lsetBuilder.Labels(), count); err != nil {
		return err
	}

	lsetBuilder.Set(model.MetricNameLabel, baseName+"_sum")
	if err := emitSeriesFn(lsetBuilder.Labels(), sum); err != nil {
		return err
	}

	return nil
}
