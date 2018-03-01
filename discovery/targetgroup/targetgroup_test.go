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
