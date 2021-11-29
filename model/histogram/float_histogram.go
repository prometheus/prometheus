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

// FloatHistogram is similar to Histogram but uses float64 for all
// counts. Additionally, bucket counts are absolute and not deltas.
//
// A FloatHistogram is needed by PromQL to handle operations that might result
// in fractional counts. Since the counts in a histogram are unlikely to be too
// large to be represented precisely by a float64, a FloatHistogram can also be
// used to represent a histogram with integer counts and thus serves as a more
// generalized representation.
type FloatHistogram struct {
	// Currently valid schema numbers are -4 <= n <= 8.  They are all for
	// base-2 bucket schemas, where 1 is a bucket boundary in each case, and
	// then each power of two is divided into 2^n logarithmic buckets.  Or
	// in other words, each bucket boundary is the previous boundary times
	// 2^(2^-n).
	Schema int32
	// Width of the zero bucket.
	ZeroThreshold float64
	// Observations falling into the zero bucket. Must be zero or positive.
	ZeroCount float64
	// Total number of observations. Must be zero or positive.
	Count float64
	// Sum of observations. This is also used as the stale marker.
	Sum float64
	// Spans for positive and negative buckets (see Span below).
	PositiveSpans, NegativeSpans []Span
	// Observation counts in buckets. Each represents an absolute count and
	// must be zero or positive.
	PositiveBuckets, NegativeBuckets []float64
}

// Copy returns a deep copy of the Histogram.
func (h *FloatHistogram) Copy() *FloatHistogram {
	c := *h

	if h.PositiveSpans != nil {
		c.PositiveSpans = make([]Span, len(h.PositiveSpans))
		copy(c.PositiveSpans, h.PositiveSpans)
	}
	if h.NegativeSpans != nil {
		c.NegativeSpans = make([]Span, len(h.NegativeSpans))
		copy(c.NegativeSpans, h.NegativeSpans)
	}
	if h.PositiveBuckets != nil {
		c.PositiveBuckets = make([]float64, len(h.PositiveBuckets))
		copy(c.PositiveBuckets, h.PositiveBuckets)
	}
	if h.NegativeBuckets != nil {
		c.NegativeBuckets = make([]float64, len(h.NegativeBuckets))
		copy(c.NegativeBuckets, h.NegativeBuckets)
	}

	return &c
}

// String returns a string representation of the Histogram.
func (h *FloatHistogram) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "{count:%g, sum:%g", h.Count, h.Sum)

	var nBuckets []FloatBucket
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
func (h *FloatHistogram) ZeroBucket() FloatBucket {
	return FloatBucket{
		Lower:          -h.ZeroThreshold,
		Upper:          h.ZeroThreshold,
		LowerInclusive: true,
		UpperInclusive: true,
		Count:          h.ZeroCount,
	}
}

// PositiveBucketIterator returns a FloatBucketIterator to iterate over all
// positive buckets in ascending order (starting next to the zero bucket and
// going up).
func (h *FloatHistogram) PositiveBucketIterator() FloatBucketIterator {
	return newFloatBucketIterator(h, true)
}

// NegativeBucketIterator returns a FloatBucketIterator to iterate over all
// negative buckets in descending order (starting next to the zero bucket and
// going down).
func (h *FloatHistogram) NegativeBucketIterator() FloatBucketIterator {
	return newFloatBucketIterator(h, false)
}

// CumulativeBucketIterator returns a FloatBucketIterator to iterate over a
// cumulative view of the buckets. This method currently only supports
// FloatHistograms without negative buckets and panics if the FloatHistogram has
// negative buckets. It is currently only used for testing.
func (h *FloatHistogram) CumulativeBucketIterator() FloatBucketIterator {
	if len(h.NegativeBuckets) > 0 {
		panic("CumulativeBucketIterator called on FloatHistogram with negative buckets")
	}
	return &cumulativeFloatBucketIterator{h: h, posSpansIdx: -1}
}

// FloatBucketIterator iterates over the buckets of a FloatHistogram, returning
// decoded buckets.
type FloatBucketIterator interface {
	// Next advances the iterator by one.
	Next() bool
	// At returns the current bucket.
	At() FloatBucket
}

// FloatBucket represents a bucket with lower and upper limit and the count of
// samples in the bucket as a float64. It also specifies if each limit is
// inclusive or not. (Mathematically, inclusive limits create a closed interval,
// and non-inclusive limits an open interval.)
//
// To represent cumulative buckets, Lower is set to -Inf, and the Count is then
// cumulative (including the counts of all buckets for smaller values).
type FloatBucket struct {
	Lower, Upper                   float64
	LowerInclusive, UpperInclusive bool
	Count                          float64
	Index                          int32 // Index within schema. To easily compare buckets that share the same schema.
}

// String returns a string representation of a FloatBucket, using the usual
// mathematical notation of '['/']' for inclusive bounds and '('/')' for
// non-inclusive bounds.
func (b FloatBucket) String() string {
	var sb strings.Builder
	if b.LowerInclusive {
		sb.WriteRune('[')
	} else {
		sb.WriteRune('(')
	}
	fmt.Fprintf(&sb, "%g,%g", b.Lower, b.Upper)
	if b.UpperInclusive {
		sb.WriteRune(']')
	} else {
		sb.WriteRune(')')
	}
	fmt.Fprintf(&sb, ":%g", b.Count)
	return sb.String()
}

type floatBucketIterator struct {
	schema  int32
	spans   []Span
	buckets []float64

	positive bool // Whether this is for positive buckets.

	spansIdx   int    // Current span within spans slice.
	idxInSpan  uint32 // Index in the current span. 0 <= idxInSpan < span.Length.
	bucketsIdx int    // Current bucket within buckets slice.

	currCount            float64 // Count in the current bucket.
	currIdx              int32   // The actual bucket index.
	currLower, currUpper float64 // Limits of the current bucket.

}

func newFloatBucketIterator(h *FloatHistogram, positive bool) *floatBucketIterator {
	r := &floatBucketIterator{schema: h.Schema, positive: positive}
	if positive {
		r.spans = h.PositiveSpans
		r.buckets = h.PositiveBuckets
	} else {
		r.spans = h.NegativeSpans
		r.buckets = h.NegativeBuckets
	}
	return r
}

func (r *floatBucketIterator) Next() bool {
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

	r.currCount = r.buckets[r.bucketsIdx]
	if r.positive {
		r.currUpper = getBound(r.currIdx, r.schema)
		r.currLower = getBound(r.currIdx-1, r.schema)
	} else {
		r.currLower = -getBound(r.currIdx, r.schema)
		r.currUpper = -getBound(r.currIdx-1, r.schema)
	}

	r.idxInSpan++
	r.bucketsIdx++
	return true
}

func (r *floatBucketIterator) At() FloatBucket {
	return FloatBucket{
		Count:          r.currCount,
		Lower:          r.currLower,
		Upper:          r.currUpper,
		LowerInclusive: r.currLower < 0,
		UpperInclusive: r.currUpper > 0,
		Index:          r.currIdx,
	}
}

type cumulativeFloatBucketIterator struct {
	h *FloatHistogram

	posSpansIdx   int    // Index in h.PositiveSpans we are in. -1 means 0 bucket.
	posBucketsIdx int    // Index in h.PositiveBuckets.
	idxInSpan     uint32 // Index in the current span. 0 <= idxInSpan < span.Length.

	initialized         bool
	currIdx             int32   // The actual bucket index after decoding from spans.
	currUpper           float64 // The upper boundary of the current bucket.
	currCumulativeCount float64 // Current "cumulative" count for the current bucket.

	// Between 2 spans there could be some empty buckets which
	// still needs to be counted for cumulative buckets.
	// When we hit the end of a span, we use this to iterate
	// through the empty buckets.
	emptyBucketCount int32
}

func (c *cumulativeFloatBucketIterator) Next() bool {
	if c.posSpansIdx == -1 {
		// Zero bucket.
		c.posSpansIdx++
		if c.h.ZeroCount == 0 {
			return c.Next()
		}

		c.currUpper = c.h.ZeroThreshold
		c.currCumulativeCount = c.h.ZeroCount
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
		c.initialized = true
	}

	c.currCumulativeCount += c.h.PositiveBuckets[c.posBucketsIdx]
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

func (c *cumulativeFloatBucketIterator) At() FloatBucket {
	return FloatBucket{
		Upper:          c.currUpper,
		Lower:          math.Inf(-1),
		UpperInclusive: true,
		LowerInclusive: true,
		Count:          c.currCumulativeCount,
		Index:          c.currIdx - 1,
	}
}
