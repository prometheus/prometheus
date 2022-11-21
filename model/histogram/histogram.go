// Copyright 2021 The Prometheus Authors
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
	"fmt"
	"math"
	"strings"
)

// Histogram encodes a sparse, high-resolution histogram. See the design
// document for full details:
// https://docs.google.com/document/d/1cLNv3aufPZb3fNfaJgdaRBZsInZKKIHo9E6HinJVbpM/edit#
//
// The most tricky bit is how bucket indices represent real bucket boundaries.
// An example for schema 0 (by which each bucket is twice as wide as the
// previous bucket):
//
//	Bucket boundaries →              [-2,-1)  [-1,-0.5) [-0.5,-0.25) ... [-0.001,0.001] ... (0.25,0.5] (0.5,1]  (1,2] ....
//	                                    ↑        ↑           ↑                  ↑                ↑         ↑      ↑
//	Zero bucket (width e.g. 0.001) →    |        |           |                  ZB               |         |      |
//	Positive bucket indices →           |        |           |                          ...     -1         0      1    2    3
//	Negative bucket indices →  3   2    1        0          -1       ...
//
// Which bucket indices are actually used is determined by the spans.
type Histogram struct {
	// Currently valid schema numbers are -4 <= n <= 8.  They are all for
	// base-2 bucket schemas, where 1 is a bucket boundary in each case, and
	// then each power of two is divided into 2^n logarithmic buckets.  Or
	// in other words, each bucket boundary is the previous boundary times
	// 2^(2^-n).
	Schema int32
	// Width of the zero bucket.
	ZeroThreshold float64
	// Observations falling into the zero bucket.
	ZeroCount uint64
	// Total number of observations.
	Count uint64
	// Sum of observations. This is also used as the stale marker.
	Sum float64
	// Spans for positive and negative buckets (see Span below).
	PositiveSpans, NegativeSpans []Span
	// Observation counts in buckets. The first element is an absolute
	// count. All following ones are deltas relative to the previous
	// element.
	PositiveBuckets, NegativeBuckets []int64
}

// A Span defines a continuous sequence of buckets.
type Span struct {
	// Gap to previous span (always positive), or starting index for the 1st
	// span (which can be negative).
	Offset int32
	// Length of the span.
	Length uint32
}

// Copy returns a deep copy of the Histogram.
func (h *Histogram) Copy() *Histogram {
	c := *h

	if len(h.PositiveSpans) != 0 {
		c.PositiveSpans = make([]Span, len(h.PositiveSpans))
		copy(c.PositiveSpans, h.PositiveSpans)
	}
	if len(h.NegativeSpans) != 0 {
		c.NegativeSpans = make([]Span, len(h.NegativeSpans))
		copy(c.NegativeSpans, h.NegativeSpans)
	}
	if len(h.PositiveBuckets) != 0 {
		c.PositiveBuckets = make([]int64, len(h.PositiveBuckets))
		copy(c.PositiveBuckets, h.PositiveBuckets)
	}
	if len(h.NegativeBuckets) != 0 {
		c.NegativeBuckets = make([]int64, len(h.NegativeBuckets))
		copy(c.NegativeBuckets, h.NegativeBuckets)
	}

	return &c
}

// String returns a string representation of the Histogram.
func (h *Histogram) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "{count:%d, sum:%g", h.Count, h.Sum)

	var nBuckets []Bucket[uint64]
	for it := h.NegativeBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			nBuckets = append(nBuckets, it.At())
		}
	}
	for i := len(nBuckets) - 1; i >= 0; i-- {
		fmt.Fprintf(&sb, ", %s", nBuckets[i].String())
	}

	if h.ZeroCount != 0 {
		fmt.Fprintf(&sb, ", %s", h.ZeroBucket().String())
	}

	for it := h.PositiveBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			fmt.Fprintf(&sb, ", %s", bucket.String())
		}
	}

	sb.WriteRune('}')
	return sb.String()
}

// ZeroBucket returns the zero bucket.
func (h *Histogram) ZeroBucket() Bucket[uint64] {
	return Bucket[uint64]{
		Lower:          -h.ZeroThreshold,
		Upper:          h.ZeroThreshold,
		LowerInclusive: true,
		UpperInclusive: true,
		Count:          h.ZeroCount,
	}
}

// PositiveBucketIterator returns a BucketIterator to iterate over all positive
// buckets in ascending order (starting next to the zero bucket and going up).
func (h *Histogram) PositiveBucketIterator() BucketIterator[uint64] {
	return newRegularBucketIterator(h.PositiveSpans, h.PositiveBuckets, h.Schema, true)
}

// NegativeBucketIterator returns a BucketIterator to iterate over all negative
// buckets in descending order (starting next to the zero bucket and going down).
func (h *Histogram) NegativeBucketIterator() BucketIterator[uint64] {
	return newRegularBucketIterator(h.NegativeSpans, h.NegativeBuckets, h.Schema, false)
}

// CumulativeBucketIterator returns a BucketIterator to iterate over a
// cumulative view of the buckets. This method currently only supports
// Histograms without negative buckets and panics if the Histogram has negative
// buckets. It is currently only used for testing.
func (h *Histogram) CumulativeBucketIterator() BucketIterator[uint64] {
	if len(h.NegativeBuckets) > 0 {
		panic("CumulativeBucketIterator called on Histogram with negative buckets")
	}
	return &cumulativeBucketIterator{h: h, posSpansIdx: -1}
}

// Equals returns true if the given histogram matches exactly.
// Exact match is when there are no new buckets (even empty) and no missing buckets,
// and all the bucket values match. Spans can have different empty length spans in between,
// but they must represent the same bucket layout to match.
func (h *Histogram) Equals(h2 *Histogram) bool {
	if h2 == nil {
		return false
	}

	if h.Schema != h2.Schema || h.ZeroThreshold != h2.ZeroThreshold ||
		h.ZeroCount != h2.ZeroCount || h.Count != h2.Count || h.Sum != h2.Sum {
		return false
	}

	if !spansMatch(h.PositiveSpans, h2.PositiveSpans) {
		return false
	}
	if !spansMatch(h.NegativeSpans, h2.NegativeSpans) {
		return false
	}

	if !bucketsMatch(h.PositiveBuckets, h2.PositiveBuckets) {
		return false
	}
	if !bucketsMatch(h.NegativeBuckets, h2.NegativeBuckets) {
		return false
	}

	return true
}

// spansMatch returns true if both spans represent the same bucket layout
// after combining zero length spans with the next non-zero length span.
func spansMatch(s1, s2 []Span) bool {
	if len(s1) == 0 && len(s2) == 0 {
		return true
	}

	s1idx, s2idx := 0, 0
	for {
		if s1idx >= len(s1) {
			return allEmptySpans(s2[s2idx:])
		}
		if s2idx >= len(s2) {
			return allEmptySpans(s1[s1idx:])
		}

		currS1, currS2 := s1[s1idx], s2[s2idx]
		s1idx++
		s2idx++
		if currS1.Length == 0 {
			// This span is zero length, so we add consecutive such spans
			// until we find a non-zero span.
			for ; s1idx < len(s1) && s1[s1idx].Length == 0; s1idx++ {
				currS1.Offset += s1[s1idx].Offset
			}
			if s1idx < len(s1) {
				currS1.Offset += s1[s1idx].Offset
				currS1.Length = s1[s1idx].Length
				s1idx++
			}
		}
		if currS2.Length == 0 {
			// This span is zero length, so we add consecutive such spans
			// until we find a non-zero span.
			for ; s2idx < len(s2) && s2[s2idx].Length == 0; s2idx++ {
				currS2.Offset += s2[s2idx].Offset
			}
			if s2idx < len(s2) {
				currS2.Offset += s2[s2idx].Offset
				currS2.Length = s2[s2idx].Length
				s2idx++
			}
		}

		if currS1.Length == 0 && currS2.Length == 0 {
			// The last spans of both set are zero length. Previous spans match.
			return true
		}

		if currS1.Offset != currS2.Offset || currS1.Length != currS2.Length {
			return false
		}
	}
}

func allEmptySpans(s []Span) bool {
	for _, ss := range s {
		if ss.Length > 0 {
			return false
		}
	}
	return true
}

func bucketsMatch(b1, b2 []int64) bool {
	if len(b1) != len(b2) {
		return false
	}
	for i, b := range b1 {
		if b != b2[i] {
			return false
		}
	}
	return true
}

// Compact works like FloatHistogram.Compact. See there for detailed
// explanations.
func (h *Histogram) Compact(maxEmptyBuckets int) *Histogram {
	h.PositiveBuckets, h.PositiveSpans = compactBuckets(
		h.PositiveBuckets, h.PositiveSpans, maxEmptyBuckets, true,
	)
	h.NegativeBuckets, h.NegativeSpans = compactBuckets(
		h.NegativeBuckets, h.NegativeSpans, maxEmptyBuckets, true,
	)
	return h
}

// ToFloat returns a FloatHistogram representation of the Histogram. It is a
// deep copy (e.g. spans are not shared).
func (h *Histogram) ToFloat() *FloatHistogram {
	var (
		positiveSpans, negativeSpans     []Span
		positiveBuckets, negativeBuckets []float64
	)
	if len(h.PositiveSpans) != 0 {
		positiveSpans = make([]Span, len(h.PositiveSpans))
		copy(positiveSpans, h.PositiveSpans)
	}
	if len(h.NegativeSpans) != 0 {
		negativeSpans = make([]Span, len(h.NegativeSpans))
		copy(negativeSpans, h.NegativeSpans)
	}
	if len(h.PositiveBuckets) != 0 {
		positiveBuckets = make([]float64, len(h.PositiveBuckets))
		var current float64
		for i, b := range h.PositiveBuckets {
			current += float64(b)
			positiveBuckets[i] = current
		}
	}
	if len(h.NegativeBuckets) != 0 {
		negativeBuckets = make([]float64, len(h.NegativeBuckets))
		var current float64
		for i, b := range h.NegativeBuckets {
			current += float64(b)
			negativeBuckets[i] = current
		}
	}

	return &FloatHistogram{
		Schema:          h.Schema,
		ZeroThreshold:   h.ZeroThreshold,
		ZeroCount:       float64(h.ZeroCount),
		Count:           float64(h.Count),
		Sum:             h.Sum,
		PositiveSpans:   positiveSpans,
		NegativeSpans:   negativeSpans,
		PositiveBuckets: positiveBuckets,
		NegativeBuckets: negativeBuckets,
	}
}

type regularBucketIterator struct {
	baseBucketIterator[uint64, int64]
}

func newRegularBucketIterator(spans []Span, buckets []int64, schema int32, positive bool) *regularBucketIterator {
	i := baseBucketIterator[uint64, int64]{
		schema:   schema,
		spans:    spans,
		buckets:  buckets,
		positive: positive,
	}
	return &regularBucketIterator{i}
}

func (r *regularBucketIterator) Next() bool {
	if r.spansIdx >= len(r.spans) {
		return false
	}
	span := r.spans[r.spansIdx]
	// Seed currIdx for the first bucket.
	if r.bucketsIdx == 0 {
		r.currIdx = span.Offset
	} else {
		r.currIdx++
	}
	for r.idxInSpan >= span.Length {
		// We have exhausted the current span and have to find a new
		// one. We'll even handle pathologic spans of length 0.
		r.idxInSpan = 0
		r.spansIdx++
		if r.spansIdx >= len(r.spans) {
			return false
		}
		span = r.spans[r.spansIdx]
		r.currIdx += span.Offset
	}

	r.currCount += r.buckets[r.bucketsIdx]
	r.idxInSpan++
	r.bucketsIdx++
	return true
}

type cumulativeBucketIterator struct {
	h *Histogram

	posSpansIdx   int    // Index in h.PositiveSpans we are in. -1 means 0 bucket.
	posBucketsIdx int    // Index in h.PositiveBuckets.
	idxInSpan     uint32 // Index in the current span. 0 <= idxInSpan < span.Length.

	initialized         bool
	currIdx             int32   // The actual bucket index after decoding from spans.
	currUpper           float64 // The upper boundary of the current bucket.
	currCount           int64   // Current non-cumulative count for the current bucket. Does not apply for empty bucket.
	currCumulativeCount uint64  // Current "cumulative" count for the current bucket.

	// Between 2 spans there could be some empty buckets which
	// still needs to be counted for cumulative buckets.
	// When we hit the end of a span, we use this to iterate
	// through the empty buckets.
	emptyBucketCount int32
}

func (c *cumulativeBucketIterator) Next() bool {
	if c.posSpansIdx == -1 {
		// Zero bucket.
		c.posSpansIdx++
		if c.h.ZeroCount == 0 {
			return c.Next()
		}

		c.currUpper = c.h.ZeroThreshold
		c.currCount = int64(c.h.ZeroCount)
		c.currCumulativeCount = uint64(c.currCount)
		return true
	}

	if c.posSpansIdx >= len(c.h.PositiveSpans) {
		return false
	}

	if c.emptyBucketCount > 0 {
		// We are traversing through empty buckets at the moment.
		c.currUpper = getBound(c.currIdx, c.h.Schema)
		c.currIdx++
		c.emptyBucketCount--
		return true
	}

	span := c.h.PositiveSpans[c.posSpansIdx]
	if c.posSpansIdx == 0 && !c.initialized {
		// Initializing.
		c.currIdx = span.Offset
		// The first bucket is an absolute value and not a delta with Zero bucket.
		c.currCount = 0
		c.initialized = true
	}

	c.currCount += c.h.PositiveBuckets[c.posBucketsIdx]
	c.currCumulativeCount += uint64(c.currCount)
	c.currUpper = getBound(c.currIdx, c.h.Schema)

	c.posBucketsIdx++
	c.idxInSpan++
	c.currIdx++
	if c.idxInSpan >= span.Length {
		// Move to the next span. This one is done.
		c.posSpansIdx++
		c.idxInSpan = 0
		if c.posSpansIdx < len(c.h.PositiveSpans) {
			c.emptyBucketCount = c.h.PositiveSpans[c.posSpansIdx].Offset
		}
	}

	return true
}

func (c *cumulativeBucketIterator) At() Bucket[uint64] {
	return Bucket[uint64]{
		Upper:          c.currUpper,
		Lower:          math.Inf(-1),
		UpperInclusive: true,
		LowerInclusive: true,
		Count:          c.currCumulativeCount,
		Index:          c.currIdx - 1,
	}
}
