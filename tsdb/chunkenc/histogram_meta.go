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

package chunkenc

import (
	"github.com/prometheus/prometheus/model/histogram"
)

func writeHistogramChunkMeta(b *bstream, schema int32, zeroThreshold float64, positiveSpans, negativeSpans []histogram.Span) {
	putVarbitInt(b, int64(schema))
	putVarbitFloat(b, zeroThreshold)
	putHistogramChunkMetaSpans(b, positiveSpans)
	putHistogramChunkMetaSpans(b, negativeSpans)
}

func putHistogramChunkMetaSpans(b *bstream, spans []histogram.Span) {
	putVarbitInt(b, int64(len(spans)))
	for _, s := range spans {
		putVarbitInt(b, int64(s.Length))
		putVarbitInt(b, int64(s.Offset))
	}
}

func readHistogramChunkMeta(b *bstreamReader) (
	schema int32, zeroThreshold float64,
	positiveSpans, negativeSpans []histogram.Span,
	err error,
) {
	_, err = b.ReadByte() // The header.
	if err != nil {
		return
	}

	v, err := readVarbitInt(b)
	if err != nil {
		return
	}
	schema = int32(v)

	zeroThreshold, err = readVarbitFloat(b)
	if err != nil {
		return
	}

	positiveSpans, err = readHistogramChunkMetaSpans(b)
	if err != nil {
		return
	}

	negativeSpans, err = readHistogramChunkMetaSpans(b)
	if err != nil {
		return
	}

	return
}

func readHistogramChunkMetaSpans(b *bstreamReader) ([]histogram.Span, error) {
	var spans []histogram.Span
	num, err := readVarbitInt(b)
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(num); i++ {

		length, err := readVarbitInt(b)
		if err != nil {
			return nil, err
		}

		offset, err := readVarbitInt(b)
		if err != nil {
			return nil, err
		}

		spans = append(spans, histogram.Span{
			Length: uint32(length),
			Offset: int32(offset),
		})
	}
	return spans, nil
}

type bucketIterator struct {
	spans  []histogram.Span
	span   int // Span position of last yielded bucket.
	bucket int // Bucket position within span of last yielded bucket.
	idx    int // Bucket index (globally across all spans) of last yielded bucket.
}

func newBucketIterator(spans []histogram.Span) *bucketIterator {
	b := bucketIterator{
		spans:  spans,
		span:   0,
		bucket: -1,
		idx:    -1,
	}
	if len(spans) > 0 {
		b.idx += int(spans[0].Offset)
	}
	return &b
}

func (b *bucketIterator) Next() (int, bool) {
	// We're already out of bounds.
	if b.span >= len(b.spans) {
		return 0, false
	}
try:
	if b.bucket < int(b.spans[b.span].Length-1) { // Try to move within same span.
		b.bucket++
		b.idx++
		return b.idx, true
	} else if b.span < len(b.spans)-1 { // Try to move from one span to the next.
		b.span++
		b.idx += int(b.spans[b.span].Offset + 1)
		b.bucket = 0
		if b.spans[b.span].Length == 0 {
			// Pathological case that should never happen. We can't use this span, let's try again.
			goto try
		}
		return b.idx, true
	}
	// We're out of options.
	return 0, false
}

// An Interjection describes how many new buckets have to be introduced before
// processing the pos'th delta from the original slice.
type Interjection struct {
	pos int
	num int
}

// compareSpans returns the interjections to convert a slice of deltas to a new
// slice representing an expanded set of buckets, or false if incompatible
// (e.g. if buckets were removed).
//
// Example:
//
// Let's say the old buckets look like this:
//
//   span syntax: [offset, length]
//   spans      : [ 0 , 2 ]               [2,1]                   [ 3 , 2 ]                     [3,1]       [1,1]
//   bucket idx : [0]   [1]    2     3    [4]    5     6     7    [8]   [9]    10    11    12   [13]   14   [15]
//   raw values    6     3                 3                       2     4                       5           1
//   deltas        6    -3                 0                      -1     2                       1          -4
//
// But now we introduce a new bucket layout. (Carefully chosen example where we
// have a span appended, one unchanged[*], one prepended, and two merge - in
// that order.)
//
// [*] unchanged in terms of which bucket indices they represent. but to achieve
// that, their offset needs to change if "disrupted" by spans changing ahead of
// them
//
//                                         \/ this one is "unchanged"
//   spans      : [  0  ,  3    ]         [1,1]       [    1    ,   4     ]                     [  3  ,   3    ]
//   bucket idx : [0]   [1]   [2]    3    [4]    5    [6]   [7]   [8]   [9]    10    11    12   [13]  [14]  [15]
//   raw values    6     3     0           3           0     0     2     4                       5     0     1
//   deltas        6    -3    -3           3          -3     0     2     2                       1    -5     1
//   delta mods:                          / \                     / \                                       / \
//
// Note that whenever any new buckets are introduced, the subsequent "old"
// bucket needs to readjust its delta to the new base of 0. Thus, for the caller
// who wants to transform the set of original deltas to a new set of deltas to
// match a new span layout that adds buckets, we simply need to generate a list
// of interjections.
//
// Note: Within compareSpans we don't have to worry about the changes to the
// spans themselves, thanks to the iterators we get to work with the more useful
// bucket indices (which of course directly correspond to the buckets we have to
// adjust).
func compareSpans(a, b []histogram.Span) ([]Interjection, bool) {
	ai := newBucketIterator(a)
	bi := newBucketIterator(b)

	var interjections []Interjection

	// When inter.num becomes > 0, this becomes a valid interjection that
	// should be yielded when we finish a streak of new buckets.
	var inter Interjection

	av, aOK := ai.Next()
	bv, bOK := bi.Next()
loop:
	for {
		switch {
		case aOK && bOK:
			switch {
			case av == bv: // Both have an identical value. move on!
				// Finish WIP interjection and reset.
				if inter.num > 0 {
					interjections = append(interjections, inter)
				}
				inter.num = 0
				av, aOK = ai.Next()
				bv, bOK = bi.Next()
				inter.pos++
			case av < bv: // b misses a value that is in a.
				return interjections, false
			case av > bv: // a misses a value that is in b. Forward b and recompare.
				inter.num++
				bv, bOK = bi.Next()
			}
		case aOK && !bOK: // b misses a value that is in a.
			return interjections, false
		case !aOK && bOK: // a misses a value that is in b. Forward b and recompare.
			inter.num++
			bv, bOK = bi.Next()
		default: // Both iterators ran out. We're done.
			if inter.num > 0 {
				interjections = append(interjections, inter)
			}
			break loop
		}
	}

	return interjections, true
}

// interject merges 'in' with the provided interjections and writes them into
// 'out', which must already have the appropriate length.
func interject(in, out []int64, interjections []Interjection) []int64 {
	var (
		j      int   // Position in out.
		v      int64 // The last value seen.
		interj int   // The next interjection to process.
	)
	for i, d := range in {
		if interj < len(interjections) && i == interjections[interj].pos {

			// We have an interjection!
			// Add interjection.num new delta values such that their
			// bucket values equate 0.
			out[j] = int64(-v)
			j++
			for x := 1; x < interjections[interj].num; x++ {
				out[j] = 0
				j++
			}
			interj++

			// Now save the value from the input. The delta value we
			// should save is the original delta value + the last
			// value of the point before the interjection (to undo
			// the delta that was introduced by the interjection).
			out[j] = d + v
			j++
			v = d + v
			continue
		}

		// If there was no interjection, the original delta is still
		// valid.
		out[j] = d
		j++
		v += d
	}
	switch interj {
	case len(interjections):
		// All interjections processed. Nothing more to do.
	case len(interjections) - 1:
		// One more interjection to process at the end.
		out[j] = int64(-v)
		j++
		for x := 1; x < interjections[interj].num; x++ {
			out[j] = 0
			j++
		}
	default:
		panic("unprocessed interjections left")
	}
	return out
}
