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
	"maps"
	"math"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bboreham/go-loser"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

const exponentialSliceGrowthFactor = 2

var allPostingsKey = labels.Label{}

// AllPostingsKey returns the label key that is used to store the postings list of all existing IDs.
func AllPostingsKey() (name, value string) {
	return allPostingsKey.Name, allPostingsKey.Value
}

// ensureOrderBatchSize is the max number of postings passed to a worker in a single batch in MemPostings.EnsureOrder().
const ensureOrderBatchSize = 1024

// ensureOrderBatchPool is a pool used to recycle batches passed to workers in MemPostings.EnsureOrder().
var ensureOrderBatchPool = sync.Pool{
	New: func() any {
		x := make([][]storage.SeriesRef, 0, ensureOrderBatchSize)
		return &x // Return pointer type as preferred by Pool.
	},
}

// MemPostings holds postings list for series ID per label pair. They may be written
// to out of order.
// EnsureOrder() must be called once before any reads are done. This allows for quick
// unordered batch fills on startup.
type MemPostings struct {
	mtx sync.RWMutex

	// m holds the postings lists for each label-value pair, indexed first by label name, and then by label value.
	//
	// mtx must be held when interacting with m (the appropriate one for reading or writing).
	// It is safe to retain a reference to a postings list after releasing the lock.
	//
	// BUG: There's currently a data race in addFor, which might modify the tail of the postings list:
	// https://github.com/prometheus/prometheus/issues/15317
	m map[string]map[string][]storage.SeriesRef

	// lvs holds the label values for each label name.
	// lvs[name] is essentially an unsorted append-only list of all keys in m[name]
	// mtx must be held when interacting with lvs.
	// Since it's append-only, it is safe to read the label values slice after releasing the lock.
	lvs map[string][]string

	ordered bool
}

const defaultLabelNamesMapSize = 512

// NewMemPostings returns a memPostings that's ready for reads and writes.
func NewMemPostings() *MemPostings {
	return &MemPostings{
		m:       make(map[string]map[string][]storage.SeriesRef, defaultLabelNamesMapSize),
		lvs:     make(map[string][]string, defaultLabelNamesMapSize),
		ordered: true,
	}
}

// NewUnorderedMemPostings returns a memPostings that is not safe to be read from
// until EnsureOrder() was called once.
func NewUnorderedMemPostings() *MemPostings {
	return &MemPostings{
		m:       make(map[string]map[string][]storage.SeriesRef, defaultLabelNamesMapSize),
		lvs:     make(map[string][]string, defaultLabelNamesMapSize),
		ordered: false,
	}
}

// Symbols returns an iterator over all unique name and value strings, in order.
func (p *MemPostings) Symbols() StringIter {
	p.mtx.RLock()
	// Make a quick clone of the map to avoid holding the lock while iterating.
	// It's safe to use the values of the map after releasing the lock, as they're append-only slices.
	lvs := maps.Clone(p.lvs)
	p.mtx.RUnlock()

	// Add all the strings to a map to de-duplicate.
	symbols := make(map[string]struct{}, defaultLabelNamesMapSize)
	for n, labelValues := range lvs {
		symbols[n] = struct{}{}
		for _, v := range labelValues {
			symbols[v] = struct{}{}
		}
	}

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
func (p *MemPostings) LabelValues(_ context.Context, name string, hints *storage.LabelHints) []string {
	p.mtx.RLock()
	values := p.lvs[name]
	p.mtx.RUnlock()

	if hints != nil && hints.Limit > 0 && len(values) > hints.Limit {
		values = values[:hints.Limit]
	}

	// The slice from p.lvs[name] is shared between all readers, and it is append-only.
	// Since it's shared, we need to make a copy of it before returning it to make
	// sure that no caller modifies the original one by sorting it or filtering it.
	// Since it's append-only, we can do this while not holding the mutex anymore.
	return slices.Clone(values)
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
// Caller can pass in a function which computes the space required for n series with a given label.
func (p *MemPostings) Stats(label string, limit int, labelSizeFunc func(string, string, uint64) uint64) *PostingsStats {
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
			size += labelSizeFunc(n, name, seriesCnt)
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

// All returns a postings list over all documents ever added.
func (p *MemPostings) All() Postings {
	return p.Postings(context.Background(), allPostingsKey.Name, allPostingsKey.Value)
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

	affectedLabelNames := map[string]struct{}{}
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
			affectedLabelNames[l.Name] = struct{}{}
		}
	}

	i := 0
	for l := range affected {
		i++
		process(l)

		// From time to time we want some readers to go through and read their postings.
		// It takes around 50ms to process a 1K series batch, and 120ms to process a 10K series batch (local benchmarks on an M3).
		// Note that a read query will most likely want to read multiple postings lists, say 5, 10 or 20 (depending on the number of matchers)
		// And that read query will most likely evaluate only one of those matchers before we unpause here, so we want to pause often.
		if i%512 == 0 {
			p.unlockWaitAndLockAgain()
		}
	}
	process(allPostingsKey)

	// Now we need to update the label values slices.
	i = 0
	for name := range affectedLabelNames {
		i++
		// From time to time we want some readers to go through and read their postings.
		if i%512 == 0 {
			p.unlockWaitAndLockAgain()
		}

		if len(p.m[name]) == 0 {
			// Delete the label name key if we deleted all values.
			delete(p.m, name)
			delete(p.lvs, name)
			continue
		}

		// Create the new slice with enough room to grow without reallocating.
		// We have deleted values here, so there's definitely some churn, so be prepared for it.
		lvs := make([]string, 0, exponentialSliceGrowthFactor*len(p.m[name]))
		for v := range p.m[name] {
			lvs = append(lvs, v)
		}
		p.lvs[name] = lvs
	}
}

// unlockWaitAndLockAgain will unlock an already locked p.mtx.Lock() and then wait a little bit before locking it again,
// letting the RLock()-waiting goroutines to get the lock.
func (p *MemPostings) unlockWaitAndLockAgain() {
	p.mtx.Unlock()
	// While it's tempting to just do a `time.Sleep(time.Millisecond)` here,
	// it wouldn't ensure use that readers actually were able to get the read lock,
	// because if there are writes waiting on same mutex, readers won't be able to get it.
	// So we just grab one RLock ourselves.
	p.mtx.RLock()
	// We shouldn't wait here, because we would be blocking a potential write for no reason.
	// Note that if there's a writer waiting for us to unlock, no reader will be able to get the read lock.
	p.mtx.RUnlock() //nolint:staticcheck // SA2001: this is an intentionally empty critical section.
	// Now we can wait a little bit just to increase the chance of a reader getting the lock.
	time.Sleep(time.Millisecond)
	p.mtx.Lock()
}

// Iter calls f for each postings list. It aborts if f returns an error and returns it.
func (p *MemPostings) Iter(f func(labels.Label, Postings) error) error {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for n, e := range p.m {
		for v, p := range e {
			if err := f(labels.Label{Name: n, Value: v}, NewListPostings(p)); err != nil {
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

func appendWithExponentialGrowth[T any](a []T, v T) []T {
	if cap(a) < len(a)+1 {
		newList := make([]T, len(a), len(a)*exponentialSliceGrowthFactor+1)
		copy(newList, a)
		a = newList
	}
	return append(a, v)
}

func (p *MemPostings) addFor(id storage.SeriesRef, l labels.Label) {
	nm, ok := p.m[l.Name]
	if !ok {
		nm = map[string][]storage.SeriesRef{}
		p.m[l.Name] = nm
	}
	vm, ok := nm[l.Value]
	if !ok {
		p.lvs[l.Name] = appendWithExponentialGrowth(p.lvs[l.Name], l.Value)
	}
	list := appendWithExponentialGrowth(vm, id)
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
	// We'll take the label values slice and then match over that,
	// this way we don't need to hold the mutex while we're matching,
	// which can be slow (seconds) if the match function is a huge regex.
	// Holding this lock prevents new series from being added (slows down the write path)
	// and blocks the compaction process.
	//
	// We just need to make sure we don't modify the slice we took,
	// so we'll append matching values to a different one.
	p.mtx.RLock()
	readOnlyLabelValues := p.lvs[name]
	p.mtx.RUnlock()

	vals := make([]string, 0, len(readOnlyLabelValues))
	for i, v := range readOnlyLabelValues {
		if i%checkContextEveryNIterations == 0 && ctx.Err() != nil {
			return ErrPostings(ctx.Err())
		}

		if match(v) {
			vals = append(vals, v)
		}
	}

	// If none matched (or this label had no values), no need to grab the lock again.
	if len(vals) == 0 {
		return EmptyPostings()
	}

	// Now `vals` only contains the values that matched, get their postings.
	its := make([]*listPostings, 0, len(vals))
	lps := make([]listPostings, len(vals))
	p.mtx.RLock()
	e := p.m[name]
	for i, v := range vals {
		if refs, ok := e[v]; ok {
			// Some of the values may have been garbage-collected in the meantime this is fine, we'll just skip them.
			// If we didn't let the mutex go, we'd have these postings here, but they would be pointing nowhere
			// because there would be a `MemPostings.Delete()` call waiting for the lock to delete these labels,
			// because the series were deleted already.
			lps[i] = listPostings{list: refs}
			its = append(its, &lps[i])
		}
	}
	// Let the mutex go before merging.
	p.mtx.RUnlock()

	return Merge(ctx, its...)
}

// Postings returns a postings iterator for the given label values.
func (p *MemPostings) Postings(ctx context.Context, name string, values ...string) Postings {
	res := make([]*listPostings, 0, len(values))
	lps := make([]listPostings, len(values))
	p.mtx.RLock()
	postingsMapForName := p.m[name]
	for i, value := range values {
		if lp := postingsMapForName[value]; lp != nil {
			lps[i] = listPostings{list: lp}
			res = append(res, &lps[i])
		}
	}
	p.mtx.RUnlock()
	return Merge(ctx, res...)
}

func (p *MemPostings) PostingsForAllLabelValues(ctx context.Context, name string) Postings {
	p.mtx.RLock()

	e := p.m[name]
	its := make([]*listPostings, 0, len(e))
	lps := make([]listPostings, len(e))
	i := 0
	for _, refs := range e {
		if len(refs) > 0 {
			lps[i] = listPostings{list: refs}
			its = append(its, &lps[i])
		}
		i++
	}

	// Let the mutex go before merging.
	p.mtx.RUnlock()
	return Merge(ctx, its...)
}

// ExpandPostings returns the postings expanded as a slice.
func ExpandPostings(p Postings) (res []storage.SeriesRef, err error) {
	for p.Next() {
		res = append(res, p.At())
	}
	return res, p.Err()
}

// Postings provides iterative access over an ordered list of SeriesRef.
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

func (errPostings) Next() bool                  { return false }
func (errPostings) Seek(storage.SeriesRef) bool { return false }
func (errPostings) At() storage.SeriesRef       { return 0 }
func (e errPostings) Err() error                { return e.err }

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
	if slices.Contains(its, EmptyPostings()) {
		return EmptyPostings()
	}

	return newIntersectPostings(its...)
}

type intersectPostings struct {
	postings []Postings        // These are the postings we will be intersecting.
	current  storage.SeriesRef // The current intersection, if Seek() or Next() has returned true.
}

func newIntersectPostings(its ...Postings) *intersectPostings {
	return &intersectPostings{postings: its}
}

func (it *intersectPostings) At() storage.SeriesRef {
	return it.current
}

func (it *intersectPostings) Seek(target storage.SeriesRef) bool {
	for {
		allEqual := true
		for _, p := range it.postings {
			if !p.Seek(target) {
				return false
			}
			if p.At() > target {
				target = p.At()
				allEqual = false
			}
		}

		// if all p.At() are all equal, we found an intersection.
		if allEqual {
			it.current = target
			return true
		}
	}
}

func (it *intersectPostings) Next() bool {
	// Move forward the first Postings and take its value as the target to match.
	if !it.postings[0].Next() {
		return false
	}
	target := it.postings[0].At()
	allEqual := true
	for _, p := range it.postings[1:] { // Now move forward all the other ones and check if they match.
		if !p.Next() {
			return false
		}
		at := p.At()
		if at > target { // This one is past the target, so pick up a new target to Seek at the end.
			target = at
			allEqual = false
		} else if at < target { // This one needs to Seek to the target, but carry on with other postings in case they have an even higher target.
			allEqual = false
		}
	}
	if allEqual {
		it.current = target
		return true
	}
	return it.Seek(target)
}

func (it *intersectPostings) Err() error {
	for _, p := range it.postings {
		if p.Err() != nil {
			return p.Err()
		}
	}
	return nil
}

// Merge returns a new iterator over the union of the input iterators.
func Merge[T Postings](_ context.Context, its ...T) Postings {
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

type mergedPostings[T Postings] struct {
	p   []T
	h   *loser.Tree[storage.SeriesRef, T]
	cur storage.SeriesRef
}

func newMergedPostings[T Postings](p []T) (m *mergedPostings[T], nonEmpty bool) {
	const maxVal = storage.SeriesRef(math.MaxUint64) // This value must be higher than all real values used in the tree.
	lt := loser.New(p, maxVal)
	return &mergedPostings[T]{p: p, h: lt}, true
}

func (it *mergedPostings[T]) Next() bool {
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

func (it *mergedPostings[T]) Seek(id storage.SeriesRef) bool {
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

func (it mergedPostings[T]) At() storage.SeriesRef {
	return it.cur
}

func (it mergedPostings[T]) Err() error {
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

// listPostings implements the Postings interface over a plain list.
type listPostings struct {
	list []storage.SeriesRef
	cur  storage.SeriesRef
}

// NewListPostings creates a Postings from the supplied SeriesRefs, which must be in order.
// The list slice passed in is retained.
func NewListPostings(list []storage.SeriesRef) Postings {
	return &listPostings{list: list}
}

func (it *listPostings) At() storage.SeriesRef {
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

func (it *listPostings) Seek(x storage.SeriesRef) bool {
	// If the current value satisfies, then return.
	if it.cur >= x {
		return true
	}
	if len(it.list) == 0 {
		return false
	}

	i := 0 // Check the next item in the list, otherwise binary search between current position and end.
	if it.list[0] < x {
		i, _ = slices.BinarySearch(it.list, x)
		if i >= len(it.list) { // Off the end - terminate the iterator.
			it.list = nil
			return false
		}
	}
	it.cur = it.list[i]
	it.list = it.list[i+1:]
	return true
}

func (*listPostings) Err() error {
	return nil
}

// Len returns the remaining number of postings in the list.
func (it *listPostings) Len() int {
	return len(it.list)
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

func (*bigEndianPostings) Err() error {
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
func (h *postingsWithIndexHeap) Push(x any) {
	*h = append(*h, x.(postingsWithIndex))
}

// Pop implements heap.Interface and pops the last element, which is NOT the min element,
// so this doesn't return the same heap.Pop()
// Although this method is implemented for correctness, we don't expect it to be used, see popIndex() method for details.
func (h *postingsWithIndexHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
