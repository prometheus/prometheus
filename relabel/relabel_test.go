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

	pkgrelabel "github.com/prometheus/prometheus/pkg/relabel"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   model.LabelSet
		relabel []*pkgrelabel.Config
		output  model.LabelSet
	}{
		{
			input: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			},
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch${1}-ch${1}",
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a", "b"},
					Regex:        pkgrelabel.MustNewRegexp("f(.*);(.*)r"),
					TargetLabel:  "a",
					Separator:    ";",
					Replacement:  "b${1}${2}m", // boobam
					Action:       pkgrelabel.Replace,
				},
				{
					SourceLabels: model.LabelNames{"c", "a"},
					Regex:        pkgrelabel.MustNewRegexp("(b).*b(.*)ba(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1$2$2$3",
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp(".*o.*"),
					Action:       pkgrelabel.Drop,
				}, {
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch$1-ch$1",
					Action:       pkgrelabel.Replace,
				},
			},
			output: nil,
		},
		{
			input: model.LabelSet{
				"a": "foo",
				"b": "bar",
			},
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp(".*o.*"),
					Action:       pkgrelabel.Drop,
				},
			},
			output: nil,
		},
		{
			input: model.LabelSet{
				"a": "abc",
			},
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp(".*(b).*"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1",
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("no-match"),
					Action:       pkgrelabel.Drop,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("f|o"),
					Action:       pkgrelabel.Drop,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("no-match"),
					Action:       pkgrelabel.Keep,
				},
			},
			output: nil,
		},
		{
			input: model.LabelSet{
				"a": "foo",
			},
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("f.*"),
					Action:       pkgrelabel.Keep,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("f"),
					TargetLabel:  "b",
					Replacement:  "bar",
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"c"},
					TargetLabel:  "d",
					Separator:    ";",
					Action:       pkgrelabel.HashMod,
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
			relabel: []*pkgrelabel.Config{
				{
					Regex:       pkgrelabel.MustNewRegexp("(b.*)"),
					Replacement: "bar_${1}",
					Action:      pkgrelabel.LabelMap,
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
			relabel: []*pkgrelabel.Config{
				{
					Regex:       pkgrelabel.MustNewRegexp("__meta_(my.*)"),
					Replacement: "${1}",
					Action:      pkgrelabel.LabelMap,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       pkgrelabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       pkgrelabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "0${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        pkgrelabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        pkgrelabel.MustNewRegexp("(?:.+,|^)path:(/[^,]+).*"),
					Action:       pkgrelabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "__metrics_path__",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        pkgrelabel.MustNewRegexp("(?:.+,|^)job:([^,]+).*"),
					Action:       pkgrelabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "job",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        pkgrelabel.MustNewRegexp("(?:.+,|^)label:([^=]+)=([^,]+).*"),
					Action:       pkgrelabel.Replace,
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
			relabel: []*pkgrelabel.Config{
				{
					Regex:  pkgrelabel.MustNewRegexp("(b.*)"),
					Action: pkgrelabel.LabelKeep,
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
			relabel: []*pkgrelabel.Config{
				{
					Regex:  pkgrelabel.MustNewRegexp("(b.*)"),
					Action: pkgrelabel.LabelDrop,
				},
			},
			output: model.LabelSet{
				"a": "foo",
			},
		},
	}

	for _, test := range tests {
		res := Process(test.input, test.relabel...)
		testutil.Equals(t, res, test.output)
	}
}
