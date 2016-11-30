package chunks

import (
	"encoding/binary"
	"math"

	bits "github.com/dgryski/go-bits"
)

// XORChunk holds XOR encoded sample data.
type XORChunk struct {
	b   *bstream
	num uint16
	sz  int
}

// NewXORChunk returns a new chunk with XOR encoding of the given size.
func NewXORChunk(size int) *XORChunk {
	b := make([]byte, 3, 64)
	b[0] = byte(EncXOR)

	return &XORChunk{
		b:   &bstream{stream: b, count: 0},
		sz:  size,
		num: 0,
	}
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XORChunk) Bytes() []byte {
	b := c.b.bytes()
	// Lazily populate length bytes – probably not necessary to have the
	// cache value in struct.
	binary.LittleEndian.PutUint16(b[1:3], c.num)
	return b
}

// Appender implements the Chunk interface.
func (c *XORChunk) Appender() (Appender, error) {
	it := c.iterator()

	// To get an appender we must know the state it would have if we had
	// appended all existing data from scratch.
	// We iterate through the end and populate via the iterator's state.
	for it.Next() {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	a := &xorAppender{
		c:        c,
		b:        c.b,
		t:        it.t,
		v:        it.val,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,
	}
	if c.num == 0 {
		a.leading = 0xff
	}
	return a, nil
}

func (c *XORChunk) iterator() *xorIterator {
	// Should iterators guarantee to act on a copy of the data so it doesn't lock append?
	// When using striped locks to guard access to chunks, probably yes.
	// Could only copy data if the chunk is not completed yet.
	return &xorIterator{
		br:       newBReader(c.b.bytes()[3:]),
		numTotal: c.num,
	}
}

// Iterator implements the Chunk interface.
func (c *XORChunk) Iterator() Iterator {
	return fancyIterator{c.iterator()}
}

type xorAppender struct {
	c *XORChunk
	b *bstream

	t      int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8
}

func (a *xorAppender) Append(t int64, v float64) error {
	var tDelta uint64
	l := len(a.b.bytes())

	if a.c.num == 0 {
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)

	} else if a.c.num == 1 {
		tDelta = uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		a.writeVDelta(v)

	} else {
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		// Gorilla has a max resolution of seconds, Prometheus milliseconds.
		// Thus we use higher value range steps with larger bit size.
		switch {
		case dod == 0:
			a.b.writeBit(zero)
		case -8191 <= dod && dod <= 8192:
			a.b.writeBits(0x02, 2) // '10'
			a.b.writeBits(uint64(dod), 14)
		case -65535 <= dod && dod <= 65536:
			a.b.writeBits(0x06, 3) // '110'
			a.b.writeBits(uint64(dod), 17)
		case -524287 <= dod && dod <= 524288:
			a.b.writeBits(0x0e, 4) // '1110'
			a.b.writeBits(uint64(dod), 20)
		default:
			a.b.writeBits(0x0f, 4) // '1111'
			a.b.writeBits(uint64(dod), 64)
		}

		a.writeVDelta(v)
	}

	if len(a.b.bytes()) > a.c.sz {
		// If the appended data exceeded the size limit, we truncate
		// the underlying data slice back to the length we started with.
		a.b.stream = a.b.stream[:l]
		return ErrChunkFull
	}

	a.t = t
	a.v = v
	a.c.num++
	a.tDelta = tDelta

	return nil
}

func (a *xorAppender) writeVDelta(v float64) {
	vDelta := math.Float64bits(v) ^ math.Float64bits(a.v)

	if vDelta == 0 {
		a.b.writeBit(zero)
		return
	}
	a.b.writeBit(one)

	leading := uint8(bits.Clz(vDelta))
	trailing := uint8(bits.Ctz(vDelta))

	// Clamp number of leading zeros to avoid overflow when encoding.
	if leading >= 32 {
		leading = 31
	}

	if a.leading != 0xff && leading >= a.leading && trailing >= a.trailing {
		a.b.writeBit(zero)
		a.b.writeBits(vDelta>>a.trailing, 64-int(a.leading)-int(a.trailing))
	} else {
		a.leading, a.trailing = leading, trailing

		a.b.writeBit(one)
		a.b.writeBits(uint64(leading), 5)

		// Note that if leading == trailing == 0, then sigbits == 64.  But that value doesn't actually fit into the 6 bits we have.
		// Luckily, we never need to encode 0 significant bits, since that would put us in the other case (vdelta == 0).
		// So instead we write out a 0 and adjust it back to 64 on unpacking.
		sigbits := 64 - leading - trailing
		a.b.writeBits(uint64(sigbits), 6)
		a.b.writeBits(vDelta>>trailing, int(sigbits))
	}
}

type xorIterator struct {
	br       *bstream
	numTotal uint16
	numRead  uint16

	t   int64
	val float64

	leading  uint8
	trailing uint8

	tDelta uint64
	err    error
}

func (it *xorIterator) Values() (int64, float64) {
	return it.t, it.val
}

func (it *xorIterator) Err() error {
	return it.err
}

func (it *xorIterator) Next() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	if it.numRead == 0 {
		t, err := binary.ReadVarint(it.br)
		if err != nil {
			it.err = err
			return false
		}
		v, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return false
		}
		it.t = int64(t)
		it.val = math.Float64frombits(v)

		it.numRead++
		return true
	}
	if it.numRead == 1 {
		tDelta, err := binary.ReadUvarint(it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.tDelta = tDelta
		it.t = it.t + int64(it.tDelta)

		return it.readValue()
	}

	var d byte
	// read delta-of-delta
	for i := 0; i < 4; i++ {
		d <<= 1
		bit, err := it.br.readBit()
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
	case 0x00:
		// dod == 0
	case 0x02:
		sz = 14
	case 0x06:
		sz = 17
	case 0x0e:
		sz = 20
	case 0x0f:
		bits, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return false
		}

		dod = int64(bits)
	}

	if sz != 0 {
		bits, err := it.br.readBits(int(sz))
		if err != nil {
			it.err = err
			return false
		}
		if bits > (1 << (sz - 1)) {
			// or something
			bits = bits - (1 << sz)
		}
		dod = int64(bits)
	}

	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t = it.t + int64(it.tDelta)

	return it.readValue()
}

func (it *xorIterator) readValue() bool {
	bit, err := it.br.readBit()
	if err != nil {
		it.err = err
		return false
	}

	if bit == zero {
		// it.val = it.val
	} else {
		bit, err := it.br.readBit()
		if err != nil {
			it.err = err
			return false
		}
		if bit == zero {
			// reuse leading/trailing zero bits
			// it.leading, it.trailing = it.leading, it.trailing
		} else {
			bits, err := it.br.readBits(5)
			if err != nil {
				it.err = err
				return false
			}
			it.leading = uint8(bits)

			bits, err = it.br.readBits(6)
			if err != nil {
				it.err = err
				return false
			}
			mbits := uint8(bits)
			// 0 significant bits here means we overflowed and we actually need 64; see comment in encoder
			if mbits == 0 {
				mbits = 64
			}
			it.trailing = 64 - it.leading - mbits
		}

		mbits := int(64 - it.leading - it.trailing)
		bits, err := it.br.readBits(mbits)
		if err != nil {
			it.err = err
			return false
		}
		vbits := math.Float64bits(it.val)
		vbits ^= (bits << it.trailing)
		it.val = math.Float64frombits(vbits)
	}

	it.numRead++
	return true
}
