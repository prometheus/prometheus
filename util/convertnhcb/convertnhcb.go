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

package convertnhcb

import (
	"math"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

type TempHistogram struct {
	BucketCounts map[float64]float64
	Count        float64
	Sum          float64
}

func NewTempHistogram() TempHistogram {
	return TempHistogram{
		BucketCounts: map[float64]float64{},
	}
}

func ProcessUpperBoundsAndCreateBaseHistogram(upperBounds0 []float64) ([]float64, *histogram.FloatHistogram) {
	sort.Float64s(upperBounds0)
	upperBounds := make([]float64, 0, len(upperBounds0))
	prevLE := math.Inf(-1)
	for _, le := range upperBounds0 {
		if le != prevLE { // deduplicate
			upperBounds = append(upperBounds, le)
			prevLE = le
		}
	}
	var customBounds []float64
	if upperBounds[len(upperBounds)-1] == math.Inf(1) {
		customBounds = upperBounds[:len(upperBounds)-1]
	} else {
		customBounds = upperBounds
	}
	return upperBounds, &histogram.FloatHistogram{
		Count:  0,
		Sum:    0,
		Schema: histogram.CustomBucketsSchema,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: uint32(len(upperBounds))},
		},
		PositiveBuckets: make([]float64, len(upperBounds)),
		CustomValues:    customBounds,
	}
}

func ConvertHistogramWrapper(hist TempHistogram, upperBounds []float64, fhBase *histogram.FloatHistogram) *histogram.FloatHistogram {
	fh := fhBase.Copy()
	var prevCount, total float64
	for i, le := range upperBounds {
		currCount, exists := hist.BucketCounts[le]
		if !exists {
			currCount = 0
		}
		count := currCount - prevCount
		fh.PositiveBuckets[i] = count
		total += count
		prevCount = currCount
	}
	fh.Sum = hist.Sum
	if hist.Count != 0 {
		total = hist.Count
	}
	fh.Count = total
	return fh.Compact(0)
}

func GetHistogramMetricBase(m labels.Labels, suffix string) labels.Labels {
	mName := m.Get(labels.MetricName)
	return labels.NewBuilder(m).
		Set(labels.MetricName, strings.TrimSuffix(mName, suffix)).
		Del(labels.BucketLabel).
		Labels()
}
