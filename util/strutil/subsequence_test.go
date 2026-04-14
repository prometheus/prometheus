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
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// generateNames returns n synthetic metric-style names for benchmarking.
// Names are varied: some share characters with the query, some do not.
func generateNames(n int) []string {
	names := make([]string, 0, n)
	prefixes := []string{
		"http_requests_total",
		"process_cpu_seconds",
		"go_goroutines",
		"node_memory_bytes",
		"prometheus_tsdb_head",
		"grpc_server_handled",
		"container_cpu_usage",
		"kube_pod_status",
		"up",
		"scrape_duration_seconds",
	}
	for i := range n {
		base := prefixes[i%len(prefixes)]
		names = append(names, fmt.Sprintf("%s_%d", base, i/len(prefixes)))
	}
	return names
}

// generateNamesRunes returns n synthetic names containing non-ASCII characters
// for benchmarking the rune path. Non-ASCII characters are inserted at varying
// positions and with varying characters across prefixes, so the benchmark
// exercises both matching and non-matching rune paths.
func generateNamesRunes(n int) []string {
	names := make([]string, 0, n)
	prefixes := []string{
		"é_http_requests_total",   // é prepended; matches "éhttp_total"
		"process_écu_seconds",     // é mid-name; no match
		"go_goroutines",           // ASCII; no match
		"énode_memory_bytes",      // é prepended, different root; no match
		"prometheus_tsdb_head",    // ASCII; no match
		"grpc_sérveur_handled",    // é mid-name; no match
		"é_http_container_total",  // é prepended; matches "éhttp_total"
		"kube_pod_status",         // ASCII; no match
		"ñup",                     // other non-ASCII; no match
		"scrape_duration_séconds", // é near end; no match
	}
	for i := range n {
		base := prefixes[i%len(prefixes)]
		names = append(names, fmt.Sprintf("%s_%d", base, i/len(prefixes)))
	}
	return names
}

func BenchmarkSubsequenceScoreString(b *testing.B) {
	// A 10-character query that partially matches many of the generated names.
	const query = "http_total"

	for _, n := range []int{1000, 2000, 10000, 100000, 1000000} {
		names := generateNames(n)
		b.Run(fmt.Sprintf("names=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			m := NewSubsequenceMatcher(query)
			for b.Loop() {
				for _, name := range names {
					m.Score(name)
				}
			}
		})
	}
}

func BenchmarkSubsequenceScoreRunes(b *testing.B) {
	// Non-ASCII query to force the rune path; matches a subset of the generated names.
	const query = "éhttp_total"

	for _, n := range []int{1000, 2000, 10000, 100000, 1000000} {
		names := generateNamesRunes(n)
		b.Run(fmt.Sprintf("names=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			m := NewSubsequenceMatcher(query)
			for b.Loop() {
				for _, name := range names {
					m.Score(name)
				}
			}
		})
	}
}

func TestSubsequenceScore(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		text      string
		wantScore float64
		wantZero  bool
	}{
		{
			name:      "empty pattern",
			pattern:   "",
			text:      "anything",
			wantScore: 1.0,
		},
		{
			name:     "empty text",
			pattern:  "abc",
			text:     "",
			wantZero: true,
		},
		{
			name:      "exact match",
			pattern:   "my awesome text",
			text:      "my awesome text",
			wantScore: 1.0,
		},
		{
			name:    "prefix match",
			pattern: "my",
			text:    "my awesome text",
			// intervals [0,1], leading=0, trailing=13. raw = 4 - 13/30, normalized by 4.
			wantScore: 107.0 / 120.0,
		},
		{
			name:    "substring match",
			pattern: "tex",
			text:    "my awesome text",
			// intervals [11,13], leading=11, trailing=1. raw = 9 - 11/15 - 1/30, normalized by 9.
			wantScore: 247.0 / 270.0,
		},
		{
			name:    "fuzzy match picks best starting position",
			pattern: "met",
			text:    "my awesome text",
			// intervals [8,9] and [11,11], leading=8, inner gap=1, trailing=3. raw = 5 - 9/15 - 3/30, normalized by 9.
			wantScore: 43.0 / 90.0,
		},
		{
			name:      "prefers later position with better consecutive run",
			pattern:   "bac",
			text:      "babac",
			wantScore: 43.0 / 45.0, // match at [2,4], leading gap=2, trailing=0. raw = 9 - 2/5, normalized by 9.
		},
		{
			name:     "pattern longer than text",
			pattern:  "abcd",
			text:     "abc",
			wantZero: true,
		},
		{
			name:     "pattern longer in runes than multi-byte text",
			pattern:  "abc",
			text:     "éé",
			wantZero: true,
		},
		{
			name:     "non-ASCII pattern with ASCII text",
			pattern:  "é",
			text:     "ab",
			wantZero: true,
		},
		{
			name:     "no subsequence match",
			pattern:  "xyz",
			text:     "abc",
			wantZero: true,
		},
		{
			name:      "unicode exact match",
			pattern:   "éàü",
			text:      "éàü",
			wantScore: 1.0,
		},
		{
			name:    "unicode prefix match",
			pattern: "éà",
			text:    "éàü",
			// intervals [0,1], leading=0, trailing=1. raw = 4 - 1/6, normalized by 4.
			wantScore: 23.0 / 24.0,
		},
		{
			name:     "unicode no match",
			pattern:  "üé",
			text:     "éàü",
			wantZero: true,
		},
		{
			name:     "unicode first char matches but pattern cannot complete",
			pattern:  "éàx",
			text:     "éàü",
			wantZero: true,
		},
		{
			name:    "unicode fuzzy match with gap between intervals",
			pattern: "éü",
			text:    "éàü",
			// intervals [0,0] and [2,2], leading=0, inner gap=1, trailing=0.
			// raw = 1 + 1 - 1/3, normalized by 4.
			wantScore: 5.0 / 12.0,
		},
		{
			name:    "mixed ascii and unicode",
			pattern: "aé",
			text:    "aéb",
			// intervals [0,1], leading=0, trailing=1. raw = 4 - 1/6, normalized by 4.
			wantScore: 23.0 / 24.0,
		},
		{
			name: "unicode chars sharing leading utf-8 byte do not match",
			// 'é' (U+00E9) encodes as [0xC3 0xA9] and 'ã' (U+00E3) as [0xC3 0xA3].
			// They share the leading byte but must not be treated as equal.
			pattern:  "é",
			text:     "ã",
			wantZero: true,
		},
		{
			name:      "single char exact match",
			pattern:   "a",
			text:      "a",
			wantScore: 1.0,
		},
		{
			name:    "consecutive match with leading gap",
			pattern: "oa",
			text:    "goat",
			// 'o'(1),'a'(2) form one interval [1,2], leading gap=1, trailing=1.
			// raw = 2² - 1/4 - 1/8 = 29/8, normalized by 2² = 4.
			wantScore: 29.0 / 32.0,
		},
		{
			name:    "repeated chars use greedy match",
			pattern: "abaa",
			text:    "abbaa",
			// Matches 'a'(0),'b'(1),'a'(3),'a'(4) as intervals [0,1] and [3,4].
			// raw = 2² + 2² - 1/5, normalized by 4² = 16.
			// A better match exists at 'a'(0),'b'(2),'a'(3),'a'(4), which would score 49/80,
			// but this test documents the current greedy behavior.
			wantScore: 39.0 / 80.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSubsequenceMatcher(tt.pattern).Score(tt.text)
			if tt.wantZero {
				require.Equal(t, 0.0, got)
				return
			}
			require.InDelta(t, tt.wantScore, got, 1e-9)
		})
	}
}

func TestSubsequenceScoreProperties(t *testing.T) {
	// Prefix match scores below 1.0; only exact match scores 1.0.
	// "pro" in "prometheus": intervals [0,2], trailing=7. raw = 9 - 7/20, normalized by 9.
	require.InDelta(t, 173.0/180.0, NewSubsequenceMatcher("pro").Score("prometheus"), 1e-9)

	// Exact match always scores 1.0.
	require.Equal(t, 1.0, NewSubsequenceMatcher("prometheus").Score("prometheus"))

	// Score is always in [0, 1].
	cases := [][2]string{
		{"abc", "xaxbxcx"},
		{"z", "aaaaaz"},
		{"ab", "ba"},
		{"met", "my awesome text"},
	}
	for _, c := range cases {
		score := NewSubsequenceMatcher(c[0]).Score(c[1])
		require.True(t, score >= 0.0 && score <= 1.0,
			"score %v out of range for pattern=%q text=%q", score, c[0], c[1])
		require.False(t, math.IsNaN(score))
	}

	// Prefix scores higher than non-prefix substring.
	prefixScore := NewSubsequenceMatcher("abc").Score("abcdef")
	suffixScore := NewSubsequenceMatcher("abc").Score("defabc")
	require.Greater(t, prefixScore, suffixScore)

	// Consecutive chars score higher than scattered.
	consecutiveScore := NewSubsequenceMatcher("abc").Score("xabcx")
	scatteredScore := NewSubsequenceMatcher("abc").Score("xaxbxcx")
	require.Greater(t, consecutiveScore, scatteredScore)
}
