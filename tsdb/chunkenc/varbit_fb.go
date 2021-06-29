package chunkenc

// putInt32VBFB saves an int32 using facebook's timestamp varbit encoding
func putInt32VBFB(b *bstream, val int32) {
	switch {
	case val == 0:
		b.writeBit(zero)
	case -63 <= val && val <= 64:
		b.writeBits(0x02, 2) // '10'
		b.writeBits(uint64(val), 7)
	case -255 <= val && val <= 256:
		b.writeBits(0x06, 3) // '110'
		b.writeBits(uint64(val), 9)
	case -2047 <= val && val <= 2048:
		b.writeBits(0x0e, 4) // '1110'
		b.writeBits(uint64(val), 12)
	default:
		b.writeBits(0x0f, 4) // '1111'
		b.writeBits(uint64(val), 32)
	}
}

// readInt32VBFB saves an int32 using facebook's timestamp varbit encoding
func readInt32VBFB(b bstreamReader) (int32, error) {
	var d byte
	for i := 0; i < 4; i++ {
		d <<= 1
		bit, err := b.readBitFast()
		if err != nil {
			bit, err = b.readBit()
		}
		if err != nil {
			return 0, err
		}
		if bit == zero {
			break
		}
		d |= 1
	}

	var val int32
	var sz uint8

	switch d {
	case 0x00:
		// val == 0
	case 0x02: // '10'
		sz = 7
	case 0x06: // '110'
		sz = 9
	case 0x0e: // '1110'
		sz = 12
	case 0x0f: // '1111'
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := b.readBits(32)
		if err != nil {
			return 0, err
		}

		val = int32(bits)
	}

	if sz != 0 {
		bits, err := b.readBitsFast(sz)
		if err != nil {
			bits, err = b.readBits(sz)
		}
		if err != nil {
			return 0, err
		}
		if bits > (1 << (sz - 1)) {
			// or something
			bits = bits - (1 << sz)
		}
		val = int32(bits)
	}

	return val, nil
}
