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

package agent

import (
	"iter"
	"sync"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// memSeries is a chunkless version of tsdb.memSeries.
type memSeries struct {
	sync.Mutex

	ref  chunks.HeadSeriesRef
	lset labels.Labels

	// Last recorded timestamp. Used by Storage.gc to determine if a series is
	// stale.
	lastTs int64
}

// updateTimestamp obtains the lock on s and will attempt to update lastTs.
// fails if newTs < lastTs.
func (m *memSeries) updateTimestamp(newTs int64) bool {
	m.Lock()
	defer m.Unlock()
	if newTs >= m.lastTs {
		m.lastTs = newTs
		return true
	}
	return false
}

func (m *memSeries) Ref() chunks.HeadSeriesRef {
	return m.ref
}

func (m *memSeries) Labels() labels.Labels {
	return m.lset
}

func (m *memSeries) LastSampleTimestamp() int64 {
	return m.lastTs
}

// seriesHashmap lets agent find a memSeries by its label set, via a 64-bit hash.
// There is one map for the common case where the hash value is unique, and a
// second map for the case that two series have the same hash value.
// Each series is in only one of the maps. Its methods require the hash to be submitted
// with the label set to avoid re-computing hash throughout the code.
type seriesHashmap struct {
	unique    map[uint64]*memSeries
	conflicts map[uint64][]*memSeries
}

func (m *seriesHashmap) Get(hash uint64, lset labels.Labels) *memSeries {
	if s, found := m.unique[hash]; found {
		if labels.Equal(s.lset, lset) {
			return s
		}
	}
	for _, s := range m.conflicts[hash] {
		if labels.Equal(s.lset, lset) {
			return s
		}
	}
	return nil
}

func (m *seriesHashmap) Set(hash uint64, s *memSeries) {
	if existing, found := m.unique[hash]; !found || labels.Equal(existing.lset, s.lset) {
		m.unique[hash] = s
		return
	}
	if m.conflicts == nil {
		m.conflicts = make(map[uint64][]*memSeries)
	}
	seriesSet := m.conflicts[hash]
	for i, prev := range seriesSet {
		if labels.Equal(prev.lset, s.lset) {
			seriesSet[i] = s
			return
		}
	}
	m.conflicts[hash] = append(seriesSet, s)
}

func (m *seriesHashmap) Delete(hash uint64, ref chunks.HeadSeriesRef) {
	var rem []*memSeries
	unique, found := m.unique[hash]
	switch {
	case !found: // Supplied hash is not stored.
		return
	case unique.ref == ref:
		conflicts := m.conflicts[hash]
		if len(conflicts) == 0 { // Exactly one series with this hash was stored
			delete(m.unique, hash)
			return
		}
		m.unique[hash] = conflicts[0] // First remaining series goes in 'unique'.
		rem = conflicts[1:]           // Keep the rest.
	default: // The series to delete is somewhere in 'conflicts'. Keep all the ones that don't match.
		for _, s := range m.conflicts[hash] {
			if s.ref != ref {
				rem = append(rem, s)
			}
		}
	}
	if len(rem) == 0 {
		delete(m.conflicts, hash)
	} else {
		m.conflicts[hash] = rem
	}
}

// stripeSeries locks modulo ranges of IDs and hashes to reduce lock
// contention. The locks are padded to not be on the same cache line.
// Filling the padded space with the maps was profiled to be slower -
// likely due to the additional pointer dereferences.
type stripeSeries struct {
	size      int
	series    []map[chunks.HeadSeriesRef]*memSeries
	hashes    []seriesHashmap
	exemplars []map[chunks.HeadSeriesRef]*exemplar.Exemplar
	locks     []stripeLock

	gcMut sync.Mutex
}

type stripeLock struct {
	sync.RWMutex
	// Padding to avoid multiple locks being on the same cache line.
	_ [40]byte
}

func newStripeSeries(stripeSize int) *stripeSeries {
	s := &stripeSeries{
		size:      stripeSize,
		series:    make([]map[chunks.HeadSeriesRef]*memSeries, stripeSize),
		hashes:    make([]seriesHashmap, stripeSize),
		exemplars: make([]map[chunks.HeadSeriesRef]*exemplar.Exemplar, stripeSize),
		locks:     make([]stripeLock, stripeSize),
	}
	for i := range s.series {
		s.series[i] = map[chunks.HeadSeriesRef]*memSeries{}
	}
	for i := range s.hashes {
		s.hashes[i] = seriesHashmap{
			unique:    map[uint64]*memSeries{},
			conflicts: nil, // Initialized on demand in set().
		}
	}
	for i := range s.exemplars {
		s.exemplars[i] = map[chunks.HeadSeriesRef]*exemplar.Exemplar{}
	}
	return s
}

// GC garbage collects old series that have not received a sample after mint
// and will fully delete them.
func (s *stripeSeries) GC(mint int64, retainLabels bool) map[chunks.HeadSeriesRef]labels.Labels {
	// gcMut serializes GC calls. Within a single GC pass, the check function
	// holds hashLock and then acquires refLock — callers must never hold both
	// simultaneously, which SetUnlessAlreadySet satisfies.
	s.gcMut.Lock()
	defer s.gcMut.Unlock()

	// labels of deleted series are used by agent.Checkpoint
	deleted := map[chunks.HeadSeriesRef]labels.Labels{}

	// For one series, truncate old chunks and check if any chunks left. If not, mark as deleted and collect the ID.
	check := func(hashLock int, hash uint64, series *memSeries) {
		series.Lock()

		// Any series that has received a write since mint is still alive.
		if series.lastTs >= mint {
			series.Unlock()
			return
		}

		// The series is stale. We need to obtain a second lock for the
		// ref if it's different than the hash lock.
		refLock := int(s.refLock(series.ref))
		if hashLock != refLock {
			s.locks[refLock].Lock()
		}

		if retainLabels {
			deleted[series.ref] = series.lset
		} else {
			deleted[series.ref] = labels.EmptyLabels()
		}

		delete(s.series[refLock], series.ref)
		s.hashes[hashLock].Delete(hash, series.ref)

		// Since the series is gone, we'll also delete
		// the latest stored exemplar.
		delete(s.exemplars[refLock], series.ref)

		if hashLock != refLock {
			s.locks[refLock].Unlock()
		}
		series.Unlock()
	}

	for hashLock := 0; hashLock < s.size; hashLock++ {
		s.locks[hashLock].Lock()

		for hash, all := range s.hashes[hashLock].conflicts {
			for _, series := range all {
				check(hashLock, hash, series)
			}
		}
		for hash, series := range s.hashes[hashLock].unique {
			check(hashLock, hash, series)
		}

		s.locks[hashLock].Unlock()
	}

	return deleted
}

func (s *stripeSeries) GetByID(id chunks.HeadSeriesRef) *memSeries {
	refLock := s.refLock(id)
	s.locks[refLock].RLock()
	defer s.locks[refLock].RUnlock()
	return s.series[refLock][id]
}

func (s *stripeSeries) GetByHash(hash uint64, lset labels.Labels) *memSeries {
	hashLock := s.hashLock(hash)

	s.locks[hashLock].RLock()
	defer s.locks[hashLock].RUnlock()
	return s.hashes[hashLock].Get(hash, lset)
}

// SetUnlessAlreadySet inserts series for the given hash if no series with the
// same label set already exists. It returns the canonical series and whether
// it was newly inserted.
//
// Insertion order is refs-before-hashes. GC only discovers series via hashes,
// so anything it finds is guaranteed to already be present in refs. We never
// hold hashLock and refLock simultaneously, preserving the no-deadlock
// invariant that GC relies on (it holds hashLock while acquiring refLock).
func (s *stripeSeries) SetUnlessAlreadySet(hash uint64, series *memSeries) (*memSeries, bool) {
	hashLock := s.hashLock(hash)

	// Fast path: series already exists.
	s.locks[hashLock].Lock()
	if prev := s.hashes[hashLock].Get(hash, series.lset); prev != nil {
		s.locks[hashLock].Unlock()
		return prev, false
	}
	s.locks[hashLock].Unlock()

	// Insert into refs first. GC discovers series through hashes, so a series
	// that is only in refs is invisible to GC and will not be removed.
	refLock := s.refLock(series.ref)
	s.locks[refLock].Lock()
	s.series[refLock][series.ref] = series
	s.locks[refLock].Unlock()

	// Re-acquire hashLock to insert into hashes. A concurrent goroutine may
	// have inserted the same label set while we were inserting into refs, so
	// check again before committing.
	s.locks[hashLock].Lock()
	if prev := s.hashes[hashLock].Get(hash, series.lset); prev != nil {
		s.locks[hashLock].Unlock()
		// We lost the race: clean up the ref we pre-inserted.
		s.locks[refLock].Lock()
		delete(s.series[refLock], series.ref)
		s.locks[refLock].Unlock()
		return prev, false
	}
	s.hashes[hashLock].Set(hash, series)
	s.locks[hashLock].Unlock()

	return series, true
}

func (s *stripeSeries) GetLatestExemplar(ref chunks.HeadSeriesRef) *exemplar.Exemplar {
	i := s.refLock(ref)

	s.locks[i].RLock()
	exemplar := s.exemplars[i][ref]
	s.locks[i].RUnlock()

	return exemplar
}

func (s *stripeSeries) SetLatestExemplar(ref chunks.HeadSeriesRef, exemplar *exemplar.Exemplar) {
	i := s.refLock(ref)

	// Make sure that's a valid series id and record its latest exemplar
	s.locks[i].Lock()
	if s.series[i][ref] != nil {
		s.exemplars[i][ref] = exemplar
	}
	s.locks[i].Unlock()
}

func (s *stripeSeries) hashLock(hash uint64) uint64 {
	return hash & uint64(s.size-1)
}

func (s *stripeSeries) refLock(ref chunks.HeadSeriesRef) uint64 {
	return uint64(ref) & uint64(s.size-1)
}

var _ ActiveSeries = (*seriesSnapshot)(nil)

// seriesSnapshot is a point-in-time copy of a memSeries fields.
// It is used to avoid holding series locks during checkpoint I/O.
type seriesSnapshot struct {
	ref    chunks.HeadSeriesRef
	lset   labels.Labels
	lastTs int64
}

func (s *seriesSnapshot) Ref() chunks.HeadSeriesRef {
	return s.ref
}

func (s *seriesSnapshot) Labels() labels.Labels {
	return s.lset
}

func (s *seriesSnapshot) LastSampleTimestamp() int64 {
	return s.lastTs
}

func (s *stripeSeries) allSeries() iter.Seq[ActiveSeries] {
	return func(yield func(ActiveSeries) bool) {
		var buf []*memSeries
		for i := 0; i < s.size; i++ {
			// Collect pointers under RLock to avoid blocking appenders during I/O.
			s.locks[i].RLock()
			buf = buf[:0]
			for _, series := range s.series[i] {
				buf = append(buf, series)
			}
			s.locks[i].RUnlock()

			// Snapshot and yield outside the stripe lock so that
			// slow consumers (e.g. checkpoint disk I/O) do not
			// block appends that need a write lock on this stripe.
			for _, series := range buf {
				series.Lock()
				snapshot := seriesSnapshot{
					ref:    series.ref,
					lset:   series.lset,
					lastTs: series.lastTs,
				}
				series.Unlock()

				if !yield(&snapshot) {
					return
				}
			}
		}
	}
}

// deletedSeriesIter returns an iterator over deleted series from the given map.
// Only series whose lastSegment is greater than last are emitted, matching the
// filtering behaviour of [wlog.Checkpoint] keep function.
func deletedSeriesIter(m map[chunks.HeadSeriesRef]deletedRefMeta, last int) iter.Seq[DeletedSeries] {
	return func(yield func(DeletedSeries) bool) {
		for ref, meta := range m {
			if meta.lastSegment > last {
				if !yield(deletedSeries{ref: ref, labels: meta.labels}) {
					return
				}
			}
		}
	}
}

var _ DeletedSeries = deletedSeries{}

type deletedSeries struct {
	ref    chunks.HeadSeriesRef
	labels labels.Labels
}

func (series deletedSeries) Ref() chunks.HeadSeriesRef {
	return series.ref
}

func (series deletedSeries) Labels() labels.Labels {
	return series.labels
}
