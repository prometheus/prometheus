package chunks

import (
	"math"

	bits "github.com/dgryski/go-bits"
	"github.com/prometheus/common/model"
)

// XORChunk holds XOR encoded sample data.
type XORChunk struct {
	bstream

	num uint16
	sz  int

	lastLen   int
	lastCount uint8
}

// NewXORChunk returns a new chunk with XOR encoding of the given size.
func NewXORChunk(sz int) *XORChunk {
	return &XORChunk{sz: sz}
}

func (c *XORChunk) Data() []byte {
	return nil
}

// Appender implements the Chunk interface.
func (c *XORChunk) Appender() Appender {
	return &xorAppender{c: c}
}

// Iterator implements the Chunk interface.
func (c *XORChunk) Iterator() Iterator {
	br := c.bstream.clone()
	br.count = 8
	return &xorIterator{br: br, numTotal: c.num}
}

type xorAppender struct {
	c *XORChunk

	t      int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8
	finished bool
}

func (a *xorAppender) Append(ts model.Time, v model.SampleValue) error {
	// TODO(fabxc): remove Prometheus types from interface.
	return a.append(int64(ts), float64(v))
}

func (a *xorAppender) append(t int64, v float64) error {
	var tDelta uint64

	if a.c.num == 0 {
		// TODO: store varint time?
		a.c.writeBits(uint64(t), 64)
		a.c.writeBits(math.Float64bits(v), 64)

	} else if a.c.num == 1 {
		tDelta = uint64(t - a.t)
		// TODO: use varint or other encoding for first delta?
		a.c.writeBits(tDelta, 64)
		a.writeVDelta(v)

	} else {
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		// Gorilla has a max resolution of seconds, Prometheus milliseconds.
		// Thus we use higher value range steps with larger bit size.
		switch {
		case dod == 0:
			a.c.writeBit(zero)
		case -8191 <= dod && dod <= 8192:
			a.c.writeBits(0x02, 2) // '10'
			a.c.writeBits(uint64(dod), 14)
		case -65535 <= dod && dod <= 65536:
			a.c.writeBits(0x06, 3) // '110'
			a.c.writeBits(uint64(dod), 17)
		case -524287 <= dod && dod <= 524288:
			a.c.writeBits(0x0e, 4) // '1110'
			a.c.writeBits(uint64(dod), 20)
		default:
			a.c.writeBits(0x0f, 4) // '1111'
			a.c.writeBits(uint64(dod), 64)
		}

		a.writeVDelta(v)
	}

	if len(a.c.stream) > a.c.sz {
		return ErrChunkFull
	}

	a.t = t
	a.v = v
	a.c.num++
	a.tDelta = tDelta
	a.c.lastCount = a.c.count
	a.c.lastLen = len(a.c.stream)
	return nil
}

func (a *xorAppender) writeVDelta(v float64) {
	vDelta := math.Float64bits(v) ^ math.Float64bits(a.v)

	if vDelta == 0 {
		a.c.writeBit(zero)
		return
	}
	a.c.writeBit(one)

	leading := uint8(bits.Clz(vDelta))
	trailing := uint8(bits.Ctz(vDelta))

	// clamp number of leading zeros to avoid overflow when encoding
	if leading >= 32 {
		leading = 31
	}

	// TODO(dgryski): check if it's 'cheaper' to reset the leading/trailing bits instead
	if a.leading != ^uint8(0) && leading >= a.leading && trailing >= a.trailing {
		a.c.writeBit(zero)
		a.c.writeBits(vDelta>>a.trailing, 64-int(a.leading)-int(a.trailing))
	} else {
		a.leading, a.trailing = leading, trailing

		a.c.writeBit(one)
		a.c.writeBits(uint64(leading), 5)

		// Note that if leading == trailing == 0, then sigbits == 64.  But that value doesn't actually fit into the 6 bits we have.
		// Luckily, we never need to encode 0 significant bits, since that would put us in the other case (vdelta == 0).
		// So instead we write out a 0 and adjust it back to 64 on unpacking.
		sigbits := 64 - leading - trailing
		a.c.writeBits(uint64(sigbits), 6)
		a.c.writeBits(vDelta>>trailing, int(sigbits))
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

	tDelta int64
	err    error
}

func (it *xorIterator) Values() (int64, float64) {
	return it.t, it.val
}

func (it *xorIterator) NextB() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	var d byte
	var dod int32
	var sz uint
	var tDelta int64

	if it.numRead == 0 {
		t, err := it.br.readBits(64)
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
		tDelta, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return false
		}
		it.tDelta = int64(tDelta)
		it.t = it.t + it.tDelta

		goto ReadValue
	}

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

		dod = int32(bits)
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
		dod = int32(bits)
	}

	tDelta = it.tDelta + int64(dod)

	it.tDelta = tDelta
	it.t = it.t + it.tDelta

ReadValue:
	// read compressed value
	bit, err := it.br.readBit()
	if err != nil {
		it.err = err
		return false
	}

	if bit == zero {
		// it.val = it.val
	} else {
		bit, itErr := it.br.readBit()
		if itErr != nil {
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
	return it.err
}
