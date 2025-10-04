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

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	ConvertedLabelSuffix = "_converted_to_classic_histogram"
)

// BucketEmitter is a callback function type for emitting histogram bucket series.
// Used in remote write to append converted bucket time series.
type BucketEmitter func(labels labels.Labels, value float64) error

// ConvertNHCBToClassicHistogram converts Native Histogram Custom Buckets (NHCB) to classic histogram series.
// This conversion is needed in various scenarios where users need to get NHCB back to classic histogram format,
// such as Remote Write v1 for external system compatibility and migration use cases.
func ConvertNHCBToClassicHistogram(nhcb any, labels labels.Labels, lblBuilder *labels.Builder, bucketSeries BucketEmitter) error {
	baseName := labels.Get("__name__")
	if baseName == "" {
		return errors.New("metric name label '__name__' is missing")
	}
	// Appending suffix to indicate conversion to classic histogram.
	// This is to avoid conflicts with existing classic histogram metrics.
	baseName += ConvertedLabelSuffix

	// checking if "le" already exists since we need to add exported_
	// prefix similar to honor label pattern of scrape
	leLabelsExists := labels.Get("le") != ""

	oldLabels := lblBuilder.Labels()
	defer lblBuilder.Reset(oldLabels)

	var (
		customValues    []float64
		positiveBuckets []float64
		count, sum      float64
	)

	switch h := nhcb.(type) {
	case *Histogram:
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
		customValues = h.CustomValues
		positiveBuckets = h.PositiveBuckets
		count = h.Count
		sum = h.Sum
	default:
		return errors.New("unsupported histogram type")
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
		lblBuilder.Reset(labels)
		lblBuilder.Set("__name__", baseName+"_bucket")

		// Handle existing "le" label by exporting it with "exported_" prefix
		// to avoid conflicts with the new "le" label for bucket upper bounds.
		if leLabelsExists {
			lblBuilder.Set(model.ExportedLabelPrefix+"le", labels.Get("le"))
		}
		lblBuilder.Set("le", fmt.Sprintf("%g", customValues[i]))
		bucketLabels := lblBuilder.Labels()
		if err := bucketSeries(bucketLabels, currCount); err != nil {
			return err
		}
	}

	lblBuilder.Reset(labels)
	lblBuilder.Set("__name__", baseName+"_bucket")
	// Handle existing "le" label by exporting it with "exported_" prefix
	// to avoid conflicts with the new "le" label for bucket upper bounds.
	if leLabelsExists {
		lblBuilder.Set(model.ExportedLabelPrefix+"le", labels.Get("le"))
	}
	lblBuilder.Set("le", fmt.Sprintf("%g", math.Inf(1)))
	infBucketLabels := lblBuilder.Labels()
	if err := bucketSeries(infBucketLabels, currCount); err != nil {
		return err
	}

	lblBuilder.Reset(labels)
	lblBuilder.Set("__name__", baseName+"_count")
	// Handle existing "le" label by exporting it with "exported_" prefix
	// to avoid conflicts with the new "le" label for bucket upper bounds.
	// we delete "le" label since it is exported and "le" is no longer needed.
	if leLabelsExists {
		lblBuilder.Set(model.ExportedLabelPrefix+"le", labels.Get("le"))
		lblBuilder.Del("le")
	}
	countLabels := lblBuilder.Labels()
	if err := bucketSeries(countLabels, count); err != nil {
		return err
	}

	lblBuilder.Reset(labels)
	lblBuilder.Set("__name__", baseName+"_sum")
	// Handle existing "le" label by exporting it with "exported_" prefix
	// to avoid conflicts with the new "le" label for bucket upper bounds.
	// we delete "le" label since it is exported and "le" is no longer needed.
	if leLabelsExists {
		lblBuilder.Set(model.ExportedLabelPrefix+"le", labels.Get("le"))
		lblBuilder.Del("le")
	}
	sumLabels := lblBuilder.Labels()
	if err := bucketSeries(sumLabels, sum); err != nil {
		return err
	}

	return nil
}
