// Copyright The Prometheus Authors
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
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustNewMatcher(t *testing.T, mType MatchType, value string) *Matcher {
	m, err := NewMatcher(mType, "test_label_name", value)
	require.NoError(t, err)
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
		{
			matcher: mustNewMatcher(t, MatchRegexp, "$*bar"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "bar^+"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "$+bar"),
			value:   "foo-bar",
			match:   false,
		},
	}

	for _, test := range tests {
		require.Equal(t, test.matcher.Matches(test.value), test.match)
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
			matcher:  &Matcher{Type: MatchRegexp, Name: "name3", Value: "value3.*"},
			expected: &Matcher{Type: MatchNotRegexp, Name: "name3", Value: "value3.*"},
		},
		{
			matcher:  &Matcher{Type: MatchNotRegexp, Name: "name4", Value: "value4.*"},
			expected: &Matcher{Type: MatchRegexp, Name: "name4", Value: "value4.*"},
		},
	}

	for _, test := range tests {
		result, err := test.matcher.Inverse()
		require.NoError(t, err)
		require.Equal(t, test.expected.Type, result.Type)
	}
}

func TestPrefix(t *testing.T) {
	for i, tc := range []struct {
		matcher *Matcher
		prefix  string
	}{
		{
			matcher: mustNewMatcher(t, MatchEqual, "abc"),
			prefix:  "",
		},
		{
			matcher: mustNewMatcher(t, MatchNotEqual, "abc"),
			prefix:  "",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "abc.+"),
			prefix:  "abc",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "abcd|abc.+"),
			prefix:  "abc",
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, "abcd|abc.+"),
			prefix:  "abc",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "abc(def|ghj)|ab|a."),
			prefix:  "a",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "foo.+bar|foo.*baz"),
			prefix:  "foo",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "abc|.*"),
			prefix:  "",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "abc|def"),
			prefix:  "",
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, ".+def"),
			prefix:  "",
		},
	} {
		t.Run(fmt.Sprintf("%d: %s", i, tc.matcher), func(t *testing.T) {
			require.Equal(t, tc.prefix, tc.matcher.Prefix())
		})
	}
}

func TestIsRegexOptimized(t *testing.T) {
	for i, tc := range []struct {
		matcher          *Matcher
		isRegexOptimized bool
	}{
		{
			matcher:          mustNewMatcher(t, MatchEqual, "abc"),
			isRegexOptimized: false,
		},
		{
			matcher:          mustNewMatcher(t, MatchRegexp, "."),
			isRegexOptimized: false,
		},
		{
			matcher:          mustNewMatcher(t, MatchRegexp, "abc.+"),
			isRegexOptimized: true,
		},
	} {
		t.Run(fmt.Sprintf("%d: %s", i, tc.matcher), func(t *testing.T) {
			require.Equal(t, tc.isRegexOptimized, tc.matcher.IsRegexOptimized())
		})
	}
}

func BenchmarkMatchType_String(b *testing.B) {
	for i := 0; i <= b.N; i++ {
		_ = MatchType(i % int(MatchNotRegexp+1)).String()
	}
}

func BenchmarkNewMatcher(b *testing.B) {
	b.Run("regex matcher with literal", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i <= b.N; i++ {
			NewMatcher(MatchRegexp, "foo", "bar")
		}
	})
	b.Run("complex regex", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i <= b.N; i++ {
			NewMatcher(MatchRegexp, "foo", "((.*)(bar|b|buzz)(.+)|foo){10}")
		}
	})
}

func BenchmarkMatcher_String(b *testing.B) {
	type benchCase struct {
		name     string
		matchers []*Matcher
	}
	cases := []benchCase{
		{
			name: "short name equal",
			matchers: []*Matcher{
				MustNewMatcher(MatchEqual, "foo", "bar"),
				MustNewMatcher(MatchEqual, "bar", "baz"),
				MustNewMatcher(MatchEqual, "abc", "def"),
				MustNewMatcher(MatchEqual, "ghi", "klm"),
				MustNewMatcher(MatchEqual, "nop", "qrs"),
			},
		},
		{
			name: "short quoted name not equal",
			matchers: []*Matcher{
				MustNewMatcher(MatchEqual, "f.o", "bar"),
				MustNewMatcher(MatchEqual, "b.r", "baz"),
				MustNewMatcher(MatchEqual, "a.c", "def"),
				MustNewMatcher(MatchEqual, "g.i", "klm"),
				MustNewMatcher(MatchEqual, "n.p", "qrs"),
			},
		},
		{
			name: "short quoted name with quotes not equal",
			matchers: []*Matcher{
				MustNewMatcher(MatchEqual, `"foo"`, "bar"),
				MustNewMatcher(MatchEqual, `"foo"`, "baz"),
				MustNewMatcher(MatchEqual, `"foo"`, "def"),
				MustNewMatcher(MatchEqual, `"foo"`, "klm"),
				MustNewMatcher(MatchEqual, `"foo"`, "qrs"),
			},
		},
		{
			name: "short name value with quotes equal",
			matchers: []*Matcher{
				MustNewMatcher(MatchEqual, "foo", `"bar"`),
				MustNewMatcher(MatchEqual, "bar", `"baz"`),
				MustNewMatcher(MatchEqual, "abc", `"def"`),
				MustNewMatcher(MatchEqual, "ghi", `"klm"`),
				MustNewMatcher(MatchEqual, "nop", `"qrs"`),
			},
		},
		{
			name: "short name and long value regexp",
			matchers: []*Matcher{
				MustNewMatcher(MatchRegexp, "foo", "five_six_seven_eight_nine_ten_one_two_three_four"),
				MustNewMatcher(MatchRegexp, "bar", "one_two_three_four_five_six_seven_eight_nine_ten"),
				MustNewMatcher(MatchRegexp, "abc", "two_three_four_five_six_seven_eight_nine_ten_one"),
				MustNewMatcher(MatchRegexp, "ghi", "three_four_five_six_seven_eight_nine_ten_one_two"),
				MustNewMatcher(MatchRegexp, "nop", "four_five_six_seven_eight_nine_ten_one_two_three"),
			},
		},
		{
			name: "short name and long value with quotes equal",
			matchers: []*Matcher{
				MustNewMatcher(MatchEqual, "foo", `five_six_seven_eight_nine_ten_"one"_two_three_four`),
				MustNewMatcher(MatchEqual, "bar", `one_two_three_four_five_six_"seven"_eight_nine_ten`),
				MustNewMatcher(MatchEqual, "abc", `two_three_four_five_six_seven_"eight"_nine_ten_one`),
				MustNewMatcher(MatchEqual, "ghi", `three_four_five_six_seven_eight_"nine"_ten_one_two`),
				MustNewMatcher(MatchEqual, "nop", `four_five_six_seven_eight_nine_"ten"_one_two_three`),
			},
		},
		{
			name: "long name regexp",
			matchers: []*Matcher{
				MustNewMatcher(MatchRegexp, "one_two_three_four_five_six_seven_eight_nine_ten", "val"),
				MustNewMatcher(MatchRegexp, "two_three_four_five_six_seven_eight_nine_ten_one", "val"),
				MustNewMatcher(MatchRegexp, "three_four_five_six_seven_eight_nine_ten_one_two", "val"),
				MustNewMatcher(MatchRegexp, "four_five_six_seven_eight_nine_ten_one_two_three", "val"),
				MustNewMatcher(MatchRegexp, "five_six_seven_eight_nine_ten_one_two_three_four", "val"),
			},
		},
		{
			name: "long quoted name regexp",
			matchers: []*Matcher{
				MustNewMatcher(MatchRegexp, "one.two.three.four.five.six.seven.eight.nine.ten", "val"),
				MustNewMatcher(MatchRegexp, "two.three.four.five.six.seven.eight.nine.ten.one", "val"),
				MustNewMatcher(MatchRegexp, "three.four.five.six.seven.eight.nine.ten.one.two", "val"),
				MustNewMatcher(MatchRegexp, "four.five.six.seven.eight.nine.ten.one.two.three", "val"),
				MustNewMatcher(MatchRegexp, "five.six.seven.eight.nine.ten.one.two.three.four", "val"),
			},
		},
		{
			name: "long name and long value regexp",
			matchers: []*Matcher{
				MustNewMatcher(MatchRegexp, "one_two_three_four_five_six_seven_eight_nine_ten", "five_six_seven_eight_nine_ten_one_two_three_four"),
				MustNewMatcher(MatchRegexp, "two_three_four_five_six_seven_eight_nine_ten_one", "one_two_three_four_five_six_seven_eight_nine_ten"),
				MustNewMatcher(MatchRegexp, "three_four_five_six_seven_eight_nine_ten_one_two", "two_three_four_five_six_seven_eight_nine_ten_one"),
				MustNewMatcher(MatchRegexp, "four_five_six_seven_eight_nine_ten_one_two_three", "three_four_five_six_seven_eight_nine_ten_one_two"),
				MustNewMatcher(MatchRegexp, "five_six_seven_eight_nine_ten_one_two_three_four", "four_five_six_seven_eight_nine_ten_one_two_three"),
			},
		},
		{
			name: "long quoted name and long value regexp",
			matchers: []*Matcher{
				MustNewMatcher(MatchRegexp, "one.two.three.four.five.six.seven.eight.nine.ten", "five.six.seven.eight.nine.ten.one.two.three.four"),
				MustNewMatcher(MatchRegexp, "two.three.four.five.six.seven.eight.nine.ten.one", "one.two.three.four.five.six.seven.eight.nine.ten"),
				MustNewMatcher(MatchRegexp, "three.four.five.six.seven.eight.nine.ten.one.two", "two.three.four.five.six.seven.eight.nine.ten.one"),
				MustNewMatcher(MatchRegexp, "four.five.six.seven.eight.nine.ten.one.two.three", "three.four.five.six.seven.eight.nine.ten.one.two"),
				MustNewMatcher(MatchRegexp, "five.six.seven.eight.nine.ten.one.two.three.four", "four.five.six.seven.eight.nine.ten.one.two.three"),
			},
		},
	}

	var mixed []*Matcher
	for _, bc := range cases {
		mixed = append(mixed, bc.matchers...)
	}
	rand.Shuffle(len(mixed), func(i, j int) { mixed[i], mixed[j] = mixed[j], mixed[i] })
	cases = append(cases, benchCase{name: "mixed", matchers: mixed})

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i <= b.N; i++ {
				m := bc.matchers[i%len(bc.matchers)]
				_ = m.String()
			}
		})
	}
}
