package tsdb

import "sort"

// Index provides read access to an inverted index.
type Index interface {
	Postings(ref uint32) Iterator
}

// memIndex is an inverted in-memory index.
type memIndex struct {
	lastID uint32
	m      map[string][]uint32
}

// Postings returns an iterator over the postings list for s.
func (ix *memIndex) Postings(s string) Iterator {
	return &listIterator{list: ix.m[s]}
}

// add adds a document to the index. The caller has to ensure that no
// term argument appears twice.
func (ix *memIndex) add(terms ...string) uint32 {
	ix.lastID++

	for _, t := range terms {
		ix.m[t] = append(ix.m[t], ix.lastID)
	}

	return ix.lastID
}

// newMemIndex returns a new in-memory index.
func newMemIndex() *memIndex {
	return &memIndex{m: make(map[string][]uint32)}
}

// Iterator provides iterative access over a postings list.
type Iterator interface {
	// Next advances the iterator and returns true if another
	// value was found.
	Next() bool
	// Seek advances the iterator to value v or greater and returns
	// true if a value was found.
	Seek(v uint32) bool
	// Value returns the value at the current iterator position.
	Value() uint32
}

// compressIndex returns a compressed index for the given input index.
func compressIndex(ix Index) {

}

// Intersect returns a new iterator over the intersection of the
// input iterators.
func Intersect(its ...Iterator) Iterator {
	if len(its) == 0 {
		return nil
	}
	a := its[0]

	for _, b := range its[1:] {
		a = &intersectIterator{a: a, b: b}
	}
	return a
}

type intersectIterator struct {
	a, b Iterator
}

func (it *intersectIterator) Value() uint32 {
	return 0
}

func (it *intersectIterator) Next() bool {
	return false
}

func (it *intersectIterator) Seek(id uint32) bool {
	return false
}

// Merge returns a new iterator over the union of the input iterators.
func Merge(its ...Iterator) Iterator {
	if len(its) == 0 {
		return nil
	}
	a := its[0]

	for _, b := range its[1:] {
		a = &mergeIterator{a: a, b: b}
	}
	return a
}

type mergeIterator struct {
	a, b Iterator
}

func (it *mergeIterator) Value() uint32 {
	return 0
}

func (it *mergeIterator) Next() bool {
	return false
}

func (it *mergeIterator) Seek(id uint32) bool {
	return false
}

// listIterator implements the Iterator interface over a plain list.
type listIterator struct {
	list []uint32
	idx  int
}

func (it *listIterator) Value() uint32 {
	return it.list[it.idx]
}

func (it *listIterator) Next() bool {
	it.idx++
	return it.idx < len(it.list)
}

func (it *listIterator) Seek(x uint32) bool {
	// Do binary search between current position and end.
	it.idx = sort.Search(len(it.list)-it.idx, func(i int) bool {
		return it.list[i+it.idx] >= x
	})
	return it.idx < len(it.list)
}
