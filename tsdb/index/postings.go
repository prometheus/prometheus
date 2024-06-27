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
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/bboreham/go-loser"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

var allPostingsKey = labels.Label{}

// AllPostingsKey returns the label key that is used to store the postings list of all existing IDs.
func AllPostingsKey() (name, value string) {
	return allPostingsKey.Name, allPostingsKey.Value
}

// ensureOrderBatchSize is the max number of postings passed to a worker in a single batch in MemPostings.EnsureOrder().
const ensureOrderBatchSize = 1024

// ensureOrderBatchPool is a pool used to recycle batches passed to workers in MemPostings.EnsureOrder().
var ensureOrderBatchPool = sync.Pool{
	New: func() interface{} {
		x := make([][]storage.SeriesRef, 0, ensureOrderBatchSize)
		return &x // Return pointer type as preferred by Pool.
	},
}

// MemPostings holds postings list for series ID per label pair. They may be written
// to out of order.
// EnsureOrder() must be called once before any reads are done. This allows for quick
// unordered batch fills on startup.
type MemPostings struct {
	mtx     sync.RWMutex
	m       map[string]map[string][]storage.SeriesRef
	ordered bool
}

// NewMemPostings returns a memPostings that's ready for reads and writes.
func NewMemPostings() *MemPostings {
	return &MemPostings{
		m:       make(map[string]map[string][]storage.SeriesRef, 512),
		ordered: true,
	}
}

// NewUnorderedMemPostings returns a memPostings that is not safe to be read from
// until EnsureOrder() was called once.
func NewUnorderedMemPostings() *MemPostings {
	return &MemPostings{
		m:       make(map[string]map[string][]storage.SeriesRef, 512),
		ordered: false,
	}
}

// Symbols returns an iterator over all unique name and value strings, in order.
func (p *MemPostings) Symbols() StringIter {
	p.mtx.RLock()

	// Add all the strings to a map to de-duplicate.
	symbols := make(map[string]struct{}, 512)
	for n, e := range p.m {
		symbols[n] = struct{}{}
		for v := range e {
			symbols[v] = struct{}{}
		}
	}
	p.mtx.RUnlock()

	res := make([]string, 0, len(symbols))
	for k := range symbols {
		res = append(res, k)
	}

	slices.Sort(res)
	return NewStringListIter(res)
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

	slices.SortFunc(keys, func(a, b labels.Label) int {
		nameCompare := strings.Compare(a.Name, b.Name)
		// If names are the same, compare values.
		if nameCompare != 0 {
			return nameCompare
		}

		return strings.Compare(a.Value, b.Value)
	})
	return keys
}

// LabelNames returns all the unique label names.
func (p *MemPostings) LabelNames() []string {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	n := len(p.m)
	if n == 0 {
		return nil
	}

	names := make([]string, 0, n-1)
	for name := range p.m {
		if name != allPostingsKey.Name {
			names = append(names, name)
		}
	}
	return names
}

// LabelValues returns label values for the given name.
func (p *MemPostings) LabelValues(_ context.Context, name string) []string {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	values := make([]string, 0, len(p.m[name]))
	for v := range p.m[name] {
		values = append(values, v)
	}
	return values
}

// PostingsStats contains cardinality based statistics for postings.
type PostingsStats struct {
	CardinalityMetricsStats []Stat
	CardinalityLabelStats   []Stat
	LabelValueStats         []Stat
	LabelValuePairsStats    []Stat
	NumLabelPairs           int
}

// Stats calculates the cardinality statistics from postings.
func (p *MemPostings) Stats(label string, limit int) *PostingsStats {
	var size uint64
	p.mtx.RLock()

	metrics := &maxHeap{}
	labels := &maxHeap{}
	labelValueLength := &maxHeap{}
	labelValuePairs := &maxHeap{}
	numLabelPairs := 0

	metrics.init(limit)
	labels.init(limit)
	labelValueLength.init(limit)
	labelValuePairs.init(limit)

	for n, e := range p.m {
		if n == "" {
			continue
		}
		labels.push(Stat{Name: n, Count: uint64(len(e))})
		numLabelPairs += len(e)
		size = 0
		for name, values := range e {
			if n == label {
				metrics.push(Stat{Name: name, Count: uint64(len(values))})
			}
			seriesCnt := uint64(len(values))
			labelValuePairs.push(Stat{Name: n + "=" + name, Count: seriesCnt})
			size += uint64(len(name)) * seriesCnt
		}
		labelValueLength.push(Stat{Name: n, Count: size})
	}

	p.mtx.RUnlock()

	return &PostingsStats{
		CardinalityMetricsStats: metrics.get(),
		CardinalityLabelStats:   labels.get(),
		LabelValueStats:         labelValueLength.get(),
		LabelValuePairsStats:    labelValuePairs.get(),
		NumLabelPairs:           numLabelPairs,
	}
}

// Get returns a postings list for the given label pair.
func (p *MemPostings) Get(name, value string) Postings {
	var lp []storage.SeriesRef
	p.mtx.RLock()
	l := p.m[name]
	if l != nil {
		lp = l[value]
	}
	p.mtx.RUnlock()

	if lp == nil {
		return EmptyPostings()
	}
	return newListPostings(lp...)
}

// All returns a postings list over all documents ever added.
func (p *MemPostings) All() Postings {
	return p.Get(AllPostingsKey())
}

// EnsureOrder ensures that all postings lists are sorted. After it returns all further
// calls to add and addFor will insert new IDs in a sorted manner.
// Parameter numberOfConcurrentProcesses is used to specify the maximal number of
// CPU cores used for this operation. If it is <= 0, GOMAXPROCS is used.
// GOMAXPROCS was the default before introducing this parameter.
func (p *MemPostings) EnsureOrder(numberOfConcurrentProcesses int) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.ordered {
		return
	}

	concurrency := numberOfConcurrentProcesses
	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
	}
	workc := make(chan *[][]storage.SeriesRef)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			for job := range workc {
				for _, l := range *job {
					slices.Sort(l)
				}

				*job = (*job)[:0]
				ensureOrderBatchPool.Put(job)
			}
			wg.Done()
		}()
	}

	nextJob := ensureOrderBatchPool.Get().(*[][]storage.SeriesRef)
	for _, e := range p.m {
		for _, l := range e {
			*nextJob = append(*nextJob, l)

			if len(*nextJob) >= ensureOrderBatchSize {
				workc <- nextJob
				nextJob = ensureOrderBatchPool.Get().(*[][]storage.SeriesRef)
			}
		}
	}

	// If the last job was partially filled, we need to push it to workers too.
	if len(*nextJob) > 0 {
		workc <- nextJob
	}

	close(workc)
	wg.Wait()

	p.ordered = true
}

// Delete removes all ids in the given map from the postings lists.
// affectedLabels contains all the labels that are affected by the deletion, there's no need to check other labels.
func (p *MemPostings) Delete(deleted map[storage.SeriesRef]struct{}, affected map[labels.Label]struct{}) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	process := func(l labels.Label) {
		orig := p.m[l.Name][l.Value]
		repl := make([]storage.SeriesRef, 0, len(orig))
		for _, id := range orig {
			if _, ok := deleted[id]; !ok {
				repl = append(repl, id)
			}
		}
		if len(repl) > 0 {
			p.m[l.Name][l.Value] = repl
		} else {
			delete(p.m[l.Name], l.Value)
			// Delete the key if we removed all values.
			if len(p.m[l.Name]) == 0 {
				delete(p.m, l.Name)
			}
		}
	}

	for l := range affected {
		process(l)
	}
	process(allPostingsKey)
}

// Iter calls f for each postings list. It aborts if f returns an error and returns it.
func (p *MemPostings) Iter(f func(labels.Label, Postings) error) error {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for n, e := range p.m {
		for v, p := range e {
			if err := f(labels.Label{Name: n, Value: v}, newListPostings(p...)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Add a label set to the postings index.
func (p *MemPostings) Add(id storage.SeriesRef, lset labels.Labels) {
	p.mtx.Lock()

	lset.Range(func(l labels.Label) {
		p.addFor(id, l)
	})
	p.addFor(id, allPostingsKey)

	p.mtx.Unlock()
}

func (p *MemPostings) addFor(id storage.SeriesRef, l labels.Label) {
	nm, ok := p.m[l.Name]
	if !ok {
		nm = map[string][]storage.SeriesRef{}
		p.m[l.Name] = nm
	}
	list := append(nm[l.Value], id)
	nm[l.Value] = list

	if !p.ordered {
		return
	}
	// There is no guarantee that no higher ID was inserted before as they may
	// be generated independently before adding them to postings.
	// We repair order violations on insert. The invariant is that the first n-1
	// items in the list are already sorted.
	for i := len(list) - 1; i >= 1; i-- {
		if list[i] >= list[i-1] {
			break
		}
		list[i], list[i-1] = list[i-1], list[i]
	}
}

func (p *MemPostings) PostingsForLabelMatching(ctx context.Context, name string, match func(string) bool) Postings {
	// We'll copy the values into a slice and then match over that,
	// this way we don't need to hold the mutex while we're matching,
	// which can be slow (seconds) if the match function is a huge regex.
	// Holding this lock prevents new series from being added (slows down the write path)
	// and blocks the compaction process.
	vals := p.labelValues(name)
	for i, count := 0, 1; i < len(vals); count++ {
		if count%checkContextEveryNIterations == 0 && ctx.Err() != nil {
			return ErrPostings(ctx.Err())
		}

		if match(vals[i]) {
			i++
			continue
		}

		// Didn't match, bring the last value to this position, make the slice shorter and check again.
		// The order of the slice doesn't matter as it comes from a map iteration.
		vals[i], vals = vals[len(vals)-1], vals[:len(vals)-1]
	}

	// If none matched (or this label had no values), no need to grab the lock again.
	if len(vals) == 0 {
		return EmptyPostings()
	}

	// Now `vals` only contains the values that matched, get their postings.
	its := make([]Postings, 0, len(vals))
	p.mtx.RLock()
	e := p.m[name]
	for _, v := range vals {
		if refs, ok := e[v]; ok {
			// Some of the values may have been garbage-collected in the meantime this is fine, we'll just skip them.
			// If we didn't let the mutex go, we'd have these postings here, but they would be pointing nowhere
			// because there would be a `MemPostings.Delete()` call waiting for the lock to delete these labels,
			// because the series were deleted already.
			its = append(its, NewListPostings(refs))
		}
	}
	// Let the mutex go before merging.
	p.mtx.RUnlock()

	return Merge(ctx, its...)
}

// labelValues returns a slice of label values for the given label name.
// It will take the read lock.
func (p *MemPostings) labelValues(name string) []string {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	e := p.m[name]
	if len(e) == 0 {
		return nil
	}

	vals := make([]string, 0, len(e))
	for v, srs := range e {
		if len(srs) > 0 {
			vals = append(vals, v)
		}
	}

	return vals
}

// ExpandPostings returns the postings expanded as a slice.
func ExpandPostings(p Postings) (res []storage.SeriesRef, err error) {
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
	// true if a value was found.
	Seek(v storage.SeriesRef) bool

	// At returns the value at the current iterator position.
	// At should only be called after a successful call to Next or Seek.
	At() storage.SeriesRef

	// Err returns the last error of the iterator.
	Err() error
}

// errPostings is an empty iterator that always errors.
type errPostings struct {
	err error
}

func (e errPostings) Next() bool                  { return false }
func (e errPostings) Seek(storage.SeriesRef) bool { return false }
func (e errPostings) At() storage.SeriesRef       { return 0 }
func (e errPostings) Err() error                  { return e.err }

var emptyPostings = errPostings{}

// EmptyPostings returns a postings list that's always empty.
// NOTE: Returning EmptyPostings sentinel when Postings struct has no postings is recommended.
// It triggers optimized flow in other functions like Intersect, Without etc.
func EmptyPostings() Postings {
	return emptyPostings
}

// IsEmptyPostingsType returns true if the postings are an empty postings list.
// When this function returns false, it doesn't mean that the postings isn't empty
// (it could be an empty intersection of two non-empty postings, for example).
func IsEmptyPostingsType(p Postings) bool {
	return p == emptyPostings
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
	cur storage.SeriesRef
}

func newIntersectPostings(its ...Postings) *intersectPostings {
	return &intersectPostings{arr: its}
}

func (it *intersectPostings) At() storage.SeriesRef {
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

func (it *intersectPostings) Seek(id storage.SeriesRef) bool {
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
func Merge(_ context.Context, its ...Postings) Postings {
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

type mergedPostings struct {
	p   []Postings
	h   *loser.Tree[storage.SeriesRef, Postings]
	cur storage.SeriesRef
}

func newMergedPostings(p []Postings) (m *mergedPostings, nonEmpty bool) {
	const maxVal = storage.SeriesRef(math.MaxUint64) // This value must be higher than all real values used in the tree.
	lt := loser.New(p, maxVal)
	return &mergedPostings{p: p, h: lt}, true
}

func (it *mergedPostings) Next() bool {
	for {
		if !it.h.Next() {
			return false
		}
		// Remove duplicate entries.
		newItem := it.h.At()
		if newItem != it.cur {
			it.cur = newItem
			return true
		}
	}
}

func (it *mergedPostings) Seek(id storage.SeriesRef) bool {
	for !it.h.IsEmpty() && it.h.At() < id {
		finished := !it.h.Winner().Seek(id)
		it.h.Fix(finished)
	}
	if it.h.IsEmpty() {
		return false
	}
	it.cur = it.h.At()
	return true
}

func (it mergedPostings) At() storage.SeriesRef {
	return it.cur
}

func (it mergedPostings) Err() error {
	for _, p := range it.p {
		if err := p.Err(); err != nil {
			return err
		}
	}
	return nil
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

	cur storage.SeriesRef

	initialized bool
	fok, rok    bool
}

func newRemovedPostings(full, remove Postings) *removedPostings {
	return &removedPostings{
		full:   full,
		remove: remove,
	}
}

func (rp *removedPostings) At() storage.SeriesRef {
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
		switch fcur, rcur := rp.full.At(), rp.remove.At(); {
		case fcur < rcur:
			rp.cur = fcur
			rp.fok = rp.full.Next()

			return true
		case rcur < fcur:
			// Forward the remove postings to the right position.
			rp.rok = rp.remove.Seek(fcur)
		default:
			// Skip the current posting.
			rp.fok = rp.full.Next()
		}
	}
}

func (rp *removedPostings) Seek(id storage.SeriesRef) bool {
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
	list []storage.SeriesRef
	cur  storage.SeriesRef
}

func NewListPostings(list []storage.SeriesRef) Postings {
	return newListPostings(list...)
}

func newListPostings(list ...storage.SeriesRef) *ListPostings {
	return &ListPostings{list: list}
}

func (it *ListPostings) At() storage.SeriesRef {
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

func (it *ListPostings) Seek(x storage.SeriesRef) bool {
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

// bigEndianPostings implements the Postings interface over a byte stream of
// big endian numbers.
type bigEndianPostings struct {
	list []byte
	cur  uint32
}

func newBigEndianPostings(list []byte) *bigEndianPostings {
	return &bigEndianPostings{list: list}
}

func (it *bigEndianPostings) At() storage.SeriesRef {
	return storage.SeriesRef(it.cur)
}

func (it *bigEndianPostings) Next() bool {
	if len(it.list) >= 4 {
		it.cur = binary.BigEndian.Uint32(it.list)
		it.list = it.list[4:]
		return true
	}
	return false
}

func (it *bigEndianPostings) Seek(x storage.SeriesRef) bool {
	if storage.SeriesRef(it.cur) >= x {
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

// FindIntersectingPostings checks the intersection of p and candidates[i] for each i in candidates,
// if intersection is non empty, then i is added to the indexes returned.
// Returned indexes are not sorted.
func FindIntersectingPostings(p Postings, candidates []Postings) (indexes []int, err error) {
	h := make(postingsWithIndexHeap, 0, len(candidates))
	for idx, it := range candidates {
		switch {
		case it.Next():
			h = append(h, postingsWithIndex{index: idx, p: it})
		case it.Err() != nil:
			return nil, it.Err()
		}
	}
	if h.empty() {
		return nil, nil
	}
	heap.Init(&h)

	for !h.empty() {
		if !p.Seek(h.at()) {
			return indexes, p.Err()
		}
		if p.At() == h.at() {
			indexes = append(indexes, h.popIndex())
		} else if err := h.next(); err != nil {
			return nil, err
		}
	}

	return indexes, nil
}

// postingsWithIndex is used as postingsWithIndexHeap elements by FindIntersectingPostings,
// keeping track of the original index of each postings while they move inside the heap.
type postingsWithIndex struct {
	index int
	p     Postings
	// popped means that these postings shouldn't be considered anymore.
	// See popIndex() comment to understand why we need this.
	popped bool
}

// postingsWithIndexHeap implements heap.Interface,
// with root always pointing to the postings with minimum Postings.At() value.
// It also implements a special way of removing elements that marks them as popped and moves them to the bottom of the
// heap instead of actually removing them, see popIndex() for more details.
type postingsWithIndexHeap []postingsWithIndex

// empty checks whether the heap is empty, which is true if it has no elements, of if the smallest element is popped.
func (h *postingsWithIndexHeap) empty() bool {
	return len(*h) == 0 || (*h)[0].popped
}

// popIndex pops the smallest heap element and returns its index.
// In our implementation we don't actually do heap.Pop(), instead we mark the element as `popped` and fix its position, which
// should be after all the non-popped elements according to our sorting strategy.
// By skipping the `heap.Pop()` call we avoid an extra allocation in this heap's Pop() implementation which returns an interface{}.
func (h *postingsWithIndexHeap) popIndex() int {
	index := (*h)[0].index
	(*h)[0].popped = true
	heap.Fix(h, 0)
	return index
}

// at provides the storage.SeriesRef where root Postings is pointing at this moment.
func (h postingsWithIndexHeap) at() storage.SeriesRef { return h[0].p.At() }

// next performs the Postings.Next() operation on the root of the heap, performing the related operation on the heap
// and conveniently returning the result of calling Postings.Err() if the result of calling Next() was false.
// If Next() succeeds, heap is fixed to move the root to its new position, according to its Postings.At() value.
// If Next() returns fails and there's no error reported by Postings.Err(), then root is marked as removed and heap is fixed.
func (h *postingsWithIndexHeap) next() error {
	pi := (*h)[0]
	next := pi.p.Next()
	if next {
		heap.Fix(h, 0)
		return nil
	}

	if err := pi.p.Err(); err != nil {
		return fmt.Errorf("postings %d: %w", pi.index, err)
	}
	h.popIndex()
	return nil
}

// Len implements heap.Interface.
// Notice that Len() > 0 does not imply that heap is not empty as elements are not removed from this heap.
// Use empty() to check whether heap is empty or not.
func (h postingsWithIndexHeap) Len() int { return len(h) }

// Less implements heap.Interface, it puts all the popped elements at the bottom,
// and then sorts by Postings.At() property of each node.
func (h postingsWithIndexHeap) Less(i, j int) bool {
	if h[i].popped != h[j].popped {
		return h[j].popped
	}
	return h[i].p.At() < h[j].p.At()
}

// Swap implements heap.Interface.
func (h *postingsWithIndexHeap) Swap(i, j int) { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }

// Push implements heap.Interface.
func (h *postingsWithIndexHeap) Push(x interface{}) {
	*h = append(*h, x.(postingsWithIndex))
}

// Pop implements heap.Interface and pops the last element, which is NOT the min element,
// so this doesn't return the same heap.Pop()
// Although this method is implemented for correctness, we don't expect it to be used, see popIndex() method for details.
func (h *postingsWithIndexHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
