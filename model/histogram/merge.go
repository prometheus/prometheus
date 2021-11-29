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
// upper boundary.
type floatBucketIteratorHeap []FloatBucketIterator

func (h floatBucketIteratorHeap) Len() int      { return len(h) }
func (h floatBucketIteratorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h floatBucketIteratorHeap) Less(i, j int) bool {
	return h[i].At().Upper < h[j].At().Upper
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
