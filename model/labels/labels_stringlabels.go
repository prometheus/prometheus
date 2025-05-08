// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !slicelabels && !dedupelabels && !toplabels

package labels

import "unsafe"

func decodeSize(data string, index int) (int, int) {
	b := data[index]
	index++
	if b == 255 {
		// Larger numbers are encoded as 3 bytes little-endian.
		// Just panic if we go of the end of data, since all Labels strings are constructed internally and
		// malformed data indicates a bug, or memory corruption.
		return int(data[index]) + (int(data[index+1]) << 8) + (int(data[index+2]) << 16), index + 3
	}
	// More common case of a single byte, value 0..254.
	return int(b), index
}

func decodeString(data string, index int) (string, int) {
	var size int
	size, index = decodeSize(data, index)
	return data[index : index+size], index + size
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	if name == "" { // Avoid crash in loop if someone asks for "".
		return "" // Prometheus does not store blank label names.
	}
	for i := 0; i < len(ls.data); {
		var size int
		size, i = decodeSize(ls.data, i)
		if ls.data[i] == name[0] {
			lName := ls.data[i : i+size]
			i += size
			if lName == name {
				lValue, _ := decodeString(ls.data, i)
				return lValue
			}
		} else {
			if ls.data[i] > name[0] { // Stop looking if we've gone past.
				break
			}
			i += size
		}
		size, i = decodeSize(ls.data, i)
		i += size
	}
	return ""
}

// Has returns true if the label with the given name is present.
func (ls Labels) Has(name string) bool {
	if name == "" { // Avoid crash in loop if someone asks for "".
		return false // Prometheus does not store blank label names.
	}
	for i := 0; i < len(ls.data); {
		var size int
		size, i = decodeSize(ls.data, i)
		if ls.data[i] == name[0] {
			lName := ls.data[i : i+size]
			i += size
			if lName == name {
				return true
			}
		} else {
			if ls.data[i] > name[0] { // Stop looking if we've gone past.
				break
			}
			i += size
		}
		size, i = decodeSize(ls.data, i)
		i += size
	}
	return false
}

// Compare compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func Compare(a, b Labels) int {
	// Find the first byte in the string where a and b differ.
	shorter, longer := a.data, b.data
	if len(b.data) < len(a.data) {
		shorter, longer = b.data, a.data
	}
	i := 0
	// First, go 8 bytes at a time. Data strings are expected to be 8-byte aligned.
	sp := unsafe.Pointer(unsafe.StringData(shorter))
	lp := unsafe.Pointer(unsafe.StringData(longer))
	for ; i < len(shorter)-8; i += 8 {
		if *(*uint64)(unsafe.Add(sp, i)) != *(*uint64)(unsafe.Add(lp, i)) {
			break
		}
	}
	// Now go 1 byte at a time.
	for ; i < len(shorter); i++ {
		if shorter[i] != longer[i] {
			break
		}
	}
	if i == len(shorter) {
		// One Labels was a prefix of the other; the set with fewer labels compares lower.
		return len(a.data) - len(b.data)
	}

	// Now we know that there is some difference before the end of a and b.
	// Go back through the fields and find which field that difference is in.
	firstCharDifferent, i := i, 0
	size, nextI := decodeSize(a.data, i)
	for nextI+size <= firstCharDifferent {
		i = nextI + size
		size, nextI = decodeSize(a.data, i)
	}
	// Difference is inside this entry.
	aStr, _ := decodeString(a.data, i)
	bStr, _ := decodeString(b.data, i)
	if aStr < bStr {
		return -1
	}
	return +1
}

// Len returns the number of labels; it is relatively slow.
func (ls Labels) Len() int {
	count := 0
	for i := 0; i < len(ls.data); {
		var size int
		size, i = decodeSize(ls.data, i)
		i += size
		size, i = decodeSize(ls.data, i)
		i += size
		count++
	}
	return count
}

// DropMetricName returns Labels with "__name__" removed.
func (ls Labels) DropMetricName() Labels {
	for i := 0; i < len(ls.data); {
		lName, i2 := decodeString(ls.data, i)
		size, i2 := decodeSize(ls.data, i2)
		i2 += size
		if lName == MetricName {
			if i == 0 { // Make common case fast with no allocations.
				ls.data = ls.data[i2:]
			} else {
				ls.data = ls.data[:i] + ls.data[i2:]
			}
			break
		} else if lName[0] > MetricName[0] { // Stop looking if we've gone past.
			break
		}
		i = i2
	}
	return ls
}

func marshalLabelToSizedBuffer(m *Label, data []byte) int {
	i := len(data)
	i -= len(m.Value)
	copy(data[i:], m.Value)
	i = encodeSize(data, i, len(m.Value))
	i -= len(m.Name)
	copy(data[i:], m.Name)
	i = encodeSize(data, i, len(m.Name))
	return len(data) - i
}

func labelSize(m *Label) (n int) {
	// strings are encoded as length followed by contents.
	l := len(m.Name)
	n += l + sizeWhenEncoded(uint64(l))
	l = len(m.Value)
	n += l + sizeWhenEncoded(uint64(l))
	return n
}
