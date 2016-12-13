package tsdb

import (
	"sort"
	"strings"
)

// Index provides read access to an inverted index.
type Index interface {
	Postings(ref uint32) Iterator
}

type memIndex struct {
	lastID uint32

	forward  map[uint32]*chunkDesc // chunk ID to chunk desc
	values   map[string]stringset  // label names to possible values
	postings *memPostings          // postings lists for terms
}

// newMemIndex returns a new in-memory  index.
func newMemIndex() *memIndex {
	return &memIndex{
		lastID:   0,
		forward:  make(map[uint32]*chunkDesc),
		values:   make(map[string]stringset),
		postings: &memPostings{m: make(map[term][]uint32)},
	}
}

func (ix *memIndex) numSeries() int {
	return len(ix.forward)
}

func (ix *memIndex) Postings(t term) Iterator {
	return ix.postings.get(t)
}

type term struct {
	name, value string
}

func (ix *memIndex) add(chkd *chunkDesc) {
	// Add each label pair as a term to the inverted index.
	terms := make([]term, 0, len(chkd.lset))

	for _, l := range chkd.lset {
		terms = append(terms, term{name: l.Name, value: l.Value})

		// Add to label name to values index.
		valset, ok := ix.values[l.Name]
		if !ok {
			valset = stringset{}
			ix.values[l.Name] = valset
		}
		valset.set(l.Value)
	}
	ix.lastID++
	id := ix.lastID

	ix.postings.add(id, terms...)

	// Store forward index for the returned ID.
	ix.forward[id] = chkd
}

type memPostings struct {
	m map[term][]uint32
}

// Postings returns an iterator over the postings list for s.
func (p *memPostings) get(t term) Iterator {
	return &listIterator{list: p.m[t], idx: -1}
}

// add adds a document to the index. The caller has to ensure that no
// term argument appears twice.
func (p *memPostings) add(id uint32, terms ...term) {
	for _, t := range terms {
		p.m[t] = append(p.m[t], id)
	}
}

// Iterator provides iterative access over a postings list.
type Iterator interface {
	// Next advances the iterator and returns true if another value was found.
	Next() bool

	// Seek advances the iterator to value v or greater and returns
	// true if a value was found.
	Seek(v uint32) bool

	// Value returns the value at the current iterator position.
	Value() uint32

	// Err returns the last error of the iterator.
	Err() error
}

// errIterator is an empty iterator that always errors.
type errIterator struct {
	err error
}

func (e errIterator) Next() bool       { return false }
func (e errIterator) Seek(uint32) bool { return false }
func (e errIterator) Value() uint32    { return 0 }
func (e errIterator) Err() error       { return e.err }

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

func (it *intersectIterator) Err() error {
	return nil
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

func (it *mergeIterator) Err() error {
	return nil
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

func (it *listIterator) Err() error {
	return nil
}

type stringset map[string]struct{}

func (ss stringset) set(s string) {
	ss[s] = struct{}{}
}

func (ss stringset) has(s string) bool {
	_, ok := ss[s]
	return ok
}

func (ss stringset) String() string {
	return strings.Join(ss.slice(), ",")
}

func (ss stringset) slice() []string {
	slice := make([]string, 0, len(ss))
	for k := range ss {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
}
