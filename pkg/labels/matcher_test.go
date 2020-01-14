// Copyright 2017 The Prometheus Authors
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
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func mustNewMatcher(t *testing.T, mType MatchType, value string) *Matcher {
	m, err := NewMatcher(mType, "", value)
	testutil.Ok(t, err)
	return m
}

func TestMatcher(t *testing.T) {
	tests := []struct {
		matcher *Matcher
		value   string
		match   bool
	}{
		{
			matcher: mustNewMatcher(t, MatchEqual, "bar"),
			value:   "bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchEqual, "bar"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchNotEqual, "bar"),
			value:   "bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchNotEqual, "bar"),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "bar"),
			value:   "bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "bar"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, ".*bar"),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, "bar"),
			value:   "bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, "bar"),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, ".*bar"),
			value:   "foo-bar",
			match:   false,
		},
	}

	for _, test := range tests {
		testutil.Equals(t, test.matcher.Matches(test.value), test.match)
	}
}

func TestInverse(t *testing.T) {
	tests := []struct {
		matcher  *Matcher
		expected *Matcher
	}{
		{
			matcher:  &Matcher{Type: MatchEqual, Name: "name1", Value: "value1"},
			expected: &Matcher{Type: MatchNotEqual, Name: "name1", Value: "value1"},
		},
		{
			matcher:  &Matcher{Type: MatchNotEqual, Name: "name2", Value: "value2"},
			expected: &Matcher{Type: MatchEqual, Name: "name2", Value: "value2"},
		},
		{
			matcher:  &Matcher{Type: MatchRegexp, Name: "name3", Value: "value3"},
			expected: &Matcher{Type: MatchNotRegexp, Name: "name3", Value: "value3"},
		},
		{
			matcher:  &Matcher{Type: MatchNotRegexp, Name: "name4", Value: "value4"},
			expected: &Matcher{Type: MatchRegexp, Name: "name4", Value: "value4"},
		},
	}

	for _, test := range tests {
		result, err := test.matcher.Inverse()
		testutil.Ok(t, err)
		testutil.Equals(t, test.expected.Type, result.Type)
	}
}
