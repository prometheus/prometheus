// Copyright 2012 Prometheus Team
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

type Set map[interface{}]bool

func (s Set) Add(v interface{}) {
	s[v] = true
}

func (s Set) Remove(v interface{}) {
	delete(s, v)
}

func (s Set) Elements() []interface{} {
	result := make([]interface{}, 0, len(s))

	for k, _ := range s {
		result = append(result, k)
	}

	return result
}

func (s Set) Has(v interface{}) bool {
	_, p := s[v]

	return p
}

func (s Set) Intersection(o Set) Set {
	result := make(Set)

	for k, _ := range s {
		if o.Has(k) {
			result[k] = true
		}
	}

	return result
}
