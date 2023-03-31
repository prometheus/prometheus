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
	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	maxSetMatches = 256

	// The minimum number of alternate values a regex should have to trigger
	// the optimization done by optimizeEqualStringMatchers(). This value has
	// been computed running BenchmarkOptimizeEqualStringMatchers.
	optimizeEqualStringMatchersThreshold = 16
)

var fastRegexMatcherCache *lru.Cache[string, *FastRegexMatcher]

func init() {
	// Ignore error because it can only return error if size is invalid,
	// but we're using an hardcoded size here.
	fastRegexMatcherCache, _ = lru.New[string, *FastRegexMatcher](10000)
}

type FastRegexMatcher struct {
	re *regexp.Regexp

	setMatches    []string
	stringMatcher StringMatcher
	prefix        string
	suffix        string
	contains      string

	// matchString is the "compiled" function to run by MatchString().
	matchString func(string) bool
}

func NewFastRegexMatcher(v string) (*FastRegexMatcher, error) {
	// Check the cache.
	if matcher, ok := fastRegexMatcherCache.Get(v); ok {
		return matcher, nil
	}

	// Create a new matcher.
	matcher, err := newFastRegexMatcherWithoutCache(v)
	if err != nil {
		return nil, err
	}

	// Cache it.
	fastRegexMatcherCache.Add(v, matcher)

	return matcher, nil
}

func newFastRegexMatcherWithoutCache(v string) (*FastRegexMatcher, error) {
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
	if matches, caseSensitive := findSetMatches(parsed); caseSensitive {
		m.setMatches = matches
	}
	m.stringMatcher = stringMatcherFromRegexp(parsed)

	m.matchString = m.compileMatchStringFunction()
	return m, nil
}

// compileMatchStringFunction returns the function to run by MatchString().
func (m *FastRegexMatcher) compileMatchStringFunction() func(string) bool {
	// If the only optimization available is the string matcher, then we can just run it.
	if len(m.setMatches) == 0 && m.prefix == "" && m.suffix == "" && m.contains == "" && m.stringMatcher != nil {
		return m.stringMatcher.Matches
	}

	return func(s string) bool {
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
}

// isOptimized returns true if any fast-path optimization is applied to the
// regex matcher.
//
//nolint:unused
func (m *FastRegexMatcher) isOptimized() bool {
	return len(m.setMatches) > 0 || m.stringMatcher != nil || m.prefix != "" || m.suffix != "" || m.contains != ""
}

// findSetMatches extract equality matches from a regexp.
// Returns nil if we can't replace the regexp by only equality matchers or the regexp contains
// a mix of case sensitive and case insensitive matchers.
func findSetMatches(re *syntax.Regexp) (matches []string, caseSensitive bool) {
	clearBeginEndText(re)

	return findSetMatchesInternal(re, "")
}

func findSetMatchesInternal(re *syntax.Regexp, base string) (matches []string, caseSensitive bool) {
	switch re.Op {
	case syntax.OpBeginText:
		// Correctly handling the begin text operator inside a regex is tricky,
		// so in this case we fallback to the regex engine.
		return nil, false
	case syntax.OpEndText:
		// Correctly handling the end text operator inside a regex is tricky,
		// so in this case we fallback to the regex engine.
		return nil, false
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
		return findSetMatchesInternal(re, base)
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
			m, caseSensitive := findSetMatchesInternal(re.Sub[i], b)
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
		found, caseSensitive := findSetMatchesInternal(sub, base)
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
	// Do not clear begin/end text from an alternate operator because it could
	// change the actual regexp properties.
	if re.Op == syntax.OpAlternate {
		return
	}

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
	return m.matchString(s)
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
	clearBeginEndText(re)

	m := stringMatcherFromRegexpInternal(re)
	m = optimizeEqualStringMatchers(m, optimizeEqualStringMatchersThreshold)

	return m
}

func stringMatcherFromRegexpInternal(re *syntax.Regexp) StringMatcher {
	clearCapture(re)

	switch re.Op {
	case syntax.OpBeginText:
		// Correctly handling the begin text operator inside a regex is tricky,
		// so in this case we fallback to the regex engine.
		return nil
	case syntax.OpEndText:
		// Correctly handling the end text operator inside a regex is tricky,
		// so in this case we fallback to the regex engine.
		return nil
	case syntax.OpPlus:
		if re.Sub[0].Op != syntax.OpAnyChar && re.Sub[0].Op != syntax.OpAnyCharNotNL {
			return nil
		}
		return &anyNonEmptyStringMatcher{
			matchNL: re.Sub[0].Op == syntax.OpAnyChar,
		}
	case syntax.OpStar:
		if re.Sub[0].Op != syntax.OpAnyChar && re.Sub[0].Op != syntax.OpAnyCharNotNL {
			return nil
		}

		// If the newline is valid, than this matcher literally match any string (even empty).
		if re.Sub[0].Op == syntax.OpAnyChar {
			return trueMatcher{}
		}

		// Any string is fine (including an empty one), as far as it doesn't contain any newline.
		return anyStringWithoutNewlineMatcher{}
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
			m := stringMatcherFromRegexpInternal(sub)
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
			return stringMatcherFromRegexpInternal(re.Sub[0])
		}
		var left, right StringMatcher
		// Let's try to find if there's a first and last any matchers.
		if re.Sub[0].Op == syntax.OpPlus || re.Sub[0].Op == syntax.OpStar {
			left = stringMatcherFromRegexpInternal(re.Sub[0])
			if left == nil {
				return nil
			}
			re.Sub = re.Sub[1:]
		}
		if re.Sub[len(re.Sub)-1].Op == syntax.OpPlus || re.Sub[len(re.Sub)-1].Op == syntax.OpStar {
			right = stringMatcherFromRegexpInternal(re.Sub[len(re.Sub)-1])
			if right == nil {
				return nil
			}
			re.Sub = re.Sub[:len(re.Sub)-1]
		}

		matches, matchesCaseSensitive := findSetMatchesInternal(re, "")
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
			searchStartPos := 0

			for {
				pos := strings.Index(s[searchStartPos:], substr)
				if pos < 0 {
					break
				}

				// Since we started searching from searchStartPos, we have to add that offset
				// to get the actual position of the substring inside the text.
				pos += searchStartPos

				// If both the left and right matchers match, then we can stop searching because
				// we've found a match.
				if m.left.Matches(s[:pos]) && m.right.Matches(s[pos+len(substr):]) {
					return true
				}

				// Continue searching for another occurrence of the substring inside the text.
				searchStartPos = pos + 1
			}
		} else if m.left != nil {
			// If we have to check for characters on the left then we need to match a suffix.
			if strings.HasSuffix(s, substr) && m.left.Matches(s[:len(s)-len(substr)]) {
				return true
			}
		} else if m.right != nil {
			if strings.HasPrefix(s, substr) && m.right.Matches(s[len(substr):]) {
				return true
			}
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

// equalMultiStringMatcher matches a string exactly against a set of valid values.
type equalMultiStringMatcher struct {
	// values to match a string against. If the matching is case insensitive,
	// the values here must be lowercase.
	values map[string]struct{}

	caseSensitive bool
}

func (m *equalMultiStringMatcher) Matches(s string) bool {
	if !m.caseSensitive {
		s = strings.ToLower(s)
	}

	_, ok := m.values[s]
	return ok
}

// anyStringWithoutNewlineMatcher is a stringMatcher which matches any string
// (including an empty one) as far as it doesn't contain any newline character.
type anyStringWithoutNewlineMatcher struct{}

func (m anyStringWithoutNewlineMatcher) Matches(s string) bool {
	// We need to make sure it doesn't contain a newline. Since the newline is
	// an ASCII character, we can use strings.IndexByte().
	return strings.IndexByte(s, '\n') == -1
}

// anyNonEmptyStringMatcher is a stringMatcher which matches any non-empty string.
type anyNonEmptyStringMatcher struct {
	matchNL bool
}

func (m *anyNonEmptyStringMatcher) Matches(s string) bool {
	if m.matchNL {
		// It's OK if the string contains a newline so we just need to make
		// sure it's non-empty.
		return len(s) > 0
	}

	// We need to make sure it non-empty and doesn't contain a newline.
	// Since the newline is an ASCII character, we can use strings.IndexByte().
	return len(s) > 0 && strings.IndexByte(s, '\n') == -1
}

// trueMatcher is a stringMatcher which matches any string (always returns true).
type trueMatcher struct{}

func (m trueMatcher) Matches(_ string) bool {
	return true
}

// optimizeEqualStringMatchers optimize a specific case where all matchers are made by an
// alternation (orStringMatcher) of strings checked for equality (equalStringMatcher). In
// this specific case, when we have many strings to match against we can use a map instead
// of iterating over the list of strings.
func optimizeEqualStringMatchers(input StringMatcher, threshold int) StringMatcher {
	var (
		caseSensitive    bool
		caseSensitiveSet bool
		numValues        int
	)

	// Analyse the input StringMatcher to count the number of occurrences
	// and ensure all of them have the same case sensitivity.
	analyseCallback := func(matcher *equalStringMatcher) bool {
		// Ensure we don't have mixed case sensitivity.
		if caseSensitiveSet && caseSensitive != matcher.caseSensitive {
			return false
		} else if !caseSensitiveSet {
			caseSensitive = matcher.caseSensitive
			caseSensitiveSet = true
		}

		numValues++
		return true
	}

	if !findEqualStringMatchers(input, analyseCallback) {
		return input
	}

	// If the number of values found is less than the threshold, then we should skip the optimization.
	if numValues < threshold {
		return input
	}

	// Parse again the input StringMatcher to extract all values and storing them.
	// We can skip the case sensitivity check because we've already checked it and
	// if the code reach this point then it means all matchers have the same case sensitivity.
	values := make(map[string]struct{}, numValues)

	// Ignore the return value because we already iterated over the input StringMatcher
	// and it was all good.
	findEqualStringMatchers(input, func(matcher *equalStringMatcher) bool {
		if caseSensitive {
			values[matcher.s] = struct{}{}
		} else {
			values[strings.ToLower(matcher.s)] = struct{}{}
		}

		return true
	})

	return &equalMultiStringMatcher{
		values:        values,
		caseSensitive: caseSensitive,
	}
}

// findEqualStringMatchers analyze the input StringMatcher and calls the callback for each
// equalStringMatcher found. Returns true if and only if the input StringMatcher is *only*
// composed by an alternation of equalStringMatcher.
func findEqualStringMatchers(input StringMatcher, callback func(matcher *equalStringMatcher) bool) bool {
	orInput, ok := input.(orStringMatcher)
	if !ok {
		return false
	}

	for _, m := range orInput {
		switch casted := m.(type) {
		case orStringMatcher:
			if !findEqualStringMatchers(m, callback) {
				return false
			}

		case *equalStringMatcher:
			if !callback(casted) {
				return false
			}

		default:
			// It's not an equal string matcher, so we have to stop searching
			// cause this optimization can't be applied.
			return false
		}
	}

	return true
}
