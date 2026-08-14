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
// Note: The first sample observed for a series is dropped to establish the
// start timestamp reference point. All subsequent samples are adjusted relative
// to this first sample.
//
// It maps closely to OpenTelemetry's cumulative-to-delta conversion patterns,
// similar to what is done in the OpenTelemetry Collector's `metricstarttimeprocessor`.
// See https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/metricstarttimeprocessor.
// TODO(ridwanmsharif): Make this a struct usable by a go interface (see https://github.com/prometheus/prometheus/pull/18279#discussion_r3032221849).
type stCache struct {
	f  *floatSynthesis
	h  *histogramSynthesis
	st int64
}

type floatSynthesis struct {
	prev     float64
	starting float64
}

// histogramSynthesis handles both Native integer Histograms and FloatHistograms.
// It works by caching the incoming histogram perfectly as a FloatHistogram
// to leverage native DetectReset and Sub methods.
type histogramSynthesis struct {
	prev     *histogram.FloatHistogram
	starting *histogram.FloatHistogram
}

// synthesizeFloat updates the synthesis cache for a float and returns the adjusted value, synthesized start time, and whether to skip append (for first sample).
func (c *stCache) synthesizeFloat(v float64, t int64) (float64, int64, bool) {
	if c.f == nil {
		c.f = &floatSynthesis{}
	}
	n := c.f

	if c.st == 0 {
		// First sample.
		n.prev = v
		n.starting = v
		c.st = t
		return v, c.st, true
	}

	if v < n.prev {
		// Reset detected.
		n.starting = 0
		// ST is somewhere between prev timestamp and current timestamp.
		// Pick the least risky guess: 1ms before the current timestamp.
		c.st = t - 1
	}

	n.prev = v
	adjustedValue := v - n.starting

	return adjustedValue, c.st, false
}

// synthesizeHistogram updates the synthesis state for a classic/native Integer Histogram and returns the adjusted histogram, synthesized start time, and whether to skip append (for first sample).
func (c *stCache) synthesizeHistogram(h *histogram.Histogram, t int64) (*histogram.Histogram, int64, bool) {
	if c.h == nil {
		c.h = &histogramSynthesis{}
	}
	n := c.h
	currFloat := h.ToFloat(nil)

	if c.st == 0 {
		// First sample.
		n.prev = currFloat.Copy()
		n.starting = n.prev
		c.st = t
		return h, c.st, true
	}

	if currFloat.DetectReset(n.prev) {
		// Reset detected.
		n.prev = currFloat.Copy()
		n.starting = nil // No starting, nothing to adjust for.
		// ST is somewhere between prev timestamp and current timestamp.
		// Pick the least risky guess: 1ms before the current timestamp.
		c.st = t - 1
		return h, c.st, false
	}

	n.prev = currFloat.Copy()
	if n.starting == nil {
		// Nothing to be adjusted for, return as-is.
		return h, c.st, false
	}

	// TODO(ridwanmsharif): If we implement DetectResets and Sub for Histograms, we
	// can do this in a cleaner way without losing precision when converting to
	// floating histograms and back. Need to look into how reset detection works
	// natively for histograms.

	// Subtract the origin anchor.
	subFloat, _, _, _ := currFloat.Sub(n.starting)
	subFloat = subFloat.Compact(0)

	// Since we are synthesizing an integer histogram, we must cast the float subtraction back to ints.
	// We've already established the risk of this approach in the comment above.
	// We can lean on the fact that FloatHistograms retain absolute bucket structure.
	// We will deeply construct a delta-encoded Histogram leveraging the subtracted absolute floats.
	adjusted := &histogram.Histogram{
		CounterResetHint: h.CounterResetHint,
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
			// Subtracted float buckets are absolute cumulative integers.
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

	return adjusted, c.st, false
}

// synthesizeFloatHistogram updates the synthesis state for a FloatHistogram and returns the adjusted histogram, synthesized start time, and whether to skip append (for first sample).
func (c *stCache) synthesizeFloatHistogram(fh *histogram.FloatHistogram, t int64) (*histogram.FloatHistogram, int64, bool) {
	if c.h == nil {
		c.h = &histogramSynthesis{}
	}
	n := c.h

	if c.st == 0 {
		// First sample.
		n.prev = fh.Copy()
		n.starting = n.prev
		c.st = t
		return fh, t, true
	}

	if fh.DetectReset(n.prev) {
		// Reset detected.
		n.prev = fh.Copy()
		n.starting = nil // No starting, nothing to adjust for.
		// ST is somewhere between prev timestamp and current timestamp.
		// Pick the least risky guess: 1ms before the current timestamp.
		c.st = t - 1
		return fh, c.st, false
	}

	n.prev = fh.Copy()
	if n.starting == nil {
		// Nothing to be adjusted for, return as-is.
		return fh, c.st, false
	}

	// Subtract the origin anchor.
	adjusted, _, _, _ := fh.Sub(n.starting)
	adjusted = adjusted.Compact(0)

	return adjusted, c.st, false
}
