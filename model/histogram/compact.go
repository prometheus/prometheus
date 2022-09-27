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

// compactBuckets is a generic function used by both Histogram.Compact and
// FloatHistogram.Compact. Set deltaBuckets to true if the provided buckets are
// deltas. Set it to false if the buckets contain absolute counts.
func compactBuckets[Bucket float64 | int64](buckets []Bucket, spans []Span, maxEmptyBuckets int, deltaBuckets bool) ([]Bucket, []Span) {
	// Fast path: If there are no empty buckets AND no offset in any span is
	// <= maxEmptyBuckets AND no span has length 0, there is nothing to do and we can return
	// immediately. We check that first because it's cheap and presumably
	// common.
	nothingToDo := true
	var currentBucketAbsolute Bucket
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
		newBuckets := make([]Bucket, len(buckets)+offset)
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
