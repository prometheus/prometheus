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

// JaroWinkler computes the Jaro-Winkler similarity between two strings,
// returning a score in [0.0, 1.0] where 1.0 means identical strings.
func JaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if s1 == "" || s2 == "" {
		return 0.0
	}

	// For ASCII strings, use the string-native path that avoids slice conversion.
	if isASCII(s1) && isASCII(s2) {
		return jaroWinklerString(s1, s2)
	}
	return jaroWinklerRunes([]rune(s1), []rune(s2))
}

// isASCII reports whether s contains only ASCII characters.
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// jaroWinklerString implements the Jaro-Winkler algorithm directly on ASCII
// strings, avoiding any []byte conversion.
func jaroWinklerString(s1, s2 string) float64 {
	l1, l2 := len(s1), len(s2)

	// Swap so s1 is always the shorter string.
	if l1 > l2 {
		s1, s2 = s2, s1
		l1, l2 = l2, l1
	}

	matchDistance := max(l2/2-1, 0)

	s1Matches := make([]bool, l1)
	s2Matches := make([]bool, l2)

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
	maxPrefix := min(4, l1, l2)
	for i := range maxPrefix {
		if s1[i] != s2[i] {
			break
		}
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

	matchDistance := max(l2/2-1, 0)

	s1Matches := make([]bool, l1)
	s2Matches := make([]bool, l2)

	var matches float64
	var transpositions float64

	for i := range l1 {
		start := max(i-matchDistance, 0)
		end := min(i+matchDistance+1, l2)

		for j := start; j < end; j++ {
			if s2Matches[j] || r1[i] != r2[j] {
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
	maxPrefix := min(4, l1, l2)
	for i := range maxPrefix {
		if r1[i] != r2[i] {
			break
		}
		prefixLen++
	}

	const p = 0.1 // Standard Winkler prefix scaling factor; not intended to be user-configurable.
	return jaro + float64(prefixLen)*p*(1.0-jaro)
}
