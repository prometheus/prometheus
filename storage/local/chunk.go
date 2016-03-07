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

// chunkDesc contains meta-data for a chunk. Pay special attention to the
// documented requirements for calling its methods concurrently (WRT pinning and
// locking). The doc comments spell out the requirements for each method, but
// here is an overview and general explanation:
//
// Everything that changes the pinning of the underlying chunk or deals with its
// eviction is protected by a mutex. This affects the following methods: pin,
// unpin, refCount, isEvicted, maybeEvict. These methods can be called at any
// time without further prerequisites.
//
// Another group of methods acts on (or sets) the underlying chunk. These
// methods involve no locking. They may only be called if the caller has pinned
// the chunk (to guarantee the chunk is not evicted concurrently). Also, the
// caller must make sure nobody else will call these methods concurrently,
// either by holding the sole reference to the chunkDesc (usually during loading
// or creation) or by locking the fingerprint of the series the chunkDesc
// belongs to. The affected methods are: add, maybePopulateLastTime, setChunk.
//
// Finally, there are the special cases firstTime and lastTime. lastTime requires
// to have locked the fingerprint of the series but the chunk does not need to
// be pinned. That's because the chunkLastTime field in chunkDesc gets populated
// upon completion of the chunk (when it is still pinned, and which happens
// while the series's fingerprint is locked). Once that has happened, calling
// lastTime does not require the chunk to be loaded anymore. Before that has
// happened, the chunk is pinned anyway. The chunkFirstTime field in chunkDesc
// is populated upon creation of a chunkDesc, so it is alway safe to call
// firstTime. The firstTime method is arguably not needed and only there for
// consistency with lastTime.
type chunkDesc struct {
	sync.Mutex           // Protects pinning.
	c              chunk // nil if chunk is evicted.
	rCnt           int
	chunkFirstTime model.Time // Populated at creation. Immutable.
	chunkLastTime  model.Time // Populated on closing of the chunk, model.Earliest if unset.

	// evictListElement is nil if the chunk is not in the evict list.
	// evictListElement is _not_ protected by the chunkDesc mutex.
	// It must only be touched by the evict list handler in memorySeriesStorage.
	evictListElement *list.Element
}

// newChunkDesc creates a new chunkDesc pointing to the provided chunk. The
// provided chunk is assumed to be not persisted yet. Therefore, the refCount of
// the new chunkDesc is 1 (preventing eviction prior to persisting).
func newChunkDesc(c chunk, firstTime model.Time) *chunkDesc {
	chunkOps.WithLabelValues(createAndPin).Inc()
	atomic.AddInt64(&numMemChunks, 1)
	numMemChunkDescs.Inc()
	return &chunkDesc{
		c:              c,
		rCnt:           1,
		chunkFirstTime: firstTime,
		chunkLastTime:  model.Earliest,
	}
}

// add adds a sample pair to the underlying chunk. For safe concurrent access,
// The chunk must be pinned, and the caller must have locked the fingerprint of
// the series.
func (cd *chunkDesc) add(s model.SamplePair) ([]chunk, error) {
	return cd.c.add(s)
}

// pin increments the refCount by one. Upon increment from 0 to 1, this
// chunkDesc is removed from the evict list. To enable the latter, the
// evictRequests channel has to be provided. This method can be called
// concurrently at any time.
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
// channel has to be provided. This method can be called concurrently at any
// time.
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

// refCount returns the number of pins. This method can be called concurrently
// at any time.
func (cd *chunkDesc) refCount() int {
	cd.Lock()
	defer cd.Unlock()

	return cd.rCnt
}

// firstTime returns the timestamp of the first sample in the chunk. This method
// can be called concurrently at any time. It only returns the immutable
// cd.chunkFirstTime without any locking. Arguably, this method is
// useless. However, it provides consistency with the lastTime method.
func (cd *chunkDesc) firstTime() model.Time {
	return cd.chunkFirstTime
}

// lastTime returns the timestamp of the last sample in the chunk. For safe
// concurrent access, this method requires the fingerprint of the time series to
// be locked.
func (cd *chunkDesc) lastTime() (model.Time, error) {
	if cd.chunkLastTime != model.Earliest || cd.c == nil {
		return cd.chunkLastTime, nil
	}
	return cd.c.newIterator().lastTimestamp()
}

// maybePopulateLastTime populates the chunkLastTime from the underlying chunk
// if it has not yet happened. Call this method directly after having added the
// last sample to a chunk or after closing a head chunk due to age. For safe
// concurrent access, the chunk must be pinned, and the caller must have locked
// the fingerprint of the series.
func (cd *chunkDesc) maybePopulateLastTime() error {
	if cd.chunkLastTime == model.Earliest && cd.c != nil {
		t, err := cd.c.newIterator().lastTimestamp()
		if err != nil {
			return err
		}
		cd.chunkLastTime = t
	}
	return nil
}

// isEvicted returns whether the chunk is evicted. For safe concurrent access,
// the caller must have locked the fingerprint of the series.
func (cd *chunkDesc) isEvicted() bool {
	// Locking required here because we do not want the caller to force
	// pinning the chunk first, so it could be evicted while this method is
	// called.
	cd.Lock()
	defer cd.Unlock()

	return cd.c == nil
}

// setChunk sets the underlying chunk. The caller must have locked the
// fingerprint of the series and must have "pre-pinned" the chunk (i.e. first
// call pin and then set the chunk).
func (cd *chunkDesc) setChunk(c chunk) {
	if cd.c != nil {
		panic("chunk already set")
	}
	cd.c = c
}

// maybeEvict evicts the chunk if the refCount is 0. It returns whether the chunk
// is now evicted, which includes the case that the chunk was evicted even
// before this method was called. It can be called concurrently at any time.
func (cd *chunkDesc) maybeEvict() bool {
	cd.Lock()
	defer cd.Unlock()

	if cd.c == nil {
		return true
	}
	if cd.rCnt != 0 {
		return false
	}
	if cd.chunkLastTime == model.Earliest {
		// This must never happen.
		panic("chunkLastTime not populated for evicted chunk")
	}
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
	// the relevant one and discard the original chunk.
	add(sample model.SamplePair) ([]chunk, error)
	clone() chunk
	firstTime() model.Time
	newIterator() chunkIterator
	marshal(io.Writer) error
	marshalToBuf([]byte) error
	unmarshal(io.Reader) error
	unmarshalFromBuf([]byte) error
	encoding() chunkEncoding
}

// A chunkIterator enables efficient access to the content of a chunk. It is
// generally not safe to use a chunkIterator concurrently with or after chunk
// mutation. The error returned by any of the methods is always the last error
// encountered by the iterator, i.e. once an error has been encountered, no
// method will ever return a nil error again. In general, errors signal data
// corruption in the chunk and require quarantining.
type chunkIterator interface {
	// Gets the last timestamp in the chunk.
	lastTimestamp() (model.Time, error)
	// Gets the value that is closest before the given time. In case a value
	// exists at precisely the given time, that value is returned. If no
	// applicable value exists, ZeroSamplePair is returned.
	valueAtOrBeforeTime(model.Time) (model.SamplePair, error)
	// Gets all values contained within a given interval.
	rangeValues(metric.Interval) ([]model.SamplePair, error)
	// Whether a given timestamp is contained between first and last value
	// in the chunk.
	contains(model.Time) (bool, error)
	// scan, value, and err implement a bufio.Scanner-like interface.  The
	// scan method advances the iterator to the next value in the chunk and
	// returns true if that worked. In that case, the value method will
	// return the sample pair the iterator has advanced to. If the scan
	// method returns false, either an error has occured or the end of the
	// chunk has been reached. In the former case, the err method will
	// return the error. In the latter case, the err method will return nil.
	// Upon creation, the iterator is at position "minus one". After the
	// first scan call, value will return the 1st value in the
	// chunk. valueAtOrBeforeTime and rangeValues all modify the iterator
	// position, too. They behave as if their return values were retrieved
	// after a scan call, i.e. calling the value or err methods after a call
	// to those methods will retrieve the same return value again (or the
	// last value in the range in case of rangeValues), and subsequent scan
	// calls will advance the iterator from there.
	scan() bool
	value() model.SamplePair
	err() error
}

func transcodeAndAdd(dst chunk, src chunk, s model.SamplePair) ([]chunk, error) {
	chunkOps.WithLabelValues(transcode).Inc()

	var (
		head            = dst
		body, newChunks []chunk
		err             error
	)

	it := src.newIterator()
	for it.scan() {
		if newChunks, err = head.add(it.value()); err != nil {
			return nil, err
		}
		body = append(body, newChunks[:len(newChunks)-1]...)
		head = newChunks[len(newChunks)-1]
	}
	if it.err() != nil {
		return nil, it.err()
	}

	if newChunks, err = head.add(s); err != nil {
		return nil, err
	}
	return append(body, newChunks...), nil
}

// newChunk creates a new chunk according to the encoding set by the
// defaultChunkEncoding flag.
func newChunk() chunk {
	chunk, err := newChunkForEncoding(DefaultChunkEncoding)
	if err != nil {
		panic(err)
	}
	return chunk
}

func newChunkForEncoding(encoding chunkEncoding) (chunk, error) {
	switch encoding {
	case delta:
		return newDeltaEncodedChunk(d1, d0, true, chunkLen), nil
	case doubleDelta:
		return newDoubleDeltaEncodedChunk(d1, d0, true, chunkLen), nil
	default:
		return nil, fmt.Errorf("unknown chunk encoding: %v", encoding)
	}
}
