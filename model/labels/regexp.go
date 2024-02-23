// Copyright 2020 The Prometheus Authors
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

package labels

import (
	"strings"
	"unicode/utf8"

	"github.com/grafana/regexp"
	"github.com/grafana/regexp/syntax"
)

// Bitmap used by func isRegexMetaCharacter to check whether a character needs to be escaped.
var regexMetaCharacterBytes [16]byte

// isRegexMetaCharacter reports whether byte b needs to be escaped.
func isRegexMetaCharacter(b byte) bool {
	return b < utf8.RuneSelf && regexMetaCharacterBytes[b%16]&(1<<(b/16)) != 0
}

func init() {
	for _, b := range []byte(`.+*?()|[]{}^$`) {
		regexMetaCharacterBytes[b%16] |= 1 << (b / 16)
	}
}

type FastRegexMatcher struct {
	re       *regexp.Regexp
	literal  map[string]struct{}
	prefix   string
	suffix   string
	contains string

	pattern string
}

func NewFastRegexMatcher(v string) (*FastRegexMatcher, error) {
	literalMatchers, remainingMatcher := findSetMatches(v)
	p := "^(?:" + v + ")$"
	if len(remainingMatcher) == 0 {
		return &FastRegexMatcher{literal: literalMatchers, pattern: p}, nil
	}
	re, err := regexp.Compile("^(?:" + remainingMatcher + ")$")
	if err != nil {
		return nil, err
	}

	parsed, err := syntax.Parse(v, syntax.Perl)
	if err != nil {
		return nil, err
	}

	m := &FastRegexMatcher{
		re:      re,
		literal: literalMatchers,
	}

	if parsed.Op == syntax.OpConcat {
		m.prefix, m.suffix, m.contains = optimizeConcatRegex(parsed)
	}

	return m, nil
}

func (m *FastRegexMatcher) MatchString(s string) bool {
	if _, ok := m.literal[s]; ok {
		return true
	}

	if m.prefix != "" && !strings.HasPrefix(s, m.prefix) {
		return false
	}
	if m.suffix != "" && !strings.HasSuffix(s, m.suffix) {
		return false
	}
	if m.contains != "" && !strings.Contains(s, m.contains) {
		return false
	}
	if m.re != nil {
		return m.re.MatchString(s)
	}
	return false
}

func (m *FastRegexMatcher) GetRegexString() string {
	return m.pattern
}

// optimizeConcatRegex returns literal prefix/suffix text that can be safely
// checked against the label value before running the regexp matcher.
func optimizeConcatRegex(r *syntax.Regexp) (prefix, suffix, contains string) {
	sub := r.Sub

	// We can safely remove begin and end text matchers respectively
	// at the beginning and end of the regexp.
	if len(sub) > 0 && sub[0].Op == syntax.OpBeginText {
		sub = sub[1:]
	}
	if len(sub) > 0 && sub[len(sub)-1].Op == syntax.OpEndText {
		sub = sub[:len(sub)-1]
	}

	if len(sub) == 0 {
		return
	}

	// Given Prometheus regex matchers are always anchored to the begin/end
	// of the text, if the first/last operations are literals, we can safely
	// treat them as prefix/suffix.
	if sub[0].Op == syntax.OpLiteral && (sub[0].Flags&syntax.FoldCase) == 0 {
		prefix = string(sub[0].Rune)
	}
	if last := len(sub) - 1; sub[last].Op == syntax.OpLiteral && (sub[last].Flags&syntax.FoldCase) == 0 {
		suffix = string(sub[last].Rune)
	}

	// If contains any literal which is not a prefix/suffix, we keep the
	// 1st one. We do not keep the whole list of literals to simplify the
	// fast path.
	for i := 1; i < len(sub)-1; i++ {
		if sub[i].Op == syntax.OpLiteral && (sub[i].Flags&syntax.FoldCase) == 0 {
			contains = string(sub[i].Rune)
			break
		}
	}

	return
}

func findSetMatches(pattern string) (map[string]struct{}, string) {
	if len(pattern) < 1 {
		return map[string]struct{}{"": {}}, pattern
	}
	escaped := false
	sets := map[string]struct{}{}
	regexSets := []string{}
	init := 0
	end := len(pattern)
	containsMeta := false
	sb := strings.Builder{}
	for i := init; i < end; i++ {
		if escaped {
			switch {
			case isRegexMetaCharacter(pattern[i]):
				sb.WriteByte(pattern[i])
			case pattern[i] == '\\':
				sb.WriteByte('\\')
			default:
				return nil, pattern
			}
			escaped = false
		} else {
			switch {
			case isRegexMetaCharacter(pattern[i]):
				if pattern[i] == '|' {
					if containsMeta {
						regexSets = append(regexSets, sb.String())
					} else {
						sets[sb.String()] = struct{}{}
					}
					sb.Reset()
					containsMeta = false
				} else {
					sb.WriteByte(pattern[i])
					containsMeta = true
				}
			case pattern[i] == '\\':
				escaped = true
			default:
				sb.WriteByte(pattern[i])
			}
		}
	}
	if containsMeta {
		regexSets = append(regexSets, sb.String())
	} else {
		sets[sb.String()] = struct{}{}
	}
	return sets, strings.Join(regexSets, "|")
}
