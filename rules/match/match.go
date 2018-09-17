// Copyright 2015 Prometheus Team
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

package match

import (
	"bytes"
	"fmt"
	"github.com/prometheus/common/model"
	"regexp"
	"strconv"
)

// Matcher defines a matching rule for the value of a given label.
type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	Regex   *regexp.Regexp
}

func (m *Matcher) String() string {
	if m.IsRegex {
		return fmt.Sprintf("%s=~%q", m.Name, m.Value)
	}
	return fmt.Sprintf("%s=%q", m.Name, m.Value)
}

// Validate returns true iff all fields of the matcher have valid values.
func (m *Matcher) Validate() error {
	if !model.LabelName(m.Name).IsValid() {
		return fmt.Errorf("invalid name %q", m.Name)
	}
	if m.IsRegex {
		if _, err := regexp.Compile(m.Value); err != nil {
			return fmt.Errorf("invalid regular expression %q", m.Value)
		}
	} else if !model.LabelValue(m.Value).IsValid() || len(m.Value) == 0 {
		return fmt.Errorf("invalid value %q", m.Value)
	}
	return nil
}

func (m *Matcher) Match(lset map[string]string) bool {
	v := lset[m.Name]
	if m.IsRegex {
		return m.Regex.MatchString(v)
	}
	return v == m.Value
}

// NewMatcher returns a new matcher that compares against equality of
// the given value.
func NewMatcher(name, value string) *Matcher {
	return &Matcher{
		Name:    name,
		Value:   value,
		IsRegex: false,
	}
}

// NewRegexMatcher returns a new matcher that compares values against
// a regular expression. The matcher is already initialized.
//
// TODO(fabxc): refactor usage.
func NewRegexMatcher(name string, re *regexp.Regexp) *Matcher {
	return &Matcher{
		Name:    name,
		Value:   re.String(),
		IsRegex: true,
		Regex:   re,
	}
}

// Matchers provides the Match and Fingerprint methods for a slice of Matchers.
// Matchers must always be sorted.
type Matchers struct {
	Matches            []*Matcher
	Value              float64
	MinValue           float64
	MaxValue           float64
	RelationalOperator string
	IsRangeValue       bool
}

func NewMatchers(matches []*Matcher, value float64, op string, minValue, maxValue float64) *Matchers {
	isRangeValue := false
	if op == "" {
		isRangeValue = true
	}
	return &Matchers{
		Matches:            matches,
		Value:              value,
		MinValue:           minValue,
		MaxValue:           maxValue,
		RelationalOperator: op,
		IsRangeValue:       isRangeValue,
	}
}

// Validate matchers
func (ms *Matchers) Validate() error {
	if len(ms.Matches) > 0 {
		for _, m := range ms.Matches {
			if err := m.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Match checks whether all matchers are fulfilled against the given label set.
func (ms Matchers) Match(lset map[string]string, v float64) bool {
	for _, m := range ms.Matches {
		if !m.Match(lset) {
			return false
		}
	}
	return ms.MatchValue(v)
}

func (ms Matchers) MatchValue(v float64) bool {
	if ms.IsRangeValue {
		if v >= ms.MinValue && v <= ms.MaxValue {
			return true
		}
		return false
	}
	// v[gt,ge] > maxValue true, v[lt,le] < minValue true
	switch ms.RelationalOperator {
	case "=", "eq":
		return ms.Value == v
	case "!=", "ne":
		return ms.Value != v
	case ">", "gt":
		return ms.Value > v
	case ">=", "ge":
		return ms.Value >= v
	case "<", "lt":
		return ms.Value < v
	case "<=", "le":
		return ms.Value <= v
	}
	return false
}

func (ms Matchers) String() string {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range ms.Matches {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(m.String())
	}
	buf.WriteByte(',')
	if ms.IsRangeValue {
		buf.WriteString(fmt.Sprintf("new_value_range: [%s, %s]", strconv.FormatFloat(ms.MinValue, 'f', -1, 64),
			strconv.FormatFloat(ms.MaxValue, 'f', -1, 64)))
	} else {
		buf.WriteString(fmt.Sprintf("new_value: %s, op: %s", strconv.FormatFloat(ms.Value, 'f', -1, 64), ms.RelationalOperator))
	}
	buf.WriteByte('}')
	return buf.String()
}

type MatchersConds []*Matchers

// Validate MatchersConds
func (conds MatchersConds) Validate() error {
	for _, mrs := range conds {
		if err := mrs.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (conds MatchersConds) String() string {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, mrs := range conds {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(mrs.String())
	}
	buf.WriteByte(']')
	return buf.String()
}
