package retrieval

import (
	"reflect"
	"regexp"
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
					Regex:        &config.Regexp{*regexp.MustCompile("f(.*)")},
					TargetLabel:  model.LabelName("d"),
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
					Regex:        &config.Regexp{*regexp.MustCompile("^f(.*);(.*)r$")},
					TargetLabel:  model.LabelName("a"),
					Separator:    ";",
					Replacement:  "b${1}${2}m", // boobam
					Action:       config.RelabelReplace,
				},
				{
					SourceLabels: model.LabelNames{"c", "a"},
					Regex:        &config.Regexp{*regexp.MustCompile("(b).*b(.*)ba(.*)")},
					TargetLabel:  model.LabelName("d"),
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
					Regex:        &config.Regexp{*regexp.MustCompile("o$")},
					Action:       config.RelabelDrop,
				}, {
					SourceLabels: model.LabelNames{"a"},
					Regex:        &config.Regexp{*regexp.MustCompile("f(.*)")},
					TargetLabel:  model.LabelName("d"),
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
			},
			relabel: []*config.RelabelConfig{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        &config.Regexp{*regexp.MustCompile("no-match")},
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
					Regex:        &config.Regexp{*regexp.MustCompile("no-match")},
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
					Regex:        &config.Regexp{*regexp.MustCompile("^f")},
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
					Regex:        &config.Regexp{*regexp.MustCompile("^f")},
					TargetLabel:  model.LabelName("b"),
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
					TargetLabel:  model.LabelName("d"),
					Separator:    ";",
					Action:       config.RelabelHashMod,
					Modulus:      1000,
				},
			},
			output: model.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "58",
			},
		},
	}

	for i, test := range tests {
		res, err := Relabel(test.input, test.relabel...)
		if err != nil {
			t.Errorf("Test %d: error relabeling: %s", i+1, err)
		}

		if !reflect.DeepEqual(res, test.output) {
			t.Errorf("Test %d: relabel output mismatch: expected %#v, got %#v", i+1, test.output, res)
		}
	}
}
