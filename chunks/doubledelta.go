package chunks

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/prometheus/common/model"
)

// DoubleDeltaChunk stores delta-delta encoded sample data.
type DoubleDeltaChunk struct {
	rawChunk
}

// NewDoubleDeltaChunk returns a new chunk using double delta encoding.
func NewDoubleDeltaChunk(sz int) *DoubleDeltaChunk {
	return &DoubleDeltaChunk{rawChunk: newRawChunk(sz, EncDoubleDelta)}
}

// Iterator implements the Chunk interface.
func (c *DoubleDeltaChunk) Iterator() Iterator {
	return &doubleDeltaIterator{d: c.d[1:c.l]}
}

// Appender implements the Chunk interface.
func (c *DoubleDeltaChunk) Appender() Appender {
	return &doubleDeltaAppender{c: &c.rawChunk}
}

type doubleDeltaIterator struct {
	d []byte

	err          error
	pos, num     int
	curT, curV   int64
	nextT, nextV int64
	deltaV       int64
	deltaT       uint64
}

func (it *doubleDeltaIterator) Err() error {
	return it.err
}

func (it *doubleDeltaIterator) readPair() bool {
	if len(it.d) == it.pos {
		return false
	}
	var (
		n, m int
		ddv  int64
		ddt  uint64
	)
	it.curT = it.nextT
	it.curV = it.nextV

	if it.num > 1 {
		ddt, n = binary.Uvarint(it.d[it.pos:])
		ddv, m = binary.Varint(it.d[it.pos+n:])
		it.deltaT += ddt
		it.deltaV += ddv
		it.nextT += int64(it.deltaT)
		it.nextV += it.deltaV
	} else if it.num == 1 {
		it.deltaT, n = binary.Uvarint(it.d[it.pos:])
		it.deltaV, m = binary.Varint(it.d[it.pos+n:])
		it.nextT += int64(it.deltaT)
		it.nextV += it.deltaV
	} else {
		it.nextT, n = binary.Varint(it.d[it.pos:])
		it.nextV, m = binary.Varint(it.d[it.pos+n:])
	}
	it.pos += n + m
	it.num++
	return true
}

func (it *doubleDeltaIterator) First() (model.SamplePair, bool) {
	it.pos = 0
	it.num = 0
	if !it.readPair() {
		it.err = io.EOF
		return model.SamplePair{}, false
	}
	return it.Next()
}

func (it *doubleDeltaIterator) Seek(ts model.Time) (model.SamplePair, bool) {
	if int64(ts) < it.nextT {
		it.pos = 0
		it.num = 0
		if !it.readPair() {
			it.err = io.EOF
			return model.SamplePair{}, false
		}
		if _, ok := it.Next(); !ok {
			return model.SamplePair{}, false
		}
	}
	for {
		if it.nextT > int64(ts) {
			if it.num < 2 {
				it.err = io.EOF
				return model.SamplePair{}, false
			}
			return model.SamplePair{
				Timestamp: model.Time(it.curT),
				Value:     model.SampleValue(it.curV),
			}, true
		}
		if _, ok := it.Next(); !ok {
			return model.SamplePair{}, false
		}
	}
}

func (it *doubleDeltaIterator) Next() (model.SamplePair, bool) {
	if it.err == io.EOF {
		return model.SamplePair{}, false
	}
	res := model.SamplePair{
		Timestamp: model.Time(it.nextT),
		Value:     model.SampleValue(it.nextV),
	}
	if !it.readPair() {
		it.err = io.EOF
	}
	return res, true
}

type doubleDeltaAppender struct {
	c   *rawChunk
	buf [16]byte
	num int // stored values so far.

	lastV, lastVDelta int64
	lastT             int64
	lastTDelta        uint64
}

func isInt(f model.SampleValue) (int64, bool) {
	x, frac := math.Modf(float64(f))
	if frac != 0 {
		return 0, false
	}
	return int64(x), true
}

// ErrNoInteger is returned if a non-integer is appended to
// a double delta chunk.
var ErrNoInteger = errors.New("not an integer")

func (a *doubleDeltaAppender) Append(ts model.Time, fv model.SampleValue) error {
	v, ok := isInt(fv)
	if !ok {
		return ErrNoInteger
	}
	if a.num == 0 {
		n := binary.PutVarint(a.buf[:], int64(ts))
		n += binary.PutVarint(a.buf[n:], v)
		if err := a.c.append(a.buf[:n]); err != nil {
			return err
		}
		a.lastT, a.lastV = int64(ts), v
		a.num++
		return nil
	}
	if a.num == 1 {
		a.lastTDelta, a.lastVDelta = uint64(int64(ts)-a.lastT), v-a.lastV
		n := binary.PutUvarint(a.buf[:], a.lastTDelta)
		n += binary.PutVarint(a.buf[n:], a.lastVDelta)
		if err := a.c.append(a.buf[:n]); err != nil {
			return err
		}
	} else {
		predT, predV := a.lastT+int64(a.lastTDelta), a.lastV+a.lastVDelta
		tdd := uint64(int64(ts) - predT)
		vdd := v - predV
		n := binary.PutUvarint(a.buf[:], tdd)
		n += binary.PutVarint(a.buf[n:], vdd)
		if err := a.c.append(a.buf[:n]); err != nil {
			return err
		}
		a.lastTDelta += tdd
		a.lastVDelta += vdd
	}
	a.lastT, a.lastV = int64(ts), v
	a.num++
	return nil
}
