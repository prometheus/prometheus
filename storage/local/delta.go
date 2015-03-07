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
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// The 21-byte header of a delta-encoded chunk looks like:
//
// - time delta bytes:  1 bytes
// - value delta bytes: 1 bytes
// - is integer:        1 byte
// - base time:         8 bytes
// - base value:        8 bytes
// - used buf bytes:    2 bytes
const (
	deltaHeaderBytes = 21

	deltaHeaderTimeBytesOffset  = 0
	deltaHeaderValueBytesOffset = 1
	deltaHeaderIsIntOffset      = 2
	deltaHeaderBaseTimeOffset   = 3
	deltaHeaderBaseValueOffset  = 11
	deltaHeaderBufLenOffset     = 19
)

// A deltaEncodedChunk adaptively stores sample timestamps and values with a
// delta encoding of various types (int, float) and bit widths. However, once 8
// bytes would be needed to encode a delta value, a fall-back to the absolute
// numbers happens (so that timestamps are saved directly as int64 and values as
// float64). It implements the chunk interface.
type deltaEncodedChunk []byte

// newDeltaEncodedChunk returns a newly allocated deltaEncodedChunk.
func newDeltaEncodedChunk(tb, vb deltaBytes, isInt bool, length int) *deltaEncodedChunk {
	if tb < 1 {
		panic("need at least 1 time delta byte")
	}
	if length < deltaHeaderBytes+16 {
		panic(fmt.Errorf(
			"chunk length %d bytes is insufficient, need at least %d",
			length, deltaHeaderBytes+16,
		))
	}
	c := make(deltaEncodedChunk, deltaHeaderIsIntOffset+1, length)

	c[deltaHeaderTimeBytesOffset] = byte(tb)
	c[deltaHeaderValueBytesOffset] = byte(vb)
	if vb < d8 && isInt { // Only use int for fewer than 8 value delta bytes.
		c[deltaHeaderIsIntOffset] = 1
	} else {
		c[deltaHeaderIsIntOffset] = 0
	}

	return &c
}

func (c deltaEncodedChunk) newFollowupChunk() chunk {
	return newDeltaEncodedChunk(d1, d0, true, cap(c))
}

// clone implements chunk.
func (c deltaEncodedChunk) clone() chunk {
	clone := make(deltaEncodedChunk, len(c), cap(c))
	copy(clone, c)
	return &clone
}

func (c deltaEncodedChunk) timeBytes() deltaBytes {
	return deltaBytes(c[deltaHeaderTimeBytesOffset])
}

func (c deltaEncodedChunk) valueBytes() deltaBytes {
	return deltaBytes(c[deltaHeaderValueBytesOffset])
}

func (c deltaEncodedChunk) isInt() bool {
	return c[deltaHeaderIsIntOffset] == 1
}

func (c deltaEncodedChunk) baseTime() clientmodel.Timestamp {
	return clientmodel.Timestamp(binary.LittleEndian.Uint64(c[deltaHeaderBaseTimeOffset:]))
}

func (c deltaEncodedChunk) baseValue() clientmodel.SampleValue {
	return clientmodel.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(c[deltaHeaderBaseValueOffset:])))
}

// add implements chunk.
func (c deltaEncodedChunk) add(s *metric.SamplePair) []chunk {
	if c.len() == 0 {
		c = c[:deltaHeaderBytes]
		binary.LittleEndian.PutUint64(c[deltaHeaderBaseTimeOffset:], uint64(s.Timestamp))
		binary.LittleEndian.PutUint64(c[deltaHeaderBaseValueOffset:], math.Float64bits(float64(s.Value)))
	}

	remainingBytes := cap(c) - len(c)
	sampleSize := c.sampleSize()

	// Do we generally have space for another sample in this chunk? If not,
	// overflow into a new one.
	if remainingBytes < sampleSize {
		overflowChunks := c.newFollowupChunk().add(s)
		return []chunk{&c, overflowChunks[0]}
	}

	baseValue := c.baseValue()
	// TODO(beorn7): Once https://github.com/prometheus/prometheus/issues/481 is
	// fixed, we should panic here if dt is negative.
	dt := s.Timestamp - c.baseTime()
	dv := s.Value - baseValue
	tb := c.timeBytes()
	vb := c.valueBytes()

	// If the new sample is incompatible with the current encoding, reencode the
	// existing chunk data into new chunk(s).
	//
	// int->float.
	if c.isInt() && !isInt64(dv) {
		return transcodeAndAdd(newDeltaEncodedChunk(tb, d4, false, cap(c)), &c, s)
	}
	// float32->float64.
	if !c.isInt() && vb == d4 && baseValue+clientmodel.SampleValue(float32(dv)) != s.Value {
		return transcodeAndAdd(newDeltaEncodedChunk(tb, d8, false, cap(c)), &c, s)
	}

	var ntb, nvb deltaBytes
	if tb < d8 {
		// Maybe more bytes for timestamp.
		ntb = bytesNeededForUnsignedTimestampDelta(dt)
	}
	if c.isInt() && vb < d8 {
		// Maybe more bytes for sample value.
		nvb = bytesNeededForIntegerSampleValueDelta(dv)
	}
	if ntb > tb || nvb > vb {
		ntb = max(ntb, tb)
		nvb = max(nvb, vb)
		return transcodeAndAdd(newDeltaEncodedChunk(ntb, nvb, c.isInt(), cap(c)), &c, s)
	}
	offset := len(c)
	c = c[:offset+sampleSize]

	switch tb {
	case d1:
		c[offset] = byte(dt)
	case d2:
		binary.LittleEndian.PutUint16(c[offset:], uint16(dt))
	case d4:
		binary.LittleEndian.PutUint32(c[offset:], uint32(dt))
	case d8:
		// Store the absolute value (no delta) in case of d8.
		binary.LittleEndian.PutUint64(c[offset:], uint64(s.Timestamp))
	default:
		panic("invalid number of bytes for time delta")
	}

	offset += int(tb)

	if c.isInt() {
		switch vb {
		case d0:
			// No-op. Constant value is stored as base value.
		case d1:
			c[offset] = byte(dv)
		case d2:
			binary.LittleEndian.PutUint16(c[offset:], uint16(dv))
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], uint32(dv))
		// d8 must not happen. Those samples are encoded as float64.
		default:
			panic("invalid number of bytes for integer delta")
		}
	} else {
		switch vb {
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], math.Float32bits(float32(dv)))
		case d8:
			// Store the absolute value (no delta) in case of d8.
			binary.LittleEndian.PutUint64(c[offset:], math.Float64bits(float64(s.Value)))
		default:
			panic("invalid number of bytes for floating point delta")
		}
	}
	return []chunk{&c}
}

func (c deltaEncodedChunk) sampleSize() int {
	return int(c.timeBytes() + c.valueBytes())
}

func (c deltaEncodedChunk) len() int {
	if len(c) < deltaHeaderBytes {
		return 0
	}
	return (len(c) - deltaHeaderBytes) / c.sampleSize()
}

// values implements chunk.
func (c deltaEncodedChunk) values() <-chan *metric.SamplePair {
	n := c.len()
	valuesChan := make(chan *metric.SamplePair)
	go func() {
		for i := 0; i < n; i++ {
			valuesChan <- c.valueAtIndex(i)
		}
		close(valuesChan)
	}()
	return valuesChan
}

func (c deltaEncodedChunk) valueAtIndex(idx int) *metric.SamplePair {
	offset := deltaHeaderBytes + idx*c.sampleSize()

	var ts clientmodel.Timestamp
	switch c.timeBytes() {
	case d1:
		ts = c.baseTime() + clientmodel.Timestamp(uint8(c[offset]))
	case d2:
		ts = c.baseTime() + clientmodel.Timestamp(binary.LittleEndian.Uint16(c[offset:]))
	case d4:
		ts = c.baseTime() + clientmodel.Timestamp(binary.LittleEndian.Uint32(c[offset:]))
	case d8:
		// Take absolute value for d8.
		ts = clientmodel.Timestamp(binary.LittleEndian.Uint64(c[offset:]))
	default:
		panic("Invalid number of bytes for time delta")
	}

	offset += int(c.timeBytes())

	var v clientmodel.SampleValue
	if c.isInt() {
		switch c.valueBytes() {
		case d0:
			v = c.baseValue()
		case d1:
			v = c.baseValue() + clientmodel.SampleValue(int8(c[offset]))
		case d2:
			v = c.baseValue() + clientmodel.SampleValue(int16(binary.LittleEndian.Uint16(c[offset:])))
		case d4:
			v = c.baseValue() + clientmodel.SampleValue(int32(binary.LittleEndian.Uint32(c[offset:])))
		// No d8 for ints.
		default:
			panic("Invalid number of bytes for integer delta")
		}
	} else {
		switch c.valueBytes() {
		case d4:
			v = c.baseValue() + clientmodel.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(c[offset:])))
		case d8:
			// Take absolute value for d8.
			v = clientmodel.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(c[offset:])))
		default:
			panic("Invalid number of bytes for floating point delta")
		}
	}
	return &metric.SamplePair{
		Timestamp: ts,
		Value:     v,
	}
}

// firstTime implements chunk.
func (c deltaEncodedChunk) firstTime() clientmodel.Timestamp {
	return c.valueAtIndex(0).Timestamp
}

// lastTime implements chunk.
func (c deltaEncodedChunk) lastTime() clientmodel.Timestamp {
	return c.valueAtIndex(c.len() - 1).Timestamp
}

// marshal implements chunk.
func (c deltaEncodedChunk) marshal(w io.Writer) error {
	if len(c) > math.MaxUint16 {
		panic("chunk buffer length would overflow a 16 bit uint.")
	}
	binary.LittleEndian.PutUint16(c[deltaHeaderBufLenOffset:], uint16(len(c)))

	n, err := w.Write(c[:cap(c)])
	if err != nil {
		return err
	}
	if n != cap(c) {
		return fmt.Errorf("wanted to write %d bytes, wrote %d", len(c), n)
	}
	return nil
}

// unmarshal implements chunk.
func (c *deltaEncodedChunk) unmarshal(r io.Reader) error {
	*c = (*c)[:cap(*c)]
	readBytes := 0
	for readBytes < len(*c) {
		n, err := r.Read((*c)[readBytes:])
		if err != nil {
			return err
		}
		readBytes += n
	}
	*c = (*c)[:binary.LittleEndian.Uint16((*c)[deltaHeaderBufLenOffset:])]
	return nil
}

// deltaEncodedChunkIterator implements chunkIterator.
type deltaEncodedChunkIterator struct {
	chunk *deltaEncodedChunk
	// TODO: add more fields here to keep track of last position.
}

// newIterator implements chunk.
func (c *deltaEncodedChunk) newIterator() chunkIterator {
	return &deltaEncodedChunkIterator{
		chunk: c,
	}
}

// getValueAtTime implements chunkIterator.
func (it *deltaEncodedChunkIterator) getValueAtTime(t clientmodel.Timestamp) metric.Values {
	i := sort.Search(it.chunk.len(), func(i int) bool {
		return !it.chunk.valueAtIndex(i).Timestamp.Before(t)
	})

	switch i {
	case 0:
		return metric.Values{*it.chunk.valueAtIndex(0)}
	case it.chunk.len():
		return metric.Values{*it.chunk.valueAtIndex(it.chunk.len() - 1)}
	default:
		v := it.chunk.valueAtIndex(i)
		if v.Timestamp.Equal(t) {
			return metric.Values{*v}
		}
		return metric.Values{*it.chunk.valueAtIndex(i - 1), *v}
	}
}

// getRangeValues implements chunkIterator.
func (it *deltaEncodedChunkIterator) getRangeValues(in metric.Interval) metric.Values {
	oldest := sort.Search(it.chunk.len(), func(i int) bool {
		return !it.chunk.valueAtIndex(i).Timestamp.Before(in.OldestInclusive)
	})

	newest := sort.Search(it.chunk.len(), func(i int) bool {
		return it.chunk.valueAtIndex(i).Timestamp.After(in.NewestInclusive)
	})

	if oldest == it.chunk.len() {
		return nil
	}

	result := make(metric.Values, 0, newest-oldest)
	for i := oldest; i < newest; i++ {
		result = append(result, *it.chunk.valueAtIndex(i))
	}
	return result
}

// contains implements chunkIterator.
func (it *deltaEncodedChunkIterator) contains(t clientmodel.Timestamp) bool {
	return !t.Before(it.chunk.firstTime()) && !t.After(it.chunk.lastTime())
}
