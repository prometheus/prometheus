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

import (
	"strings"
	"testing"
)

var benchCases = []struct {
	name, s1, s2 string
}{
	{"identical_short", "prometheus", "prometheus"},
	{"similar_short", "martha", "marhta"},
	{"dissimilar_short", "dixon", "dicksonx"},
	{"long_ascii", "http_requests_total_by_method_and_path", "http_requests_count_by_method_and_path"},
	{"short_term_medium_candidate", "prometheus", "prometheus_http_requests_total_by_handler_code"},
	{"two_kilobyte_candidate", "search-term", strings.Repeat("candidate", 226)},
	{"unicode", "naïve", "naive"},
	{"ascii_term_unicode_candidate", "prometheus", "prométheus"},
	{"long_ascii_term_unicode_candidate", strings.Repeat("a", 40), strings.Repeat("a", 39) + "é"},
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

var benchmarkMatcher *JaroWinklerMatcher

func BenchmarkNewJaroWinklerMatcher(b *testing.B) {
	for _, bc := range []struct {
		name string
		term string
	}{
		{name: "short_ASCII", term: "prometheus"},
		{name: "long_ASCII", term: strings.Repeat("a", 40)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			for range b.N {
				benchmarkMatcher = NewJaroWinklerMatcher(bc.term)
			}
		})
	}
}

func BenchmarkJaroWinklerMatcherParallel(b *testing.B) {
	m := NewJaroWinklerMatcher("prometheus")
	candidate := "promethus" + strings.Repeat("x", 128)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Score(candidate)
		}
	})
}

// BenchmarkJaroWinklerMatcherManyCandidates models one search: a single matcher
// scoring 10,000 long label values. Before pooling, each score allocated match
// buffers proportional to the candidate length.
func BenchmarkJaroWinklerMatcherManyCandidates(b *testing.B) {
	const numCandidates = 10_000
	candidate := strings.Repeat("candidate", 226) // ~2 KiB.
	candidates := make([]string, numCandidates)
	for i := range candidates {
		candidates[i] = candidate[:len(candidate)-i%8]
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m := NewJaroWinklerMatcher("search-term")
		for _, c := range candidates {
			m.Score(c)
		}
	}
}
