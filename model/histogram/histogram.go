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

package histogram

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// CounterResetHint contains the known information about a counter reset,
// or alternatively that we are dealing with a gauge histogram, where counter resets do not apply.
type CounterResetHint byte

const (
	UnknownCounterReset CounterResetHint = iota // UnknownCounterReset means we cannot say if this histogram signals a counter reset or not.
	CounterReset                                // CounterReset means there was definitely a counter reset starting from this histogram.
	NotCounterReset                             // NotCounterReset means there was definitely no counter reset with this histogram.
	GaugeType                                   // GaugeType means this is a gauge histogram, where counter resets do not happen.
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
	// Counter reset information.
	CounterResetHint CounterResetHint
	// Currently valid schema numbers are -4 <= n <= 8 for exponential buckets,
	// They are all for base-2 bucket schemas, where 1 is a bucket boundary in
	// each case, and then each power of two is divided into 2^n logarithmic buckets.
	// Or in other words, each bucket boundary is the previous boundary times
	// 2^(2^-n). Another valid schema number is -53 for custom buckets, defined by
	// the CustomValues field.
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
	// Holds the custom (usually upper) bounds for bucket definitions, otherwise nil.
	// This slice is interned, to be treated as immutable and copied by reference.
	// These numbers should be strictly increasing. This field is only used when the
	// schema is for custom buckets, and the ZeroThreshold, ZeroCount, NegativeSpans
	// and NegativeBuckets fields are not used in that case.
	CustomValues []float64
}

// A Span defines a continuous sequence of buckets.
type Span struct {
	// Gap to previous span (always positive), or starting index for the 1st
	// span (which can be negative).
	Offset int32
	// Length of the span.
	Length uint32
}

func (h *Histogram) UsesCustomBuckets() bool {
	return IsCustomBucketsSchema(h.Schema)
}

// Copy returns a deep copy of the Histogram.
func (h *Histogram) Copy() *Histogram {
	c := Histogram{
		CounterResetHint: h.CounterResetHint,
		Schema:           h.Schema,
		Count:            h.Count,
		Sum:              h.Sum,
	}

	if h.UsesCustomBuckets() {
		// Custom values are interned, it's ok to copy by reference.
		c.CustomValues = h.CustomValues
	} else {
		c.ZeroThreshold = h.ZeroThreshold
		c.ZeroCount = h.ZeroCount

		if len(h.NegativeSpans) != 0 {
			c.NegativeSpans = make([]Span, len(h.NegativeSpans))
			copy(c.NegativeSpans, h.NegativeSpans)
		}
		if len(h.NegativeBuckets) != 0 {
			c.NegativeBuckets = make([]int64, len(h.NegativeBuckets))
			copy(c.NegativeBuckets, h.NegativeBuckets)
		}
	}

	if len(h.PositiveSpans) != 0 {
		c.PositiveSpans = make([]Span, len(h.PositiveSpans))
		copy(c.PositiveSpans, h.PositiveSpans)
	}
	if len(h.PositiveBuckets) != 0 {
		c.PositiveBuckets = make([]int64, len(h.PositiveBuckets))
		copy(c.PositiveBuckets, h.PositiveBuckets)
	}

	return &c
}

// CopyTo makes a deep copy into the given Histogram object.
// The destination object has to be a non-nil pointer.
func (h *Histogram) CopyTo(to *Histogram) {
	to.CounterResetHint = h.CounterResetHint
	to.Schema = h.Schema
	to.Count = h.Count
	to.Sum = h.Sum

	if h.UsesCustomBuckets() {
		to.ZeroThreshold = 0
		to.ZeroCount = 0

		to.NegativeSpans = clearIfNotNil(to.NegativeSpans)
		to.NegativeBuckets = clearIfNotNil(to.NegativeBuckets)
		// Custom values are interned, it's ok to copy by reference.
		to.CustomValues = h.CustomValues
	} else {
		to.ZeroThreshold = h.ZeroThreshold
		to.ZeroCount = h.ZeroCount

		to.NegativeSpans = resize(to.NegativeSpans, len(h.NegativeSpans))
		copy(to.NegativeSpans, h.NegativeSpans)

		to.NegativeBuckets = resize(to.NegativeBuckets, len(h.NegativeBuckets))
		copy(to.NegativeBuckets, h.NegativeBuckets)
		// Custom values are interned, no need to reset.
		to.CustomValues = nil
	}

	to.PositiveSpans = resize(to.PositiveSpans, len(h.PositiveSpans))
	copy(to.PositiveSpans, h.PositiveSpans)

	to.PositiveBuckets = resize(to.PositiveBuckets, len(h.PositiveBuckets))
	copy(to.PositiveBuckets, h.PositiveBuckets)
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

// ZeroBucket returns the zero bucket. This method panics if the schema is for custom buckets.
func (h *Histogram) ZeroBucket() Bucket[uint64] {
	if h.UsesCustomBuckets() {
		panic("histograms with custom buckets have no zero bucket")
	}
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
	it := newRegularBucketIterator(h.PositiveSpans, h.PositiveBuckets, h.Schema, true, h.CustomValues)
	return &it
}

// NegativeBucketIterator returns a BucketIterator to iterate over all negative
// buckets in descending order (starting next to the zero bucket and going down).
func (h *Histogram) NegativeBucketIterator() BucketIterator[uint64] {
	it := newRegularBucketIterator(h.NegativeSpans, h.NegativeBuckets, h.Schema, false, nil)
	return &it
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
// Sum is compared based on its bit pattern because this method
// is about data equality rather than mathematical equality.
// We ignore fields that are not used based on the exponential / custom buckets schema,
// but check fields where differences may cause unintended behaviour even if they are not
// supposed to be used according to the schema.
func (h *Histogram) Equals(h2 *Histogram) bool {
	if h2 == nil {
		return h == nil
	}

	if h.Schema != h2.Schema || h.Count != h2.Count ||
		math.Float64bits(h.Sum) != math.Float64bits(h2.Sum) {
		return false
	}

	if h.UsesCustomBuckets() {
		if !CustomBucketBoundsMatch(h.CustomValues, h2.CustomValues) {
			return false
		}
	}

	if h.ZeroThreshold != h2.ZeroThreshold || h.ZeroCount != h2.ZeroCount {
		return false
	}

	if !spansMatch(h.NegativeSpans, h2.NegativeSpans) {
		return false
	}
	if !slices.Equal(h.NegativeBuckets, h2.NegativeBuckets) {
		return false
	}

	if !spansMatch(h.PositiveSpans, h2.PositiveSpans) {
		return false
	}
	if !slices.Equal(h.PositiveBuckets, h2.PositiveBuckets) {
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

// Compact works like FloatHistogram.Compact. See there for detailed
// explanations.
func (h *Histogram) Compact(maxEmptyBuckets int) *Histogram {
	h.PositiveBuckets, _, h.PositiveSpans = compactBuckets(
		h.PositiveBuckets, nil, h.PositiveSpans, maxEmptyBuckets, true,
	)
	h.NegativeBuckets, _, h.NegativeSpans = compactBuckets(
		h.NegativeBuckets, nil, h.NegativeSpans, maxEmptyBuckets, true,
	)
	return h
}

// ToFloat returns a FloatHistogram representation of the Histogram. It is a deep
// copy (e.g. spans are not shared). The function accepts a FloatHistogram as an
// argument whose memory will be reused and overwritten if provided. If this
// argument is nil, a new FloatHistogram will be allocated.
func (h *Histogram) ToFloat(fh *FloatHistogram) *FloatHistogram {
	if fh == nil {
		fh = &FloatHistogram{}
	}
	fh.CounterResetHint = h.CounterResetHint
	fh.Schema = h.Schema
	fh.Count = float64(h.Count)
	fh.Sum = h.Sum

	if h.UsesCustomBuckets() {
		fh.ZeroThreshold = 0
		fh.ZeroCount = 0
		fh.NegativeSpans = clearIfNotNil(fh.NegativeSpans)
		fh.NegativeBuckets = clearIfNotNil(fh.NegativeBuckets)
		// Custom values are interned, it's ok to copy by reference.
		fh.CustomValues = h.CustomValues
	} else {
		fh.ZeroThreshold = h.ZeroThreshold
		fh.ZeroCount = float64(h.ZeroCount)

		fh.NegativeSpans = resize(fh.NegativeSpans, len(h.NegativeSpans))
		copy(fh.NegativeSpans, h.NegativeSpans)

		fh.NegativeBuckets = resize(fh.NegativeBuckets, len(h.NegativeBuckets))
		var currentNegative float64
		for i, b := range h.NegativeBuckets {
			currentNegative += float64(b)
			fh.NegativeBuckets[i] = currentNegative
		}
		// Custom values are interned, no need to reset.
		fh.CustomValues = nil
	}

	fh.PositiveSpans = resize(fh.PositiveSpans, len(h.PositiveSpans))
	copy(fh.PositiveSpans, h.PositiveSpans)

	fh.PositiveBuckets = resize(fh.PositiveBuckets, len(h.PositiveBuckets))
	var currentPositive float64
	for i, b := range h.PositiveBuckets {
		currentPositive += float64(b)
		fh.PositiveBuckets[i] = currentPositive
	}

	return fh
}

func resize[T any](items []T, n int) []T {
	if cap(items) < n {
		return make([]T, n)
	}
	return items[:n]
}

// Validate validates consistency between span and bucket slices. Also, buckets are checked
// against negative values. We check to make sure there are no unexpected fields or field values
// based on the exponential / custom buckets schema.
// For histograms that have not observed any NaN values (based on IsNaN(h.Sum) check), a
// strict h.Count = nCount + pCount + h.ZeroCount check is performed.
// Otherwise, only a lower bound check will be done (h.Count >= nCount + pCount + h.ZeroCount),
// because NaN observations do not increment the values of buckets (but they do increment
// the total h.Count).
func (h *Histogram) Validate() error {
	var nCount, pCount uint64
	switch {
	case IsCustomBucketsSchema(h.Schema):
		if err := checkHistogramCustomBounds(h.CustomValues, h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
			return fmt.Errorf("custom buckets: %w", err)
		}
		if h.ZeroCount != 0 {
			return ErrHistogramCustomBucketsZeroCount
		}
		if h.ZeroThreshold != 0 {
			return ErrHistogramCustomBucketsZeroThresh
		}
		if len(h.NegativeSpans) > 0 {
			return ErrHistogramCustomBucketsNegSpans
		}
		if len(h.NegativeBuckets) > 0 {
			return ErrHistogramCustomBucketsNegBuckets
		}
	case IsExponentialSchema(h.Schema):
		if err := checkHistogramSpans(h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
			return fmt.Errorf("positive side: %w", err)
		}
		if err := checkHistogramSpans(h.NegativeSpans, len(h.NegativeBuckets)); err != nil {
			return fmt.Errorf("negative side: %w", err)
		}
		err := checkHistogramBuckets(h.NegativeBuckets, &nCount, true)
		if err != nil {
			return fmt.Errorf("negative side: %w", err)
		}
		if h.CustomValues != nil {
			return ErrHistogramExpSchemaCustomBounds
		}
	default:
		return InvalidSchemaError(h.Schema)
	}
	err := checkHistogramBuckets(h.PositiveBuckets, &pCount, true)
	if err != nil {
		return fmt.Errorf("positive side: %w", err)
	}

	sumOfBuckets := nCount + pCount + h.ZeroCount
	if math.IsNaN(h.Sum) {
		if sumOfBuckets > h.Count {
			return fmt.Errorf("%d observations found in buckets, but the Count field is %d: %w", sumOfBuckets, h.Count, ErrHistogramCountNotBigEnough)
		}
	} else {
		if sumOfBuckets != h.Count {
			return fmt.Errorf("%d observations found in buckets, but the Count field is %d: %w", sumOfBuckets, h.Count, ErrHistogramCountMismatch)
		}
	}

	return nil
}

type regularBucketIterator struct {
	baseBucketIterator[uint64, int64]
}

func newRegularBucketIterator(spans []Span, buckets []int64, schema int32, positive bool, customValues []float64) regularBucketIterator {
	i := baseBucketIterator[uint64, int64]{
		schema:       schema,
		spans:        spans,
		buckets:      buckets,
		positive:     positive,
		customValues: customValues,
	}
	return regularBucketIterator{i}
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

	// This protects against index out of range panic, which
	// can only happen with an invalid histogram.
	if r.bucketsIdx >= len(r.buckets) {
		return false
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
		c.currUpper = getBound(c.currIdx, c.h.Schema, c.h.CustomValues)
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

	// This protects against index out of range panic, which
	// can only happen with an invalid histogram.
	if c.posBucketsIdx >= len(c.h.PositiveBuckets) {
		return false
	}
	c.currCount += c.h.PositiveBuckets[c.posBucketsIdx]
	c.currCumulativeCount += uint64(c.currCount)
	c.currUpper = getBound(c.currIdx, c.h.Schema, c.h.CustomValues)

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

// ReduceResolution reduces the histogram's spans, buckets into target schema.
// An error is returned in the following cases:
//   - The target schema is not smaller than the current histogram's schema.
//   - The histogram has custom buckets.
//   - The target schema is a custom buckets schema.
//   - Any spans have an invalid offset.
//   - The spans are inconsistent with the number of buckets.
func (h *Histogram) ReduceResolution(targetSchema int32) error {
	// Note that the follow three returns are not returning a
	// histogram.Error because they are programming errors.
	if h.UsesCustomBuckets() {
		return errors.New("cannot reduce resolution when there are custom buckets")
	}
	if IsCustomBucketsSchema(targetSchema) {
		return errors.New("cannot reduce resolution to custom buckets schema")
	}
	if targetSchema >= h.Schema {
		return fmt.Errorf("cannot reduce resolution from schema %d to %d", h.Schema, targetSchema)
	}

	var err error

	if h.PositiveSpans, h.PositiveBuckets, err = reduceResolution(
		h.PositiveSpans, h.PositiveBuckets, h.Schema, targetSchema, true, true,
	); err != nil {
		return err
	}
	if h.NegativeSpans, h.NegativeBuckets, err = reduceResolution(
		h.NegativeSpans, h.NegativeBuckets, h.Schema, targetSchema, true, true,
	); err != nil {
		return err
	}
	h.Schema = targetSchema
	return nil
}

func decodeToMap(spans []Span, buckets []int64) map[int32]int64 {
	m := make(map[int32]int64)
	if len(spans) == 0 {
		return m
	}
	idx := int32(0)
	bucketIdx := 0
	var absNum int64
	for _, s := range spans {
		idx += s.Offset
		for j := uint32(0); j < s.Length; j++ {
			absNum += buckets[bucketIdx]
			if absNum != 0 {
				m[idx] = absNum
			}
			bucketIdx++
			idx++
		}
	}
	return m
}

func encodeFromMap(m map[int32]int64, schema int32, threshold float64, _ []float64) ([]Span, []int64) {
	if len(m) == 0 {
		return nil, nil
	}

	var keys []int32
	for k, v := range m {
		if v != 0 {
			if IsExponentialSchema(schema) && getBoundExponential(k, schema) <= threshold {
				continue
			}
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	slices.Sort(keys)

	var spans []Span
	var buckets []int64
	var lastCount int64
	var lastIdx int32 = -1

	for i, k := range keys {
		if i == 0 {
			spans = append(spans, Span{Offset: k, Length: 1})
			buckets = append(buckets, m[k])
			lastCount = m[k]
			lastIdx = k
			continue
		}

		gap := k - lastIdx - 1
		if gap == 0 {
			spans[len(spans)-1].Length++
		} else {
			spans = append(spans, Span{Offset: gap, Length: 1})
		}
		buckets = append(buckets, m[k]-lastCount)
		lastCount = m[k]
		lastIdx = k
	}

	return spans, buckets
}

func addIntegerBuckets(
	schema int32, threshold float64, negative bool,
	spansA []Span, bucketsA []int64,
	spansB []Span, bucketsB []int64,
) ([]Span, []int64) {
	mA := decodeToMap(spansA, bucketsA)
	mB := decodeToMap(spansB, bucketsB)

	for k, v := range mB {
		if negative {
			mA[k] -= v
		} else {
			mA[k] += v
		}
	}

	return encodeFromMap(mA, schema, threshold, nil)
}

func addCustomIntegerBucketsWithMismatches(
	negative bool,
	spansA []Span, bucketsA []int64, boundsA []float64,
	spansB []Span, bucketsB []int64, boundsB []float64,
	intersectedBounds []float64,
) ([]Span, []int64) {
	targetBuckets := make([]int64, len(intersectedBounds)+1)

	mapBuckets := func(spans []Span, buckets []int64, bounds []float64, negative bool) {
		mA := decodeToMap(spans, buckets)
		for srcIdx, value := range mA {
			targetIdx := len(targetBuckets) - 1 // Default to +Inf
			if int(srcIdx) < len(bounds) {
				srcBound := bounds[srcIdx]
				for intersectIdx := range intersectedBounds {
					if intersectedBounds[intersectIdx] >= srcBound {
						targetIdx = intersectIdx
						break
					}
				}
			}
			if negative {
				targetBuckets[targetIdx] -= value
			} else {
				targetBuckets[targetIdx] += value
			}
		}
	}

	mapBuckets(spansA, bucketsA, boundsA, false)
	mapBuckets(spansB, bucketsB, boundsB, negative)

	mRes := make(map[int32]int64)
	for i, v := range targetBuckets {
		if v != 0 {
			mRes[int32(i)] = v
		}
	}
	return encodeFromMap(mRes, CustomBucketsSchema, 0, intersectedBounds)
}

// DetectReset returns true if the receiving histogram is missing any buckets
// that have a non-zero population in the provided previous histogram. It also
// returns true if any count (in any bucket, in the zero count, or in the count
// of observations, but NOT the sum of observations) is smaller in the receiving
// histogram compared to the previous histogram. Otherwise, it returns false.
func (h *Histogram) DetectReset(previous *Histogram) bool {
	if h.CounterResetHint == CounterReset {
		return true
	}
	if h.CounterResetHint == NotCounterReset {
		return false
	}
	if h.Count < previous.Count {
		return true
	}
	if h.UsesCustomBuckets() {
		if !previous.UsesCustomBuckets() {
			return true
		}
		if !CustomBucketBoundsMatch(h.CustomValues, previous.CustomValues) {
			return h.detectResetWithMismatchedCustomBounds(previous, h.CustomValues, previous.CustomValues)
		}
	}
	if h.Schema > previous.Schema {
		return true
	}
	if h.ZeroThreshold < previous.ZeroThreshold {
		return true
	}

	previousZeroCount, newThreshold := previous.zeroCountForLargerThreshold(h.ZeroThreshold)
	if newThreshold != h.ZeroThreshold {
		return true
	}
	if h.ZeroCount < previousZeroCount {
		return true
	}

	currIt := h.PositiveBucketIterator()
	prevIt := previous.PositiveBucketIterator()
	if h.Schema < previous.Schema {
		p := previous.Copy()
		if err := p.ReduceResolution(h.Schema); err != nil {
			return true
		}
		prevIt = p.PositiveBucketIterator()
	}
	if detectResetInt(currIt, prevIt, h.ZeroThreshold) {
		return true
	}

	currIt = h.NegativeBucketIterator()
	prevIt = previous.NegativeBucketIterator()
	if h.Schema < previous.Schema {
		p := previous.Copy()
		if err := p.ReduceResolution(h.Schema); err != nil {
			return true
		}
		prevIt = p.NegativeBucketIterator()
	}
	return detectResetInt(currIt, prevIt, h.ZeroThreshold)
}

func detectResetInt(currIt, prevIt BucketIterator[uint64], startValue float64) bool {
	advanceToThreshold := func(it BucketIterator[uint64], bound float64) (bool, Bucket[uint64]) {
		for it.Next() {
			b := it.At()
			upperBoundRaw := b.Upper
			if upperBoundRaw < 0 {
				upperBoundRaw = -b.Lower
			}
			if upperBoundRaw > bound {
				return true, b
			}
		}
		return false, Bucket[uint64]{}
	}

	hasPrev, prevBucket := advanceToThreshold(prevIt, startValue)
	if !hasPrev {
		return false
	}
	hasCurr, currBucket := advanceToThreshold(currIt, startValue)
	if !hasCurr {
		for {
			if prevBucket.Count != 0 {
				return true
			}
			if !prevIt.Next() {
				return false
			}
			prevBucket = prevIt.At()
		}
	}

	for {
		for currBucket.Index < prevBucket.Index {
			if !currIt.Next() {
				for {
					if prevBucket.Count != 0 {
						return true
					}
					if !prevIt.Next() {
						return false
					}
					prevBucket = prevIt.At()
				}
			}
			currBucket = currIt.At()
		}
		if currBucket.Index > prevBucket.Index {
			if prevBucket.Count != 0 {
				return true
			}
		} else {
			if currBucket.Count < prevBucket.Count {
				return true
			}
		}
		if !prevIt.Next() {
			return false
		}
		prevBucket = prevIt.At()
	}
}

func (h *Histogram) detectResetWithMismatchedCustomBounds(
	previous *Histogram, currBounds, prevBounds []float64,
) bool {
	if h.Schema != CustomBucketsSchema || previous.Schema != CustomBucketsSchema {
		panic("detectResetWithMismatchedCustomBounds called with non-NHCB schema")
	}
	currIt := h.PositiveBucketIterator()
	prevIt := previous.PositiveBucketIterator()

	rollupSumForBound := func(iter BucketIterator[uint64], iterStarted bool, iterBucket Bucket[uint64], bound float64) (uint64, Bucket[uint64], bool) {
		if !iterStarted {
			if !iter.Next() {
				return 0, Bucket[uint64]{}, false
			}
			iterBucket = iter.At()
		}
		var sum uint64
		for iterBucket.Upper <= bound {
			sum += iterBucket.Count
			if !iter.Next() {
				return sum, Bucket[uint64]{}, false
			}
			iterBucket = iter.At()
		}
		return sum, iterBucket, true
	}

	var (
		currBoundIdx, prevBoundIdx   = 0, 0
		currBucket, prevBucket       Bucket[uint64]
		currIterStarted, currHasMore bool
		prevIterStarted, prevHasMore bool
	)

	for currBoundIdx <= len(currBounds) && prevBoundIdx <= len(prevBounds) {
		currBound := math.Inf(1)
		if currBoundIdx < len(currBounds) {
			currBound = currBounds[currBoundIdx]
		}
		prevBound := math.Inf(1)
		if prevBoundIdx < len(prevBounds) {
			prevBound = prevBounds[prevBoundIdx]
		}

		switch {
		case currBound == prevBound:
			var currRollupSum uint64
			if !currIterStarted || currHasMore {
				currRollupSum, currBucket, currHasMore = rollupSumForBound(currIt, currIterStarted, currBucket, currBound)
				currIterStarted = true
			}
			var prevRollupSum uint64
			if !prevIterStarted || prevHasMore {
				prevRollupSum, prevBucket, prevHasMore = rollupSumForBound(prevIt, prevIterStarted, prevBucket, currBound)
				prevIterStarted = true
			}
			if currRollupSum < prevRollupSum {
				return true
			}
			currBoundIdx++
			prevBoundIdx++
		case currBound < prevBound:
			currBoundIdx++
		default:
			prevBoundIdx++
		}
	}
	return false
}

func (h *Histogram) zeroCountForLargerThreshold(
	largerThreshold float64,
) (hZeroCount uint64, threshold float64) {
	if largerThreshold == h.ZeroThreshold {
		return h.ZeroCount, largerThreshold
	}
	if largerThreshold < h.ZeroThreshold {
		panic(fmt.Errorf("new threshold %f is less than old threshold %f", largerThreshold, h.ZeroThreshold))
	}
outer:
	for {
		hZeroCount = h.ZeroCount
		i := h.PositiveBucketIterator()
		for i.Next() {
			b := i.At()
			if b.Lower >= largerThreshold {
				break
			}
			hZeroCount += b.Count
			if b.Upper > largerThreshold {
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
			hZeroCount += b.Count
			if b.Lower < -largerThreshold {
				if b.Count != 0 {
					largerThreshold = -b.Lower
					continue outer
				}
				break
			}
		}
		return hZeroCount, largerThreshold
	}
}

func (h *Histogram) reconcileZeroBuckets(other *Histogram) uint64 {
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

func (h *Histogram) trimBucketsInZeroBucket() {
	mA := decodeToMap(h.PositiveSpans, h.PositiveBuckets)
	mB := decodeToMap(h.NegativeSpans, h.NegativeBuckets)

	for k := range mA {
		if getBoundExponential(k, h.Schema) <= h.ZeroThreshold {
			delete(mA, k)
		}
	}
	for k := range mB {
		if getBoundExponential(k, h.Schema) <= h.ZeroThreshold {
			delete(mB, k)
		}
	}
	h.PositiveSpans, h.PositiveBuckets = encodeFromMap(mA, h.Schema, h.ZeroThreshold, nil)
	h.NegativeSpans, h.NegativeBuckets = encodeFromMap(mB, h.Schema, h.ZeroThreshold, nil)
}

func (h *Histogram) adjustCounterReset(other *Histogram) bool {
	if h.CounterResetHint == CounterReset || h.CounterResetHint == NotCounterReset {
		return false
	}
	if h.CounterResetHint == UnknownCounterReset && other.CounterResetHint == CounterReset {
		h.CounterResetHint = CounterReset
		return false
	}
	if h.CounterResetHint == GaugeType && other.CounterResetHint == CounterReset {
		return true
	}
	return false
}

func (h *Histogram) checkSchemaAndBounds(other *Histogram) error {
	if h.UsesCustomBuckets() != other.UsesCustomBuckets() {
		return ErrHistogramsIncompatibleSchema
	}
	if h.UsesCustomBuckets() && !CustomBucketBoundsMatch(h.CustomValues, other.CustomValues) {
		return nil
	}
	if !h.UsesCustomBuckets() && (!IsKnownSchema(h.Schema) || !IsKnownSchema(other.Schema)) {
		return UnknownSchemaError(h.Schema)
	}
	return nil
}

func (h *Histogram) Sub(other *Histogram) (*Histogram, bool, bool, error) {
	if err := h.checkSchemaAndBounds(other); err != nil {
		return nil, false, false, err
	}
	counterResetCollision := h.adjustCounterReset(other)
	if !h.UsesCustomBuckets() {
		otherZeroCount := h.reconcileZeroBuckets(other)
		h.ZeroCount -= otherZeroCount
	}
	h.Count -= other.Count
	h.Sum -= other.Sum

	var (
		hPositiveSpans       = h.PositiveSpans
		hPositiveBuckets     = h.PositiveBuckets
		otherPositiveSpans   = other.PositiveSpans
		otherPositiveBuckets = other.PositiveBuckets
		nhcbBoundsReconciled bool
	)

	if h.UsesCustomBuckets() {
		if CustomBucketBoundsMatch(h.CustomValues, other.CustomValues) {
			h.PositiveSpans, h.PositiveBuckets = addIntegerBuckets(h.Schema, h.ZeroThreshold, true, hPositiveSpans, hPositiveBuckets, otherPositiveSpans, otherPositiveBuckets)
		} else {
			nhcbBoundsReconciled = true
			intersectedBounds := intersectCustomBucketBounds(h.CustomValues, other.CustomValues)

			h.PositiveSpans, h.PositiveBuckets = addCustomIntegerBucketsWithMismatches(
				true,
				hPositiveSpans, hPositiveBuckets, h.CustomValues,
				otherPositiveSpans, otherPositiveBuckets, other.CustomValues,
				intersectedBounds)
			h.CustomValues = intersectedBounds
		}
		return h, counterResetCollision, nhcbBoundsReconciled, nil
	}

	var (
		hNegativeSpans       = h.NegativeSpans
		hNegativeBuckets     = h.NegativeBuckets
		otherNegativeSpans   = other.NegativeSpans
		otherNegativeBuckets = other.NegativeBuckets
	)

	switch {
	case other.Schema < h.Schema:
		hPositiveSpans, hPositiveBuckets, _ = reduceResolution(hPositiveSpans, hPositiveBuckets, h.Schema, other.Schema, true, false)
		hNegativeSpans, hNegativeBuckets, _ = reduceResolution(hNegativeSpans, hNegativeBuckets, h.Schema, other.Schema, true, false)
		h.Schema = other.Schema
	case other.Schema > h.Schema:
		otherPositiveSpans, otherPositiveBuckets, _ = reduceResolution(otherPositiveSpans, otherPositiveBuckets, other.Schema, h.Schema, true, false)
		otherNegativeSpans, otherNegativeBuckets, _ = reduceResolution(otherNegativeSpans, otherNegativeBuckets, other.Schema, h.Schema, true, false)
	}

	h.PositiveSpans, h.PositiveBuckets = addIntegerBuckets(h.Schema, h.ZeroThreshold, true, hPositiveSpans, hPositiveBuckets, otherPositiveSpans, otherPositiveBuckets)
	h.NegativeSpans, h.NegativeBuckets = addIntegerBuckets(h.Schema, h.ZeroThreshold, true, hNegativeSpans, hNegativeBuckets, otherNegativeSpans, otherNegativeBuckets)

	return h, counterResetCollision, nhcbBoundsReconciled, nil
}
