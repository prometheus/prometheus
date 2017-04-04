// +build !amd64 appengine

package bits

// Ctz counts trailing zeroes
func Ctz(x uint64) uint64 {

	if x == 0 {
		return 64
	}

	var n uint64

	if (x & 0x00000000FFFFFFFF) == 0 {
		n = n + 32
		x = x >> 32
	}
	if (x & 0x000000000000FFFF) == 0 {
		n = n + 16
		x = x >> 16
	}
	if (x & 0x00000000000000FF) == 0 {
		n = n + 8
		x = x >> 8
	}
	if (x & 0x000000000000000F) == 0 {
		n = n + 4
		x = x >> 4
	}
	if (x & 0x0000000000000003) == 0 {
		n = n + 2
		x = x >> 2
	}
	if (x & 0x0000000000000001) == 0 {
		n = n + 1
	}

	return n
}
