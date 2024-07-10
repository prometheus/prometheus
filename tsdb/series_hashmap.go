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
	meta     []hashMeta
	hashes   [][groupSize]uint64
	series   [][groupSize]*memSeries
	resident uint32
	dead     uint32
	limit    uint32
}

// hashMeta is the metadata array for a group.
// It contains either `empty` or `tombstone` marks, or the `h1` hash prefix for an item, if there's one in that bucket.
// Find operations first probe the controls bytes to filter candidates before matching keys,
// and to know when to stop searching.
type hashMeta [groupSize]uint8

const (
	h1Offset        = 2
	empty     uint8 = 0b0000_0000
	tombstone uint8 = 0b0000_0001
)

// h1 is a 7 bit hash prefix.
type h1 uint8

// h2 is a 57 bit hash suffix.
type h2 uint64

func (m *seriesHashmap) get(hash uint64, lset labels.Labels) *memSeries {
	if len(m.meta) == 0 {
		return nil
	}

	hi, lo := splitHash(hash)
	g := probeStart(lo, len(m.meta))
	for { // inlined find loop
		matches := metaMatchH1(&m.meta[g], hi)
		for matches != 0 {
			s := nextMatch(&matches)
			if hash != m.hashes[g][s] {
				continue
			}

			if labels.Equal(lset, m.series[g][s].labels()) {
				return m.series[g][s]
			}
		}
		// |key| is not in group |g|,
		// stop probing if we see an empty slot
		matches = metaMatchEmpty(&m.meta[g])
		if matches != 0 {
			return nil
		}
		if g++; g >= uint32(len(m.meta)) {
			g = 0
		}
	}
}

func (m *seriesHashmap) set(hash uint64, series *memSeries) {
	if m.resident >= m.limit {
		m.rehash(m.nextSize())
	}

	hi, lo := splitHash(hash)
	g := probeStart(lo, len(m.meta))
	for { // inlined find loop
		matches := metaMatchH1(&m.meta[g], hi)
		for matches != 0 {
			s := nextMatch(&matches)
			if hash != m.hashes[g][s] {
				continue
			}

			// We only read series.labels() if we actually have the same hash,
			// because the implementation of series.labels() is expensive with dedupelabels.
			if labels.Equal(series.labels(), m.series[g][s].labels()) { // update, although we could also panic: I think we don't expect updates in the series hashmap
				m.hashes[g][s] = hash
				m.series[g][s] = series
				return
			}
		}
		// |key| is not in group |g|,
		// stop probing if we see an empty slot
		if matches := metaMatchEmpty(&m.meta[g]); matches != 0 { // insert
			s := nextMatch(&matches)
			m.hashes[g][s] = hash
			m.series[g][s] = series
			m.meta[g][s] = uint8(hi)
			m.resident++
			return
		}
		if g++; g >= uint32(len(m.meta)) {
			g = 0
		}
	}
}

func (m *seriesHashmap) del(hash uint64, ref chunks.HeadSeriesRef) {
	if len(m.meta) == 0 {
		return
	}

	hi, lo := splitHash(hash)
	g := probeStart(lo, len(m.meta))
	for {
		matches := metaMatchH1(&m.meta[g], hi)
		for matches != 0 {
			s := nextMatch(&matches)
			if hash == m.hashes[g][s] && ref == m.series[g][s].ref {
				// optimization: if |m.meta[g]| contains any empty
				// meta bytes, we can physically delete |key|
				// rather than placing a tombstone.
				// The observation is that any probes into group |g|
				// would already be terminated by the existing empty
				// slot, and therefore reclaiming slot |s| will not
				// cause premature termination of probes into |g|.
				if metaMatchEmpty(&m.meta[g]) != 0 {
					m.meta[g][s] = empty
					m.resident--
				} else {
					m.meta[g][s] = tombstone
					m.dead++
				}
				m.hashes[g][s] = 0
				m.series[g][s] = nil
				return
			}
		}
		// |key| is not in group |g|,
		// stop probing if we see an empty slot
		if matches := metaMatchEmpty(&m.meta[g]); matches != 0 { // |key| absent
			return
		}
		if g++; g >= uint32(len(m.meta)) {
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
	if len(m.meta) == 0 {
		return
	}

	// take a consistent view of the table in case
	// we rehash during iteration
	meta, hashes, series := m.meta, m.hashes, m.series
	// pick a random starting group
	g := randIntN(len(meta))
	for n := 0; n < len(meta); n++ {
		meta, hashes, series := meta[g], hashes[g], series[g]
		for s, c := range meta {
			if c == empty || c == tombstone {
				continue
			}
			if stop := cb(hashes[s], series[s]); stop {
				return
			}
		}

		if g++; g >= uint32(len(m.meta)) {
			g = 0
		}
	}
}

func (m *seriesHashmap) nextSize() (n uint32) {
	if len(m.meta) == 0 {
		return numGroups(initialSeriesHashmapSize)
	}
	n = uint32(len(m.meta)) * 2
	if m.dead >= (m.resident / 2) {
		n = uint32(len(m.meta))
	}
	return
}

func (m *seriesHashmap) rehash(n uint32) {
	meta, hashes, series := m.meta, m.hashes, m.series
	m.meta = make([]hashMeta, n)
	m.hashes = make([][groupSize]uint64, n)
	m.series = make([][groupSize]*memSeries, n)
	m.limit = n * maxAvgSeriesHashmapGroupLoad
	m.resident, m.dead = 0, 0
	for g := range meta {
		meta, hashes, series := meta[g], hashes[g], series[g]
		for s := range meta {
			c := meta[s]
			if c == empty || c == tombstone {
				continue
			}
			m.set(hashes[s], series[s])
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
// h1 is the upper 7 bits plus two, h2 is the lower 57 bits.
// By adding 2 to h1, it ensures that h1 is never uint8(0) or uint8(1).
func splitHash(h uint64) (h1, h2) {
	const h2Mask uint64 = 0x01ff_ffff_ffff_ffff
	return h1(h>>57) + h1Offset, h2(h & h2Mask)
}

func probeStart(lo h2, groups int) uint32 {
	// Since x is `lo` here, which are the lower 57 bits of a 64 bit hash,
	// and this is used in stripeSeries, with 16K stripes by default,
	// that means that most likely (except for a config change)
	// the lower 14 bits of lo in this hashmap are always the same.
	// However, as we shift 32 bits right in the fastModN function,
	// We'll discard those equal bits.
	// If log2(n) (log2(groups)) is > (32-14)=18, i.e. 260K groups,
	// we'll start having a bias in the distribution of the keys,
	// but at that point our instance would have ~(8*MaxUint32) series (stripes * groups * group size).
	return fastModN(uint32(lo), uint32(groups))
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

func metaMatchH1(m *hashMeta, h h1) bitset {
	// https://graphics.stanford.edu/~seander/bithacks.html##ValueInWord
	return hasZeroByte(castUint64(m) ^ (loBits * uint64(h)))
}

func metaMatchEmpty(m *hashMeta) bitset {
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

func castUint64(m *hashMeta) uint64 {
	return *(*uint64)((unsafe.Pointer)(m))
}
