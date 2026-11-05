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

package rules

import (
	"encoding/json"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRuleLogValue(t *testing.T) {
	expr, err := testParser.ParseExpr("up")
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		rule Rule
		want any
	}{
		{
			name: "recording",
			rule: NewRecordingRule("recorded_metric", expr, labels.EmptyLabels()),
			want: "record: recorded_metric\nexpr: up\n",
		},
		{
			name: "alerting",
			rule: NewAlertingRule("TestAlert", expr, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", false, promslog.NewNopLogger()),
			want: "alert: TestAlert\nexpr: up\n",
		},
		{name: "nil recording", rule: (*RecordingRule)(nil)},
		{name: "nil alerting", rule: (*AlertingRule)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.rule)
			require.NoError(t, err)
			if tc.want == nil {
				require.JSONEq(t, "null", string(data))
			} else {
				require.JSONEq(t, "{}", string(data))
			}

			testutil.RequireLogValue(t, "rule", tc.rule, tc.want)
		})
	}
}
