// +build amd64,!appengine,!popcntgo

package bits

// Popcnt counts the number of bits set
func Popcnt(x uint64) uint64
