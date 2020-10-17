// +build !amd64 noasm

package sip13

import (
	"math/bits"
	"unsafe"
)

func Sum64(k0, k1 uint64, b []byte) uint64 {
	return Sum64Str(k0, k1, unsafeB2S(b))
}

func readUint64(b string) uint64 {
	_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func unsafeB2S(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func Sum64Str(k0, k1 uint64, p string) uint64 {
	var v0, v1, v2, v3 uint64

	v0 = k0 ^ 0x736f6d6570736575
	v1 = k1 ^ 0x646f72616e646f6d
	v2 = k0 ^ 0x6c7967656e657261
	v3 = k1 ^ 0x7465646279746573
	b := uint64(len(p)) << 56

	for len(p) >= 8 {
		m := readUint64(p)
		v3 ^= m
		v0 += v1
		v2 += v3
		v1 = bits.RotateLeft64(v1, 13)
		v3 = bits.RotateLeft64(v3, 16)
		v1 ^= v0
		v3 ^= v2
		v0 = bits.RotateLeft64(v0, 32)
		v2 += v1
		v0 += v3
		v1 = bits.RotateLeft64(v1, 17)
		v3 = bits.RotateLeft64(v3, 21)
		v1 ^= v2
		v3 ^= v0
		v2 = bits.RotateLeft64(v2, 32)
		v0 ^= m
		p = p[8:]
	}

	for _, c := range []byte(p) {
		b = bits.RotateLeft64(b|uint64(c), 56)
	}
	m := bits.RotateLeft64(b, len(p)*8)

	// last block with finalization
	v3 ^= m
	f := uint64(0xff)
	for i := 0; i < 4; i++ {
		v0 += v1
		v2 += v3
		v1 = bits.RotateLeft64(v1, 13)
		v3 = bits.RotateLeft64(v3, 16)
		v1 ^= v0
		v3 ^= v2
		v0 = bits.RotateLeft64(v0, 32)
		v2 += v1
		v0 += v3
		v1 = bits.RotateLeft64(v1, 17)
		v3 = bits.RotateLeft64(v3, 21)
		v1 ^= v2
		v3 ^= v0
		v2 = bits.RotateLeft64(v2, 32)
		v2 ^= f
		v0 ^= m
		m, f = 0, 0 // clear last block and finalization mixins
	}

	return v0 ^ v1 ^ v2 ^ v3
}
