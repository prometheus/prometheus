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
	doubleDeltaHeaderBytes = 37

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

// add implements chunk.
func (c doubleDeltaEncodedChunk) add(s *model.SamplePair) []chunk {
	if c.len() == 0 {
		return c.addFirstSample(s)
	}

	tb := c.timeBytes()
	vb := c.valueBytes()

	if c.len() == 1 {
		return c.addSecondSample(s, tb, vb)
	}

	remainingBytes := cap(c) - len(c)
	sampleSize := c.sampleSize()

	// Do we generally have space for another sample in this chunk? If not,
	// overflow into a new one.
	if remainingBytes < sampleSize {
		overflowChunks := newChunk().add(s)
		return []chunk{&c, overflowChunks[0]}
	}

	projectedTime := c.baseTime() + model.Time(c.len())*c.baseTimeDelta()
	ddt := s.Timestamp - projectedTime

	projectedValue := c.baseValue() + model.SampleValue(c.len())*c.baseValueDelta()
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
		overflowChunks := newChunk().add(s)
		return []chunk{&c, overflowChunks[0]}
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
		panic("invalid number of bytes for time delta")
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
			panic("invalid number of bytes for integer delta")
		}
	} else {
		switch vb {
		case d4:
			binary.LittleEndian.PutUint32(c[offset:], math.Float32bits(float32(ddv)))
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
func (c doubleDeltaEncodedChunk) clone() chunk {
	clone := make(doubleDeltaEncodedChunk, len(c), cap(c))
	copy(clone, c)
	return &clone
}

// firstTime implements chunk.
func (c doubleDeltaEncodedChunk) firstTime() model.Time {
	return c.baseTime()
}

// newIterator implements chunk.
func (c *doubleDeltaEncodedChunk) newIterator() chunkIterator {
	return &doubleDeltaEncodedChunkIterator{
		c:      *c,
		len:    c.len(),
		baseT:  c.baseTime(),
		baseΔT: c.baseTimeDelta(),
		baseV:  c.baseValue(),
		baseΔV: c.baseValueDelta(),
		tBytes: c.timeBytes(),
		vBytes: c.valueBytes(),
		isInt:  c.isInt(),
	}
}

// marshal implements chunk.
func (c doubleDeltaEncodedChunk) marshal(w io.Writer) error {
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

// marshalToBuf implements chunk.
func (c doubleDeltaEncodedChunk) marshalToBuf(buf []byte) error {
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

// unmarshal implements chunk.
func (c *doubleDeltaEncodedChunk) unmarshal(r io.Reader) error {
	*c = (*c)[:cap(*c)]
	if _, err := io.ReadFull(r, *c); err != nil {
		return err
	}
	*c = (*c)[:binary.LittleEndian.Uint16((*c)[doubleDeltaHeaderBufLenOffset:])]
	return nil
}

// unmarshalFromBuf implements chunk.
func (c *doubleDeltaEncodedChunk) unmarshalFromBuf(buf []byte) {
	*c = (*c)[:cap(*c)]
	copy(*c, buf)
	*c = (*c)[:binary.LittleEndian.Uint16((*c)[doubleDeltaHeaderBufLenOffset:])]
}

// encoding implements chunk.
func (c doubleDeltaEncodedChunk) encoding() chunkEncoding { return doubleDelta }

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

func (c doubleDeltaEncodedChunk) len() int {
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
func (c doubleDeltaEncodedChunk) addFirstSample(s *model.SamplePair) []chunk {
	c = c[:doubleDeltaHeaderBaseValueOffset+8]
	binary.LittleEndian.PutUint64(
		c[doubleDeltaHeaderBaseTimeOffset:],
		uint64(s.Timestamp),
	)
	binary.LittleEndian.PutUint64(
		c[doubleDeltaHeaderBaseValueOffset:],
		math.Float64bits(float64(s.Value)),
	)
	return []chunk{&c}
}

// addSecondSample is a helper method only used by c.add(). It calculates the
// base delta from the provided sample and adds it to the chunk.
func (c doubleDeltaEncodedChunk) addSecondSample(s *model.SamplePair, tb, vb deltaBytes) []chunk {
	baseTimeDelta := s.Timestamp - c.baseTime()
	if baseTimeDelta < 0 {
		panic("base time delta is less than zero")
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
	return []chunk{&c}
}

// doubleDeltaEncodedChunkIterator implements chunkIterator.
type doubleDeltaEncodedChunkIterator struct {
	c              doubleDeltaEncodedChunk
	len            int
	baseT, baseΔT  model.Time
	baseV, baseΔV  model.SampleValue
	tBytes, vBytes deltaBytes
	isInt          bool
}

// length implements chunkIterator.
func (it *doubleDeltaEncodedChunkIterator) length() int { return it.len }

// valueAtTime implements chunkIterator.
func (it *doubleDeltaEncodedChunkIterator) valueAtTime(t model.Time) []model.SamplePair {
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
func (it *doubleDeltaEncodedChunkIterator) rangeValues(in metric.Interval) []model.SamplePair {
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
func (it *doubleDeltaEncodedChunkIterator) contains(t model.Time) bool {
	return !t.Before(it.baseT) && !t.After(it.timestampAtIndex(it.len-1))
}

// values implements chunkIterator.
func (it *doubleDeltaEncodedChunkIterator) values() <-chan *model.SamplePair {
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
func (it *doubleDeltaEncodedChunkIterator) timestampAtIndex(idx int) model.Time {
	if idx == 0 {
		return it.baseT
	}
	if idx == 1 {
		// If time bytes are at d8, the time is saved directly rather
		// than as a difference.
		if it.tBytes == d8 {
			return it.baseΔT
		}
		return it.baseT + it.baseΔT
	}

	offset := doubleDeltaHeaderBytes + (idx-2)*int(it.tBytes+it.vBytes)

	switch it.tBytes {
	case d1:
		return it.baseT +
			model.Time(idx)*it.baseΔT +
			model.Time(int8(it.c[offset]))
	case d2:
		return it.baseT +
			model.Time(idx)*it.baseΔT +
			model.Time(int16(binary.LittleEndian.Uint16(it.c[offset:])))
	case d4:
		return it.baseT +
			model.Time(idx)*it.baseΔT +
			model.Time(int32(binary.LittleEndian.Uint32(it.c[offset:])))
	case d8:
		// Take absolute value for d8.
		return model.Time(binary.LittleEndian.Uint64(it.c[offset:]))
	default:
		panic("invalid number of bytes for time delta")
	}
}

// lastTimestamp implements chunkIterator.
func (it *doubleDeltaEncodedChunkIterator) lastTimestamp() model.Time {
	return it.timestampAtIndex(it.len - 1)
}

// sampleValueAtIndex implements chunkIterator.
func (it *doubleDeltaEncodedChunkIterator) sampleValueAtIndex(idx int) model.SampleValue {
	if idx == 0 {
		return it.baseV
	}
	if idx == 1 {
		// If value bytes are at d8, the value is saved directly rather
		// than as a difference.
		if it.vBytes == d8 {
			return it.baseΔV
		}
		return it.baseV + it.baseΔV
	}

	offset := doubleDeltaHeaderBytes + (idx-2)*int(it.tBytes+it.vBytes) + int(it.tBytes)

	if it.isInt {
		switch it.vBytes {
		case d0:
			return it.baseV +
				model.SampleValue(idx)*it.baseΔV
		case d1:
			return it.baseV +
				model.SampleValue(idx)*it.baseΔV +
				model.SampleValue(int8(it.c[offset]))
		case d2:
			return it.baseV +
				model.SampleValue(idx)*it.baseΔV +
				model.SampleValue(int16(binary.LittleEndian.Uint16(it.c[offset:])))
		case d4:
			return it.baseV +
				model.SampleValue(idx)*it.baseΔV +
				model.SampleValue(int32(binary.LittleEndian.Uint32(it.c[offset:])))
		// No d8 for ints.
		default:
			panic("invalid number of bytes for integer delta")
		}
	} else {
		switch it.vBytes {
		case d4:
			return it.baseV +
				model.SampleValue(idx)*it.baseΔV +
				model.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(it.c[offset:])))
		case d8:
			// Take absolute value for d8.
			return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(it.c[offset:])))
		default:
			panic("invalid number of bytes for floating point delta")
		}
	}
}

// lastSampleValue implements chunkIterator.
func (it *doubleDeltaEncodedChunkIterator) lastSampleValue() model.SampleValue {
	return it.sampleValueAtIndex(it.len - 1)
}
