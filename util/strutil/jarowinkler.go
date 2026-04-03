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

	// Convert to runes for proper UTF-8 handling.
	r1, r2 := []rune(s1), []rune(s2)
	l1, l2 := len(r1), len(r2)
	if l1 == 0 || l2 == 0 {
		return 0.0
	}

	// Jaro similarity.
	matchDistance := max(l1, l2)/2 - 1
	matchDistance = max(matchDistance, 0)

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

	jaro := (matches/float64(l1) + matches/float64(l2) + (matches-transpositions/2)/matches) / 3.0

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
