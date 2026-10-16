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
	"hash/maphash"
	"slices"
	"strings"
	"sync"

	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/metadata"
)

const (
	nativeMetadataValueShards       = 64
	nativeMetadataValuesPerShard    = 64
	nativeMetadataRecentMisses      = 64
	nativeMetadataSharedStringBytes = 4 << 20
)

// nativeMetricMetadataValueCache shares immutable values within one Head. Entry
// and string-payload limits bound cache ownership, not live histories or results.
// Values own their strings and never refer back to this cache.
type nativeMetricMetadataValueCache struct {
	bytes  atomic.Int64
	seed   maphash.Seed
	shards [nativeMetadataValueShards]atomic.Pointer[nativeMetricMetadataValueShard]
}

func newNativeMetricMetadataValueCache() *nativeMetricMetadataValueCache {
	return &nativeMetricMetadataValueCache{seed: maphash.MakeSeed()}
}

// resolve returns an immutable owned value. Callers must not hold series locks.
// Sharing is opportunistic; exact value equality never depends on admission.
// strs, when not nil, copies each long string once per transaction.
func (c *nativeMetricMetadataValueCache) resolve(m metadata.Metadata, strs *nativeMetricMetadataStrings) *metadata.Metadata {
	// Values the Head stores always fit; larger values are copied and never cached.
	if !fits(&m) {
		return strs.cloneMetadata(m)
	}
	size := len(m.Type) + len(m.Unit) + len(m.Help)
	hash := maphash.Comparable(c.seed, m)
	slot := &c.shards[hash%nativeMetadataValueShards]
	shard := slot.Load()
	if shard == nil {
		shard = &nativeMetricMetadataValueShard{}
		if !slot.CompareAndSwap(nil, shard) {
			shard = slot.Load()
		}
	}
	shard.Lock()
	if existing := shard.values[m]; existing != nil {
		shard.Unlock()
		return existing
	}
	shard.Unlock()

	owned := strs.cloneMetadata(m)
	shard.Lock()
	defer shard.Unlock()
	if existing := shard.values[m]; existing != nil {
		return existing
	}
	// A second observation admits a value. Fingerprints retain no strings;
	// collisions can cause admission but cannot select a different value.
	if !slices.Contains(shard.recent[:shard.recentCount], hash) {
		shard.recent[shard.recentNext] = hash
		shard.recentNext = (shard.recentNext + 1) % len(shard.recent)
		shard.recentCount = min(shard.recentCount+1, len(shard.recent))
		return owned
	}
	for shard.count >= len(shard.fifo) || !c.reserve(int64(size)) {
		if shard.count == 0 {
			return owned
		}
		old := shard.fifo[shard.first]
		shard.fifo[shard.first] = nil
		shard.first = (shard.first + 1) % len(shard.fifo)
		shard.count--
		delete(shard.values, *old)
		c.bytes.Add(-int64(len(old.Type) + len(old.Unit) + len(old.Help)))
	}
	if shard.values == nil {
		shard.values = make(map[metadata.Metadata]*metadata.Metadata)
	}
	// The key must own strings too: using m here could pin caller buffers even
	// though the returned value has been cloned.
	shard.values[*owned] = owned
	shard.fifo[(shard.first+shard.count)%len(shard.fifo)] = owned
	shard.count++
	return owned
}

func (c *nativeMetricMetadataValueCache) reserve(size int64) bool {
	if size == 0 {
		return true
	}
	for {
		used := c.bytes.Load()
		if size > nativeMetadataSharedStringBytes-used {
			return false
		}
		if c.bytes.CompareAndSwap(used, used+size) {
			return true
		}
	}
}

// nativeMetricMetadataValueShard owns its map, FIFO and recent-miss ledger under
// one lock. Eviction releases references; published values are never modified.
type nativeMetricMetadataValueShard struct {
	sync.Mutex
	values      map[metadata.Metadata]*metadata.Metadata
	fifo        [nativeMetadataValuesPerShard]*metadata.Metadata
	first       int
	count       int
	recent      [nativeMetadataRecentMisses]uint64
	recentCount int
	recentNext  int
}

// nativeMetadataInternMinBytes is the length from which a transaction copies a
// metadata string only once.
const nativeMetadataInternMinBytes = 64

// nativeMetricMetadataStrings copies each long metadata string once per transaction.
type nativeMetricMetadataStrings map[string]string

// clone returns an owned copy of s, sharing the transaction's earlier copy of a
// long string. Keys are copies, so they never pin caller buffers.
func (strs *nativeMetricMetadataStrings) clone(s string) string {
	if len(s) < nativeMetadataInternMinBytes {
		return strings.Clone(s)
	}
	if c, ok := (*strs)[s]; ok {
		return c
	}
	if *strs == nil {
		*strs = make(nativeMetricMetadataStrings)
	}
	c := strings.Clone(s)
	(*strs)[c] = c
	return c
}

// cloneMetadata copies m's strings through strs; a nil strs copies each string.
func (strs *nativeMetricMetadataStrings) cloneMetadata(m metadata.Metadata) *metadata.Metadata {
	// Values without long strings need no lookups.
	if strs == nil || len(m.Type) < nativeMetadataInternMinBytes && len(m.Unit) < nativeMetadataInternMinBytes && len(m.Help) < nativeMetadataInternMinBytes {
		return cloneNativeMetricMetadata(m)
	}
	return &metadata.Metadata{
		Type: model.MetricType(strs.clone(string(m.Type))),
		Unit: strs.clone(m.Unit),
		Help: strs.clone(m.Help),
	}
}

func cloneNativeMetricMetadata(m metadata.Metadata) *metadata.Metadata {
	return &metadata.Metadata{
		Type: model.MetricType(strings.Clone(string(m.Type))),
		Unit: strings.Clone(m.Unit),
		Help: strings.Clone(m.Help),
	}
}

func equalNativeMetricMetadata(a, b *metadata.Metadata) bool {
	return a == b || a != nil && b != nil && *a == *b
}
