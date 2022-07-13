// Copyright 2015 The Prometheus Authors
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

package relabel

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   labels.Labels
		relabel []*Config
		output  labels.Labels
	}{
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch${1}-ch${1}",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "choo-choo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a", "b"},
					Regex:        MustNewRegexp("f(.*);(.*)r"),
					TargetLabel:  "a",
					Separator:    ";",
					Replacement:  "b${1}${2}m", // boobam
					Action:       Replace,
				},
				{
					SourceLabels: model.LabelNames{"c", "a"},
					Regex:        MustNewRegexp("(b).*b(.*)ba(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1$2$2$3",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "boobam",
				"b": "bar",
				"c": "baz",
				"d": "boooom",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp(".*o.*"),
					Action:       Drop,
				}, {
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch$1-ch$1",
					Action:       Replace,
				},
			},
			output: nil,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp(".*o.*"),
					Action:       Drop,
				},
			},
			output: nil,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "abc",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp(".*(b).*"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "abc",
				"d": "b",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("no-match"),
					Action:       Drop,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f|o"),
					Action:       Drop,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("no-match"),
					Action:       Keep,
				},
			},
			output: nil,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f.*"),
					Action:       Keep,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			// No replacement must be applied if there is no match.
			input: labels.FromMap(map[string]string{
				"a": "boo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f"),
					TargetLabel:  "b",
					Replacement:  "bar",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "boo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"c"},
					TargetLabel:  "d",
					Separator:    ";",
					Action:       HashMod,
					Modulus:      1000,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "976",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo\nbar",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					TargetLabel:  "b",
					Separator:    ";",
					Action:       HashMod,
					Modulus:      1000,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo\nbar",
				"b": "734",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*Config{
				{
					Regex:       MustNewRegexp("(b.*)"),
					Replacement: "bar_${1}",
					Action:      LabelMap,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":      "foo",
				"b1":     "bar",
				"b2":     "baz",
				"bar_b1": "bar",
				"bar_b2": "baz",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
			}),
			relabel: []*Config{
				{
					Regex:       MustNewRegexp("__meta_(my.*)"),
					Replacement: "${1}",
					Action:      LabelMap,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
				"my_bar":        "aaa",
				"my_baz":        "bbb",
			}),
		},
		{ // valid case
			input: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a":    "some-name-value",
				"name": "value",
			}),
		},
		{ // invalid replacement ""
			input: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${3}",
					TargetLabel:  "${1}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
		},
		{ // invalid target_labels
			input: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "0${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "-${3}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
		},
		{ // more complex real-life like usecase
			input: labels.FromMap(map[string]string{
				"__meta_sd_tags": "path:/secret,job:some-job,label:foo=bar",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        MustNewRegexp("(?:.+,|^)path:(/[^,]+).*"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "__metrics_path__",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        MustNewRegexp("(?:.+,|^)job:([^,]+).*"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "job",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        MustNewRegexp("(?:.+,|^)label:([^=]+)=([^,]+).*"),
					Action:       Replace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: labels.FromMap(map[string]string{
				"__meta_sd_tags":   "path:/secret,job:some-job,label:foo=bar",
				"__metrics_path__": "/secret",
				"job":              "some-job",
				"foo":              "bar",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelKeep,
				},
			},
			output: labels.FromMap(map[string]string{
				"b1": "bar",
				"b2": "baz",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelDrop,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"foo": "bAr123Foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"foo"},
					Action:       Uppercase,
					TargetLabel:  "foo_uppercase",
				},
				{
					SourceLabels: model.LabelNames{"foo"},
					Action:       Lowercase,
					TargetLabel:  "foo_lowercase",
				},
			},
			output: labels.FromMap(map[string]string{
				"foo":           "bAr123Foo",
				"foo_lowercase": "bar123foo",
				"foo_uppercase": "BAR123FOO",
			}),
		},
	}

	for _, test := range tests {
		// Setting default fields, mimicking the behaviour in Prometheus.
		for _, cfg := range test.relabel {
			if cfg.Action == "" {
				cfg.Action = DefaultRelabelConfig.Action
			}
			if cfg.Separator == "" {
				cfg.Separator = DefaultRelabelConfig.Separator
			}
			if cfg.Regex.Regexp == nil || cfg.Regex.String() == "" {
				cfg.Regex = DefaultRelabelConfig.Regex
			}
			if cfg.Replacement == "" {
				cfg.Replacement = DefaultRelabelConfig.Replacement
			}
		}

		res := Process(test.input, test.relabel...)
		require.Equal(t, test.output, res)
	}
}

func TestTargetLabelValidity(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"-label", false},
		{"label", true},
		{"label${1}", true},
		{"${1}label", true},
		{"${1}", true},
		{"${1}label", true},
		{"${", false},
		{"$", false},
		{"${}", false},
		{"foo${", false},
		{"$1", true},
		{"asd$2asd", true},
		{"-foo${1}bar-", false},
		{"_${1}_", true},
		{"foo${bar}foo", true},
	}
	for _, test := range tests {
		require.Equal(t, test.valid, relabelTarget.Match([]byte(test.str)),
			"Expected %q to be %v", test.str, test.valid)
	}
}
