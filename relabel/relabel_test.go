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
	"reflect"
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   model.LabelSet
		relabel []*config.RelabelConfig
		output  model.LabelSet
	}{
		{
			input: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch${1}-ch${1}",
					Action:       config.RelabelReplace,
				},
			},
			output: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "choo-choo",
			},
		},
		{
			input: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a", "b"},
					Regex:        config.MustNewRegexp("f(.*);(.*)r"),
					TargetLabel:  "a",
					Separator:    ";",
					Replacement:  "b${1}${2}m", // boobam
					Action:       config.RelabelReplace,
				},
				{
					SourceLabels: model.LabelNames{"c", "a"},
					Regex:        config.MustNewRegexp("(b).*b(.*)ba(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1$2$2$3",
					Action:       config.RelabelReplace,
				},
			},
			output: model.LabelSet{
				"a": "boobam",
				"b": "bar",
				"c": "baz",
				"d": "boooom",
			},
		},
		{
			input: model.LabelSet{
				"a": "foo",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp(".*o.*"),
					Action:       config.RelabelDrop,
				}, {
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch$1-ch$1",
					Action:       config.RelabelReplace,
				},
			},
			output: nil,
		},
		{
			input: model.LabelSet{
				"a": "foo",
				"b": "bar",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp(".*o.*"),
					Action:       config.RelabelDrop,
				},
			},
			output: nil,
		},
		{
			input: model.LabelSet{
				"a": "abc",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp(".*(b).*"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1",
					Action:       config.RelabelReplace,
				},
			},
			output: model.LabelSet{
				"a": "abc",
				"d": "b",
			},
		},
		{
			input: model.LabelSet{
				"a": "foo",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("no-match"),
					Action:       config.RelabelDrop,
				},
			},
			output: model.LabelSet{
				"a": "foo",
			},
		},
		{
			input: model.LabelSet{
				"a": "foo",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("f|o"),
					Action:       config.RelabelDrop,
				},
			},
			output: model.LabelSet{
				"a": "foo",
			},
		},
		{
			input: model.LabelSet{
				"a": "foo",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("no-match"),
					Action:       config.RelabelKeep,
				},
			},
			output: nil,
		},
		{
			input: model.LabelSet{
				"a": "foo",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("f.*"),
					Action:       config.RelabelKeep,
				},
			},
			output: model.LabelSet{
				"a": "foo",
			},
		},
		{
			// No replacement must be applied if there is no match.
			input: model.LabelSet{
				"a": "boo",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("f"),
					TargetLabel:  "b",
					Replacement:  "bar",
					Action:       config.RelabelReplace,
				},
			},
			output: model.LabelSet{
				"a": "boo",
			},
		},
		{
			input: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"c"},
					TargetLabel:  "d",
					Separator:    ";",
					Action:       config.RelabelHashMod,
					Modulus:      1000,
				},
			},
			output: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "976",
			},
		},
		{
			input: model.LabelSet{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			},
			relabel: []*config.RelabelConfig{
				{
					Regex:       config.MustNewRegexp("(b.*)"),
					Replacement: "bar_${1}",
					Action:      config.RelabelLabelMap,
				},
			},
			output: model.LabelSet{
				"a":      "foo",
				"b1":     "bar",
				"b2":     "baz",
				"bar_b1": "bar",
				"bar_b2": "baz",
			},
		},
		{
			input: model.LabelSet{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
			},
			relabel: []*config.RelabelConfig{
				{
					Regex:       config.MustNewRegexp("__meta_(my.*)"),
					Replacement: "${1}",
					Action:      config.RelabelLabelMap,
				},
			},
			output: model.LabelSet{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
				"my_bar":        "aaa",
				"my_baz":        "bbb",
			},
		},
		{ // valid case
			input: model.LabelSet{
				"a": "some-name-value",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       config.RelabelReplace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: model.LabelSet{
				"a":    "some-name-value",
				"name": "value",
			},
		},
		{ // invalid replacement ""
			input: model.LabelSet{
				"a": "some-name-value",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       config.RelabelReplace,
					Replacement:  "${3}",
					TargetLabel:  "${1}",
				},
			},
			output: model.LabelSet{
				"a": "some-name-value",
			},
		},
		{ // invalid target_labels
			input: model.LabelSet{
				"a": "some-name-value",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       config.RelabelReplace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       config.RelabelReplace,
					Replacement:  "${1}",
					TargetLabel:  "0${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        config.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       config.RelabelReplace,
					Replacement:  "${1}",
					TargetLabel:  "-${3}",
				},
			},
			output: model.LabelSet{
				"a": "some-name-value",
			},
		},
		{ // more complex real-life like usecase
			input: model.LabelSet{
				"__meta_sd_tags": "path:/secret,job:some-job,label:foo=bar",
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        config.MustNewRegexp("(?:.+,|^)path:(/[^,]+).*"),
					Action:       config.RelabelReplace,
					Replacement:  "${1}",
					TargetLabel:  "__metrics_path__",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        config.MustNewRegexp("(?:.+,|^)job:([^,]+).*"),
					Action:       config.RelabelReplace,
					Replacement:  "${1}",
					TargetLabel:  "job",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        config.MustNewRegexp("(?:.+,|^)label:([^=]+)=([^,]+).*"),
					Action:       config.RelabelReplace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: model.LabelSet{
				"__meta_sd_tags":   "path:/secret,job:some-job,label:foo=bar",
				"__metrics_path__": "/secret",
				"job":              "some-job",
				"foo":              "bar",
			},
		},
		{
			input: model.LabelSet{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			},
			relabel: []*config.RelabelConfig{
				{
					Regex:  config.MustNewRegexp("(b.*)"),
					Action: config.RelabelLabelKeep,
				},
			},
			output: model.LabelSet{
				"b1": "bar",
				"b2": "baz",
			},
		},
		{
			input: model.LabelSet{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			},
			relabel: []*config.RelabelConfig{
				{
					Regex:  config.MustNewRegexp("(b.*)"),
					Action: config.RelabelLabelDrop,
				},
			},
			output: model.LabelSet{
				"a": "foo",
			},
		},
	}

	for i, test := range tests {
		res := Process(test.input, test.relabel...)

		if !reflect.DeepEqual(res, test.output) {
			t.Errorf("Test %d: relabel output mismatch: expected %#v, got %#v", i+1, test.output, res)
		}
	}
}
