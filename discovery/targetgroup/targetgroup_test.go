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

	"github.com/prometheus/prometheus/util/testutil"
	"gopkg.in/yaml.v2"
)

func TestTargetGroupStrictJsonUnmarshal(t *testing.T) {
	tests := []struct {
		json          string
		expectedReply error
	}{
		{
			json: `	{"labels": {},"targets": []}`,
			expectedReply: nil,
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
	tg := Group{}

	for _, test := range tests {
		actual := tg.UnmarshalJSON([]byte(test.json))
		testutil.Equals(t, test.expectedReply, actual)
	}

}

func TestTargetGroupYamlUnmarshal(t *testing.T) {
	unmarshal := func(d []byte) func(interface{}) error {
		return func(o interface{}) error {
			return yaml.Unmarshal(d, o)
		}
	}
	tests := []struct {
		yaml                    string
		expectedNumberOfTargets int
		expectedNumberOfLabels  int
		expectedReply           error
	}{
		{
			yaml:                    "labels:\ntargets:\n",
			expectedNumberOfTargets: 0,
			expectedNumberOfLabels:  0,
			expectedReply:           nil,
		},
		{
			yaml:                    "labels:\n  my:  label\ntargets:\n  ['localhost:9090', 'localhost:9191']",
			expectedNumberOfTargets: 2,
			expectedNumberOfLabels:  1,
			expectedReply:           nil,
		},
		{
			yaml:                    "labels:\ntargets:\n  'localhost:9090'",
			expectedNumberOfTargets: 0,
			expectedNumberOfLabels:  0,
			expectedReply:           &yaml.TypeError{Errors: []string{"line 3: cannot unmarshal !!str `localho...` into []string"}},
		},
	}

	for _, test := range tests {
		tg := Group{}
		actual := tg.UnmarshalYAML(unmarshal([]byte(test.yaml)))
		testutil.Equals(t, test.expectedReply, actual)
		testutil.Equals(t, test.expectedNumberOfTargets, len(tg.Targets))
		testutil.Equals(t, test.expectedNumberOfLabels, len(tg.Labels))
	}

}
