// Copyright 2025 The Prometheus Authors
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
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	common "github.com/prometheus/prometheus/model/labels"
)

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
	x := 0
	labels.Range(func(l common.Label) {
		switch x {
		case 0:
			require.Equal(t, common.Label{Name: "aaa", Value: "111"}, l, "unexpected value")
		case 1:
			require.Equal(t, common.Label{Name: "bbb", Value: "222"}, l, "unexpected value")
		default:
			t.Fatalf("unexpected labelset value %d: %v", x, l)
		}
		x++
	})

	require.Panics(t, func() { FromStrings("aaa", "111", "bbb") }) //nolint:staticcheck // Ignore SA5012, error is intentional test.
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
	require.Empty(t, FromStrings("aaa", "111", "bbb", "222").Get("foo"))
	require.Equal(t, "111", FromStrings("aaaa", "111", "bbb", "222").Get("aaaa"))
	require.Equal(t, "222", FromStrings("aaaa", "111", "bbb", "222").Get("bbb"))
}

func ScratchBuilderForBenchmark() ScratchBuilder {
	// (Only relevant to -tags dedupelabels: stuff the symbol table before adding the real labels, to avoid having everything fitting into 1 byte.)
	b := NewScratchBuilder(256)
	for i := 0; i < 256; i++ {
		b.Add(fmt.Sprintf("name%d", i), fmt.Sprintf("value%d", i))
	}
	b.Labels()
	b.Reset()
	return b
}

func NewForBenchmark(ls ...common.Label) Labels {
	b := ScratchBuilderForBenchmark()
	for _, l := range ls {
		b.Add(l.Name, l.Value)
	}
	b.Sort()
	return b.Labels()
}

func FromStringsForBenchmark(ss ...string) Labels {
	if len(ss)%2 != 0 {
		panic("invalid number of strings")
	}
	b := ScratchBuilderForBenchmark()
	for i := 0; i < len(ss); i += 2 {
		b.Add(ss[i], ss[i+1])
	}
	b.Sort()
	return b.Labels()
}

// BenchmarkLabels_Get was written to check whether a binary search can improve the performance vs the linear search implementation
// The results have shown that binary search would only be better when searching last labels in scenarios with more than 10 labels.
// In the following list, `old` is the linear search while `new` is the binary search implementation (without calling sort.Search, which performs even worse here)
//
//	name                                        old time/op    new time/op    delta
//	Labels_Get/with_5_labels/get_first_label      5.12ns ± 0%   14.24ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_5_labels/get_middle_label     13.5ns ± 0%    18.5ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_5_labels/get_last_label       21.9ns ± 0%    18.9ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_10_labels/get_first_label     5.11ns ± 0%   19.47ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_10_labels/get_middle_label    26.2ns ± 0%    19.3ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_10_labels/get_last_label      42.8ns ± 0%    23.4ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_30_labels/get_first_label     5.10ns ± 0%   24.63ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_30_labels/get_middle_label    75.8ns ± 0%    29.7ns ± 0%   ~     (p=1.000 n=1+1)
//	Labels_Get/with_30_labels/get_last_label       169ns ± 0%      29ns ± 0%   ~     (p=1.000 n=1+1)
func BenchmarkLabels_Get(b *testing.B) {
	maxLabels := 30
	allLabels := make([]common.Label, maxLabels)
	for i := 0; i < maxLabels; i++ {
		allLabels[i] = common.Label{Name: strings.Repeat(string('a'+byte(i)), 5+(i%5))}
	}
	for _, size := range []int{5, 10, maxLabels} {
		b.Run(fmt.Sprintf("with %d labels", size), func(b *testing.B) {
			labels := NewForBenchmark(allLabels[:size]...)
			for _, scenario := range []struct {
				desc, label string
			}{
				{"first label", allLabels[0].Name},
				{"middle label", allLabels[size/2].Name},
				{"last label", allLabels[size-1].Name},
				{"not-found label", "benchmark"},
			} {
				b.Run(scenario.desc, func(b *testing.B) {
					b.Run("get", func(b *testing.B) {
						for i := 0; i < b.N; i++ {
							_ = labels.Get(scenario.label)
						}
					})
					b.Run("has", func(b *testing.B) {
						for i := 0; i < b.N; i++ {
							_ = labels.Has(scenario.label)
						}
					})
				})
			}
		})
	}
}

var comparisonBenchmarkScenarios = []struct {
	desc        string
	base, other Labels
}{
	{
		"equal",
		FromStringsForBenchmark("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
		FromStringsForBenchmark("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
	},
	{
		"not equal",
		FromStringsForBenchmark("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
		FromStringsForBenchmark("a_label_name", "a_label_value", "another_label_name", "a_different_label_value"),
	},
	{
		"different sizes",
		FromStringsForBenchmark("a_label_name", "a_label_value", "another_label_name", "another_label_value"),
		FromStringsForBenchmark("a_label_name", "a_label_value"),
	},
	{
		"lots",
		FromStringsForBenchmark("aaa", "bbb", "ccc", "ddd", "eee", "fff", "ggg", "hhh", "iii", "jjj", "kkk", "lll", "mmm", "nnn", "ooo", "ppp", "qqq", "rrz"),
		FromStringsForBenchmark("aaa", "bbb", "ccc", "ddd", "eee", "fff", "ggg", "hhh", "iii", "jjj", "kkk", "lll", "mmm", "nnn", "ooo", "ppp", "qqq", "rrr"),
	},
	{
		"real long equal",
		FromStringsForBenchmark("__name__", "kube_pod_container_status_last_terminated_exitcode", "cluster", "prod-af-north-0", " container", "prometheus", "instance", "kube-state-metrics-0:kube-state-metrics:ksm", "job", "kube-state-metrics/kube-state-metrics", " namespace", "observability-prometheus", "pod", "observability-prometheus-0", "uid", "d3ec90b2-4975-4607-b45d-b9ad64bb417e"),
		FromStringsForBenchmark("__name__", "kube_pod_container_status_last_terminated_exitcode", "cluster", "prod-af-north-0", " container", "prometheus", "instance", "kube-state-metrics-0:kube-state-metrics:ksm", "job", "kube-state-metrics/kube-state-metrics", " namespace", "observability-prometheus", "pod", "observability-prometheus-0", "uid", "d3ec90b2-4975-4607-b45d-b9ad64bb417e"),
	},
	{
		"real long different end",
		FromStringsForBenchmark("__name__", "kube_pod_container_status_last_terminated_exitcode", "cluster", "prod-af-north-0", " container", "prometheus", "instance", "kube-state-metrics-0:kube-state-metrics:ksm", "job", "kube-state-metrics/kube-state-metrics", " namespace", "observability-prometheus", "pod", "observability-prometheus-0", "uid", "d3ec90b2-4975-4607-b45d-b9ad64bb417e"),
		FromStringsForBenchmark("__name__", "kube_pod_container_status_last_terminated_exitcode", "cluster", "prod-af-north-0", " container", "prometheus", "instance", "kube-state-metrics-0:kube-state-metrics:ksm", "job", "kube-state-metrics/kube-state-metrics", " namespace", "observability-prometheus", "pod", "observability-prometheus-0", "uid", "deadbeef-0000-1111-2222-b9ad64bb417e"),
	},
}

func BenchmarkLabels_Equals(b *testing.B) {
	for _, scenario := range comparisonBenchmarkScenarios {
		b.Run(scenario.desc, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = Equal(scenario.base, scenario.other)
			}
		})
	}
}

func TestBuilder(t *testing.T) {
	reuseBuilder := NewBuilderWithSymbolTable(NewSymbolTable())
	for i, tcase := range []struct {
		base Labels
		del  []string
		keep []string
		set  []common.Label
		want Labels
	}{
		{
			base: FromStrings("aaa", "111"),
			want: FromStrings("aaa", "111"),
		},
		{
			base: EmptyLabels(),
			set:  []common.Label{{Name: "aaa", Value: "444"}, {Name: "bbb", Value: "555"}, {Name: "ccc", Value: "666"}},
			want: FromStrings("aaa", "444", "bbb", "555", "ccc", "666"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			set:  []common.Label{{Name: "aaa", Value: "444"}, {Name: "bbb", Value: "555"}, {Name: "ccc", Value: "666"}},
			want: FromStrings("aaa", "444", "bbb", "555", "ccc", "666"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			del:  []string{"bbb"},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			set:  []common.Label{{Name: "aaa", Value: "111"}, {Name: "bbb", Value: "222"}, {Name: "ccc", Value: "333"}},
			del:  []string{"bbb"},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: FromStrings("aaa", "111"),
			set:  []common.Label{{Name: "bbb", Value: "222"}},
			want: FromStrings("aaa", "111", "bbb", "222"),
		},
		{
			base: FromStrings("aaa", "111"),
			set:  []common.Label{{Name: "bbb", Value: "222"}, {Name: "bbb", Value: "333"}},
			want: FromStrings("aaa", "111", "bbb", "333"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			del:  []string{"bbb"},
			set:  []common.Label{{Name: "ddd", Value: "444"}},
			want: FromStrings("aaa", "111", "ccc", "333", "ddd", "444"),
		},
		{ // Blank value is interpreted as delete.
			base: FromStrings("aaa", "111", "bbb", "", "ccc", "333"),
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			set:  []common.Label{{Name: "bbb", Value: ""}},
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
			set:  []common.Label{{Name: "ddd", Value: "444"}},
			keep: []string{"aaa", "ddd"},
			want: FromStrings("aaa", "111", "ddd", "444"),
		},
	} {
		test := func(t *testing.T, b *Builder) {
			for _, lbl := range tcase.set {
				b.Set(lbl.Name, lbl.Value)
			}
			if len(tcase.keep) > 0 {
				b.Keep(tcase.keep...)
			}
			b.Del(tcase.del...)
			require.True(t, Equal(tcase.want, b.Labels()))

			// Check what happens when we call Range and mutate the builder.
			b.Range(func(l common.Label) {
				if l.Name == "aaa" || l.Name == "bbb" {
					b.Del(l.Name)
				}
			})
			// require.Equal(t, tcase.want.BytesWithoutLabels(nil, "aaa", "bbb"), b.Labels().Bytes(nil))
		}
		t.Run(fmt.Sprintf("NewBuilder %d", i), func(t *testing.T) {
			test(t, NewBuilder(tcase.base))
		})
		t.Run(fmt.Sprintf("NewSymbolTable %d", i), func(t *testing.T) {
			b := NewBuilderWithSymbolTable(NewSymbolTable())
			b.Reset(tcase.base)
			test(t, b)
		})
		t.Run(fmt.Sprintf("reuseBuilder %d", i), func(t *testing.T) {
			reuseBuilder.Reset(tcase.base)
			test(t, reuseBuilder)
		})
	}
	t.Run("set_after_del", func(t *testing.T) {
		b := NewBuilder(FromStrings("aaa", "111"))
		b.Del("bbb")
		b.Set("bbb", "222")
		require.Equal(t, FromStrings("aaa", "111", "bbb", "222"), b.Labels())
		require.Equal(t, "222", b.Get("bbb"))
	})
}

func TestScratchBuilder(t *testing.T) {
	for i, tcase := range []struct {
		add  []common.Label
		want Labels
	}{
		{
			add:  []common.Label{},
			want: EmptyLabels(),
		},
		{
			add:  []common.Label{{Name: "aaa", Value: "111"}},
			want: FromStrings("aaa", "111"),
		},
		{
			add:  []common.Label{{Name: "aaa", Value: "111"}, {Name: "bbb", Value: "222"}, {Name: "ccc", Value: "333"}},
			want: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
		},
		{
			add:  []common.Label{{Name: "bbb", Value: "222"}, {Name: "aaa", Value: "111"}, {Name: "ccc", Value: "333"}},
			want: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
		},
		{
			add:  []common.Label{{Name: "ddd", Value: "444"}},
			want: FromStrings("ddd", "444"),
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			b := NewScratchBuilder(len(tcase.add))
			for _, lbl := range tcase.add {
				b.Add(lbl.Name, lbl.Value)
			}
			b.Sort()
			require.True(t, Equal(tcase.want, b.Labels()))
			b.Assign(tcase.want)
			require.True(t, Equal(tcase.want, b.Labels()))
		})
	}
}

var benchmarkLabels = []common.Label{
	{Name: "job", Value: "node"},
	{Name: "instance", Value: "123.123.1.211:9090"},
	{Name: "path", Value: "/api/v1/namespaces/<namespace>/deployments/<name>"},
	{Name: "method", Value: http.MethodGet},
	{Name: "namespace", Value: "system"},
	{Name: "status", Value: "500"},
	{Name: "prometheus", Value: "prometheus-core-1"},
	{Name: "datacenter", Value: "eu-west-1"},
	{Name: "pod_name", Value: "abcdef-99999-defee"},
}

func BenchmarkBuilder(b *testing.B) {
	var l Labels
	builder := NewBuilder(EmptyLabels())
	for i := 0; i < b.N; i++ {
		builder.Reset(EmptyLabels())
		for _, l := range benchmarkLabels {
			builder.Set(l.Name, l.Value)
		}
		l = builder.Labels()
	}
	require.Equal(b, 9, l.Len())
}
