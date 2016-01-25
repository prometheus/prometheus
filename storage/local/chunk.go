// Copyright 2014 The Prometheus Authors
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
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// The DefaultChunkEncoding can be changed via a flag.
var DefaultChunkEncoding = doubleDelta

type chunkEncoding byte

// String implements flag.Value.
func (ce chunkEncoding) String() string {
	return fmt.Sprintf("%d", ce)
}

// Set implements flag.Value.
func (ce *chunkEncoding) Set(s string) error {
	switch s {
	case "0":
		*ce = delta
	case "1":
		*ce = doubleDelta
	default:
		return fmt.Errorf("invalid chunk encoding: %s", s)
	}
	return nil
}

const (
	delta chunkEncoding = iota
	doubleDelta
)

// chunkDesc contains meta-data for a chunk. Many of its methods are
// goroutine-safe proxies for chunk methods.
type chunkDesc struct {
	sync.Mutex
	c              chunk // nil if chunk is evicted.
	rCnt           int
	chunkFirstTime model.Time // Used if chunk is evicted.
	chunkLastTime  model.Time // Used if chunk is evicted.

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
	numMemChunkDescs.Inc()
	return &chunkDesc{c: c, rCnt: 1}
}

func (cd *chunkDesc) add(s *model.SamplePair) []chunk {
	cd.Lock()
	defer cd.Unlock()

	return cd.c.add(s)
}

// pin increments the refCount by one. Upon increment from 0 to 1, this
// chunkDesc is removed from the evict list. To enable the latter, the
// evictRequests channel has to be provided.
func (cd *chunkDesc) pin(evictRequests chan<- evictRequest) {
	cd.Lock()
	defer cd.Unlock()

	if cd.rCnt == 0 {
		// Remove ourselves from the evict list.
		evictRequests <- evictRequest{cd, false}
	}
	cd.rCnt++
}

// unpin decrements the refCount by one. Upon decrement from 1 to 0, this
// chunkDesc is added to the evict list. To enable the latter, the evictRequests
// channel has to be provided.
func (cd *chunkDesc) unpin(evictRequests chan<- evictRequest) {
	cd.Lock()
	defer cd.Unlock()

	if cd.rCnt == 0 {
		panic("cannot unpin already unpinned chunk")
	}
	cd.rCnt--
	if cd.rCnt == 0 {
		// Add ourselves to the back of the evict list.
		evictRequests <- evictRequest{cd, true}
	}
}

func (cd *chunkDesc) refCount() int {
	cd.Lock()
	defer cd.Unlock()

	return cd.rCnt
}

func (cd *chunkDesc) firstTime() model.Time {
	cd.Lock()
	defer cd.Unlock()

	if cd.c == nil {
		return cd.chunkFirstTime
	}
	return cd.c.firstTime()
}

func (cd *chunkDesc) lastTime() model.Time {
	cd.Lock()
	defer cd.Unlock()

	if cd.c == nil {
		return cd.chunkLastTime
	}
	return cd.c.newIterator().lastTimestamp()
}

func (cd *chunkDesc) lastSamplePair() *model.SamplePair {
	cd.Lock()
	defer cd.Unlock()

	if cd.c == nil {
		return nil
	}
	it := cd.c.newIterator()
	return &model.SamplePair{
		Timestamp: it.lastTimestamp(),
		Value:     it.lastSampleValue(),
	}
}

func (cd *chunkDesc) isEvicted() bool {
	cd.Lock()
	defer cd.Unlock()

	return cd.c == nil
}

func (cd *chunkDesc) contains(t model.Time) bool {
	return !t.Before(cd.firstTime()) && !t.After(cd.lastTime())
}

func (cd *chunkDesc) chunk() chunk {
	cd.Lock()
	defer cd.Unlock()

	return cd.c
}

func (cd *chunkDesc) setChunk(c chunk) {
	cd.Lock()
	defer cd.Unlock()

	if cd.c != nil {
		panic("chunk already set")
	}
	cd.c = c
}

// maybeEvict evicts the chunk if the refCount is 0. It returns whether the chunk
// is now evicted, which includes the case that the chunk was evicted even
// before this method was called.
func (cd *chunkDesc) maybeEvict() bool {
	cd.Lock()
	defer cd.Unlock()

	if cd.c == nil {
		return true
	}
	if cd.rCnt != 0 {
		return false
	}
	cd.chunkFirstTime = cd.c.firstTime()
	cd.chunkLastTime = cd.c.newIterator().lastTimestamp()
	cd.c = nil
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
	add(sample *model.SamplePair) []chunk
	clone() chunk
	firstTime() model.Time
	newIterator() chunkIterator
	marshal(io.Writer) error
	marshalToBuf([]byte) error
	unmarshal(io.Reader) error
	unmarshalFromBuf([]byte)
	encoding() chunkEncoding
}

// A chunkIterator enables efficient access to the content of a chunk. It is
// generally not safe to use a chunkIterator concurrently with or after chunk
// mutation.
type chunkIterator interface {
	// length returns the number of samples in the chunk.
	length() int
	// Gets the timestamp of the n-th sample in the chunk.
	timestampAtIndex(int) model.Time
	// Gets the last timestamp in the chunk.
	lastTimestamp() model.Time
	// Gets the sample value of the n-th sample in the chunk.
	sampleValueAtIndex(int) model.SampleValue
	// Gets the last sample value in the chunk.
	lastSampleValue() model.SampleValue
	// Gets the two values that are immediately adjacent to a given time. In
	// case a value exist at precisely the given time, only that single
	// value is returned. Only the first or last value is returned (as a
	// single value), if the given time is before or after the first or last
	// value, respectively.
	valueAtTime(model.Time) []model.SamplePair
	// Gets all values contained within a given interval.
	rangeValues(metric.Interval) []model.SamplePair
	// Whether a given timestamp is contained between first and last value
	// in the chunk.
	contains(model.Time) bool
	// values returns a channel, from which all sample values in the chunk
	// can be received in order. The channel is closed after the last
	// one. It is generally not safe to mutate the chunk while the channel
	// is still open.
	values() <-chan *model.SamplePair
}

func transcodeAndAdd(dst chunk, src chunk, s *model.SamplePair) []chunk {
	chunkOps.WithLabelValues(transcode).Inc()

	head := dst
	body := []chunk{}
	for v := range src.newIterator().values() {
		newChunks := head.add(v)
		body = append(body, newChunks[:len(newChunks)-1]...)
		head = newChunks[len(newChunks)-1]
	}
	newChunks := head.add(s)
	return append(body, newChunks...)
}

// newChunk creates a new chunk according to the encoding set by the
// defaultChunkEncoding flag.
func newChunk() chunk {
	return newChunkForEncoding(DefaultChunkEncoding)
}

func newChunkForEncoding(encoding chunkEncoding) chunk {
	switch encoding {
	case delta:
		return newDeltaEncodedChunk(d1, d0, true, chunkLen)
	case doubleDelta:
		return newDoubleDeltaEncodedChunk(d1, d0, true, chunkLen)
	default:
		panic(fmt.Errorf("unknown chunk encoding: %v", encoding))
	}
}
