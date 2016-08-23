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
	"math"
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type uint32s []uint32

func (x uint32s) Len() int           { return len(x) }
func (x uint32s) Less(i, j int) bool { return x[i] < x[j] }
func (x uint32s) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// ErrEmptyRing is the error returned when trying to get an element when nothing has been added to hash.
var ErrEmptyRing = errors.New("empty circle")

var ingestorOwnershipDesc = prometheus.NewDesc(
	"prometheus_distributor_ingester_ownership_percent",
	"The percent ownership of the ring by ingestor",
	[]string{"ingester"}, nil,
)

// Ring holds the information about the members of the consistent hash circle.
type Ring struct {
	mtx          sync.RWMutex
	ingesters    map[string]IngesterDesc // source of truth - indexed by key
	circle       map[uint32]IngesterDesc // derived - indexed by token
	sortedHashes uint32s                 // derived
}

// NewRing creates a new Ring object.
func NewRing() *Ring {
	return &Ring{
		circle:    map[uint32]IngesterDesc{},
		ingesters: map[string]IngesterDesc{},
	}
}

// Update inserts a collector in the consistent hash.
func (r *Ring) Update(col IngesterDesc) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.ingesters[col.ID] = col
	r.updateSortedHashes()
}

func (r *Ring) Delete(col IngesterDesc) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	delete(r.ingesters, col.ID)
	r.updateSortedHashes()
}

// Get returns a collector close to the hash in the circle.
func (r *Ring) Get(key uint32) (IngesterDesc, error) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	if len(r.circle) == 0 {
		return IngesterDesc{}, ErrEmptyRing
	}
	i := r.search(key)
	return r.circle[r.sortedHashes[i]], nil
}

// GetAll returns all ingesters in the circle.
func (r *Ring) GetAll() []IngesterDesc {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	ingesters := make([]IngesterDesc, 0, len(r.ingesters))
	for _, c := range r.ingesters {
		// Ignore ingesters with no tokens.
		if len(c.Tokens) > 0 {
			ingesters = append(ingesters, c)
		}
	}
	return ingesters
}

func (r *Ring) search(key uint32) (i int) {
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
	hashes := uint32s{}
	circle := map[uint32]IngesterDesc{}
	for _, col := range r.ingesters {
		hashes = append(hashes, col.Tokens...)
		for _, token := range col.Tokens {
			circle[token] = col
		}
	}
	sort.Sort(hashes)
	r.sortedHashes = hashes
	r.circle = circle
}

// Describe implements prometheus.Collector.
func (r *Ring) Describe(ch chan<- *prometheus.Desc) {
	ch <- ingestorOwnershipDesc
}

// Collect implements prometheus.Collector.
func (r *Ring) Collect(ch chan<- prometheus.Metric) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	owned := map[string]uint32{}
	for i, token := range r.sortedHashes {
		var diff uint32
		if i+1 == len(r.sortedHashes) {
			diff = (math.MaxUint32 - token) + r.sortedHashes[0]
		} else {
			diff = r.sortedHashes[i+1] - token
		}
		collector := r.circle[token]
		owned[collector.ID] = owned[collector.ID] + diff
	}

	for id, totalOwned := range owned {
		ch <- prometheus.MustNewConstMetric(
			ingestorOwnershipDesc,
			prometheus.GaugeValue,
			float64(totalOwned)/float64(math.MaxUint32),
			id,
		)
	}
}
