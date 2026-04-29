// Copyright 2026 The Prometheus Authors
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

package stateset

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// MaxStates is the maximum number of states a StateSet may have.
// Limited by the width of the Values bitset.
const MaxStates = 64

// ErrTooManyStates is returned when a StateSet exceeds MaxStates entries.
var ErrTooManyStates = fmt.Errorf("stateset exceeds maximum of %d states", MaxStates)

// StateSet represents a single native stateset sample. LabelName is the label
// name used to carry state values in the legacy OM1 wire format (e.g.
// "kube_pod_status_phase"). It is stored in chunk metadata and must not appear
// in the TSDB series labelset. Names must be sorted lexicographically and
// contain at most MaxStates entries. Bit i of Values is set if Names[i] is an
// active state.
type StateSet struct {
	LabelName string
	Names     []string
	Values    uint64
}

// String returns a human-readable representation of the stateset in the
// promqltest load/eval syntax: ss(labelName, state1=0 state2=1 ...).
func (s *StateSet) String() string {
	if s == nil {
		return "ss()"
	}
	var b strings.Builder
	b.WriteString("ss(")
	b.WriteString(s.LabelName)
	b.WriteString(",")
	for i, n := range s.Names {
		b.WriteByte(' ')
		b.WriteString(n)
		b.WriteByte('=')
		if s.Values>>uint(i)&1 == 1 {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	b.WriteByte(')')
	return b.String()
}

// Validate reports whether s satisfies all invariants: LabelName non-empty,
// Names sorted and within MaxStates, Values has no bits set beyond len(Names).
func (s *StateSet) Validate() error {
	if s.LabelName == "" {
		return errors.New("stateset LabelName must not be empty")
	}
	if len(s.Names) > MaxStates {
		return ErrTooManyStates
	}
	if !sort.StringsAreSorted(s.Names) {
		return errors.New("stateset Names must be sorted lexicographically")
	}
	if len(s.Names) < MaxStates && s.Values>>uint(len(s.Names)) != 0 {
		return errors.New("stateset Values has bits set beyond len(Names)")
	}
	return nil
}

// IsActive reports whether the named state is currently active.
func (s *StateSet) IsActive(name string) bool {
	i, ok := s.indexOf(name)
	if !ok {
		return false
	}
	return s.Values>>uint(i)&1 == 1
}

// ActiveNames returns the names of all currently active states in the order
// they appear in Names.
func (s *StateSet) ActiveNames() []string {
	out := make([]string, 0, len(s.Names))
	for i, n := range s.Names {
		if s.Values>>uint(i)&1 == 1 {
			out = append(out, n)
		}
	}
	return out
}

// Copy returns a deep copy of s.
func (s *StateSet) Copy() *StateSet {
	c := &StateSet{
		LabelName: s.LabelName,
		Names:     make([]string, len(s.Names)),
		Values:    s.Values,
	}
	copy(c.Names, s.Names)
	return c
}

// Equals reports whether s and o represent the same stateset sample.
func (s *StateSet) Equals(o *StateSet) bool {
	if s.LabelName != o.LabelName || s.Values != o.Values || len(s.Names) != len(o.Names) {
		return false
	}
	for i := range s.Names {
		if s.Names[i] != o.Names[i] {
			return false
		}
	}
	return true
}

// NamesMatch reports whether s and o have identical Names slices, i.e. the
// same set of possible states in the same order. This is used to decide
// whether a new chunk must be started (recode) when a sample arrives.
func (s *StateSet) NamesMatch(o *StateSet) bool {
	if len(s.Names) != len(o.Names) {
		return false
	}
	for i := range s.Names {
		if s.Names[i] != o.Names[i] {
			return false
		}
	}
	return true
}

// indexOf returns the position of name in s.Names using binary search, and
// whether it was found.
func (s *StateSet) indexOf(name string) (int, bool) {
	i, found := sort.Find(len(s.Names), func(i int) int {
		if name < s.Names[i] {
			return -1
		}
		if name > s.Names[i] {
			return 1
		}
		return 0
	})
	return i, found
}

// FromActive constructs a StateSet from a labelName, an ordered list of all
// known state names, and a set of active state names. activeNames need not be
// sorted. Returns ErrTooManyStates if allNames exceeds MaxStates.
func FromActive(labelName string, allNames []string, activeNames map[string]struct{}) (*StateSet, error) {
	if len(allNames) > MaxStates {
		return nil, ErrTooManyStates
	}
	sorted := make([]string, len(allNames))
	copy(sorted, allNames)
	sort.Strings(sorted)

	ss := &StateSet{
		LabelName: labelName,
		Names:     sorted,
	}
	for i, n := range sorted {
		if _, ok := activeNames[n]; ok {
			ss.Values |= 1 << uint(i)
		}
	}
	return ss, nil
}
