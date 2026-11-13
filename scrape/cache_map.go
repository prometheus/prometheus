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

package scrape

import "slices"

// scrapeCacheKey identifies the original exposed metric, before relabeling.
// Keys passed to set must own their metric string; lookup keys may borrow it.
type scrapeCacheKey struct {
	hash   uint64
	metric string
}

type scrapeCacheItem[T any] struct {
	metric string
	value  T
}

// scrapeCacheMap verifies metric equality after hashing. The first entry for a
// hash stays in unique; additional entries use lazily allocated collision lists.
// The zero value is ready for use. Like scrapeCache, it is not concurrency-safe.
type scrapeCacheMap[T any] struct {
	unique    map[uint64]scrapeCacheItem[T]
	conflicts map[uint64][]scrapeCacheItem[T]
	count     int
}

func (m *scrapeCacheMap[T]) getEntry(key scrapeCacheKey) (scrapeCacheItem[T], bool) {
	if e, ok := m.unique[key.hash]; ok && e.metric == key.metric {
		return e, true
	}
	for _, e := range m.conflicts[key.hash] {
		if e.metric == key.metric {
			return e, true
		}
	}
	return scrapeCacheItem[T]{}, false
}

func (m *scrapeCacheMap[T]) get(key scrapeCacheKey) (T, bool) {
	e, ok := m.getEntry(key)
	return e.value, ok
}

func (m *scrapeCacheMap[T]) set(key scrapeCacheKey, value T) {
	if m.unique == nil {
		m.unique = make(map[uint64]scrapeCacheItem[T])
	}
	old, ok := m.unique[key.hash]
	if !ok || old.metric == key.metric {
		m.unique[key.hash] = scrapeCacheItem[T]{metric: key.metric, value: value}
		if !ok {
			m.count++
		}
		return
	}
	entries := m.conflicts[key.hash]
	for i := range entries {
		if entries[i].metric == key.metric {
			entries[i].value = value
			return
		}
	}
	if m.conflicts == nil {
		m.conflicts = make(map[uint64][]scrapeCacheItem[T])
	}
	m.conflicts[key.hash] = append(entries, scrapeCacheItem[T]{metric: key.metric, value: value})
	m.count++
}

func (m *scrapeCacheMap[T]) del(key scrapeCacheKey) {
	if e, ok := m.unique[key.hash]; ok && e.metric == key.metric {
		delete(m.unique, key.hash)
		m.count--
		m.promote(key.hash)
		return
	}
	entries := m.conflicts[key.hash]
	for i, e := range entries {
		if e.metric == key.metric {
			entries = slices.Delete(entries, i, i+1)
			if len(entries) == 0 {
				delete(m.conflicts, key.hash)
			} else {
				m.conflicts[key.hash] = entries
			}
			m.count--
			return
		}
	}
}

// promote moves the first remaining collision into an empty unique slot.
func (m *scrapeCacheMap[T]) promote(hash uint64) {
	entries := m.conflicts[hash]
	if len(entries) == 0 {
		return
	}
	m.unique[hash] = entries[0]
	entries = slices.Delete(entries, 0, 1)
	if len(entries) == 0 {
		delete(m.conflicts, hash)
	} else {
		m.conflicts[hash] = entries
	}
}

// retain removes entries rejected by keep. The callback must not mutate m.
func (m *scrapeCacheMap[T]) retain(keep func(scrapeCacheKey, T) bool) {
	// Filter collisions first, so promotion cannot visit an entry twice.
	for hash, entries := range m.conflicts {
		remaining := entries[:0]
		for _, e := range entries {
			if keep(scrapeCacheKey{hash: hash, metric: e.metric}, e.value) {
				remaining = append(remaining, e)
			} else {
				m.count--
			}
		}
		clear(entries[len(remaining):])
		if len(remaining) == 0 {
			delete(m.conflicts, hash)
		} else {
			m.conflicts[hash] = remaining
		}
	}
	for hash, e := range m.unique {
		if !keep(scrapeCacheKey{hash: hash, metric: e.metric}, e.value) {
			delete(m.unique, hash)
			m.count--
			m.promote(hash)
		}
	}
}
