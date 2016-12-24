package labels

import "regexp"

// Selector holds constraints for matching against a label set.
type Selector []Matcher

// Matches returns whether the labels satisfy all matchers.
func (s Selector) Matches(labels Labels) bool {
	for _, m := range s {
		if v := labels.Get(m.Name()); !m.Matches(v) {
			return false
		}
	}
	return true
}

// Matcher specifies a constraint for the value of a label.
type Matcher interface {
	// Name returns the label name the matcher should apply to.
	Name() string
	// Matches checks whether a value fulfills the constraints.
	Matches(v string) bool
}

type EqualMatcher struct {
	LabelName, Value string
}

func (m *EqualMatcher) Name() string          { return m.LabelName }
func (m *EqualMatcher) Matches(v string) bool { return v == m.Value }

// NewEqualMatcher returns a new matcher matching an exact label value.
func NewEqualMatcher(name, value string) Matcher {
	return &EqualMatcher{LabelName: name, Value: value}
}

type regexpMatcher struct {
	name string
	re   *regexp.Regexp
}

func (m *regexpMatcher) Name() string          { return m.name }
func (m *regexpMatcher) Matches(v string) bool { return m.re.MatchString(v) }

// NewRegexpMatcher returns a new matcher verifying that a value matches
// the regular expression pattern.
func NewRegexpMatcher(name, pattern string) (Matcher, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &regexpMatcher{name: name, re: re}, nil
}

// notMatcher inverts the matching result for a matcher.
type notMatcher struct {
	Matcher
}

func (m *notMatcher) Matches(v string) bool {
	return !m.Matcher.Matches(v)
}

// Not inverts the matcher's matching result.
func Not(m Matcher) Matcher {
	return &notMatcher{m}
}
