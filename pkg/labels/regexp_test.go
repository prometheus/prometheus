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
	"regexp/syntax"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestNewFastRegexMatcher(t *testing.T) {
	cases := []struct {
		regex    string
		value    string
		expected bool
	}{
		{regex: "(foo|bar)", value: "foo", expected: true},
		{regex: "(foo|bar)", value: "foo bar", expected: false},
		{regex: "(foo|bar)", value: "bar", expected: true},
		{regex: "foo.*", value: "foo bar", expected: true},
		{regex: "foo.*", value: "bar foo", expected: false},
		{regex: ".*foo", value: "foo bar", expected: false},
		{regex: ".*foo", value: "bar foo", expected: true},
		{regex: ".*foo", value: "foo", expected: true},
		{regex: "^.*foo$", value: "foo", expected: true},
		{regex: "^.+foo$", value: "foo", expected: false},
		{regex: "^.+foo$", value: "bfoo", expected: true},
		{regex: ".*", value: "\n", expected: false},
		{regex: ".*", value: "\nfoo", expected: false},
		{regex: ".*foo", value: "\nfoo", expected: false},
		{regex: "foo.*", value: "foo\n", expected: false},
		{regex: "foo\n.*", value: "foo\n", expected: true},
		{regex: ".*foo.*", value: "foo", expected: true},
		{regex: ".*foo.*", value: "foo bar", expected: true},
		{regex: ".*foo.*", value: "hello foo world", expected: true},
		{regex: ".*foo.*", value: "hello foo\n world", expected: false},
		{regex: ".*foo\n.*", value: "hello foo\n world", expected: true},
		{regex: ".*", value: "foo", expected: true},
		{regex: "", value: "foo", expected: false},
		{regex: "", value: "", expected: true},
	}

	for _, c := range cases {
		m, err := NewFastRegexMatcher(c.regex)
		testutil.Ok(t, err)
		testutil.Equals(t, c.expected, m.MatchString(c.value))
	}
}

func TestOptimizeConcatRegex(t *testing.T) {
	cases := []struct {
		regex    string
		prefix   string
		suffix   string
		contains string
	}{
		{regex: "foo(hello|bar)", prefix: "foo", suffix: "", contains: ""},
		{regex: "foo(hello|bar)world", prefix: "foo", suffix: "world", contains: ""},
		{regex: "foo.*", prefix: "foo", suffix: "", contains: ""},
		{regex: "foo.*hello.*bar", prefix: "foo", suffix: "bar", contains: "hello"},
		{regex: ".*foo", prefix: "", suffix: "foo", contains: ""},
		{regex: "^.*foo$", prefix: "", suffix: "foo", contains: ""},
		{regex: ".*foo.*", prefix: "", suffix: "", contains: "foo"},
		{regex: ".*foo.*bar.*", prefix: "", suffix: "", contains: "foo"},
		{regex: ".*(foo|bar).*", prefix: "", suffix: "", contains: ""},
		{regex: ".*[abc].*", prefix: "", suffix: "", contains: ""},
	}

	for _, c := range cases {
		parsed, err := syntax.Parse(c.regex, syntax.Perl)
		testutil.Ok(t, err)

		prefix, suffix, contains := optimizeConcatRegex(parsed)
		testutil.Equals(t, c.prefix, prefix)
		testutil.Equals(t, c.suffix, suffix)
		testutil.Equals(t, c.contains, contains)
	}
}
