// package common contains shared types (like Decoder, Encoder, Redundant, Segment)
// between delivery, server and related.

// This file contains the init() function for whole package.
package common

import "github.com/prometheus/prometheus/pp/go/common/internal"

func init() {
	internal.Init()
}

// EnableCoreDumps set fork-terminate on exceptions to get coredumps
func EnableCoreDumps() {
	internal.EnableCoreDumps(true)
}
