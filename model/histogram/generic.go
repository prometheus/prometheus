// Copyright 2022 The Prometheus Authors
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
	"strings"
)

// BucketCount is a type constraint for the count in a bucket, which can be
// float64 (for type FloatHistogram) or uint64 (for type Histogram).
type BucketCount interface {
	float64 | uint64
}

// internalBucketCount is used internally by Histogram and FloatHistogram. The
// difference to the BucketCount above is that Histogram internally uses deltas
// between buckets rather than absolute counts (while FloatHistogram uses
// absolute counts directly). Go type parameters don't allow type
// specialization. Therefore, where special treatment of deltas between buckets
// vs. absolute counts is important, this information has to be provided as a
// separate boolean parameter "deltaBuckets"
type internalBucketCount interface {
	float64 | int64
}

// Bucket represents a bucket with lower and upper limit and the absolute count
// of samples in the bucket. It also specifies if each limit is inclusive or
// not. (Mathematically, inclusive limits create a closed interval, and
// non-inclusive limits an open interval.)
//
// To represent cumulative buckets, Lower is set to -Inf, and the Count is then
// cumulative (including the counts of all buckets for smaller values).
type Bucket[BC BucketCount] struct {
	Lower, Upper                   float64
	LowerInclusive, UpperInclusive bool
	Count                          BC

	// Index within schema. To easily compare buckets that share the same
	// schema and sign (positive or negative). Irrelevant for the zero bucket.
	Index int32
}

// String returns a string representation of a Bucket, using the usual
// mathematical notation of '['/']' for inclusive bounds and '('/')' for
// non-inclusive bounds.
func (b Bucket[BC]) String() string {
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
	fmt.Fprintf(&sb, ":%v", b.Count)
	return sb.String()
}

// BucketIterator iterates over the buckets of a Histogram, returning decoded
// buckets.
type BucketIterator[BC BucketCount] interface {
	// Next advances the iterator by one.
	Next() bool
	// At returns the current bucket.
	At() Bucket[BC]
}

// baseBucketIterator provides a struct that is shared by most BucketIterator
// implementations, together with an implementation of the At method. This
// iterator can be embedded in full implementations of BucketIterator to save on
// code replication.
type baseBucketIterator[BC BucketCount, IBC internalBucketCount] struct {
	schema  int32
	spans   []Span
	buckets []IBC

	positive bool // Whether this is for positive buckets.

	spansIdx   int    // Current span within spans slice.
	idxInSpan  uint32 // Index in the current span. 0 <= idxInSpan < span.Length.
	bucketsIdx int    // Current bucket within buckets slice.

	currCount IBC   // Count in the current bucket.
	currIdx   int32 // The actual bucket index.
}

func (b baseBucketIterator[BC, IBC]) At() Bucket[BC] {
	bucket := Bucket[BC]{
		Count: BC(b.currCount),
		Index: b.currIdx,
	}
	if b.positive {
		bucket.Upper = getBound(b.currIdx, b.schema)
		bucket.Lower = getBound(b.currIdx-1, b.schema)
	} else {
		bucket.Lower = -getBound(b.currIdx, b.schema)
		bucket.Upper = -getBound(b.currIdx-1, b.schema)
	}
	bucket.LowerInclusive = bucket.Lower < 0
	bucket.UpperInclusive = bucket.Upper > 0
	return bucket
}

// compactBuckets is a generic function used by both Histogram.Compact and
// FloatHistogram.Compact. Set deltaBuckets to true if the provided buckets are
// deltas. Set it to false if the buckets contain absolute counts.
func compactBuckets[IBC internalBucketCount](buckets []IBC, spans []Span, maxEmptyBuckets int, deltaBuckets bool) ([]IBC, []Span) {
	// Fast path: If there are no empty buckets AND no offset in any span is
	// <= maxEmptyBuckets AND no span has length 0, there is nothing to do and we can return
	// immediately. We check that first because it's cheap and presumably
	// common.
	nothingToDo := true
	var currentBucketAbsolute IBC
	for _, bucket := range buckets {
		if deltaBuckets {
			currentBucketAbsolute += bucket
		} else {
			currentBucketAbsolute = bucket
		}
		if currentBucketAbsolute == 0 {
			nothingToDo = false
			break
		}
	}
	if nothingToDo {
		for _, span := range spans {
			if int(span.Offset) <= maxEmptyBuckets || span.Length == 0 {
				nothingToDo = false
				break
			}
		}
		if nothingToDo {
			return buckets, spans
		}
	}

	var iBucket, iSpan int
	var posInSpan uint32
	currentBucketAbsolute = 0

	// Helper function.
	emptyBucketsHere := func() int {
		i := 0
		abs := currentBucketAbsolute
		for uint32(i)+posInSpan < spans[iSpan].Length && abs == 0 {
			i++
			if i+iBucket >= len(buckets) {
				break
			}
			abs = buckets[i+iBucket]
		}
		return i
	}

	// Merge spans with zero-offset to avoid special cases later.
	if len(spans) > 1 {
		for i, span := range spans[1:] {
			if span.Offset == 0 {
				spans[iSpan].Length += span.Length
				continue
			}
			iSpan++
			if i+1 != iSpan {
				spans[iSpan] = span
			}
		}
		spans = spans[:iSpan+1]
		iSpan = 0
	}

	// Merge spans with zero-length to avoid special cases later.
	for i, span := range spans {
		if span.Length == 0 {
			if i+1 < len(spans) {
				spans[i+1].Offset += span.Offset
			}
			continue
		}
		if i != iSpan {
			spans[iSpan] = span
		}
		iSpan++
	}
	spans = spans[:iSpan]
	iSpan = 0

	// Cut out empty buckets from start and end of spans, no matter
	// what. Also cut out empty buckets from the middle of a span but only
	// if there are more than maxEmptyBuckets consecutive empty buckets.
	for iBucket < len(buckets) {
		if deltaBuckets {
			currentBucketAbsolute += buckets[iBucket]
		} else {
			currentBucketAbsolute = buckets[iBucket]
		}
		if nEmpty := emptyBucketsHere(); nEmpty > 0 {
			if posInSpan > 0 &&
				nEmpty < int(spans[iSpan].Length-posInSpan) &&
				nEmpty <= maxEmptyBuckets {
				// The empty buckets are in the middle of a
				// span, and there are few enough to not bother.
				// Just fast-forward.
				iBucket += nEmpty
				if deltaBuckets {
					currentBucketAbsolute = 0
				}
				posInSpan += uint32(nEmpty)
				continue
			}
			// In all other cases, we cut out the empty buckets.
			if deltaBuckets && iBucket+nEmpty < len(buckets) {
				currentBucketAbsolute = -buckets[iBucket]
				buckets[iBucket+nEmpty] += buckets[iBucket]
			}
			buckets = append(buckets[:iBucket], buckets[iBucket+nEmpty:]...)
			if posInSpan == 0 {
				// Start of span.
				if nEmpty == int(spans[iSpan].Length) {
					// The whole span is empty.
					offset := spans[iSpan].Offset
					spans = append(spans[:iSpan], spans[iSpan+1:]...)
					if len(spans) > iSpan {
						spans[iSpan].Offset += offset + int32(nEmpty)
					}
					continue
				}
				spans[iSpan].Length -= uint32(nEmpty)
				spans[iSpan].Offset += int32(nEmpty)
				continue
			}
			// It's in the middle or in the end of the span.
			// Split the current span.
			newSpan := Span{
				Offset: int32(nEmpty),
				Length: spans[iSpan].Length - posInSpan - uint32(nEmpty),
			}
			spans[iSpan].Length = posInSpan
			// In any case, we have to split to the next span.
			iSpan++
			posInSpan = 0
			if newSpan.Length == 0 {
				// The span is empty, so we were already at the end of a span.
				// We don't have to insert the new span, just adjust the next
				// span's offset, if there is one.
				if iSpan < len(spans) {
					spans[iSpan].Offset += int32(nEmpty)
				}
				continue
			}
			// Insert the new span.
			spans = append(spans, Span{})
			if iSpan+1 < len(spans) {
				copy(spans[iSpan+1:], spans[iSpan:])
			}
			spans[iSpan] = newSpan
			continue
		}
		iBucket++
		posInSpan++
		if posInSpan >= spans[iSpan].Length {
			posInSpan = 0
			iSpan++
		}
	}
	if maxEmptyBuckets == 0 || len(buckets) == 0 {
		return buckets, spans
	}

	// Finally, check if any offsets between spans are small enough to merge
	// the spans.
	iBucket = int(spans[0].Length)
	if deltaBuckets {
		currentBucketAbsolute = 0
		for _, bucket := range buckets[:iBucket] {
			currentBucketAbsolute += bucket
		}
	}
	iSpan = 1
	for iSpan < len(spans) {
		if int(spans[iSpan].Offset) > maxEmptyBuckets {
			l := int(spans[iSpan].Length)
			if deltaBuckets {
				for _, bucket := range buckets[iBucket : iBucket+l] {
					currentBucketAbsolute += bucket
				}
			}
			iBucket += l
			iSpan++
			continue
		}
		// Merge span with previous one and insert empty buckets.
		offset := int(spans[iSpan].Offset)
		spans[iSpan-1].Length += uint32(offset) + spans[iSpan].Length
		spans = append(spans[:iSpan], spans[iSpan+1:]...)
		newBuckets := make([]IBC, len(buckets)+offset)
		copy(newBuckets, buckets[:iBucket])
		copy(newBuckets[iBucket+offset:], buckets[iBucket:])
		if deltaBuckets {
			newBuckets[iBucket] = -currentBucketAbsolute
			newBuckets[iBucket+offset] += currentBucketAbsolute
		}
		iBucket += offset
		buckets = newBuckets
		currentBucketAbsolute = buckets[iBucket]
		// Note that with many merges, it would be more efficient to
		// first record all the chunks of empty buckets to insert and
		// then do it in one go through all the buckets.
	}

	return buckets, spans
}
