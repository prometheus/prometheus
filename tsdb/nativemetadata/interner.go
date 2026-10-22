// Copyright The Prometheus Authors
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

package nativemetadata

import "github.com/prometheus/prometheus/model/metadata"

// Interner shares equal values within one reduction, such as a checkpoint or a
// replay. It retains at most limit values; later new values are not shared.
// It is not safe for concurrent use.
type Interner struct {
	values map[metadata.Metadata]*metadata.Metadata
	limit  int
}

// NewInterner returns an interner that retains at most limit values.
func NewInterner(limit int) *Interner {
	return &Interner{values: map[metadata.Metadata]*metadata.Metadata{}, limit: limit}
}

// Intern returns an immutable value equal to m, retaining m's strings if it is new.
func (i *Interner) Intern(m metadata.Metadata) *metadata.Metadata {
	if v, ok := i.values[m]; ok {
		return v
	}
	// Copy m here: taking m's address would move it to the heap on entry,
	// allocating on every hit.
	owned := m
	v := &owned
	if len(i.values) < i.limit {
		i.values[m] = v
	}
	return v
}
