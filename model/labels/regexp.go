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

package labels

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/regexp"
	"github.com/grafana/regexp/syntax"
	"golang.org/x/text/unicode/norm"
)

const (
	maxSetMatches = 256

	// The minimum number of alternate values a regex should have to trigger
	// the optimization done by optimizeEqualOrPrefixStringMatchers() and so use a map
	// to match values instead of iterating over a list. This value has
	// been computed running BenchmarkOptimizeEqualStringMatchers.
	minEqualMultiStringMatcherMapThreshold = 16
)

type FastRegexMatcher struct {
	// Under some conditions, re is nil because the expression is never parsed.
	// We store the original string to be able to return it in GetRegexString().
	reString string
	re       *regexp.Regexp

	setMatches    []string
	stringMatcher StringMatcher

	// matchString is the "compiled" function to run by MatchString().
	matchString func(string) bool
}

func NewFastRegexMatcher(v string) (*FastRegexMatcher, error) {
	m := &FastRegexMatcher{
		reString: v,
	}

	m.stringMatcher, m.setMatches = optimizeAlternatingLiterals(v)
	if m.stringMatcher != nil {
		// If we already have a string matcher, we don't need to parse the regex
		// or compile the matchString function. This also avoids the behavior in
		// compileMatchStringFunction where it prefers to use setMatches when
		// available, even if the string matcher is faster.
		m.matchString = m.stringMatcher.Matches
	} else {
		parsed, err := syntax.Parse(v, syntax.Perl|syntax.DotNL)
		if err != nil {
			return nil, err
		}

		parsed = optimizeAlternatingSimpleContains(parsed)

		m.re, err = regexp.Compile("^(?s:" + parsed.String() + ")$")
		if err != nil {
			return nil, err
		}

		// Remove any capture operations before trying to optimize the remaining operations.
		clearCapture(parsed)

		if matches, caseSensitive := findSetMatches(parsed); len(matches) > 0 {
			if caseSensitive {
				m.setMatches = matches
			}
			if len(matches) > 1 {
				emsm := newEqualMultiStringMatcher(caseSensitive, len(matches), 0, 0)
				for _, match := range matches {
					emsm.add(match)
				}
				m.stringMatcher = emsm
			}
		}

		if m.stringMatcher == nil {
			m.stringMatcher = stringMatcherFromRegexp(parsed)
		}
		// Fall back to the compiled regexp itself, where a literal pre-filter still pays for itself because the regexp, not a matcher tree testing the same literals, does the matching.
		if m.stringMatcher == nil {
			m.stringMatcher = newRegexpStringMatcher(m.re, requiredLiteralsInOrder(parsed))
		}

		m.matchString = m.compileMatchStringFunction()
	}

	return m, nil
}

// compileMatchStringFunction returns the function to run by MatchString().
func (m *FastRegexMatcher) compileMatchStringFunction() func(string) bool {
	// Special case for a single element matcher (equality).
	if len(m.setMatches) == 1 {
		return func(s string) bool { return s == m.setMatches[0] }
	}

	// Inline the literal prefix test rather than dispatching into the matcher for it: most values are rejected by it, so keeping it out of the interface call matters.
	if p, ok := m.stringMatcher.(*literalPrefixSensitiveStringMatcher); ok {
		prefix, right := p.prefix, p.right
		return func(s string) bool {
			return strings.HasPrefix(s, prefix) && right.Matches(s[len(prefix):])
		}
	}
	if prefix, caseSensitive := requiredLiteralPrefix(m.stringMatcher); prefix != "" {
		matches := m.stringMatcher.Matches
		if caseSensitive {
			return func(s string) bool {
				return strings.HasPrefix(s, prefix) && matches(s)
			}
		}
		return func(s string) bool {
			return hasPrefixCaseInsensitive(s, prefix) && matches(s)
		}
	}

	return m.stringMatcher.Matches
}

// requiredLiteralPrefix returns a literal that every matching string must start with, and whether it must match byte-for-byte, or "" if the matcher doesn't imply one. It's only a rejection aid: the matcher itself remains authoritative.
func requiredLiteralPrefix(m StringMatcher) (prefix string, caseSensitive bool) {
	switch v := m.(type) {
	case *containsInOrderStringMatcher:
		return v.prefix, true
	case *containsStringMatcher:
		// With no left matcher the substrings are anchored at the start, so whatever they share must be present.
		if v.left == nil {
			return longestCommonPrefix(v.substrings), true
		}
		// A left side that must equal a fixed string is itself an anchored prefix.
		if eq, ok := v.left.(*equalStringMatcher); ok && !eq.caseSensitive {
			return eq.s, false
		}
	case *equalMultiStringSliceMatcher:
		// Case-sensitive sets already reject cheaply on the length bitmask, so a prefix test would only add work there.
		if !v.caseSensitive {
			return longestCommonPrefix(v.values), false
		}
	}
	return "", true
}

func longestCommonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	prefix := ss[0]
	for _, s := range ss[1:] {
		for prefix != "" && !strings.HasPrefix(s, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		if prefix == "" {
			return ""
		}
	}
	return prefix
}

// IsOptimized returns true if any fast-path optimization is applied to the
// regex matcher.
func (m *FastRegexMatcher) IsOptimized() bool {
	if len(m.setMatches) > 0 {
		return true
	}
	if _, ok := m.stringMatcher.(*regexpStringMatcher); ok {
		return false
	}
	return m.stringMatcher != nil
}

// Prefix returns the required case-sensitive literal prefix of the value to match byte-for-byte, or "" if there's none (including when it would only be case-insensitive).
func (m *FastRegexMatcher) Prefix() string {
	if p, ok := m.stringMatcher.(*literalPrefixSensitiveStringMatcher); ok {
		return p.prefix
	}
	return ""
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
		for i := 0; i+1 < len(re.Rune); i += 2 {
			totalSet += int(re.Rune[i+1]-re.Rune[i]) + 1
		}
		// limits the total characters that can be used to create matches.
		// In some case like negation [^0-9] a lot of possibilities exists and that
		// can create thousands of possible matches at which points we're better off using regexp.
		if totalSet > maxSetMatches {
			return nil, false
		}
		for i := 0; i+1 < len(re.Rune); i += 2 {
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
		// Iterate on the regexp because capture groups could be nested.
		for r.Op == syntax.OpCapture {
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

// tooManyMatches guards against creating too many set matches.
func tooManyMatches(matches []string, added ...string) bool {
	return len(matches)+len(added) > maxSetMatches
}

func (m *FastRegexMatcher) MatchString(s string) bool {
	return m.matchString(s)
}

func (m *FastRegexMatcher) SetMatches() []string {
	// IMPORTANT: always return a copy, otherwise if the caller manipulate this slice it will
	// also get manipulated in the cached FastRegexMatcher instance.
	return slices.Clone(m.setMatches)
}

func (m *FastRegexMatcher) GetRegexString() string {
	return m.reString
}

// optimizeAlternatingLiterals optimizes a regex of the form
//
//	`literal1|literal2|literal3|...`
//
// this function returns an optimized StringMatcher or nil if the regex
// cannot be optimized in this way, and a list of setMatches up to maxSetMatches.
func optimizeAlternatingLiterals(s string) (StringMatcher, []string) {
	if s == "" {
		return emptyStringMatcher{}, nil
	}

	estimatedAlternates := strings.Count(s, "|") + 1

	// If there are no alternates, check if the string is a literal
	if estimatedAlternates == 1 {
		if regexp.QuoteMeta(s) == s {
			return &equalStringMatcher{s: s, caseSensitive: true}, []string{s}
		}
		return nil, nil
	}

	multiMatcher := newEqualMultiStringMatcher(true, estimatedAlternates, 0, 0)

	for end := strings.IndexByte(s, '|'); end > -1; end = strings.IndexByte(s, '|') {
		// Split the string into the next literal and the remainder
		subMatch := s[:end]
		s = s[end+1:]

		// break if any of the submatches are not literals
		if regexp.QuoteMeta(subMatch) != subMatch {
			return nil, nil
		}

		multiMatcher.add(subMatch)
	}

	// break if the remainder is not a literal
	if regexp.QuoteMeta(s) != s {
		return nil, nil
	}
	multiMatcher.add(s)

	return multiMatcher, multiMatcher.setMatches()
}

// optimizeAlternatingSimpleContains checks to see if a regex is a series of alternations that take the form .*literal.*
// In these cases, the regex itself can be rewritten as .*(foo|bar).*,
// which can result in a significant performance improvement at execution.
func optimizeAlternatingSimpleContains(r *syntax.Regexp) *syntax.Regexp {
	if r.Op != syntax.OpAlternate {
		return r
	}
	containsLiterals := make([]*syntax.Regexp, 0, len(r.Sub))
	for _, sub := range r.Sub {
		// If any subexpression does not take the form .*literal.*, we should not try to optimize this
		if sub.Op != syntax.OpConcat || len(sub.Sub) != 3 {
			return r
		}
		concatSubs := sub.Sub
		if !isCaseSensitiveLiteral(concatSubs[1]) || !isMatchAny(concatSubs[0]) || !isMatchAny(concatSubs[2]) {
			return r
		}
		containsLiterals = append(containsLiterals, concatSubs[1])
	}

	// Only rewrite the regex if there's more than one literal
	if len(containsLiterals) > 1 {
		returnRegex := &syntax.Regexp{Op: syntax.OpConcat}
		prefixAnyMatcher := &syntax.Regexp{Op: syntax.OpStar, Sub: []*syntax.Regexp{{Op: syntax.OpAnyChar}}, Flags: syntax.Perl | syntax.DotNL}
		suffixAnyMatcher := &syntax.Regexp{Op: syntax.OpStar, Sub: []*syntax.Regexp{{Op: syntax.OpAnyChar}}, Flags: syntax.Perl | syntax.DotNL}
		alts := &syntax.Regexp{Op: syntax.OpAlternate}
		alts.Sub = containsLiterals
		returnRegex.Sub = []*syntax.Regexp{
			prefixAnyMatcher,
			alts,
			suffixAnyMatcher,
		}
		return returnRegex
	}
	return r
}

// StringMatcher is a matcher that matches a string in place of a regular expression.
type StringMatcher interface {
	Matches(s string) bool
}

// stringMatcherFromRegexp attempts to replace a common regexp with a string matcher.
// It returns nil if the regexp is not supported.
func stringMatcherFromRegexp(re *syntax.Regexp) StringMatcher {
	clearBeginEndText(re)

	m := stringMatcherFromRegexpInternal(re)
	m = optimizeEqualOrPrefixStringMatchers(m, minEqualMultiStringMatcherMapThreshold)

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
	case syntax.OpQuest:
		// Only optimize for ".?".
		if len(re.Sub) != 1 || (re.Sub[0].Op != syntax.OpAnyChar && re.Sub[0].Op != syntax.OpAnyCharNotNL) {
			return nil
		}

		return &zeroOrOneCharacterStringMatcher{
			matchNL: re.Sub[0].Op == syntax.OpAnyChar,
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

		// Preserved so an edge quantifier that fails to resolve below can still fall back to peeling a literal prefix/suffix instead of giving up entirely.
		originalSubs := re.Sub

		var left, right StringMatcher
		var leftSub, rightSub *syntax.Regexp

		// Let's try to find if there's a first and last any matchers.
		if re.Sub[0].Op == syntax.OpPlus || re.Sub[0].Op == syntax.OpStar || re.Sub[0].Op == syntax.OpQuest {
			left = stringMatcherFromRegexpInternal(re.Sub[0])
			if left == nil {
				return stringMatcherLiteralPeelFallback(originalSubs, nil, nil, nil, nil)
			}
			leftSub = re.Sub[0]
			re.Sub = re.Sub[1:]
		}
		if re.Sub[len(re.Sub)-1].Op == syntax.OpPlus || re.Sub[len(re.Sub)-1].Op == syntax.OpStar || re.Sub[len(re.Sub)-1].Op == syntax.OpQuest {
			right = stringMatcherFromRegexpInternal(re.Sub[len(re.Sub)-1])
			if right == nil {
				return stringMatcherLiteralPeelFallback(originalSubs, nil, nil, nil, nil)
			}
			rightSub = re.Sub[len(re.Sub)-1]
			re.Sub = re.Sub[:len(re.Sub)-1]
		}

		matches, matchesCaseSensitive := findSetMatchesInternal(re, "")

		if len(matches) == 0 && len(re.Sub) == 2 {
			// We have not find fixed set matches. We look for other known cases that
			// we can optimize.
			switch {
			// Prefix is literal.
			case right == nil && re.Sub[0].Op == syntax.OpLiteral:
				right = stringMatcherFromRegexpInternal(re.Sub[1])
				if right != nil {
					matches = []string{string(re.Sub[0].Rune)}
					matchesCaseSensitive = !isCaseInsensitive(re.Sub[0])
				}

			// Suffix is literal.
			case left == nil && re.Sub[1].Op == syntax.OpLiteral:
				left = stringMatcherFromRegexpInternal(re.Sub[0])
				if left != nil {
					matches = []string{string(re.Sub[1].Rune)}
					matchesCaseSensitive = !isCaseInsensitive(re.Sub[1])
				}
			}
		}

		if len(matches) == 0 {
			// Decompose the remaining subs into alternating literal/fixed-set runs separated by unbounded wildcard gaps (e.g. "foo.*hello.*bar", ".*-.*-.*-.*-.*").
			if m := stringMatcherFromRunsAndGaps(re.Sub, left, right); m != nil {
				return m
			}
			// Otherwise peel a single leading/trailing literal and wrap the rest as a compiled-regexp leaf, so Matcher.Prefix() still sees a prefix even when the remainder is too complex.
			if m := stringMatcherLiteralPeelFallback(re.Sub, left, right, leftSub, rightSub); m != nil {
				return m
			}
			return nil
		}

		// Use the right (and best) matcher based on what we've found.
		switch {
		// No left and right matchers (only fixed set matches).
		case left == nil && right == nil:
			// if there's no any matchers on both side it's a concat of literals
			or := make([]StringMatcher, 0, len(matches))
			for _, match := range matches {
				or = append(or, &equalStringMatcher{
					s:             match,
					caseSensitive: matchesCaseSensitive,
				})
			}
			return orStringMatcher(or)

		// Right matcher with 1 fixed set match.
		case left == nil && len(matches) == 1:
			return newLiteralPrefixStringMatcher(matches[0], matchesCaseSensitive, right)

		// Left matcher with 1 fixed set match.
		case right == nil && len(matches) == 1:
			return &literalSuffixStringMatcher{
				left:                left,
				suffix:              matches[0],
				suffixCaseSensitive: matchesCaseSensitive,
			}

		// We found literals in the middle. We can trigger the fast path only if
		// the matches are case sensitive because containsStringMatcher doesn't
		// support case insensitive.
		case matchesCaseSensitive:
			return &containsStringMatcher{
				substrings: matches,
				left:       left,
				right:      right,
			}
		}
	}
	return nil
}

// regexpStringMatcher matches using a compiled regexp; it's the fallback leaf for sub-expressions that can't be reduced to a more specific StringMatcher.
type regexpStringMatcher struct {
	re *regexp.Regexp

	// required are literals that any match must contain in this order, used to reject values without running the regexp; it may be empty and never causes a match on its own.
	required []string
}

func (m *regexpStringMatcher) Matches(s string) bool {
	// Rule the value out on the literals it must contain before paying for the regexp engine.
	off := 0
	for _, sub := range m.required {
		i := strings.Index(s[off:], sub)
		if i < 0 {
			return false
		}
		off += i + len(sub)
	}
	return m.re.MatchString(s)
}

func newRegexpStringMatcher(re *regexp.Regexp, required []string) *regexpStringMatcher {
	return &regexpStringMatcher{re: re, required: required}
}

// requiredLiteralsInOrder returns the case-sensitive literals of a concatenation, in order. Every element of a concatenation has to match, so a value that doesn't contain these in order cannot match the whole expression.
func requiredLiteralsInOrder(re *syntax.Regexp) []string {
	if re.Op != syntax.OpConcat {
		return nil
	}
	var out []string
	for _, sub := range re.Sub {
		if sub.Op == syntax.OpLiteral && isCaseSensitive(sub) {
			out = append(out, string(sub.Rune))
		}
	}
	return out
}

// regexpLeafFromSubs compiles an anchored regexp matching exactly the concatenation of subs, wrapped as a StringMatcher.
func regexpLeafFromSubs(subs []*syntax.Regexp) StringMatcher {
	if len(subs) == 0 {
		return emptyStringMatcher{}
	}
	concat := &syntax.Regexp{Op: syntax.OpConcat, Sub: subs}
	// Each sub already carries the DotNL flag from the original parse, so no need to add our own (?s:...) here.
	re, err := regexp.Compile("^(?:" + concat.String() + ")$")
	if err != nil {
		return nil
	}
	return &regexpStringMatcher{re: re}
}

// isUnboundedAnyWildcard reports whether re is an unbounded "any character" repeat (.* or .+), i.e. a gap of any length with no other constraint.
func isUnboundedAnyWildcard(re *syntax.Regexp) bool {
	if re.Op != syntax.OpStar && re.Op != syntax.OpPlus {
		return false
	}
	return re.Sub[0].Op == syntax.OpAnyChar || re.Sub[0].Op == syntax.OpAnyCharNotNL
}

// splitRunsAndGaps splits subs into alternating runs (stretches with no unbounded any-wildcard) and the gaps (the wildcards) between them; len(runs) == len(gaps)+1.
func splitRunsAndGaps(subs []*syntax.Regexp) (runs [][]*syntax.Regexp, gaps []*syntax.Regexp) {
	var cur []*syntax.Regexp
	for _, s := range subs {
		if isUnboundedAnyWildcard(s) {
			runs = append(runs, cur)
			gaps = append(gaps, s)
			cur = nil
		} else {
			cur = append(cur, s)
		}
	}
	runs = append(runs, cur)
	return runs, gaps
}

// stringMatcherFromRunsAndGaps generalizes literal-prefix/literal-suffix/contains composition to an arbitrary number of literal-or-fixed-set runs separated by unbounded wildcard gaps (e.g. "foo.*hello.*bar", ".*-.*-.*-.*-.*"); outerLeft/outerRight are the matchers already built by the caller for a wildcard bordering subs on that side, nil meaning that side is anchored to the true start/end; returns nil if subs can't be decomposed this way.
func stringMatcherFromRunsAndGaps(subs []*syntax.Regexp, outerLeft, outerRight StringMatcher) StringMatcher {
	runSubs, gapSubs := splitRunsAndGaps(subs)
	if len(gapSubs) == 0 {
		return nil
	}

	n := len(runSubs)
	runs := make([][]string, n)
	runsCS := make([]bool, n)
	for i, rs := range runSubs {
		if len(rs) == 0 {
			return nil
		}
		concat := &syntax.Regexp{Op: syntax.OpConcat, Sub: rs}
		m, cs := findSetMatchesInternal(concat, "")
		if len(m) == 0 {
			return nil
		}
		runs[i] = m
		runsCS[i] = cs
	}

	gaps := make([]StringMatcher, len(gapSubs))
	for i, g := range gapSubs {
		gm := stringMatcherFromRegexpInternal(g)
		if gm == nil {
			return nil
		}
		gaps[i] = gm
	}

	firstIsAnchor := outerLeft == nil
	lastIsAnchor := outerRight == nil

	// Runs located via substring search must be case-sensitive, since containsStringMatcher doesn't support case-insensitive substrings.
	for i := 1; i < n-1; i++ {
		if !runsCS[i] {
			return nil
		}
	}
	if !firstIsAnchor && !runsCS[0] {
		return nil
	}
	if !lastIsAnchor && !runsCS[n-1] {
		return nil
	}

	// Prefer a single greedy scan when every literal is separated by an unconstrained gap: it's exact for that shape and avoids the nested chain's backtracking search.
	if m := containsInOrderFromRuns(runs, runsCS, gaps, outerLeft, outerRight); m != nil {
		return m
	}

	lastGap := gaps[n-2]
	var tail StringMatcher
	if lastIsAnchor {
		tail = orLiteralSuffixMatcher(runs[n-1], runsCS[n-1], lastGap)
	} else {
		tail = &containsStringMatcher{substrings: runs[n-1], left: lastGap, right: outerRight}
	}

	for i := n - 2; i >= 1; i-- {
		tail = &containsStringMatcher{substrings: runs[i], left: gaps[i-1], right: tail}
	}

	if firstIsAnchor {
		return orLiteralPrefixMatcher(runs[0], runsCS[0], tail)
	}
	return &containsStringMatcher{substrings: runs[0], left: outerLeft, right: tail}
}

func orLiteralPrefixMatcher(matches []string, caseSensitive bool, rest StringMatcher) StringMatcher {
	if len(matches) == 1 {
		return newLiteralPrefixStringMatcher(matches[0], caseSensitive, rest)
	}
	or := make([]StringMatcher, len(matches))
	for i, s := range matches {
		or[i] = newLiteralPrefixStringMatcher(s, caseSensitive, rest)
	}
	return orStringMatcher(or)
}

func orLiteralSuffixMatcher(matches []string, caseSensitive bool, left StringMatcher) StringMatcher {
	if len(matches) == 1 {
		return &literalSuffixStringMatcher{left: left, suffix: matches[0], suffixCaseSensitive: caseSensitive}
	}
	or := make([]StringMatcher, len(matches))
	for i, s := range matches {
		or[i] = &literalSuffixStringMatcher{left: left, suffix: s, suffixCaseSensitive: caseSensitive}
	}
	return orStringMatcher(or)
}

// containsInOrderStringMatcher matches a string starting with prefix, ending with suffix, and containing each of the contains literals in order in between (an empty prefix/suffix means that side is unanchored). It is an exact matcher, not a pre-check: it is only built when every literal is separated by an unconstrained gap, where a greedy left-to-right scan is equivalent to the regexp because a literal occurring after any occurrence of the previous one also occurs after the first.
type containsInOrderStringMatcher struct {
	prefix   string
	contains []string
	suffix   string

	// minLeading/minTrailing are the characters that must precede the first literal and follow the last one when that side is unanchored but non-empty (i.e. the outer gap was ".+" not ".*").
	minLeading  int
	minTrailing int

	// minLen is the shortest string that can possibly match, checked first as a cheap rejection.
	minLen int
}

func (m *containsInOrderStringMatcher) Matches(s string) bool {
	if len(s) < m.minLen {
		return false
	}

	start, end := m.minLeading, len(s)-m.minTrailing
	if m.prefix != "" {
		if !strings.HasPrefix(s, m.prefix) {
			return false
		}
		start = len(m.prefix)
	}
	if m.suffix != "" {
		if !strings.HasSuffix(s, m.suffix) {
			return false
		}
		end = len(s) - len(m.suffix)
	}
	if start > end {
		return false
	}

	mid := s[start:end]
	for _, sub := range m.contains {
		i := strings.Index(mid, sub)
		if i < 0 {
			return false
		}
		mid = mid[i+len(sub):]
	}
	return true
}

// unconstrainedGapLen returns the minimum length a gap matcher consumes, and whether it accepts any character (so a greedy scan across it is exact).
func unconstrainedGapLen(m StringMatcher) (minLen int, ok bool) {
	switch v := m.(type) {
	case trueMatcher:
		return 0, true
	case *anyNonEmptyStringMatcher:
		if v.matchNL {
			return 1, true
		}
	}
	return 0, false
}

// containsInOrderFromRuns collapses a run/gap decomposition into a single containsInOrderStringMatcher, or returns nil if the shape doesn't allow an exact greedy scan.
func containsInOrderFromRuns(runs [][]string, runsCS []bool, gaps []StringMatcher, outerLeft, outerRight StringMatcher) StringMatcher {
	// Every literal must be a single case-sensitive string: alternations would need to be tried independently, which a single scan can't express.
	for i, r := range runs {
		if len(r) != 1 || !runsCS[i] {
			return nil
		}
	}
	// Interior gaps must be fully unconstrained, otherwise the scan would have to enforce a minimum distance between consecutive literals.
	for _, g := range gaps {
		if !isTrueMatcher(g) {
			return nil
		}
	}

	m := &containsInOrderStringMatcher{}
	lits := make([]string, 0, len(runs))
	for _, r := range runs {
		lits = append(lits, r[0])
	}

	if outerLeft == nil {
		m.prefix = lits[0]
		lits = lits[1:]
	} else {
		minLen, ok := unconstrainedGapLen(outerLeft)
		if !ok {
			return nil
		}
		m.minLeading = minLen
	}

	if outerRight == nil {
		m.suffix = lits[len(lits)-1]
		lits = lits[:len(lits)-1]
	} else {
		minLen, ok := unconstrainedGapLen(outerRight)
		if !ok {
			return nil
		}
		m.minTrailing = minLen
	}

	m.contains = lits
	m.minLen = len(m.prefix) + len(m.suffix) + m.minLeading + m.minTrailing
	for _, l := range m.contains {
		m.minLen += len(l)
	}
	return m
}

// stringMatcherLiteralPeelFallback peels a leading/trailing literal not already consumed by outerLeft/outerRight and compiles a single regexp leaf for whatever remains (including leftSub/rightSub), so callers like Matcher.Prefix() still see a literal prefix even when subs is too complex to reduce to an exact StringMatcher.
func stringMatcherLiteralPeelFallback(subs []*syntax.Regexp, outerLeft, outerRight StringMatcher, leftSub, rightSub *syntax.Regexp) StringMatcher {
	inner := subs

	var prefix string
	var prefixCS bool
	havePrefix := false
	if outerLeft == nil && len(inner) > 0 && inner[0].Op == syntax.OpLiteral {
		prefix = string(inner[0].Rune)
		prefixCS = !isCaseInsensitive(inner[0])
		havePrefix = true
		inner = inner[1:]
	}

	var suffix string
	var suffixCS bool
	haveSuffix := false
	if outerRight == nil && len(inner) > 0 && inner[len(inner)-1].Op == syntax.OpLiteral {
		last := inner[len(inner)-1]
		suffix = string(last.Rune)
		suffixCS = !isCaseInsensitive(last)
		haveSuffix = true
		inner = inner[:len(inner)-1]
	}

	if !havePrefix && !haveSuffix {
		return nil
	}

	coreSubs := make([]*syntax.Regexp, 0, len(inner)+2)
	if leftSub != nil {
		coreSubs = append(coreSubs, leftSub)
	}
	coreSubs = append(coreSubs, inner...)
	if rightSub != nil {
		coreSubs = append(coreSubs, rightSub)
	}

	core := regexpLeafFromSubs(coreSubs)
	if core == nil {
		return nil
	}

	if haveSuffix {
		core = &literalSuffixStringMatcher{left: core, suffix: suffix, suffixCaseSensitive: suffixCS}
	}
	if havePrefix {
		core = newLiteralPrefixStringMatcher(prefix, prefixCS, core)
	}
	return core
}

func isMatchAny(re *syntax.Regexp) bool {
	return re.Op == syntax.OpStar && re.Sub[0].Op == syntax.OpAnyChar
}

func isCaseSensitiveLiteral(re *syntax.Regexp) bool {
	return re.Op == syntax.OpLiteral && isCaseSensitive(re)
}

// containsStringMatcher matches a string if it contains any of the substrings.
// If left and right are not nil, it's a contains operation where left and right must match.
// If left is nil, it's a hasPrefix operation and right must match.
// Finally, if right is nil it's a hasSuffix operation and left must match.
type containsStringMatcher struct {
	// The matcher that must match the left side. Can be nil.
	left StringMatcher

	// At least one of these strings must match in the "middle", between left and right matchers.
	substrings []string

	// The matcher that must match the right side. Can be nil.
	right StringMatcher
}

func (m *containsStringMatcher) Matches(s string) bool {
	for _, substr := range m.substrings {
		switch {
		case m.right != nil && m.left != nil:
			// Fast path: if both sides are unconstrained, any occurrence of substr is a match.
			if isTrueMatcher(m.left) && isTrueMatcher(m.right) {
				if strings.Contains(s, substr) {
					return true
				}
				continue
			}

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
		case m.left != nil:
			// If we have to check for characters on the left then we need to match a suffix.
			if strings.HasSuffix(s, substr) && m.left.Matches(s[:len(s)-len(substr)]) {
				return true
			}
		case m.right != nil:
			if strings.HasPrefix(s, substr) && m.right.Matches(s[len(substr):]) {
				return true
			}
		}
	}
	return false
}

func newLiteralPrefixStringMatcher(prefix string, prefixCaseSensitive bool, right StringMatcher) StringMatcher {
	if prefixCaseSensitive {
		return &literalPrefixSensitiveStringMatcher{
			prefix: prefix,
			right:  right,
		}
	}

	return &literalPrefixInsensitiveStringMatcher{
		prefix: prefix,
		right:  right,
	}
}

// literalPrefixSensitiveStringMatcher matches a string with the given literal case-sensitive prefix and right side matcher.
type literalPrefixSensitiveStringMatcher struct {
	prefix string

	// The matcher that must match the right side. Can be nil.
	right StringMatcher
}

func (m *literalPrefixSensitiveStringMatcher) Matches(s string) bool {
	if !strings.HasPrefix(s, m.prefix) {
		return false
	}

	// Ensure the right side matches.
	return m.right.Matches(s[len(m.prefix):])
}

// literalPrefixInsensitiveStringMatcher matches a string with the given literal case-insensitive prefix and right side matcher.
type literalPrefixInsensitiveStringMatcher struct {
	prefix string

	// The matcher that must match the right side. Can be nil.
	right StringMatcher
}

func (m *literalPrefixInsensitiveStringMatcher) Matches(s string) bool {
	prefixLen, ok := prefixCaseInsensitiveMatchLen(s, m.prefix)
	if !ok {
		return false
	}

	// Ensure the right side matches.
	return m.right.Matches(s[prefixLen:])
}

// literalSuffixStringMatcher matches a string with the given literal suffix and left side matcher.
type literalSuffixStringMatcher struct {
	// The matcher that must match the left side. Can be nil.
	left StringMatcher

	suffix              string
	suffixCaseSensitive bool
}

func (m *literalSuffixStringMatcher) Matches(s string) bool {
	// Ensure the suffix matches.
	if m.suffixCaseSensitive {
		if !strings.HasSuffix(s, m.suffix) {
			return false
		}

		// Ensure the left side matches.
		return m.left.Matches(s[:len(s)-len(m.suffix)])
	}

	suffixLen, ok := suffixCaseInsensitiveMatchLen(s, m.suffix)
	if !ok {
		return false
	}

	// Ensure the left side matches.
	return m.left.Matches(s[:len(s)-suffixLen])
}

// emptyStringMatcher matches an empty string.
type emptyStringMatcher struct{}

func (emptyStringMatcher) Matches(s string) bool {
	return s == ""
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

type multiStringMatcherBuilder interface {
	StringMatcher
	add(s string)
	addPrefix(prefix string, prefixCaseSensitive bool, matcher StringMatcher)
	setMatches() []string
}

func newEqualMultiStringMatcher(caseSensitive bool, estimatedSize, estimatedPrefixes, minPrefixLength int) multiStringMatcherBuilder {
	// If the estimated size is low enough, it's faster to use a slice instead of a map.
	if estimatedSize < minEqualMultiStringMatcherMapThreshold && estimatedPrefixes == 0 {
		return &equalMultiStringSliceMatcher{caseSensitive: caseSensitive, values: make([]string, 0, estimatedSize)}
	}

	return &equalMultiStringMapMatcher{
		values:        make(map[string]struct{}, estimatedSize),
		prefixes:      make(map[string][]StringMatcher, estimatedPrefixes),
		minPrefixLen:  minPrefixLength,
		caseSensitive: caseSensitive,
	}
}

// equalMultiStringSliceMatcher matches a string exactly against a slice of valid values.
type equalMultiStringSliceMatcher struct {
	values []string
	// lengthsMask is a bitmask of the lengths of the strings in values.
	// If the bit at position i is set, it means that there's at least one string of length i in values.
	// It's like a bloom filter but we don't hash, we just take the values.
	// Bit 64 means there are strings longer than 63 characters.
	// This can be used to filter case-sensitive values.
	// Case-insensitive Unicode strings can have different lengths when case folded.
	lengthsMask uint64

	caseSensitive bool
}

func (m *equalMultiStringSliceMatcher) add(s string) {
	m.values = append(m.values, s)
	m.lengthsMask |= lengthMask(s)
}

func (*equalMultiStringSliceMatcher) addPrefix(string, bool, StringMatcher) {
	panic("not implemented")
}

func (m *equalMultiStringSliceMatcher) setMatches() []string {
	return m.values
}

func (m *equalMultiStringSliceMatcher) Matches(s string) bool {
	if m.caseSensitive {
		return m.lengthsMask&lengthMask(s) > 0 && slices.Contains(m.values, s)
	}
	for _, v := range m.values {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

// equalMultiStringMapMatcher matches a string exactly against a map of valid values
// or against a set of prefix matchers.
type equalMultiStringMapMatcher struct {
	// values contains values to match a string against. If the matching is case insensitive,
	// the values here must be lowercase.
	values map[string]struct{}
	// lengthsMask is a bitmask of the lengths of the strings in values.
	// If the bit at position i is set, it means that there's at least one string of length i in values.
	// It's like a bloom filter but we don't hash, we just take the values.
	// Bit 64 means there are strings longer than 63 characters.
	// This can be used to filter case-sensitive values.
	// Case-insensitive Unicode strings can have different lengths when case folded.
	lengthsMask uint64
	// prefixes maps strings, all of length minPrefixLen, to sets of matchers to check the rest of the string.
	// If the matching is case insensitive, prefixes are all lowercase.
	prefixes map[string][]StringMatcher
	// minPrefixLen can be zero, meaning there are no prefix matchers.
	minPrefixLen  int
	caseSensitive bool
}

func (m *equalMultiStringMapMatcher) add(s string) {
	if !m.caseSensitive {
		s = toNormalisedLower(s, nil) // Don't pass a stack buffer here - it will always escape to heap.
	} else {
		m.lengthsMask |= lengthMask(s)
	}

	m.values[s] = struct{}{}
}

func (m *equalMultiStringMapMatcher) addPrefix(prefix string, prefixCaseSensitive bool, matcher StringMatcher) {
	if m.minPrefixLen == 0 {
		panic("addPrefix called when no prefix length defined")
	}
	if len(prefix) < m.minPrefixLen {
		panic("addPrefix called with a too short prefix")
	}
	if m.caseSensitive != prefixCaseSensitive {
		panic("addPrefix called with a prefix whose case sensitivity is different than the expected one")
	}

	s := prefix[:m.minPrefixLen]
	if !m.caseSensitive {
		s = strings.ToLower(s)
	}

	m.prefixes[s] = append(m.prefixes[s], matcher)
}

func (m *equalMultiStringMapMatcher) setMatches() []string {
	if len(m.values) >= maxSetMatches || len(m.prefixes) > 0 {
		return nil
	}

	matches := make([]string, 0, len(m.values))
	for s := range m.values {
		matches = append(matches, s)
	}
	return matches
}

func (m *equalMultiStringMapMatcher) Matches(s string) bool {
	if len(m.values) > 0 {
		if m.minPrefixLen == 0 && m.caseSensitive && m.lengthsMask&lengthMask(s) == 0 {
			return false
		}
		sNorm := s
		var a [32]byte
		if !m.caseSensitive {
			sNorm = toNormalisedLower(s, a[:])
		}
		if _, ok := m.values[sNorm]; ok {
			return true
		}
	}

	if m.minPrefixLen > 0 && len(s) >= m.minPrefixLen {
		prefix := s[:m.minPrefixLen]
		var a [32]byte
		if !m.caseSensitive {
			prefix = toNormalisedLower(s[:m.minPrefixLen], a[:])
		}
		for _, matcher := range m.prefixes[prefix] {
			if matcher.Matches(s) {
				return true
			}
		}
	}
	return false
}

// toNormalisedLower normalise the input string using "Unicode Normalization Form D" and then convert
// it to lower case.
func toNormalisedLower(s string, a []byte) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			return strings.Map(unicode.ToLower, norm.NFKD.String(s))
		}
		if 'A' <= c && c <= 'Z' {
			return toNormalisedLowerSlow(s, i, a)
		}
	}
	return s
}

// toNormalisedLowerSlow is split from toNormalisedLower because having a call
// to `copy` slows it down even when it is not called.
func toNormalisedLowerSlow(s string, i int, a []byte) string {
	var buf []byte
	if cap(a) > len(s) {
		buf = a[:len(s)]
		copy(buf, s)
	} else {
		buf = []byte(s)
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			return strings.Map(unicode.ToLower, norm.NFKD.String(s))
		}
		if 'A' <= c && c <= 'Z' {
			buf[i] = c + 'a' - 'A'
		}
	}
	return yoloString(buf)
}

// anyStringWithoutNewlineMatcher is a stringMatcher which matches any string
// (including an empty one) as far as it doesn't contain any newline character.
type anyStringWithoutNewlineMatcher struct{}

func (anyStringWithoutNewlineMatcher) Matches(s string) bool {
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
		return s != ""
	}

	// We need to make sure it non-empty and doesn't contain a newline.
	// Since the newline is an ASCII character, we can use strings.IndexByte().
	return s != "" && strings.IndexByte(s, '\n') == -1
}

// zeroOrOneCharacterStringMatcher is a StringMatcher which matches zero or one occurrence
// of any character. The newline character is matches only if matchNL is set to true.
type zeroOrOneCharacterStringMatcher struct {
	matchNL bool
}

func (m *zeroOrOneCharacterStringMatcher) Matches(s string) bool {
	// If there's more than one rune in the string, then it can't match.
	if r, size := utf8.DecodeRuneInString(s); r == utf8.RuneError {
		// Size is 0 for empty strings, 1 for invalid rune.
		// Empty string matches, invalid rune matches if there isn't anything else.
		return size == len(s)
	} else if size < len(s) {
		return false
	}

	// No need to check for the newline if the string is empty or matching a newline is OK.
	if m.matchNL || s == "" {
		return true
	}

	return s[0] != '\n'
}

// trueMatcher is a stringMatcher which matches any string (always returns true).
type trueMatcher struct{}

func (trueMatcher) Matches(string) bool {
	return true
}

// isTrueMatcher reports whether m is a trueMatcher, used to fast-path containsStringMatcher when a side is unconstrained.
func isTrueMatcher(m StringMatcher) bool {
	_, ok := m.(trueMatcher)
	return ok
}

// optimizeEqualOrPrefixStringMatchers optimize a specific case where all matchers are made by an
// alternation (orStringMatcher) of strings checked for equality (equalStringMatcher) or
// with a literal prefix (literalPrefixSensitiveStringMatcher or literalPrefixInsensitiveStringMatcher).
//
// In this specific case, when we have many strings to match against we can use a map instead
// of iterating over the list of strings.
func optimizeEqualOrPrefixStringMatchers(input StringMatcher, threshold int) StringMatcher {
	var (
		caseSensitive    bool
		caseSensitiveSet bool
		numValues        int
		numPrefixes      int
		minPrefixLength  int
	)

	// Analyse the input StringMatcher to count the number of occurrences
	// and ensure all of them have the same case sensitivity.
	analyseEqualMatcherCallback := func(matcher *equalStringMatcher) bool {
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

	analysePrefixMatcherCallback := func(prefix string, prefixCaseSensitive bool, _ StringMatcher) bool {
		// Ensure we don't have mixed case sensitivity.
		if caseSensitiveSet && caseSensitive != prefixCaseSensitive {
			return false
		} else if !caseSensitiveSet {
			caseSensitive = prefixCaseSensitive
			caseSensitiveSet = true
		}
		if numPrefixes == 0 || len(prefix) < minPrefixLength {
			minPrefixLength = len(prefix)
		}

		numPrefixes++
		return true
	}

	if !findEqualOrPrefixStringMatchers(input, analyseEqualMatcherCallback, analysePrefixMatcherCallback) {
		return input
	}

	// If the number of values and prefixes found is less than the threshold, then we should skip the optimization.
	if (numValues + numPrefixes) < threshold {
		return input
	}

	// Parse again the input StringMatcher to extract all values and storing them.
	// We can skip the case sensitivity check because we've already checked it and
	// if the code reach this point then it means all matchers have the same case sensitivity.
	multiMatcher := newEqualMultiStringMatcher(caseSensitive, numValues, numPrefixes, minPrefixLength)

	// Ignore the return value because we already iterated over the input StringMatcher
	// and it was all good.
	findEqualOrPrefixStringMatchers(input, func(matcher *equalStringMatcher) bool {
		multiMatcher.add(matcher.s)
		return true
	}, func(prefix string, _ bool, matcher StringMatcher) bool {
		multiMatcher.addPrefix(prefix, caseSensitive, matcher)
		return true
	})

	return multiMatcher
}

// findEqualOrPrefixStringMatchers analyze the input StringMatcher and calls the equalMatcherCallback for each
// equalStringMatcher found, and prefixMatcherCallback for each literalPrefixSensitiveStringMatcher and literalPrefixInsensitiveStringMatcher found.
//
// Returns true if and only if the input StringMatcher is *only* composed by an alternation of equalStringMatcher and/or
// literal prefix matcher. Returns false if prefixMatcherCallback is nil and a literal prefix matcher is encountered.
func findEqualOrPrefixStringMatchers(input StringMatcher, equalMatcherCallback func(matcher *equalStringMatcher) bool, prefixMatcherCallback func(prefix string, prefixCaseSensitive bool, matcher StringMatcher) bool) bool {
	orInput, ok := input.(orStringMatcher)
	if !ok {
		return false
	}

	for _, m := range orInput {
		switch casted := m.(type) {
		case orStringMatcher:
			if !findEqualOrPrefixStringMatchers(m, equalMatcherCallback, prefixMatcherCallback) {
				return false
			}

		case *equalStringMatcher:
			if !equalMatcherCallback(casted) {
				return false
			}

		case *literalPrefixSensitiveStringMatcher:
			if prefixMatcherCallback == nil || !prefixMatcherCallback(casted.prefix, true, casted) {
				return false
			}

		case *literalPrefixInsensitiveStringMatcher:
			if prefixMatcherCallback == nil || !prefixMatcherCallback(casted.prefix, false, casted) {
				return false
			}

		default:
			// It's not an equal or prefix string matcher, so we have to stop searching
			// cause this optimization can't be applied.
			return false
		}
	}

	return true
}

func hasPrefixCaseInsensitive(s, prefix string) bool {
	_, ok := prefixCaseInsensitiveMatchLen(s, prefix)
	return ok
}

// prefixCaseInsensitiveMatchLen checks whether s begins with a prefix that is
// equal to prefix under Unicode simple case folding (the same folding the
// regexp engine applies for case-insensitive matching). It returns the length
// in bytes of that prefix in s, and whether such a prefix exists.
//
// The returned length can differ from len(prefix) because simple case folding
// does not preserve the encoded length of a rune, e.g. 'K' (the Kelvin sign,
// U+212A, 3 bytes) folds with 'k' (1 byte). For this reason a simple
// strings.EqualFold(s[:len(prefix)], prefix) check is not equivalent: it would
// slice s in the middle of a rune and fail to match.
func prefixCaseInsensitiveMatchLen(s, prefix string) (int, bool) {
	// Fast path: process ASCII characters in lockstep while we can.
	i := 0
	for ; i < len(prefix) && i < len(s); i++ {
		pc, sc := prefix[i], s[i]
		if pc >= utf8.RuneSelf || sc >= utf8.RuneSelf {
			break
		}
		if pc != sc && lowerASCII(pc) != lowerASCII(sc) {
			return 0, false
		}
	}
	if i == len(prefix) {
		return i, true
	}

	// Slow path: at least one of the next characters is non-ASCII, so runes
	// must be compared one by one under simple case folding. Both prefix[i:]
	// and s[i:] start at a rune boundary because the fast path above only
	// consumed ASCII bytes from both.
	n := i
	for _, pr := range prefix[i:] {
		if n >= len(s) {
			return 0, false
		}
		sr, size := utf8.DecodeRuneInString(s[n:])
		if sr != pr && !runeFoldEqual(sr, pr) {
			return 0, false
		}
		n += size
	}
	return n, true
}

// suffixCaseInsensitiveMatchLen is the equivalent of
// prefixCaseInsensitiveMatchLen for suffixes: it checks whether s ends with a
// suffix that is equal to suffix under Unicode simple case folding, and
// returns the length in bytes of that suffix in s.
func suffixCaseInsensitiveMatchLen(s, suffix string) (int, bool) {
	// Fast path: process ASCII characters in lockstep while we can. Bytes of
	// multi-byte runes are >= utf8.RuneSelf, so this cannot stop in the middle
	// of a rune.
	i, j := len(suffix), len(s)
	for i > 0 && j > 0 {
		pc, sc := suffix[i-1], s[j-1]
		if pc >= utf8.RuneSelf || sc >= utf8.RuneSelf {
			break
		}
		if pc != sc && lowerASCII(pc) != lowerASCII(sc) {
			return 0, false
		}
		i--
		j--
	}
	if i == 0 {
		return len(s) - j, true
	}

	// Slow path: compare the remaining runes one by one, from the end, under
	// simple case folding.
	for i > 0 {
		if j == 0 {
			return 0, false
		}
		pr, prSize := utf8.DecodeLastRuneInString(suffix[:i])
		sr, srSize := utf8.DecodeLastRuneInString(s[:j])
		if sr != pr && !runeFoldEqual(sr, pr) {
			return 0, false
		}
		i -= prSize
		j -= srSize
	}
	return len(s) - j, true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// runeFoldEqual tells whether two distinct runes are equal under Unicode
// simple case folding.
func runeFoldEqual(a, b rune) bool {
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}

// lengthMask returns a bitmask with the bit at position len(s) set to 1, and all other bits set to 0.
// If len(s) is greater than 63, it returns a bitmask with only the bit at position 63 set to 1.
func lengthMask(s string) uint64 {
	return 1 << min(len(s), 63)
}
