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

package utility

import (
	"sort"
)

// ReverseSorter embeds a sort.Interface value and implements a reverse sort
// over that value.
type ReverseSorter struct {
	// This embedded interface permits ReverseSorter to use the methods of
	// another Interface implementation.
	sort.Interface
}

// Less returns the opposite of the embedded implementation's Less method.
func (r ReverseSorter) Less(i, j int) bool {
	return r.Interface.Less(j, i)
}
