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
	// Counter reset information.
	CounterResetHint CounterResetHint
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

// CopyToSchema works like Copy, but the returned deep copy has the provided
// target schema, which must be ≤ the original schema (i.e. it must have a lower
// resolution).
func (h *FloatHistogram) CopyToSchema(targetSchema int32) *FloatHistogram {
	if targetSchema == h.Schema {
		// Fast path.
		return h.Copy()
	}
	if targetSchema > h.Schema {
		panic(fmt.Errorf("cannot copy from schema %d to %d", h.Schema, targetSchema))
	}
	c := FloatHistogram{
		Schema:        targetSchema,
		ZeroThreshold: h.ZeroThreshold,
		ZeroCount:     h.ZeroCount,
		Count:         h.Count,
		Sum:           h.Sum,
	}

	// TODO(beorn7): This is a straight-forward implementation using merging
	// iterators for the original buckets and then adding one merged bucket
	// after another to the newly created FloatHistogram. It's well possible
	// that a more involved implementation performs much better, which we
	// could do if this code path turns out to be performance-critical.
	var iInSpan, index int32
	for iSpan, iBucket, it := -1, -1, h.floatBucketIterator(true, 0, targetSchema); it.Next(); {
		b := it.At()
		c.PositiveSpans, c.PositiveBuckets, iSpan, iBucket, iInSpan = addBucket(
			b, c.PositiveSpans, c.PositiveBuckets, iSpan, iBucket, iInSpan, index,
		)
		index = b.Index
	}
	for iSpan, iBucket, it := -1, -1, h.floatBucketIterator(false, 0, targetSchema); it.Next(); {
		b := it.At()
		c.NegativeSpans, c.NegativeBuckets, iSpan, iBucket, iInSpan = addBucket(
			b, c.NegativeSpans, c.NegativeBuckets, iSpan, iBucket, iInSpan, index,
		)
		index = b.Index
	}

	return &c
}

// String returns a string representation of the Histogram.
func (h *FloatHistogram) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "{count:%g, sum:%g", h.Count, h.Sum)

	var nBuckets []Bucket[float64]
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
func (h *FloatHistogram) ZeroBucket() Bucket[float64] {
	return Bucket[float64]{
		Lower:          -h.ZeroThreshold,
		Upper:          h.ZeroThreshold,
		LowerInclusive: true,
		UpperInclusive: true,
		Count:          h.ZeroCount,
	}
}

// Scale scales the FloatHistogram by the provided factor, i.e. it scales all
// bucket counts including the zero bucket and the count and the sum of
// observations. The bucket layout stays the same. This method changes the
// receiving histogram directly (rather than acting on a copy). It returns a
// pointer to the receiving histogram for convenience.
func (h *FloatHistogram) Scale(factor float64) *FloatHistogram {
	h.ZeroCount *= factor
	h.Count *= factor
	h.Sum *= factor
	for i := range h.PositiveBuckets {
		h.PositiveBuckets[i] *= factor
	}
	for i := range h.NegativeBuckets {
		h.NegativeBuckets[i] *= factor
	}
	return h
}

// Add adds the provided other histogram to the receiving histogram. Count, Sum,
// and buckets from the other histogram are added to the corresponding
// components of the receiving histogram. Buckets in the other histogram that do
// not exist in the receiving histogram are inserted into the latter. The
// resulting histogram might have buckets with a population of zero or directly
// adjacent spans (offset=0). To normalize those, call the Compact method.
//
// The method reconciles differences in the zero threshold and in the schema,
// but the schema of the other histogram must be ≥ the schema of the receiving
// histogram (i.e. must have an equal or higher resolution). This means that the
// schema of the receiving histogram won't change. Its zero threshold, however,
// will change if needed. The other histogram will not be modified in any case.
//
// This method returns a pointer to the receiving histogram for convenience.
func (h *FloatHistogram) Add(other *FloatHistogram) *FloatHistogram {
	otherZeroCount := h.reconcileZeroBuckets(other)
	h.ZeroCount += otherZeroCount
	h.Count += other.Count
	h.Sum += other.Sum

	// TODO(beorn7): If needed, this can be optimized by inspecting the
	// spans in other and create missing buckets in h in batches.
	var iInSpan, index int32
	for iSpan, iBucket, it := -1, -1, other.floatBucketIterator(true, h.ZeroThreshold, h.Schema); it.Next(); {
		b := it.At()
		h.PositiveSpans, h.PositiveBuckets, iSpan, iBucket, iInSpan = addBucket(
			b, h.PositiveSpans, h.PositiveBuckets, iSpan, iBucket, iInSpan, index,
		)
		index = b.Index
	}
	for iSpan, iBucket, it := -1, -1, other.floatBucketIterator(false, h.ZeroThreshold, h.Schema); it.Next(); {
		b := it.At()
		h.NegativeSpans, h.NegativeBuckets, iSpan, iBucket, iInSpan = addBucket(
			b, h.NegativeSpans, h.NegativeBuckets, iSpan, iBucket, iInSpan, index,
		)
		index = b.Index
	}
	return h
}

// Sub works like Add but subtracts the other histogram.
func (h *FloatHistogram) Sub(other *FloatHistogram) *FloatHistogram {
	otherZeroCount := h.reconcileZeroBuckets(other)
	h.ZeroCount -= otherZeroCount
	h.Count -= other.Count
	h.Sum -= other.Sum

	// TODO(beorn7): If needed, this can be optimized by inspecting the
	// spans in other and create missing buckets in h in batches.
	var iInSpan, index int32
	for iSpan, iBucket, it := -1, -1, other.floatBucketIterator(true, h.ZeroThreshold, h.Schema); it.Next(); {
		b := it.At()
		b.Count *= -1
		h.PositiveSpans, h.PositiveBuckets, iSpan, iBucket, iInSpan = addBucket(
			b, h.PositiveSpans, h.PositiveBuckets, iSpan, iBucket, iInSpan, index,
		)
		index = b.Index
	}
	for iSpan, iBucket, it := -1, -1, other.floatBucketIterator(false, h.ZeroThreshold, h.Schema); it.Next(); {
		b := it.At()
		b.Count *= -1
		h.NegativeSpans, h.NegativeBuckets, iSpan, iBucket, iInSpan = addBucket(
			b, h.NegativeSpans, h.NegativeBuckets, iSpan, iBucket, iInSpan, index,
		)
		index = b.Index
	}
	return h
}

// Equals returns true if the given float histogram matches exactly.
// Exact match is when there are no new buckets (even empty) and no missing buckets,
// and all the bucket values match. Spans can have different empty length spans in between,
// but they must represent the same bucket layout to match.
func (h *FloatHistogram) Equals(h2 *FloatHistogram) bool {
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

// addBucket takes the "coordinates" of the last bucket that was handled and
// adds the provided bucket after it. If a corresponding bucket exists, the
// count is added. If not, the bucket is inserted. The updated slices and the
// coordinates of the inserted or added-to bucket are returned.
func addBucket(
	b Bucket[float64],
	spans []Span, buckets []float64,
	iSpan, iBucket int,
	iInSpan, index int32,
) (
	newSpans []Span, newBuckets []float64,
	newISpan, newIBucket int, newIInSpan int32,
) {
	if iSpan == -1 {
		// First add, check if it is before all spans.
		if len(spans) == 0 || spans[0].Offset > b.Index {
			// Add bucket before all others.
			buckets = append(buckets, 0)
			copy(buckets[1:], buckets)
			buckets[0] = b.Count
			if len(spans) > 0 && spans[0].Offset == b.Index+1 {
				spans[0].Length++
				spans[0].Offset--
				return spans, buckets, 0, 0, 0
			}
			spans = append(spans, Span{})
			copy(spans[1:], spans)
			spans[0] = Span{Offset: b.Index, Length: 1}
			if len(spans) > 1 {
				// Convert the absolute offset in the formerly
				// first span to a relative offset.
				spans[1].Offset -= b.Index + 1
			}
			return spans, buckets, 0, 0, 0
		}
		if spans[0].Offset == b.Index {
			// Just add to first bucket.
			buckets[0] += b.Count
			return spans, buckets, 0, 0, 0
		}
		// We are behind the first bucket, so set everything to the
		// first bucket and continue normally.
		iSpan, iBucket, iInSpan = 0, 0, 0
		index = spans[0].Offset
	}
	deltaIndex := b.Index - index
	for {
		remainingInSpan := int32(spans[iSpan].Length) - iInSpan
		if deltaIndex < remainingInSpan {
			// Bucket is in current span.
			iBucket += int(deltaIndex)
			iInSpan += deltaIndex
			buckets[iBucket] += b.Count
			return spans, buckets, iSpan, iBucket, iInSpan
		}
		deltaIndex -= remainingInSpan
		iBucket += int(remainingInSpan)
		iSpan++
		if iSpan == len(spans) || deltaIndex < spans[iSpan].Offset {
			// Bucket is in gap behind previous span (or there are no further spans).
			buckets = append(buckets, 0)
			copy(buckets[iBucket+1:], buckets[iBucket:])
			buckets[iBucket] = b.Count
			if deltaIndex == 0 {
				// Directly after previous span, extend previous span.
				if iSpan < len(spans) {
					spans[iSpan].Offset--
				}
				iSpan--
				iInSpan = int32(spans[iSpan].Length)
				spans[iSpan].Length++
				return spans, buckets, iSpan, iBucket, iInSpan
			}
			if iSpan < len(spans) && deltaIndex == spans[iSpan].Offset-1 {
				// Directly before next span, extend next span.
				iInSpan = 0
				spans[iSpan].Offset--
				spans[iSpan].Length++
				return spans, buckets, iSpan, iBucket, iInSpan
			}
			// No next span, or next span is not directly adjacent to new bucket.
			// Add new span.
			iInSpan = 0
			if iSpan < len(spans) {
				spans[iSpan].Offset -= deltaIndex + 1
			}
			spans = append(spans, Span{})
			copy(spans[iSpan+1:], spans[iSpan:])
			spans[iSpan] = Span{Length: 1, Offset: deltaIndex}
			return spans, buckets, iSpan, iBucket, iInSpan
		}
		// Try start of next span.
		deltaIndex -= spans[iSpan].Offset
		iInSpan = 0
	}
}

// Compact eliminates empty buckets at the beginning and end of each span, then
// merges spans that are consecutive or at most maxEmptyBuckets apart, and
// finally splits spans that contain more consecutive empty buckets than
// maxEmptyBuckets. (The actual implementation might do something more efficient
// but with the same result.)  The compaction happens "in place" in the
// receiving histogram, but a pointer to it is returned for convenience.
//
// The ideal value for maxEmptyBuckets depends on circumstances. The motivation
// to set maxEmptyBuckets > 0 is the assumption that is is less overhead to
// represent very few empty buckets explicitly within one span than cutting the
// one span into two to treat the empty buckets as a gap between the two spans,
// both in terms of storage requirement as well as in terms of encoding and
// decoding effort. However, the tradeoffs are subtle. For one, they are
// different in the exposition format vs. in a TSDB chunk vs. for the in-memory
// representation as Go types. In the TSDB, as an additional aspects, the span
// layout is only stored once per chunk, while many histograms with that same
// chunk layout are then only stored with their buckets (so that even a single
// empty bucket will be stored many times).
//
// For the Go types, an additional Span takes 8 bytes. Similarly, an additional
// bucket takes 8 bytes. Therefore, with a single separating empty bucket, both
// options have the same storage requirement, but the single-span solution is
// easier to iterate through. Still, the safest bet is to use maxEmptyBuckets==0
// and only use a larger number if you know what you are doing.
func (h *FloatHistogram) Compact(maxEmptyBuckets int) *FloatHistogram {
	h.PositiveBuckets, h.PositiveSpans = compactBuckets(
		h.PositiveBuckets, h.PositiveSpans, maxEmptyBuckets, false,
	)
	h.NegativeBuckets, h.NegativeSpans = compactBuckets(
		h.NegativeBuckets, h.NegativeSpans, maxEmptyBuckets, false,
	)
	return h
}

// DetectReset returns true if the receiving histogram is missing any buckets
// that have a non-zero population in the provided previous histogram. It also
// returns true if any count (in any bucket, in the zero count, or in the count
// of observations, but NOT the sum of observations) is smaller in the receiving
// histogram compared to the previous histogram. Otherwise, it returns false.
//
// Special behavior in case the Schema or the ZeroThreshold are not the same in
// both histograms:
//
//   - A decrease of the ZeroThreshold or an increase of the Schema (i.e. an
//     increase of resolution) can only happen together with a reset. Thus, the
//     method returns true in either case.
//
//   - Upon an increase of the ZeroThreshold, the buckets in the previous
//     histogram that fall within the new ZeroThreshold are added to the ZeroCount
//     of the previous histogram (without mutating the provided previous
//     histogram). The scenario that a populated bucket of the previous histogram
//     is partially within, partially outside of the new ZeroThreshold, can only
//     happen together with a counter reset and therefore shortcuts to returning
//     true.
//
//   - Upon a decrease of the Schema, the buckets of the previous histogram are
//     merged so that they match the new, lower-resolution schema (again without
//     mutating the provided previous histogram).
//
// Note that this kind of reset detection is quite expensive. Ideally, resets
// are detected at ingest time and stored in the TSDB, so that the reset
// information can be read directly from there rather than be detected each time
// again.
func (h *FloatHistogram) DetectReset(previous *FloatHistogram) bool {
	if h.Count < previous.Count {
		return true
	}
	if h.Schema > previous.Schema {
		return true
	}
	if h.ZeroThreshold < previous.ZeroThreshold {
		// ZeroThreshold decreased.
		return true
	}
	previousZeroCount, newThreshold := previous.zeroCountForLargerThreshold(h.ZeroThreshold)
	if newThreshold != h.ZeroThreshold {
		// ZeroThreshold is within a populated bucket in previous
		// histogram.
		return true
	}
	if h.ZeroCount < previousZeroCount {
		return true
	}
	currIt := h.floatBucketIterator(true, h.ZeroThreshold, h.Schema)
	prevIt := previous.floatBucketIterator(true, h.ZeroThreshold, h.Schema)
	if detectReset(currIt, prevIt) {
		return true
	}
	currIt = h.floatBucketIterator(false, h.ZeroThreshold, h.Schema)
	prevIt = previous.floatBucketIterator(false, h.ZeroThreshold, h.Schema)
	return detectReset(currIt, prevIt)
}

func detectReset(currIt, prevIt BucketIterator[float64]) bool {
	if !prevIt.Next() {
		return false // If no buckets in previous histogram, nothing can be reset.
	}
	prevBucket := prevIt.At()
	if !currIt.Next() {
		// No bucket in current, but at least one in previous
		// histogram. Check if any of those are non-zero, in which case
		// this is a reset.
		for {
			if prevBucket.Count != 0 {
				return true
			}
			if !prevIt.Next() {
				return false
			}
		}
	}
	currBucket := currIt.At()
	for {
		// Forward currIt until we find the bucket corresponding to prevBucket.
		for currBucket.Index < prevBucket.Index {
			if !currIt.Next() {
				// Reached end of currIt early, therefore
				// previous histogram has a bucket that the
				// current one does not have. Unlass all
				// remaining buckets in the previous histogram
				// are unpopulated, this is a reset.
				for {
					if prevBucket.Count != 0 {
						return true
					}
					if !prevIt.Next() {
						return false
					}
				}
			}
			currBucket = currIt.At()
		}
		if currBucket.Index > prevBucket.Index {
			// Previous histogram has a bucket the current one does
			// not have. If it's populated, it's a reset.
			if prevBucket.Count != 0 {
				return true
			}
		} else {
			// We have reached corresponding buckets in both iterators.
			// We can finally compare the counts.
			if currBucket.Count < prevBucket.Count {
				return true
			}
		}
		if !prevIt.Next() {
			// Reached end of prevIt without finding offending buckets.
			return false
		}
		prevBucket = prevIt.At()
	}
}

// PositiveBucketIterator returns a BucketIterator to iterate over all positive
// buckets in ascending order (starting next to the zero bucket and going up).
func (h *FloatHistogram) PositiveBucketIterator() BucketIterator[float64] {
	return h.floatBucketIterator(true, 0, h.Schema)
}

// NegativeBucketIterator returns a BucketIterator to iterate over all negative
// buckets in descending order (starting next to the zero bucket and going
// down).
func (h *FloatHistogram) NegativeBucketIterator() BucketIterator[float64] {
	return h.floatBucketIterator(false, 0, h.Schema)
}

// PositiveReverseBucketIterator returns a BucketIterator to iterate over all
// positive buckets in descending order (starting at the highest bucket and
// going down towards the zero bucket).
func (h *FloatHistogram) PositiveReverseBucketIterator() BucketIterator[float64] {
	return newReverseFloatBucketIterator(h.PositiveSpans, h.PositiveBuckets, h.Schema, true)
}

// NegativeReverseBucketIterator returns a BucketIterator to iterate over all
// negative buckets in ascending order (starting at the lowest bucket and going
// up towards the zero bucket).
func (h *FloatHistogram) NegativeReverseBucketIterator() BucketIterator[float64] {
	return newReverseFloatBucketIterator(h.NegativeSpans, h.NegativeBuckets, h.Schema, false)
}

// AllBucketIterator returns a BucketIterator to iterate over all negative,
// zero, and positive buckets in ascending order (starting at the lowest bucket
// and going up). If the highest negative bucket or the lowest positive bucket
// overlap with the zero bucket, their upper or lower boundary, respectively, is
// set to the zero threshold.
func (h *FloatHistogram) AllBucketIterator() BucketIterator[float64] {
	return &allFloatBucketIterator{
		h:       h,
		negIter: h.NegativeReverseBucketIterator(),
		posIter: h.PositiveBucketIterator(),
		state:   -1,
	}
}

// zeroCountForLargerThreshold returns what the histogram's zero count would be
// if the ZeroThreshold had the provided larger (or equal) value. If the
// provided value is less than the histogram's ZeroThreshold, the method panics.
// If the largerThreshold ends up within a populated bucket of the histogram, it
// is adjusted upwards to the lower limit of that bucket (all in terms of
// absolute values) and that bucket's count is included in the returned
// count. The adjusted threshold is returned, too.
func (h *FloatHistogram) zeroCountForLargerThreshold(largerThreshold float64) (count, threshold float64) {
	// Fast path.
	if largerThreshold == h.ZeroThreshold {
		return h.ZeroCount, largerThreshold
	}
	if largerThreshold < h.ZeroThreshold {
		panic(fmt.Errorf("new threshold %f is less than old threshold %f", largerThreshold, h.ZeroThreshold))
	}
outer:
	for {
		count = h.ZeroCount
		i := h.PositiveBucketIterator()
		for i.Next() {
			b := i.At()
			if b.Lower >= largerThreshold {
				break
			}
			count += b.Count // Bucket to be merged into zero bucket.
			if b.Upper > largerThreshold {
				// New threshold ended up within a bucket. if it's
				// populated, we need to adjust largerThreshold before
				// we are done here.
				if b.Count != 0 {
					largerThreshold = b.Upper
				}
				break
			}
		}
		i = h.NegativeBucketIterator()
		for i.Next() {
			b := i.At()
			if b.Upper <= -largerThreshold {
				break
			}
			count += b.Count // Bucket to be merged into zero bucket.
			if b.Lower < -largerThreshold {
				// New threshold ended up within a bucket. If
				// it's populated, we need to adjust
				// largerThreshold and have to redo the whole
				// thing because the treatment of the positive
				// buckets is invalid now.
				if b.Count != 0 {
					largerThreshold = -b.Lower
					continue outer
				}
				break
			}
		}
		return count, largerThreshold
	}
}

// trimBucketsInZeroBucket removes all buckets that are within the zero
// bucket. It assumes that the zero threshold is at a bucket boundary and that
// the counts in the buckets to remove are already part of the zero count.
func (h *FloatHistogram) trimBucketsInZeroBucket() {
	i := h.PositiveBucketIterator()
	bucketsIdx := 0
	for i.Next() {
		b := i.At()
		if b.Lower >= h.ZeroThreshold {
			break
		}
		h.PositiveBuckets[bucketsIdx] = 0
		bucketsIdx++
	}
	i = h.NegativeBucketIterator()
	bucketsIdx = 0
	for i.Next() {
		b := i.At()
		if b.Upper <= -h.ZeroThreshold {
			break
		}
		h.NegativeBuckets[bucketsIdx] = 0
		bucketsIdx++
	}
	// We are abusing Compact to trim the buckets set to zero
	// above. Premature compacting could cause additional cost, but this
	// code path is probably rarely used anyway.
	h.Compact(0)
}

// reconcileZeroBuckets finds a zero bucket large enough to include the zero
// buckets of both histograms (the receiving histogram and the other histogram)
// with a zero threshold that is not within a populated bucket in either
// histogram. This method modifies the receiving histogram accourdingly, but
// leaves the other histogram as is. Instead, it returns the zero count the
// other histogram would have if it were modified.
func (h *FloatHistogram) reconcileZeroBuckets(other *FloatHistogram) float64 {
	otherZeroCount := other.ZeroCount
	otherZeroThreshold := other.ZeroThreshold

	for otherZeroThreshold != h.ZeroThreshold {
		if h.ZeroThreshold > otherZeroThreshold {
			otherZeroCount, otherZeroThreshold = other.zeroCountForLargerThreshold(h.ZeroThreshold)
		}
		if otherZeroThreshold > h.ZeroThreshold {
			h.ZeroCount, h.ZeroThreshold = h.zeroCountForLargerThreshold(otherZeroThreshold)
			h.trimBucketsInZeroBucket()
		}
	}
	return otherZeroCount
}

// floatBucketIterator is a low-level constructor for bucket iterators.
//
// If positive is true, the returned iterator iterates through the positive
// buckets, otherwise through the negative buckets.
//
// If absoluteStartValue is < the lowest absolute value of any upper bucket
// boundary, the iterator starts with the first bucket. Otherwise, it will skip
// all buckets with an absolute value of their upper boundary ≤
// absoluteStartValue.
//
// targetSchema must be ≤ the schema of FloatHistogram (and of course within the
// legal values for schemas in general). The buckets are merged to match the
// targetSchema prior to iterating (without mutating FloatHistogram).
func (h *FloatHistogram) floatBucketIterator(
	positive bool, absoluteStartValue float64, targetSchema int32,
) *floatBucketIterator {
	if targetSchema > h.Schema {
		panic(fmt.Errorf("cannot merge from schema %d to %d", h.Schema, targetSchema))
	}
	i := &floatBucketIterator{
		baseBucketIterator: baseBucketIterator[float64, float64]{
			schema:   h.Schema,
			positive: positive,
		},
		targetSchema:       targetSchema,
		absoluteStartValue: absoluteStartValue,
	}
	if positive {
		i.spans = h.PositiveSpans
		i.buckets = h.PositiveBuckets
	} else {
		i.spans = h.NegativeSpans
		i.buckets = h.NegativeBuckets
	}
	return i
}

// reverseFloatbucketiterator is a low-level constructor for reverse bucket iterators.
func newReverseFloatBucketIterator(
	spans []Span, buckets []float64, schema int32, positive bool,
) *reverseFloatBucketIterator {
	r := &reverseFloatBucketIterator{
		baseBucketIterator: baseBucketIterator[float64, float64]{
			schema:   schema,
			spans:    spans,
			buckets:  buckets,
			positive: positive,
		},
	}

	r.spansIdx = len(r.spans) - 1
	r.bucketsIdx = len(r.buckets) - 1
	if r.spansIdx >= 0 {
		r.idxInSpan = int32(r.spans[r.spansIdx].Length) - 1
	}
	r.currIdx = 0
	for _, s := range r.spans {
		r.currIdx += s.Offset + int32(s.Length)
	}

	return r
}

type floatBucketIterator struct {
	baseBucketIterator[float64, float64]

	targetSchema       int32   // targetSchema is the schema to merge to and must be ≤ schema.
	origIdx            int32   // The bucket index within the original schema.
	absoluteStartValue float64 // Never return buckets with an upper bound ≤ this value.
}

func (i *floatBucketIterator) Next() bool {
	if i.spansIdx >= len(i.spans) {
		return false
	}

	// Copy all of these into local variables so that we can forward to the
	// next bucket and then roll back if needed.
	origIdx, spansIdx, idxInSpan := i.origIdx, i.spansIdx, i.idxInSpan
	span := i.spans[spansIdx]
	firstPass := true
	i.currCount = 0

mergeLoop: // Merge together all buckets from the original schema that fall into one bucket in the targetSchema.
	for {
		if i.bucketsIdx == 0 {
			// Seed origIdx for the first bucket.
			origIdx = span.Offset
		} else {
			origIdx++
		}
		for idxInSpan >= span.Length {
			// We have exhausted the current span and have to find a new
			// one. We even handle pathologic spans of length 0 here.
			idxInSpan = 0
			spansIdx++
			if spansIdx >= len(i.spans) {
				if firstPass {
					return false
				}
				break mergeLoop
			}
			span = i.spans[spansIdx]
			origIdx += span.Offset
		}
		currIdx := i.targetIdx(origIdx)
		if firstPass {
			i.currIdx = currIdx
			firstPass = false
		} else if currIdx != i.currIdx {
			// Reached next bucket in targetSchema.
			// Do not actually forward to the next bucket, but break out.
			break mergeLoop
		}
		i.currCount += i.buckets[i.bucketsIdx]
		idxInSpan++
		i.bucketsIdx++
		i.origIdx, i.spansIdx, i.idxInSpan = origIdx, spansIdx, idxInSpan
		if i.schema == i.targetSchema {
			// Don't need to test the next bucket for mergeability
			// if we have no schema change anyway.
			break mergeLoop
		}
	}
	// Skip buckets before absoluteStartValue.
	// TODO(beorn7): Maybe do something more efficient than this recursive call.
	if getBound(i.currIdx, i.targetSchema) <= i.absoluteStartValue {
		return i.Next()
	}
	return true
}

// targetIdx returns the bucket index within i.targetSchema for the given bucket
// index within i.schema.
func (i *floatBucketIterator) targetIdx(idx int32) int32 {
	if i.schema == i.targetSchema {
		// Fast path for the common case. The below would yield the same
		// result, just with more effort.
		return idx
	}
	return ((idx - 1) >> (i.schema - i.targetSchema)) + 1
}

type reverseFloatBucketIterator struct {
	baseBucketIterator[float64, float64]
	idxInSpan int32 // Changed from uint32 to allow negative values for exhaustion detection.
}

func (i *reverseFloatBucketIterator) Next() bool {
	i.currIdx--
	if i.bucketsIdx < 0 {
		return false
	}

	for i.idxInSpan < 0 {
		// We have exhausted the current span and have to find a new
		// one. We'll even handle pathologic spans of length 0.
		i.spansIdx--
		i.idxInSpan = int32(i.spans[i.spansIdx].Length) - 1
		i.currIdx -= i.spans[i.spansIdx+1].Offset
	}

	i.currCount = i.buckets[i.bucketsIdx]
	i.bucketsIdx--
	i.idxInSpan--
	return true
}

type allFloatBucketIterator struct {
	h                *FloatHistogram
	negIter, posIter BucketIterator[float64]
	// -1 means we are iterating negative buckets.
	// 0 means it is time for the zero bucket.
	// 1 means we are iterating positive buckets.
	// Anything else means iteration is over.
	state      int8
	currBucket Bucket[float64]
}

func (i *allFloatBucketIterator) Next() bool {
	switch i.state {
	case -1:
		if i.negIter.Next() {
			i.currBucket = i.negIter.At()
			if i.currBucket.Upper > -i.h.ZeroThreshold {
				i.currBucket.Upper = -i.h.ZeroThreshold
			}
			return true
		}
		i.state = 0
		return i.Next()
	case 0:
		i.state = 1
		if i.h.ZeroCount > 0 {
			i.currBucket = Bucket[float64]{
				Lower:          -i.h.ZeroThreshold,
				Upper:          i.h.ZeroThreshold,
				LowerInclusive: true,
				UpperInclusive: true,
				Count:          i.h.ZeroCount,
				// Index is irrelevant for the zero bucket.
			}
			return true
		}
		return i.Next()
	case 1:
		if i.posIter.Next() {
			i.currBucket = i.posIter.At()
			if i.currBucket.Lower < i.h.ZeroThreshold {
				i.currBucket.Lower = i.h.ZeroThreshold
			}
			return true
		}
		i.state = 42
		return false
	}

	return false
}

func (i *allFloatBucketIterator) At() Bucket[float64] {
	return i.currBucket
}
