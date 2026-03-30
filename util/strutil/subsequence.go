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

// SubsequenceScore computes a fuzzy match score between pattern and text using
// a greedy character matching algorithm. Characters in pattern must appear in
// text in order (subsequence matching).
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
func SubsequenceScore(pattern, text string) float64 {
	if pattern == "" {
		return 1.0
	}
	if text == "" {
		return 0.0
	}

	// Exact match: perfect score, checked before any allocation.
	if pattern == text {
		return 1.0
	}

	// Byte length >= rune count, so this is a safe early exit before any allocation.
	if len(pattern) > len(text) {
		return 0.0
	}

	// For ASCII strings, use byte slices to avoid the 4x memory overhead of rune conversion.
	if isASCII(pattern) && isASCII(text) {
		return matchSubsequence([]byte(pattern), []byte(text))
	}
	return matchSubsequence([]rune(pattern), []rune(text))
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

// matchSubsequence implements the scoring algorithm over a pre-converted
// character slice. T is either byte (ASCII path) or rune (Unicode path).
func matchSubsequence[T byte | rune](patternSlice, textSlice []T) float64 {
	patternLen := len(patternSlice)
	textLen := len(textSlice)

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
					score -= float64(gapSize) / float64(textLen)
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
			score -= float64(trailingGap) / float64(2*textLen)
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
	normalized := bestScore / float64(patternLen*patternLen)
	if normalized > 1.0 {
		normalized = 1.0
	}
	return normalized
}
