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

// The scoring algorithm is inspired by two JavaScript libraries:
// https://github.com/Nexucis/fuzzy (MIT License), used by the Prometheus UI,
// which itself was inspired by https://github.com/mattyork/fuzzy (MIT License).

package strutil

import "strings"

// SubsequenceMatcher pre-computes the encoding of a fixed search pattern so
// that it can be scored against many candidate strings without repeating the
// ASCII check or rune conversion on the pattern for every call. The first
// Score call with a Unicode candidate lazily caches the pattern's rune slice.
// It is not safe for concurrent use.
type SubsequenceMatcher struct {
	pattern      string
	patternLen   int    // byte length; used for the pre-check len(pattern) > len(text)
	patternASCII bool   // whether pattern is pure ASCII
	patternRunes []rune // pre-converted runes; set when !patternASCII or on first Unicode text
}

// NewSubsequenceMatcher returns a matcher for the given pattern.
func NewSubsequenceMatcher(pattern string) *SubsequenceMatcher {
	if isASCII(pattern) {
		return &SubsequenceMatcher{pattern: pattern, patternLen: len(pattern), patternASCII: true}
	}
	return &SubsequenceMatcher{pattern: pattern, patternLen: len(pattern), patternRunes: []rune(pattern)}
}

// Score computes a fuzzy match score between the matcher's pattern and text
// using a greedy character matching algorithm. Characters in pattern must
// appear in text in order (subsequence matching).
// The score is normalized to [0.0, 1.0] where:
//   - 1.0 means exact match only.
//   - 0.0 means no match (pattern is not a subsequence of text).
//   - Intermediate values reward consecutive matches and penalize gaps.
//
// This is a simple scorer for autocomplete ranking. It does not try every
// possible match, so it may miss the best score when the pattern can match the
// text in more than one way.
//
// The raw scoring formula is: Σ(interval_size²) − Σ(gap_size / text_length) − trailing_gap / (2 * text_length).
// The result is normalized by pattern_length² (the maximum possible raw score).
func (m *SubsequenceMatcher) Score(text string) float64 {
	if m.pattern == "" {
		return 1.0
	}
	if text == "" {
		return 0.0
	}

	// Exact match: perfect score, checked before any allocation.
	if m.pattern == text {
		return 1.0
	}

	// Byte length >= rune count, so this is a safe early exit before any allocation.
	if m.patternLen > len(text) {
		return 0.0
	}

	// For ASCII strings, use the string-native path that avoids []rune conversion.
	// If pattern has non-ASCII runes but text is pure ASCII, no non-ASCII
	// pattern rune can ever match, so the pattern cannot be a subsequence.
	textASCII := isASCII(text)
	switch {
	case m.patternASCII && textASCII:
		return matchSubsequenceString(m.pattern, text)
	case !m.patternASCII && textASCII:
		return 0.0
	}
	if m.patternRunes == nil {
		// pattern is ASCII but text is Unicode; convert and cache pattern runes.
		m.patternRunes = []rune(m.pattern)
	}
	return matchSubsequenceRunes(m.patternRunes, []rune(text))
}

// isASCII reports whether s contains only ASCII characters.
func isASCII(s string) bool {
	for _, c := range s {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

// matchSubsequenceString is the string-native implementation of the scoring
// algorithm for ASCII inputs. It uses strings.IndexByte for character scanning,
// with divisions by textLen replaced by a precomputed reciprocal multiply.
func matchSubsequenceString(pattern, text string) float64 {
	patternLen := len(pattern)
	textLen := len(text)
	invTextLen := 1.0 / float64(textLen)
	maxStart := textLen - patternLen

	// scoreFrom scores a match starting at startPos, where
	// text[startPos] == pattern[0] is guaranteed by the caller.
	scoreFrom := func(startPos int) (float64, bool) {
		i := startPos
		from := i
		to := i
		patternIdx := 1
		i++
		// Extend the initial consecutive run.
		for patternIdx < patternLen && i < textLen && text[i] == pattern[patternIdx] {
			to = i
			patternIdx++
			i++
		}
		var score float64
		if from > 0 {
			score -= float64(from) * invTextLen
		}
		size := to - from + 1
		score += float64(size * size)
		prevTo := to

		for patternIdx < patternLen {
			// Jump to the next occurrence of pattern[patternIdx].
			j := strings.IndexByte(text[i:], pattern[patternIdx])
			if j < 0 {
				return 0, false
			}
			i += j
			from = i
			to = i
			patternIdx++
			i++
			// Extend the consecutive run.
			for patternIdx < patternLen && i < textLen && text[i] == pattern[patternIdx] {
				to = i
				patternIdx++
				i++
			}
			if gap := from - prevTo - 1; gap > 0 {
				score -= float64(gap) * invTextLen
			}
			size = to - from + 1
			score += float64(size * size)
			prevTo = to
		}

		// Penalise unmatched trailing characters at half the leading/inner rate.
		if trailing := textLen - 1 - prevTo; trailing > 0 {
			score -= float64(trailing) * invTextLen * 0.5
		}
		return score, true
	}

	bestScore := -1.0
	for i := 0; i <= maxStart; {
		// Scan for the first pattern character.
		j := strings.IndexByte(text[i:maxStart+1], pattern[0])
		if j < 0 {
			break
		}
		i += j
		s, matched := scoreFrom(i)
		if !matched {
			// If the pattern cannot be completed from i, no later start can
			// succeed: text[i+1:] is a strict subset of text[i:].
			break
		}
		if s > bestScore {
			bestScore = s
		}
		i++
	}

	if bestScore < 0 {
		return 0.0
	}
	return bestScore / float64(patternLen*patternLen)
}

// matchSubsequenceRunes implements the scoring algorithm over pre-converted
// rune slices for the Unicode path.
func matchSubsequenceRunes(patternSlice, textSlice []rune) float64 {
	patternLen := len(patternSlice)
	textLen := len(textSlice)
	invTextLen := 1.0 / float64(textLen)

	// matchFromPos tries to match all pattern characters as a subsequence of
	// text starting at startPos. Returns the raw score and true on success, or
	// 0 and false if the pattern cannot be fully matched.
	// The score is accumulated inline, tracking only prevTo, to avoid any allocation.
	matchFromPos := func(startPos int) (float64, bool) {
		patternIdx := 0
		i := startPos
		var score float64
		prevTo := -1

		for i < textLen && patternIdx < patternLen {
			if textSlice[i] == patternSlice[patternIdx] {
				from := i
				to := i
				patternIdx++
				i++
				for i < textLen && patternIdx < patternLen && textSlice[i] == patternSlice[patternIdx] {
					to = i
					patternIdx++
					i++
				}
				var gapSize int
				if prevTo < 0 {
					gapSize = from
				} else {
					gapSize = from - prevTo - 1
				}
				if gapSize > 0 {
					score -= float64(gapSize) * invTextLen
				}
				size := to - from + 1
				score += float64(size * size)
				prevTo = to
			} else {
				i++
			}
		}

		if patternIdx < patternLen {
			return 0, false
		}

		// Penalize unmatched trailing characters at half the leading/inner gap rate.
		trailingGap := textLen - 1 - prevTo
		if trailingGap > 0 {
			score -= float64(trailingGap) * invTextLen * 0.5
		}

		return score, true
	}

	bestScore := -1.0
	// Only iterate while there are enough characters left for the pattern to fit.
	maxStart := textLen - patternLen
	for i := 0; i <= maxStart; i++ {
		if textSlice[i] != patternSlice[0] {
			continue
		}
		s, matched := matchFromPos(i)
		if !matched {
			// If matching fails from this position, no later position can succeed
			// since the remaining text is a strict subset.
			break
		}
		if s > bestScore {
			bestScore = s
		}
	}

	if bestScore < 0 {
		return 0.0
	}

	// Normalize by pattern_length² (the maximum possible raw score).
	return bestScore / float64(patternLen*patternLen)
}
