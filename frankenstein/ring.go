// Copyright 2016 The Prometheus Authors
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

package frankenstein

// Based on https://raw.githubusercontent.com/stathat/consistent/master/consistent.go

import (
	"errors"
	"sort"
	"sync"
)

type uint64s []uint64

func (x uint64s) Len() int           { return len(x) }
func (x uint64s) Less(i, j int) bool { return x[i] < x[j] }
func (x uint64s) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// ErrEmptyRing is the error returned when trying to get an element when nothing has been added to hash.
var ErrEmptyRing = errors.New("empty circle")

// Ring holds the information about the members of the consistent hash circle.
type Ring struct {
	mtx          sync.RWMutex
	circle       map[uint64]Collector
	sortedHashes uint64s
}

// NewRing creates a new Ring object.
func NewRing() *Ring {
	return &Ring{
		circle: map[uint64]Collector{},
	}
}

// Update inserts a collector in the consistent hash.
func (r *Ring) Update(col Collector) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, token := range col.Tokens {
		r.circle[token] = col
	}
	r.updateSortedHashes()
}

// Get returns a collector close to the hash in the circle.
func (r *Ring) Get(key uint64) (Collector, error) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	if len(r.circle) == 0 {
		return Collector{}, ErrEmptyRing
	}
	i := r.search(key)
	return r.circle[r.sortedHashes[i]], nil
}

func (r *Ring) search(key uint64) (i int) {
	f := func(x int) bool {
		return r.sortedHashes[x] > key
	}
	i = sort.Search(len(r.sortedHashes), f)
	if i >= len(r.sortedHashes) {
		i = 0
	}
	return
}

func (r *Ring) updateSortedHashes() {
	hashes := uint64s{}
	for k := range r.circle {
		hashes = append(hashes, k)
	}
	sort.Sort(hashes)
	r.sortedHashes = hashes
}
