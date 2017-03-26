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

// The 37-byte header of a delta-encoded chunk looks like:
//
// - used buf bytes:           2 bytes
// - time double-delta bytes:  1 bytes
// - value double-delta bytes: 1 bytes
// - is integer:               1 byte
// - base time:                8 bytes
// - base value:               8 bytes
// - base time delta:          8 bytes
// - base value delta:         8 bytes
const (
	doubleDeltaHeaderBytes    = 37
	doubleDeltaHeaderMinBytes = 21 // header isn't full for chunk w/ one sample

	doubleDeltaHeaderBufLenOffset         = 0
	doubleDeltaHeaderTimeBytesOffset      = 2
	doubleDeltaHeaderValueBytesOffset     = 3
	doubleDeltaHeaderIsIntOffset          = 4
	doubleDeltaHeaderBaseTimeOffset       = 5
	doubleDeltaHeaderBaseValueOffset      = 13
	doubleDeltaHeaderBaseTimeDeltaOffset  = 21
	doubleDeltaHeaderBaseValueDeltaOffset = 29
)

// A doubleDeltaEncodedChunk adaptively stores sample timestamps and values with
// a double-delta encoding of various types (int, float) and bit widths. A base
// value and timestamp and a base delta for each is saved in the header. The
// payload consists of double-deltas, i.e. deviations from the values and
// timestamps calculated by applying the base value and time and the base deltas.
// However, once 8 bytes would be needed to encode a double-delta value, a
// fall-back to the absolute numbers happens (so that timestamps are saved
// directly as int64 and values as float64).
// doubleDeltaEncodedChunk implements the chunk interface.
type doubleDeltaEncodedChunk []byte

// newDoubleDeltaEncodedChunk returns a newly allocated doubleDeltaEncodedChunk.
func newDoubleDeltaEncodedChunk(tb, vb deltaBytes, isInt bool, length int) *doubleDeltaEncodedChunk {
	if tb < 1 {
		panic("need at least 1 time delta byte")
	}
	if length < doubleDeltaHeaderBytes+16 {
		panic(fmt.Errorf(
			"chunk length %d bytes is insufficient, need at least %d",
			length, doubleDeltaHeaderBytes+16,
		))
	}
	c := make(doubleDeltaEncodedChunk, doubleDeltaHeaderIsIntOffset+1, length)

	c[doubleDeltaHeaderTimeBytesOffset] = byte(tb)
	c[doubleDeltaHeaderValueBytesOffset] = byte(vb)
	if vb < d8 && isInt { // Only use int for fewer than 8 value double-delta bytes.
		c[doubleDeltaHeaderIsIntOffset] = 1
	} else {
		c[doubleDeltaHeaderIsIntOffset] = 0
	}
	return &c
}

// Add implements chunk.
func (c doubleDeltaEncodedChunk) Add(s model.SamplePair) ([]Chunk, error) {
	// TODO(beorn7): Since we return &c, this method might cause an unnecessary allocation.
	if c.Len() == 0 {
		return c.addFirstSample(s), nil
	}

	tb := c.timeBytes()
	vb := c.valueBytes()

	if c.Len() == 1 {
		return c.addSecondSample(s, tb, vb)
	}

	remainingBytes := cap(c) - len(c)
	sampleSize := c.sampleSize()

	// Do we generally have space for another sample in this chunk? If not,
	// overflow into a new one.
	if remainingBytes < sampleSize {
		return addToOverflowChunk(&c, s)
	}

	projectedTime := c.baseTime() + model.Time(c.Len())*c.baseTimeDelta()
	ddt := s.Timestamp - projectedTime

	projectedValue := c.baseValue() + model.SampleValue(c.Len())*c.baseValueDelta()
	ddv := s.Value - projectedValue

	ntb, nvb, nInt := tb, vb, c.isInt()
	// If the new sample is incompatible with the current encoding, reencode the
	// existing chunk data into new chunk(s).
	if c.isInt() && !isInt64(ddv) {
		// int->float.
		nvb = d4
		nInt = false
	} else if !c.isInt() && vb == d4 && projectedValue+model.SampleValue(float32(ddv)) != s.Value {
		// float32->float64.
		nvb = d8
	} else {
		if tb < d8 {
			// Maybe more bytes for timestamp.
			ntb = max(tb, bytesNeededForSignedTimestampDelta(ddt))
		}
		if c.isInt() && vb < d8 {
			// Maybe more bytes for sample value.
			nvb = max(vb, bytesNeededForIntegerSampleValueDelta(ddv))
		}
	}
	if tb != ntb || vb != nvb || c.isInt() != nInt {
		if len(c)*2 < cap(c) {
			return transcodeAndAdd(newDoubleDeltaEncodedChunk(ntb, nvb, nInt, cap(c)), &c, s)
		}
		// Chunk is already half full. Better create a new one and save the transcoding efforts.
		return addToOverflowChunk(&c, s)
	}

	offset := len(c)
	c = c[:offset+sampleSize]

	switch tb {
	case d1:
		c[offset] = byte(ddt)
	case d2:
		binary.LittleEndian.PutUint16(c[offset:], uint16(ddt))
	case d4:
		binary.LittleEndian.PutUint32(c[offset:], uint32(ddt))
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
			// No-op. Constant delta is stored as base value.
		case d1:
			c[offset] = byte(int8(ddv))
		case d2:
			binary.LittleEndian.PutUint16(c[offset:], uint16(int16(ddv)))
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], uint32(int32(ddv)))
		// d8 must not happen. Those samples are encoded as float64.
		default:
			return nil, fmt.Errorf("invalid number of bytes for integer delta: %d", vb)
		}
	} else {
		switch vb {
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], math.Float32bits(float32(ddv)))
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
func (c doubleDeltaEncodedChunk) Clone() Chunk {
	clone := make(doubleDeltaEncodedChunk, len(c), cap(c))
	copy(clone, c)
	return &clone
}

// FirstTime implements chunk.
func (c doubleDeltaEncodedChunk) FirstTime() model.Time {
	return c.baseTime()
}

// NewIterator( implements chunk.
func (c *doubleDeltaEncodedChunk) NewIterator() Iterator {
	return newIndexAccessingChunkIterator(c.Len(), &doubleDeltaEncodedIndexAccessor{
		c:      *c,
		baseT:  c.baseTime(),
		baseΔT: c.baseTimeDelta(),
		baseV:  c.baseValue(),
		baseΔV: c.baseValueDelta(),
		tBytes: c.timeBytes(),
		vBytes: c.valueBytes(),
		isInt:  c.isInt(),
	})
}

// Marshal implements chunk.
func (c doubleDeltaEncodedChunk) Marshal(w io.Writer) error {
	if len(c) > math.MaxUint16 {
		panic("chunk buffer length would overflow a 16 bit uint")
	}
	binary.LittleEndian.PutUint16(c[doubleDeltaHeaderBufLenOffset:], uint16(len(c)))

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
func (c doubleDeltaEncodedChunk) MarshalToBuf(buf []byte) error {
	if len(c) > math.MaxUint16 {
		panic("chunk buffer length would overflow a 16 bit uint")
	}
	binary.LittleEndian.PutUint16(c[doubleDeltaHeaderBufLenOffset:], uint16(len(c)))

	n := copy(buf, c)
	if n != len(c) {
		return fmt.Errorf("wanted to copy %d bytes to buffer, copied %d", len(c), n)
	}
	return nil
}

// Unmarshal implements chunk.
func (c *doubleDeltaEncodedChunk) Unmarshal(r io.Reader) error {
	*c = (*c)[:cap(*c)]
	if _, err := io.ReadFull(r, *c); err != nil {
		return err
	}
	return c.setLen()
}

// UnmarshalFromBuf implements chunk.
func (c *doubleDeltaEncodedChunk) UnmarshalFromBuf(buf []byte) error {
	*c = (*c)[:cap(*c)]
	copy(*c, buf)
	return c.setLen()
}

// setLen sets the length of the underlying slice and performs some sanity checks.
func (c *doubleDeltaEncodedChunk) setLen() error {
	l := binary.LittleEndian.Uint16((*c)[doubleDeltaHeaderBufLenOffset:])
	if int(l) > cap(*c) {
		return fmt.Errorf("doubledelta chunk length exceeded during unmarshaling: %d", l)
	}
	if int(l) < doubleDeltaHeaderMinBytes {
		return fmt.Errorf("doubledelta chunk length less than header size: %d < %d", l, doubleDeltaHeaderMinBytes)
	}
	switch c.timeBytes() {
	case d1, d2, d4, d8:
		// Pass.
	default:
		return fmt.Errorf("invalid number of time bytes in doubledelta chunk: %d", c.timeBytes())
	}
	switch c.valueBytes() {
	case d0, d1, d2, d4, d8:
		// Pass.
	default:
		return fmt.Errorf("invalid number of value bytes in doubledelta chunk: %d", c.valueBytes())
	}
	*c = (*c)[:l]
	return nil
}

// Encoding implements chunk.
func (c doubleDeltaEncodedChunk) Encoding() Encoding { return DoubleDelta }

// Utilization implements chunk.
func (c doubleDeltaEncodedChunk) Utilization() float64 {
	return float64(len(c)-doubleDeltaHeaderIsIntOffset-1) / float64(cap(c))
}

func (c doubleDeltaEncodedChunk) baseTime() model.Time {
	return model.Time(
		binary.LittleEndian.Uint64(
			c[doubleDeltaHeaderBaseTimeOffset:],
		),
	)
}

func (c doubleDeltaEncodedChunk) baseValue() model.SampleValue {
	return model.SampleValue(
		math.Float64frombits(
			binary.LittleEndian.Uint64(
				c[doubleDeltaHeaderBaseValueOffset:],
			),
		),
	)
}

func (c doubleDeltaEncodedChunk) baseTimeDelta() model.Time {
	if len(c) < doubleDeltaHeaderBaseTimeDeltaOffset+8 {
		return 0
	}
	return model.Time(
		binary.LittleEndian.Uint64(
			c[doubleDeltaHeaderBaseTimeDeltaOffset:],
		),
	)
}

func (c doubleDeltaEncodedChunk) baseValueDelta() model.SampleValue {
	if len(c) < doubleDeltaHeaderBaseValueDeltaOffset+8 {
		return 0
	}
	return model.SampleValue(
		math.Float64frombits(
			binary.LittleEndian.Uint64(
				c[doubleDeltaHeaderBaseValueDeltaOffset:],
			),
		),
	)
}

func (c doubleDeltaEncodedChunk) timeBytes() deltaBytes {
	return deltaBytes(c[doubleDeltaHeaderTimeBytesOffset])
}

func (c doubleDeltaEncodedChunk) valueBytes() deltaBytes {
	return deltaBytes(c[doubleDeltaHeaderValueBytesOffset])
}

func (c doubleDeltaEncodedChunk) sampleSize() int {
	return int(c.timeBytes() + c.valueBytes())
}

// Len implements Chunk. Runs in constant time.
func (c doubleDeltaEncodedChunk) Len() int {
	if len(c) <= doubleDeltaHeaderIsIntOffset+1 {
		return 0
	}
	if len(c) <= doubleDeltaHeaderBaseValueOffset+8 {
		return 1
	}
	return (len(c)-doubleDeltaHeaderBytes)/c.sampleSize() + 2
}

func (c doubleDeltaEncodedChunk) isInt() bool {
	return c[doubleDeltaHeaderIsIntOffset] == 1
}

// addFirstSample is a helper method only used by c.add(). It adds timestamp and
// value as base time and value.
func (c doubleDeltaEncodedChunk) addFirstSample(s model.SamplePair) []Chunk {
	c = c[:doubleDeltaHeaderBaseValueOffset+8]
	binary.LittleEndian.PutUint64(
		c[doubleDeltaHeaderBaseTimeOffset:],
		uint64(s.Timestamp),
	)
	binary.LittleEndian.PutUint64(
		c[doubleDeltaHeaderBaseValueOffset:],
		math.Float64bits(float64(s.Value)),
	)
	return []Chunk{&c}
}

// addSecondSample is a helper method only used by c.add(). It calculates the
// base delta from the provided sample and adds it to the chunk.
func (c doubleDeltaEncodedChunk) addSecondSample(s model.SamplePair, tb, vb deltaBytes) ([]Chunk, error) {
	baseTimeDelta := s.Timestamp - c.baseTime()
	if baseTimeDelta < 0 {
		return nil, fmt.Errorf("base time delta is less than zero: %v", baseTimeDelta)
	}
	c = c[:doubleDeltaHeaderBytes]
	if tb >= d8 || bytesNeededForUnsignedTimestampDelta(baseTimeDelta) >= d8 {
		// If already the base delta needs d8 (or we are at d8
		// already, anyway), we better encode this timestamp
		// directly rather than as a delta and switch everything
		// to d8.
		c[doubleDeltaHeaderTimeBytesOffset] = byte(d8)
		binary.LittleEndian.PutUint64(
			c[doubleDeltaHeaderBaseTimeDeltaOffset:],
			uint64(s.Timestamp),
		)
	} else {
		binary.LittleEndian.PutUint64(
			c[doubleDeltaHeaderBaseTimeDeltaOffset:],
			uint64(baseTimeDelta),
		)
	}
	baseValue := c.baseValue()
	baseValueDelta := s.Value - baseValue
	if vb >= d8 || baseValue+baseValueDelta != s.Value {
		// If we can't reproduce the original sample value (or
		// if we are at d8 already, anyway), we better encode
		// this value directly rather than as a delta and switch
		// everything to d8.
		c[doubleDeltaHeaderValueBytesOffset] = byte(d8)
		c[doubleDeltaHeaderIsIntOffset] = 0
		binary.LittleEndian.PutUint64(
			c[doubleDeltaHeaderBaseValueDeltaOffset:],
			math.Float64bits(float64(s.Value)),
		)
	} else {
		binary.LittleEndian.PutUint64(
			c[doubleDeltaHeaderBaseValueDeltaOffset:],
			math.Float64bits(float64(baseValueDelta)),
		)
	}
	return []Chunk{&c}, nil
}

// doubleDeltaEncodedIndexAccessor implements indexAccessor.
type doubleDeltaEncodedIndexAccessor struct {
	c              doubleDeltaEncodedChunk
	baseT, baseΔT  model.Time
	baseV, baseΔV  model.SampleValue
	tBytes, vBytes deltaBytes
	isInt          bool
	lastErr        error
}

func (acc *doubleDeltaEncodedIndexAccessor) err() error {
	return acc.lastErr
}

func (acc *doubleDeltaEncodedIndexAccessor) timestampAtIndex(idx int) model.Time {
	if idx == 0 {
		return acc.baseT
	}
	if idx == 1 {
		// If time bytes are at d8, the time is saved directly rather
		// than as a difference.
		if acc.tBytes == d8 {
			return acc.baseΔT
		}
		return acc.baseT + acc.baseΔT
	}

	offset := doubleDeltaHeaderBytes + (idx-2)*int(acc.tBytes+acc.vBytes)

	switch acc.tBytes {
	case d1:
		return acc.baseT +
			model.Time(idx)*acc.baseΔT +
			model.Time(int8(acc.c[offset]))
	case d2:
		return acc.baseT +
			model.Time(idx)*acc.baseΔT +
			model.Time(int16(binary.LittleEndian.Uint16(acc.c[offset:])))
	case d4:
		return acc.baseT +
			model.Time(idx)*acc.baseΔT +
			model.Time(int32(binary.LittleEndian.Uint32(acc.c[offset:])))
	case d8:
		// Take absolute value for d8.
		return model.Time(binary.LittleEndian.Uint64(acc.c[offset:]))
	default:
		acc.lastErr = fmt.Errorf("invalid number of bytes for time delta: %d", acc.tBytes)
		return model.Earliest
	}
}

func (acc *doubleDeltaEncodedIndexAccessor) sampleValueAtIndex(idx int) model.SampleValue {
	if idx == 0 {
		return acc.baseV
	}
	if idx == 1 {
		// If value bytes are at d8, the value is saved directly rather
		// than as a difference.
		if acc.vBytes == d8 {
			return acc.baseΔV
		}
		return acc.baseV + acc.baseΔV
	}

	offset := doubleDeltaHeaderBytes + (idx-2)*int(acc.tBytes+acc.vBytes) + int(acc.tBytes)

	if acc.isInt {
		switch acc.vBytes {
		case d0:
			return acc.baseV +
				model.SampleValue(idx)*acc.baseΔV
		case d1:
			return acc.baseV +
				model.SampleValue(idx)*acc.baseΔV +
				model.SampleValue(int8(acc.c[offset]))
		case d2:
			return acc.baseV +
				model.SampleValue(idx)*acc.baseΔV +
				model.SampleValue(int16(binary.LittleEndian.Uint16(acc.c[offset:])))
		case d4:
			return acc.baseV +
				model.SampleValue(idx)*acc.baseΔV +
				model.SampleValue(int32(binary.LittleEndian.Uint32(acc.c[offset:])))
		// No d8 for ints.
		default:
			acc.lastErr = fmt.Errorf("invalid number of bytes for integer delta: %d", acc.vBytes)
			return 0
		}
	} else {
		switch acc.vBytes {
		case d4:
			return acc.baseV +
				model.SampleValue(idx)*acc.baseΔV +
				model.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(acc.c[offset:])))
		case d8:
			// Take absolute value for d8.
			return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(acc.c[offset:])))
		default:
			acc.lastErr = fmt.Errorf("invalid number of bytes for floating point delta: %d", acc.vBytes)
			return 0
		}
	}
}
