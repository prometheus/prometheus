// Copyright 2025 The Prometheus Authors
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

// This file is copied from model/labels/labels_slicelabels.go and
// labels_common.go. Unused functions are removed.

package labels

import (
	"slices"
	"strings"
	"unsafe"

	"github.com/cespare/xxhash/v2"

	common "github.com/prometheus/prometheus/model/labels"
)

const sep = '\xff'     // Used between labels in `Bytes` and `Hash`.
var seps = []byte{sep} // Used with Hash, which has no WriteByte method.

// Labels is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type Labels []common.Label

func (ls Labels) Len() int           { return len(ls) }
func (ls Labels) Swap(i, j int)      { ls[i], ls[j] = ls[j], ls[i] }
func (ls Labels) Less(i, j int) bool { return ls[i].Name < ls[j].Name }

// Hash returns a hash value for the label set.
// Note: the result is not guaranteed to be consistent across different runs of Prometheus.
func (ls Labels) Hash() uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range ls {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range ls[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, sep)
		b = append(b, v.Value...)
		b = append(b, sep)
	}
	return xxhash.Sum64(b)
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	for _, l := range ls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// Has returns true if the label with the given name is present.
func (ls Labels) Has(name string) bool {
	for _, l := range ls {
		if l.Name == name {
			return true
		}
	}
	return false
}

// Equal returns whether the two label sets are equal.
func Equal(ls, o Labels) bool {
	return slices.Equal(ls, o)
}

// EmptyLabels returns n empty Labels value, for convenience.
func EmptyLabels() Labels {
	return Labels{}
}

// FromStrings creates new labels from pairs of strings.
func FromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 {
		panic("invalid number of strings")
	}
	res := make(Labels, 0, len(ss)/2)
	for i := 0; i < len(ss); i += 2 {
		res = append(res, common.Label{Name: ss[i], Value: ss[i+1]})
	}

	slices.SortFunc(res, func(a, b common.Label) int { return strings.Compare(a.Name, b.Name) })
	return res
}

// Range calls f on each label.
func (ls Labels) Range(f func(l common.Label)) {
	for _, l := range ls {
		f(l)
	}
}

// Builder allows modifying Labels.
type Builder struct {
	base Labels
	del  []string
	add  []common.Label
}

// NewBuilder returns a new LabelsBuilder.
func NewBuilder(base Labels) *Builder {
	b := &Builder{
		del: make([]string, 0, 5),
		add: make([]common.Label, 0, 5),
	}
	b.Reset(base)
	return b
}

// Reset clears all current state for the builder.
func (b *Builder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	b.base.Range(func(l common.Label) {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	})
}

// Del deletes the label of the given name.
func (b *Builder) Del(ns ...string) *Builder {
	for _, n := range ns {
		for i, a := range b.add {
			if a.Name == n {
				b.add = append(b.add[:i], b.add[i+1:]...)
			}
		}
		b.del = append(b.del, n)
	}
	return b
}

// Keep removes all labels from the base except those with the given names.
func (b *Builder) Keep(ns ...string) *Builder {
	b.base.Range(func(l common.Label) {
		if slices.Contains(ns, l.Name) {
			return
		}
		b.del = append(b.del, l.Name)
	})
	return b
}

// Set the name/value pair as a label. A value of "" means delete that label.
func (b *Builder) Set(n, v string) *Builder {
	if v == "" {
		// Empty labels are the same as missing labels.
		return b.Del(n)
	}
	for i, a := range b.add {
		if a.Name == n {
			b.add[i].Value = v
			return b
		}
	}
	b.add = append(b.add, common.Label{Name: n, Value: v})

	return b
}

func (b *Builder) Get(n string) string {
	// Del() removes entries from .add but Set() does not remove from .del, so check .add first.
	for _, a := range b.add {
		if a.Name == n {
			return a.Value
		}
	}
	if slices.Contains(b.del, n) {
		return ""
	}
	return b.base.Get(n)
}

// Range calls f on each label in the Builder.
func (b *Builder) Range(f func(l common.Label)) {
	// Stack-based arrays to avoid heap allocation in most cases.
	var addStack [128]common.Label
	var delStack [128]string
	// Take a copy of add and del, so they are unaffected by calls to Set() or Del().
	origAdd, origDel := append(addStack[:0], b.add...), append(delStack[:0], b.del...)
	b.base.Range(func(l common.Label) {
		if !slices.Contains(origDel, l.Name) && !common.Contains(origAdd, l.Name) {
			f(l)
		}
	})
	for _, a := range origAdd {
		f(a)
	}
}

// Labels returns the labels from the builder.
// If no modifications were made, the original labels are returned.
func (b *Builder) Labels() Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	expectedSize := len(b.base) + len(b.add) - len(b.del)
	if expectedSize < 1 {
		expectedSize = 1
	}
	res := make(Labels, 0, expectedSize)
	for _, l := range b.base {
		if slices.Contains(b.del, l.Name) || common.Contains(b.add, l.Name) {
			continue
		}
		res = append(res, l)
	}
	if len(b.add) > 0 { // Base is already in order, so we only need to sort if we add to it.
		res = append(res, b.add...)
		slices.SortFunc(res, func(a, b common.Label) int { return strings.Compare(a.Name, b.Name) })
	}
	return res
}

// ScratchBuilder allows efficient construction of a Labels from scratch.
type ScratchBuilder struct {
	add Labels
}

// SymbolTable is no-op, just for api parity with dedupelabels.
type SymbolTable struct{}

func NewSymbolTable() *SymbolTable { return nil }

func (*SymbolTable) Len() int { return 0 }

// NewScratchBuilder creates a ScratchBuilder initialized for Labels with n entries.
func NewScratchBuilder(n int) ScratchBuilder {
	return ScratchBuilder{add: make([]common.Label, 0, n)}
}

// NewBuilderWithSymbolTable creates a Builder, for api parity with dedupelabels.
func NewBuilderWithSymbolTable(_ *SymbolTable) *Builder {
	return NewBuilder(EmptyLabels())
}

// NewScratchBuilderWithSymbolTable creates a ScratchBuilder, for api parity with dedupelabels.
func NewScratchBuilderWithSymbolTable(_ *SymbolTable, n int) ScratchBuilder {
	return NewScratchBuilder(n)
}

func (*ScratchBuilder) SetSymbolTable(_ *SymbolTable) {
	// no-op
}

func (b *ScratchBuilder) Reset() {
	b.add = b.add[:0]
}

// Add a name/value pair.
// Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b *ScratchBuilder) Add(name, value string) {
	b.add = append(b.add, common.Label{Name: name, Value: value})
}

// UnsafeAddBytes adds a name/value pair, using []byte instead of string.
// The '-tags stringlabels' version of this function is unsafe, hence the name.
// This version is safe - it copies the strings immediately - but we keep the same name so everything compiles.
func (b *ScratchBuilder) UnsafeAddBytes(name, value []byte) {
	b.add = append(b.add, common.Label{Name: string(name), Value: string(value)})
}

// Sort the labels added so far by name.
func (b *ScratchBuilder) Sort() {
	slices.SortFunc(b.add, func(a, b common.Label) int { return strings.Compare(a.Name, b.Name) })
}

// Assign is for when you already have a Labels which you want this ScratchBuilder to return.
func (b *ScratchBuilder) Assign(ls Labels) {
	b.add = append(b.add[:0], ls...) // Copy on top of our slice, so we don't retain the input slice.
}

// Labels returns the name/value pairs added so far as a Labels object.
// Note: if you want them sorted, call Sort() first.
func (b *ScratchBuilder) Labels() Labels {
	// Copy the slice, so the next use of ScratchBuilder doesn't overwrite.
	return append([]common.Label{}, b.add...)
}

// Overwrite the newly-built Labels out to ls.
// Callers must ensure that there are no other references to ls, or any strings fetched from it.
func (b *ScratchBuilder) Overwrite(ls *Labels) {
	*ls = append((*ls)[:0], b.add...)
}

// SizeOfLabels returns the approximate space required for n copies of a label.
func SizeOfLabels(name, value string, n uint64) uint64 {
	return (uint64(len(name)) + uint64(unsafe.Sizeof(name)) + uint64(len(value)) + uint64(unsafe.Sizeof(value))) * n
}
