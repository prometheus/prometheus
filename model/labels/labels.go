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

//go:build !slicelabels && !dedupelabels

package labels

import (
	"slices"
	"strings"
	"unsafe"

	"github.com/cespare/xxhash/v2"
)

// Labels is implemented by a single flat string holding name/value pairs.
// Each name and value is preceded by its length, encoded as a single byte
// for size 0-254, or the following 3 bytes little-endian, if the first byte is 255.
// Maximum length allowed is 2^24 or 16MB.
// Names are in order.
type Labels struct {
	data string
}

// Bytes returns an opaque, not-human-readable, encoding of ls, usable as a map key.
// Encoding may change over time or between runs of Prometheus.
func (ls Labels) Bytes(buf []byte) []byte {
	if cap(buf) < len(ls.data) {
		buf = make([]byte, len(ls.data))
	} else {
		buf = buf[:len(ls.data)]
	}
	copy(buf, ls.data)
	return buf
}

// IsZero implements yaml.IsZeroer - if we don't have this then 'omitempty' fields are always omitted.
func (ls Labels) IsZero() bool {
	return len(ls.data) == 0
}

// MatchLabels returns a subset of Labels that matches/does not match with the provided label names based on the 'on' boolean.
// If on is set to true, it returns the subset of labels that match with the provided label names and its inverse when 'on' is set to false.
// TODO: This is only used in printing an error message.
func (ls Labels) MatchLabels(on bool, names ...string) Labels {
	b := NewBuilder(ls)
	if on {
		b.Keep(names...)
	} else {
		b.Del(MetricName)
		b.Del(names...)
	}
	return b.Labels()
}

// Hash returns a hash value for the label set.
// Note: the result is not guaranteed to be consistent across different runs of Prometheus.
func (ls Labels) Hash() uint64 {
	return xxhash.Sum64(yoloBytes(ls.data))
}

// HashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashForLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	j := 0
	for i := 0; i < len(ls.data); {
		var name, value string
		name, i = decodeString(ls.data, i)
		value, i = decodeString(ls.data, i)
		for j < len(names) && names[j] < name {
			j++
		}
		if j == len(names) {
			break
		}
		if name == names[j] {
			b = append(b, name...)
			b = append(b, sep)
			b = append(b, value...)
			b = append(b, sep)
		}
	}

	return xxhash.Sum64(b), b
}

// HashWithoutLabels returns a hash value for all labels except those matching
// the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashWithoutLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	j := 0
	for i := 0; i < len(ls.data); {
		var name, value string
		name, i = decodeString(ls.data, i)
		value, i = decodeString(ls.data, i)
		for j < len(names) && names[j] < name {
			j++
		}
		if name == MetricName || (j < len(names) && name == names[j]) {
			continue
		}
		b = append(b, name...)
		b = append(b, sep)
		b = append(b, value...)
		b = append(b, sep)
	}
	return xxhash.Sum64(b), b
}

// BytesWithLabels is just as Bytes(), but only for labels matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) BytesWithLabels(buf []byte, names ...string) []byte {
	b := buf[:0]
	j := 0
	for pos := 0; pos < len(ls.data); {
		lName, newPos := decodeString(ls.data, pos)
		_, newPos = decodeString(ls.data, newPos)
		for j < len(names) && names[j] < lName {
			j++
		}
		if j == len(names) {
			break
		}
		if lName == names[j] {
			b = append(b, ls.data[pos:newPos]...)
		}
		pos = newPos
	}
	return b
}

// BytesWithoutLabels is just as Bytes(), but only for labels not matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) BytesWithoutLabels(buf []byte, names ...string) []byte {
	b := buf[:0]
	j := 0
	for pos := 0; pos < len(ls.data); {
		lName, newPos := decodeString(ls.data, pos)
		_, newPos = decodeString(ls.data, newPos)
		for j < len(names) && names[j] < lName {
			j++
		}
		if j == len(names) || lName != names[j] {
			b = append(b, ls.data[pos:newPos]...)
		}
		pos = newPos
	}
	return b
}

// Copy returns a copy of the labels.
func (ls Labels) Copy() Labels {
	return Labels{data: strings.Clone(ls.data)}
}

// HasDuplicateLabelNames returns whether ls has duplicate label names.
// It assumes that the labelset is sorted.
func (ls Labels) HasDuplicateLabelNames() (string, bool) {
	var lName, prevName string
	for i := 0; i < len(ls.data); {
		lName, i = decodeString(ls.data, i)
		_, i = decodeString(ls.data, i)
		if lName == prevName {
			return lName, true
		}
		prevName = lName
	}
	return "", false
}

// WithoutEmpty returns the labelset without empty labels.
// May return the same labelset.
func (ls Labels) WithoutEmpty() Labels {
	for pos := 0; pos < len(ls.data); {
		_, newPos := decodeString(ls.data, pos)
		lValue, newPos := decodeString(ls.data, newPos)
		if lValue != "" {
			pos = newPos
			continue
		}
		// Do not copy the slice until it's necessary.
		// TODO: could optimise the case where all blanks are at the end.
		// Note: we size the new buffer on the assumption there is exactly one blank value.
		buf := make([]byte, pos, pos+(len(ls.data)-newPos))
		copy(buf, ls.data[:pos]) // copy the initial non-blank labels
		pos = newPos             // move past the first blank value
		for pos < len(ls.data) {
			var newPos int
			_, newPos = decodeString(ls.data, pos)
			lValue, newPos = decodeString(ls.data, newPos)
			if lValue != "" {
				buf = append(buf, ls.data[pos:newPos]...)
			}
			pos = newPos
		}
		return Labels{data: yoloString(buf)}
	}
	return ls
}

// Equal returns whether the two label sets are equal.
func Equal(ls, o Labels) bool {
	return ls.data == o.data
}

// EmptyLabels returns an empty Labels value, for convenience.
func EmptyLabels() Labels {
	return Labels{}
}

func yoloBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// New returns a sorted Labels from the given labels.
// The caller has to guarantee that all label names are unique.
func New(ls ...Label) Labels {
	slices.SortFunc(ls, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })
	size := labelsSize(ls)
	buf := make([]byte, size)
	marshalLabelsToSizedBuffer(ls, buf)
	return Labels{data: yoloString(buf)}
}

// FromStrings creates new labels from pairs of strings.
func FromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 {
		panic("invalid number of strings")
	}
	ls := make([]Label, 0, len(ss)/2)
	for i := 0; i < len(ss); i += 2 {
		ls = append(ls, Label{Name: ss[i], Value: ss[i+1]})
	}

	return New(ls...)
}

// CopyFrom will copy labels from b on top of whatever was in ls previously, reusing memory or expanding if needed.
func (ls *Labels) CopyFrom(b Labels) {
	ls.data = b.data // strings are immutable
}

// IsEmpty returns true if ls represents an empty set of labels.
func (ls Labels) IsEmpty() bool {
	return len(ls.data) == 0
}

// Range calls f on each label.
func (ls Labels) Range(f func(l Label)) {
	for i := 0; i < len(ls.data); {
		var lName, lValue string
		lName, i = decodeString(ls.data, i)
		lValue, i = decodeString(ls.data, i)
		f(Label{Name: lName, Value: lValue})
	}
}

// Validate calls f on each label. If f returns a non-nil error, then it returns that error cancelling the iteration.
func (ls Labels) Validate(f func(l Label) error) error {
	for i := 0; i < len(ls.data); {
		var lName, lValue string
		lName, i = decodeString(ls.data, i)
		lValue, i = decodeString(ls.data, i)
		err := f(Label{Name: lName, Value: lValue})
		if err != nil {
			return err
		}
	}
	return nil
}

// InternStrings is a no-op because it would only save when the whole set of labels is identical.
func (ls *Labels) InternStrings(_ func(string) string) {
}

// ReleaseStrings is a no-op for the same reason as InternStrings.
func (ls Labels) ReleaseStrings(_ func(string)) {
}

// Builder allows modifying Labels.
type Builder struct {
	base Labels
	del  []string
	add  []Label
}

// Reset clears all current state for the builder.
func (b *Builder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	b.base.Range(func(l Label) {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	})
}

// Labels returns the labels from the builder.
// If no modifications were made, the original labels are returned.
func (b *Builder) Labels() Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	slices.SortFunc(b.add, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })
	slices.Sort(b.del)
	a, d := 0, 0

	bufSize := len(b.base.data) + labelsSize(b.add)
	buf := make([]byte, 0, bufSize)
	for pos := 0; pos < len(b.base.data); {
		oldPos := pos
		var lName string
		lName, pos = decodeString(b.base.data, pos)
		_, pos = decodeString(b.base.data, pos)
		for d < len(b.del) && b.del[d] < lName {
			d++
		}
		if d < len(b.del) && b.del[d] == lName {
			continue // This label has been deleted.
		}
		for ; a < len(b.add) && b.add[a].Name < lName; a++ {
			buf = appendLabelTo(buf, &b.add[a]) // Insert label that was not in the base set.
		}
		if a < len(b.add) && b.add[a].Name == lName {
			buf = appendLabelTo(buf, &b.add[a])
			a++
			continue // This label has been replaced.
		}
		buf = append(buf, b.base.data[oldPos:pos]...)
	}
	// We have come to the end of the base set; add any remaining labels.
	for ; a < len(b.add); a++ {
		buf = appendLabelTo(buf, &b.add[a])
	}
	return Labels{data: yoloString(buf)}
}

func marshalLabelsToSizedBuffer(lbls []Label, data []byte) int {
	i := len(data)
	for index := len(lbls) - 1; index >= 0; index-- {
		size := marshalLabelToSizedBuffer(&lbls[index], data[:i])
		i -= size
	}
	return len(data) - i
}

func sizeWhenEncoded(x uint64) (n int) {
	if x < 255 {
		return 1
	} else if x <= 1<<24 {
		return 4
	}
	panic("String too long to encode as label.")
}

func encodeSize(data []byte, offset, v int) int {
	if v < 255 {
		offset--
		data[offset] = uint8(v)
		return offset
	}
	offset -= 4
	data[offset] = 255
	data[offset+1] = byte(v)
	data[offset+2] = byte((v >> 8))
	data[offset+3] = byte((v >> 16))
	return offset
}

func labelsSize(lbls []Label) (n int) {
	// we just encode name/value/name/value, without any extra tags or length bytes
	for _, e := range lbls {
		n += labelSize(&e)
	}
	return n
}

func appendLabelTo(buf []byte, m *Label) []byte {
	size := labelSize(m)
	sizeRequired := len(buf) + size
	if cap(buf) >= sizeRequired {
		buf = buf[:sizeRequired]
	} else {
		bufSize := cap(buf)
		// Double size of buffer each time it needs to grow, to amortise copying cost.
		for bufSize < sizeRequired {
			bufSize = bufSize*2 + 1
		}
		newBuf := make([]byte, sizeRequired, bufSize)
		copy(newBuf, buf)
		buf = newBuf
	}
	marshalLabelToSizedBuffer(m, buf)
	return buf
}

// ScratchBuilder allows efficient construction of a Labels from scratch.
type ScratchBuilder struct {
	add             []Label
	output          Labels
	overwriteBuffer []byte
}

// NewScratchBuilder creates a ScratchBuilder initialized for Labels with n entries.
func NewScratchBuilder(n int) ScratchBuilder {
	return ScratchBuilder{add: make([]Label, 0, n)}
}

func (b *ScratchBuilder) Reset() {
	b.add = b.add[:0]
	b.output = EmptyLabels()
}

// Add a name/value pair.
// Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b *ScratchBuilder) Add(name, value string) {
	b.add = append(b.add, Label{Name: name, Value: value})
}

// UnsafeAddBytes adds a name/value pair using []byte instead of string to reduce memory allocations.
// The values must remain live until Labels() is called.
func (b *ScratchBuilder) UnsafeAddBytes(name, value []byte) {
	b.add = append(b.add, Label{Name: yoloString(name), Value: yoloString(value)})
}

// Sort the labels added so far by name.
func (b *ScratchBuilder) Sort() {
	slices.SortFunc(b.add, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })
}

// Assign is for when you already have a Labels which you want this ScratchBuilder to return.
func (b *ScratchBuilder) Assign(l Labels) {
	b.output = l
}

// Labels returns the name/value pairs added as a Labels object. Calling Add() after Labels() has no effect.
// Note: if you want them sorted, call Sort() first.
func (b *ScratchBuilder) Labels() Labels {
	if b.output.IsEmpty() {
		size := labelsSize(b.add)
		buf := make([]byte, size)
		marshalLabelsToSizedBuffer(b.add, buf)
		b.output = Labels{data: yoloString(buf)}
	}
	return b.output
}

// Overwrite will write the newly-built Labels out to ls, reusing an internal buffer.
// Callers must ensure that there are no other references to ls, or any strings fetched from it.
func (b *ScratchBuilder) Overwrite(ls *Labels) {
	size := labelsSize(b.add)
	if size <= cap(b.overwriteBuffer) {
		b.overwriteBuffer = b.overwriteBuffer[:size]
	} else {
		b.overwriteBuffer = make([]byte, size)
	}
	marshalLabelsToSizedBuffer(b.add, b.overwriteBuffer)
	ls.data = yoloString(b.overwriteBuffer)
}

// SymbolTable is no-op, just for api parity with dedupelabels.
type SymbolTable struct{}

func NewSymbolTable() *SymbolTable { return nil }

func (t *SymbolTable) Len() int { return 0 }

// NewBuilderWithSymbolTable creates a Builder, for api parity with dedupelabels.
func NewBuilderWithSymbolTable(_ *SymbolTable) *Builder {
	return NewBuilder(EmptyLabels())
}

// NewScratchBuilderWithSymbolTable creates a ScratchBuilder, for api parity with dedupelabels.
func NewScratchBuilderWithSymbolTable(_ *SymbolTable, n int) ScratchBuilder {
	return NewScratchBuilder(n)
}

func (b *ScratchBuilder) SetSymbolTable(_ *SymbolTable) {
	// no-op
}

// SizeOfLabels returns the approximate space required for n copies of a label.
func SizeOfLabels(name, value string, n uint64) uint64 {
	return uint64(labelSize(&Label{Name: name, Value: value})) * n
}
