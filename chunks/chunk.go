package chunks

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"

	"github.com/prometheus/common/model"
)

// Encoding is the identifier for a chunk encoding
type Encoding uint8

func (e Encoding) String() string {
	switch e {
	case EncNone:
		return "none"
	case EncPlain:
		return "plain"
	case EncXOR:
		return "XOR"
	case EncDoubleDelta:
		return "doubleDelta"
	}
	return "<unknown>"
}

// The different available chunk encodings.
const (
	EncNone Encoding = iota
	EncPlain
	EncXOR
	EncDoubleDelta
)

var (
	// ErrChunkFull is returned if the remaining size of a chunk cannot
	// fit the appended data.
	ErrChunkFull = errors.New("chunk full")
)

// Chunk holds a sequence of sample pairs that can be iterated over and appended to.
type Chunk interface {
	Data() []byte
	Appender() Appender
	Iterator() Iterator
}

// FromData returns a chunk from a byte slice of chunk data.
func FromData(d []byte) (Chunk, error) {
	if len(d) == 0 {
		return nil, fmt.Errorf("no data")
	}
	e := Encoding(d[0])

	switch e {
	case EncPlain:
		rc := rawChunk{d: d, l: uint64(len(d))}
		return &PlainChunk{rawChunk: rc}, nil
	}
	return nil, fmt.Errorf("unknown chunk encoding: %d", e)
}

// Iterator provides iterating access over sample pairs in a chunk.
type Iterator interface {
	// Seek moves the iterator to the element at or after the given time
	// and returns the sample pair at the position.
	Seek(model.Time) (model.SamplePair, bool)
	// Next returns the next sample pair in the iterator.
	Next() (model.SamplePair, bool)

	// SeekBefore(model.Time) (model.SamplePair, bool)
	First() (model.SamplePair, bool)
	// Last() (model.SamplePair, bool)

	// Err returns a non-nil error if Next or Seek returned false.
	// Their behavior on subsequent calls after one of them returned false
	// is undefined.
	Err() error
}

// Appender adds sample pairs to a chunk.
type Appender interface {
	Append(model.Time, model.SampleValue) error
}

// rawChunk provides a basic byte slice and is used by higher-level
// Chunk implementations. It can be safely appended to without causing
// any further allocations.
type rawChunk struct {
	d []byte
	l uint64
}

func newRawChunk(sz int, enc Encoding) rawChunk {
	c := rawChunk{d: make([]byte, sz), l: 1}
	c.d[0] = byte(enc)
	return c
}

func (c *rawChunk) encoding() Encoding {
	return Encoding(c.d[0])
}

func (c *rawChunk) Data() []byte {
	return c.d[:c.l]
}

func (c *rawChunk) append(b []byte) error {
	if len(b) > len(c.d)-int(c.l) {
		return ErrChunkFull
	}
	copy(c.d[c.l:], b)
	// Atomically increment the length so we can safely retrieve iterators
	// for a chunk that is being appended to.
	// This does not make it safe for concurrent appends!
	atomic.AddUint64(&c.l, uint64(len(b)))
	return nil
}

type bitChunk struct {
	d []byte

	sz    int
	pos   uint32 // bytes used in the chunk
	count uint32 // valid bits in last byte

	// Read copies of above values used when retrieving iterators.
	rl     uint32
	rcount uint32
}

type bit bool

const (
	zero bit = false
	one  bit = true
)

func newBitChunk(sz int, enc Encoding) bitChunk {
	c := bitChunk{d: make([]byte, sz+1), pos: 1, count: 8}
	c.d[0] = byte(enc)
	return c
}

func (c *bitChunk) encoding() Encoding {
	return Encoding(c.d[0])
}

func (c *bitChunk) Data() []byte {
	return c.d[:c.pos]
}

func (c *bitChunk) reader() *bitChunkReader {
	fmt.Println(len(c.d), c.pos)
	return &bitChunkReader{d: c.d[1 : c.pos+1], count: 8}
}

type bitChunkReader struct {
	d     []byte
	count uint8
	l     uint32
}

func (r *bitChunkReader) readBit() (bit, error) {
	if len(r.d) == 0 {
		return false, io.EOF
	}

	if r.count == 0 {
		r.d = r.d[1:]
		// did we just run out of stuff to read?
		if len(r.d) == 0 {
			return false, io.EOF
		}
		r.count = 8
	}

	r.count--
	d := r.d[0] & 0x80
	r.d[0] <<= 1
	return d != 0, nil
}

func (r *bitChunkReader) readByte() (byte, error) {
	if len(r.d) == 0 {
		return 0, io.EOF
	}

	if r.count == 0 {
		r.d = r.d[1:]

		if len(r.d) == 0 {
			return 0, io.EOF
		}

		r.count = 8
	}

	if r.count == 8 {
		r.count = 0
		return r.d[0], nil
	}

	byt := r.d[0]
	r.d = r.d[1:]

	if len(r.d) == 0 {
		return 0, io.EOF
	}

	byt |= r.d[0] >> r.count
	r.d[0] <<= (8 - r.count)

	return byt, nil
}

func (r *bitChunkReader) readBits(nbits int) (uint64, error) {
	var u uint64

	for nbits >= 8 {
		byt, err := r.readByte()
		if err != nil {
			return 0, err
		}

		u = (u << 8) | uint64(byt)
		nbits -= 8
	}

	if nbits == 0 {
		return u, nil
	}

	if nbits > int(r.count) {
		u = (u << uint(r.count)) | uint64(r.d[0]>>(8-r.count))
		nbits -= int(r.count)
		r.d = r.d[1:]

		if len(r.d) == 0 {
			return 0, io.EOF
		}
		r.count = 8
	}

	u = (u << uint(nbits)) | uint64(r.d[0]>>(8-uint(nbits)))
	r.d[0] <<= uint(nbits)
	r.count -= uint8(nbits)
	return u, nil
}

// append appends the first nbits bits from b into the chunk.
// b must contain at least nbits bits.
// We are using fixed 16 bytes as it might perform better due to
// more static assumptions.
func (c *bitChunk) append(b [20]byte, nbits int) error {
	if nbits > 8*(len(c.d)-int(c.pos)-1)-int(c.count) {
		return ErrChunkFull
	}

	c.writeBits(b, nbits)
	// Swap the working length and count integers into the ones used
	// to retrieve iterators. This allows to concurrently retrieve
	// iteartors while appending to a chunk.
	// This does not make it safe for concurrent appends!
	atomic.StoreUint32(&c.rl, c.pos)
	atomic.StoreUint32(&c.rcount, c.count)
	return nil
}

func (c *bitChunk) writeBit(bit bit) {
	if c.count == 0 {
		c.pos++
		c.count = 8
	}

	if bit {
		c.d[c.pos] |= 1 << (c.count - 1)
	}

	c.count--
}

func (c *bitChunk) writeByte(byt byte) {
	if c.count == 0 {
		c.pos++
		c.count = 8
	}

	// fill up b.b with b.count bits from byt
	c.d[c.pos] |= byt >> (8 - c.count)

	c.pos++
	c.d[c.pos] = byt << c.count
}

func (c *bitChunk) writeBits(b [20]byte, nbits int) {
	i := 0
	for nbits >= 8 {
		c.writeByte(b[i])
		i++
		nbits -= 8
	}

	bi := b[i]
	for nbits > 0 {
		c.writeBit((bi >> 7) == 1)
		bi <<= 1
		nbits--
	}
}

// PlainChunk implements a Chunk using simple 16 byte representations
// of sample pairs.
type PlainChunk struct {
	rawChunk
}

// NewPlainChunk returns a new chunk using EncPlain.
func NewPlainChunk(sz int) *PlainChunk {
	return &PlainChunk{rawChunk: newRawChunk(sz, EncPlain)}
}

// Iterator implements the Chunk interface.
func (c *PlainChunk) Iterator() Iterator {
	return &plainChunkIterator{c: c.d[1:c.l]}
}

// Appender implements the Chunk interface.
func (c *PlainChunk) Appender() Appender {
	return &plainChunkAppender{c: &c.rawChunk}
}

type plainChunkAppender struct {
	c *rawChunk
}

// Append implements the Appender interface,
func (a *plainChunkAppender) Append(ts model.Time, v model.SampleValue) error {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, uint64(ts))
	binary.LittleEndian.PutUint64(b[8:], math.Float64bits(float64(v)))
	return a.c.append(b)
}

type plainChunkIterator struct {
	c   []byte // chunk data
	pos int    // position of last emitted element
	err error  // last error
}

func (it *plainChunkIterator) Err() error {
	return it.err
}

func (it *plainChunkIterator) timeAt(pos int) model.Time {
	return model.Time(binary.LittleEndian.Uint64(it.c[pos:]))
}

func (it *plainChunkIterator) valueAt(pos int) model.SampleValue {
	return model.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(it.c[pos:])))
}

func (it *plainChunkIterator) First() (model.SamplePair, bool) {
	it.pos = 0
	return it.Next()
}

func (it *plainChunkIterator) Seek(ts model.Time) (model.SamplePair, bool) {
	for it.pos = 0; it.pos < len(it.c); it.pos += 16 {
		if t := it.timeAt(it.pos); t >= ts {
			return model.SamplePair{
				Timestamp: t,
				Value:     it.valueAt(it.pos + 8),
			}, true
		}
	}
	it.err = io.EOF
	return model.SamplePair{}, false
}

func (it *plainChunkIterator) Next() (model.SamplePair, bool) {
	it.pos += 16
	if it.pos >= len(it.c) {
		it.err = io.EOF
		return model.SamplePair{}, false
	}
	return model.SamplePair{
		Timestamp: it.timeAt(it.pos),
		Value:     it.valueAt(it.pos + 8),
	}, true
}
