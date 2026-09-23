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
	"math/rand/v2"
	"strings"
	"sync"
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

func TestJaroWinklerMatcherAcrossCandidateSizes(t *testing.T) {
	matcher := NewJaroWinklerMatcher("prometheus")
	tests := []struct {
		name      string
		candidate string
		min       float64
		max       float64
	}{
		{name: "large similar", candidate: "promethus" + strings.Repeat("x", 128), min: 0.79, max: 0.80},
		{name: "small similar", candidate: "promethus", min: 0.979, max: 0.981},
		{name: "large dissimilar", candidate: strings.Repeat("z", 256), min: 0.0, max: 0.0},
		{name: "exact", candidate: "prometheus", min: 1.0, max: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := matcher.Score(tt.candidate)
			if score < tt.min || score > tt.max {
				t.Errorf("Score(%q) = %f, want in [%f, %f]", tt.candidate, score, tt.min, tt.max)
			}
		})
	}
}

func TestJaroWinklerWorkspaceMatchSlices(t *testing.T) {
	// A workspace whose buffer was shrunk by an earlier, smaller call, with stale
	// flags left anywhere in the backing array.
	backing := make([]bool, 200)
	for i := range backing {
		backing[i] = true
	}
	w := &jaroWinklerWorkspace{matches: backing[:50]}

	steps := []struct {
		name        string
		l1, l2      int
		wantReused  bool
		wantCapFrom int
	}{
		{name: "grow past len within cap", l1: 60, l2: 90, wantReused: true, wantCapFrom: 200},
		{name: "shrink", l1: 3, l2: 40, wantReused: true, wantCapFrom: 200},
		{name: "grow to exactly cap", l1: 100, l2: 100, wantReused: true, wantCapFrom: 200},
		{name: "grow past cap", l1: 150, l2: 151, wantReused: false, wantCapFrom: 301},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			prev := &w.matches[:cap(w.matches)][0]
			s1, s2 := w.matchSlices(step.l1, step.l2)
			if len(s1) != step.l1 || len(s2) != step.l2 {
				t.Fatalf("lengths = %d, %d, want %d, %d", len(s1), len(s2), step.l1, step.l2)
			}
			if reused := &w.matches[:cap(w.matches)][0] == prev; reused != step.wantReused {
				t.Fatalf("reused backing array = %v, want %v", reused, step.wantReused)
			}
			if cap(w.matches) < step.wantCapFrom {
				t.Fatalf("cap = %d, want at least %d", cap(w.matches), step.wantCapFrom)
			}
			for i, v := range w.matches {
				if v {
					t.Fatalf("flag %d is stale", i)
				}
			}
			// Dirty the buffers, as a score would, before the next step.
			for i := range s1 {
				s1[i] = true
			}
			for i := range s2 {
				s2[i] = true
			}
		})
	}
}

func TestJaroWinklerMatcherAllocations(t *testing.T) {
	tests := []struct {
		name      string
		term      string
		candidate string
		maxAllocs float64
	}{
		{
			name:      "long ASCII",
			term:      strings.Repeat("a", 40),
			candidate: strings.Repeat("a", 39) + "b",
			maxAllocs: 0,
		},
		{
			name:      "short term and medium ASCII candidate",
			term:      "prometheus",
			candidate: strings.Repeat("a", 49) + "b",
			maxAllocs: 0,
		},
		{
			name:      "ASCII term and Unicode candidate",
			term:      strings.Repeat("a", 40),
			candidate: strings.Repeat("a", 39) + "é",
			maxAllocs: 1,
		},
		{
			name:      "Unicode term and candidate",
			term:      strings.Repeat("a", 39) + "é",
			candidate: strings.Repeat("a", 39) + "è",
			maxAllocs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewJaroWinklerMatcher(tt.term)
			// AllocsPerRun performs one unmeasured warm-up call, matching the
			// steady-state reuse expected when one matcher scores many values.
			got := testing.AllocsPerRun(1000, func() {
				matcher.Score(tt.candidate)
			})
			if got > tt.maxAllocs {
				t.Errorf("Score() allocations = %v, want at most %v", got, tt.maxAllocs)
			}
		})
	}
}

func TestJaroWinklerMatcherConcurrentUnicode(t *testing.T) {
	const (
		iterations = 10
		workers    = 32
	)
	const term = "prometheus"
	candidate := "prométheus" + strings.Repeat("x", 128)
	want := NewJaroWinklerMatcher(term).Score(candidate)

	for range iterations {
		matcher := NewJaroWinklerMatcher(term)
		scores := make([]float64, workers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range workers {
			wg.Go(func() {
				<-start
				scores[i] = matcher.Score(candidate)
			})
		}
		close(start)
		wg.Wait()

		for _, score := range scores {
			if score != want {
				t.Fatalf("Score() = %f, want %f", score, want)
			}
		}
	}
}

// referenceJaroWinkler is the pre-pooling implementation: it allocates fresh
// match buffers on every call and always works on runes. It serves as an oracle
// for the pooled buffers and the ASCII fast path.
func referenceJaroWinkler(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}
	r1, r2 := []rune(a), []rune(b)
	if len(r1) > len(r2) {
		r1, r2 = r2, r1
	}
	l1, l2 := len(r1), len(r2)
	matchDistance := max(l2/2-1, 0)
	r1Matches := make([]bool, l1)
	r2Matches := make([]bool, l2)

	var matches, transpositions float64
	for i := range l1 {
		for j := max(i-matchDistance, 0); j < min(i+matchDistance+1, l2); j++ {
			if r2Matches[j] || r1[i] != r2[j] {
				continue
			}
			r1Matches[i] = true
			r2Matches[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := range l1 {
		if !r1Matches[i] {
			continue
		}
		for !r2Matches[k] {
			k++
		}
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}

	jaro := (matches/float64(l1) + matches/float64(l2) + (matches-transpositions/2)/matches) / 3
	prefixLen := 0
	for prefixLen < min(4, l1) && r1[prefixLen] == r2[prefixLen] {
		prefixLen++
	}
	return jaro + float64(prefixLen)*0.1*(1-jaro)
}

func TestJaroWinklerMatcherMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	// A small alphabet produces plenty of matches and transpositions. The
	// accented runes route some pairs through the rune path.
	alphabets := []string{"abcdef_", "abcdef_é"}
	randomString := func(alphabet []rune, n int) string {
		var sb strings.Builder
		for range n {
			sb.WriteRune(alphabet[rng.IntN(len(alphabet))])
		}
		return sb.String()
	}

	// One matcher per term scores many candidates, so pooled buffers are
	// reused across calls of different sizes, and lengths straddle
	// jaroWinklerPoolThreshold.
	for range 200 {
		alphabet := []rune(alphabets[rng.IntN(len(alphabets))])
		term := randomString(alphabet, rng.IntN(100))
		matcher := NewJaroWinklerMatcher(term)
		for range 100 {
			candidate := randomString(alphabet, rng.IntN(100))
			got := matcher.Score(candidate)
			want := referenceJaroWinkler(term, candidate)
			if math.Abs(got-want) > 1e-12 {
				t.Fatalf("NewJaroWinklerMatcher(%q).Score(%q) = %v, want %v", term, candidate, got, want)
			}
		}
	}
}
