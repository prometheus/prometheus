// Copyright 2024 The Prometheus Authors
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
//
// Provenance-includes-location: https://github.com/dolthub/swiss/blob/f4b2babd2bc1cf0a2d66bab4e579ca35b6202338/map.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright 2023 Dolthub, Inc.

package tsdb

import (
	"math/bits"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

const (
	initialSeriesHashmapSize = 128
)

// Map is an open-addressing hash map
// based on Abseil's flat_hash_map.
type seriesHashmap struct {
	ctrl     []hashSuffixes
	groups   []group
	resident uint32
	dead     uint32
	limit    uint32
}

// hashSuffixes is the h2 hashSuffixes array for a group.
// Find operations first probe the controls bytes to filter candidates before matching keys.
type hashSuffixes [groupSize]uint8

// group is a group of 16 key-value pairs.
type group struct {
	hashes [groupSize]uint64
	series [groupSize]*memSeries
}

const (
	h1Mask    uint64 = 0xffff_ffff_ffff_ff80
	h2Mask    uint64 = 0x0000_0000_0000_007f
	h2Offset         = 2
	empty     uint8  = 0b0000_0000
	tombstone uint8  = 0b0000_0001
)

// h1 is a 57 bit hash prefix.
type h1 uint64

// h2 is a 7 bit hash suffix.
type h2 uint8

func (m *seriesHashmap) get(hash uint64, lset labels.Labels) *memSeries {
	if len(m.groups) == 0 {
		return nil
	}

	hi, lo := splitHash(hash)
	g := probeStart(hi, len(m.groups))
	for { // inlined find loop
		matches := metaMatchH2(&m.ctrl[g], lo)
		for matches != 0 {
			s := nextMatch(&matches)
			if hash == m.groups[g].hashes[s] && labels.Equal(lset, m.groups[g].series[s].labels()) {
				return m.groups[g].series[s]
			}
		}
		// |key| is not in group |g|,
		// stop probing if we see an empty slot
		matches = metaMatchEmpty(&m.ctrl[g])
		if matches != 0 {
			return nil
		}
		if g++; g >= uint32(len(m.groups)) {
			g = 0
		}
	}
}

func (m *seriesHashmap) set(hash uint64, series *memSeries) {
	if m.resident >= m.limit {
		m.rehash(m.nextSize())
	}

	hi, lo := splitHash(hash)
	g := probeStart(hi, len(m.groups))
	for { // inlined find loop
		matches := metaMatchH2(&m.ctrl[g], lo)
		for matches != 0 {
			s := nextMatch(&matches)
			// We only read series.labels() if we actually have the same hash,
			// because the implementation of series.labels() is expensive with dedupelabels.
			if hash == m.groups[g].hashes[s] && labels.Equal(series.labels(), m.groups[g].series[s].labels()) { // update (do we expect updates here? this is just inherited from swiss map)
				m.groups[g].hashes[s] = hash
				m.groups[g].series[s] = series
				return
			}
		}
		// |key| is not in group |g|,
		// stop probing if we see an empty slot
		if matches := metaMatchEmpty(&m.ctrl[g]); matches != 0 { // insert
			s := nextMatch(&matches)
			m.groups[g].hashes[s] = hash
			m.groups[g].series[s] = series
			m.ctrl[g][s] = uint8(lo)
			m.resident++
			return
		}
		if g++; g >= uint32(len(m.groups)) {
			g = 0
		}
	}
}

func (m *seriesHashmap) del(hash uint64, ref chunks.HeadSeriesRef) {
	if len(m.groups) == 0 {
		return
	}

	hi, lo := splitHash(hash)
	g := probeStart(hi, len(m.groups))
	for {
		matches := metaMatchH2(&m.ctrl[g], lo)
		for matches != 0 {
			s := nextMatch(&matches)
			if hash == m.groups[g].hashes[s] && ref == m.groups[g].series[s].ref { // update (do we expect updates here? this is just inherited from swiss map)
				// optimization: if |m.ctrl[g]| contains any empty
				// hashSuffixes bytes, we can physically delete |key|
				// rather than placing a tombstone.
				// The observation is that any probes into group |g|
				// would already be terminated by the existing empty
				// slot, and therefore reclaiming slot |s| will not
				// cause premature termination of probes into |g|.
				if metaMatchEmpty(&m.ctrl[g]) != 0 {
					m.ctrl[g][s] = empty
					m.resident--
				} else {
					m.ctrl[g][s] = tombstone
					m.dead++
				}
				m.groups[g].hashes[s] = 0
				m.groups[g].series[s] = nil
				return
			}
		}
		// |key| is not in group |g|,
		// stop probing if we see an empty slot
		if matches := metaMatchEmpty(&m.ctrl[g]); matches != 0 { // |key| absent
			return
		}
		if g++; g >= uint32(len(m.groups)) {
			g = 0
		}
	}
}

// iter iterates the series of the seriesHashmap, passing them to the callback.
// It guarantees that any key in the Map will be visited only once,
// and or un-mutated maps, every key will be visited once.
// Series added while iterating might, or might not be visited.
// Series deleted from outside the iterator's callback might be iterated.
func (m *seriesHashmap) iter(cb func(uint64, *memSeries) (stop bool)) {
	if len(m.groups) == 0 {
		return
	}

	// take a consistent view of the table in case
	// we rehash during iteration
	ctrl, groups := m.ctrl, m.groups
	// pick a random starting group
	g := randIntN(len(groups))
	for n := 0; n < len(groups); n++ {
		for s, c := range ctrl[g] {
			if c == empty || c == tombstone {
				continue
			}
			if stop := cb(groups[g].hashes[s], groups[g].series[s]); stop {
				return
			}
		}
		g++
		if g >= uint32(len(groups)) {
			g = 0
		}
	}
}

func (m *seriesHashmap) nextSize() (n uint32) {
	if len(m.groups) == 0 {
		return numGroups(initialSeriesHashmapSize)
	}
	n = uint32(len(m.groups)) * 2
	if m.dead >= (m.resident / 2) {
		n = uint32(len(m.groups))
	}
	return
}

func (m *seriesHashmap) rehash(n uint32) {
	groups, ctrl := m.groups, m.ctrl
	m.groups = make([]group, n)
	m.ctrl = make([]hashSuffixes, n)
	m.limit = n * maxAvgSeriesHashmapGroupLoad
	m.resident, m.dead = 0, 0
	for g := range ctrl {
		for s := range ctrl[g] {
			c := ctrl[g][s]
			if c == empty || c == tombstone {
				continue
			}
			m.set(groups[g].hashes[s], groups[g].series[s])
		}
	}
}

// numGroups returns the minimum number of groups needed to store |n| elems.
func numGroups(n uint32) (groups uint32) {
	groups = (n + maxAvgSeriesHashmapGroupLoad - 1) / maxAvgSeriesHashmapGroupLoad
	if groups == 0 {
		groups = 1
	}
	return
}

// splitHash extracts the h1 and h2 components from a 64 bit hash.
// h1 is the upper 57 bits, h2 is the lower 7 bits plus two.
// By adding 2, it ensures that h2 is never uint8(0) or uint8(1).
func splitHash(h uint64) (h1, h2) {
	return h1((h & h1Mask) >> 7), h2(h&h2Mask) + h2Offset
}

func probeStart(hi h1, groups int) uint32 {
	return fastModN(uint32(hi), uint32(groups))
}

// fastModN is an alternative to modulo operation to evenly distribute keys.
// lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/.
func fastModN(x, n uint32) uint32 {
	return uint32((uint64(x) * uint64(n)) >> 32)
}

const (
	groupSize                    = 8
	maxAvgSeriesHashmapGroupLoad = 7

	loBits uint64 = 0x0101010101010101
	hiBits uint64 = 0x8080808080808080
)

type bitset uint64

func metaMatchH2(m *hashSuffixes, h h2) bitset {
	// https://graphics.stanford.edu/~seander/bithacks.html##ValueInWord
	return hasZeroByte(castUint64(m) ^ (loBits * uint64(h)))
}

func metaMatchEmpty(m *hashSuffixes) bitset {
	return hasZeroByte(castUint64(m))
}

func nextMatch(b *bitset) uint32 {
	s := uint32(bits.TrailingZeros64(uint64(*b)))
	*b &= ^(1 << s) // clear bit |s|
	return s >> 3   // div by 8
}

func hasZeroByte(x uint64) bitset {
	return bitset(((x - loBits) & ^(x)) & hiBits)
}

func castUint64(m *hashSuffixes) uint64 {
	return *(*uint64)((unsafe.Pointer)(m))
}
