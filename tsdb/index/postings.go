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

package index

import (
	"container/heap"
	"encoding/binary"
	"github.com/prometheus/prometheus/pkg/labels"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/RoaringBitmap/roaring/roaring64"
)

var allPostingsKey = labels.Label{}

// AllPostingsKey returns the label key that is used to store the postings list of all existing IDs.
func AllPostingsKey() (name, value string) {
	return allPostingsKey.Name, allPostingsKey.Value
}

// MemPostings holds postings list for series ID per label pair. They may be written
// to out of order.
// ensureOptimize() should be called once before any reads are done.
// after the optimization will have less memory usage and better search performance.
type MemPostings struct {
	mtx       sync.RWMutex
	m         map[string]map[string]*RoaringBitmapPosting
	optimized bool
}

// NewMemPostings returns a memPostings that's ready for reads and writes.
func NewMemPostings() *MemPostings {
	return &MemPostings{
		m:         make(map[string]map[string]*RoaringBitmapPosting, 512),
		optimized: true,
	}
}

// NewUnorderedMemPostings returns a memPostings that is not safe to be read from
// until ensureOrder was called once.
func NewUnorderedMemPostings() *MemPostings {
	return &MemPostings{
		m:         make(map[string]map[string]*RoaringBitmapPosting, 512),
		optimized: false,
	}
}

// SortedKeys returns a list of sorted label keys of the postings.
func (p *MemPostings) SortedKeys() []labels.Label {
	p.mtx.RLock()
	keys := make([]labels.Label, 0, len(p.m))

	for n, e := range p.m {
		for v := range e {
			keys = append(keys, labels.Label{Name: n, Value: v})
		}
	}
	p.mtx.RUnlock()

	sort.Slice(keys, func(i, j int) bool {
		if d := strings.Compare(keys[i].Name, keys[j].Name); d != 0 {
			return d < 0
		}
		return keys[i].Value < keys[j].Value
	})
	return keys
}

// PostingsStats contains cardinality based statistics for postings.
type PostingsStats struct {
	CardinalityMetricsStats []Stat
	CardinalityLabelStats   []Stat
	LabelValueStats         []Stat
	LabelValuePairsStats    []Stat
}

// Stats calculates the cardinality statistics from postings.
func (p *MemPostings) Stats(label string) *PostingsStats {
	const maxNumOfRecords = 10
	var size uint64

	p.mtx.RLock()

	metrics := &maxHeap{}
	labels := &maxHeap{}
	labelValueLength := &maxHeap{}
	labelValuePairs := &maxHeap{}

	metrics.init(maxNumOfRecords)
	labels.init(maxNumOfRecords)
	labelValueLength.init(maxNumOfRecords)
	labelValuePairs.init(maxNumOfRecords)

	for n, e := range p.m {
		if n == "" {
			continue
		}
		labels.push(Stat{Name: n, Count: uint64(len(e))})
		size = 0
		for name, values := range e {
			if n == label {
				metrics.push(Stat{Name: name, Count: values.Size()})
			}
			labelValuePairs.push(Stat{Name: n + "=" + name, Count: values.Size()})
			size += uint64(len(name))
		}
		labelValueLength.push(Stat{Name: n, Count: size})
	}

	p.mtx.RUnlock()

	return &PostingsStats{
		CardinalityMetricsStats: metrics.get(),
		CardinalityLabelStats:   labels.get(),
		LabelValueStats:         labelValueLength.get(),
		LabelValuePairsStats:    labelValuePairs.get(),
	}
}

// Get returns a postings list for the given label pair.
func (p *MemPostings) Get(name, value string) Postings {
	var lp *RoaringBitmapPosting
	p.mtx.RLock()
	l := p.m[name]
	if l != nil {
		lp = l[value]
	}
	p.mtx.RUnlock()

	if lp == nil {
		return EmptyPostings()
	}

	return NewRoaringBitmapIterator(lp)
}

// All returns a postings list over all documents ever added.
func (p *MemPostings) All() Postings {
	return p.Get(AllPostingsKey())
}

// EnsureOptimize ensures that the postings lists are optimized for search.
func (p *MemPostings) EnsureOptimize() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.optimized {
		return
	}

	n := runtime.GOMAXPROCS(0)
	workc := make(chan *RoaringBitmapPosting)

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			for l := range workc {
				l.m.RunOptimize()
			}
			wg.Done()
		}()
	}

	for _, e := range p.m {
		for _, l := range e {
			workc <- l
		}
	}
	close(workc)
	wg.Wait()

	p.optimized = true
}

// Delete removes all ids in the given map from the postings lists.
func (p *MemPostings) Delete(deleted map[uint64]struct{}) {
	var keys, vals []string

	// Collect all keys relevant for deletion once. New keys added afterwards
	// can by definition not be affected by any of the given deletes.
	p.mtx.RLock()
	for n := range p.m {
		keys = append(keys, n)
	}
	p.mtx.RUnlock()

	for _, n := range keys {
		p.mtx.RLock()
		vals = vals[:0]
		for v := range p.m[n] {
			vals = append(vals, v)
		}
		p.mtx.RUnlock()

		// For each posting we first analyse whether the postings list is affected by the deletes.
		// If yes, we modify the postings and re-run optimize.
		var valuesToDelete []uint64
		for _, l := range vals {
			// Only lock for processing one postings list so we don't block reads for too long.
			p.mtx.Lock()

			var list = p.m[n][l]
			valuesToDelete = valuesToDelete[:0]

			for iter := NewRoaringBitmapIterator(list); iter.Next(); {
				var value = iter.At()
				if _, ok := deleted[value]; ok {
					valuesToDelete = append(valuesToDelete, value)
				}
			}

			if len(valuesToDelete) == 0 {
				p.mtx.Unlock()
				continue
			}

			// Check if the entire postings will be deleted.
			// if the postings list will be delete just drop the post list
			if int(list.Size())-len(valuesToDelete) <= 0 {
				delete(p.m[n], l)
			} else {
				for _, toDelete := range valuesToDelete {
					list.Remove(toDelete)
				}
				list.m.RunOptimize()
			}

			p.mtx.Unlock()
		}
		p.mtx.Lock()
		if len(p.m[n]) == 0 {
			delete(p.m, n)
		}
		p.mtx.Unlock()
	}
}

// Iter calls f for each postings list. It aborts if f returns an error and returns it.
func (p *MemPostings) Iter(f func(labels.Label, Postings) error) error {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for n, e := range p.m {
		for v, p := range e {
			if err := f(labels.Label{Name: n, Value: v}, NewRoaringBitmapIterator(p)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Add a label set to the postings index.
func (p *MemPostings) Add(id uint64, lset labels.Labels) {
	p.mtx.Lock()

	for _, l := range lset {
		p.addFor(id, l)
	}
	p.addFor(id, allPostingsKey)

	p.mtx.Unlock()
}

func (p *MemPostings) addFor(id uint64, l labels.Label) {
	nm, ok := p.m[l.Name]
	if !ok {
		nm = map[string]*RoaringBitmapPosting{}
		p.m[l.Name] = nm
	}

	nmm, ok := nm[l.Value]
	if !ok {
		nmm = newRoaringBitmapPosting(id)
		nm[l.Value] = nmm
	} else {
		nmm.Add(id)
	}
}

// ExpandPostings returns the postings expanded as a slice.
func ExpandPostings(p Postings) (res []uint64, err error) {
	for p.Next() {
		res = append(res, p.At())
	}
	return res, p.Err()
}

// Postings provides iterative access over a postings list.
type Postings interface {
	// Next advances the iterator and returns true if another value was found.
	Next() bool

	// Seek advances the iterator to value v or greater and returns
	// true if postings remaining.
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

// EmptyPostings returns a postings list that's always empty.
// NOTE: Returning EmptyPostings sentinel when index.Postings struct has no postings is recommended.
// It triggers optimized flow in other functions like Intersect, Without etc.
func EmptyPostings() Postings {
	return emptyPostings
}

// ErrPostings returns new postings that immediately error.
func ErrPostings(err error) Postings {
	return errPostings{err}
}

// Intersect returns a new postings list over the intersection of the
// input postings.
func Intersect(its ...Postings) Postings {
	if len(its) == 0 {
		return EmptyPostings()
	}
	if len(its) == 1 {
		return its[0]
	}
	for _, p := range its {
		if p == EmptyPostings() {
			return EmptyPostings()
		}
	}

	return newIntersectPostings(its...)
}

type intersectPostings struct {
	arr []Postings
	cur uint64
}

func newIntersectPostings(its ...Postings) *intersectPostings {
	return &intersectPostings{arr: its}
}

func (it *intersectPostings) At() uint64 {
	return it.cur
}

func (it *intersectPostings) doNext() bool {
Loop:
	for {
		for _, p := range it.arr {
			if !p.Seek(it.cur) {
				return false
			}
			if p.At() > it.cur {
				it.cur = p.At()
				continue Loop
			}
		}
		return true
	}
}

func (it *intersectPostings) Next() bool {
	for _, p := range it.arr {
		if !p.Next() {
			return false
		}
		if p.At() > it.cur {
			it.cur = p.At()
		}
	}
	return it.doNext()
}

func (it *intersectPostings) Seek(id uint64) bool {
	it.cur = id
	return it.doNext()
}

func (it *intersectPostings) Err() error {
	for _, p := range it.arr {
		if p.Err() != nil {
			return p.Err()
		}
	}
	return nil
}

// Merge returns a new iterator over the union of the input iterators.
func Merge(its ...Postings) Postings {
	if len(its) == 0 {
		return EmptyPostings()
	}
	if len(its) == 1 {
		return its[0]
	}

	p, ok := newMergedPostings(its)
	if !ok {
		return EmptyPostings()
	}
	return p
}

type postingsHeap []Postings

func (h postingsHeap) Len() int           { return len(h) }
func (h postingsHeap) Less(i, j int) bool { return h[i].At() < h[j].At() }
func (h *postingsHeap) Swap(i, j int)     { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }

func (h *postingsHeap) Push(x interface{}) {
	*h = append(*h, x.(Postings))
}

func (h *postingsHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type mergedPostings struct {
	h           postingsHeap
	initialized bool
	cur         uint64
	err         error
}

func newMergedPostings(p []Postings) (m *mergedPostings, nonEmpty bool) {
	ph := make(postingsHeap, 0, len(p))

	for _, it := range p {
		// NOTE: mergedPostings struct requires the user to issue an initial Next.
		if it.Next() {
			ph = append(ph, it)
		} else {
			if it.Err() != nil {
				return &mergedPostings{err: it.Err()}, true
			}
		}
	}

	if len(ph) == 0 {
		return nil, false
	}
	return &mergedPostings{h: ph}, true
}

func (it *mergedPostings) Next() bool {
	if it.h.Len() == 0 || it.err != nil {
		return false
	}

	// The user must issue an initial Next.
	if !it.initialized {
		heap.Init(&it.h)
		it.cur = it.h[0].At()
		it.initialized = true
		return true
	}

	for {
		cur := it.h[0]
		if !cur.Next() {
			heap.Pop(&it.h)
			if cur.Err() != nil {
				it.err = cur.Err()
				return false
			}
			if it.h.Len() == 0 {
				return false
			}
		} else {
			// Value of top of heap has changed, re-heapify.
			heap.Fix(&it.h, 0)
		}

		if it.h[0].At() != it.cur {
			it.cur = it.h[0].At()
			return true
		}
	}
}

func (it *mergedPostings) Seek(id uint64) bool {
	if it.h.Len() == 0 || it.err != nil {
		return false
	}
	if !it.initialized {
		if !it.Next() {
			return false
		}
	}
	for it.cur < id {
		cur := it.h[0]
		if !cur.Seek(id) {
			heap.Pop(&it.h)
			if cur.Err() != nil {
				it.err = cur.Err()
				return false
			}
			if it.h.Len() == 0 {
				return false
			}
		} else {
			// Value of top of heap has changed, re-heapify.
			heap.Fix(&it.h, 0)
		}

		it.cur = it.h[0].At()
	}
	return true
}

func (it mergedPostings) At() uint64 {
	return it.cur
}

func (it mergedPostings) Err() error {
	return it.err
}

// Without returns a new postings list that contains all elements from the full list that
// are not in the drop list.
func Without(full, drop Postings) Postings {
	if full == EmptyPostings() {
		return EmptyPostings()
	}

	if drop == EmptyPostings() {
		return full
	}
	return newRemovedPostings(full, drop)
}

type removedPostings struct {
	full, remove Postings

	cur uint64

	initialized bool
	fok, rok    bool
}

func newRemovedPostings(full, remove Postings) *removedPostings {
	return &removedPostings{
		full:   full,
		remove: remove,
	}
}

func (rp *removedPostings) At() uint64 {
	return rp.cur
}

func (rp *removedPostings) Next() bool {
	if !rp.initialized {
		rp.fok = rp.full.Next()
		rp.rok = rp.remove.Next()
		rp.initialized = true
	}
	for {
		if !rp.fok {
			return false
		}

		if !rp.rok {
			rp.cur = rp.full.At()
			rp.fok = rp.full.Next()
			return true
		}

		fcur, rcur := rp.full.At(), rp.remove.At()
		if fcur < rcur {
			rp.cur = fcur
			rp.fok = rp.full.Next()

			return true
		} else if rcur < fcur {
			// Forward the remove postings to the right position.
			rp.rok = rp.remove.Seek(fcur)
		} else {
			// Skip the current posting.
			rp.fok = rp.full.Next()
		}
	}
}

func (rp *removedPostings) Seek(id uint64) bool {
	if rp.cur >= id {
		return true
	}

	rp.fok = rp.full.Seek(id)
	rp.rok = rp.remove.Seek(id)
	rp.initialized = true

	return rp.Next()
}

func (rp *removedPostings) Err() error {
	if rp.full.Err() != nil {
		return rp.full.Err()
	}

	return rp.remove.Err()
}

// ListPostings implements the Postings interface over a plain list.
type ListPostings struct {
	list []uint64
	cur  uint64
}

func NewListPostings(list []uint64) Postings {
	return newListPostings(list...)
}

func newListPostings(list ...uint64) *ListPostings {
	return &ListPostings{list: list}
}

func (it *ListPostings) At() uint64 {
	return it.cur
}

func (it *ListPostings) Next() bool {
	if len(it.list) > 0 {
		it.cur = it.list[0]
		it.list = it.list[1:]
		return true
	}
	it.cur = 0
	return false
}

func (it *ListPostings) Seek(x uint64) bool {
	// If the current value satisfies, then return.
	if it.cur >= x {
		return true
	}
	if len(it.list) == 0 {
		return false
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

func (it *ListPostings) Err() error {
	return nil
}

// RoaringBitmapPosting implements the Postings interface over a roaring bitmap.
type RoaringBitmapPosting struct {
	count uint64 // RoaringBitmap can not write in parallel, the counter can use simple uint64
	m     roaring64.Bitmap
}

func newRoaringBitmapPosting(ls ...uint64) *RoaringBitmapPosting {
	var m = roaring64.Bitmap{}
	m.AddMany(ls)
	return &RoaringBitmapPosting{m: m, count: uint64(len(ls))}
}

func (r *RoaringBitmapPosting) isEmpty() bool {
	return r.Size() == 0
}

func (r *RoaringBitmapPosting) Add(l ...uint64) {
	r.count += uint64(len(l))

	// fast path for add only 1 item
	// the Add api is 0-alloc
	if len(l) == 1 {
		r.m.Add(l[0])
		return
	}
	r.m.AddMany(l)
}

func (r *RoaringBitmapPosting) Remove(l uint64) {
	r.count--
	r.m.Remove(l)
}

func (r *RoaringBitmapPosting) Size() uint64 {
	return r.count
}

// NewRoaringBitmapIterator return a iterator can iter the posting in order.
func NewRoaringBitmapIterator(r *RoaringBitmapPosting) Postings {
	return &RoaringBitmapPostingListIterator{r: r.m.Iterator()}
}

type RoaringBitmapPostingListIterator struct {
	r   roaring64.IntPeekable64
	cur uint64
}

func (r *RoaringBitmapPostingListIterator) Next() bool {
	if r.r.HasNext() {
		r.cur = r.r.Next()
		return true
	}
	return false
}

func (r *RoaringBitmapPostingListIterator) Seek(v uint64) bool {
	// If the current value satisfies, then return.
	if r.cur >= v {
		return true
	}

	r.r.AdvanceIfNeeded(v)
	if r.r.HasNext() {
		r.cur = r.r.Next()
		return true
	}
	return false
}

func (r RoaringBitmapPostingListIterator) At() uint64 {
	return r.cur
}

func (r RoaringBitmapPostingListIterator) Err() error {
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
