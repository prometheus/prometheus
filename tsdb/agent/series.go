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

package agent

import (
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
func (s *stripeSeries) GC(mint int64) map[chunks.HeadSeriesRef]struct{} {
	// NOTE(rfratto): GC will grab two locks, one for the hash and the other for
	// series. It's not valid for any other function to grab both locks,
	// otherwise a deadlock might occur when running GC in parallel with
	// appending.
	s.gcMut.Lock()
	defer s.gcMut.Unlock()

	deleted := map[chunks.HeadSeriesRef]struct{}{}

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
		refLock := int(series.ref) & (s.size - 1)
		if hashLock != refLock {
			s.locks[refLock].Lock()
		}

		deleted[series.ref] = struct{}{}
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
	refLock := uint64(id) & uint64(s.size-1)
	s.locks[refLock].RLock()
	defer s.locks[refLock].RUnlock()
	return s.series[refLock][id]
}

func (s *stripeSeries) GetByHash(hash uint64, lset labels.Labels) *memSeries {
	hashLock := hash & uint64(s.size-1)

	s.locks[hashLock].RLock()
	defer s.locks[hashLock].RUnlock()
	return s.hashes[hashLock].Get(hash, lset)
}

func (s *stripeSeries) Set(hash uint64, series *memSeries) {
	var (
		hashLock = hash & uint64(s.size-1)
		refLock  = uint64(series.ref) & uint64(s.size-1)
	)

	// We can't hold both locks at once otherwise we might deadlock with a
	// simultaneous call to GC.
	//
	// We update s.series first because GC expects anything in s.hashes to
	// already exist in s.series.
	s.locks[refLock].Lock()
	s.series[refLock][series.ref] = series
	s.locks[refLock].Unlock()

	s.locks[hashLock].Lock()
	s.hashes[hashLock].Set(hash, series)
	s.locks[hashLock].Unlock()
}

func (s *stripeSeries) GetLatestExemplar(ref chunks.HeadSeriesRef) *exemplar.Exemplar {
	i := uint64(ref) & uint64(s.size-1)

	s.locks[i].RLock()
	exemplar := s.exemplars[i][ref]
	s.locks[i].RUnlock()

	return exemplar
}

func (s *stripeSeries) SetLatestExemplar(ref chunks.HeadSeriesRef, exemplar *exemplar.Exemplar) {
	i := uint64(ref) & uint64(s.size-1)

	// Make sure that's a valid series id and record its latest exemplar
	s.locks[i].Lock()
	if s.series[i][ref] != nil {
		s.exemplars[i][ref] = exemplar
	}
	s.locks[i].Unlock()
}
