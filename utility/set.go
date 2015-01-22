// Copyright 2013 The Prometheus Authors
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

package utility

// Set is a type which models a set.
type Set map[interface{}]struct{}

// Add adds an item to the set.
func (s Set) Add(v interface{}) {
	s[v] = struct{}{}
}

// Remove removes an item from the set.
func (s Set) Remove(v interface{}) {
	delete(s, v)
}

// Elements returns a slice containing all elements in the set.
func (s Set) Elements() []interface{} {
	result := make([]interface{}, 0, len(s))

	for k := range s {
		result = append(result, k)
	}

	return result
}

// Has returns true if an element is contained in the set.
func (s Set) Has(v interface{}) bool {
	_, p := s[v]

	return p
}

// Intersection returns a new set with items that exist in both sets.
func (s Set) Intersection(o Set) Set {
	result := Set{}

	for k := range s {
		if o.Has(k) {
			result[k] = struct{}{}
		}
	}

	return result
}
