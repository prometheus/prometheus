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
	"math"
	"sync"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/storage"
)

const (
	maxShardedPostingsIdleBuffers = 128
	seriesRefBytes                = uint64(unsafe.Sizeof(storage.SeriesRef(0)))
)

// ShardedPostingsBufferRecycler reuses dirty-repair buffers across Heads.
// It is safe for concurrent use.
type ShardedPostingsBufferRecycler struct {
	mtx              sync.Mutex
	maxRetainedBytes uint64
	retainedBytes    uint64
	idle             [][]storage.SeriesRef // Oldest to newest.

	pendingBuffers atomic.Uint64
	pendingBytes   atomic.Uint64
	metrics        shardedPostingsBufferRecyclerMetrics
}

type shardedPostingsBufferRecyclerMetrics struct {
	hits            prometheus.Counter
	misses          prometheus.Counter
	evictions       prometheus.Counter
	rejections      prometheus.Counter
	retainedBuffers prometheus.Gauge
	retainedBytes   prometheus.Gauge
	pendingBuffers  prometheus.GaugeFunc
	pendingBytes    prometheus.GaugeFunc
}

// NewShardedPostingsBufferRecycler creates a recycler bounded by retained
// capacity. It returns nil when maxRetainedBytes is zero. Metrics use the
// "tsdb_" prefix so callers can add their product namespace via reg.
func NewShardedPostingsBufferRecycler(maxRetainedBytes uint64, reg prometheus.Registerer) *ShardedPostingsBufferRecycler {
	if maxRetainedBytes == 0 {
		return nil
	}

	r := &ShardedPostingsBufferRecycler{maxRetainedBytes: maxRetainedBytes}
	r.metrics = shardedPostingsBufferRecyclerMetrics{
		hits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_hits_total",
			Help: "Total number of shard repair buffers reused by the recycler.",
		}),
		misses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_misses_total",
			Help: "Total number of shard repair buffer requests not served by the recycler.",
		}),
		evictions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_evictions_total",
			Help: "Total number of idle shard repair buffers evicted by recycler limits.",
		}),
		rejections: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_rejections_total",
			Help: "Total number of shard repair buffers too large for the recycler budget.",
		}),
		retainedBuffers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_retained_buffers",
			Help: "Number of idle shard repair buffers retained by the recycler.",
		}),
		retainedBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_retained_capacity_bytes",
			Help: "Capacity in bytes of idle shard repair buffers retained by the recycler.",
		}),
		pendingBuffers: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_pending_retirement_buffers",
			Help: "Number of displaced shard repair buffers still visible to readers.",
		}, func() float64 {
			return float64(r.pendingBuffers.Load())
		}),
		pendingBytes: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "tsdb_sharded_postings_buffer_recycler_pending_retirement_capacity_bytes",
			Help: "Capacity in bytes of displaced shard repair buffers still visible to readers.",
		}, func() float64 {
			return float64(r.pendingBytes.Load())
		}),
	}
	if reg != nil {
		reg.MustRegister(
			r.metrics.hits,
			r.metrics.misses,
			r.metrics.evictions,
			r.metrics.rejections,
			r.metrics.retainedBuffers,
			r.metrics.retainedBytes,
			r.metrics.pendingBuffers,
			r.metrics.pendingBytes,
		)
	}
	return r
}

func (r *ShardedPostingsBufferRecycler) get(length, minCapacity int) []storage.SeriesRef {
	if r == nil {
		return nil
	}

	maxCapacity := minCapacity + max(minCapacity/4, replacementTailHeadroom)
	if maxCapacity < minCapacity {
		maxCapacity = math.MaxInt
	}

	r.mtx.Lock()
	best := -1
	for i, refs := range r.idle {
		capacity := cap(refs)
		if capacity < minCapacity || capacity > maxCapacity {
			continue
		}
		if best < 0 || capacity < cap(r.idle[best]) {
			best = i
		}
	}
	if best < 0 {
		r.metrics.misses.Inc()
		r.mtx.Unlock()
		return nil
	}

	refs := r.idle[best]
	r.retainedBytes -= shardPostingsBufferBytes(refs)
	copy(r.idle[best:], r.idle[best+1:])
	r.idle[len(r.idle)-1] = nil
	r.idle = r.idle[:len(r.idle)-1]
	r.metrics.hits.Inc()
	r.updateRetainedMetricsLocked()
	r.mtx.Unlock()
	return refs[:length]
}

func (r *ShardedPostingsBufferRecycler) put(refs []storage.SeriesRef) {
	if r == nil || cap(refs) == 0 {
		return
	}

	refs = refs[:0]
	bytes := shardPostingsBufferBytes(refs)
	if bytes > r.maxRetainedBytes {
		r.metrics.rejections.Inc()
		return
	}

	r.mtx.Lock()
	for len(r.idle) >= maxShardedPostingsIdleBuffers || r.retainedBytes > r.maxRetainedBytes-bytes {
		r.retainedBytes -= shardPostingsBufferBytes(r.idle[0])
		copy(r.idle, r.idle[1:])
		r.idle[len(r.idle)-1] = nil
		r.idle = r.idle[:len(r.idle)-1]
		r.metrics.evictions.Inc()
	}
	r.idle = append(r.idle, refs)
	r.retainedBytes += bytes
	r.updateRetainedMetricsLocked()
	r.mtx.Unlock()
}

func (r *ShardedPostingsBufferRecycler) updateRetainedMetricsLocked() {
	r.metrics.retainedBuffers.Set(float64(len(r.idle)))
	r.metrics.retainedBytes.Set(float64(r.retainedBytes))
}

func (r *ShardedPostingsBufferRecycler) addPending(refs []storage.SeriesRef) {
	r.pendingBuffers.Inc()
	r.pendingBytes.Add(shardPostingsBufferBytes(refs))
}

func (r *ShardedPostingsBufferRecycler) removePending(refs []storage.SeriesRef) {
	r.pendingBuffers.Dec()
	r.pendingBytes.Sub(shardPostingsBufferBytes(refs))
}

func shardPostingsBufferBytes(refs []storage.SeriesRef) uint64 {
	return uint64(cap(refs)) * seriesRefBytes
}

type shardPostingsBufferLifecycle struct {
	mtx sync.Mutex

	lastReaderSequence uint64
	oldestReader       *shardPostingsReaderLease
	newestReader       *shardPostingsReaderLease
	retired            []retiredShardPostingsBuffer
	retiredHead        int
	recycler           *ShardedPostingsBufferRecycler
}

type shardPostingsReaderLease struct {
	lifecycle *shardPostingsBufferLifecycle
	sequence  uint64
	previous  *shardPostingsReaderLease
	next      *shardPostingsReaderLease
}

type retiredShardPostingsBuffer struct {
	refs               []storage.SeriesRef
	lastReaderSequence uint64
}

func newShardPostingsBufferLifecycle(recycler *ShardedPostingsBufferRecycler) *shardPostingsBufferLifecycle {
	if recycler == nil {
		return nil
	}
	return &shardPostingsBufferLifecycle{recycler: recycler}
}

func (l *shardPostingsBufferLifecycle) acquireReader(lease *shardPostingsReaderLease) {
	if l == nil {
		return
	}

	lease.lifecycle = l
	l.mtx.Lock()
	l.lastReaderSequence++
	lease.sequence = l.lastReaderSequence
	lease.previous = l.newestReader
	if l.newestReader == nil {
		l.oldestReader = lease
	} else {
		l.newestReader.next = lease
	}
	l.newestReader = lease
	l.mtx.Unlock()
}

func (l *shardPostingsBufferLifecycle) retire(refs ...[]storage.SeriesRef) {
	if l == nil {
		return
	}

	l.mtx.Lock()
	lastReaderSequence := l.lastReaderSequence
	if l.oldestReader == nil || l.oldestReader.sequence > lastReaderSequence {
		l.mtx.Unlock()
		for _, refs := range refs {
			l.recycler.put(refs)
		}
		return
	}
	for _, refs := range refs {
		if cap(refs) == 0 {
			continue
		}
		if l.retiredHead > 0 && len(l.retired) == cap(l.retired) {
			remaining := copy(l.retired, l.retired[l.retiredHead:])
			clear(l.retired[remaining:])
			l.retired = l.retired[:remaining]
			l.retiredHead = 0
		}
		l.retired = append(l.retired, retiredShardPostingsBuffer{
			refs:               refs,
			lastReaderSequence: lastReaderSequence,
		})
		l.recycler.addPending(refs)
	}
	l.mtx.Unlock()
}

func (l *shardPostingsBufferLifecycle) put(refs []storage.SeriesRef) {
	if l != nil {
		l.recycler.put(refs)
	}
}

func (l *shardPostingsBufferLifecycle) get(length, capacity int) []storage.SeriesRef {
	if l == nil {
		return nil
	}
	return l.recycler.get(length, capacity)
}

func (lease *shardPostingsReaderLease) close() {
	if lease == nil || lease.lifecycle == nil {
		return
	}

	l := lease.lifecycle
	l.mtx.Lock()
	if lease.lifecycle != l {
		l.mtx.Unlock()
		return
	}
	if lease.previous == nil {
		l.oldestReader = lease.next
	} else {
		lease.previous.next = lease.next
	}
	if lease.next == nil {
		l.newestReader = lease.previous
	} else {
		lease.next.previous = lease.previous
	}
	lease.lifecycle = nil
	lease.previous = nil
	lease.next = nil

	var ready [maxShardedPostingsIdleBuffers][]storage.SeriesRef
	for {
		n := 0
		for n < len(ready) && l.retiredHead < len(l.retired) && (l.oldestReader == nil || l.oldestReader.sequence > l.retired[l.retiredHead].lastReaderSequence) {
			buffer := &l.retired[l.retiredHead]
			ready[n] = buffer.refs
			l.recycler.removePending(buffer.refs)
			*buffer = retiredShardPostingsBuffer{}
			l.retiredHead++
			n++
		}
		if l.retiredHead == len(l.retired) {
			l.retired = l.retired[:0]
			l.retiredHead = 0
		}
		l.mtx.Unlock()

		for i := range n {
			l.recycler.put(ready[i])
			ready[i] = nil
		}
		if n < len(ready) {
			return
		}
		l.mtx.Lock()
	}
}
