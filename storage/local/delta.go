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

	"github.com/prometheus/common/model"

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

// add implements chunk.
func (c deltaEncodedChunk) add(s *model.SamplePair) []chunk {
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
		overflowChunks := newChunk().add(s)
		return []chunk{&c, overflowChunks[0]}
	}

	baseValue := c.baseValue()
	dt := s.Timestamp - c.baseTime()
	if dt < 0 {
		panic("time delta is less than zero")
	}

	dv := s.Value - baseValue
	tb := c.timeBytes()
	vb := c.valueBytes()
	isInt := c.isInt()

	// If the new sample is incompatible with the current encoding, reencode the
	// existing chunk data into new chunk(s).

	ntb, nvb, nInt := tb, vb, isInt
	if isInt && !isInt64(dv) {
		// int->float.
		nvb = d4
		nInt = false
	} else if !isInt && vb == d4 && baseValue+model.SampleValue(float32(dv)) != s.Value {
		// float32->float64.
		nvb = d8
	} else {
		if tb < d8 {
			// Maybe more bytes for timestamp.
			ntb = max(tb, bytesNeededForUnsignedTimestampDelta(dt))
		}
		if c.isInt() && vb < d8 {
			// Maybe more bytes for sample value.
			nvb = max(vb, bytesNeededForIntegerSampleValueDelta(dv))
		}
	}
	if tb != ntb || vb != nvb || isInt != nInt {
		if len(c)*2 < cap(c) {
			return transcodeAndAdd(newDeltaEncodedChunk(ntb, nvb, nInt, cap(c)), &c, s)
		}
		// Chunk is already half full. Better create a new one and save the transcoding efforts.
		overflowChunks := newChunk().add(s)
		return []chunk{&c, overflowChunks[0]}
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
			c[offset] = byte(int8(dv))
		case d2:
			binary.LittleEndian.PutUint16(c[offset:], uint16(int16(dv)))
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], uint32(int32(dv)))
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

// clone implements chunk.
func (c deltaEncodedChunk) clone() chunk {
	clone := make(deltaEncodedChunk, len(c), cap(c))
	copy(clone, c)
	return &clone
}

// firstTime implements chunk.
func (c deltaEncodedChunk) firstTime() model.Time {
	return c.baseTime()
}

// newIterator implements chunk.
func (c *deltaEncodedChunk) newIterator() chunkIterator {
	return &deltaEncodedChunkIterator{
		c:      *c,
		len:    c.len(),
		baseT:  c.baseTime(),
		baseV:  c.baseValue(),
		tBytes: c.timeBytes(),
		vBytes: c.valueBytes(),
		isInt:  c.isInt(),
	}
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
		return fmt.Errorf("wanted to write %d bytes, wrote %d", cap(c), n)
	}
	return nil
}

// marshalToBuf implements chunk.
func (c deltaEncodedChunk) marshalToBuf(buf []byte) error {
	if len(c) > math.MaxUint16 {
		panic("chunk buffer length would overflow a 16 bit uint")
	}
	binary.LittleEndian.PutUint16(c[deltaHeaderBufLenOffset:], uint16(len(c)))

	n := copy(buf, c)
	if n != len(c) {
		return fmt.Errorf("wanted to copy %d bytes to buffer, copied %d", len(c), n)
	}
	return nil
}

// unmarshal implements chunk.
func (c *deltaEncodedChunk) unmarshal(r io.Reader) error {
	*c = (*c)[:cap(*c)]
	if _, err := io.ReadFull(r, *c); err != nil {
		return err
	}
	*c = (*c)[:binary.LittleEndian.Uint16((*c)[deltaHeaderBufLenOffset:])]
	return nil
}

// unmarshalFromBuf implements chunk.
func (c *deltaEncodedChunk) unmarshalFromBuf(buf []byte) {
	*c = (*c)[:cap(*c)]
	copy(*c, buf)
	*c = (*c)[:binary.LittleEndian.Uint16((*c)[deltaHeaderBufLenOffset:])]
}

// encoding implements chunk.
func (c deltaEncodedChunk) encoding() chunkEncoding { return delta }

func (c deltaEncodedChunk) timeBytes() deltaBytes {
	return deltaBytes(c[deltaHeaderTimeBytesOffset])
}

func (c deltaEncodedChunk) valueBytes() deltaBytes {
	return deltaBytes(c[deltaHeaderValueBytesOffset])
}

func (c deltaEncodedChunk) isInt() bool {
	return c[deltaHeaderIsIntOffset] == 1
}

func (c deltaEncodedChunk) baseTime() model.Time {
	return model.Time(binary.LittleEndian.Uint64(c[deltaHeaderBaseTimeOffset:]))
}

func (c deltaEncodedChunk) baseValue() model.SampleValue {
	return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(c[deltaHeaderBaseValueOffset:])))
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

// deltaEncodedChunkIterator implements chunkIterator.
type deltaEncodedChunkIterator struct {
	c              deltaEncodedChunk
	len            int
	baseT          model.Time
	baseV          model.SampleValue
	tBytes, vBytes deltaBytes
	isInt          bool
}

// length implements chunkIterator.
func (it *deltaEncodedChunkIterator) length() int { return it.len }

// valueAtTime implements chunkIterator.
func (it *deltaEncodedChunkIterator) valueAtTime(t model.Time) []model.SamplePair {
	i := sort.Search(it.len, func(i int) bool {
		return !it.timestampAtIndex(i).Before(t)
	})

	switch i {
	case 0:
		return []model.SamplePair{{
			Timestamp: it.timestampAtIndex(0),
			Value:     it.sampleValueAtIndex(0),
		}}
	case it.len:
		return []model.SamplePair{{
			Timestamp: it.timestampAtIndex(it.len - 1),
			Value:     it.sampleValueAtIndex(it.len - 1),
		}}
	default:
		ts := it.timestampAtIndex(i)
		if ts.Equal(t) {
			return []model.SamplePair{{
				Timestamp: ts,
				Value:     it.sampleValueAtIndex(i),
			}}
		}
		return []model.SamplePair{
			{
				Timestamp: it.timestampAtIndex(i - 1),
				Value:     it.sampleValueAtIndex(i - 1),
			},
			{
				Timestamp: ts,
				Value:     it.sampleValueAtIndex(i),
			},
		}
	}
}

// rangeValues implements chunkIterator.
func (it *deltaEncodedChunkIterator) rangeValues(in metric.Interval) []model.SamplePair {
	oldest := sort.Search(it.len, func(i int) bool {
		return !it.timestampAtIndex(i).Before(in.OldestInclusive)
	})

	newest := sort.Search(it.len, func(i int) bool {
		return it.timestampAtIndex(i).After(in.NewestInclusive)
	})

	if oldest == it.len {
		return nil
	}

	result := make([]model.SamplePair, 0, newest-oldest)
	for i := oldest; i < newest; i++ {
		result = append(result, model.SamplePair{
			Timestamp: it.timestampAtIndex(i),
			Value:     it.sampleValueAtIndex(i),
		})
	}
	return result
}

// contains implements chunkIterator.
func (it *deltaEncodedChunkIterator) contains(t model.Time) bool {
	return !t.Before(it.baseT) && !t.After(it.timestampAtIndex(it.len-1))
}

// values implements chunkIterator.
func (it *deltaEncodedChunkIterator) values() <-chan *model.SamplePair {
	valuesChan := make(chan *model.SamplePair)
	go func() {
		for i := 0; i < it.len; i++ {
			valuesChan <- &model.SamplePair{
				Timestamp: it.timestampAtIndex(i),
				Value:     it.sampleValueAtIndex(i),
			}
		}
		close(valuesChan)
	}()
	return valuesChan
}

// timestampAtIndex implements chunkIterator.
func (it *deltaEncodedChunkIterator) timestampAtIndex(idx int) model.Time {
	offset := deltaHeaderBytes + idx*int(it.tBytes+it.vBytes)

	switch it.tBytes {
	case d1:
		return it.baseT + model.Time(uint8(it.c[offset]))
	case d2:
		return it.baseT + model.Time(binary.LittleEndian.Uint16(it.c[offset:]))
	case d4:
		return it.baseT + model.Time(binary.LittleEndian.Uint32(it.c[offset:]))
	case d8:
		// Take absolute value for d8.
		return model.Time(binary.LittleEndian.Uint64(it.c[offset:]))
	default:
		panic("invalid number of bytes for time delta")
	}
}

// lastTimestamp implements chunkIterator.
func (it *deltaEncodedChunkIterator) lastTimestamp() model.Time {
	return it.timestampAtIndex(it.len - 1)
}

// sampleValueAtIndex implements chunkIterator.
func (it *deltaEncodedChunkIterator) sampleValueAtIndex(idx int) model.SampleValue {
	offset := deltaHeaderBytes + idx*int(it.tBytes+it.vBytes) + int(it.tBytes)

	if it.isInt {
		switch it.vBytes {
		case d0:
			return it.baseV
		case d1:
			return it.baseV + model.SampleValue(int8(it.c[offset]))
		case d2:
			return it.baseV + model.SampleValue(int16(binary.LittleEndian.Uint16(it.c[offset:])))
		case d4:
			return it.baseV + model.SampleValue(int32(binary.LittleEndian.Uint32(it.c[offset:])))
		// No d8 for ints.
		default:
			panic("invalid number of bytes for integer delta")
		}
	} else {
		switch it.vBytes {
		case d4:
			return it.baseV + model.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(it.c[offset:])))
		case d8:
			// Take absolute value for d8.
			return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(it.c[offset:])))
		default:
			panic("invalid number of bytes for floating point delta")
		}
	}
}

// lastSampleValue implements chunkIterator.
func (it *deltaEncodedChunkIterator) lastSampleValue() model.SampleValue {
	return it.sampleValueAtIndex(it.len - 1)
}
