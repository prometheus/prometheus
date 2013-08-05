// Copyright 2013 Prometheus Team
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

package raw

import (
	"github.com/prometheus/prometheus/utility"
)

type DatabaseState struct {
	Name string

	Size utility.ByteSize

	Location string
	Purpose  string

	Supplemental map[string]string
}

type DatabaseStates []*DatabaseState

func (s DatabaseStates) Len() int {
	return len(s)
}

func (s DatabaseStates) Less(i, j int) bool {
	l := s[i]
	r := s[j]

	if l.Name > r.Name {
		return false
	}
	if l.Name < r.Name {
		return true
	}

	return l.Size < r.Size
}

func (s DatabaseStates) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
