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
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

var (
	s254 = strings.Repeat("x", 254) // Edge cases for stringlabels encoding.
	s255 = strings.Repeat("x", 255)
)

var testCaseLabels = []Labels{
	FromStrings("t1", "t1", "t2", "t2"),
	{},
	FromStrings("service.name", "t1", "whatever\\whatever", "t2"),
	FromStrings("aaa", "111", "xx", s254),
	FromStrings("aaa", "111", "xx", s255),
	FromStrings("__name__", "kube_pod_container_status_last_terminated_exitcode", "cluster", "prod-af-north-0", " container", "prometheus", "instance", "kube-state-metrics-0:kube-state-metrics:ksm", "job", "kube-state-metrics/kube-state-metrics", " namespace", "observability-prometheus", "pod", "observability-prometheus-0", "uid", "d3ec90b2-4975-4607-b45d-b9ad64bb417e"),
}

func TestLabels_String(t *testing.T) {
	expected := []string{ // Values must line up with testCaseLabels.
		"{t1=\"t1\", t2=\"t2\"}",
		"{}",
		`{"service.name"="t1", "whatever\\whatever"="t2"}`,
		`{aaa="111", xx="` + s254 + `"}`,
		`{aaa="111", xx="` + s255 + `"}`,
		`{" container"="prometheus", " namespace"="observability-prometheus", __name__="kube_pod_container_status_last_terminated_exitcode", cluster="prod-af-north-0", instance="kube-state-metrics-0:kube-state-metrics:ksm", job="kube-state-metrics/kube-state-metrics", pod="observability-prometheus-0", uid="d3ec90b2-4975-4607-b45d-b9ad64bb417e"}`,
	}
	require.Len(t, expected, len(testCaseLabels))
	for i, c := range expected {
		str := testCaseLabels[i].String()
		require.Equal(t, c, str)
	}
}

func BenchmarkString(b *testing.B) {
	ls := New(benchmarkLabels...)
	for b.Loop() {
		_ = ls.String()
	}
}

func TestSizeOfLabels(t *testing.T) {
	require.Len(t, expectedSizeOfLabels, len(testCaseLabels))
	for i, c := range expectedSizeOfLabels { // Declared in build-tag-specific files, e.g. labels_slicelabels_test.go.
		var total uint64
		testCaseLabels[i].Range(func(l Label) {
			total += SizeOfLabels(l.Name, l.Value, 1)
		})
		require.Equal(t, c, total)
	}
}

func TestByteSize(t *testing.T) {
	require.Len(t, expectedByteSize, len(testCaseLabels))
	for i, c := range expectedByteSize { // Declared in build-tag-specific files, e.g. labels_slicelabels_test.go.
		require.Equal(t, c, testCaseLabels[i].ByteSize())
	}
}

var GlobalTotal uint64 // Encourage the compiler not to elide the benchmark computation.

func BenchmarkSize(b *testing.B) {
	lb := New(benchmarkLabels...)
	b.Run("SizeOfLabels", func(b *testing.B) {
		for b.Loop() {
			var total uint64
			lb.Range(func(l Label) {
				total += SizeOfLabels(l.Name, l.Value, 1)
			})
			GlobalTotal = total
		}
	})
	b.Run("ByteSize", func(b *testing.B) {
		for b.Loop() {
			GlobalTotal = lb.ByteSize()
		}
	})
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
		require.True(t, Equal(test.expected, got), "unexpected labelset for test case %d", i)
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
			require.True(t, Equal(test.expected, test.input.WithoutEmpty()))
		})
	}
}

func TestLabels_IsValid(t *testing.T) {
	for _, test := range []struct {
		input    Labels
		expected bool
	}{
		{
			input: FromStrings(
				"__name__", "test",
				"hostname", "localhost",
				"job", "check",
			),
			expected: true,
		},
		{
			input: FromStrings(
				"__name__", "test:ms",
				"hostname_123", "localhost",
				"_job", "check",
			),
			expected: true,
		},
		{
			input:    FromStrings("__name__", "test-ms"),
			expected: false,
		},
		{
			input:    FromStrings("__name__", "0zz"),
			expected: false,
		},
		{
			input:    FromStrings("abc:xyz", "invalid"),
			expected: false,
		},
		{
			input:    FromStrings("123abc", "invalid"),
			expected: false,
		},
		{
			input:    FromStrings("中文abc", "invalid"),
			expected: false,
		},
		{
			input:    FromStrings("invalid", "aa\xe2"),
			expected: false,
		},
		{
			input:    FromStrings("invalid", "\xF7\xBF\xBF\xBF"),
			expected: false,
		},
	} {
		t.Run("", func(t *testing.T) {
			require.Equal(t, test.expected, test.input.IsValid(model.LegacyValidation))
		})
	}
}

func TestLabels_ValidationModes(t *testing.T) {
	for _, test := range []struct {
		input    Labels
		callMode model.ValidationScheme
		expected bool
	}{
		{
			input: FromStrings(
				"__name__", "test.metric",
				"hostname", "localhost",
				"job", "check",
			),
			callMode: model.UTF8Validation,
			expected: true,
		},
		{
			input: FromStrings(
				"__name__", "test",
				"\xc5 bad utf8", "localhost",
				"job", "check",
			),
			callMode: model.UTF8Validation,
			expected: false,
		},
		{
			input: FromStrings(
				"__name__", "test.utf8.metric",
				"hostname", "localhost",
				"job", "check",
			),
			callMode: model.LegacyValidation,
			expected: false,
		},
		{
			input: FromStrings(
				"__name__", "test",
				"host.name", "localhost",
				"job", "check",
			),
			callMode: model.LegacyValidation,
			expected: false,
		},
	} {
		require.Equal(t, test.expected, test.input.IsValid(test.callMode))
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
	x := 0
	labels.Range(func(l Label) {
		switch x {
		case 0:
			require.Equal(t, Label{Name: "aaa", Value: "111"}, l, "unexpected value")
		case 1:
			require.Equal(t, Label{Name: "bbb", Value: "222"}, l, "unexpected value")
		default:
			t.Fatalf("unexpected labelset value %d: %v", x, l)
		}
		x++
	})

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
				"aaa", "111",
				"bb", "222"),
			expected: 1,
		},
		{
			compared: FromStrings(
				"aaa", "111",
				"bbbb", "222"),
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
		{
			compared: EmptyLabels(),
			expected: 1,
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
		got = Compare(test.compared, labels)
		require.Equal(t, -sign(test.expected), sign(got), "unexpected comparison result for reverse test case %d", i)
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
	require.Empty(t, FromStrings("aaa", "111", "bbb", "222").Get("foo"))
	require.Equal(t, "111", FromStrings("aaaa", "111", "bbb", "222").Get("aaaa"))
	require.Equal(t, "222", FromStrings("aaaa", "111", "bbb", "222").Get("bbb"))
}

func TestLabels_DropMetricName(t *testing.T) {
	require.True(t, Equal(FromStrings("aaa", "111", "bbb", "222"), FromStrings("aaa", "111", "bbb", "222").DropMetricName()))
	require.True(t, Equal(FromStrings("aaa", "111"), FromStrings(MetricName, "myname", "aaa", "111").DropMetricName()))

	original := FromStrings("__aaa__", "111", MetricName, "myname", "bbb", "222")
	check := original.Copy()
	require.True(t, Equal(FromStrings("__aaa__", "111", "bbb", "222"), check.DropMetricName()))
	require.True(t, Equal(original, check))
}

func TestLabels_DropReserved(t *testing.T) {
	shouldDropFn := func(n string) bool {
		return n == MetricName || n == "__something__"
	}
	require.True(t, Equal(FromStrings("aaa", "111", "bbb", "222"), FromStrings("aaa", "111", "bbb", "222").DropReserved(shouldDropFn)))
	require.True(t, Equal(FromStrings("aaa", "111"), FromStrings(MetricName, "myname", "aaa", "111").DropReserved(shouldDropFn)))
	require.True(t, Equal(FromStrings("aaa", "111"), FromStrings(MetricName, "myname", "__something__", string(model.MetricTypeCounter), "aaa", "111").DropReserved(shouldDropFn)))

	original := FromStrings("__aaa__", "111", MetricName, "myname", "bbb", "222")
	check := original.Copy()
	require.True(t, Equal(FromStrings("__aaa__", "111", "bbb", "222"), check.DropReserved(shouldDropFn)))
	require.True(t, Equal(original, check))
}

func ScratchBuilderForBenchmark() ScratchBuilder {
	// (Only relevant to -tags dedupelabels: stuff the symbol table before adding the real labels, to avoid having everything fitting into 1 byte.)
	b := NewScratchBuilder(256)
	for i := range 256 {
		b.Add(fmt.Sprintf("name%d", i), fmt.Sprintf("value%d", i))
	}
	b.Labels()
	b.Reset()
	return b
}

func NewForBenchmark(ls ...Label) Labels {
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
	allLabels := make([]Label, maxLabels)
	for i := range maxLabels {
		allLabels[i] = Label{Name: strings.Repeat(string('a'+byte(i)), 5+(i%5))}
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
						for b.Loop() {
							_ = labels.Get(scenario.label)
						}
					})
					b.Run("has", func(b *testing.B) {
						for b.Loop() {
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
			for b.Loop() {
				_ = Equal(scenario.base, scenario.other)
			}
		})
	}
}

func BenchmarkLabels_Compare(b *testing.B) {
	for _, scenario := range comparisonBenchmarkScenarios {
		b.Run(scenario.desc, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_ = Compare(scenario.base, scenario.other)
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
	reuseBuilder := NewBuilderWithSymbolTable(NewSymbolTable())
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
			base: EmptyLabels(),
			set:  []Label{{"aaa", "444"}, {"bbb", "555"}, {"ccc", "666"}},
			want: FromStrings("aaa", "444", "bbb", "555", "ccc", "666"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			set:  []Label{{"aaa", "444"}, {"bbb", "555"}, {"ccc", "666"}},
			want: FromStrings("aaa", "444", "bbb", "555", "ccc", "666"),
		},
		{
			base: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
			del:  []string{"bbb"},
			want: FromStrings("aaa", "111", "ccc", "333"),
		},
		{
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
			b.Range(func(l Label) {
				if l.Name == "aaa" || l.Name == "bbb" {
					b.Del(l.Name)
				}
			})
			require.Equal(t, tcase.want.BytesWithoutLabels(nil, "aaa", "bbb"), b.Labels().Bytes(nil))
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
		add  []Label
		want Labels
	}{
		{
			add:  []Label{},
			want: EmptyLabels(),
		},
		{
			add:  []Label{{"aaa", "111"}},
			want: FromStrings("aaa", "111"),
		},
		{
			add:  []Label{{"aaa", "111"}, {"bbb", "222"}, {"ccc", "333"}},
			want: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
		},
		{
			add:  []Label{{"bbb", "222"}, {"aaa", "111"}, {"ccc", "333"}},
			want: FromStrings("aaa", "111", "bbb", "222", "ccc", "333"),
		},
		{
			add:  []Label{{"ddd", "444"}},
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

func TestLabels_Hash(t *testing.T) {
	lbls := FromStrings("foo", "bar", "baz", "qux")
	hash1, hash2 := lbls.Hash(), lbls.Hash()
	require.Equal(t, hash1, hash2)
	require.NotEqual(t, lbls.Hash(), FromStrings("foo", "bar").Hash(), "different labels match.")
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
				for i := range 10 {
					// Label ~20B name, 50B value.
					b.Set(fmt.Sprintf("abcdefghijabcdefghijabcdefghij%d", i), fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i))
				}
				return b.Labels()
			}(),
		},
		{
			name: "bigger labels over 1KB",
			lbls: func() Labels {
				b := NewBuilder(EmptyLabels())
				for i := range 10 {
					// Label ~50B name, 50B value.
					b.Set(fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i), fmt.Sprintf("abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij%d", i))
				}
				return b.Labels()
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
			for b.Loop() {
				h = tcase.lbls.Hash()
			}
			benchmarkLabelsResult = h
		})
	}
}

var benchmarkLabels = []Label{
	{"job", "node"},
	{"instance", "123.123.1.211:9090"},
	{"path", "/api/v1/namespaces/<namespace>/deployments/<name>"},
	{"method", http.MethodGet},
	{"namespace", "system"},
	{"status", "500"},
	{"prometheus", "prometheus-core-1"},
	{"datacenter", "eu-west-1"},
	{"pod_name", "abcdef-99999-defee"},
}

func BenchmarkBuilder(b *testing.B) {
	var l Labels
	builder := NewBuilder(EmptyLabels())
	for b.Loop() {
		builder.Reset(EmptyLabels())
		for _, l := range benchmarkLabels {
			builder.Set(l.Name, l.Value)
		}
		l = builder.Labels()
	}
	require.Equal(b, 9, l.Len())
}

func BenchmarkLabels_Copy(b *testing.B) {
	l := NewForBenchmark(benchmarkLabels...)

	for b.Loop() {
		l = l.Copy()
	}
}

func TestMarshaling(t *testing.T) {
	lbls := FromStrings("aaa", "111", "bbb", "2222", "ccc", "33333")
	expectedJSON := "{\"aaa\":\"111\",\"bbb\":\"2222\",\"ccc\":\"33333\"}"
	b, err := json.Marshal(lbls)
	require.NoError(t, err)
	require.JSONEq(t, expectedJSON, string(b))

	var gotJ Labels
	err = json.Unmarshal(b, &gotJ)
	require.NoError(t, err)
	require.Equal(t, lbls, gotJ)

	expectedYAML := "aaa: \"111\"\nbbb: \"2222\"\nccc: \"33333\"\n"
	b, err = yaml.Marshal(lbls)
	require.NoError(t, err)
	require.YAMLEq(t, expectedYAML, string(b))

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
	require.JSONEq(t, expectedJSONFromStruct, string(b))

	var gotFJ foo
	err = json.Unmarshal(b, &gotFJ)
	require.NoError(t, err)
	require.Equal(t, f, gotFJ)

	b, err = yaml.Marshal(f)
	require.NoError(t, err)
	expectedYAMLFromStruct := "a_labels:\n  aaa: \"111\"\n  bbb: \"2222\"\n  ccc: \"33333\"\n"
	require.YAMLEq(t, expectedYAMLFromStruct, string(b))

	var gotFY foo
	err = yaml.Unmarshal(b, &gotFY)
	require.NoError(t, err)
	require.Equal(t, f, gotFY)
}
