package storage_ng

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

type deltaBytes int

const (
	d0 deltaBytes = 0
	d1            = 1
	d2            = 2
	d4            = 4
	d8            = 8
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
// delta encoding of various types (int, float) and bit width. However, once 8
// bytes would be needed to encode a delta value, a fall-back to the absolute
// numbers happens (so that timestamps are saved directly as int64 and values as
// float64).
type deltaEncodedChunk struct {
	buf []byte
}

func newDeltaEncodedChunk(tb, vb deltaBytes, isInt bool) *deltaEncodedChunk {
	buf := chunkBufs.Get()
	buf = buf[:deltaHeaderIsIntOffset+1]

	buf[deltaHeaderTimeBytesOffset] = byte(tb)
	buf[deltaHeaderValueBytesOffset] = byte(vb)
	if vb < d8 && isInt { // Only use int for fewer than 8 value delta bytes.
		buf[deltaHeaderIsIntOffset] = 1
	} else {
		buf[deltaHeaderIsIntOffset] = 0
	}

	return &deltaEncodedChunk{
		buf: buf,
	}
}

func (c *deltaEncodedChunk) newFollowupChunk() chunk {
	return newDeltaEncodedChunk(d1, d1, true)
	//return newDeltaEncodedChunk(c.timeBytes(), c.valueBytes(), c.isInt())
}

func (c *deltaEncodedChunk) clone() chunk {
	buf := chunkBufs.Get()
	buf = buf[:len(c.buf)]
	copy(buf, c.buf)
	return &deltaEncodedChunk{
		buf: buf,
	}
}

func neededDeltaBytes(deltaT clientmodel.Timestamp, deltaV clientmodel.SampleValue, isInt bool) (dtb, dvb deltaBytes) {
	dtb = d1
	if deltaT >= 256 {
		dtb = d2
	}
	if deltaT >= 256*256 {
		dtb = d4
	}
	if deltaT >= 256*256*256*256 {
		dtb = d8
	}

	if isInt {
		dvb = d0
		if deltaV != 0 {
			dvb = d1
		}
		if deltaV < -(256/2) || deltaV > (256/2)-1 {
			dvb = d2
		}
		if deltaV < -(256*256/2) || deltaV > (256*256/2)-1 {
			dvb = d4
		}
		if deltaV < -(256*256*256*256/2) || deltaV > (256*256*256*256/2)-1 {
			dvb = d8
		}
	} else {
		dvb = d4
		if clientmodel.SampleValue(float32(deltaV)) != deltaV {
			dvb = d8
		}
	}
	return dtb, dvb
}

func max(a, b deltaBytes) deltaBytes {
	if a > b {
		return a
	}
	return b
}

func (c *deltaEncodedChunk) timeBytes() deltaBytes {
	return deltaBytes(c.buf[deltaHeaderTimeBytesOffset])
}

func (c *deltaEncodedChunk) valueBytes() deltaBytes {
	return deltaBytes(c.buf[deltaHeaderValueBytesOffset])
}

func (c *deltaEncodedChunk) isInt() bool {
	return c.buf[deltaHeaderIsIntOffset] == 1
}

func (c *deltaEncodedChunk) baseTime() clientmodel.Timestamp {
	return clientmodel.Timestamp(binary.LittleEndian.Uint64(c.buf[deltaHeaderBaseTimeOffset:]))
}

func (c *deltaEncodedChunk) baseValue() clientmodel.SampleValue {
	return clientmodel.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(c.buf[deltaHeaderBaseValueOffset:])))
}

func (c *deltaEncodedChunk) add(s *metric.SamplePair) chunks {
	if len(c.buf) < deltaHeaderBytes {
		c.buf = c.buf[:deltaHeaderBytes]
		binary.LittleEndian.PutUint64(c.buf[deltaHeaderBaseTimeOffset:], uint64(s.Timestamp))
		binary.LittleEndian.PutUint64(c.buf[deltaHeaderBaseValueOffset:], math.Float64bits(float64(s.Value)))
	}

	remainingBytes := cap(c.buf) - len(c.buf)
	sampleSize := c.sampleSize()

	// Do we generally have space for another sample in this chunk? If not,
	// overflow into a new one.
	if remainingBytes < sampleSize {
		//fmt.Println("overflow")
		overflowChunks := c.newFollowupChunk().add(s)
		return chunks{c, overflowChunks[0]}
	}

	dt := s.Timestamp - c.baseTime()
	dv := s.Value - c.baseValue()
	tb := c.timeBytes()
	vb := c.valueBytes()

	// If the new sample is incompatible with the current encoding, reencode the
	// existing chunk data into new chunk(s).
	//
	// int->float.
	// TODO: compare speed with Math.Modf.
	if c.isInt() && clientmodel.SampleValue(int64(dv)) != dv {
		//fmt.Println("int->float", len(c.buf), cap(c.buf), dv)
		return transcodeAndAdd(newDeltaEncodedChunk(tb, d4, false), c, s)
	}
	// float32->float64.
	if !c.isInt() && vb == d4 && clientmodel.SampleValue(float32(dv)) != dv {
		//fmt.Println("float32->float64", float32(dv), dv, len(c.buf), cap(c.buf))
		return transcodeAndAdd(newDeltaEncodedChunk(tb, d8, false), c, s)
	}
	if tb < d8 || vb < d8 {
		// Maybe more bytes per sample.
		if ntb, nvb := neededDeltaBytes(dt, dv, c.isInt()); ntb > tb || nvb > vb {
			//fmt.Printf("transcoding T: %v->%v, V: %v->%v, I: %v; len %v, cap %v\n", tb, ntb, vb, nvb, c.isInt(), len(c.buf), cap(c.buf))
			ntb = max(ntb, tb)
			nvb = max(nvb, vb)
			return transcodeAndAdd(newDeltaEncodedChunk(ntb, nvb, c.isInt()), c, s)
		}
	}
	offset := len(c.buf)
	c.buf = c.buf[:offset+sampleSize]

	switch tb {
	case d1:
		c.buf[offset] = byte(dt)
	case d2:
		binary.LittleEndian.PutUint16(c.buf[offset:], uint16(dt))
	case d4:
		binary.LittleEndian.PutUint32(c.buf[offset:], uint32(dt))
	case d8:
		// Store the absolute value (no delta) in case of d8.
		binary.LittleEndian.PutUint64(c.buf[offset:], uint64(s.Timestamp))
	default:
		panic("invalid number of bytes for time delta")
	}

	offset += int(tb)

	if c.isInt() {
		switch vb {
		case d0:
			// No-op. Constant value is stored as base value.
		case d1:
			c.buf[offset] = byte(dv)
		case d2:
			binary.LittleEndian.PutUint16(c.buf[offset:], uint16(dv))
		case d4:
			binary.LittleEndian.PutUint32(c.buf[offset:], uint32(dv))
		// d8 must not happen. Those samples are encoded as float64.
		default:
			panic("invalid number of bytes for integer delta")
		}
	} else {
		switch vb {
		case d4:
			binary.LittleEndian.PutUint32(c.buf[offset:], math.Float32bits(float32(dv)))
		case d8:
			// Store the absolute value (no delta) in case of d8.
			binary.LittleEndian.PutUint64(c.buf[offset:], math.Float64bits(float64(s.Value)))
		default:
			panic("invalid number of bytes for floating point delta")
		}
	}
	return chunks{c}
}

func (c *deltaEncodedChunk) close() {
	//fmt.Println("returning chunk")
	chunkBufs.Give(c.buf)
}

func (c *deltaEncodedChunk) sampleSize() int {
	return int(c.timeBytes() + c.valueBytes())
}

func (c *deltaEncodedChunk) len() int {
	if len(c.buf) < deltaHeaderBytes {
		return 0
	}
	return (len(c.buf) - deltaHeaderBytes) / c.sampleSize()
}

// TODO: remove?
func (c *deltaEncodedChunk) values() <-chan *metric.SamplePair {
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

func (c *deltaEncodedChunk) valueAtIndex(idx int) *metric.SamplePair {
	offset := deltaHeaderBytes + idx*c.sampleSize()

	var ts clientmodel.Timestamp
	switch c.timeBytes() {
	case d1:
		ts = c.baseTime() + clientmodel.Timestamp(uint8(c.buf[offset]))
	case d2:
		ts = c.baseTime() + clientmodel.Timestamp(binary.LittleEndian.Uint16(c.buf[offset:]))
	case d4:
		ts = c.baseTime() + clientmodel.Timestamp(binary.LittleEndian.Uint32(c.buf[offset:]))
	case d8:
		// Take absolute value for d8.
		ts = clientmodel.Timestamp(binary.LittleEndian.Uint64(c.buf[offset:]))
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
			v = c.baseValue() + clientmodel.SampleValue(int8(c.buf[offset]))
		case d2:
			v = c.baseValue() + clientmodel.SampleValue(int16(binary.LittleEndian.Uint16(c.buf[offset:])))
		case d4:
			v = c.baseValue() + clientmodel.SampleValue(int32(binary.LittleEndian.Uint32(c.buf[offset:])))
		// No d8 for ints.
		default:
			panic("Invalid number of bytes for integer delta")
		}
	} else {
		switch c.valueBytes() {
		case d4:
			v = c.baseValue() + clientmodel.SampleValue(math.Float32frombits(binary.LittleEndian.Uint32(c.buf[offset:])))
		case d8:
			// Take absolute value for d8.
			v = clientmodel.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(c.buf[offset:])))
		default:
			panic("Invalid number of bytes for floating point delta")
		}
	}
	return &metric.SamplePair{
		Timestamp: ts,
		Value:     v,
	}
}

func (c *deltaEncodedChunk) firstTime() clientmodel.Timestamp {
	return c.valueAtIndex(0).Timestamp
}

func (c *deltaEncodedChunk) lastTime() clientmodel.Timestamp {
	return c.valueAtIndex(c.len() - 1).Timestamp
}

func (c *deltaEncodedChunk) marshal(w io.Writer) error {
	// TODO: check somewhere that configured buf len cannot overflow 16 bit.
	binary.LittleEndian.PutUint16(c.buf[deltaHeaderBufLenOffset:], uint16(len(c.buf)))

	n, err := w.Write(c.buf[:cap(c.buf)])
	if err != nil {
		return err
	}
	if n != cap(c.buf) {
		return fmt.Errorf("wanted to write %d bytes, wrote %d", len(c.buf), n)
	}
	return nil
}

func (c *deltaEncodedChunk) unmarshal(r io.Reader) error {
	c.buf = c.buf[:cap(c.buf)]
	readBytes := 0
	for readBytes < len(c.buf) {
		n, err := r.Read(c.buf[readBytes:])
		if err != nil {
			return err
		}
		readBytes += n
	}
	c.buf = c.buf[:binary.LittleEndian.Uint16(c.buf[deltaHeaderBufLenOffset:])]
	return nil
}

type deltaEncodedChunkIterator struct {
	chunk *deltaEncodedChunk
	// TODO: add more fields here to keep track of last position.
}

func (c *deltaEncodedChunk) newIterator() chunkIterator {
	return &deltaEncodedChunkIterator{
		chunk: c,
	}
}

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

func (it *deltaEncodedChunkIterator) getBoundaryValues(in metric.Interval) metric.Values {
	return nil
}

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

func (it *deltaEncodedChunkIterator) contains(t clientmodel.Timestamp) bool {
	return !t.Before(it.chunk.firstTime()) && !t.After(it.chunk.lastTime())
}
