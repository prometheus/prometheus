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

	"github.com/grafana/regexp"
	"github.com/grafana/regexp/syntax"
)

const maxSetMatches = 256

type FastRegexMatcher struct {
	re *regexp.Regexp

	setMatches    []string
	stringMatcher StringMatcher
	prefix        string
	suffix        string
	contains      string
}

func NewFastRegexMatcher(v string) (*FastRegexMatcher, error) {
	parsed, err := syntax.Parse(v, syntax.Perl)
	if err != nil {
		return nil, err
	}
	// Simplify the syntax tree to run faster.
	parsed = parsed.Simplify()
	re, err := regexp.Compile("^(?:" + parsed.String() + ")$")
	if err != nil {
		return nil, err
	}
	m := &FastRegexMatcher{
		re: re,
	}
	if parsed.Op == syntax.OpConcat {
		m.prefix, m.suffix, m.contains = optimizeConcatRegex(parsed)
	}
	if matches, caseSensitive := findSetMatches(parsed, ""); caseSensitive {
		m.setMatches = matches
	}
	m.stringMatcher = stringMatcherFromRegexp(parsed)

	return m, nil
}

// findSetMatches extract equality matches from a regexp.
// Returns nil if we can't replace the regexp by only equality matchers or the regexp contains
// a mix of case sensitive and case insensitive matchers.
func findSetMatches(re *syntax.Regexp, base string) (matches []string, caseSensitive bool) {
	clearBeginEndText(re)

	switch re.Op {
	case syntax.OpLiteral:
		return []string{base + string(re.Rune)}, isCaseSensitive(re)
	case syntax.OpEmptyMatch:
		if base != "" {
			return []string{base}, isCaseSensitive(re)
		}
	case syntax.OpAlternate:
		return findSetMatchesFromAlternate(re, base)
	case syntax.OpCapture:
		clearCapture(re)
		return findSetMatches(re, base)
	case syntax.OpConcat:
		return findSetMatchesFromConcat(re, base)
	case syntax.OpCharClass:
		if len(re.Rune)%2 != 0 {
			return nil, false
		}
		var matches []string
		var totalSet int
		for i := 0; i+1 < len(re.Rune); i = i + 2 {
			totalSet += int(re.Rune[i+1]-re.Rune[i]) + 1
		}
		// limits the total characters that can be used to create matches.
		// In some case like negation [^0-9] a lot of possibilities exists and that
		// can create thousands of possible matches at which points we're better off using regexp.
		if totalSet > maxSetMatches {
			return nil, false
		}
		for i := 0; i+1 < len(re.Rune); i = i + 2 {
			lo, hi := re.Rune[i], re.Rune[i+1]
			for c := lo; c <= hi; c++ {
				matches = append(matches, base+string(c))
			}
		}
		return matches, isCaseSensitive(re)
	default:
		return nil, false
	}
	return nil, false
}

func findSetMatchesFromConcat(re *syntax.Regexp, base string) (matches []string, matchesCaseSensitive bool) {
	if len(re.Sub) == 0 {
		return nil, false
	}
	clearCapture(re.Sub...)

	matches = []string{base}

	for i := 0; i < len(re.Sub); i++ {
		var newMatches []string
		for j, b := range matches {
			m, caseSensitive := findSetMatches(re.Sub[i], b)
			if m == nil {
				return nil, false
			}
			if tooManyMatches(newMatches, m...) {
				return nil, false
			}

			// All matches must have the same case sensitivity. If it's the first set of matches
			// returned, we store its sensitivity as the expected case, and then we'll check all
			// other ones.
			if i == 0 && j == 0 {
				matchesCaseSensitive = caseSensitive
			}
			if matchesCaseSensitive != caseSensitive {
				return nil, false
			}

			newMatches = append(newMatches, m...)
		}
		matches = newMatches
	}

	return matches, matchesCaseSensitive
}

func findSetMatchesFromAlternate(re *syntax.Regexp, base string) (matches []string, matchesCaseSensitive bool) {
	for i, sub := range re.Sub {
		found, caseSensitive := findSetMatches(sub, base)
		if found == nil {
			return nil, false
		}
		if tooManyMatches(matches, found...) {
			return nil, false
		}

		// All matches must have the same case sensitivity. If it's the first set of matches
		// returned, we store its sensitivity as the expected case, and then we'll check all
		// other ones.
		if i == 0 {
			matchesCaseSensitive = caseSensitive
		}
		if matchesCaseSensitive != caseSensitive {
			return nil, false
		}

		matches = append(matches, found...)
	}

	return matches, matchesCaseSensitive
}

// clearCapture removes capture operation as they are not used for matching.
func clearCapture(regs ...*syntax.Regexp) {
	for _, r := range regs {
		if r.Op == syntax.OpCapture {
			*r = *r.Sub[0]
		}
	}
}

// clearBeginEndText removes the begin and end text from the regexp. Prometheus regexp are anchored to the beginning and end of the string.
func clearBeginEndText(re *syntax.Regexp) {
	if len(re.Sub) == 0 {
		return
	}
	if len(re.Sub) == 1 {
		if re.Sub[0].Op == syntax.OpBeginText || re.Sub[0].Op == syntax.OpEndText {
			// We need to remove this element. Since it's the only one, we convert into a matcher of an empty string.
			// OpEmptyMatch is regexp's nop operator.
			re.Op = syntax.OpEmptyMatch
			re.Sub = nil
			return
		}
	}
	if re.Sub[0].Op == syntax.OpBeginText {
		re.Sub = re.Sub[1:]
	}
	if re.Sub[len(re.Sub)-1].Op == syntax.OpEndText {
		re.Sub = re.Sub[:len(re.Sub)-1]
	}
}

// isCaseInsensitive tells if a regexp is case insensitive.
// The flag should be check at each level of the syntax tree.
func isCaseInsensitive(reg *syntax.Regexp) bool {
	return (reg.Flags & syntax.FoldCase) != 0
}

// isCaseSensitive tells if a regexp is case sensitive.
// The flag should be check at each level of the syntax tree.
func isCaseSensitive(reg *syntax.Regexp) bool {
	return !isCaseInsensitive(reg)
}

// tooManyMatches guards against creating too many set matches
func tooManyMatches(matches []string, new ...string) bool {
	return len(matches)+len(new) > maxSetMatches
}

func (m *FastRegexMatcher) MatchString(s string) bool {
	if len(m.setMatches) != 0 {
		for _, match := range m.setMatches {
			if match == s {
				return true
			}
		}
		return false
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
	if m.stringMatcher != nil {
		return m.stringMatcher.Matches(s)
	}
	return m.re.MatchString(s)
}

func (m *FastRegexMatcher) SetMatches() []string {
	return m.setMatches
}

func (m *FastRegexMatcher) GetRegexString() string {
	return m.re.String()
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

// StringMatcher is a matcher that matches a string in place of a regular expression.
type StringMatcher interface {
	Matches(s string) bool
}

// stringMatcherFromRegexp attempts to replace a common regexp with a string matcher.
// It returns nil if the regexp is not supported.
// For examples, it will replace `.*foo` with `foo.*` and `.*foo.*` with `(?i)foo`.
func stringMatcherFromRegexp(re *syntax.Regexp) StringMatcher {
	clearCapture(re)
	clearBeginEndText(re)

	switch re.Op {
	case syntax.OpPlus, syntax.OpStar:
		if re.Sub[0].Op != syntax.OpAnyChar && re.Sub[0].Op != syntax.OpAnyCharNotNL {
			return nil
		}
		return &anyStringMatcher{
			allowEmpty: re.Op == syntax.OpStar,
			matchNL:    re.Sub[0].Op == syntax.OpAnyChar,
		}
	case syntax.OpEmptyMatch:
		return emptyStringMatcher{}

	case syntax.OpLiteral:
		return &equalStringMatcher{
			s:             string(re.Rune),
			caseSensitive: !isCaseInsensitive(re),
		}
	case syntax.OpAlternate:
		or := make([]StringMatcher, 0, len(re.Sub))
		for _, sub := range re.Sub {
			m := stringMatcherFromRegexp(sub)
			if m == nil {
				return nil
			}
			or = append(or, m)
		}
		return orStringMatcher(or)
	case syntax.OpConcat:
		clearCapture(re.Sub...)
		if len(re.Sub) == 0 {
			return emptyStringMatcher{}
		}
		if len(re.Sub) == 1 {
			return stringMatcherFromRegexp(re.Sub[0])
		}
		var left, right StringMatcher
		// Let's try to find if there's a first and last any matchers.
		if re.Sub[0].Op == syntax.OpPlus || re.Sub[0].Op == syntax.OpStar {
			left = stringMatcherFromRegexp(re.Sub[0])
			if left == nil {
				return nil
			}
			re.Sub = re.Sub[1:]
		}
		if re.Sub[len(re.Sub)-1].Op == syntax.OpPlus || re.Sub[len(re.Sub)-1].Op == syntax.OpStar {
			right = stringMatcherFromRegexp(re.Sub[len(re.Sub)-1])
			if right == nil {
				return nil
			}
			re.Sub = re.Sub[:len(re.Sub)-1]
		}

		matches, matchesCaseSensitive := findSetMatches(re, "")
		if len(matches) == 0 {
			return nil
		}

		if left == nil && right == nil {
			// if there's no any matchers on both side it's a concat of literals
			or := make([]StringMatcher, 0, len(matches))
			for _, match := range matches {
				or = append(or, &equalStringMatcher{
					s:             match,
					caseSensitive: matchesCaseSensitive,
				})
			}
			return orStringMatcher(or)
		}

		// We found literals in the middle. We can triggered the fast path only if
		// the matches are case sensitive because containsStringMatcher doesn't
		// support case insensitive.
		if matchesCaseSensitive {
			return &containsStringMatcher{
				substrings: matches,
				left:       left,
				right:      right,
			}
		}
	}
	return nil
}

// containsStringMatcher matches a string if it contains any of the substrings.
// If left and right are not nil, it's a contains operation where left and right must match.
// If left is nil, it's a hasPrefix operation and right must match.
// Finally if right is nil it's a hasSuffix operation and left must match.
type containsStringMatcher struct {
	substrings []string
	left       StringMatcher
	right      StringMatcher
}

func (m *containsStringMatcher) Matches(s string) bool {
	for _, substr := range m.substrings {
		if m.right != nil && m.left != nil {
			pos := strings.Index(s, substr)
			if pos < 0 {
				continue
			}
			if m.left.Matches(s[:pos]) && m.right.Matches(s[pos+len(substr):]) {
				return true
			}
			continue
		}
		// If we have to check for characters on the left then we need to match a suffix.
		if m.left != nil {
			if strings.HasSuffix(s, substr) && m.left.Matches(s[:len(s)-len(substr)]) {
				return true
			}
			continue
		}
		if m.right != nil {
			if strings.HasPrefix(s, substr) && m.right.Matches(s[len(substr):]) {
				return true
			}
			continue
		}
	}
	return false
}

// emptyStringMatcher matches an empty string.
type emptyStringMatcher struct{}

func (m emptyStringMatcher) Matches(s string) bool {
	return len(s) == 0
}

// orStringMatcher matches any of the sub-matchers.
type orStringMatcher []StringMatcher

func (m orStringMatcher) Matches(s string) bool {
	for _, matcher := range m {
		if matcher.Matches(s) {
			return true
		}
	}
	return false
}

// equalStringMatcher matches a string exactly and support case insensitive.
type equalStringMatcher struct {
	s             string
	caseSensitive bool
}

func (m *equalStringMatcher) Matches(s string) bool {
	if m.caseSensitive {
		return m.s == s
	}
	return strings.EqualFold(m.s, s)
}

// anyStringMatcher is a matcher that matches any string.
// It is used for the + and * operator. matchNL tells if it should matches newlines or not.
type anyStringMatcher struct {
	allowEmpty bool
	matchNL    bool
}

func (m *anyStringMatcher) Matches(s string) bool {
	if !m.allowEmpty && len(s) == 0 {
		return false
	}
	if !m.matchNL && strings.ContainsRune(s, '\n') {
		return false
	}
	return true
}
