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
	"strings"
	"testing"

	"github.com/grafana/regexp/syntax"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, err)
		require.Equal(t, c.expected, m.MatchString(c.value))
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
		{regex: ".*((?i)abc).*", prefix: "", suffix: "", contains: ""},
		{regex: ".*(?i:abc).*", prefix: "", suffix: "", contains: ""},
		{regex: "(?i:abc).*", prefix: "", suffix: "", contains: ""},
		{regex: ".*(?i:abc)", prefix: "", suffix: "", contains: ""},
		{regex: ".*(?i:abc)def.*", prefix: "", suffix: "", contains: "def"},
		{regex: "(?i).*(?-i:abc)def", prefix: "", suffix: "", contains: "abc"},
		{regex: ".*(?msU:abc).*", prefix: "", suffix: "", contains: "abc"},
		{regex: "[aA]bc.*", prefix: "", suffix: "", contains: "bc"},
	}

	for _, c := range cases {
		parsed, err := syntax.Parse(c.regex, syntax.Perl)
		require.NoError(t, err)

		prefix, suffix, contains := optimizeConcatRegex(parsed)
		require.Equal(t, c.prefix, prefix)
		require.Equal(t, c.suffix, suffix)
		require.Equal(t, c.contains, contains)
	}
}

func BenchmarkFastRegexMatcher(b *testing.B) {
	var (
		x = strings.Repeat("x", 50)
		y = "foo" + x
		z = x + "foo"
	)
	regexes := []string{
		"foo",
		"^foo",
		"(foo|bar)",
		"foo.*",
		".*foo",
		"^.*foo$",
		"^.+foo$",
		".*",
		".+",
		"foo.+",
		".+foo",
		".*foo.*",
		"(?i:foo)",
		"(prometheus|api_prom)_api_v1_.+",
		"((fo(bar))|.+foo)",
	}
	for _, r := range regexes {
		r := r
		b.Run(r, func(b *testing.B) {
			m, err := NewFastRegexMatcher(r)
			require.NoError(b, err)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.MatchString(x)
				_ = m.MatchString(y)
				_ = m.MatchString(z)
			}
		})

	}
}
