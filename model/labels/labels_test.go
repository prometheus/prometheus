// Copyright 2019 The Prometheus Authors
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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestLabels_String(t *testing.T) {
	cases := []struct {
		lables   Labels
		expected string
	}{
		{
			lables:   FromStrings("t1", "t1", "t2", "t2"),
			expected: "{t1=\"t1\", t2=\"t2\"}",
		},
		{
			lables:   Labels{},
			expected: "{}",
		},
		{
			lables:   nil,
			expected: "{}",
		},
	}
	for _, c := range cases {
		str := c.lables.String()
		require.Equal(t, c.expected, str)
	}
}

func TestLabels_MatchLabels(t *testing.T) {
	labels := FromStrings(
		"__name__", "ALERTS",
		"alertname", "HTTPRequestRateLow",
		"alertstate", "pending",
		"instance", "0",
		"job", "app-server",
		"severity", "critical")

	tests := []struct {
		providedNames []string
		on            bool
		expected      Labels
	}{
		// on = true, explicitly including metric name in matching.
		{
			providedNames: []string{
				"__name__",
				"alertname",
				"alertstate",
				"instance",
			},
			on: true,
			expected: FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "pending",
				"instance", "0"),
		},
		// on = false, explicitly excluding metric name from matching.
		{
			providedNames: []string{
				"__name__",
				"alertname",
				"alertstate",
				"instance",
			},
			on: false,
			expected: FromStrings(
				"job", "app-server",
				"severity", "critical"),
		},
		// on = true, explicitly excluding metric name from matching.
		{
			providedNames: []string{
				"alertname",
				"alertstate",
				"instance",
			},
			on: true,
			expected: FromStrings(
				"alertname", "HTTPRequestRateLow",
				"alertstate", "pending",
				"instance", "0"),
		},
		// on = false, implicitly excluding metric name from matching.
		{
			providedNames: []string{
				"alertname",
				"alertstate",
				"instance",
			},
			on: false,
			expected: FromStrings(
				"job", "app-server",
				"severity", "critical"),
		},
	}

	for i, test := range tests {
		got := labels.MatchLabels(test.on, test.providedNames...)
		require.Equal(t, test.expected, got, "unexpected labelset for test case %d", i)
	}
}

func TestLabels_HasDuplicateLabelNames(t *testing.T) {
	cases := []struct {
		Input     Labels
		Duplicate bool
		LabelName string
	}{
		{
			Input:     FromMap(map[string]string{"__name__": "up", "hostname": "localhost"}),
			Duplicate: false,
		}, {
			Input:     FromStrings("__name__", "up", "hostname", "localhost", "hostname", "127.0.0.1"),
			Duplicate: true,
			LabelName: "hostname",
		},
	}

	for i, c := range cases {
		l, d := c.Input.HasDuplicateLabelNames()
		require.Equal(t, c.Duplicate, d, "test %d: incorrect duplicate bool", i)
		require.Equal(t, c.LabelName, l, "test %d: incorrect label name", i)
	}
}

func TestLabels_WithoutEmpty(t *testing.T) {
	for _, test := range []struct {
		input    Labels
		expected Labels
	}{
		{
			input: FromStrings(
				"foo", "",
				"bar", ""),
			expected: EmptyLabels(),
		},
		{
			input: FromStrings(
				"foo", "",
				"bar", "",
				"baz", ""),
			expected: EmptyLabels(),
		},
		{
			input: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"job", "check"),
			expected: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"job", "check"),
		},
		{
			input: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"bar", "",
				"job", "check"),
			expected: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"job", "check"),
		},
		{
			input: FromStrings(
				"__name__", "test",
				"foo", "",
				"hostname", "localhost",
				"bar", "",
				"job", "check"),
			expected: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"job", "check"),
		},
		{
			input: FromStrings(
				"__name__", "test",
				"foo", "",
				"baz", "",
				"hostname", "localhost",
				"bar", "",
				"job", "check"),
			expected: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"job", "check"),
		},
	} {
		t.Run("", func(t *testing.T) {
			require.Equal(t, test.expected, test.input.WithoutEmpty())
		})
	}
}

func TestLabels_Equal(t *testing.T) {
	labels := FromStrings(
		"aaa", "111",
		"bbb", "222")

	tests := []struct {
		compared Labels
		expected bool
	}{
		{
			compared: FromStrings(
				"aaa", "111",
				"bbb", "222",
				"ccc", "333"),
			expected: false,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bar", "222"),
			expected: false,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbb", "233"),
			expected: false,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbb", "222"),
			expected: true,
		},
	}

	for i, test := range tests {
		got := Equal(labels, test.compared)
		require.Equal(t, test.expected, got, "unexpected comparison result for test case %d", i)
	}
}

func TestLabels_FromStrings(t *testing.T) {
	labels := FromStrings("aaa", "111", "bbb", "222")
	expected := Labels{
		{
			Name:  "aaa",
			Value: "111",
		},
		{
			Name:  "bbb",
			Value: "222",
		},
	}

	require.Equal(t, expected, labels, "unexpected labelset")

	require.Panics(t, func() { FromStrings("aaa", "111", "bbb") }) //nolint:staticcheck // Ignore SA5012, error is intentional test.
}

func TestLabels_Compare(t *testing.T) {
	labels := FromStrings(
		"aaa", "111",
		"bbb", "222")

	tests := []struct {
		compared Labels
		expected int
	}{
		{
			compared: FromStrings(
				"aaa", "110",
				"bbb", "222"),
			expected: 1,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbb", "233"),
			expected: -1,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bar", "222"),
			expected: 1,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbc", "222"),
			expected: -1,
		},
		{
			compared: FromStrings(
				"aaa", "111"),
			expected: 1,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbb", "222",
				"ccc", "333",
				"ddd", "444"),
			expected: -2,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbb", "222"),
			expected: 0,
		},
	}

	sign := func(a int) int {
		switch {
		case a < 0:
			return -1
		case a > 0:
			return 1
		}
		return 0
	}

	for i, test := range tests {
		got := Compare(labels, test.compared)
		require.Equal(t, sign(test.expected), sign(got), "unexpected comparison result for test case %d", i)
	}
}

func TestLabels_Has(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{
			input:    "foo",
			expected: false,
		},
		{
			input:    "aaa",
			expected: true,
		},
	}

	labelsSet := FromStrings(
		"aaa", "111",
		"bbb", "222")

	for i, test := range tests {
		got := labelsSet.Has(test.input)
		require.Equal(t, test.expected, got, "unexpected comparison result for test case %d", i)
	}
}

func TestLabels_Get(t *testing.T) {
	require.Equal(t, "", FromStrings("aaa", "111", "bbb", "222").Get("foo"))
	require.Equal(t, "111", FromStrings("aaa", "111", "bbb", "222").Get("aaa"))
}

// BenchmarkLabels_Get was written to check whether a binary search can improve the performance vs the linear search implementation
// The results have shown that binary search would only be better when searching last labels in scenarios with more than 10 labels.
// In the following list, `old` is the linear search while `new` is the binary search implementation (without calling sort.Search, which performs even worse here)
// name                                        old time/op    new time/op    delta
// Labels_Get/with_5_labels/get_first_label      5.12ns ± 0%   14.24ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_5_labels/get_middle_label     13.5ns ± 0%    18.5ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_5_labels/get_last_label       21.9ns ± 0%    18.9ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_10_labels/get_first_label     5.11ns ± 0%   19.47ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_10_labels/get_middle_label    26.2ns ± 0%    19.3ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_10_labels/get_last_label      42.8ns ± 0%    23.4ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_30_labels/get_first_label     5.10ns ± 0%   24.63ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_30_labels/get_middle_label    75.8ns ± 0%    29.7ns ± 0%   ~     (p=1.000 n=1+1)
// Labels_Get/with_30_labels/get_last_label       169ns ± 0%      29ns ± 0%   ~     (p=1.000 n=1+1)
func BenchmarkLabels_Get(b *testing.B) {
	maxLabels := 30
	allLabels := make([]Label, maxLabels)
	for i := 0; i < maxLabels; i++ {
		allLabels[i] = Label{Name: strings.Repeat(string('a'+byte(i)), 5)}
	}
	for _, size := range []int{5, 10, maxLabels} {
		b.Run(fmt.Sprintf("with %d labels", size), func(b *testing.B) {
			labels := New(allLabels[:size]...)
			for _, scenario := range []struct {
				desc, label string
			}{
				{"get first label", allLabels[0].Name},
				{"get middle label", allLabels[size/2].Name},
				{"get last label", allLabels[size-1].Name},
			} {
				b.Run(scenario.desc, func(b *testing.B) {
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_ = labels.Get(scenario.label)
					}
				})
			}
		})
	}
}

func BenchmarkLabels_Equals(b *testing.B) {
	for _, scenario := range []struct {
		desc        string
		base, other Labels
	}{
		{
			"equal",
			FromStrings("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
			FromStrings("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
		},
		{
			"not equal",
			FromStrings("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
			FromStrings("a_label_name", "a_label_value", "another_label_name", "a_different_label_value"),
		},
		{
			"different sizes",
			FromStrings("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
			FromStrings("a_label_name", "a_label_value"),
		},
	} {
		b.Run(scenario.desc, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = Equal(scenario.base, scenario.other)
			}
		})
	}
}

func TestLabels_Copy(t *testing.T) {
	require.Equal(t, FromStrings("aaa", "111", "bbb", "222"), FromStrings("aaa", "111", "bbb", "222").Copy())
}

func TestLabels_Map(t *testing.T) {
	require.Equal(t, map[string]string{"aaa": "111", "bbb": "222"}, FromStrings("aaa", "111", "bbb", "222").Map())
}

func TestLabels_BytesWithLabels(t *testing.T) {
	require.Equal(t, FromStrings("aaa", "111", "bbb", "222").Bytes(nil), FromStrings("aaa", "111", "bbb", "222", "ccc", "333").BytesWithLabels(nil, "aaa", "bbb"))
	require.Equal(t, FromStrings().Bytes(nil), FromStrings("aaa", "111", "bbb", "222", "ccc", "333").BytesWithLabels(nil))
}

func TestLabels_BytesWithoutLabels(t *testing.T) {
	require.Equal(t, FromStrings("aaa", "111").Bytes(nil), FromStrings("aaa", "111", "bbb", "222", "ccc", "333").BytesWithoutLabels(nil, "bbb", "ccc"))
	require.Equal(t, FromStrings(MetricName, "333", "aaa", "111").Bytes(nil), FromStrings(MetricName, "333", "aaa", "111", "bbb", "222").BytesWithoutLabels(nil, "bbb"))
	require.Equal(t, FromStrings("aaa", "111").Bytes(nil), FromStrings(MetricName, "333", "aaa", "111", "bbb", "222").BytesWithoutLabels(nil, MetricName, "bbb"))
}

func TestBuilder(t *testing.T) {
	for i, tcase := range []struct {
		base Labels
		del  []string
		keep []string
		set  []Label
		want Labels
	}{
		{
			base: FromStrings("aaa", "111"),
			want: FromStrings("aaa", "111"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			del:  []string{"bbb"},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: nil,
			set:  []Label{{"aaa", "111"}, {"bbb", "222"}, {"ccc", "333"}},
			del:  []string{"bbb"},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: FromStrings("aaa", "111"),
			set:  []Label{{"bbb", "222"}},
			want: FromStrings("aaa", "111", "bbb", "222"),
		},
		{
			base: FromStrings("aaa", "111"),
			set:  []Label{{"bbb", "222"}, {"bbb", "333"}},
			want: FromStrings("aaa", "111", "bbb", "333"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			del:  []string{"bbb"},
			set:  []Label{{"ddd", "444"}},
			want: FromStrings("aaa", "111", "ccc", "333", "ddd", "444"),
		},
		{ // Blank value is interpreted as delete.
			base: FromStrings("aaa", "111", "bbb", "", "ccc", "333"),
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			set:  []Label{{"bbb", ""}},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			keep: []string{"bbb"},
			want: FromStrings("bbb", "222"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			keep: []string{"aaa", "ccc"},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			del:  []string{"bbb"},
			set:  []Label{{"ddd", "444"}},
			keep: []string{"aaa", "ddd"},
			want: FromStrings("aaa", "111", "ddd", "444"),
		},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			b := NewBuilder(tcase.base)
			for _, lbl := range tcase.set {
				b.Set(lbl.Name, lbl.Value)
			}
			if len(tcase.keep) > 0 {
				b.Keep(tcase.keep...)
			}
			b.Del(tcase.del...)
			require.Equal(t, tcase.want, b.Labels(tcase.base))
		})
	}
}

func TestLabels_Hash(t *testing.T) {
	lbls := FromStrings("foo", "bar", "baz", "qux")
	require.Equal(t, lbls.Hash(), lbls.Hash())
	require.NotEqual(t, lbls.Hash(), Labels{lbls[1], lbls[0]}.Hash(), "unordered labels match.")
	require.NotEqual(t, lbls.Hash(), Labels{lbls[0]}.Hash(), "different labels match.")
}

var benchmarkLabelsResult uint64

func BenchmarkLabels_Hash(b *testing.B) {
	for _, tcase := range []struct {
		name string
		lbls Labels
	}{
		{
			name: "typical labels under 1KB",
			lbls: func() Labels {
				b := NewBuilder(EmptyLabels())
				for i := 0; i < 10; i++ {
					// Label ~20B name, 50B value.
					b.Set(fmt.Sprintf("abcdefghijabcdefghijabcdefghij%d", i), fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i))
				}
				return b.Labels(nil)
			}(),
		},
		{
			name: "bigger labels over 1KB",
			lbls: func() Labels {
				b := NewBuilder(EmptyLabels())
				for i := 0; i < 10; i++ {
					// Label ~50B name, 50B value.
					b.Set(fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i), fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i))
				}
				return b.Labels(nil)
			}(),
		},
		{
			name: "extremely large label value 10MB",
			lbls: func() Labels {
				lbl := &strings.Builder{}
				lbl.Grow(1024 * 1024 * 10) // 10MB.
				word := "abcdefghij"
				for i := 0; i < lbl.Cap()/len(word); i++ {
					_, _ = lbl.WriteString(word)
				}
				return FromStrings("__name__", lbl.String())
			}(),
		},
	} {
		b.Run(tcase.name, func(b *testing.B) {
			var h uint64

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h = tcase.lbls.Hash()
			}
			benchmarkLabelsResult = h
		})
	}
}

func TestMarshaling(t *testing.T) {
	lbls := FromStrings("aaa", "111", "bbb", "2222", "ccc", "33333")
	expectedJSON := "{\"aaa\":\"111\",\"bbb\":\"2222\",\"ccc\":\"33333\"}"
	b, err := json.Marshal(lbls)
	require.NoError(t, err)
	require.Equal(t, expectedJSON, string(b))

	var gotJ Labels
	err = json.Unmarshal(b, &gotJ)
	require.NoError(t, err)
	require.Equal(t, lbls, gotJ)

	expectedYAML := "aaa: \"111\"\nbbb: \"2222\"\nccc: \"33333\"\n"
	b, err = yaml.Marshal(lbls)
	require.NoError(t, err)
	require.Equal(t, expectedYAML, string(b))

	var gotY Labels
	err = yaml.Unmarshal(b, &gotY)
	require.NoError(t, err)
	require.Equal(t, lbls, gotY)

	// Now in a struct with a tag
	type foo struct {
		ALabels Labels `json:"a_labels,omitempty" yaml:"a_labels,omitempty"`
	}

	f := foo{ALabels: lbls}
	b, err = json.Marshal(f)
	require.NoError(t, err)
	expectedJSONFromStruct := "{\"a_labels\":" + expectedJSON + "}"
	require.Equal(t, expectedJSONFromStruct, string(b))

	var gotFJ foo
	err = json.Unmarshal(b, &gotFJ)
	require.NoError(t, err)
	require.Equal(t, f, gotFJ)

	b, err = yaml.Marshal(f)
	require.NoError(t, err)
	expectedYAMLFromStruct := "a_labels:\n  aaa: \"111\"\n  bbb: \"2222\"\n  ccc: \"33333\"\n"
	require.Equal(t, expectedYAMLFromStruct, string(b))

	var gotFY foo
	err = yaml.Unmarshal(b, &gotFY)
	require.NoError(t, err)
	require.Equal(t, f, gotFY)
}
