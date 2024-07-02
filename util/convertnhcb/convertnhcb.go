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
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/grafana/regexp"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

var histogramNameSuffixReplacements = []struct {
	pattern *regexp.Regexp
	repl    string
}{
	{
		pattern: regexp.MustCompile(`_bucket$`),
		repl:    "",
	},
	{
		pattern: regexp.MustCompile(`_sum$`),
		repl:    "",
	},
	{
		pattern: regexp.MustCompile(`_count$`),
		repl:    "",
	},
}

type TempHistogram struct {
	BucketCounts map[float64]float64
	Count        float64
	Sum          float64
	HasFloat     bool
}

func NewTempHistogram() TempHistogram {
	return TempHistogram{
		BucketCounts: map[float64]float64{},
	}
}

func (h TempHistogram) getIntBucketCounts() (map[float64]int64, error) {
	bucketCounts := map[float64]int64{}
	for le, count := range h.BucketCounts {
		intCount := int64(math.Round(count))
		if float64(intCount) != count {
			return nil, fmt.Errorf("bucket count %f for le %g is not an integer", count, le)
		}
		bucketCounts[le] = intCount
	}
	return bucketCounts, nil
}

func ProcessUpperBoundsAndCreateBaseHistogram(upperBounds0 []float64, needsDedup bool) ([]float64, *histogram.Histogram) {
	sort.Float64s(upperBounds0)
	var upperBounds []float64
	if needsDedup {
		upperBounds = make([]float64, 0, len(upperBounds0))
		prevLE := math.Inf(-1)
		for _, le := range upperBounds0 {
			if le != prevLE {
				upperBounds = append(upperBounds, le)
				prevLE = le
			}
		}
	} else {
		upperBounds = upperBounds0
	}
	var customBounds []float64
	if upperBounds[len(upperBounds)-1] == math.Inf(1) {
		customBounds = upperBounds[:len(upperBounds)-1]
	} else {
		customBounds = upperBounds
	}
	return upperBounds, &histogram.Histogram{
		Count:  0,
		Sum:    0,
		Schema: histogram.CustomBucketsSchema,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: uint32(len(upperBounds))},
		},
		PositiveBuckets: make([]int64, len(upperBounds)),
		CustomValues:    customBounds,
	}
}

func ConvertHistogramWrapper(histogram TempHistogram, upperBounds []float64, hBase *histogram.Histogram, fhBase *histogram.FloatHistogram) (*histogram.Histogram, *histogram.FloatHistogram) {
	intBucketCounts, err := histogram.getIntBucketCounts()
	if err != nil {
		return nil, convertFloatHistogramWrapper(histogram, upperBounds, histogram.BucketCounts, fhBase)
	}
	return convertIntHistogramWrapper(histogram, upperBounds, intBucketCounts, hBase), nil
}

func convertIntHistogramWrapper(histogram TempHistogram, upperBounds []float64, bucketCounts map[float64]int64, hBase *histogram.Histogram) *histogram.Histogram {
	h := hBase.Copy()
	absBucketCounts := make([]int64, len(h.PositiveBuckets))
	var prevCount, total int64
	for i, le := range upperBounds {
		currCount, exists := bucketCounts[le]
		if !exists {
			currCount = 0
		}
		count := currCount - prevCount
		absBucketCounts[i] = count
		total += count
		prevCount = currCount
	}
	h.PositiveBuckets[0] = absBucketCounts[0]
	for i := 1; i < len(h.PositiveBuckets); i++ {
		h.PositiveBuckets[i] = absBucketCounts[i] - absBucketCounts[i-1]
	}
	h.Sum = histogram.Sum
	if histogram.Count != 0 {
		total = int64(histogram.Count)
	}
	h.Count = uint64(total)
	return h.Compact(0)
}

func convertFloatHistogramWrapper(histogram TempHistogram, upperBounds []float64, bucketCounts map[float64]float64, fhBase *histogram.FloatHistogram) *histogram.FloatHistogram {
	fh := fhBase.Copy()
	var prevCount, total float64
	for i, le := range upperBounds {
		currCount, exists := bucketCounts[le]
		if !exists {
			currCount = 0
		}
		count := currCount - prevCount
		fh.PositiveBuckets[i] = count
		total += count
		prevCount = currCount
	}
	fh.Sum = histogram.Sum
	if histogram.Count != 0 {
		total = histogram.Count
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

func GetHistogramMetricBaseName(m labels.Labels) string {
	s := m.Get(labels.MetricName)
	for _, rep := range histogramNameSuffixReplacements {
		s = rep.pattern.ReplaceAllString(s, rep.repl)
	}
	return s
}
