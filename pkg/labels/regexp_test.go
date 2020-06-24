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
		regex  string
		prefix string
		suffix string
	}{
		{regex: "foo(hello|bar)", prefix: "foo", suffix: ""},
		{regex: "foo(hello|bar)world", prefix: "foo", suffix: "world"},
		{regex: "foo.*", prefix: "foo", suffix: ""},
		{regex: "foo.*hello.*bar", prefix: "foo", suffix: "bar"},
		{regex: ".*foo", prefix: "", suffix: "foo"},
		{regex: "^.*foo$", prefix: "", suffix: "foo"},
	}

	for _, c := range cases {
		parsed, err := syntax.Parse(c.regex, syntax.Perl)
		testutil.Ok(t, err)

		prefix, suffix := optimizeConcatRegex(parsed)
		testutil.Equals(t, c.prefix, prefix)
		testutil.Equals(t, c.suffix, suffix)
	}
}
