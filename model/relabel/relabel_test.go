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

package relabel

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   labels.Labels
		relabel []*Config
		output  labels.Labels
		drop    bool
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
			drop: true,
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
			drop: true,
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
			drop: true,
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
			// Blank replacement should delete the label.
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"f": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("(f).*"),
					TargetLabel:  "$1",
					Replacement:  "$2",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
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
				"a": "some-name-0",
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
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)(-[^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "some-name-0",
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
		{ // From https://github.com/prometheus/prometheus/issues/12283
			input: labels.FromMap(map[string]string{
				"__meta_kubernetes_pod_container_port_name":         "foo",
				"__meta_kubernetes_pod_annotation_XXX_metrics_port": "9091",
			}),
			relabel: []*Config{
				{
					Regex:  MustNewRegexp("^__meta_kubernetes_pod_container_port_name$"),
					Action: LabelDrop,
				},
				{
					SourceLabels: model.LabelNames{"__meta_kubernetes_pod_annotation_XXX_metrics_port"},
					Regex:        MustNewRegexp("(.+)"),
					Action:       Replace,
					Replacement:  "metrics",
					TargetLabel:  "__meta_kubernetes_pod_container_port_name",
				},
				{
					SourceLabels: model.LabelNames{"__meta_kubernetes_pod_container_port_name"},
					Regex:        MustNewRegexp("^metrics$"),
					Action:       Keep,
				},
			},
			output: labels.FromMap(map[string]string{
				"__meta_kubernetes_pod_annotation_XXX_metrics_port": "9091",
				"__meta_kubernetes_pod_container_port_name":         "metrics",
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
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       KeepEqual,
					TargetLabel:  "__port1",
				},
			},
			output: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       DropEqual,
					TargetLabel:  "__port1",
				},
			},
			drop: true,
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       DropEqual,
					TargetLabel:  "__port2",
				},
			},
			output: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       KeepEqual,
					TargetLabel:  "__port2",
				},
			},
			drop: true,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "line1\nline2",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("line1.*line2"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "match${1}",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "line1\nline2",
				"b": "bar",
				"c": "baz",
				"d": "match",
			}),
		},
		{ // Replace on source label with UTF-8 characters.
			input: labels.FromMap(map[string]string{
				"utf-8.label": "line1\nline2",
				"b":           "bar",
				"c":           "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"utf-8.label"},
					Regex:        MustNewRegexp("line1.*line2"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  `match${1}`,
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"utf-8.label": "line1\nline2",
				"b":           "bar",
				"c":           "baz",
				"d":           "match",
			}),
		},
		{ // Replace targetLabel with UTF-8 characters.
			input: labels.FromMap(map[string]string{
				"a": "line1\nline2",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("line1.*line2"),
					TargetLabel:  "utf-8.label",
					Separator:    ";",
					Replacement:  `match${1}`,
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":           "line1\nline2",
				"b":           "bar",
				"c":           "baz",
				"utf-8.label": "match",
			}),
		},
		{ // Replace targetLabel with UTF-8 characters and $variable.
			input: labels.FromMap(map[string]string{
				"a": "line1\nline2",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("line1.*line2"),
					TargetLabel:  "utf-8.label${1}",
					Separator:    ";",
					Replacement:  "match${1}",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":           "line1\nline2",
				"b":           "bar",
				"c":           "baz",
				"utf-8.label": "match",
			}),
		},
		{ // Replace targetLabel with UTF-8 characters and various $var styles.
			input: labels.FromMap(map[string]string{
				"a": "line1\nline2",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("(line1).*(?<second>line2)"),
					TargetLabel:  "label1_${1}",
					Separator:    ";",
					Replacement:  "val_${second}",
					Action:       Replace,
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("(line1).*(?<second>line2)"),
					TargetLabel:  "label2_$1",
					Separator:    ";",
					Replacement:  "val_$second",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":            "line1\nline2",
				"b":            "bar",
				"c":            "baz",
				"label1_line1": "val_line2",
				"label2_line1": "val_line2",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__name__": "http_requests_total",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        MustNewRegexp(".*_total$"),
					TargetLabel:  "__type__",
					Replacement:  "counter",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"__name__": "http_requests_total",
				"__type__": "counter",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__name__": "disk_usage_bytes",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        MustNewRegexp(".*_bytes$"),
					TargetLabel:  "__unit__",
					Replacement:  "bytes",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"__name__": "disk_usage_bytes",
				"__unit__": "bytes",
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
			cfg.NameValidationScheme = model.UTF8Validation
			require.NoError(t, cfg.Validate(model.UTF8Validation))
		}

		res, keep := Process(test.input, test.relabel...)
		require.Equal(t, !test.drop, keep)
		if keep {
			testutil.RequireEqual(t, test.output, res)
		}
	}
}

func TestRelabelValidate(t *testing.T) {
	tests := []struct {
		config   Config
		expected string
	}{
		{
			config: Config{
				NameValidationScheme: model.UTF8Validation,
			},
			expected: `relabel action cannot be empty`,
		},
		{
			config: Config{
				Action:               Replace,
				NameValidationScheme: model.UTF8Validation,
			},
			expected: `requires 'target_label' value`,
		},
		{
			config: Config{
				Action:               Lowercase,
				NameValidationScheme: model.UTF8Validation,
			},
			expected: `requires 'target_label' value`,
		},
		{
			config: Config{
				Action:               Lowercase,
				Replacement:          DefaultRelabelConfig.Replacement,
				TargetLabel:          "${3}", // With UTF-8 naming, this is now a legal relabel rule.
				NameValidationScheme: model.UTF8Validation,
			},
		},
		{
			config: Config{
				Action:               Lowercase,
				Replacement:          DefaultRelabelConfig.Replacement,
				TargetLabel:          "${3}", // Fails with legacy validation
				NameValidationScheme: model.LegacyValidation,
			},
			expected: "\"${3}\" is invalid 'target_label' for lowercase action",
		},
		{
			config: Config{
				SourceLabels:         model.LabelNames{"a"},
				Regex:                MustNewRegexp("some-([^-]+)-([^,]+)"),
				Action:               Replace,
				Replacement:          "${1}",
				TargetLabel:          "${3}",
				NameValidationScheme: model.UTF8Validation,
			},
		},
		{
			config: Config{
				SourceLabels:         model.LabelNames{"a"},
				Regex:                MustNewRegexp("some-([^-]+)-([^,]+)"),
				Action:               Replace,
				Replacement:          "${1}",
				TargetLabel:          "0${3}", // With UTF-8 naming this targets a valid label.
				NameValidationScheme: model.UTF8Validation,
			},
		},
		{
			config: Config{
				SourceLabels:         model.LabelNames{"a"},
				Regex:                MustNewRegexp("some-([^-]+)-([^,]+)"),
				Action:               Replace,
				Replacement:          "${1}",
				TargetLabel:          "-${3}", // With UTF-8 naming this targets a valid label.
				NameValidationScheme: model.UTF8Validation,
			},
		},
		{
			config: Config{
				Regex:                MustNewRegexp("__meta_kubernetes_pod_label_(strimzi_io_.+)"),
				Action:               LabelMap,
				Replacement:          "$1",
				NameValidationScheme: model.LegacyValidation,
			},
		},
		{
			config: Config{
				Regex:                MustNewRegexp("__meta_(.+)"),
				Action:               LabelMap,
				Replacement:          "${1}",
				NameValidationScheme: model.LegacyValidation,
			},
		},
	}
	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			err := test.config.Validate(model.UTF8Validation)
			if test.expected == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.expected)
			}
		})
	}
}

func TestTargetLabelLegacyValidity(t *testing.T) {
	for _, test := range []struct {
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
		{"bar.foo${1}bar", false},
		{"_${1}_", true},
		{"foo${bar}foo", true},
	} {
		require.Equal(t, test.valid, relabelTargetLegacy.Match([]byte(test.str)),
			"Expected %q to be %v", test.str, test.valid)
	}
}

func BenchmarkRelabel(b *testing.B) {
	tests := []struct {
		name   string
		lbls   labels.Labels
		config string
		cfgs   []*Config
	}{
		{
			name: "example", // From prometheus/config/testdata/conf.good.yml.
			config: `
            - source_labels: [job, __meta_dns_name]
              regex: "(.*)some-[regex]"
              target_label: job
              replacement: foo-${1}
              # action defaults to 'replace'
            - source_labels: [abc]
              target_label: cde
            - replacement: static
              target_label: abc
            - regex:
              replacement: static
              target_label: abc`,
			lbls: labels.FromStrings("__meta_dns_name", "example-some-x.com", "abc", "def", "job", "foo"),
		},
		{
			name: "kubernetes",
			config: `
        - source_labels:
            - __meta_kubernetes_pod_container_port_name
          regex: .*-metrics
          action: keep
        - source_labels:
            - __meta_kubernetes_pod_label_name
          action: drop
          regex: ""
        - source_labels:
            - __meta_kubernetes_pod_phase
          regex: Succeeded|Failed
          action: drop
        - source_labels:
            - __meta_kubernetes_pod_annotation_prometheus_io_scrape
          regex: "false"
          action: drop
        - source_labels:
            - __meta_kubernetes_pod_annotation_prometheus_io_scheme
          target_label: __scheme__
          regex: (https?)
          replacement: $1
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_annotation_prometheus_io_path
          target_label: __metrics_path__
          regex: (.+)
          replacement: $1
          action: replace
        - source_labels:
            - __address__
            - __meta_kubernetes_pod_annotation_prometheus_io_port
          target_label: __address__
          regex: (.+?)(\:\d+)?;(\d+)
          replacement: $1:$3
          action: replace
        - regex: __meta_kubernetes_pod_annotation_prometheus_io_param_(.+)
          replacement: __param_$1
          action: labelmap
        - regex: __meta_kubernetes_pod_label_prometheus_io_label_(.+)
          action: labelmap
        - regex: __meta_kubernetes_pod_annotation_prometheus_io_label_(.+)
          action: labelmap
        - source_labels:
            - __meta_kubernetes_namespace
            - __meta_kubernetes_pod_label_name
          separator: /
          target_label: job
          replacement: $1
          action: replace
        - source_labels:
          - __meta_kubernetes_namespace
          target_label: namespace
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_name
          target_label: pod
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_container_name
          target_label: container
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_name
            - __meta_kubernetes_pod_container_name
            - __meta_kubernetes_pod_container_port_name
          separator: ':'
          target_label: instance
          action: replace
        - target_label: cluster
          replacement: dev-us-central-0
        - source_labels:
            - __meta_kubernetes_namespace
          regex: hosted-grafana
          action: drop
        - source_labels:
            - __address__
          target_label: __tmp_hash
          modulus: 3
          action: hashmod
        - source_labels:
            - __tmp_hash
          regex: ^0$
          action: keep
        - regex: __tmp_hash
          action: labeldrop`,
			lbls: labels.FromStrings(
				"__address__", "10.132.183.40:80",
				"__meta_kubernetes_namespace", "loki-boltdb-shipper",
				"__meta_kubernetes_pod_annotation_promtail_loki_boltdb_shipper_hash", "50523b9759094a144adcec2eae0aa4ad",
				"__meta_kubernetes_pod_annotationpresent_promtail_loki_boltdb_shipper_hash", "true",
				"__meta_kubernetes_pod_container_init", "false",
				"__meta_kubernetes_pod_container_name", "promtail",
				"__meta_kubernetes_pod_container_port_name", "http-metrics",
				"__meta_kubernetes_pod_container_port_number", "80",
				"__meta_kubernetes_pod_container_port_protocol", "TCP",
				"__meta_kubernetes_pod_controller_kind", "DaemonSet",
				"__meta_kubernetes_pod_controller_name", "promtail-loki-boltdb-shipper",
				"__meta_kubernetes_pod_host_ip", "10.128.0.178",
				"__meta_kubernetes_pod_ip", "10.132.183.40",
				"__meta_kubernetes_pod_label_controller_revision_hash", "555b77cd7d",
				"__meta_kubernetes_pod_label_name", "promtail-loki-boltdb-shipper",
				"__meta_kubernetes_pod_label_pod_template_generation", "45",
				"__meta_kubernetes_pod_labelpresent_controller_revision_hash", "true",
				"__meta_kubernetes_pod_labelpresent_name", "true",
				"__meta_kubernetes_pod_labelpresent_pod_template_generation", "true",
				"__meta_kubernetes_pod_name", "promtail-loki-boltdb-shipper-jgtr7",
				"__meta_kubernetes_pod_node_name", "gke-dev-us-central-0-main-n2s8-2-14d53341-9hkr",
				"__meta_kubernetes_pod_phase", "Running",
				"__meta_kubernetes_pod_ready", "true",
				"__meta_kubernetes_pod_uid", "4c586419-7f6c-448d-aeec-ca4fa5b05e60",
				"__metrics_path__", "/metrics",
				"__scheme__", "http",
				"__scrape_interval__", "15s",
				"__scrape_timeout__", "10s",
				"job", "kubernetes-pods"),
		},
		{
			name: "static label pair",
			config: `
        - replacement: wwwwww
          target_label: wwwwww
        - replacement: yyyyyyyyyyyy
          target_label: xxxxxxxxx
        - replacement: xxxxxxxxx
          target_label: yyyyyyyyyyyy
        - source_labels: ["something"]
          target_label: with_source_labels
          replacement: value
        - replacement: dropped
          target_label: ${0}
        - replacement: ${0}
          target_label: dropped`,
			lbls: labels.FromStrings(
				"abcdefg01", "hijklmn1",
				"abcdefg02", "hijklmn2",
				"abcdefg03", "hijklmn3",
				"abcdefg04", "hijklmn4",
				"abcdefg05", "hijklmn5",
				"abcdefg06", "hijklmn6",
				"abcdefg07", "hijklmn7",
				"abcdefg08", "hijklmn8",
				"job", "foo",
			),
		},
	}
	for i := range tests {
		err := yaml.UnmarshalStrict([]byte(tests[i].config), &tests[i].cfgs)
		require.NoError(b, err)
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = Process(tt.lbls, tt.cfgs...)
			}
		})
	}
}

func TestConfig_UnmarshalThenMarshal(t *testing.T) {
	tests := []struct {
		name      string
		inputYaml string
	}{
		{
			name: "Values provided",
			inputYaml: `source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
separator: ;
regex: \\d+
target_label: __meta_kubernetes_pod_container_port_number
replacement: $1
action: replace
`,
		},
		{
			name: "No regex provided",
			inputYaml: `source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
separator: ;
target_label: __meta_kubernetes_pod_container_port_number
replacement: $1
action: keepequal
`,
		},
		{
			name: "Default regex provided",
			inputYaml: `source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
separator: ;
regex: (.*)
target_label: __meta_kubernetes_pod_container_port_number
replacement: $1
action: replace
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unmarshalled := Config{}
			err := yaml.Unmarshal([]byte(test.inputYaml), &unmarshalled)
			require.NoError(t, err)

			marshalled, err := yaml.Marshal(&unmarshalled)
			require.NoError(t, err)

			require.Equal(t, test.inputYaml, string(marshalled))
		})
	}
}

func TestRegexp_ShouldMarshalAndUnmarshalZeroValue(t *testing.T) {
	var zero Regexp

	marshalled, err := yaml.Marshal(&zero)
	require.NoError(t, err)
	require.Equal(t, "null\n", string(marshalled))

	var unmarshalled Regexp
	err = yaml.Unmarshal(marshalled, &unmarshalled)
	require.NoError(t, err)
	require.Nil(t, unmarshalled.Regexp)
}

func TestRegexp_JSONUnmarshalThenMarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Empty regex",
			input: `{"regex":""}`,
		},
		{
			name:  "string literal",
			input: `{"regex":"foo"}`,
		},
		{
			name:  "regex",
			input: `{"regex":".*foo.*"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var unmarshalled Config
			err := json.Unmarshal([]byte(test.input), &unmarshalled)
			require.NoError(t, err)

			marshalled, err := json.Marshal(&unmarshalled)
			require.NoError(t, err)

			require.Equal(t, test.input, string(marshalled))
		})
	}
}
