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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It was modified to accommodate reading from byte slices without modifying
// the underlying bytes, which would panic when reading from mmap'd
// read-only byte slices.
package chunkenc

import "github.com/prometheus/prometheus/pkg/histogram"

func writeHistoChunkMeta(b *bstream, schema int32, posSpans, negSpans []histogram.Span) {
	putInt64VBBucket(b, int64(schema))
	putHistoChunkMetaSpans(b, posSpans)
	putHistoChunkMetaSpans(b, negSpans)
}

func putHistoChunkMetaSpans(b *bstream, spans []histogram.Span) {
	putInt64VBBucket(b, int64(len(spans)))
	for _, s := range spans {
		putInt64VBBucket(b, int64(s.Length))
		putInt64VBBucket(b, int64(s.Offset))
	}
}

func readHistoChunkMeta(b *bstreamReader) (int32, []histogram.Span, []histogram.Span, error) {

	v, err := readInt64VBBucket(b)
	if err != nil {
		return 0, nil, nil, err
	}
	schema := int32(v)

	posSpans, err := readHistoChunkMetaSpans(b)
	if err != nil {
		return 0, nil, nil, err
	}

	negSpans, err := readHistoChunkMetaSpans(b)
	if err != nil {
		return 0, nil, nil, err
	}

	return schema, posSpans, negSpans, nil
}

func readHistoChunkMetaSpans(b *bstreamReader) ([]histogram.Span, error) {
	var spans []histogram.Span
	num, err := readInt64VBBucket(b)
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(num); i++ {

		length, err := readInt64VBBucket(b)
		if err != nil {
			return nil, err
		}

		offset, err := readInt64VBBucket(b)
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
	span   int // span position of last yielded bucket
	bucket int // bucket position within span of last yielded bucket
	idx    int // bucket index (globally across all spans) of last yielded bucket
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
	// we're already out of bounds
	if b.span >= len(b.spans) {
		return 0, false
	}

	if b.bucket < int(b.spans[b.span].Length-1) { // try to move within same span.
		b.bucket++
		b.idx++
		return b.idx, true
	} else if b.span < len(b.spans)-1 { // try to move from one span to the next
		b.span++
		if b.spans[b.span].Length == 0 {
			panic("span is invalid because it has length 0")
		}
		b.idx += int(b.spans[b.span].Offset + 1)
		b.bucket = 0
		return b.idx, true
	}
	// we're out of options
	return 0, false
}

// interjection describes that num new buckets are introduced before processing the pos'th delta from the original slice
type interjection struct {
	pos int
	num int
}

// compareSpans returns the interjections to convert a slice of deltas to a new slice representing an expanded set of buckets, or false if incompatible (e.g. if buckets were removed)
// For example:
// Let's say the old buckets look like this:
// span syntax: [offset, length]
// spans      : [ 0 , 2 ]               [2,1]                   [ 3 , 2 ]                     [3,1]       [1,1]
// bucket idx : [0]   [1]    2     3    [4]    5     6     7    [8]   [9]    10    11    12   [13]   14   [15]
// raw values    6     3                 3                       2     4                       5           1
// deltas        6    -3                 0                      -1     2                       1          -4

// But now we introduce a new bucket layout. (carefully chosen example where we have a span appended, one unchanged[*], one prepended, and two merge - in that order)
// [*] unchanged in terms of which bucket indices they represent. but to achieve that, their offset needs to change if "disrupted" by spans changing ahead of them
//                                       \/ this one is "unchanged"
// spans      : [  0  ,  3    ]         [1,1]       [    1    ,   4     ]                     [  3  ,   3    ]
// bucket idx : [0]   [1]   [2]    3    [4]    5    [6]   [7]   [8]   [9]    10    11    12   [13]  [14]  [15]
// raw values    6     3     0           3           0     0     2     4                       5     0     1
// deltas        6    -3    -3           3          -3     0     2     2                       1    -5     1
// delta mods:                          / \                     / \                                       / \
// note that whenever any new buckets are introduced, the subsequent "old" bucket needs to readjust its delta to the new base of 0
// thus, for the caller, who wants to transform the set of original deltas to a new set of deltas to match a new span layout that adds buckets, we simply
// need to generate a list of interjections
// note: within compareSpans we don't have to worry about the changes to the spans themselves,
// thanks to the iterators, we get to work with the more useful bucket indices (which of course directly correspond to the buckets we have to adjust)
func compareSpans(a, b []histogram.Span) ([]interjection, bool) {
	ai := newBucketIterator(a)
	bi := newBucketIterator(b)

	var interjections []interjection

	// when inter.num becomes > 0, this becomes a valid interjection that should be yielded when we finish a streak of new buckets
	var inter interjection

	av, aok := ai.Next()
	bv, bok := bi.Next()
	for {
		if aok && bok {
			if av == bv { // both have an identical value. move on!
				// finish WIP interjection and reset
				if inter.num > 0 {
					interjections = append(interjections, inter)
				}
				inter.num = 0
				av, aok = ai.Next()
				bv, bok = bi.Next()
				if aok {
					inter.pos++
				}
				continue
			}
			if av < bv { // b misses a value that is in a.
				return interjections, false
			}
			if av > bv { // a misses a value that is in b. forward b and recompare
				inter.num++
				bv, bok = bi.Next()
				continue
			}
		} else if aok && !bok { // b misses a value that is in a.
			return interjections, false
		} else if !aok && bok { // a misses a value that is in b. forward b and recompare
			inter.num++
			bv, bok = bi.Next()
			continue
		} else { // both iterators ran out. we're done
			if inter.num > 0 {
				interjections = append(interjections, inter)
			}
			break
		}
	}

	return interjections, true
}
