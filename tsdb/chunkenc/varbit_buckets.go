package chunkenc

// putInt64VBBucket writes an int64 using varbit optimized for SHS buckets
// note: we could improve this further: each branch doesn't need to support any values of any of the prior branches, so we can expand the range of each branch. do more with fewer bits
func putInt64VBBucket(b *bstream, val int64) {
	switch {
	case val == 0:
		b.writeBit(zero)
	case bitRange(val, 3): // -3 <= val <= 4
		b.writeBits(0x02, 2) // '10'
		b.writeBits(uint64(val), 3)
	case bitRange(val, 6): // -31 <= val <= 32
		b.writeBits(0x06, 3) // '110'
		b.writeBits(uint64(val), 6)
	case bitRange(val, 9): // -255 <= val <= 256
		b.writeBits(0x0e, 4) // '1110'
		b.writeBits(uint64(val), 9)
	case bitRange(val, 12): // -2047 <= val <= 2048
		b.writeBits(0x1e, 5) // '11110'
		b.writeBits(uint64(val), 12)
	default:
		b.writeBits(0x3e, 5) // '11111'
		b.writeBits(uint64(val), 64)
	}
}

// readInt64VBBucket reads an int64 using varbit optimized for SHS buckets
func readInt64VBBucket(b *bstreamReader) (int64, error) {
	var d byte
	for i := 0; i < 5; i++ {
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
		sz = 3
	case 0x06: // '110'
		sz = 6
	case 0x0e: // '1110'
		sz = 9
	case 0x1e: // '11110'
		sz = 12
	case 0x3e: // '11111'
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
