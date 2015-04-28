package retrieval

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	pb "github.com/prometheus/prometheus/config/generated"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   clientmodel.LabelSet
		relabel []pb.RelabelConfig
		output  clientmodel.LabelSet
	}{
		{
			input: clientmodel.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a"},
					Regex:       proto.String("f(.*)"),
					TargetLabel: proto.String("d"),
					Separator:   proto.String(";"),
					Replacement: proto.String("ch${1}-ch${1}"),
					Action:      pb.RelabelConfig_REPLACE.Enum(),
				},
			},
			output: clientmodel.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "choo-choo",
			},
		},
		{
			input: clientmodel.LabelSet{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a", "b"},
					Regex:       proto.String("^f(.*);(.*)r$"),
					TargetLabel: proto.String("a"),
					Separator:   proto.String(";"),
					Replacement: proto.String("b${1}${2}m"), // boobam
				},
				{
					SourceLabel: []string{"c", "a"},
					Regex:       proto.String("(b).*b(.*)ba(.*)"),
					TargetLabel: proto.String("d"),
					Separator:   proto.String(";"),
					Replacement: proto.String("$1$2$2$3"),
					Action:      pb.RelabelConfig_REPLACE.Enum(),
				},
			},
			output: clientmodel.LabelSet{
				"a": "boobam",
				"b": "bar",
				"c": "baz",
				"d": "boooom",
			},
		},
		{
			input: clientmodel.LabelSet{
				"a": "foo",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a"},
					Regex:       proto.String("o$"),
					Action:      pb.RelabelConfig_DROP.Enum(),
				}, {
					SourceLabel: []string{"a"},
					Regex:       proto.String("f(.*)"),
					TargetLabel: proto.String("d"),
					Separator:   proto.String(";"),
					Replacement: proto.String("ch$1-ch$1"),
					Action:      pb.RelabelConfig_REPLACE.Enum(),
				},
			},
			output: nil,
		},
		{
			input: clientmodel.LabelSet{
				"a": "foo",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a"},
					Regex:       proto.String("no-match"),
					Action:      pb.RelabelConfig_DROP.Enum(),
				},
			},
			output: clientmodel.LabelSet{
				"a": "foo",
			},
		},
		{
			input: clientmodel.LabelSet{
				"a": "foo",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a"},
					Regex:       proto.String("no-match"),
					Action:      pb.RelabelConfig_KEEP.Enum(),
				},
			},
			output: nil,
		},
		{
			input: clientmodel.LabelSet{
				"a": "foo",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a"},
					Regex:       proto.String("^f"),
					Action:      pb.RelabelConfig_KEEP.Enum(),
				},
			},
			output: clientmodel.LabelSet{
				"a": "foo",
			},
		},
		{
			// No replacement must be applied if there is no match.
			input: clientmodel.LabelSet{
				"a": "boo",
			},
			relabel: []pb.RelabelConfig{
				{
					SourceLabel: []string{"a"},
					Regex:       proto.String("^f"),
					Action:      pb.RelabelConfig_REPLACE.Enum(),
					TargetLabel: proto.String("b"),
					Replacement: proto.String("bar"),
				},
			},
			output: clientmodel.LabelSet{
				"a": "boo",
			},
		},
	}

	for i, test := range tests {
		var relabel []*config.RelabelConfig
		for _, rl := range test.relabel {
			proto.SetDefaults(&rl)
			relabel = append(relabel, &config.RelabelConfig{rl})
		}
		res, err := Relabel(test.input, relabel...)
		if err != nil {
			t.Errorf("Test %d: error relabeling: %s", i+1, err)
		}

		if !reflect.DeepEqual(res, test.output) {
			t.Errorf("Test %d: relabel output mismatch: expected %#v, got %#v", i+1, test.output, res)
		}
	}
}
