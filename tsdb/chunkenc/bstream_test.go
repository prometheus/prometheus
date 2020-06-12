package chunkenc

import (
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestBstreamReader(t *testing.T) {
	// Write to the bit stream.
	w := bstream{}
	for _, bit := range []bit{true, false} {
		w.writeBit(bit)
	}
	for nbits := 1; nbits <= 64; nbits++ {
		w.writeBits(uint64(nbits), nbits)
	}
	for v := 1; v < 10000; v += 123 {
		w.writeBits(uint64(v), 29)
	}

	// Read back.
	r := newBReader(w.bytes())
	for _, bit := range []bit{true, false} {
		v, err := r.readBitFast()
		if err != nil {
			v, err = r.readBit()
		}
		testutil.Ok(t, err)
		testutil.Equals(t, bit, v)
	}
	for nbits := uint8(1); nbits <= 64; nbits++ {
		v, err := r.readBitsFast(nbits)
		if err != nil {
			v, err = r.readBits(nbits)
		}
		testutil.Ok(t, err)
		testutil.Equals(t, uint64(nbits), v, "nbits=%d", nbits)
	}
	for v := 1; v < 10000; v += 123 {
		actual, err := r.readBitsFast(29)
		if err != nil {
			actual, err = r.readBits(29)
		}
		testutil.Ok(t, err)
		testutil.Equals(t, uint64(v), actual, "v=%d", v)
	}
}
