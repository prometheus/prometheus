// Copyright 2018 The Prometheus Authors
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

package targetgroup

import (
	"errors"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestTargetGroupStrictJsonUnmarshal(t *testing.T) {
	tests := []struct {
		json          string
		expectedReply error
		expectedGroup Group
	}{
		{
			json: `	{"labels": {},"targets": []}`,
			expectedReply: nil,
			expectedGroup: Group{Targets: []model.LabelSet{}, Labels: model.LabelSet{}},
		},
		{
			json: `	{"labels": {"my":"label"},"targets": ["localhost:9090","localhost:9091"]}`,
			expectedReply: nil,
			expectedGroup: Group{Targets: []model.LabelSet{
				{"__address__": "localhost:9090"},
				{"__address__": "localhost:9091"},
			}, Labels: model.LabelSet{"my": "label"}},
		},
		{
			json: `	{"label": {},"targets": []}`,
			expectedReply: errors.New("json: unknown field \"label\""),
		},
		{
			json: `	{"labels": {},"target": []}`,
			expectedReply: errors.New("json: unknown field \"target\""),
		},
	}

	for _, test := range tests {
		tg := Group{}
		actual := tg.UnmarshalJSON([]byte(test.json))
		require.Equal(t, test.expectedReply, actual)
		require.Equal(t, test.expectedGroup, tg)
	}
}

func TestTargetGroupYamlMarshal(t *testing.T) {
	marshal := func(g interface{}) []byte {
		d, err := yaml.Marshal(g)
		if err != nil {
			panic(err)
		}
		return d
	}

	tests := []struct {
		expectedYaml string
		expectedErr  error
		group        Group
	}{
		{
			// labels should be omitted if empty.
			group:        Group{},
			expectedYaml: "targets: []\n",
			expectedErr:  nil,
		},
		{
			// targets only exposes addresses.
			group: Group{
				Targets: []model.LabelSet{
					{"__address__": "localhost:9090"},
					{"__address__": "localhost:9091"},
				},
				Labels: model.LabelSet{"foo": "bar", "bar": "baz"},
			},
			expectedYaml: "targets:\n- localhost:9090\n- localhost:9091\nlabels:\n  bar: baz\n  foo: bar\n",
			expectedErr:  nil,
		},
	}

	for _, test := range tests {
		actual, err := test.group.MarshalYAML()
		require.Equal(t, test.expectedErr, err)
		require.Equal(t, test.expectedYaml, string(marshal(actual)))
	}
}

func TestTargetGroupYamlUnmarshal(t *testing.T) {
	unmarshal := func(d []byte) func(interface{}) error {
		return func(o interface{}) error {
			return yaml.Unmarshal(d, o)
		}
	}
	tests := []struct {
		yaml          string
		expectedGroup Group
		expectedReply error
	}{
		{
			// empty target group.
			yaml:          "labels:\ntargets:\n",
			expectedGroup: Group{Targets: []model.LabelSet{}},
			expectedReply: nil,
		},
		{
			// brackets syntax.
			yaml:          "labels:\n  my:  label\ntargets:\n  ['localhost:9090', 'localhost:9191']",
			expectedReply: nil,
			expectedGroup: Group{Targets: []model.LabelSet{
				{"__address__": "localhost:9090"},
				{"__address__": "localhost:9191"},
			}, Labels: model.LabelSet{"my": "label"}},
		},
		{
			// incorrect syntax.
			yaml:          "labels:\ntargets:\n  'localhost:9090'",
			expectedReply: &yaml.TypeError{Errors: []string{"line 3: cannot unmarshal !!str `localho...` into []string"}},
		},
	}

	for _, test := range tests {
		tg := Group{}
		actual := tg.UnmarshalYAML(unmarshal([]byte(test.yaml)))
		require.Equal(t, test.expectedReply, actual)
		require.Equal(t, test.expectedGroup, tg)
	}
}

func TestString(t *testing.T) {
	// String() should return only the source, regardless of other attributes.
	group1 := Group{
		Targets: []model.LabelSet{
			{"__address__": "localhost:9090"},
			{"__address__": "localhost:9091"},
		},
		Source: "<source>",
		Labels: model.LabelSet{"foo": "bar", "bar": "baz"},
	}
	group2 := Group{
		Targets: []model.LabelSet{},
		Source:  "<source>",
		Labels:  model.LabelSet{},
	}
	require.Equal(t, "<source>", group1.String())
	require.Equal(t, "<source>", group2.String())
	require.Equal(t, group1.String(), group2.String())
}
