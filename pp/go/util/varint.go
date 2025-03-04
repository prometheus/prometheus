package util

// VarintLen returns how many bytes needed to store x as varint
func VarintLen(x uint64) (n int64) {
	for x >= 0x80 {
		x >>= 7
		n++
	}
	return n + 1
}
