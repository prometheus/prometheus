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

package chunk

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/prometheus/common/model"
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

// Add implements chunk.
func (c deltaEncodedChunk) Add(s model.SamplePair) ([]Chunk, error) {
	// TODO(beorn7): Since we return &c, this method might cause an unnecessary allocation.
	if c.Len() == 0 {
		c = c[:deltaHeaderBytes]
		binary.LittleEndian.PutUint64(c[deltaHeaderBaseTimeOffset:], uint64(s.Timestamp))
		binary.LittleEndian.PutUint64(c[deltaHeaderBaseValueOffset:], math.Float64bits(float64(s.Value)))
	}

	remainingBytes := cap(c) - len(c)
	sampleSize := c.sampleSize()

	// Do we generally have space for another sample in this chunk? If not,
	// overflow into a new one.
	if remainingBytes < sampleSize {
		return addToOverflowChunk(&c, s)
	}

	baseValue := c.baseValue()
	dt := s.Timestamp - c.baseTime()
	if dt < 0 {
		return nil, fmt.Errorf("time delta is less than zero: %v", dt)
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
		return addToOverflowChunk(&c, s)
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
		return nil, fmt.Errorf("invalid number of bytes for time delta: %d", tb)
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
			return nil, fmt.Errorf("invalid number of bytes for integer delta: %d", vb)
		}
	} else {
		switch vb {
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], math.Float32bits(float32(dv)))
		case d8:
			// Store the absolute value (no delta) in case of d8.
			binary.LittleEndian.PutUint64(c[offset:], math.Float64bits(float64(s.Value)))
		default:
			return nil, fmt.Errorf("invalid number of bytes for floating point delta: %d", vb)
		}
	}
	return []Chunk{&c}, nil
}

// Clone implements chunk.
func (c deltaEncodedChunk) Clone() Chunk {
	clone := make(deltaEncodedChunk, len(c), cap(c))
	copy(clone, c)
	return &clone
}

// FirstTime implements chunk.
func (c deltaEncodedChunk) FirstTime() model.Time {
	return c.baseTime()
}

// NewIterator implements chunk.
func (c *deltaEncodedChunk) NewIterator() Iterator {
	return newIndexAccessingChunkIterator(c.Len(), &deltaEncodedIndexAccessor{
		c:      *c,
		baseT:  c.baseTime(),
		baseV:  c.baseValue(),
		tBytes: c.timeBytes(),
		vBytes: c.valueBytes(),
		isInt:  c.isInt(),
	})
}

// Marshal implements chunk.
func (c deltaEncodedChunk) Marshal(w io.Writer) error {
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

// MarshalToBuf implements chunk.
func (c deltaEncodedChunk) MarshalToBuf(buf []byte) error {
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

// Unmarshal implements chunk.
func (c *deltaEncodedChunk) Unmarshal(r io.Reader) error {
	*c = (*c)[:cap(*c)]
	if _, err := io.ReadFull(r, *c); err != nil {
		return err
	}
	return c.setLen()
}

// UnmarshalFromBuf implements chunk.
func (c *deltaEncodedChunk) UnmarshalFromBuf(buf []byte) error {
	*c = (*c)[:cap(*c)]
	copy(*c, buf)
	return c.setLen()
}

// setLen sets the length of the underlying slice and performs some sanity checks.
func (c *deltaEncodedChunk) setLen() error {
	l := binary.LittleEndian.Uint16((*c)[deltaHeaderBufLenOffset:])
	if int(l) > cap(*c) {
		return fmt.Errorf("delta chunk length exceeded during unmarshaling: %d", l)
	}
	if int(l) < deltaHeaderBytes {
		return fmt.Errorf("delta chunk length less than header size: %d < %d", l, deltaHeaderBytes)
	}
	switch c.timeBytes() {
	case d1, d2, d4, d8:
		// Pass.
	default:
		return fmt.Errorf("invalid number of time bytes in delta chunk: %d", c.timeBytes())
	}
	switch c.valueBytes() {
	case d0, d1, d2, d4, d8:
		// Pass.
	default:
		return fmt.Errorf("invalid number of value bytes in delta chunk: %d", c.valueBytes())
	}
	*c = (*c)[:l]
	return nil
}

// Encoding implements chunk.
func (c deltaEncodedChunk) Encoding() Encoding { return Delta }

// Utilization implements chunk.
func (c deltaEncodedChunk) Utilization() float64 {
	return float64(len(c)) / float64(cap(c))
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

func (c deltaEncodedChunk) baseTime() model.Time {
	return model.Time(binary.LittleEndian.Uint64(c[deltaHeaderBaseTimeOffset:]))
}

func (c deltaEncodedChunk) baseValue() model.SampleValue {
	return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(c[deltaHeaderBaseValueOffset:])))
}

func (c deltaEncodedChunk) sampleSize() int {
	return int(c.timeBytes() + c.valueBytes())
}

// Len implements Chunk. Runs in constant time.
func (c deltaEncodedChunk) Len() int {
	if len(c) < deltaHeaderBytes {
		return 0
	}
	return (len(c) - deltaHeaderBytes) / c.sampleSize()
}

// deltaEncodedIndexAccessor implements indexAccessor.
type deltaEncodedIndexAccessor struct {
	c              deltaEncodedChunk
	baseT          model.Time
	baseV          model.SampleValue
	tBytes, vBytes deltaBytes
	isInt          bool
	lastErr        error
}

func (acc *deltaEncodedIndexAccessor) err() error {
	return acc.lastErr
}

func (acc *deltaEncodedIndexAccessor) timestampAtIndex(idx int) model.Time {
	offset := deltaHeaderBytes + idx*int(acc.tBytes+acc.vBytes)

	switch acc.tBytes {
	case d1:
		return acc.baseT + model.Time(uint8(acc.c[offset]))
	case d2:
		return acc.baseT + model.Time(binary.LittleEndian.Uint16(acc.c[offset:]))
	case d4:
		return acc.baseT + model.Time(binary.LittleEndian.Uint32(acc.c[offset:]))
	case d8:
		// Take absolute value for d8.
		return model.Time(binary.LittleEndian.Uint64(acc.c[offset:]))
	default:
		acc.lastErr = fmt.Errorf("invalid number of bytes for time delta: %d", acc.tBytes)
		return model.Earliest
	}
}

func (acc *deltaEncodedIndexAccessor) sampleValueAtIndex(idx int) model.SampleValue {
	offset := deltaHeaderBytes + idx*int(acc.tBytes+acc.vBytes) + int(acc.tBytes)

	if acc.isInt {
		switch acc.vBytes {
		case d0:
			return acc.baseV
		case d1:
			return acc.baseV + model.SampleValue(int8(acc.c[offset]))
		case d2:
			return acc.baseV + model.SampleValue(int16(binary.LittleEndian.Uint16(acc.c[offset:])))
		case d4:
			return acc.baseV + model.SampleValue(int32(binary.LittleEndian.Uint32(acc.c[offset:])))
		// No d8 for ints.
		default:
			acc.lastErr = fmt.Errorf("invalid number of bytes for integer delta: %d", acc.vBytes)
			return 0
		}
	} else {
		switch acc.vBytes {
		case d4:
			return acc.baseV + model.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(acc.c[offset:])))
		case d8:
			// Take absolute value for d8.
			return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(acc.c[offset:])))
		default:
			acc.lastErr = fmt.Errorf("invalid number of bytes for floating point delta: %d", acc.vBytes)
			return 0
		}
	}
}
