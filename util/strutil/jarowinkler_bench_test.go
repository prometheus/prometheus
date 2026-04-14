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

package strutil

import "testing"

var benchCases = []struct {
	name, s1, s2 string
}{
	{"identical_short", "prometheus", "prometheus"},
	{"similar_short", "martha", "marhta"},
	{"dissimilar_short", "dixon", "dicksonx"},
	{"long_ascii", "http_requests_total_by_method_and_path", "http_requests_count_by_method_and_path"},
	{"unicode", "naïve", "naive"},
}

func BenchmarkJaroWinklerMatcher(b *testing.B) {
	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			m := NewJaroWinklerMatcher(bc.s1)
			for range b.N {
				m.Score(bc.s2)
			}
		})
	}
}
