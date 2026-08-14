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

		customValues = h.CustomValues
		positiveBuckets = make([]float64, len(customValues)+1)

		// Histograms are in delta format so we first bring them to absolute format.
		acc := int64(0)
		for _, s := range h.PositiveSpans {
			for i := 0; i < int(s.Offset); i++ {
				positiveBuckets[idx] = float64(acc)
				idx++
			}
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
			return errors.New("unsupported histogram schema, not a NHCB")
		}

		// Validate the histogram before conversion.
		// The caller must ensure that the provided histogram is valid NHCB.
		if h.Validate() != nil {
			return errors.New(h.Validate().Error())
		}
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
		count = h.Count
		sum = h.Sum
	default:
		return fmt.Errorf("unsupported histogram type: %T", h)
	}

	currCount := float64(0)
	for i, val := range customValues {
		currCount += positiveBuckets[i]
		lsetBuilder.Reset(lset)
		lsetBuilder.Set(model.MetricNameLabel, baseName+"_bucket")
		lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(val))
		if err := emitSeriesFn(lsetBuilder.Labels(), currCount); err != nil {
			return err
		}
	}

	currCount += positiveBuckets[len(positiveBuckets)-1]

	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+"_bucket")
	lsetBuilder.Set(model.BucketLabel, labels.FormatOpenMetricsFloat(math.Inf(1)))
	if err := emitSeriesFn(lsetBuilder.Labels(), currCount); err != nil {
		return err
	}

	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+"_count")
	if err := emitSeriesFn(lsetBuilder.Labels(), count); err != nil {
		return err
	}

	lsetBuilder.Reset(lset)
	lsetBuilder.Set(model.MetricNameLabel, baseName+"_sum")
	if err := emitSeriesFn(lsetBuilder.Labels(), sum); err != nil {
		return err
	}

	return nil
}
