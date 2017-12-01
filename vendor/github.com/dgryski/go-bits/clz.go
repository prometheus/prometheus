// +build !amd64 appengine

package bits

// Clz counts leading zeroes
func Clz(x uint64) uint64 {
	var n uint64

	n = 1

	if (x >> 32) == 0 {
		n = n + 32
		x = x << 32
	}
	if (x >> (32 + 16)) == 0 {
		n = n + 16
		x = x << 16
	}

	if (x >> (32 + 16 + 8)) == 0 {
		n = n + 8
		x = x << 8
	}

	if (x >> (32 + 16 + 8 + 4)) == 0 {
		n = n + 4
		x = x << 4
	}

	if (x >> (32 + 16 + 8 + 4 + 2)) == 0 {
		n = n + 2
		x = x << 2
	}

	n = n - (x >> 63)
	return uint64(n)
}
