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

package remote

import (
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

const (
	// defaultRingBufferSize is the fixed capacity of pending exemplars per shard coalescer.
	defaultRingBufferSize = 2048

	// maxCoalescingTimeDeltaMs is the maximum allowed time delta (50ms) between a sample
	// and an exemplar for them to be considered part of the same scrape.
	maxCoalescingTimeDeltaMs int64 = 50
)

type coalescerIndexEntry struct {
	slotIdx    int
	generation uint64
}

type coalescerSlot struct {
	seriesRef  chunks.HeadSeriesRef
	exemplar   exemplar.Exemplar
	generation uint64
	nextSlot   int
	tombstone  bool
}

// shardCoalescer manages in-memory correlation and coalescing of samples and exemplars
// within a single Remote Write QueueManager shard worker.
// It is single-threaded per shard worker goroutine (lock-free).
type shardCoalescer struct {
	capacity    int
	slots       []coalescerSlot
	generations []uint64
	seriesIndex map[chunks.HeadSeriesRef]coalescerIndexEntry
	head        int
	onDrop      func(exemplar.Exemplar)
}

func newShardCoalescer(capacity int, onDrop func(exemplar.Exemplar)) *shardCoalescer {
	if capacity <= 0 {
		capacity = defaultRingBufferSize
	}
	slots := make([]coalescerSlot, capacity)
	for i := range slots {
		slots[i].nextSlot = -1
		slots[i].tombstone = true
	}
	generations := make([]uint64, capacity)
	for i := range generations {
		generations[i] = 1
	}
	return &shardCoalescer{
		capacity:    capacity,
		slots:       slots,
		generations: generations,
		seriesIndex: make(map[chunks.HeadSeriesRef]coalescerIndexEntry, capacity),
		head:        0,
		onDrop:      onDrop,
	}
}

// TryAttachToBatch checks if there is an un-flushed sample or histogram in the active batch
// matching the exemplar's seriesRef and scrape timestamp (|sample.T - ex.Ts| <= 50ms).
// If found, attaches the exemplar to that batch item and returns true.
func (c *shardCoalescer) TryAttachToBatch(batch []timeSeries, ref chunks.HeadSeriesRef, ex exemplar.Exemplar) bool {
	// Search from newest to oldest in the active batch.
	for i := len(batch) - 1; i >= 0; i-- {
		item := &batch[i]
		if item.seriesRef != ref {
			continue
		}
		if item.sType != tSample && item.sType != tHistogram && item.sType != tFloatHistogram {
			continue
		}
		diff := item.timestamp - ex.Ts
		if diff < 0 {
			diff = -diff
		}
		if diff <= maxCoalescingTimeDeltaMs {
			item.exemplars = append(item.exemplars, ex)
			return true
		}
	}
	return false
}

// AddPendingExemplar inserts an exemplar into the circular ring buffer.
// If the write pointer wraps around and overwrites an active pending exemplar,
// the overwritten exemplar is evicted and onDrop is invoked.
func (c *shardCoalescer) AddPendingExemplar(ref chunks.HeadSeriesRef, ex exemplar.Exemplar) {
	slotIdx := c.head % c.capacity
	oldSlot := &c.slots[slotIdx]

	// Check if we are overwriting an active slot due to wrap-around.
	if !oldSlot.tombstone && oldSlot.generation == c.generations[slotIdx] && oldSlot.seriesRef != 0 {
		if c.onDrop != nil {
			c.onDrop(oldSlot.exemplar)
		}
		oldSlot.tombstone = true
		if curEntry, ok := c.seriesIndex[oldSlot.seriesRef]; ok && curEntry.slotIdx == slotIdx && curEntry.generation == oldSlot.generation {
			if oldSlot.nextSlot >= 0 && c.isValidSlot(oldSlot.nextSlot, oldSlot.seriesRef) {
				c.seriesIndex[oldSlot.seriesRef] = coalescerIndexEntry{
					slotIdx:    oldSlot.nextSlot,
					generation: c.slots[oldSlot.nextSlot].generation,
				}
			} else {
				delete(c.seriesIndex, oldSlot.seriesRef)
			}
		}
	}

	c.generations[slotIdx]++
	gen := c.generations[slotIdx]

	nextSlot := -1
	if existing, ok := c.seriesIndex[ref]; ok {
		if c.isValidSlot(existing.slotIdx, ref) && existing.generation == c.slots[existing.slotIdx].generation {
			nextSlot = existing.slotIdx
		}
	}

	c.slots[slotIdx] = coalescerSlot{
		seriesRef:  ref,
		exemplar:   ex,
		generation: gen,
		nextSlot:   nextSlot,
		tombstone:  false,
	}
	c.seriesIndex[ref] = coalescerIndexEntry{
		slotIdx:    slotIdx,
		generation: gen,
	}

	c.head++
}

func (c *shardCoalescer) isValidSlot(slotIdx int, ref chunks.HeadSeriesRef) bool {
	if slotIdx < 0 || slotIdx >= c.capacity {
		return false
	}
	s := &c.slots[slotIdx]
	return !s.tombstone && s.seriesRef == ref && s.generation == c.generations[slotIdx]
}

// TryAttachMatchingExemplars finds and removes all matching pending exemplars in the ring buffer
// for the given seriesRef and sample timestamp (|sampleTs - ex.Ts| <= 50ms).
// Any expired exemplars (sampleTs - ex.Ts > 50ms) are tombstoned, dropped, and notified via onDrop.
func (c *shardCoalescer) TryAttachMatchingExemplars(ref chunks.HeadSeriesRef, sampleTs int64) []exemplar.Exemplar {
	entry, ok := c.seriesIndex[ref]
	if !ok {
		return nil
	}

	var matched []exemplar.Exemplar
	currIdx := entry.slotIdx

	for currIdx >= 0 && currIdx < c.capacity {
		slot := &c.slots[currIdx]
		nextIdx := slot.nextSlot

		// Validate generation & tombstone.
		if slot.generation == c.generations[currIdx] && !slot.tombstone && slot.seriesRef == ref {
			diff := sampleTs - slot.exemplar.Ts
			if diff < 0 {
				diff = -diff
			}

			if diff <= maxCoalescingTimeDeltaMs {
				matched = append(matched, slot.exemplar)
				slot.tombstone = true
			} else if sampleTs > slot.exemplar.Ts+maxCoalescingTimeDeltaMs {
				// Exemplar is stale from an earlier scrape interval.
				slot.tombstone = true
				if c.onDrop != nil {
					c.onDrop(slot.exemplar)
				}
			}
		}

		currIdx = nextIdx
	}

	delete(c.seriesIndex, ref)
	return matched
}

// EvictOlderThan evicts any pending exemplars older than cutoffTimestamp - maxCoalescingTimeDeltaMs.
func (c *shardCoalescer) EvictOlderThan(cutoffTimestamp int64) int {
	evicted := 0
	for i := range c.slots {
		slot := &c.slots[i]
		if !slot.tombstone && slot.generation == c.generations[i] && slot.seriesRef != 0 {
			if slot.exemplar.Ts < cutoffTimestamp-maxCoalescingTimeDeltaMs {
				slot.tombstone = true
				if curEntry, ok := c.seriesIndex[slot.seriesRef]; ok && curEntry.slotIdx == i && curEntry.generation == slot.generation {
					delete(c.seriesIndex, slot.seriesRef)
				}
				if c.onDrop != nil {
					c.onDrop(slot.exemplar)
				}
				evicted++
			}
		}
	}
	return evicted
}

// FlushAndClear drains all pending exemplars from the coalescer, invoking onDrop for each,
// and resets the ring buffer and index.
func (c *shardCoalescer) FlushAndClear() int {
	dropped := 0
	for i := range c.slots {
		slot := &c.slots[i]
		if !slot.tombstone && slot.generation == c.generations[i] && slot.seriesRef != 0 {
			slot.tombstone = true
			if c.onDrop != nil {
				c.onDrop(slot.exemplar)
			}
			dropped++
		}
		slot.seriesRef = 0
		slot.nextSlot = -1
		slot.tombstone = true
	}
	clear(c.seriesIndex)
	c.head = 0
	return dropped
}

// PendingCount returns the number of active un-tombstoned exemplars in the ring buffer.
func (c *shardCoalescer) PendingCount() int {
	count := 0
	for i := range c.slots {
		slot := &c.slots[i]
		if !slot.tombstone && slot.generation == c.generations[i] && slot.seriesRef != 0 {
			count++
		}
	}
	return count
}
