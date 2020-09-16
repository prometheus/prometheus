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

package prompb

import (
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
)

func (m Sample) T() int64   { return m.Timestamp }
func (m Sample) V() float64 { return m.Value }

// Translate Functions Copied from:
// https://github.com/thanos-io/thanos/blob/v0.15.0/pkg/store/storepb/custom.go#L448

// TranslatePromMatchers returns proto matchers from Prometheus matchers.
// NOTE: It allocates memory.
func TranslatePromMatchers(ms ...*labels.Matcher) ([]LabelMatcher, error) {
	res := make([]LabelMatcher, 0, len(ms))
	for _, m := range ms {
		var t LabelMatcher_Type

		switch m.Type {
		case labels.MatchEqual:
			t = LabelMatcher_EQ
		case labels.MatchNotEqual:
			t = LabelMatcher_NEQ
		case labels.MatchRegexp:
			t = LabelMatcher_RE
		case labels.MatchNotRegexp:
			t = LabelMatcher_NRE
		default:
			return nil, errors.Errorf("unrecognized matcher type %d", m.Type)
		}
		res = append(res, LabelMatcher{Type: t, Name: m.Name, Value: m.Value})
	}
	return res, nil
}

// TranslateFromPromMatchers returns Prometheus matchers from proto matchers.
// NOTE: It allocates memory.
func TranslateFromPromMatchers(ms ...LabelMatcher) ([]*labels.Matcher, error) {
	res := make([]*labels.Matcher, 0, len(ms))
	for _, m := range ms {
		var t labels.MatchType

		switch m.Type {
		case LabelMatcher_EQ:
			t = labels.MatchEqual
		case LabelMatcher_NEQ:
			t = labels.MatchNotEqual
		case LabelMatcher_RE:
			t = labels.MatchRegexp
		case LabelMatcher_NRE:
			t = labels.MatchNotRegexp
		default:
			return nil, errors.Errorf("unrecognized matcher type %d", m.Type)
		}
		res = append(res, &labels.Matcher{Type: t, Name: m.Name, Value: m.Value})
	}
	return res, nil
}
