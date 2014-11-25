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
	"container/list"
	"io"
	"sync"
	"sync/atomic"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// chunkDesc contains meta-data for a chunk. Many of its methods are
// goroutine-safe proxies for chunk methods.
type chunkDesc struct {
	sync.Mutex
	chunk          chunk // nil if chunk is evicted.
	refCount       int
	chunkFirstTime clientmodel.Timestamp // Used if chunk is evicted.
	chunkLastTime  clientmodel.Timestamp // Used if chunk is evicted.

	// evictListElement is nil if the chunk is not in the evict list.
	// evictListElement is _not_ protected by the chunkDesc mutex.
	// It must only be touched by the evict list handler in memorySeriesStorage.
	evictListElement *list.Element
}

// newChunkDesc creates a new chunkDesc pointing to the provided chunk. The
// provided chunk is assumed to be not persisted yet. Therefore, the refCount of
// the new chunkDesc is 1 (preventing eviction prior to persisting).
func newChunkDesc(c chunk) *chunkDesc {
	chunkOps.WithLabelValues(createAndPin).Inc()
	atomic.AddInt64(&numMemChunks, 1)
	// TODO: numMemChunkDescs is actually never read except during metrics
	// collection. Turn it into a real metric.
	atomic.AddInt64(&numMemChunkDescs, 1)
	return &chunkDesc{chunk: c, refCount: 1}
}

func (cd *chunkDesc) add(s *metric.SamplePair) []chunk {
	cd.Lock()
	defer cd.Unlock()

	return cd.chunk.add(s)
}

// pin increments the refCount by one. Upon increment from 0 to 1, this
// chunkDesc is removed from the evict list. To enable the latter, the
// evictRequests channel has to be provided.
func (cd *chunkDesc) pin(evictRequests chan<- evictRequest) {
	cd.Lock()
	defer cd.Unlock()

	if cd.refCount == 0 {
		// Remove ourselves from the evict list.
		evictRequests <- evictRequest{cd, false}
	}
	cd.refCount++
}

// unpin decrements the refCount by one. Upon decrement from 1 to 0, this
// chunkDesc is added to the evict list. To enable the latter, the evictRequests
// channel has to be provided.
func (cd *chunkDesc) unpin(evictRequests chan<- evictRequest) {
	cd.Lock()
	defer cd.Unlock()

	if cd.refCount == 0 {
		panic("cannot unpin already unpinned chunk")
	}
	cd.refCount--
	if cd.refCount == 0 {
		// Add ourselves to the back of the evict list.
		evictRequests <- evictRequest{cd, true}
	}
}

func (cd *chunkDesc) getRefCount() int {
	cd.Lock()
	defer cd.Unlock()

	return cd.refCount
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

func (cd *chunkDesc) setChunk(c chunk) {
	cd.Lock()
	defer cd.Unlock()

	if cd.chunk != nil {
		panic("chunk already set")
	}
	cd.chunk = c
}

// maybeEvict evicts the chunk if the refCount is 0. It returns whether the chunk
// is now evicted, which includes the case that the chunk was evicted even
// before this method was called.
func (cd *chunkDesc) maybeEvict() bool {
	cd.Lock()
	defer cd.Unlock()

	if cd.chunk == nil {
		return true
	}
	if cd.refCount != 0 {
		return false
	}
	cd.chunkFirstTime = cd.chunk.firstTime()
	cd.chunkLastTime = cd.chunk.lastTime()
	cd.chunk = nil
	chunkOps.WithLabelValues(evict).Inc()
	atomic.AddInt64(&numMemChunks, -1)
	return true
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
