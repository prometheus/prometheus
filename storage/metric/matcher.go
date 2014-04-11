// Copyright 2014 Prometheus Team
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

package metric

import (
	"regexp"

	clientmodel "github.com/prometheus/client_golang/model"
)

// MatchType is an enum for label matching types.
type MatchType int

// Possible MatchTypes.
const (
	Equal MatchType = iota
	NotEqual
	RegexMatch
	RegexNoMatch
)

func (m MatchType) String() string {
	typeToStr := map[MatchType]string{
		Equal:        "=",
		NotEqual:     "!=",
		RegexMatch:   "=~",
		RegexNoMatch: "!~",
	}
	if str, ok := typeToStr[m]; ok {
		return str
	}
	panic("unknown match type")
}

// LabelMatchers is a slice of LabelMatcher objects.
type LabelMatchers []*LabelMatcher

// LabelMatcher models the matching of a label.
type LabelMatcher struct {
	Type  MatchType
	Name  clientmodel.LabelName
	Value clientmodel.LabelValue
	re    *regexp.Regexp
}

// NewLabelMatcher returns a LabelMatcher object ready to use.
func NewLabelMatcher(matchType MatchType, name clientmodel.LabelName, value clientmodel.LabelValue) (*LabelMatcher, error) {
	m := &LabelMatcher{
		Type:  matchType,
		Name:  name,
		Value: value,
	}
	if matchType == RegexMatch || matchType == RegexNoMatch {
		re, err := regexp.Compile(string(value))
		if err != nil {
			return nil, err
		}
		m.re = re
	}
	return m, nil
}

// Match returns true if the label matcher matches the supplied label value.
func (m *LabelMatcher) Match(v clientmodel.LabelValue) bool {
	switch m.Type {
	case Equal:
		return m.Value == v
	case NotEqual:
		return m.Value != v
	case RegexMatch:
		return m.re.MatchString(string(v))
	case RegexNoMatch:
		return !m.re.MatchString(string(v))
	default:
		panic("invalid match type")
	}
}

// Filter takes a list of label values and returns all label values which match
// the label matcher.
func (m *LabelMatcher) Filter(in clientmodel.LabelValues) clientmodel.LabelValues {
	out := clientmodel.LabelValues{}
	for _, v := range in {
		if m.Match(v) {
			out = append(out, v)
		}
	}
	return out
}
