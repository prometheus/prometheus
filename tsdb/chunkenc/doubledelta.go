// Copyright 2017 The Prometheus Authors
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

package chunkenc

import (
	"encoding/binary"
	"math"
)

// DoubleDeltaChunk holds double delta encoded sample data.
type DoubleDeltaChunk struct {
	b bstream
}

func NewDoubleDeltaChunk() *DoubleDeltaChunk {
	b := make([]byte, 2, 128)
	return &DoubleDeltaChunk{b: bstream{stream: b, count: 0}}
}

func (c *DoubleDeltaChunk) Encoding() Encoding {
	return EncDoubleDelta
}

func (c *DoubleDeltaChunk) Bytes() []byte {
	return c.b.bytes()
}

func (c *DoubleDeltaChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

func (c *DoubleDeltaChunk) Compact() {
	// TODO
}

func (c *DoubleDeltaChunk) Appender() (Appender, error) {
	it := c.iterator(nil)
	for it.Next() {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	a := &doubleDeltaAppender{
		b:      &c.b,
		t:      it.t,
		v:      it.v,
		tDelta: it.tDelta,
		vDelta: it.vDelta,
	}
	return a, nil
}

func (c *DoubleDeltaChunk) iterator(it Iterator) *doubleDeltaIterator {
	if ddIter, ok := it.(*doubleDeltaIterator); ok {
		ddIter.Reset(c.b.bytes())
		return ddIter
	}
	return &doubleDeltaIterator{
		br:       newBReader(c.b.bytes()[2:]),
		numTotal: binary.BigEndian.Uint16(c.b.bytes()),
		t:        math.MinInt64,
	}
}

func (c *DoubleDeltaChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type doubleDeltaAppender struct {
	b *bstream

	t      int64
	v      int64
	tDelta uint64
	vDelta uint64
}

func (a *doubleDeltaAppender) Append(t int64, vf float64) {
	// We convert float64 to int64, so make sure not a single value has decimals before using this chunk!
	v := int64(vf)

	num := binary.BigEndian.Uint16(a.b.bytes())

	if num == 0 {
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		for _, b := range buf[:binary.PutVarint(buf, v)] {
			a.b.writeByte(b)
		}
	} else if num == 1 {
		tDelta := uint64(t - a.t)
		vDelta := uint64(v) - uint64(a.v)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}
		for _, b := range buf[:binary.PutUvarint(buf, vDelta)] {
			a.b.writeByte(b)
		}

		a.tDelta = tDelta
		a.vDelta = vDelta
	} else {
		writeDoD := func(dod int64) {
			// Gorilla has a max resolution of seconds, Prometheus milliseconds.
			// Thus we use higher value range steps with larger bit size.
			switch {
			case dod == 0:
				a.b.writeBit(zero)
			case bitRange(dod, 14):
				a.b.writeBits(0b10, 2)
				a.b.writeBits(uint64(dod), 14)
			case bitRange(dod, 17):
				a.b.writeBits(0b110, 3)
				a.b.writeBits(uint64(dod), 17)
			case bitRange(dod, 20):
				a.b.writeBits(0b1110, 4)
				a.b.writeBits(uint64(dod), 20)
			default:
				a.b.writeBits(0b1111, 4)
				a.b.writeBits(uint64(dod), 64)
			}
		}

		tDelta := uint64(t - a.t)
		vDelta := uint64(v) - uint64(a.v)
		tDoD := int64(tDelta - a.tDelta)
		vDoD := int64(vDelta - a.vDelta)

		writeDoD(tDoD)
		writeDoD(vDoD)

		a.tDelta = tDelta
		a.vDelta = vDelta
	}

	a.t = t
	a.v = v
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
}

type doubleDeltaIterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	t      int64
	v      int64
	tDelta uint64
	vDelta uint64

	err error
}

func (it *doubleDeltaIterator) Seek(t int64) bool {
	if it.err != nil {
		return false
	}

	for t > it.t || it.numRead == 0 {
		if !it.Next() {
			return false
		}
	}
	return true
}

func (it *doubleDeltaIterator) At() (int64, float64) {
	return it.t, float64(it.v)
}

func (it *doubleDeltaIterator) Err() error {
	return it.err
}

func (it *doubleDeltaIterator) Reset(b []byte) {
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[2:])
	it.numTotal = binary.BigEndian.Uint16(b)

	it.numRead = 0
	it.t = 0
	it.v = 0
	it.tDelta = 0
	it.vDelta = 0
	it.err = nil
}

func (it *doubleDeltaIterator) Next() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	if it.numRead == 0 {
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		v, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.t = t
		it.v = v
		it.numRead++
		return true
	} else if it.numRead == 1 {
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		vDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}

		it.tDelta = tDelta
		it.t = it.t + int64(it.tDelta)
		it.vDelta = vDelta
		it.v = it.v + int64(it.vDelta)

		it.numRead++
		return true
	}

	tDoD, next := it.readDoD()
	if !next {
		return next
	}

	vDoD, next := it.readDoD()
	if !next {
		return next
	}

	it.tDelta = uint64(int64(it.tDelta) + tDoD)
	it.t = it.t + int64(it.tDelta)

	it.vDelta = uint64(int64(it.vDelta) + vDoD)
	it.v = it.v + int64(it.vDelta)

	it.numRead++
	return true
}

func (it *doubleDeltaIterator) readDoD() (int64, bool) {
	var d byte
	for i := 0; i < 4; i++ {
		d <<= 1
		bit, err := it.br.readBitFast()
		if err != nil {
			bit, err = it.br.readBit()
		}
		if err != nil {
			it.err = err
			return 0, false
		}
		if bit == zero {
			break
		}
		d |= 1
	}
	var sz uint8
	var dod int64
	switch d {
	case 0b0:
		// dod == 0
	case 0b10:
		sz = 14
	case 0b110:
		sz = 17
	case 0b1110:
		sz = 20
	case 0b1111:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return 0, false
		}

		dod = int64(bits)
	}

	if sz != 0 {
		bits, err := it.br.readBitsFast(sz)
		if err != nil {
			bits, err = it.br.readBits(sz)
		}
		if err != nil {
			it.err = err
			return 0, false
		}

		// Account for negative numbers, which come back as high unsigned numbers.
		// See docs/bstream.md.
		if bits > (1 << (sz - 1)) {
			bits -= 1 << sz
		}
		dod = int64(bits)
	}
	return dod, true
}
