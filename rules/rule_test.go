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
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
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
			require.Equal(t, tc.want, slog.AnyValue(tc.rule).Resolve().Any())

			data, err := json.Marshal(tc.rule)
			require.NoError(t, err)
			if tc.want == nil {
				require.JSONEq(t, "null", string(data))
			} else {
				require.JSONEq(t, "{}", string(data))
			}

			for _, style := range []promslog.LogStyle{promslog.SlogStyle, promslog.GoKitStyle} {
				for _, formatName := range []string{"json", "logfmt"} {
					for _, placement := range []string{"direct", "with", "group", "with-group"} {
						t.Run(string(style)+"/"+formatName+"/"+placement, func(t *testing.T) {
							var output bytes.Buffer
							format := promslog.NewFormat()
							require.NoError(t, format.Set(formatName))
							logger := promslog.New(&promslog.Config{Writer: &output, Format: format, Style: style})
							switch placement {
							case "direct":
								logger.Info("test", "rule", tc.rule)
							case "with":
								logger.With("rule", tc.rule).Info("test")
							case "group":
								logger.Info("test", slog.Group("group", "rule", tc.rule))
							case "with-group":
								logger.WithGroup("group").With("rule", tc.rule).Info("test")
							}

							if formatName == "json" {
								var entry map[string]any
								require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
								if placement == "group" || placement == "with-group" {
									entry = entry["group"].(map[string]any)
								}
								require.Contains(t, entry, "rule")
								require.Equal(t, tc.want, entry["rule"])
							} else {
								key := "rule"
								if placement == "group" || placement == "with-group" {
									key = "group.rule"
								}
								want := "<nil>"
								if tc.want != nil {
									want = fmt.Sprintf("%q", tc.want)
								}
								require.Contains(t, output.String(), key+"="+want)
							}
						})
					}
				}
			}
		})
	}
}
