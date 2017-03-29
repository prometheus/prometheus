package tsdb

import (
	"encoding/binary"
	"sort"
	"strings"
)

type memPostings struct {
	m map[term][]uint32
}

type term struct {
	name, value string
}

// Postings returns an iterator over the postings list for s.
func (p *memPostings) get(t term) Postings {
	l := p.m[t]
	if l == nil {
		return emptyPostings
	}
	return &listPostings{list: l, idx: -1}
}

// add adds a document to the index. The caller has to ensure that no
// term argument appears twice.
func (p *memPostings) add(id uint32, terms ...term) {
	for _, t := range terms {
		p.m[t] = append(p.m[t], id)
	}
}

// Postings provides iterative access over a postings list.
type Postings interface {
	// Next advances the iterator and returns true if another value was found.
	Next() bool

	// Seek advances the iterator to value v or greater and returns
	// true if a value was found.
	Seek(v uint32) bool

	// At returns the value at the current iterator position.
	At() uint32

	// Err returns the last error of the iterator.
	Err() error
}

// errPostings is an empty iterator that always errors.
type errPostings struct {
	err error
}

func (e errPostings) Next() bool       { return false }
func (e errPostings) Seek(uint32) bool { return false }
func (e errPostings) At() uint32       { return 0 }
func (e errPostings) Err() error       { return e.err }

func expandPostings(p Postings) (res []uint32, err error) {
	for p.Next() {
		res = append(res, p.At())
	}
	return res, p.Err()
}

// Intersect returns a new postings list over the intersection of the
// input postings.
func Intersect(its ...Postings) Postings {
	if len(its) == 0 {
		return errPostings{err: nil}
	}
	a := its[0]

	for _, b := range its[1:] {
		a = newIntersectPostings(a, b)
	}
	return a
}

var emptyPostings = errPostings{}

type intersectPostings struct {
	a, b     Postings
	aok, bok bool
	cur      uint32
}

func newIntersectPostings(a, b Postings) *intersectPostings {
	it := &intersectPostings{a: a, b: b}
	it.aok = it.a.Next()
	it.bok = it.b.Next()

	return it
}

func (it *intersectPostings) At() uint32 {
	return it.cur
}

func (it *intersectPostings) Next() bool {
	for {
		if !it.aok || !it.bok {
			return false
		}
		av, bv := it.a.At(), it.b.At()

		if av < bv {
			it.aok = it.a.Seek(bv)
		} else if bv < av {
			it.bok = it.b.Seek(av)
		} else {
			it.cur = av
			it.aok = it.a.Next()
			it.bok = it.b.Next()
			return true
		}
	}
}

func (it *intersectPostings) Seek(id uint32) bool {
	it.aok = it.a.Seek(id)
	it.bok = it.b.Seek(id)
	return it.Next()
}

func (it *intersectPostings) Err() error {
	if it.a.Err() != nil {
		return it.a.Err()
	}
	return it.b.Err()
}

// Merge returns a new iterator over the union of the input iterators.
func Merge(its ...Postings) Postings {
	if len(its) == 0 {
		return nil
	}
	a := its[0]

	for _, b := range its[1:] {
		a = newMergePostings(a, b)
	}
	return a
}

type mergePostings struct {
	a, b     Postings
	aok, bok bool
	cur      uint32
}

func newMergePostings(a, b Postings) *mergePostings {
	it := &mergePostings{a: a, b: b}
	it.aok = it.a.Next()
	it.bok = it.b.Next()

	return it
}

func (it *mergePostings) At() uint32 {
	return it.cur
}

func (it *mergePostings) Next() bool {
	if !it.aok && !it.bok {
		return false
	}

	if !it.aok {
		it.cur = it.b.At()
		it.bok = it.b.Next()
		return true
	}
	if !it.bok {
		it.cur = it.a.At()
		it.aok = it.a.Next()
		return true
	}

	acur, bcur := it.a.At(), it.b.At()

	if acur < bcur {
		it.cur = acur
		it.aok = it.a.Next()
		return true
	}
	if bcur < acur {
		it.cur = bcur
		it.bok = it.b.Next()
		return true
	}
	it.cur = acur
	it.aok = it.a.Next()
	it.bok = it.b.Next()

	return true
}

func (it *mergePostings) Seek(id uint32) bool {
	it.aok = it.a.Seek(id)
	it.bok = it.b.Seek(id)
	return it.Next()
}

func (it *mergePostings) Err() error {
	if it.a.Err() != nil {
		return it.a.Err()
	}
	return it.b.Err()
}

// listPostings implements the Postings interface over a plain list.
type listPostings struct {
	list []uint32
	idx  int
}

func newListPostings(list []uint32) *listPostings {
	return &listPostings{list: list, idx: -1}
}

func (it *listPostings) At() uint32 {
	return it.list[it.idx]
}

func (it *listPostings) Next() bool {
	it.idx++
	return it.idx < len(it.list)
}

func (it *listPostings) Seek(x uint32) bool {
	// Do binary search between current position and end.
	it.idx += sort.Search(len(it.list)-it.idx, func(i int) bool {
		return it.list[i+it.idx] >= x
	})
	return it.idx < len(it.list)
}

func (it *listPostings) Err() error {
	return nil
}

// bigEndianPostings implements the Postings interface over a byte stream of
// big endian numbers.
type bigEndianPostings struct {
	list []byte
	idx  int
}

func newBigEndianPostings(list []byte) *bigEndianPostings {
	return &bigEndianPostings{list: list, idx: -1}
}

func (it *bigEndianPostings) At() uint32 {
	idx := 4 * it.idx
	return binary.BigEndian.Uint32(it.list[idx : idx+4])
}

func (it *bigEndianPostings) Next() bool {
	it.idx++
	return it.idx*4 < len(it.list)
}

func (it *bigEndianPostings) Seek(x uint32) bool {
	num := len(it.list) / 4
	// Do binary search between current position and end.
	it.idx += sort.Search(num-it.idx, func(i int) bool {
		idx := 4 * (it.idx + i)
		val := binary.BigEndian.Uint32(it.list[idx : idx+4])
		return val >= x
	})
	return it.idx*4 < len(it.list)
}

func (it *bigEndianPostings) Err() error {
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
