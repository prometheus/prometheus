// Copyright 2014 The Prometheus Authors
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
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/common/model"
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

// LabelMatchers is a slice of LabelMatcher objects. By implementing the
// sort.Interface, it is sortable by cardinality score, i.e. after sorting, the
// LabelMatcher that is expected to yield the fewest matches is first in the
// slice, and LabelMatchers that match the empty string are last.
type LabelMatchers []*LabelMatcher

func (lms LabelMatchers) Len() int           { return len(lms) }
func (lms LabelMatchers) Swap(i, j int)      { lms[i], lms[j] = lms[j], lms[i] }
func (lms LabelMatchers) Less(i, j int) bool { return lms[i].score < lms[j].score }

// LabelMatcher models the matching of a label. Create with NewLabelMatcher.
type LabelMatcher struct {
	Type  MatchType
	Name  model.LabelName
	Value model.LabelValue
	re    *regexp.Regexp
	score float64 // Cardinality score, between 0 and 1, 0 is lowest cardinality.
}

// NewLabelMatcher returns a LabelMatcher object ready to use.
func NewLabelMatcher(matchType MatchType, name model.LabelName, value model.LabelValue) (*LabelMatcher, error) {
	m := &LabelMatcher{
		Type:  matchType,
		Name:  name,
		Value: value,
	}
	if matchType == RegexMatch || matchType == RegexNoMatch {
		re, err := regexp.Compile("^(?:" + string(value) + ")$")
		if err != nil {
			return nil, err
		}
		m.re = re
	}
	m.calculateScore()
	return m, nil
}

// calculateScore is a helper method only called in the constructor. It
// calculates the cardinality score upfront, so that sorting by it is faster and
// doesn't change internal state of the matcher.
//
// The score is based on a pretty bad but still quite helpful heuristics for
// now. Note that this is an interim solution until the work in progress to
// properly intersect matchers is complete. We intend to not invest any further
// effort into tweaking the score calculation, as this could easily devolve into
// a rabbit hole.
//
// The heuristics works along the following lines:
//
// - A matcher that is known to match nothing would have a score of 0. (This
//   case doesn't happen in the scope of this method.)
//
// - A matcher that matches the empty string has a score of 1.
//
// - Equal matchers have a score <= 0.5. The order in score for other matchers
//   are RegexMatch, RegexNoMatch, NotEqual.
//
// - There are a number of score adjustments for known "magic" parts, like
//   instance labels, metric names containing a colon (which are probably
//   recording rules) and such.
//
// - On top, there is a tiny adjustment for the length of the matcher, following
//   the blunt expectation that a long label name and/or value is more specific
//   and will therefore have a lower cardinality.
//
// To reiterate on the above: PLEASE RESIST THE TEMPTATION TO TWEAK THIS
// METHOD. IT IS "MAGIC" ENOUGH ALREADY AND WILL GO AWAY WITH THE UPCOMING MORE
// POWERFUL INDEXING.
func (m *LabelMatcher) calculateScore() {
	if m.Match("") {
		m.score = 1
		return
	}
	// lengthCorrection is between 0 (for length 0) and 0.1 (for length +Inf).
	lengthCorrection := 0.1 * (1 - 1/float64(len(m.Name)+len(m.Value)+1))
	switch m.Type {
	case Equal:
		m.score = 0.3 - lengthCorrection
	case RegexMatch:
		m.score = 0.6 - lengthCorrection
	case RegexNoMatch:
		m.score = 0.8 + lengthCorrection
	case NotEqual:
		m.score = 0.9 + lengthCorrection
	}
	if m.Type != Equal {
		// Don't bother anymore in this case.
		return
	}
	switch m.Name {
	case model.InstanceLabel:
		// Matches only metrics from a single instance, which clearly
		// limits the damage.
		m.score -= 0.2
	case model.JobLabel:
		// The usual case is a relatively low number of jobs with many
		// metrics each.
		m.score += 0.1
	case model.BucketLabel, model.QuantileLabel:
		// Magic labels for buckets and quantiles will match copiously.
		m.score += 0.2
	case model.MetricNameLabel:
		if strings.Contains(string(m.Value), ":") {
			// Probably a recording rule with limited cardinality.
			m.score -= 0.1
			return
		}
		if m.Value == "up" || m.Value == "scrape_duration_seconds" {
			// Synthetic metrics which are contained in every scrape
			// exactly once.  There might be less frequent metric
			// names, but the worst case is limited here, so give it
			// a bump.
			m.score -= 0.05
			return
		}
	}
}

// MatchesEmptyString returns true if the LabelMatcher matches the empty string.
func (m *LabelMatcher) MatchesEmptyString() bool {
	return m.score >= 1
}

func (m *LabelMatcher) String() string {
	return fmt.Sprintf("%s%s%q", m.Name, m.Type, m.Value)
}

// Match returns true if the label matcher matches the supplied label value.
func (m *LabelMatcher) Match(v model.LabelValue) bool {
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
func (m *LabelMatcher) Filter(in model.LabelValues) model.LabelValues {
	out := model.LabelValues{}
	for _, v := range in {
		if m.Match(v) {
			out = append(out, v)
		}
	}
	return out
}
