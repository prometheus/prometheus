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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLabels_String(t *testing.T) {
	cases := []struct {
		lables   Labels
		expected string
	}{
		{
			lables: Labels{
				{
					Name:  "t1",
					Value: "t1",
				},
				{
					Name:  "t2",
					Value: "t2",
				},
			},
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
	labels := Labels{
		{
			Name:  "__name__",
			Value: "ALERTS",
		},
		{
			Name:  "alertname",
			Value: "HTTPRequestRateLow",
		},
		{
			Name:  "alertstate",
			Value: "pending",
		},
		{
			Name:  "instance",
			Value: "0",
		},
		{
			Name:  "job",
			Value: "app-server",
		},
		{
			Name:  "severity",
			Value: "critical",
		},
	}

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
			expected: Labels{
				{
					Name:  "__name__",
					Value: "ALERTS",
				},
				{
					Name:  "alertname",
					Value: "HTTPRequestRateLow",
				},
				{
					Name:  "alertstate",
					Value: "pending",
				},
				{
					Name:  "instance",
					Value: "0",
				},
			},
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
			expected: Labels{
				{
					Name:  "job",
					Value: "app-server",
				},
				{
					Name:  "severity",
					Value: "critical",
				},
			},
		},
		// on = true, explicitly excluding metric name from matching.
		{
			providedNames: []string{
				"alertname",
				"alertstate",
				"instance",
			},
			on: true,
			expected: Labels{
				{
					Name:  "alertname",
					Value: "HTTPRequestRateLow",
				},
				{
					Name:  "alertstate",
					Value: "pending",
				},
				{
					Name:  "instance",
					Value: "0",
				},
			},
		},
		// on = false, implicitly excluding metric name from matching.
		{
			providedNames: []string{
				"alertname",
				"alertstate",
				"instance",
			},
			on: false,
			expected: Labels{
				{
					Name:  "job",
					Value: "app-server",
				},
				{
					Name:  "severity",
					Value: "critical",
				},
			},
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
			Input: append(
				FromMap(map[string]string{"__name__": "up", "hostname": "localhost"}),
				FromMap(map[string]string{"hostname": "127.0.0.1"})...,
			),
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
			input: Labels{
				{Name: "foo"},
				{Name: "bar"},
			},
			expected: Labels{},
		},
		{
			input: Labels{
				{Name: "foo"},
				{Name: "bar"},
				{Name: "baz"},
			},
			expected: Labels{},
		},
		{
			input: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "hostname", Value: "localhost"},
				{Name: "job", Value: "check"},
			},
			expected: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "hostname", Value: "localhost"},
				{Name: "job", Value: "check"},
			},
		},
		{
			input: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "hostname", Value: "localhost"},
				{Name: "bar"},
				{Name: "job", Value: "check"},
			},
			expected: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "hostname", Value: "localhost"},
				{Name: "job", Value: "check"},
			},
		},
		{
			input: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "foo"},
				{Name: "hostname", Value: "localhost"},
				{Name: "bar"},
				{Name: "job", Value: "check"},
			},
			expected: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "hostname", Value: "localhost"},
				{Name: "job", Value: "check"},
			},
		},
		{
			input: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "foo"},
				{Name: "baz"},
				{Name: "hostname", Value: "localhost"},
				{Name: "bar"},
				{Name: "job", Value: "check"},
			},
			expected: Labels{
				{Name: "__name__", Value: "test"},
				{Name: "hostname", Value: "localhost"},
				{Name: "job", Value: "check"},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			require.Equal(t, test.expected, test.input.WithoutEmpty())
		})
	}
}

func TestLabels_Equal(t *testing.T) {
	labels := Labels{
		{
			Name:  "aaa",
			Value: "111",
		},
		{
			Name:  "bbb",
			Value: "222",
		},
	}

	tests := []struct {
		compared Labels
		expected bool
	}{
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbb",
					Value: "222",
				},
				{
					Name:  "ccc",
					Value: "333",
				},
			},
			expected: false,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bar",
					Value: "222",
				},
			},
			expected: false,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbb",
					Value: "233",
				},
			},
			expected: false,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbb",
					Value: "222",
				},
			},
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
	labels := Labels{
		{
			Name:  "aaa",
			Value: "111",
		},
		{
			Name:  "bbb",
			Value: "222",
		},
	}

	tests := []struct {
		compared Labels
		expected int
	}{
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "110",
				},
				{
					Name:  "bbb",
					Value: "222",
				},
			},
			expected: 1,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbb",
					Value: "233",
				},
			},
			expected: -1,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bar",
					Value: "222",
				},
			},
			expected: 1,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbc",
					Value: "222",
				},
			},
			expected: -1,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
			},
			expected: 1,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbb",
					Value: "222",
				},
				{
					Name:  "ccc",
					Value: "333",
				},
				{
					Name:  "ddd",
					Value: "444",
				},
			},
			expected: -2,
		},
		{
			compared: Labels{
				{
					Name:  "aaa",
					Value: "111",
				},
				{
					Name:  "bbb",
					Value: "222",
				},
			},
			expected: 0,
		},
	}

	for i, test := range tests {
		got := Compare(labels, test.compared)
		require.Equal(t, test.expected, got, "unexpected comparison result for test case %d", i)
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

	labelsSet := Labels{
		{
			Name:  "aaa",
			Value: "111",
		},
		{
			Name:  "bbb",
			Value: "222",
		},
	}

	for i, test := range tests {
		got := labelsSet.Has(test.input)
		require.Equal(t, test.expected, got, "unexpected comparison result for test case %d", i)
	}
}

func TestLabels_Get(t *testing.T) {
	require.Equal(t, "", Labels{{"aaa", "111"}, {"bbb", "222"}}.Get("foo"))
	require.Equal(t, "111", Labels{{"aaa", "111"}, {"bbb", "222"}}.Get("aaa"))
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
	allLabels := make(Labels, maxLabels)
	for i := 0; i < maxLabels; i++ {
		allLabels[i] = Label{Name: strings.Repeat(string('a'+byte(i)), 5)}
	}
	for _, size := range []int{5, 10, maxLabels} {
		b.Run(fmt.Sprintf("with %d labels", size), func(b *testing.B) {
			labels := allLabels[:size]
			for _, scenario := range []struct {
				desc, label string
			}{
				{"get first label", labels[0].Name},
				{"get middle label", labels[size/2].Name},
				{"get last label", labels[size-1].Name},
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
			Labels{{"a_label_name", "a_label_value"}, {"another_label_name", "another_label_value"}},
			Labels{{"a_label_name", "a_label_value"}, {"another_label_name", "another_label_value"}},
		},
		{
			"not equal",
			Labels{{"a_label_name", "a_label_value"}, {"another_label_name", "another_label_value"}},
			Labels{{"a_label_name", "a_label_value"}, {"another_label_name", "a_different_label_value"}},
		},
		{
			"different sizes",
			Labels{{"a_label_name", "a_label_value"}, {"another_label_name", "another_label_value"}},
			Labels{{"a_label_name", "a_label_value"}},
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
	require.Equal(t, Labels{{"aaa", "111"}, {"bbb", "222"}}, Labels{{"aaa", "111"}, {"bbb", "222"}}.Copy())
}

func TestLabels_Map(t *testing.T) {
	require.Equal(t, map[string]string{"aaa": "111", "bbb": "222"}, Labels{{"aaa", "111"}, {"bbb", "222"}}.Map())
}

func TestLabels_WithLabels(t *testing.T) {
	require.Equal(t, Labels{{"aaa", "111"}, {"bbb", "222"}}, Labels{{"aaa", "111"}, {"bbb", "222"}, {"ccc", "333"}}.WithLabels("aaa", "bbb"))
}

func TestLabels_WithoutLabels(t *testing.T) {
	require.Equal(t, Labels{{"aaa", "111"}}, Labels{{"aaa", "111"}, {"bbb", "222"}, {"ccc", "333"}}.WithoutLabels("bbb", "ccc"))
	require.Equal(t, Labels{{"aaa", "111"}}, Labels{{"aaa", "111"}, {"bbb", "222"}, {MetricName, "333"}}.WithoutLabels("bbb"))
}

func TestBulider_NewBulider(t *testing.T) {
	require.Equal(
		t,
		&Builder{
			base: Labels{{"aaa", "111"}},
			del:  []string{},
			add:  []Label{},
		},
		NewBuilder(Labels{{"aaa", "111"}}),
	)
}

func TestBuilder_Del(t *testing.T) {
	require.Equal(
		t,
		&Builder{
			del: []string{"bbb"},
			add: []Label{{"aaa", "111"}, {"ccc", "333"}},
		},
		(&Builder{
			del: []string{},
			add: []Label{{"aaa", "111"}, {"bbb", "222"}, {"ccc", "333"}},
		}).Del("bbb"),
	)
}

func TestBuilder_Set(t *testing.T) {
	require.Equal(
		t,
		&Builder{
			base: Labels{{"aaa", "111"}},
			del:  []string{},
			add:  []Label{{"bbb", "222"}},
		},
		(&Builder{
			base: Labels{{"aaa", "111"}},
			del:  []string{},
			add:  []Label{},
		}).Set("bbb", "222"),
	)

	require.Equal(
		t,
		&Builder{
			base: Labels{{"aaa", "111"}},
			del:  []string{},
			add:  []Label{{"bbb", "333"}},
		},
		(&Builder{
			base: Labels{{"aaa", "111"}},
			del:  []string{},
			add:  []Label{{"bbb", "222"}},
		}).Set("bbb", "333"),
	)
}

func TestBuilder_Labels(t *testing.T) {
	require.Equal(
		t,
		Labels{{"aaa", "111"}, {"ccc", "333"}, {"ddd", "444"}},
		(&Builder{
			base: Labels{{"aaa", "111"}, {"bbb", "222"}, {"ccc", "333"}},
			del:  []string{"bbb"},
			add:  []Label{{"ddd", "444"}},
		}).Labels(),
	)
}

func TestLabels_Hash(t *testing.T) {
	lbls := Labels{
		{Name: "foo", Value: "bar"},
		{Name: "baz", Value: "qux"},
	}
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
				lbls := make(Labels, 10)
				for i := 0; i < len(lbls); i++ {
					// Label ~20B name, 50B value.
					lbls[i] = Label{Name: fmt.Sprintf("abcdefghijabcdefghijabcdefghij%d", i), Value: fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i)}
				}
				return lbls
			}(),
		},
		{
			name: "bigger labels over 1KB",
			lbls: func() Labels {
				lbls := make(Labels, 10)
				for i := 0; i < len(lbls); i++ {
					// Label ~50B name, 50B value.
					lbls[i] = Label{Name: fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i), Value: fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i)}
				}
				return lbls
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
				return Labels{{Name: "__name__", Value: lbl.String()}}
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
