// +build !amd64 noasm

package sip13

import (
	"encoding/binary"
	"math/bits"
)

type sip struct {
	v0, v1, v2, v3 uint64
}

func (s *sip) round() {
	s.v0 += s.v1
	s.v2 += s.v3
	s.v1 = bits.RotateLeft64(s.v1, 13)
	s.v3 = bits.RotateLeft64(s.v3, 16)
	s.v1 ^= s.v0
	s.v3 ^= s.v2
	s.v0 = bits.RotateLeft64(s.v0, 32)
	s.v2 += s.v1
	s.v0 += s.v3
	s.v1 = bits.RotateLeft64(s.v1, 17)
	s.v3 = bits.RotateLeft64(s.v3, 21)
	s.v1 ^= s.v2
	s.v3 ^= s.v0
	s.v2 = bits.RotateLeft64(s.v2, 32)
}

func Sum64Str(k0, k1 uint64, p string) uint64 {
	return Sum64(k0, k1, []byte(p))
}

func Sum64(k0, k1 uint64, p []byte) uint64 {

	s := sip{
		v0: k0 ^ 0x736f6d6570736575,
		v1: k1 ^ 0x646f72616e646f6d,
		v2: k0 ^ 0x6c7967656e657261,
		v3: k1 ^ 0x7465646279746573,
	}
	b := uint64(len(p)) << 56

	for len(p) >= 8 {
		m := binary.LittleEndian.Uint64(p[:8])
		s.v3 ^= m
		s.round()
		s.v0 ^= m
		p = p[8:]
	}

	switch len(p) {
	case 7:
		b |= uint64(p[6]) << 48
		fallthrough
	case 6:
		b |= uint64(p[5]) << 40
		fallthrough
	case 5:
		b |= uint64(p[4]) << 32
		fallthrough
	case 4:
		b |= uint64(p[3]) << 24
		fallthrough
	case 3:
		b |= uint64(p[2]) << 16
		fallthrough
	case 2:
		b |= uint64(p[1]) << 8
		fallthrough
	case 1:
		b |= uint64(p[0])
	}

	// last block
	s.v3 ^= b
	s.round()
	s.v0 ^= b

	// finalization
	s.v2 ^= 0xff
	s.round()
	s.round()
	s.round()

	return s.v0 ^ s.v1 ^ s.v2 ^ s.v3
}
