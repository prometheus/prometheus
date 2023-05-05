//go:build stringlabels

package labels

import (
	"github.com/cespare/xxhash/v2"
)

// StableHash is a labels hashing implementation which is guaranteed to not change over time.
// This function should be used whenever labels hashing backward compatibility must be guaranteed.
func StableHash(ls Labels) uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	var h *xxhash.Digest
	for i := 0; i < len(ls.data); {
		var v Label
		v.Name, i = decodeString(ls.data, i)
		v.Value, i = decodeString(ls.data, i)
		if h == nil && len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+, switch to Write API. Copy in the values up to this point.
			h = xxhash.New()
			_, _ = h.Write(b)
		}
		if h != nil {
			_, _ = h.WriteString(v.Name)
			_, _ = h.Write(seps)
			_, _ = h.WriteString(v.Value)
			_, _ = h.Write(seps)
			continue
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	if h != nil {
		return h.Sum64()
	}
	return xxhash.Sum64(b)
}
