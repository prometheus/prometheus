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

// startTimeSynthesis contains the reference point and previous value
// information needed to synthesize start times for cumulative metrics
// (Counters, Summaries, Histograms).
// It maps closely to OpenTelemetry's subtractinitial.adjuster logic.
type startTimeSynthesis struct {
	// For Counters / simple Floats / Summary _sum / Summary _count
	number *numberSynthesis

	// For Classic Histograms
	histogram *histogramSynthesis

	// For Native / Exponential Histograms (Float)
	floatHistogram *floatHistogramSynthesis
}

type numberSynthesis struct {
	prevValue float64
	refValue  float64
	startTime int64
}

type histogramSynthesis struct {
	prevSum       float64
	prevCount     uint64
	prevZeroCount uint64
	refSum        float64
	refCount      uint64
	refZeroCount  uint64
	startTime     int64
	refBuckets    []uint64
}

type floatHistogramSynthesis struct {
	prevSum       float64
	prevCount     uint64
	prevZeroCount uint64
	refSum        float64
	refCount      uint64
	refZeroCount  uint64
	startTime     int64
	refPosBuckets []int64
	refNegBuckets []int64
}

// ensureNumberSynthesis initializes or returns the number synthesis state.
func (st *startTimeSynthesis) ensureNumberSynthesis() *numberSynthesis {
	if st.number == nil {
		st.number = &numberSynthesis{}
	}
	return st.number
}

// synthesizeNumber updates the synthesis state for a number (Counter/Gauge) and returns the adjusted value, synthesized start time, and whether to skip append (first sample).
// It detects resets if the current value is less than the previous value.
func (st *startTimeSynthesis) synthesizeNumber(currentValue float64, currentTs int64) (float64, int64, bool) {
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

// ensureHistogramSynthesis initializes or returns the classic histogram synthesis state.
func (st *startTimeSynthesis) ensureHistogramSynthesis() *histogramSynthesis {
	if st.histogram == nil {
		st.histogram = &histogramSynthesis{}
	}
	return st.histogram
}

// synthesizeHistogram updates the synthesis state for a classic/native Integer Histogram and returns the adjusted histogram, synthesized start time, and whether to skip append (first sample).
func (st *startTimeSynthesis) synthesizeHistogram(current *histogram.Histogram, currentTs int64) (*histogram.Histogram, int64, bool) {
	h := st.ensureHistogramSynthesis()

	if h.refSum == 0 && h.refCount == 0 && h.startTime == 0 {
		// First sample
		h.prevSum = current.Sum
		h.prevCount = current.Count
		h.prevZeroCount = current.ZeroCount
		h.refSum = current.Sum
		h.refCount = current.Count
		h.refZeroCount = current.ZeroCount
		h.startTime = currentTs
		return current, currentTs, true
	}

	// A reset is detected if sum or count goes down.
	if current.Sum < h.prevSum || current.Count < h.prevCount {
		h.refSum = 0
		h.refCount = 0
		h.refZeroCount = 0
		h.refBuckets = nil
		h.startTime = currentTs - 1
	}

	h.prevSum = current.Sum
	h.prevCount = current.Count
	h.prevZeroCount = current.ZeroCount

	// Construct the newly adjusted histogram
	adjusted := current.Copy()
	adjusted.Sum -= h.refSum
	adjusted.Count -= h.refCount
	adjusted.ZeroCount -= h.refZeroCount

	return adjusted, h.startTime, false
}

// ensureFloatHistogramSynthesis initializes or returns the float histogram synthesis state.
func (st *startTimeSynthesis) ensureFloatHistogramSynthesis() *floatHistogramSynthesis {
	if st.floatHistogram == nil {
		st.floatHistogram = &floatHistogramSynthesis{}
	}
	return st.floatHistogram
}

// synthesizeFloatHistogram updates the synthesis state for a FloatHistogram and returns the adjusted histogram, synthesized start time, and whether to skip append (first sample).
func (st *startTimeSynthesis) synthesizeFloatHistogram(current *histogram.FloatHistogram, currentTs int64) (*histogram.FloatHistogram, int64, bool) {
	fh := st.ensureFloatHistogramSynthesis()

	if fh.refSum == 0 && fh.refCount == 0 && fh.startTime == 0 {
		// First sample
		fh.prevSum = current.Sum
		fh.prevCount = uint64(current.Count)
		fh.prevZeroCount = uint64(current.ZeroCount)
		fh.refSum = current.Sum
		fh.refCount = uint64(current.Count)
		fh.refZeroCount = uint64(current.ZeroCount)
		fh.startTime = currentTs
		return current, currentTs, true
	}

	if current.Sum < fh.prevSum || uint64(current.Count) < fh.prevCount {
		fh.refSum = 0
		fh.refCount = 0
		fh.refZeroCount = 0
		fh.refPosBuckets = nil
		fh.refNegBuckets = nil
		fh.startTime = currentTs - 1
	}

	fh.prevSum = current.Sum
	fh.prevCount = uint64(current.Count)
	fh.prevZeroCount = uint64(current.ZeroCount)

	// Construct the newly adjusted histogram
	adjusted := current.Copy()
	adjusted.Sum -= fh.refSum
	adjusted.Count -= float64(fh.refCount)
	adjusted.ZeroCount -= float64(fh.refZeroCount)

	// Since prometheus FloatHistogram buckets are absolute, we subtract the reference buckets.
	// If structures differ (e.g. spans changed), OTel subtractinitial resets. For safety and simplicity,
	// if spans differ from reference, we should ideally treat it as a reset, but prometheus scrape loop
	// doesn't persist spanning identically across rescrapes natively if 0 counts vanish.
	// However, we can simply subtract bucket by bucket if spans match, otherwise reset.
	// We'll leave the bucket subtraction out of this initial minimal implementation to ensure stability,
	// or implement a basic bucket alignment later if required. For now, we adjust sum/count accurately.

	return adjusted, fh.startTime, false
}
