package tsdb

import (
	"fmt"
	"math/rand"

	"github.com/prometheus/prometheus/pkg/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// this file contains helpers for head_test.go

// IntermediateHistogram represents a histogram by using the raw counts for both the pos and neg buckets
// The benefit of this type is that it becomes easy to expand the amount of buckets used (just interject 0 values)
// as opposed to SparseHistogram which needs to recompute delta's. Also, taking a different approach than
// the coder under test, is useful in its own right
type intermediateHistogram struct {
	ts        int64
	count     uint64
	zeroCount uint64
	sum       float64 // TODO
	schema    int32
	posSpans  []histogram.Span
	negSpans  []histogram.Span
	posCounts []int64
	negCounts []int64
}

func newIntermediateHistogram(posSpans, negSpans []histogram.Span) intermediateHistogram {
	return intermediateHistogram{
		posSpans:  posSpans,
		negSpans:  negSpans,
		posCounts: make([]int64, histogram.CountSpans(posSpans)),
		negCounts: make([]int64, histogram.CountSpans(negSpans)),
	}
}

// Bump mimics changes caused by a future observation. Bucket counts increase by a random amount (possibly 0)
// ts must be > than previous ts!
// we reuse the existing slices, so you should use the returned value such that all properties reflect their update
func (h intermediateHistogram) Bump(ts int64) intermediateHistogram {
	bumpCounts := func(in []int64) {
		for i := range in {
			in[i] = in[i] + int64(rand.Int31()/1e6)
		}
	}
	h.ts = ts
	bumpCounts(h.posCounts)
	bumpCounts(h.negCounts)
	h.count += uint64(rand.Int31() / 1e3)
	h.zeroCount += uint64(rand.Int31() / 1e5)
	return h
}

func (h intermediateHistogram) ToSparse() timedHist {
	countsToDeltas := func(counts []int64) []int64 {
		deltas := make([]int64, len(counts))
		prev := int64(0)
		for i := 0; i < len(counts); i++ {
			deltas[i] = counts[i] - prev
			prev = counts[i]
		}
		return deltas
	}

	return timedHist{
		t: h.ts,
		h: histogram.SparseHistogram{
			Count:     h.count,
			ZeroCount: h.zeroCount,
			// TODO Zerothreshold
			Sum:             h.sum,
			Schema:          h.schema,
			PositiveSpans:   h.posSpans,
			NegativeSpans:   h.negSpans,
			PositiveBuckets: countsToDeltas(h.posCounts),
			NegativeBuckets: countsToDeltas(h.negCounts),
		},
	}
}

func (h intermediateHistogram) expand(posSpans, negSpans []histogram.Span) intermediateHistogram {
	if histogram.CountSpans(posSpans) > histogram.CountSpans(h.posSpans) {
		h.posCounts = expandCounts(h.posCounts, h.posSpans, posSpans)
		h.posSpans = posSpans
	}
	if histogram.CountSpans(negSpans) > histogram.CountSpans(h.negSpans) {
		h.negCounts = expandCounts(h.negCounts, h.negSpans, negSpans)
		h.negSpans = negSpans
	}
	return h
}

// caller must assure that b is a superset of a
// note: this function looks like chunkenc.compareSpans
// not ideal to write test code that is similar to code under test,
// but we test compareSpans separately
// we may want a test for this function too. maybe later :?
func expandCounts(countsA []int64, a, b []histogram.Span) []int64 {
	countsB := make([]int64, histogram.CountSpans(b))

	ait := chunkenc.NewBucketIterator(a)
	bit := chunkenc.NewBucketIterator(b)

	var ai, bi int // position in their respective counts

	av, aok := ait.Next()
	bv, bok := bit.Next()
	for {
		if aok && bok {
			if av == bv { // both contain the same bucket index.
				countsB[bi] = countsB[ai]
				av, aok = ait.Next()
				bv, bok = bit.Next()
				ai++
				bi++
			} else if av < bv {
				panic(fmt.Sprintf("b misses a value that is in a (%d). invalid call", av))
			} else { // av > bv -> a misses a value that is in b.
				countsB[bi] = 0
				bv, bok = bit.Next()
				bi++
				continue
			}
		} else if aok && !bok {
			panic(fmt.Sprintf("b misses a value that is in a (%d). invalid call", av))
		} else if !aok && bok { // a misses a value that is in b.
			countsB[bi] = 0
			bv, bok = bit.Next()
			bi++
			continue
		} else { // both iterators ran out. we're done
			break
		}
	}

	return countsB
}

/*
// bumpHistogram generates a histogram that "resumes" from a previous counts.
// it accommodates buckets expansion (by starting new counts) but cannot handle contraction of the amount of buckets.
func bumpHistogram(i int, prevPosCounts, prevNegCounts counts, posSpans, negSpans []histogram.Span) histogram.SparseHistogram {
	if histogram.CountSpans(posSpans) != len(prevPosCounts) || histogram.CountSpans(negSpans) != len(prevNegCounts) {
		panic("can only generate new histogram with prev counts matching spans")
	}
	return histogram.SparseHistogram{
		Count:           5 + uint64(i*4),
		ZeroCount:       2 + uint64(i),
		Sum:             18.4 * float64(i+1),
		Schema:          1,
		PositiveSpans:   posSpans,
		NegativeSpans:   negSpans,
		PositiveBuckets: countsToDeltas(bumpCounts(prevCounts)),
		NegativeBuckets: []int64{},
	}
}
*/
