package chunks

import "io"

// bstream is a stream of bits
type bstream struct {
	stream []byte // the data stream
	count  uint8  // how many bits are valid in current byte
}

func newBReader(b []byte) *bstream {
	return &bstream{stream: b, count: 8}
}

func newBWriter(size int) *bstream {
	return &bstream{stream: make([]byte, 0, size), count: 0}
}

func (b *bstream) clone() *bstream {
	d := make([]byte, len(b.stream))
	copy(d, b.stream)
	return &bstream{stream: d, count: b.count}
}

func (b *bstream) bytes() []byte {
	return b.stream
}

type bit bool

const (
	zero bit = false
	one  bit = true
)

func (b *bstream) writeBit(bit bit) {
	if b.count == 0 {
		b.stream = append(b.stream, 0)
		b.count = 8
	}

	i := len(b.stream) - 1

	if bit {
		b.stream[i] |= 1 << (b.count - 1)
	}

	b.count--
}

func (b *bstream) writeByte(byt byte) {
	if b.count == 0 {
		b.stream = append(b.stream, 0)
		b.count = 8
	}

	i := len(b.stream) - 1

	// fill up b.b with b.count bits from byt
	b.stream[i] |= byt >> (8 - b.count)

	b.stream = append(b.stream, 0)
	i++
	b.stream[i] = byt << b.count
}

func (b *bstream) writeBits(u uint64, nbits int) {
	u <<= (64 - uint(nbits))
	for nbits >= 8 {
		byt := byte(u >> 56)
		b.writeByte(byt)
		u <<= 8
		nbits -= 8
	}

	for nbits > 0 {
		b.writeBit((u >> 63) == 1)
		u <<= 1
		nbits--
	}
}

func (b *bstream) readBit() (bit, error) {
	if len(b.stream) == 0 {
		return false, io.EOF
	}

	if b.count == 0 {
		b.stream = b.stream[1:]

		if len(b.stream) == 0 {
			return false, io.EOF
		}
		b.count = 8
	}

	d := (b.stream[0] << (8 - b.count)) & 0x80
	b.count--
	return d != 0, nil
}

func (b *bstream) ReadByte() (byte, error) {
	return b.readByte()
}

func (b *bstream) readByte() (byte, error) {
	if len(b.stream) == 0 {
		return 0, io.EOF
	}

	if b.count == 0 {
		b.stream = b.stream[1:]

		if len(b.stream) == 0 {
			return 0, io.EOF
		}
		return b.stream[0], nil
	}

	if b.count == 8 {
		b.count = 0
		return b.stream[0], nil
	}

	byt := b.stream[0] << (8 - b.count)
	b.stream = b.stream[1:]

	if len(b.stream) == 0 {
		return 0, io.EOF
	}

	// We just advanced the stream and can assume the shift to be 0.
	byt |= b.stream[0] >> b.count

	return byt, nil
}

func (b *bstream) readBits(nbits int) (uint64, error) {
	var u uint64

	for nbits >= 8 {
		byt, err := b.readByte()
		if err != nil {
			return 0, err
		}

		u = (u << 8) | uint64(byt)
		nbits -= 8
	}

	if nbits == 0 {
		return u, nil
	}

	if nbits > int(b.count) {
		u = (u << uint(b.count)) | uint64((b.stream[0]<<(8-b.count))>>(8-b.count))
		nbits -= int(b.count)
		b.stream = b.stream[1:]

		if len(b.stream) == 0 {
			return 0, io.EOF
		}
		b.count = 8
	}

	u = (u << uint(nbits)) | uint64((b.stream[0]<<(8-b.count))>>(8-uint(nbits)))
	b.count -= uint8(nbits)
	return u, nil
}
