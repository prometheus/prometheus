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

// SubsequenceScore computes a fuzzy match score between pattern and text using
// a sequential character matching algorithm. Characters in pattern must appear
// in text in order (subsequence matching).
// The score is normalized to [0.0, 1.0] where:
//   - 1.0 means exact match only.
//   - 0.0 means no match (pattern is not a subsequence of text).
//   - Intermediate values reward consecutive matches and penalize gaps.
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

	patternRunes := []rune(pattern)
	textRunes := []rune(text)
	patternLen := len(patternRunes)
	textLen := len(textRunes)

	if patternLen > textLen {
		return 0.0
	}

	// Exact match: perfect score.
	if pattern == text {
		return 1.0
	}

	type matchInterval struct {
		from, to int // inclusive bounds
	}

	// calcRawScore computes the raw score for a set of matching intervals.
	calcRawScore := func(intervals []matchInterval) float64 {
		var result float64
		for i, iv := range intervals {
			var gapSize int
			if i == 0 {
				gapSize = iv.from
			} else {
				gapSize = iv.from - intervals[i-1].to - 1
			}
			if gapSize > 0 {
				result -= float64(gapSize) / float64(textLen)
			}
			size := iv.to - iv.from + 1
			result += float64(size * size)
		}
		// Penalize unmatched trailing characters at half the leading/inner gap rate.
		trailingGap := textLen - 1 - intervals[len(intervals)-1].to
		if trailingGap > 0 {
			result -= float64(trailingGap) / float64(2*textLen)
		}
		return result
	}

	// matchFromPos tries to match all pattern characters as a subsequence of text
	// starting at startPos. Returns the raw score and true on success, or 0 and
	// false if the pattern cannot be fully matched.
	matchFromPos := func(startPos int) (float64, bool) {
		patternIdx := 0
		var intervals []matchInterval
		i := startPos
		for i < textLen && patternIdx < patternLen {
			if textRunes[i] == patternRunes[patternIdx] {
				iv := matchInterval{from: i, to: i}
				patternIdx++
				i++
				for i < textLen && patternIdx < patternLen && textRunes[i] == patternRunes[patternIdx] {
					iv.to = i
					patternIdx++
					i++
				}
				intervals = append(intervals, iv)
			} else {
				i++
			}
		}
		if patternIdx < patternLen {
			return 0, false
		}
		return calcRawScore(intervals), true
	}

	bestScore := -1.0
	// Only iterate while there are enough characters left for the pattern to fit.
	maxStart := textLen - patternLen
	for i := 0; i <= maxStart; i++ {
		if textRunes[i] != patternRunes[0] {
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
