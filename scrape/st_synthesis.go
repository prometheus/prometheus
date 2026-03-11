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

package scrape

import (
	"github.com/prometheus/prometheus/model/histogram"
)

// stCache contains the reference point and previous value
// information needed to synthesize start times for cumulative metrics
// (Counters, Summaries, Histograms).
//
// Only Counters and Histograms (Classic/Native/Float) are supported.
//
// It maps closely to OpenTelemetry's subtractinitial.adjuster logic.
type stCache struct {
	// For Counters / simple Floats / Summary _sum / Summary _count
	counter *floatSynthesis

	// For Native / Exponential Histograms (Both Integer and Float natively use this state)
	nativeHistogram *nativeHistogramSynthesis
}

type floatSynthesis struct {
	prevValue float64
	refValue  float64
	startTime int64
}

// nativeHistogramSynthesis handles both Native integer Histograms and FloatHistograms.
// It works by caching the incoming histogram perfectly as a FloatHistogram
// to leverage native DetectReset and Sub methods.
type nativeHistogramSynthesis struct {
	prevFloat *histogram.FloatHistogram
	refFloat  *histogram.FloatHistogram
	startTime int64
}

// ensureNumberSynthesis initializes or returns the number synthesis state.
func (st *stCache) ensureNumberSynthesis() *floatSynthesis {
	if st.counter == nil {
		st.counter = &floatSynthesis{}
	}
	return st.counter
}

// synthesizeNumber updates the synthesis state for a number (Counter/Gauge) and returns the adjusted value, synthesized start time, and whether to skip append (first sample).
// It detects resets if the current value is less than the previous value.
func (st *stCache) synthesizeNumber(currentValue float64, currentTs int64) (float64, int64, bool) {
	n := st.ensureNumberSynthesis()

	if n.startTime == 0 {
		// First sample
		n.prevValue = currentValue
		n.refValue = currentValue
		n.startTime = currentTs
		return currentValue, currentTs, true
	}

	if currentValue < n.prevValue {
		// Reset detected
		n.refValue = 0
		n.startTime = currentTs - 1
	}

	n.prevValue = currentValue
	adjustedValue := currentValue - n.refValue

	return adjustedValue, n.startTime, false
}

// ensureNativeHistogramSynthesis initializes or returns the unified native histogram synthesis state.
func (st *stCache) ensureNativeHistogramSynthesis() *nativeHistogramSynthesis {
	if st.nativeHistogram == nil {
		st.nativeHistogram = &nativeHistogramSynthesis{}
	}
	return st.nativeHistogram
}

// synthesizeHistogram updates the synthesis state for a classic/native Integer Histogram and returns the adjusted histogram, synthesized start time, and whether to skip append (first sample).
func (st *stCache) synthesizeHistogram(current *histogram.Histogram, currentTs int64) (*histogram.Histogram, int64, bool) {
	n := st.ensureNativeHistogramSynthesis()
	currFloat := current.ToFloat(nil)

	if n.startTime == 0 {
		// First sample
		n.prevFloat = currFloat.Copy()
		n.refFloat = currFloat.Copy()
		n.startTime = currentTs
		return current, currentTs, true
	}

	if currFloat.DetectReset(n.prevFloat) {
		// Reset detected
		n.prevFloat = currFloat.Copy()
		n.refFloat = currFloat.Copy()
		n.startTime = currentTs - 1
		return current, n.startTime, false
	}

	n.prevFloat = currFloat.Copy()

	// TODO(ridwanmsharif): If we implement DetectResets and Sub for Histograms, we
	// can do this in a cleaner way without losing precision when converting to
	// floating histograms and back. Need to look into how reset detection works
	// natively for histograms.

	// Mathematically subtract the origin anchor
	subFloat, _, _, _ := currFloat.Sub(n.refFloat)
	subFloat = subFloat.Compact(0)

	// Since we are synthesizing an integer histogram, we must cast the float subtraction back to ints.
	// We've already established the risk of this approach in the comment above.
	// We can lean on the fact that FloatHistograms retain absolute bucket structure.
	// We will deeply construct a delta-encoded Histogram leveraging the subtracted absolute floats.
	adjusted := &histogram.Histogram{
		CounterResetHint: current.CounterResetHint,
		Schema:           subFloat.Schema,
		ZeroThreshold:    subFloat.ZeroThreshold,
		ZeroCount:        uint64(subFloat.ZeroCount),
		Count:            uint64(subFloat.Count),
		Sum:              subFloat.Sum,
		CustomValues:     subFloat.CustomValues,
	}

	if len(subFloat.PositiveSpans) > 0 {
		adjusted.PositiveSpans = make([]histogram.Span, len(subFloat.PositiveSpans))
		copy(adjusted.PositiveSpans, subFloat.PositiveSpans)

		adjusted.PositiveBuckets = make([]int64, len(subFloat.PositiveBuckets))
		var last uint64
		for i, v := range subFloat.PositiveBuckets {
			// Subtracted float buckets are absolute cumulative integers mathematically
			absolute := uint64(v)
			adjusted.PositiveBuckets[i] = int64(absolute - last)
			last = absolute
		}
	}

	if len(subFloat.NegativeSpans) > 0 {
		adjusted.NegativeSpans = make([]histogram.Span, len(subFloat.NegativeSpans))
		copy(adjusted.NegativeSpans, subFloat.NegativeSpans)

		adjusted.NegativeBuckets = make([]int64, len(subFloat.NegativeBuckets))
		var last uint64
		for i, v := range subFloat.NegativeBuckets {
			absolute := uint64(v)
			adjusted.NegativeBuckets[i] = int64(absolute - last)
			last = absolute
		}
	}

	return adjusted, n.startTime, false
}

// synthesizeFloatHistogram updates the synthesis state for a FloatHistogram and returns the adjusted histogram, synthesized start time, and whether to skip append (first sample).
func (st *stCache) synthesizeFloatHistogram(current *histogram.FloatHistogram, currentTs int64) (*histogram.FloatHistogram, int64, bool) {
	n := st.ensureNativeHistogramSynthesis()

	if n.startTime == 0 {
		// First sample
		n.prevFloat = current.Copy()
		n.refFloat = current.Copy()
		n.startTime = currentTs
		return current, currentTs, true
	}

	if current.DetectReset(n.prevFloat) {
		// Reset detected
		n.prevFloat = current.Copy()
		n.refFloat = current.Copy()
		n.startTime = currentTs - 1
		return current, n.startTime, false
	}

	n.prevFloat = current.Copy()

	// Mathematically subtract the origin anchor
	adjusted, _, _, _ := current.Copy().Sub(n.refFloat)
	adjusted = adjusted.Compact(0)

	return adjusted, n.startTime, false
}
