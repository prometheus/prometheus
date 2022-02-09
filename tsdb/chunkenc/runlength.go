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
)

type RunLength struct {
	bt bstream // holds the stream for timestamps
	bv bstream // holds the stream for values
}

// NewRunLength returns a new chunk with run-length encoding.
func NewRunLength() *RunLength {
	return &RunLength{
		bt: bstream{stream: make([]byte, 2, 128)},
		bv: bstream{stream: make([]byte, 4, 128)},
	}
}

// Encoding returns the encoding type.
func (c *RunLength) Encoding() Encoding {
	return EncRunLength
}

func (c *RunLength) Bytes() []byte {
	b := make([]byte, len(c.bt.bytes())+len(c.bv.bytes()))
	copy(b, c.bt.bytes())
	copy(b, c.bv.bytes())
	return b
}

func (c *RunLength) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

func (c *RunLength) Compact() {
	//TODO implement me
}

func (c *RunLength) Appender() (Appender, error) {
	it := c.iterator(nil)
	for it.Next() {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	a := &runLengthAppender{
		bt: &c.bt,
		bv: &c.bv,

		t:      it.t,
		v:      uint64(it.v),
		tDelta: it.tDelta,
	}
	return a, nil
}

func (c *RunLength) iterator(it Iterator) *runLengthIterator {
	if rleIter, ok := it.(*runLengthIterator); ok {
		rleIter.Reset(c.bt.bytes(), c.bv.bytes())
		return rleIter
	}
	return &runLengthIterator{
		tbr: newBReader(c.bt.bytes()[2:]),
		vbr: newBReader(c.bv.bytes()[4:]),

		numTotal: binary.BigEndian.Uint16(c.bt.bytes()),
	}
}

func (c *RunLength) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type runLengthAppender struct {
	bt *bstream
	bv *bstream

	t      int64
	tDelta uint64

	v uint64
}

func (a *runLengthAppender) Append(t int64, vf float64) {
	//// We convert float64 to uint64 for storing.
	//v := math.Float64bits(vf)
	v := uint64(vf) // Only works with integers

	num := binary.BigEndian.Uint16(a.bt.bytes())

	if num == 0 {
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.bt.writeByte(b)
		}
	} else if num == 1 {
		tDelta := uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.bt.writeByte(b)
		}
		a.tDelta = tDelta
	} else {
		tDelta := uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		// Gorilla has a max resolution of seconds, Prometheus milliseconds.
		// Thus, we use higher value range steps with larger bit size.
		switch {
		case dod == 0:
			a.bt.writeBit(zero)
		case bitRange(dod, 14):
			a.bt.writeBits(0b10, 2)
			a.bt.writeBits(uint64(dod), 14)
		case bitRange(dod, 17):
			a.bt.writeBits(0b110, 3)
			a.bt.writeBits(uint64(dod), 17)
		case bitRange(dod, 20):
			a.bt.writeBits(0b1110, 4)
			a.bt.writeBits(uint64(dod), 20)
		default:
			a.bt.writeBits(0b1111, 4)
			a.bt.writeBits(uint64(dod), 64)
		}

		a.tDelta = tDelta
	}

	// Always append the first value regardless of its value.
	// Then always write the next new value when the values differ.
	// Otherwise, simply increase the count of the current value.
	if num == 0 || a.v != v {
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, v)] {
			a.bv.writeByte(b)
		}

		buf = make([]byte, 2)
		binary.BigEndian.PutUint16(buf, 1)
		for _, b := range buf {
			a.bv.writeByte(b)
		}

		vals := binary.BigEndian.Uint16(a.bv.bytes()[2:])
		binary.BigEndian.PutUint16(a.bv.bytes()[2:], vals+1)
	} else {
		b := a.bv.bytes()
		// Read the last 3 bytes as the bstream always appends a trailing 0,
		// and we need to two bytes before for the length uint16.
		count := binary.BigEndian.Uint16(b[len(b)-3:])
		binary.BigEndian.PutUint16(a.bv.bytes()[len(b)-3:], count+1)
	}

	a.t = t
	a.v = v
	binary.BigEndian.PutUint16(a.bt.bytes(), num+1)
}

type runLengthIterator struct {
	tbr bstreamReader
	vbr bstreamReader

	numTotal uint16
	numRead  uint16

	t int64
	v float64

	// lengthLeft stores how often we need to still iterate over the same value
	lengthLeft uint16

	// vals stores how many values we have yet to see
	vals uint16

	err    error
	tDelta uint64
}

func (it *runLengthIterator) Seek(t int64) bool {
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

func (it *runLengthIterator) At() (int64, float64) {
	return it.t, it.v
}

func (it *runLengthIterator) Err() error {
	return it.err
}

func (it *runLengthIterator) Reset(bt []byte, bv []byte) {
	it.tbr = newBReader(bt[2:])
	it.vbr = newBReader(bv[4:])
	it.numTotal = binary.BigEndian.Uint16(bt)

	it.numRead = 0
	it.t = 0
	it.v = 0
	it.lengthLeft = 0
	it.vals = 0
	it.tDelta = 0
	it.err = nil
}

func (it *runLengthIterator) Next() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	if it.numRead == 0 {
		t, err := binary.ReadVarint(&it.tbr)
		if err != nil {
			it.err = err
			return false
		}
		it.t = t
	} else if it.numRead == 1 {
		tDelta, err := binary.ReadUvarint(&it.tbr)
		if err != nil {
			it.err = err
			return false
		}
		it.tDelta = tDelta
		it.t = it.t + int64(it.tDelta)
	} else {
		var d byte
		// read delta-of-delta
		for i := 0; i < 4; i++ {
			d <<= 1
			bit, err := it.tbr.readBitFast()
			if err != nil {
				bit, err = it.tbr.readBit()
			}
			if err != nil {
				it.err = err
				return false
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
			bits, err := it.tbr.readBits(64)
			if err != nil {
				it.err = err
				return false
			}

			dod = int64(bits)
		}

		if sz != 0 {
			bits, err := it.tbr.readBitsFast(sz)
			if err != nil {
				bits, err = it.tbr.readBits(sz)
			}
			if err != nil {
				it.err = err
				return false
			}

			// Account for negative numbers, which come back as high unsigned numbers.
			// See docs/bstream.md.
			if bits > (1 << (sz - 1)) {
				bits -= 1 << sz
			}
			dod = int64(bits)
		}

		it.tDelta = uint64(int64(it.tDelta) + dod)
		it.t = it.t + int64(it.tDelta)
	}

	if it.lengthLeft == 0 {
		v, err := binary.ReadUvarint(&it.vbr)
		if err != nil {
			it.err = err
			return false
		}
		//it.v = math.Float64frombits(v)
		it.v = float64(v) // only works with integers

		lengthBytes := make([]byte, 2)
		for i := 0; i < 2; i++ {
			b, err := it.vbr.ReadByte()
			if err != nil {
				it.err = err
				return false
			}
			lengthBytes[i] = b
		}
		it.vals--
		if it.vals > 0 {
			it.lengthLeft = binary.BigEndian.Uint16(lengthBytes) - 1 // we've already read the first one
		}
		if it.vals == 0 {
			// We can't read the length bytes of the last value, because it may
			// be actively written to, so we infer it from how many samples we
			// know must be left. This is safe because we know this is the last
			// value.
			it.lengthLeft = it.numTotal - it.numRead
		}
	} else {
		it.lengthLeft--
	}

	it.numRead++
	return true
}
