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
	"container/heap"
	"math"
)

// NewMergeFloatBucketIterator returns an iterator which merges the results from
// all the given iterators. Buckets with same upper bound are merged by summing their counts.
//
// All iterators should have the same schema. TODO(codesome): support having mixed schemas.
//
// All the passed iterators should either have positive buckets or negative buckets
// and not a mix of both.
func NewMergeFloatBucketIterator(its ...FloatBucketIterator) FloatBucketIterator {
	var h floatBucketIteratorHeap
	for _, it := range its {
		if it.Next() {
			heap.Push(&h, it)
		}
	}

	return &mergeFloatBucketIterator{
		heap: h,
	}
}

type mergeFloatBucketIterator struct {
	// Heap based on the upper bound of the buckets.
	heap floatBucketIteratorHeap

	// currentIters contains the iterators that correspond to the
	// current iteration element and are not in the heap.
	currentIters []FloatBucketIterator
}

func (m *mergeFloatBucketIterator) Next() bool {
	for _, it := range m.currentIters {
		if it.Next() {
			heap.Push(&m.heap, it)
		}
	}

	if len(m.heap) == 0 {
		return false
	}

	m.currentIters = nil
	currUpper := m.heap[0].At().Upper
	for len(m.heap) > 0 && m.heap[0].At().Upper == currUpper {
		it := heap.Pop(&m.heap).(FloatBucketIterator)
		m.currentIters = append(m.currentIters, it)
	}

	return true
}

func (m *mergeFloatBucketIterator) At() FloatBucket {
	if len(m.currentIters) == 0 {
		return FloatBucket{}
	}
	// Add all the counts for the same bucket boundaries.
	fb := m.currentIters[0].At()
	for _, it := range m.currentIters[1:] {
		fb.Count += it.At().Count
	}
	return fb
}

// floatBucketIteratorHeap is a min-heap based on the bucket's
// index in the schema boundary.
type floatBucketIteratorHeap []FloatBucketIterator

func (h floatBucketIteratorHeap) Len() int      { return len(h) }
func (h floatBucketIteratorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h floatBucketIteratorHeap) Less(i, j int) bool {
	return h[i].At().Index < h[j].At().Index
}

func (h *floatBucketIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(FloatBucketIterator))
}

func (h *floatBucketIteratorHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// MergeHistograms adds all histograms together into a single histogram
// by adding counts of the corresponding buckets and their count and sum
// including the zero count.
//
// Important: All the histograms must have the same schema and zero threshold, else the behaviour is undefined.
// TODO(codesome): Fix the above.
func MergeHistograms(hists ...FloatHistogram) FloatHistogram {
	if len(hists) == 0 {
		return FloatHistogram{}
	}

	res := FloatHistogram{
		Schema:        hists[0].Schema,
		ZeroThreshold: hists[0].ZeroThreshold,
	}

	// Merge positive buckets.
	its := make([]FloatBucketIterator, 0, len(hists))
	for _, h := range hists {
		if h.Schema != res.Schema || h.ZeroThreshold != res.ZeroThreshold {
			// Not supported yet.
			return FloatHistogram{}
		}

		its = append(its, h.PositiveBucketIterator())
		res.Count += h.Count
		res.ZeroCount += h.ZeroCount
		res.Sum += h.Sum
	}
	it := NewMergeFloatBucketIterator(its...)
	res.PositiveBuckets, res.PositiveSpans = generateSpansFromBucketIterator(it)

	// Merge negative buckets.
	its = its[:0]
	for _, h := range hists {
		its = append(its, h.NegativeBucketIterator())
	}
	it = NewMergeFloatBucketIterator(its...)
	res.NegativeBuckets, res.NegativeSpans = generateSpansFromBucketIterator(it)

	return res
}

func generateSpansFromBucketIterator(it FloatBucketIterator) (buckets []float64, spans []Span) {
	lastIdx := int32(math.MinInt32)
	for it.Next() {
		b := it.At()
		buckets = append(buckets, b.Count)

		if len(spans) == 0 {
			// First bucket.
			spans = append(spans, Span{
				Offset: b.Index,
				Length: 1,
			})
			lastIdx = b.Index
			continue
		}

		gap := b.Index - lastIdx - 1
		if gap > 0 {
			// TODO(codesome): allow some tolerance. i.e. >0 gap.
			spans = append(spans, Span{Offset: gap})
		}

		lastIdx = b.Index
		spans[len(spans)-1].Length++
	}

	return buckets, spans
}
