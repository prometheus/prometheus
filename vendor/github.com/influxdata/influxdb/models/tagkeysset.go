package models

import (
	"bytes"
	"strings"
)

// TagKeysSet provides set operations for combining Tags.
type TagKeysSet struct {
	i    int
	keys [2][][]byte
	tmp  [][]byte
}

// Clear removes all the elements of TagKeysSet and ensures all internal
// buffers are reset.
func (set *TagKeysSet) Clear() {
	set.clear(set.keys[0])
	set.clear(set.keys[1])
	set.clear(set.tmp)
	set.i = 0
	set.keys[0] = set.keys[0][:0]
}

func (set *TagKeysSet) clear(b [][]byte) {
	b = b[:cap(b)]
	for i := range b {
		b[i] = nil
	}
}

// KeysBytes returns the merged keys in lexicographical order.
// The slice is valid until the next call to UnionKeys, UnionBytes or Reset.
func (set *TagKeysSet) KeysBytes() [][]byte {
	return set.keys[set.i&1]
}

// Keys returns a copy of the merged keys in lexicographical order.
func (set *TagKeysSet) Keys() []string {
	keys := set.KeysBytes()
	s := make([]string, 0, len(keys))
	for i := range keys {
		s = append(s, string(keys[i]))
	}
	return s
}

func (set *TagKeysSet) String() string {
	var s []string
	for _, k := range set.KeysBytes() {
		s = append(s, string(k))
	}
	return strings.Join(s, ",")
}

// IsSupersetKeys returns true if the TagKeysSet is a superset of all the keys
// contained in other.
func (set *TagKeysSet) IsSupersetKeys(other Tags) bool {
	keys := set.keys[set.i&1]
	i, j := 0, 0
	for i < len(keys) && j < len(other) {
		if cmp := bytes.Compare(keys[i], other[j].Key); cmp > 0 {
			return false
		} else if cmp == 0 {
			j++
		}
		i++
	}

	return j == len(other)
}

// IsSupersetBytes returns true if the TagKeysSet is a superset of all the keys
// in other.
// Other must be lexicographically sorted or the results are undefined.
func (set *TagKeysSet) IsSupersetBytes(other [][]byte) bool {
	keys := set.keys[set.i&1]
	i, j := 0, 0
	for i < len(keys) && j < len(other) {
		if cmp := bytes.Compare(keys[i], other[j]); cmp > 0 {
			return false
		} else if cmp == 0 {
			j++
		}
		i++
	}

	return j == len(other)
}

// UnionKeys updates the set so that it is the union of itself and all the
// keys contained in other.
func (set *TagKeysSet) UnionKeys(other Tags) {
	if set.IsSupersetKeys(other) {
		return
	}

	if l := len(other); cap(set.tmp) < l {
		set.tmp = make([][]byte, l)
	} else {
		set.tmp = set.tmp[:l]
	}

	for i := range other {
		set.tmp[i] = other[i].Key
	}

	set.merge(set.tmp)
}

// UnionBytes updates the set so that it is the union of itself and all the
// keys contained in other.
// Other must be lexicographically sorted or the results are undefined.
func (set *TagKeysSet) UnionBytes(other [][]byte) {
	if set.IsSupersetBytes(other) {
		return
	}

	set.merge(other)
}

func (set *TagKeysSet) merge(in [][]byte) {
	keys := set.keys[set.i&1]
	l := len(keys) + len(in)
	set.i = (set.i + 1) & 1
	keya := set.keys[set.i&1]
	if cap(keya) < l {
		keya = make([][]byte, 0, l)
	} else {
		keya = keya[:0]
	}

	i, j := 0, 0
	for i < len(keys) && j < len(in) {
		ki, kj := keys[i], in[j]
		if cmp := bytes.Compare(ki, kj); cmp < 0 {
			i++
		} else if cmp > 0 {
			ki = kj
			j++
		} else {
			i++
			j++
		}

		keya = append(keya, ki)
	}

	if i < len(keys) {
		keya = append(keya, keys[i:]...)
	} else if j < len(in) {
		keya = append(keya, in[j:]...)
	}

	set.keys[set.i&1] = keya
}
