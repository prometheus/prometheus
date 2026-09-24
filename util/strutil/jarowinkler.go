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

import "sync"

const (
	// jaroWinklerPoolThreshold is the largest match buffer, in flags, that is
	// allocated locally. The compiler keeps variable-sized buffers of up to 32
	// bytes on the stack, so only longer inputs need the pool.
	jaroWinklerPoolThreshold = 32

	// jaroWinklerMaxPooledCapacity prevents unusually large inputs from retaining
	// a correspondingly large buffer in each sync.Pool shard.
	jaroWinklerMaxPooledCapacity = 64 << 10
)

type jaroWinklerWorkspace struct {
	matches []bool
}

var jaroWinklerWorkspacePool = sync.Pool{
	New: func() any {
		return &jaroWinklerWorkspace{}
	},
}

// matchSlices returns cleared match buffers of lengths l1 and l2, reusing the
// workspace's backing array when it is large enough. Reslicing up to the
// capacity is valid even when the previous length was shorter.
func (w *jaroWinklerWorkspace) matchSlices(l1, l2 int) (s1Matches, s2Matches []bool) {
	total := l1 + l2
	if cap(w.matches) < total {
		w.matches = make([]bool, total)
	} else {
		w.matches = w.matches[:total]
		clear(w.matches)
	}
	return w.matches[:l1], w.matches[l1:]
}

func getJaroWinklerMatchSlices(l1, l2 int) (s1Matches, s2Matches []bool, workspace *jaroWinklerWorkspace) {
	workspace = jaroWinklerWorkspacePool.Get().(*jaroWinklerWorkspace)
	s1Matches, s2Matches = workspace.matchSlices(l1, l2)
	return s1Matches, s2Matches, workspace
}

func putJaroWinklerWorkspace(workspace *jaroWinklerWorkspace) {
	if cap(workspace.matches) > jaroWinklerMaxPooledCapacity {
		return
	}
	jaroWinklerWorkspacePool.Put(workspace)
}

// JaroWinklerMatcher records the encoding of a fixed search term so that it can
// be scored against many candidate strings without repeating work on the term.
// It is safe for concurrent use.
type JaroWinklerMatcher struct {
	term          string // original term; used directly on the ASCII path
	termASCII     bool   // whether term is pure ASCII
	termRunesOnce sync.Once
	termRunes     []rune // pre-converted runes for the Unicode path
}

// NewJaroWinklerMatcher returns a matcher for the given term.
func NewJaroWinklerMatcher(term string) *JaroWinklerMatcher {
	if isASCII(term) {
		return &JaroWinklerMatcher{term: term, termASCII: true}
	}
	return &JaroWinklerMatcher{term: term, termRunes: []rune(term)}
}

// Score returns the Jaro-Winkler similarity between the matcher's term and s,
// in [0.0, 1.0] where 1.0 means identical strings.
func (m *JaroWinklerMatcher) Score(s string) float64 {
	if m.term == s {
		return 1.0
	}
	if m.term == "" || s == "" {
		return 0.0
	}
	if m.termASCII && isASCII(s) {
		return jaroWinklerString(m.term, s)
	}
	// Either the term or s is Unicode; use the rune path.
	if m.termASCII {
		m.termRunesOnce.Do(func() {
			m.termRunes = []rune(m.term)
		})
	}
	return jaroWinklerRunes(m.termRunes, []rune(s))
}

// jaroWinklerString implements the Jaro-Winkler algorithm directly on ASCII
// strings, avoiding any []rune conversion.
func jaroWinklerString(s1, s2 string) float64 {
	l1, l2 := len(s1), len(s2)

	// Swap so s1 is always the shorter string.
	if l1 > l2 {
		s1, s2 = s2, s1
		l1, l2 = l2, l1
	}

	// Jaro match distance: characters must be within this many positions to match.
	matchDistance := max(l2/2-1, 0)

	var s1Matches, s2Matches []bool
	var workspace *jaroWinklerWorkspace
	if l2 > jaroWinklerPoolThreshold { // l2 is the longer input.
		s1Matches, s2Matches, workspace = getJaroWinklerMatchSlices(l1, l2)
		defer putJaroWinklerWorkspace(workspace)
	} else {
		s1Matches = make([]bool, l1)
		s2Matches = make([]bool, l2)
	}

	var matches float64
	var transpositions float64

	for i := range l1 {
		start := max(i-matchDistance, 0)
		end := min(i+matchDistance+1, l2)

		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := range l1 {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	// Use precomputed reciprocals to replace repeated divisions.
	invL1 := 1.0 / float64(l1)
	invL2 := 1.0 / float64(l2)
	jaro := (matches*invL1 + matches*invL2 + (matches-transpositions*0.5)/matches) / 3.0

	// Winkler modification: boost for common prefix up to 4 characters.
	prefixLen := 0
	for prefixLen < min(4, l1, l2) && s1[prefixLen] == s2[prefixLen] {
		prefixLen++
	}

	const p = 0.1 // Standard Winkler prefix scaling factor; not intended to be user-configurable.
	return jaro + float64(prefixLen)*p*(1.0-jaro)
}

// jaroWinklerRunes implements the Jaro-Winkler algorithm over pre-converted
// rune slices for the Unicode path.
func jaroWinklerRunes(r1, r2 []rune) float64 {
	l1, l2 := len(r1), len(r2)

	// Swap so r1 is always the shorter slice.
	if l1 > l2 {
		r1, r2 = r2, r1
		l1, l2 = l2, l1
	}

	// Jaro match distance: characters must be within this many positions to match.
	matchDistance := max(l2/2-1, 0)

	var r1Matches, r2Matches []bool
	var workspace *jaroWinklerWorkspace
	if l2 > jaroWinklerPoolThreshold { // l2 is the longer input.
		r1Matches, r2Matches, workspace = getJaroWinklerMatchSlices(l1, l2)
		defer putJaroWinklerWorkspace(workspace)
	} else {
		r1Matches = make([]bool, l1)
		r2Matches = make([]bool, l2)
	}

	var matches float64
	var transpositions float64

	for i := range l1 {
		start := max(i-matchDistance, 0)
		end := min(i+matchDistance+1, l2)

		for j := start; j < end; j++ {
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

	// Use precomputed reciprocals to replace repeated divisions.
	invL1 := 1.0 / float64(l1)
	invL2 := 1.0 / float64(l2)
	jaro := (matches*invL1 + matches*invL2 + (matches-transpositions*0.5)/matches) / 3.0

	// Winkler modification: boost for common prefix up to 4 characters.
	prefixLen := 0
	for prefixLen < min(4, l1, l2) && r1[prefixLen] == r2[prefixLen] {
		prefixLen++
	}

	const p = 0.1 // Standard Winkler prefix scaling factor; not intended to be user-configurable.
	return jaro + float64(prefixLen)*p*(1.0-jaro)
}
