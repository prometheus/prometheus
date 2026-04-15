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
	"math"
	"testing"
)

func TestJaroWinklerMatcher(t *testing.T) {
	tests := []struct {
		s1, s2 string
		min    float64
		max    float64
	}{
		// Identical strings.
		{"prometheus", "prometheus", 1.0, 1.0},
		{"", "", 1.0, 1.0},

		// Empty vs non-empty.
		{"", "abc", 0.0, 0.0},
		{"abc", "", 0.0, 0.0},

		// Completely different strings.
		{"abc", "xyz", 0.0, 0.01},

		// Similar strings.
		{"mimir", "mimer", 0.90, 0.92},
		{"martha", "marhta", 0.96, 0.97},
		{"dwayne", "duane", 0.83, 0.85},
		{"dixon", "dicksonx", 0.81, 0.83},

		// Single character strings.
		{"a", "a", 1.0, 1.0},
		{"a", "b", 0.0, 0.0},

		// Common prefix boost.
		{"prefix_abc", "prefix_xyz", 0.80, 0.90},

		// Unicode strings (exercises the rune path).
		{"café", "cafe", 0.88, 0.89},
		{"naïve", "naive", 0.89, 0.90},
		{"résumé", "resume", 0.79, 0.81},
		// Identical Unicode strings.
		{"café", "café", 1.0, 1.0},
		// Empty vs Unicode.
		{"", "café", 0.0, 0.0},
		{"café", "", 0.0, 0.0},
		// Two Unicode strings compared to each other.
		{"café", "cafè", 0.88, 0.89},
		// Common Unicode prefix (exercises Winkler boost on runes).
		{"préfixe_abc", "préfixe_xyz", 0.80, 0.90},
		// Unicode strings with unequal rune lengths (exercises swap in rune path).
		{"naïve_long", "naïve", 0.89, 0.91},
		// Completely different Unicode strings (exercises zero-matches in rune path).
		{"äöü", "éèê", 0.0, 0.01},
		// Unicode transpositions (mirrors martha/marhta in rune path).
		{"màrthà", "màrhtà", 0.96, 0.97},
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_"+tt.s2, func(t *testing.T) {
			score := NewJaroWinklerMatcher(tt.s1).Score(tt.s2)
			if score < tt.min || score > tt.max {
				t.Errorf("NewJaroWinklerMatcher(%q).Score(%q) = %f, want in [%f, %f]", tt.s1, tt.s2, score, tt.min, tt.max)
			}
			// Verify symmetry.
			reverse := NewJaroWinklerMatcher(tt.s2).Score(tt.s1)
			if math.Abs(score-reverse) > 1e-10 {
				t.Errorf("NewJaroWinklerMatcher(%q).Score(%q) = %f, but NewJaroWinklerMatcher(%q).Score(%q) = %f (not symmetric)", tt.s1, tt.s2, score, tt.s2, tt.s1, reverse)
			}
		})
	}
}
