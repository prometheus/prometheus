package oss

import (
	"hash"
	"hash/crc64"
)

// digest represents the partial evaluation of a checksum.
type digest struct {
	crc uint64
	tab *crc64.Table
}

// NewCRC creates a new hash.Hash64 computing the CRC64 checksum
// using the polynomial represented by the Table.
func NewCRC(tab *crc64.Table, init uint64) hash.Hash64 { return &digest{init, tab} }

// Size returns the number of bytes sum will return.
func (d *digest) Size() int { return crc64.Size }

// BlockSize returns the hash's underlying block size.
// The Write method must be able to accept any amount
// of data, but it may operate more efficiently if all writes
// are a multiple of the block size.
func (d *digest) BlockSize() int { return 1 }

// Reset resets the hash to its initial state.
func (d *digest) Reset() { d.crc = 0 }

// Write (via the embedded io.Writer interface) adds more data to the running hash.
// It never returns an error.
func (d *digest) Write(p []byte) (n int, err error) {
	d.crc = crc64.Update(d.crc, d.tab, p)
	return len(p), nil
}

// Sum64 returns CRC64 value.
func (d *digest) Sum64() uint64 { return d.crc }

// Sum returns hash value.
func (d *digest) Sum(in []byte) []byte {
	s := d.Sum64()
	return append(in, byte(s>>56), byte(s>>48), byte(s>>40), byte(s>>32), byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

// gf2Dim dimension of GF(2) vectors (length of CRC)
const gf2Dim int = 64

func gf2MatrixTimes(mat []uint64, vec uint64) uint64 {
	var sum uint64
	for i := 0; vec != 0; i++ {
		if vec&1 != 0 {
			sum ^= mat[i]
		}

		vec >>= 1
	}
	return sum
}

func gf2MatrixSquare(square []uint64, mat []uint64) {
	for n := 0; n < gf2Dim; n++ {
		square[n] = gf2MatrixTimes(mat, mat[n])
	}
}

// CRC64Combine combines CRC64
func CRC64Combine(crc1 uint64, crc2 uint64, len2 uint64) uint64 {
	var even [gf2Dim]uint64 // Even-power-of-two zeros operator
	var odd [gf2Dim]uint64  // Odd-power-of-two zeros operator

	// Degenerate case
	if len2 == 0 {
		return crc1
	}

	// Put operator for one zero bit in odd
	odd[0] = crc64.ECMA // CRC64 polynomial
	var row uint64 = 1
	for n := 1; n < gf2Dim; n++ {
		odd[n] = row
		row <<= 1
	}

	// Put operator for two zero bits in even
	gf2MatrixSquare(even[:], odd[:])

	// Put operator for four zero bits in odd
	gf2MatrixSquare(odd[:], even[:])

	// Apply len2 zeros to crc1, first square will put the operator for one zero byte, eight zero bits, in even
	for {
		// Apply zeros operator for this bit of len2
		gf2MatrixSquare(even[:], odd[:])

		if len2&1 != 0 {
			crc1 = gf2MatrixTimes(even[:], crc1)
		}

		len2 >>= 1

		// If no more bits set, then done
		if len2 == 0 {
			break
		}

		// Another iteration of the loop with odd and even swapped
		gf2MatrixSquare(odd[:], even[:])
		if len2&1 != 0 {
			crc1 = gf2MatrixTimes(odd[:], crc1)
		}
		len2 >>= 1

		// If no more bits set, then done
		if len2 == 0 {
			break
		}
	}

	// Return combined CRC
	crc1 ^= crc2
	return crc1
}
