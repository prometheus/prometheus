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
func (c deltaEncodedChunk) add(s model.SamplePair) ([]chunk, error) {
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
		overflowChunks, err := newChunk().add(s)
		if err != nil {
			return nil, err
		}
		return []chunk{&c, overflowChunks[0]}, nil
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
		overflowChunks, err := newChunk().add(s)
		if err != nil {
			return nil, err
		}
		return []chunk{&c, overflowChunks[0]}, nil
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
	return []chunk{&c}, nil
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
	l := binary.LittleEndian.Uint16((*c)[deltaHeaderBufLenOffset:])
	if int(l) > cap(*c) {
		return fmt.Errorf("chunk length exceeded during unmarshaling: %d", l)
	}
	*c = (*c)[:l]
	return nil
}

// unmarshalFromBuf implements chunk.
func (c *deltaEncodedChunk) unmarshalFromBuf(buf []byte) error {
	*c = (*c)[:cap(*c)]
	copy(*c, buf)
	l := binary.LittleEndian.Uint16((*c)[deltaHeaderBufLenOffset:])
	if int(l) > cap(*c) {
		return fmt.Errorf("chunk length exceeded during unmarshaling: %d", l)
	}
	*c = (*c)[:l]
	return nil
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

// valueAtOrBeforeTime implements chunkIterator.
func (it *deltaEncodedChunkIterator) valueAtOrBeforeTime(t model.Time) (model.SamplePair, error) {
	var lastErr error
	i := sort.Search(it.len, func(i int) bool {
		ts, err := it.timestampAtIndex(i)
		if err != nil {
			lastErr = err
		}
		return ts.After(t)
	})
	if i == 0 || lastErr != nil {
		return ZeroSamplePair, lastErr
	}
	ts, err := it.timestampAtIndex(i - 1)
	if err != nil {
		return ZeroSamplePair, err
	}
	v, err := it.sampleValueAtIndex(i - 1)
	if err != nil {
		return ZeroSamplePair, err
	}
	return model.SamplePair{Timestamp: ts, Value: v}, nil
}

// rangeValues implements chunkIterator.
func (it *deltaEncodedChunkIterator) rangeValues(in metric.Interval) ([]model.SamplePair, error) {
	var lastErr error

	oldest := sort.Search(it.len, func(i int) bool {
		t, err := it.timestampAtIndex(i)
		if err != nil {
			lastErr = err
		}
		return !t.Before(in.OldestInclusive)
	})

	newest := sort.Search(it.len, func(i int) bool {
		t, err := it.timestampAtIndex(i)
		if err != nil {
			lastErr = err
		}
		return t.After(in.NewestInclusive)
	})

	if oldest == it.len || lastErr != nil {
		return nil, lastErr
	}

	result := make([]model.SamplePair, 0, newest-oldest)
	for i := oldest; i < newest; i++ {
		t, err := it.timestampAtIndex(i)
		if err != nil {
			return nil, err
		}
		v, err := it.sampleValueAtIndex(i)
		if err != nil {
			return nil, err
		}
		result = append(result, model.SamplePair{Timestamp: t, Value: v})
	}
	return result, nil
}

// contains implements chunkIterator.
func (it *deltaEncodedChunkIterator) contains(t model.Time) (bool, error) {
	lastT, err := it.timestampAtIndex(it.len - 1)
	if err != nil {
		return false, err
	}
	return !t.Before(it.baseT) && !t.After(lastT), nil
}

// values implements chunkIterator.
func (it *deltaEncodedChunkIterator) values() <-chan struct {
	model.SamplePair
	error
} {
	valuesChan := make(chan struct {
		model.SamplePair
		error
	})
	go func() {
		for i := 0; i < it.len; i++ {
			t, err := it.timestampAtIndex(i)
			if err != nil {
				valuesChan <- struct {
					model.SamplePair
					error
				}{ZeroSamplePair, err}
				break
			}
			v, err := it.sampleValueAtIndex(i)
			if err != nil {
				valuesChan <- struct {
					model.SamplePair
					error
				}{ZeroSamplePair, err}
				break
			}
			valuesChan <- struct {
				model.SamplePair
				error
			}{model.SamplePair{Timestamp: t, Value: v}, nil}
		}
		close(valuesChan)
	}()
	return valuesChan
}

// timestampAtIndex implements chunkIterator.
func (it *deltaEncodedChunkIterator) timestampAtIndex(idx int) (model.Time, error) {
	offset := deltaHeaderBytes + idx*int(it.tBytes+it.vBytes)

	switch it.tBytes {
	case d1:
		return it.baseT + model.Time(uint8(it.c[offset])), nil
	case d2:
		return it.baseT + model.Time(binary.LittleEndian.Uint16(it.c[offset:])), nil
	case d4:
		return it.baseT + model.Time(binary.LittleEndian.Uint32(it.c[offset:])), nil
	case d8:
		// Take absolute value for d8.
		return model.Time(binary.LittleEndian.Uint64(it.c[offset:])), nil
	default:
		return 0, fmt.Errorf("invalid number of bytes for time delta: %d", it.tBytes)
	}
}

// lastTimestamp implements chunkIterator.
func (it *deltaEncodedChunkIterator) lastTimestamp() (model.Time, error) {
	return it.timestampAtIndex(it.len - 1)
}

// sampleValueAtIndex implements chunkIterator.
func (it *deltaEncodedChunkIterator) sampleValueAtIndex(idx int) (model.SampleValue, error) {
	offset := deltaHeaderBytes + idx*int(it.tBytes+it.vBytes) + int(it.tBytes)

	if it.isInt {
		switch it.vBytes {
		case d0:
			return it.baseV, nil
		case d1:
			return it.baseV + model.SampleValue(int8(it.c[offset])), nil
		case d2:
			return it.baseV + model.SampleValue(int16(binary.LittleEndian.Uint16(it.c[offset:]))), nil
		case d4:
			return it.baseV + model.SampleValue(int32(binary.LittleEndian.Uint32(it.c[offset:]))), nil
		// No d8 for ints.
		default:
			return 0, fmt.Errorf("invalid number of bytes for integer delta: %d", it.vBytes)
		}
	} else {
		switch it.vBytes {
		case d4:
			return it.baseV + model.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(it.c[offset:]))), nil
		case d8:
			// Take absolute value for d8.
			return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(it.c[offset:]))), nil
		default:
			return 0, fmt.Errorf("invalid number of bytes for floating point delta: %d", it.vBytes)
		}
	}
}

// lastSampleValue implements chunkIterator.
func (it *deltaEncodedChunkIterator) lastSampleValue() (model.SampleValue, error) {
	return it.sampleValueAtIndex(it.len - 1)
}
