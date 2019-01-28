// Copyright 2017, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package view

import (
	"math"

	"go.opencensus.io/exemplar"
)

// AggregationData represents an aggregated value from a collection.
// They are reported on the view data during exporting.
// Mosts users won't directly access aggregration data.
type AggregationData interface {
	isAggregationData() bool
	addSample(e *exemplar.Exemplar)
	clone() AggregationData
	equal(other AggregationData) bool
}

const epsilon = 1e-9

// CountData is the aggregated data for the Count aggregation.
// A count aggregation processes data and counts the recordings.
//
// Most users won't directly access count data.
type CountData struct {
	Value int64
}

func (a *CountData) isAggregationData() bool { return true }

func (a *CountData) addSample(_ *exemplar.Exemplar) {
	a.Value = a.Value + 1
}

func (a *CountData) clone() AggregationData {
	return &CountData{Value: a.Value}
}

func (a *CountData) equal(other AggregationData) bool {
	a2, ok := other.(*CountData)
	if !ok {
		return false
	}

	return a.Value == a2.Value
}

// SumData is the aggregated data for the Sum aggregation.
// A sum aggregation processes data and sums up the recordings.
//
// Most users won't directly access sum data.
type SumData struct {
	Value float64
}

func (a *SumData) isAggregationData() bool { return true }

func (a *SumData) addSample(e *exemplar.Exemplar) {
	a.Value += e.Value
}

func (a *SumData) clone() AggregationData {
	return &SumData{Value: a.Value}
}

func (a *SumData) equal(other AggregationData) bool {
	a2, ok := other.(*SumData)
	if !ok {
		return false
	}
	return math.Pow(a.Value-a2.Value, 2) < epsilon
}

// DistributionData is the aggregated data for the
// Distribution aggregation.
//
// Most users won't directly access distribution data.
//
// For a distribution with N bounds, the associated DistributionData will have
// N+1 buckets.
type DistributionData struct {
	Count           int64   // number of data points aggregated
	Min             float64 // minimum value in the distribution
	Max             float64 // max value in the distribution
	Mean            float64 // mean of the distribution
	SumOfSquaredDev float64 // sum of the squared deviation from the mean
	CountPerBucket  []int64 // number of occurrences per bucket
	// ExemplarsPerBucket is slice the same length as CountPerBucket containing
	// an exemplar for the associated bucket, or nil.
	ExemplarsPerBucket []*exemplar.Exemplar
	bounds             []float64 // histogram distribution of the values
}

func newDistributionData(bounds []float64) *DistributionData {
	bucketCount := len(bounds) + 1
	return &DistributionData{
		CountPerBucket:     make([]int64, bucketCount),
		ExemplarsPerBucket: make([]*exemplar.Exemplar, bucketCount),
		bounds:             bounds,
		Min:                math.MaxFloat64,
		Max:                math.SmallestNonzeroFloat64,
	}
}

// Sum returns the sum of all samples collected.
func (a *DistributionData) Sum() float64 { return a.Mean * float64(a.Count) }

func (a *DistributionData) variance() float64 {
	if a.Count <= 1 {
		return 0
	}
	return a.SumOfSquaredDev / float64(a.Count-1)
}

func (a *DistributionData) isAggregationData() bool { return true }

func (a *DistributionData) addSample(e *exemplar.Exemplar) {
	f := e.Value
	if f < a.Min {
		a.Min = f
	}
	if f > a.Max {
		a.Max = f
	}
	a.Count++
	a.addToBucket(e)

	if a.Count == 1 {
		a.Mean = f
		return
	}

	oldMean := a.Mean
	a.Mean = a.Mean + (f-a.Mean)/float64(a.Count)
	a.SumOfSquaredDev = a.SumOfSquaredDev + (f-oldMean)*(f-a.Mean)
}

func (a *DistributionData) addToBucket(e *exemplar.Exemplar) {
	var count *int64
	var ex **exemplar.Exemplar
	for i, b := range a.bounds {
		if e.Value < b {
			count = &a.CountPerBucket[i]
			ex = &a.ExemplarsPerBucket[i]
			break
		}
	}
	if count == nil {
		count = &a.CountPerBucket[len(a.bounds)]
		ex = &a.ExemplarsPerBucket[len(a.bounds)]
	}
	*count++
	*ex = maybeRetainExemplar(*ex, e)
}

func maybeRetainExemplar(old, cur *exemplar.Exemplar) *exemplar.Exemplar {
	if old == nil {
		return cur
	}

	// Heuristic to pick the "better" exemplar: first keep the one with a
	// sampled trace attachment, if neither have a trace attachment, pick the
	// one with more attachments.
	_, haveTraceID := cur.Attachments[exemplar.KeyTraceID]
	if haveTraceID || len(cur.Attachments) >= len(old.Attachments) {
		return cur
	}
	return old
}

func (a *DistributionData) clone() AggregationData {
	c := *a
	c.CountPerBucket = append([]int64(nil), a.CountPerBucket...)
	c.ExemplarsPerBucket = append([]*exemplar.Exemplar(nil), a.ExemplarsPerBucket...)
	return &c
}

func (a *DistributionData) equal(other AggregationData) bool {
	a2, ok := other.(*DistributionData)
	if !ok {
		return false
	}
	if a2 == nil {
		return false
	}
	if len(a.CountPerBucket) != len(a2.CountPerBucket) {
		return false
	}
	for i := range a.CountPerBucket {
		if a.CountPerBucket[i] != a2.CountPerBucket[i] {
			return false
		}
	}
	return a.Count == a2.Count && a.Min == a2.Min && a.Max == a2.Max && math.Pow(a.Mean-a2.Mean, 2) < epsilon && math.Pow(a.variance()-a2.variance(), 2) < epsilon
}

// LastValueData returns the last value recorded for LastValue aggregation.
type LastValueData struct {
	Value float64
}

func (l *LastValueData) isAggregationData() bool {
	return true
}

func (l *LastValueData) addSample(e *exemplar.Exemplar) {
	l.Value = e.Value
}

func (l *LastValueData) clone() AggregationData {
	return &LastValueData{l.Value}
}

func (l *LastValueData) equal(other AggregationData) bool {
	a2, ok := other.(*LastValueData)
	if !ok {
		return false
	}
	return l.Value == a2.Value
}
