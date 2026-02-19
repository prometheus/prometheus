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

func TestJaroWinkler(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_"+tt.s2, func(t *testing.T) {
			score := JaroWinkler(tt.s1, tt.s2)
			if score < tt.min || score > tt.max {
				t.Errorf("JaroWinkler(%q, %q) = %f, want in [%f, %f]", tt.s1, tt.s2, score, tt.min, tt.max)
			}
			// Verify symmetry.
			reverse := JaroWinkler(tt.s2, tt.s1)
			if math.Abs(score-reverse) > 1e-10 {
				t.Errorf("JaroWinkler(%q, %q) = %f, but JaroWinkler(%q, %q) = %f (not symmetric)", tt.s1, tt.s2, score, tt.s2, tt.s1, reverse)
			}
		})
	}
}
