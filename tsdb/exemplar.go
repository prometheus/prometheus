// Copyright The Prometheus Authors
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
	"context"
	"errors"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

const (
	// Indicates that there is no index entry for an exemplar.
	noExemplar = -1
	// Estimated number of exemplars per series, for sizing the index.
	estimatedExemplarsPerSeries = 16
)

type CircularExemplarStorage struct {
	lock                sync.RWMutex
	exemplars           []circularBufferEntry
	nextIndex           int
	metrics             *ExemplarMetrics
	oooTimeWindowMillis int64

	// Map of series labels as a string to index entry, which points to the first
	// and last exemplar for the series in the exemplars circular buffer.
	index map[string]*indexEntry
}

type indexEntry struct {
	oldest       int
	newest       int
	seriesLabels labels.Labels
}

type circularBufferEntry struct {
	exemplar exemplar.Exemplar
	next     int
	prev     int
	ref      *indexEntry
}

type ExemplarMetrics struct {
	exemplarsAppended            prometheus.Counter
	exemplarsInStorage           prometheus.Gauge
	seriesWithExemplarsInStorage prometheus.Gauge
	lastExemplarsTs              prometheus.Gauge
	maxExemplars                 prometheus.Gauge
	outOfOrderExemplars          prometheus.Counter
}

func NewExemplarMetrics(reg prometheus.Registerer) *ExemplarMetrics {
	m := ExemplarMetrics{
		exemplarsAppended: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_appended_total",
			Help: "Total number of appended exemplars.",
		}),
		exemplarsInStorage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_in_storage",
			Help: "Number of exemplars currently in circular storage.",
		}),
		seriesWithExemplarsInStorage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_series_with_exemplars_in_storage",
			Help: "Number of series with exemplars currently in circular storage.",
		}),
		lastExemplarsTs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds",
			Help: "The timestamp of the oldest exemplar stored in circular storage. Useful to check for what time" +
				"range the current exemplar buffer limit allows. This usually means the last timestamp" +
				"for all exemplars for a typical setup. This is not true though if one of the series timestamp is in future compared to rest series.",
		}),
		outOfOrderExemplars: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_out_of_order_exemplars_total",
			Help: "Total number of out of order exemplar ingestion failed attempts.",
		}),
		maxExemplars: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_max_exemplars",
			Help: "Total number of exemplars the exemplar storage can store, resizeable.",
		}),
	}

	if reg != nil {
		reg.MustRegister(
			m.exemplarsAppended,
			m.exemplarsInStorage,
			m.seriesWithExemplarsInStorage,
			m.lastExemplarsTs,
			m.outOfOrderExemplars,
			m.maxExemplars,
		)
	}

	return &m
}

// NewCircularExemplarStorage creates a circular in memory exemplar storage.
// If we assume the average case 95 bytes per exemplar we can fit 5651272 exemplars in
// 1GB of extra memory, accounting for the fact that this is heap allocated space.
// If len <= 0, then the exemplar storage is essentially a noop storage but can later be
// resized to store exemplars. If oooTimeWindowMillis <= 0, out-of-order exemplars are disabled.
func NewCircularExemplarStorage(length int64, m *ExemplarMetrics, oooTimeWindowMillis int64) (ExemplarStorage, error) {
	if length < 0 {
		length = 0
	}
	if oooTimeWindowMillis < 0 {
		oooTimeWindowMillis = 0
	}
	c := &CircularExemplarStorage{
		exemplars:           make([]circularBufferEntry, length),
		index:               make(map[string]*indexEntry, length/estimatedExemplarsPerSeries),
		metrics:             m,
		oooTimeWindowMillis: oooTimeWindowMillis,
	}

	c.metrics.maxExemplars.Set(float64(length))

	return c, nil
}

func (ce *CircularExemplarStorage) ApplyConfig(cfg *config.Config) error {
	ce.Resize(cfg.StorageConfig.ExemplarsConfig.MaxExemplars)
	return nil
}

func (ce *CircularExemplarStorage) Appender() *CircularExemplarStorage {
	return ce
}

func (ce *CircularExemplarStorage) ExemplarQuerier(context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

func (ce *CircularExemplarStorage) Querier(context.Context) (storage.ExemplarQuerier, error) {
	return ce, nil
}

// Select returns exemplars for a given set of label matchers.
func (ce *CircularExemplarStorage) Select(start, end int64, matchers ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	ret := make([]exemplar.QueryResult, 0)

	ce.lock.RLock()
	defer ce.lock.RUnlock()

	if len(ce.exemplars) == 0 {
		return ret, nil
	}

	// Loop through each index entry, which will point us to first/last exemplar for each series.
	for _, idx := range ce.index {
		var se exemplar.QueryResult
		e := ce.exemplars[idx.oldest]
		if e.exemplar.Ts > end || ce.exemplars[idx.newest].exemplar.Ts < start {
			continue
		}
		if !matchesSomeMatcherSet(idx.seriesLabels, matchers) {
			continue
		}
		se.SeriesLabels = idx.seriesLabels

		// TODO: Since we maintain a doubly-linked-list, we can also iterate from head to tail
		//  which might be more performant if the selected interval is skewed to the head.

		// Loop through all exemplars in the circular buffer for the current series.
		for e.exemplar.Ts <= end {
			if e.exemplar.Ts >= start {
				se.Exemplars = append(se.Exemplars, e.exemplar)
			}
			if e.next == noExemplar {
				break
			}
			e = ce.exemplars[e.next]
		}
		if len(se.Exemplars) > 0 {
			ret = append(ret, se)
		}
	}

	slices.SortFunc(ret, func(a, b exemplar.QueryResult) int {
		return labels.Compare(a.SeriesLabels, b.SeriesLabels)
	})

	return ret, nil
}

func matchesSomeMatcherSet(lbls labels.Labels, matchers [][]*labels.Matcher) bool {
Outer:
	for _, ms := range matchers {
		for _, m := range ms {
			if !m.Matches(lbls.Get(m.Name)) {
				continue Outer
			}
		}
		return true
	}
	return false
}

func (ce *CircularExemplarStorage) ValidateExemplar(l labels.Labels, e exemplar.Exemplar) error {
	var buf [1024]byte
	seriesLabels := l.Bytes(buf[:])

	// TODO(bwplotka): This lock can lock all scrapers, there might high contention on this on scale.
	// Optimize by moving the lock to be per series (& benchmark it).
	ce.lock.RLock()
	defer ce.lock.RUnlock()
	return ce.validateExemplar(ce.index[string(seriesLabels)], e, false)
}

// Not thread safe. The appended parameters tells us whether this is an external validation, or internal
// as a result of an AddExemplar call, in which case we should update any relevant metrics.
func (ce *CircularExemplarStorage) validateExemplar(idx *indexEntry, e exemplar.Exemplar, appended bool) error {
	if len(ce.exemplars) == 0 {
		return storage.ErrExemplarsDisabled
	}

	// Exemplar label length does not include chars involved in text rendering such as quotes
	// equals sign, or commas. See definition of const ExemplarMaxLabelLength.
	labelSetLen := 0
	if err := e.Labels.Validate(func(l labels.Label) error {
		labelSetLen += utf8.RuneCountInString(l.Name)
		labelSetLen += utf8.RuneCountInString(l.Value)

		if labelSetLen > exemplar.ExemplarMaxLabelSetLength {
			return storage.ErrExemplarLabelLength
		}
		return nil
	}); err != nil {
		return err
	}

	if idx == nil {
		return nil
	}

	// Check for duplicate vs last stored exemplar for this series.
	// NB these are expected, and appending them is a no-op.
	// For floats and classic histograms, there is only 1 exemplar per series,
	// so this is sufficient. For native histograms with multiple exemplars per series,
	// we have another check below.
	newestExemplar := ce.exemplars[idx.newest].exemplar
	if newestExemplar.Equals(e) {
		return storage.ErrDuplicateExemplar
	}

	// Reject exemplars older than the OOO time window relative to the newest exemplar.
	// Exemplars with the same timestamp are ordered by value then label hash to detect
	// duplicates without iterating through all stored exemplars, which would be too
	// expensive under lock. Exemplars with equal timestamps but different values or
	// labels are allowed to support multiple buckets of native histograms.
	if (e.Ts < newestExemplar.Ts && e.Ts <= newestExemplar.Ts-ce.oooTimeWindowMillis) ||
		(e.Ts == newestExemplar.Ts && e.Value < newestExemplar.Value) ||
		(e.Ts == newestExemplar.Ts && e.Value == newestExemplar.Value && e.Labels.Hash() < newestExemplar.Labels.Hash()) {
		if appended {
			ce.metrics.outOfOrderExemplars.Inc()
		}
		return storage.ErrOutOfOrderExemplar
	}
	return nil
}

// SetOutOfOrderTimeWindow sets the out-of-order time window for exemplars in
// milliseconds. Exemplars older than it are not added to the circular exemplar
// buffer.
func (ce *CircularExemplarStorage) SetOutOfOrderTimeWindow(d int64) {
	ce.lock.Lock()
	defer ce.lock.Unlock()
	ce.oooTimeWindowMillis = d
}

// Resize changes the size of exemplar buffer by allocating a new buffer and
// migrating data to it. Exemplars are kept when possible. Shrinking will discard
// old data (in order of ingestion) as needed. Returns the number of migrated
// exemplars.
func (ce *CircularExemplarStorage) Resize(l int64) int {
	// Accept negative values as just 0 size.
	if l <= 0 {
		l = 0
	}

	ce.lock.Lock()
	defer ce.lock.Unlock()

	oldSize := int64(len(ce.exemplars))
	migrated := 0
	switch {
	case l == oldSize:
		// NOOP.
		return migrated
	case l > oldSize:
		migrated = ce.grow(l)
	case l < oldSize:
		migrated = ce.shrink(l)
	}

	ce.computeMetrics()
	ce.metrics.maxExemplars.Set(float64(l))
	return migrated
}

// grow the circular buffer to have size l by allocating a new slice and copying
// the old data to it. After growing, ce.nextIndex points to the next free entry
// in the buffer. This function must be called with the lock acquired.
func (ce *CircularExemplarStorage) grow(l int64) int {
	oldSize := len(ce.exemplars)
	newSlice := make([]circularBufferEntry, l)
	ranges := []intRange{
		{from: ce.nextIndex, to: oldSize},
		{from: 0, to: ce.nextIndex},
	}
	totalCopied, migrated := copyExemplarRanges(ce.index, newSlice, ce.exemplars, ranges)
	ce.nextIndex = totalCopied
	ce.exemplars = newSlice
	return migrated
}

// shrink the circular buffer by either trimming from the right or deleting the
// oldest samples to accommodate the new size l. This function must be called
// with the lock acquired.
func (ce *CircularExemplarStorage) shrink(l int64) (migrated int) {
	oldSize := len(ce.exemplars)
	diff := int(int64(oldSize) - l)
	deleteStart := ce.nextIndex
	deleteEnd := (deleteStart + diff) % oldSize

	// Remove items from the buffer starting from c.nextIndex. This drops older
	// entries first in the order of ingestion.
	for i := range diff {
		idx := (deleteStart + i) % oldSize
		ref := ce.exemplars[idx].ref
		if ce.removeExemplar(&ce.exemplars[idx]) {
			ce.removeIndex(ref)
		}
	}

	newSlice := make([]circularBufferEntry, int(l))

	var totalCopied int
	switch {
	case deleteStart == deleteEnd:
		// The entire buffer was cleared (shrink to zero). Note that we don't have to
		// delete the index since removeExemplar already did. Simply remove all elements
		// and reset tracking pointers.
		ce.exemplars = newSlice
		ce.nextIndex = 0
		return 0
	case deleteStart < deleteEnd:
		// We delete an "inner" section of the circular buffer.
		totalCopied, migrated = copyExemplarRanges(ce.index, newSlice, ce.exemplars, []intRange{
			{from: deleteEnd, to: oldSize},
			{from: 0, to: deleteStart},
		})
	case deleteStart > deleteEnd:
		// We keep an "inner" section of the circular buffer.
		totalCopied, migrated = copyExemplarRanges(ce.index, newSlice, ce.exemplars, []intRange{
			{from: deleteEnd, to: deleteStart},
		})
	}

	ce.nextIndex = totalCopied % int(l)
	ce.exemplars = newSlice
	return migrated
}

func (ce *CircularExemplarStorage) AddExemplar(l labels.Labels, e exemplar.Exemplar) error {
	// TODO(bwplotka): This lock can lock all scrapers, there might high contention on this on scale.
	// Optimize by moving the lock to be per series (& benchmark it).
	ce.lock.Lock()
	defer ce.lock.Unlock()

	if len(ce.exemplars) == 0 {
		return storage.ErrExemplarsDisabled
	}

	var buf [1024]byte
	seriesLabels := l.Bytes(buf[:])

	idx, indexExists := ce.index[string(seriesLabels)]
	err := ce.validateExemplar(idx, e, true)
	if err != nil {
		if errors.Is(err, storage.ErrDuplicateExemplar) {
			// Duplicate exemplar, noop.
			return nil
		}
		return err
	}

	// If we insert an out-of-order exemplar, we preemptively find the insertion
	// index to check for duplicates.
	var insertionIndex int
	var outOfOrder bool
	if indexExists {
		outOfOrder = e.Ts >= ce.exemplars[idx.oldest].exemplar.Ts && e.Ts < ce.exemplars[idx.newest].exemplar.Ts
		if outOfOrder {
			insertionIndex = ce.findInsertionIndex(e, idx)
			if ce.exemplars[insertionIndex].exemplar.Ts == e.Ts {
				// Assume duplicate exemplar, noop.
				// Native histograms will exercise this code path a lot due to
				// having multiple exemplars per series so checking the
				// value and labels would be too expensive.
				return nil
			}
		}
	}

	// If the index didn't exist (new series), create one.
	if !indexExists {
		idx = &indexEntry{seriesLabels: l}
		ce.index[string(seriesLabels)] = idx
	}

	// Remove entries if the buffer is full.
	if prev := &ce.exemplars[ce.nextIndex]; prev.ref != nil {
		prevRef := prev.ref
		if ce.removeExemplar(prev) {
			if prevRef == idx {
				// Do not delete the indexEntry we're inserting to.
				indexExists = false
			} else {
				ce.removeIndex(prevRef)
			}
		} else if outOfOrder && insertionIndex == ce.nextIndex && prevRef == idx {
			// The entry we were going to insert after was removed from the same series.
			// Recalculate the insertion point in the updated linked list to avoid
			// creating a self-referencing loop.
			insertionIndex = ce.findInsertionIndex(e, idx)
		}
	}

	// We create a new entry in the linked list.
	ce.exemplars[ce.nextIndex].exemplar = e
	ce.exemplars[ce.nextIndex].ref = idx

	switch {
	case !indexExists:
		// Add the first and only exemplar to the list.
		idx.oldest = ce.nextIndex
		idx.newest = ce.nextIndex
		ce.exemplars[ce.nextIndex].prev = noExemplar
		ce.exemplars[ce.nextIndex].next = noExemplar
	case e.Ts >= ce.exemplars[idx.newest].exemplar.Ts:
		// Add the exemplar at the tip (after newest).
		ce.exemplars[idx.newest].next = ce.nextIndex
		ce.exemplars[ce.nextIndex].prev = idx.newest
		ce.exemplars[ce.nextIndex].next = noExemplar
		idx.newest = ce.nextIndex
	case e.Ts < ce.exemplars[idx.oldest].exemplar.Ts:
		// Add the exemplar at the tail (before oldest).
		ce.exemplars[idx.oldest].prev = ce.nextIndex
		ce.exemplars[ce.nextIndex].prev = noExemplar
		ce.exemplars[ce.nextIndex].next = idx.oldest
		idx.oldest = ce.nextIndex
	default:
		// Insert the exemplar into the list by finding the most recent
		// in-order exemplar that precedes it, and placing it after.
		nextExemplar := ce.exemplars[insertionIndex].next
		ce.exemplars[ce.nextIndex].prev = insertionIndex
		ce.exemplars[ce.nextIndex].next = nextExemplar
		ce.exemplars[insertionIndex].next = ce.nextIndex
		if nextExemplar != noExemplar {
			ce.exemplars[nextExemplar].prev = ce.nextIndex
		}
	}

	ce.nextIndex = (ce.nextIndex + 1) % len(ce.exemplars)

	ce.metrics.exemplarsAppended.Inc()
	ce.computeMetrics()
	return nil
}

// removeExemplar removes the given entry from the circular buffer. Returns true
// iff the deleted entry was the last entry (and the index is now empty).
// This function must be called with the lock acquired.
func (ce *CircularExemplarStorage) removeExemplar(entry *circularBufferEntry) bool {
	ref := entry.ref
	if ref == nil {
		return false
	}

	if entry.prev != noExemplar {
		ce.exemplars[entry.prev].next = entry.next
	} else {
		ref.oldest = entry.next
	}

	if entry.next != noExemplar {
		ce.exemplars[entry.next].prev = entry.prev
	} else {
		ref.newest = entry.prev
	}

	// Mark this item as deleted.
	entry.ref = nil

	return ref.oldest == noExemplar && ref.newest == noExemplar
}

// removeIndex removes an indexEntry from the circular exemplar storage.
// This function must be called with the lock acquired.
func (ce *CircularExemplarStorage) removeIndex(ref *indexEntry) {
	var buf [1024]byte
	entryLabels := ref.seriesLabels.Bytes(buf[:])
	delete(ce.index, string(entryLabels))
}

// findInsertionIndex finds the position at which e should be placed in the
// doubly-linked list by traversing the linked list from idx.newest to idx.oldest
// and following back links. Since out-of-order exemplars commonly lie close to
// the newest entry, traversing from newest to oldest is usually faster.
func (ce *CircularExemplarStorage) findInsertionIndex(e exemplar.Exemplar, idx *indexEntry) int {
	for i := idx.newest; i != noExemplar; {
		current := ce.exemplars[i]
		if current.exemplar.Ts <= e.Ts {
			return i
		}
		i = current.prev
	}
	return idx.oldest
}

func (ce *CircularExemplarStorage) computeMetrics() {
	ce.metrics.seriesWithExemplarsInStorage.Set(float64(len(ce.index)))

	if len(ce.exemplars) == 0 {
		ce.metrics.exemplarsInStorage.Set(float64(0))
		ce.metrics.lastExemplarsTs.Set(float64(0))
		return
	}

	if ce.exemplars[ce.nextIndex].ref != nil {
		ce.metrics.exemplarsInStorage.Set(float64(len(ce.exemplars)))
		ce.metrics.lastExemplarsTs.Set(float64(ce.exemplars[ce.nextIndex].exemplar.Ts) / 1000)
		return
	}

	// We did not yet fill the buffer.
	ce.metrics.exemplarsInStorage.Set(float64(ce.nextIndex))
	if ce.exemplars[0].ref != nil {
		ce.metrics.lastExemplarsTs.Set(float64(ce.exemplars[0].exemplar.Ts) / 1000)
	}
}

// IterateExemplars iterates through all the exemplars from oldest to newest appended and calls
// the given function on all of them till the end (or) till the first function call that returns an error.
func (ce *CircularExemplarStorage) IterateExemplars(f func(seriesLabels labels.Labels, e exemplar.Exemplar) error) error {
	ce.lock.RLock()
	defer ce.lock.RUnlock()

	idx := ce.nextIndex
	l := len(ce.exemplars)
	for i := 0; i < l; i, idx = i+1, (idx+1)%l {
		if ce.exemplars[idx].ref == nil {
			continue
		}
		err := f(ce.exemplars[idx].ref.seriesLabels, ce.exemplars[idx].exemplar)
		if err != nil {
			return err
		}
	}
	return nil
}

type intRange struct {
	from, to int
}

func (e intRange) contains(i int) bool {
	return i >= e.from && i < e.to
}

// copyExemplarRanges copies non-overlapping ranges from src into dest and
// adjusts list pointers in dest and index accordingly. Returns the total
// number of slots copied (for nextIndex) and the number of non-empty entries
// migrated.
func copyExemplarRanges(
	index map[string]*indexEntry,
	dest, src []circularBufferEntry,
	ranges []intRange,
) (totalCopied, migratedEntries int) {
	offsets := make([]int, len(ranges))
	n := 0
	for i, rng := range ranges {
		offsets[i] = n - rng.from
		n += copy(dest[n:], src[rng.from:rng.to])
	}
	migratedEntries = n
	for di := range n {
		e := &dest[di]
		if e.ref == nil {
			// We potentially copied empty entries. Subtract them now to correctly show the
			// number of "migrated" items.
			migratedEntries--
			continue
		}
		for i, rng := range ranges {
			if rng.contains(e.prev) {
				e.prev += offsets[i]
				break
			}
		}
		for i, rng := range ranges {
			if rng.contains(e.next) {
				e.next += offsets[i]
				break
			}
		}
	}
	for _, idx := range index {
		for i, rng := range ranges {
			if rng.contains(idx.oldest) {
				idx.oldest += offsets[i]
				break
			}
		}
		for i, rng := range ranges {
			if rng.contains(idx.newest) {
				idx.newest += offsets[i]
				break
			}
		}
	}
	return n, migratedEntries
}
