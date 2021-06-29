package chunkenc

// putInt64PromVBXor writes an int64 using varbit like xor.go uses for millisec timestamps
func putInt64PromVBXor(b *bstream, val int64) {
	switch {
	case val == 0:
		b.writeBit(zero)
	case bitRange(val, 14): // -8191 <= val <= 8192
		b.writeBits(0x02, 2) // '10'
		b.writeBits(uint64(val), 14)
	case bitRange(val, 17): // -65535 <= val <= 65536
		b.writeBits(0x06, 3) // '110'
		b.writeBits(uint64(val), 17)
	case bitRange(val, 20): // -524287 <= val <= 524288
		b.writeBits(0x0e, 4) // '1110'
		b.writeBits(uint64(val), 20)
	default:
		b.writeBits(0x0f, 4) // '1111'
		b.writeBits(uint64(val), 64)
	}
}

// readInt64PromVBXor reads an int64 using varbit like xor.go uses for millisec timestamps
func readInt64PromVBXor(b bstreamReader) (int64, error) {
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

	var val int64
	var sz uint8

	switch d {
	case 0x00:
		// val == 0
	case 0x02: // '10'
		sz = 14
	case 0x06: // '110'
		sz = 17
	case 0x0e: // '1110'
		sz = 20
	case 0x0f: // '1111'
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := b.readBits(64)
		if err != nil {
			return 0, err
		}

		val = int64(bits)
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
		val = int64(bits)
	}

	return val, nil
}
