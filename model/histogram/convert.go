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

package histogram

import (
	"errors"
	"fmt"
	"math"

	"github.com/prometheus/prometheus/model/labels"
)

type TempHistogramBucket struct {
	Le    float64
	Count float64
}

type TempHistogram struct {
	Buckets  []TempHistogramBucket
	Count    uint64
	Sum      float64
	Err      error
	HasCount bool
}

type nhcbData struct {
	schema          int32
	customValues    []float64
	positiveBuckets []float64
	count           float64
	sum             float64
}

func validateNHCB(data *nhcbData) error {
	if data.schema != -53 {
		return errors.New("not an NHCB histogram (schema must be -53)")
	}

	if len(data.customValues) == 0 {
		return errors.New("NHCB histogram must have custom bucket boundaries")
	}

	if len(data.positiveBuckets) != len(data.customValues) {
		return errors.New("number of buckets must match number of custom values")
	}

	return nil
}

func convertNHCBToClassic(data *nhcbData) (*TempHistogram, error) {
	if err := validateNHCB(data); err != nil {
		return nil, err
	}

	buckets := make([]TempHistogramBucket, 0, len(data.customValues)+1)
	var currCount float64

	for i, upperBound := range data.customValues {
		currCount += data.positiveBuckets[i]
		buckets = append(buckets, TempHistogramBucket{
			Le:    upperBound,
			Count: currCount,
		})
	}

	buckets = append(buckets, TempHistogramBucket{
		Le:    math.Inf(1),
		Count: data.count,
	})

	return &TempHistogram{
		Buckets:  buckets,
		Count:    uint64(data.count),
		Sum:      data.sum,
		Err:      nil,
		HasCount: true,
	}, nil
}

func ConvertNHCBToClassicFloatHistogram(nhcb *FloatHistogram) (*TempHistogram, error) {
	if nhcb == nil {
		return nil, errors.New("input histogram is nil")
	}

	data := &nhcbData{
		schema:          nhcb.Schema,
		customValues:    nhcb.CustomValues,
		positiveBuckets: nhcb.PositiveBuckets,
		count:           nhcb.Count,
		sum:             nhcb.Sum,
	}

	return convertNHCBToClassic(data)
}

func ConvertNHCBToClassicHistogram(nhcb *Histogram) (*TempHistogram, error) {
	if nhcb == nil {
		return nil, errors.New("input histogram is nil")
	}

	positiveBuckets := make([]float64, len(nhcb.PositiveBuckets))
	for i, v := range nhcb.PositiveBuckets {
		positiveBuckets[i] = float64(v)
	}

	data := &nhcbData{
		schema:          nhcb.Schema,
		customValues:    nhcb.CustomValues,
		positiveBuckets: positiveBuckets,
		count:           float64(nhcb.Count),
		sum:             float64(nhcb.Sum),
	}

	return convertNHCBToClassic(data)
}

func BuildBucketOrSuffixLabels(lbls labels.Labels, baseName string, bucket *TempHistogramBucket, suffixLabel string) labels.Labels {
	labelBuilder := labels.NewBuilder(lbls)
	if bucket != nil {
		labelBuilder.Set("__name__", baseName+"_bucket")
		labelBuilder.Set("le", fmt.Sprintf("%g", bucket.Le))
	} else {
		labelBuilder.Set("__name__", baseName+suffixLabel)
	}
	return labelBuilder.Labels()
}
