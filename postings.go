// Copyright 2017 The Prometheus Authors
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

package tsdb

import (
	"encoding/binary"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/tsdb/labels"
)

type memPostings struct {
	mtx sync.RWMutex
	m   map[labels.Label][]uint64
}

func newMemPostings() *memPostings {
	return &memPostings{
		m: make(map[labels.Label][]uint64, 512),
	}
}

// Postings returns an iterator over the postings list for s.
func (p *memPostings) get(name, value string) Postings {
	p.mtx.RLock()
	l := p.m[labels.Label{Name: name, Value: value}]
	p.mtx.RUnlock()

	if l == nil {
		return emptyPostings
	}
	return newListPostings(l)
}

var allLabel = labels.Label{}

// add adds a document to the index. The caller has to ensure that no
// term argument appears twice.
func (p *memPostings) add(id uint64, lset labels.Labels) {
	p.mtx.Lock()

	for _, l := range lset {
		p.m[l] = append(p.m[l], id)
	}
	p.m[allLabel] = append(p.m[allLabel], id)

	p.mtx.Unlock()
}

// Postings provides iterative access over a postings list.
type Postings interface {
	// Next advances the iterator and returns true if another value was found.
	Next() bool

	// Seek advances the iterator to value v or greater and returns
	// true if a value was found.
	Seek(v uint64) bool

	// At returns the value at the current iterator position.
	At() uint64

	// Err returns the last error of the iterator.
	Err() error
}

// errPostings is an empty iterator that always errors.
type errPostings struct {
	err error
}

func (e errPostings) Next() bool       { return false }
func (e errPostings) Seek(uint64) bool { return false }
func (e errPostings) At() uint64       { return 0 }
func (e errPostings) Err() error       { return e.err }

var emptyPostings = errPostings{}

// Intersect returns a new postings list over the intersection of the
// input postings.
func Intersect(its ...Postings) Postings {
	if len(its) == 0 {
		return emptyPostings
	}
	if len(its) == 1 {
		return its[0]
	}
	l := len(its) / 2
	return newIntersectPostings(Intersect(its[:l]...), Intersect(its[l:]...))
}

type intersectPostings struct {
	a, b     Postings
	aok, bok bool
	cur      uint64
}

func newIntersectPostings(a, b Postings) *intersectPostings {
	return &intersectPostings{a: a, b: b}
}

func (it *intersectPostings) At() uint64 {
	return it.cur
}

func (it *intersectPostings) doNext(id uint64) bool {
	for {
		if !it.b.Seek(id) {
			return false
		}
		if vb := it.b.At(); vb != id {
			if !it.a.Seek(vb) {
				return false
			}
			id = it.a.At()
			if vb != id {
				continue
			}
		}
		it.cur = id
		return true
	}
}

func (it *intersectPostings) Next() bool {
	if !it.a.Next() {
		return false
	}
	return it.doNext(it.a.At())
}

func (it *intersectPostings) Seek(id uint64) bool {
	if !it.a.Seek(id) {
		return false
	}
	return it.doNext(it.a.At())
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
	if len(its) == 1 {
		return its[0]
	}
	l := len(its) / 2
	return newMergedPostings(Merge(its[:l]...), Merge(its[l:]...))
}

type mergedPostings struct {
	a, b        Postings
	initialized bool
	aok, bok    bool
	cur         uint64
}

func newMergedPostings(a, b Postings) *mergedPostings {
	return &mergedPostings{a: a, b: b}
}

func (it *mergedPostings) At() uint64 {
	return it.cur
}

func (it *mergedPostings) Next() bool {
	if !it.initialized {
		it.aok = it.a.Next()
		it.bok = it.b.Next()
		it.initialized = true
	}

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
	} else if acur > bcur {
		it.cur = bcur
		it.bok = it.b.Next()
	} else {
		it.cur = acur
		it.aok = it.a.Next()
		it.bok = it.b.Next()
	}
	return true
}

func (it *mergedPostings) Seek(id uint64) bool {
	if it.cur >= id {
		return true
	}

	it.aok = it.a.Seek(id)
	it.bok = it.b.Seek(id)
	it.initialized = true

	return it.Next()
}

func (it *mergedPostings) Err() error {
	if it.a.Err() != nil {
		return it.a.Err()
	}
	return it.b.Err()
}

// listPostings implements the Postings interface over a plain list.
type listPostings struct {
	list []uint64
	cur  uint64
}

func newListPostings(list []uint64) *listPostings {
	return &listPostings{list: list}
}

func (it *listPostings) At() uint64 {
	return it.cur
}

func (it *listPostings) Next() bool {
	if len(it.list) > 0 {
		it.cur = it.list[0]
		it.list = it.list[1:]
		return true
	}
	it.cur = 0
	return false
}

func (it *listPostings) Seek(x uint64) bool {
	// If the current value satisfies, then return.
	if it.cur >= x {
		return true
	}

	// Do binary search between current position and end.
	i := sort.Search(len(it.list), func(i int) bool {
		return it.list[i] >= x
	})
	if i < len(it.list) {
		it.cur = it.list[i]
		it.list = it.list[i+1:]
		return true
	}
	it.list = nil
	return false
}

func (it *listPostings) Err() error {
	return nil
}

// bigEndianPostings implements the Postings interface over a byte stream of
// big endian numbers.
type bigEndianPostings struct {
	list []byte
	cur  uint32
}

func newBigEndianPostings(list []byte) *bigEndianPostings {
	return &bigEndianPostings{list: list}
}

func (it *bigEndianPostings) At() uint64 {
	return uint64(it.cur)
}

func (it *bigEndianPostings) Next() bool {
	if len(it.list) >= 4 {
		it.cur = binary.BigEndian.Uint32(it.list)
		it.list = it.list[4:]
		return true
	}
	return false
}

func (it *bigEndianPostings) Seek(x uint64) bool {
	if uint64(it.cur) >= x {
		return true
	}

	num := len(it.list) / 4
	// Do binary search between current position and end.
	i := sort.Search(num, func(i int) bool {
		return binary.BigEndian.Uint32(it.list[i*4:]) >= uint32(x)
	})
	if i < num {
		j := i * 4
		it.cur = binary.BigEndian.Uint32(it.list[j:])
		it.list = it.list[j+4:]
		return true
	}
	it.list = nil
	return false
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
