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

package labels

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/common/model"
)

// Well-known label names used by Prometheus components.
const (
	MetricName   = "__name__"
	AlertName    = "alertname"
	BucketLabel  = "le"
	InstanceName = "instance"

	labelSep = '\xfe'
)

var seps = []byte{'\xff'}

// Label is a key/value pair of strings.
type Label struct {
	Name, Value string
}

// Labels is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type Labels struct {
	lbls []Label
}

func (ls Labels) Len() int           { return len(ls.lbls) }
func (ls Labels) Swap(i, j int)      { ls.lbls[i], ls.lbls[j] = ls.lbls[j], ls.lbls[i] }
func (ls Labels) Less(i, j int) bool { return ls.lbls[i].Name < ls.lbls[j].Name }

func (ls Labels) String() string {
	var b bytes.Buffer

	b.WriteByte('{')
	for i, l := range ls.lbls {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(l.Value))
	}
	b.WriteByte('}')
	return b.String()
}

// Bytes returns ls as a byte slice.
// It uses an byte invalid character as a separator and so should not be used for printing.
func (ls Labels) Bytes(buf []byte) []byte {
	b := bytes.NewBuffer(buf[:0])
	b.WriteByte(labelSep)
	for i, l := range ls.lbls {
		if i > 0 {
			b.WriteByte(seps[0])
		}
		b.WriteString(l.Name)
		b.WriteByte(seps[0])
		b.WriteString(l.Value)
	}
	return b.Bytes()
}

// MarshalJSON implements json.Marshaler.
func (ls Labels) MarshalJSON() ([]byte, error) {
	return json.Marshal(ls.Map())
}

// UnmarshalJSON implements json.Unmarshaler.
func (ls *Labels) UnmarshalJSON(b []byte) error {
	var m map[string]string

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (ls Labels) MarshalYAML() (interface{}, error) {
	return ls.Map(), nil
}

// IsZero implements yaml.IsZeroer - if we don't have this then 'omitempty' fields are always omitted.
func (ls Labels) IsZero() bool {
	return len(ls.lbls) == 0
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (ls *Labels) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]string

	if err := unmarshal(&m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// MatchLabels returns a subset of Labels that matches/does not match with the provided label names based on the 'on' boolean.
// If on is set to true, it returns the subset of labels that match with the provided label names and its inverse when 'on' is set to false.
func (ls Labels) MatchLabels(on bool, names ...string) Labels {
	matchedLabels := Labels{}

	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}

	for _, v := range ls.lbls {
		if _, ok := nameSet[v.Name]; on == ok && (on || v.Name != MetricName) {
			matchedLabels.lbls = append(matchedLabels.lbls, v)
		}
	}

	return matchedLabels
}

// Hash returns a hash value for the label set.
// Note: the result is not guaranteed to be consistent across different runs of Prometheus.
func (ls Labels) Hash() uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range ls.lbls {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range ls.lbls[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

// HashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashForLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	i, j := 0, 0
	for i < len(ls.lbls) && j < len(names) {
		if names[j] < ls.lbls[i].Name {
			j++
		} else if ls.lbls[i].Name < names[j] {
			i++
		} else {
			b = append(b, ls.lbls[i].Name...)
			b = append(b, seps[0])
			b = append(b, ls.lbls[i].Value...)
			b = append(b, seps[0])
			i++
			j++
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
	for i := range ls.lbls {
		for j < len(names) && names[j] < ls.lbls[i].Name {
			j++
		}
		if ls.lbls[i].Name == MetricName || (j < len(names) && ls.lbls[i].Name == names[j]) {
			continue
		}
		b = append(b, ls.lbls[i].Name...)
		b = append(b, seps[0])
		b = append(b, ls.lbls[i].Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b), b
}

// BytesWithLabels is just as Bytes(), but only for labels matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) BytesWithLabels(buf []byte, names ...string) []byte {
	b := bytes.NewBuffer(buf[:0])
	b.WriteByte(labelSep)
	i, j := 0, 0
	for i < len(ls.lbls) && j < len(names) {
		if names[j] < ls.lbls[i].Name {
			j++
		} else if ls.lbls[i].Name < names[j] {
			i++
		} else {
			if b.Len() > 1 {
				b.WriteByte(seps[0])
			}
			b.WriteString(ls.lbls[i].Name)
			b.WriteByte(seps[0])
			b.WriteString(ls.lbls[i].Value)
			i++
			j++
		}
	}
	return b.Bytes()
}

// BytesWithoutLabels is just as Bytes(), but only for labels not matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) BytesWithoutLabels(buf []byte, names ...string) []byte {
	b := bytes.NewBuffer(buf[:0])
	b.WriteByte(labelSep)
	j := 0
	for i := range ls.lbls {
		for j < len(names) && names[j] < ls.lbls[i].Name {
			j++
		}
		if j < len(names) && ls.lbls[i].Name == names[j] {
			continue
		}
		if b.Len() > 1 {
			b.WriteByte(seps[0])
		}
		b.WriteString(ls.lbls[i].Name)
		b.WriteByte(seps[0])
		b.WriteString(ls.lbls[i].Value)
	}
	return b.Bytes()
}

// Copy returns a copy of the labels.
func (ls Labels) Copy() Labels {
	res := Labels{lbls: make([]Label, len(ls.lbls))}
	copy(res.lbls, ls.lbls)
	return res
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	for _, l := range ls.lbls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// Has returns true if the label with the given name is present.
func (ls Labels) Has(name string) bool {
	for _, l := range ls.lbls {
		if l.Name == name {
			return true
		}
	}
	return false
}

// HasDuplicateLabelNames returns whether ls has duplicate label names.
// It assumes that the labelset is sorted.
func (ls Labels) HasDuplicateLabelNames() (string, bool) {
	for i, l := range ls.lbls {
		if i == 0 {
			continue
		}
		if l.Name == ls.lbls[i-1].Name {
			return l.Name, true
		}
	}
	return "", false
}

// WithoutEmpty returns the labelset without empty labels.
// May return the same labelset.
func (ls Labels) WithoutEmpty() Labels {
	for _, v := range ls.lbls {
		if v.Value != "" {
			continue
		}
		// Do not copy the slice until it's necessary.
		els := Labels{lbls: make([]Label, 0, len(ls.lbls)-1)}
		for _, v := range ls.lbls {
			if v.Value != "" {
				els.lbls = append(els.lbls, v)
			}
		}
		return els
	}
	return ls
}

// IsValid checks if the metric name or label names are valid.
func (ls Labels) IsValid() bool {
	for _, l := range ls {
		if l.Name == model.MetricNameLabel && !model.IsValidMetricName(model.LabelValue(l.Value)) {
			return false
		}
		if !model.LabelName(l.Name).IsValid() || !model.LabelValue(l.Value).IsValid() {
			return false
		}
	}
	return true
}

// Equal returns whether the two label sets are equal.
func Equal(ls, o Labels) bool {
	if len(ls.lbls) != len(o.lbls) {
		return false
	}
	for i, l := range ls.lbls {
		if l != o.lbls[i] {
			return false
		}
	}
	return true
}

// Map returns a string map of the labels.
func (ls Labels) Map() map[string]string {
	m := make(map[string]string, len(ls.lbls))
	for _, l := range ls.lbls {
		m[l.Name] = l.Value
	}
	return m
}

// EmptyLabels returns an empty Labels value, for convenience.
func EmptyLabels() Labels {
	return Labels{lbls: []Label{}}
}

// New returns a sorted Labels from the given labels.
// The caller has to guarantee that all label names are unique.
func New(ls ...Label) Labels {
	set := Labels{lbls: make([]Label, 0, len(ls))}
	set.lbls = append(set.lbls, ls...)
	sort.Sort(set)

	return set
}

// FromMap returns new sorted Labels from the given map.
func FromMap(m map[string]string) Labels {
	l := make([]Label, 0, len(m))
	for k, v := range m {
		l = append(l, Label{Name: k, Value: v})
	}
	return New(l...)
}

// FromStrings creates new labels from pairs of strings.
func FromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 {
		panic("invalid number of strings")
	}
	res := Labels{lbls: make([]Label, 0, len(ss)/2)}
	for i := 0; i < len(ss); i += 2 {
		res.lbls = append(res.lbls, Label{Name: ss[i], Value: ss[i+1]})
	}

	sort.Sort(res)
	return res
}

// Compare compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func Compare(a, b Labels) int {
	l := len(a.lbls)
	if len(b.lbls) < l {
		l = len(b.lbls)
	}

	for i := 0; i < l; i++ {
		if a.lbls[i].Name != b.lbls[i].Name {
			if a.lbls[i].Name < b.lbls[i].Name {
				return -1
			}
			return 1
		}
		if a.lbls[i].Value != b.lbls[i].Value {
			if a.lbls[i].Value < b.lbls[i].Value {
				return -1
			}
			return 1
		}
	}
	// If all labels so far were in common, the set with fewer labels comes first.
	return len(a.lbls) - len(b.lbls)
}

// Copy labels from b on top of whatever was in ls previously, reusing memory or expanding if needed.
func (ls *Labels) CopyFrom(b Labels) {
	ls.lbls = append(ls.lbls[:0], b.lbls...)
}

// IsEmpty returns true if ls represents an empty set of labels.
func (ls Labels) IsEmpty() bool {
	return len(ls.lbls) == 0
}

// Range calls f on each label.
func (ls Labels) Range(f func(l Label)) {
	for _, l := range ls.lbls {
		f(l)
	}
}

// Validate calls f on each label. If f returns a non-nil error, then it returns that error cancelling the iteration.
func (ls Labels) Validate(f func(l Label) error) error {
	for _, l := range ls.lbls {
		if err := f(l); err != nil {
			return err
		}
	}
	return nil
}

// InternStrings calls intern on every string value inside ls, replacing them with what it returns.
func (ls *Labels) InternStrings(intern func(string) string) {
	for i, l := range ls.lbls {
		ls.lbls[i].Name = intern(l.Name)
		ls.lbls[i].Value = intern(l.Value)
	}
}

// ReleaseStrings calls release on every string value inside ls.
func (ls Labels) ReleaseStrings(release func(string)) {
	for _, l := range ls.lbls {
		release(l.Name)
		release(l.Value)
	}
}

// Builder allows modifying Labels.
type Builder struct {
	base Labels
	del  []string
	add  []Label
}

// NewBuilder returns a new LabelsBuilder.
func NewBuilder(base Labels) *Builder {
	b := &Builder{
		del: make([]string, 0, 5),
		add: make([]Label, 0, 5),
	}
	b.Reset(base)
	return b
}

// Reset clears all current state for the builder.
func (b *Builder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	for _, l := range b.base.lbls {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	}
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
Outer:
	for _, l := range b.base.lbls {
		for _, n := range ns {
			if l.Name == n {
				continue Outer
			}
		}
		b.del = append(b.del, l.Name)
	}
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
	b.add = append(b.add, Label{Name: n, Value: v})

	return b
}

// Labels returns the labels from the builder, adding them to res if non-nil.
// Argument res can be the same as b.base, if caller wants to overwrite that slice.
// If no modifications were made, the original labels are returned.
func (b *Builder) Labels(res Labels) Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	if res.lbls == nil {
		// In the general case, labels are removed, modified or moved
		// rather than added.
		res.lbls = make([]Label, 0, len(b.base.lbls))
	} else {
		res.lbls = res.lbls[:0]
	}
Outer:
	// Justification that res can be the same slice as base: in this loop
	// we move forward through base, and either skip an element or assign
	// it to res at its current position or an earlier position.
	for _, l := range b.base.lbls {
		for _, n := range b.del {
			if l.Name == n {
				continue Outer
			}
		}
		for _, la := range b.add {
			if l.Name == la.Name {
				continue Outer
			}
		}
		res.lbls = append(res.lbls, l)
	}
	if len(b.add) > 0 { // Base is already in order, so we only need to sort if we add to it.
		res.lbls = append(res.lbls, b.add...)
		sort.Sort(res)
	}
	return res
}

// ScratchBuilder allows efficient construction of a Labels from scratch.
type ScratchBuilder struct {
	add Labels
}

// NewScratchBuilder creates a ScratchBuilder initialized for Labels with n entries.
func NewScratchBuilder(n int) ScratchBuilder {
	return ScratchBuilder{add: Labels{lbls: make([]Label, 0, n)}}
}

func (b *ScratchBuilder) Reset() {
	b.add.lbls = b.add.lbls[:0]
}

// Add a name/value pair.
// Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b *ScratchBuilder) Add(name, value string) {
	b.add.lbls = append(b.add.lbls, Label{Name: name, Value: value})
}

// Sort the labels added so far by name.
func (b *ScratchBuilder) Sort() {
	sort.Sort(b.add)
}

// Return the name/value pairs added so far as a Labels object.
// Note: if you want them sorted, call Sort() first.
func (b *ScratchBuilder) Labels() Labels {
	// Copy the slice, so the next use of SimpleBuilder doesn't overwrite.
	return Labels{lbls: append([]Label{}, b.add.lbls...)}
}

// Write the newly-built Labels out to ls, reusing its buffer if long enough.
// Callers must ensure that there are no other references to ls.
func (b *ScratchBuilder) Overwrite(ls *Labels) {
	ls.lbls = append(ls.lbls[:0], b.add.lbls...)
}
