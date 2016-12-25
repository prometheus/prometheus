package labels

import (
	"fmt"
	"regexp"
)

// MatchType is an enum for label matching types.
type MatchType int

// Possible MatchTypes.
const (
	MatchEqual MatchType = iota
	MatchNotEqual
	MatchRegexp
	MatchNotRegexp
)

func (m MatchType) String() string {
	typeToStr := map[MatchType]string{
		MatchEqual:     "=",
		MatchNotEqual:  "!=",
		MatchRegexp:    "=~",
		MatchNotRegexp: "!~",
	}
	if str, ok := typeToStr[m]; ok {
		return str
	}
	panic("unknown match type")
}

// Matcher models the matching of a label.
type Matcher struct {
	Type  MatchType
	Name  string
	Value string

	re *regexp.Regexp
}

// NewMatcher returns a matcher object.
func NewMatcher(t MatchType, n, v string) (*Matcher, error) {
	m := &Matcher{
		Type:  t,
		Name:  n,
		Value: v,
	}
	if t == MatchRegexp || t == MatchNotRegexp {
		m.Value = "^(?:" + v + ")$"
		re, err := regexp.Compile(m.Value)
		if err != nil {
			return nil, err
		}
		m.re = re
	}
	return m, nil
}

func (m *Matcher) String() string {
	return fmt.Sprintf("%s%s%q", m.Name, m.Type, m.Value)
}

// Matches returns whether the matcher matches the given string value.
func (m *Matcher) Matches(s string) bool {
	switch m.Type {
	case MatchEqual:
		return s == m.Value
	case MatchNotEqual:
		return s != m.Value
	case MatchRegexp:
		return m.re.MatchString(s)
	case MatchNotRegexp:
		return !m.re.MatchString(s)
	}
	panic("labels.Matcher.Matches: invalid match type")
}
