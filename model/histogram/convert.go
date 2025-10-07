// Copyright 2025 The Prometheus Authors
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

	"github.com/prometheus/prometheus/model/labels"
)

// ConvertNHCBToClassic converts Native Histogram Custom Buckets (NHCB) to classic histogram series.
// This conversion is needed in various scenarios where users need to get NHCB back to classic histogram format,
// such as Remote Write v1 for external system compatibility and migration use cases.
func ConvertNHCBToClassic(nhcb any, lset labels.Labels, lsetBuilder *labels.Builder, emitSeriesFn func(labels labels.Labels, value float64) error) error {
	baseName := lset.Get("__name__")
	if baseName == "" {
		return errors.New("metric name label '__name__' is missing")
	}

	oldLabels := lsetBuilder.Labels()
	defer lsetBuilder.Reset(oldLabels)

	var (
		customValues    []float64
		positiveBuckets []float64
		count, sum      float64
	)

	switch h := nhcb.(type) {
	case *Histogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not a NHCB")
		}
		customValues = h.CustomValues
		positiveBuckets = make([]float64, len(h.PositiveBuckets))
		for i, v := range h.PositiveBuckets {
			if i == 0 {
				positiveBuckets[i] = float64(v)
			} else {
				positiveBuckets[i] = float64(v) + positiveBuckets[i-1]
			}
		}
		count = float64(h.Count)
		sum = h.Sum
	case *FloatHistogram:
		if !IsCustomBucketsSchema(h.Schema) {
			return errors.New("unsupported histogram schema, not a NHCB")
		}
		customValues = h.CustomValues
		positiveBuckets = h.PositiveBuckets
		count = h.Count
		sum = h.Sum
	default:
		return fmt.Errorf("unsupported histogram type: %T", h)
	}

	// Each customValue corresponds to a positive bucket (aligned with the "le" label).
	// The lengths of customValues and positiveBuckets must match to avoid inconsistencies
	// while mapping bucket counts to their upper bounds.
	if len(customValues) != len(positiveBuckets) {
		return errors.New("mismatched lengths of custom values and positive buckets")
	}

	currCount := float64(0)
	for i := range customValues {
		currCount = positiveBuckets[i]
		lsetBuilder.Reset(lset)
		lsetBuilder.Set("__name__", baseName+"_bucket")
		lsetBuilder.Set("le", labels.FormatOpenMetricsFloat(customValues[i]))
		if err := emitSeriesFn(lsetBuilder.Labels(), currCount); err != nil {
			return err
		}
	}

	lsetBuilder.Reset(lset)
	lsetBuilder.Set("__name__", baseName+"_bucket")
	lsetBuilder.Set("le", labels.FormatOpenMetricsFloat(math.Inf(1)))
	if err := emitSeriesFn(lsetBuilder.Labels(), currCount); err != nil {
		return err
	}

	lsetBuilder.Reset(lset)
	lsetBuilder.Set("__name__", baseName+"_count")
	if err := emitSeriesFn(lsetBuilder.Labels(), count); err != nil {
		return err
	}

	lsetBuilder.Reset(lset)
	lsetBuilder.Set("__name__", baseName+"_sum")
	if err := emitSeriesFn(lsetBuilder.Labels(), sum); err != nil {
		return err
	}

	return nil
}
