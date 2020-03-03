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
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

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
		testutil.Equals(t, test.expected, got, "unexpected labelset for test case %d", i)
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
		testutil.Equals(t, c.Duplicate, d, "test %d: incorrect duplicate bool", i)
		testutil.Equals(t, c.LabelName, l, "test %d: incorrect label name", i)
	}
}

func TestLabels_WithoutEmpty(t *testing.T) {
	tests := []struct {
		input    Labels
		expected Labels
	}{
		{
			input: Labels{
				{
					Name:  "__name__",
					Value: "test",
				},
				{
					Name: "foo",
				},
				{
					Name:  "hostname",
					Value: "localhost",
				},
				{
					Name: "bar",
				},
				{
					Name:  "job",
					Value: "check",
				},
			},
			expected: Labels{
				{
					Name:  "__name__",
					Value: "test",
				},
				{
					Name:  "hostname",
					Value: "localhost",
				},
				{
					Name:  "job",
					Value: "check",
				},
			},
		},
		{
			input: Labels{
				{
					Name:  "__name__",
					Value: "test",
				},
				{
					Name:  "hostname",
					Value: "localhost",
				},
				{
					Name:  "job",
					Value: "check",
				},
			},
			expected: Labels{
				{
					Name:  "__name__",
					Value: "test",
				},
				{
					Name:  "hostname",
					Value: "localhost",
				},
				{
					Name:  "job",
					Value: "check",
				},
			},
		},
	}

	for i, test := range tests {
		got := test.input.WithoutEmpty()
		testutil.Equals(t, test.expected, got, "unexpected labelset for test case %d", i)
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
		testutil.Equals(t, test.expected, got, "unexpected comparison result for test case %d", i)
	}
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
		testutil.Equals(t, test.expected, got, "unexpected comparison result for test case %d", i)
	}
}
