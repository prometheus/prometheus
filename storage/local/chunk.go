// Copyright 2014 Prometheus Team
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

package local

import (
	"io"
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/storage/metric"
)

// Instrumentation.
// Note that the metrics are often set in bulk by the caller
// for performance reasons.
var (
	chunkOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunk_ops_total",
			Help:      "The total number of chunk operations by their type.",
		},
		[]string{opTypeLabel},
	)
	chunkDescOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunkdesc_ops_total",
			Help:      "The total number of chunk descriptor operations by their type.",
		},
		[]string{opTypeLabel},
	)
)

const (
	// Op-types for chunkOps.
	createAndPin    = "create" // A chunkDesc creation with refCount=1.
	persistAndUnpin = "persist"
	pin             = "pin"   // Excluding the pin on creation.
	unpin           = "unpin" // Excluding the unpin on persisting.
	clone           = "clone"
	transcode       = "transcode"
	purge           = "purge"

	// Op-types for chunkOps and chunkDescOps.
	evict = "evict"
	load  = "load"
)

func init() {
	prometheus.MustRegister(chunkOps)
	prometheus.MustRegister(chunkDescOps)
}

type chunkDesc struct {
	sync.Mutex
	chunk          chunk
	refCount       int
	evict          bool
	chunkFirstTime clientmodel.Timestamp // Used if chunk is evicted.
	chunkLastTime  clientmodel.Timestamp // Used if chunk is evicted.
}

// newChunkDesc creates a new chunkDesc pointing to the given chunk (may be
// nil). The refCount of the new chunkDesc is 1.
func newChunkDesc(c chunk) *chunkDesc {
	chunkOps.WithLabelValues(createAndPin).Inc()
	return &chunkDesc{chunk: c, refCount: 1}
}

func (cd *chunkDesc) add(s *metric.SamplePair) []chunk {
	cd.Lock()
	defer cd.Unlock()

	return cd.chunk.add(s)
}

func (cd *chunkDesc) pin() {
	cd.Lock()
	defer cd.Unlock()

	cd.refCount++
}

func (cd *chunkDesc) unpin() {
	cd.Lock()
	defer cd.Unlock()

	if cd.refCount == 0 {
		panic("cannot unpin already unpinned chunk")
	}
	cd.refCount--
	if cd.refCount == 0 && cd.evict {
		cd.evictNow()
	}
}

func (cd *chunkDesc) firstTime() clientmodel.Timestamp {
	cd.Lock()
	defer cd.Unlock()

	if cd.chunk == nil {
		return cd.chunkFirstTime
	}
	return cd.chunk.firstTime()
}

func (cd *chunkDesc) lastTime() clientmodel.Timestamp {
	cd.Lock()
	defer cd.Unlock()

	if cd.chunk == nil {
		return cd.chunkLastTime
	}
	return cd.chunk.lastTime()
}

func (cd *chunkDesc) isEvicted() bool {
	cd.Lock()
	defer cd.Unlock()

	return cd.chunk == nil
}

func (cd *chunkDesc) contains(t clientmodel.Timestamp) bool {
	return !t.Before(cd.firstTime()) && !t.After(cd.lastTime())
}

// setChunk points this chunkDesc to the given chunk. It panics if
// this chunkDesc already has a chunk set.
func (cd *chunkDesc) setChunk(c chunk) {
	cd.Lock()
	defer cd.Unlock()

	if cd.chunk != nil {
		panic("chunk already set")
	}
	cd.evict = false
	cd.chunk = c
}

// evictOnUnpin evicts the chunk once unpinned. If it is not pinned when this
// method is called, it evicts the chunk immediately and returns true. If the
// chunk is already evicted when this method is called, it returns true, too.
func (cd *chunkDesc) evictOnUnpin() bool {
	cd.Lock()
	defer cd.Unlock()

	if cd.chunk == nil {
		// Already evicted.
		return true
	}
	cd.evict = true
	if cd.refCount == 0 {
		cd.evictNow()
		return true
	}
	return false
}

// evictNow is an internal helper method.
func (cd *chunkDesc) evictNow() {
	cd.chunkFirstTime = cd.chunk.firstTime()
	cd.chunkLastTime = cd.chunk.lastTime()
	cd.chunk = nil
	chunkOps.WithLabelValues(evict).Inc()
}

// chunk is the interface for all chunks. Chunks are generally not
// goroutine-safe.
type chunk interface {
	// add adds a SamplePair to the chunks, performs any necessary
	// re-encoding, and adds any necessary overflow chunks. It returns the
	// new version of the original chunk, followed by overflow chunks, if
	// any. The first chunk returned might be the same as the original one
	// or a newly allocated version. In any case, take the returned chunk as
	// the relevant one and discard the orginal chunk.
	add(*metric.SamplePair) []chunk
	clone() chunk
	firstTime() clientmodel.Timestamp
	lastTime() clientmodel.Timestamp
	newIterator() chunkIterator
	marshal(io.Writer) error
	unmarshal(io.Reader) error
	// values returns a channel, from which all sample values in the chunk
	// can be received in order. The channel is closed after the last
	// one. It is generally not safe to mutate the chunk while the channel
	// is still open.
	values() <-chan *metric.SamplePair
}

// A chunkIterator enables efficient access to the content of a chunk. It is
// generally not safe to use a chunkIterator concurrently with or after chunk
// mutation.
type chunkIterator interface {
	// Gets the two values that are immediately adjacent to a given time. In
	// case a value exist at precisely the given time, only that single
	// value is returned. Only the first or last value is returned (as a
	// single value), if the given time is before or after the first or last
	// value, respectively.
	getValueAtTime(clientmodel.Timestamp) metric.Values
	// Gets all values contained within a given interval.
	getRangeValues(metric.Interval) metric.Values
	// Whether a given timestamp is contained between first and last value
	// in the chunk.
	contains(clientmodel.Timestamp) bool
}

func transcodeAndAdd(dst chunk, src chunk, s *metric.SamplePair) []chunk {
	chunkOps.WithLabelValues(transcode).Inc()

	head := dst
	body := []chunk{}
	for v := range src.values() {
		newChunks := head.add(v)
		body = append(body, newChunks[:len(newChunks)-1]...)
		head = newChunks[len(newChunks)-1]
	}
	newChunks := head.add(s)
	body = append(body, newChunks[:len(newChunks)-1]...)
	head = newChunks[len(newChunks)-1]
	return append(body, head)
}

func chunkType(c chunk) byte {
	switch c.(type) {
	case *deltaEncodedChunk:
		return 0
	default:
		panic("unknown chunk type")
	}
}

func chunkForType(chunkType byte) chunk {
	switch chunkType {
	case 0:
		return newDeltaEncodedChunk(d1, d0, true)
	default:
		panic("unknown chunk type")
	}
}
