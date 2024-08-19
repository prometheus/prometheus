package model

const (
	// MatcherTypeExactMatch - exact match.
	MatcherTypeExactMatch uint8 = iota
	// MatcherTypeExactNotMatch - exact not match.
	MatcherTypeExactNotMatch
	// MatcherTypeRegexpMatch - regexp match.
	MatcherTypeRegexpMatch
	// MatcherTypeRegexpNotMatch - regexp not match.
	MatcherTypeRegexpNotMatch
)

// LabelMatcher - label matcher.
type LabelMatcher struct {
	Name        string
	Value       string
	MatcherType uint8
}
