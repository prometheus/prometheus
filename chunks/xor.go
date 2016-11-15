package chunks

import (
	"encoding/binary"
	"math"

	"github.com/prometheus/common/model"
)

// XORChunk holds XOR encoded sample data.
type XORChunk struct {
	rawChunk
}

// NewXORChunk returns a new chunk with XOR encoding of the given size.
func NewXORChunk(sz int) *XORChunk {
	return &XORChunk{rawChunk: newRawChunk(sz, EncXOR)}
}

// Appender implements the Chunk interface.
func (c *XORChunk) Appender() Appender {
	return &xorAppender{c: &c.rawChunk}
}

// Iterator implements the Chunk interface.
func (c *XORChunk) Iterator() Iterator {
	return &xorIterator{d: c.d[1:c.l]}
}

type xorAppender struct {
	c   *rawChunk
	num int
	buf [16]byte

	lastV      float64
	lastT      int64
	lastTDelta uint64
}

func (a *xorAppender) Append(ts model.Time, v model.SampleValue) error {
	if a.num == 0 {
		n := binary.PutVarint(a.buf[:], int64(ts))
		binary.BigEndian.PutUint64(a.buf[n:], math.Float64bits(float64(v)))
		if err := a.c.append(a.buf[:n+8]); err != nil {
			return err
		}
		a.lastT, a.lastV = int64(ts), float64(v)
		a.num++
		return nil
	}
	if a.num == 1 {
		a.lastTDelta = uint64(int64(ts) - a.lastT)
	}

	a.num++
	return nil
}

type xorIterator struct {
	d []byte
}

func (it *xorIterator) First() (model.SamplePair, bool) {
	return model.SamplePair{}, false
}

func (it *xorIterator) Seek(ts model.Time) (model.SamplePair, bool) {
	return model.SamplePair{}, false
}

func (it *xorIterator) Next() (model.SamplePair, bool) {
	return model.SamplePair{}, false
}

func (it *xorIterator) Err() error {
	return nil
}
