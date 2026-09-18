// Copyright 2025 The Prometheus Authors
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
// limitations under the License

package promql_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestToPiped(t *testing.T) {
	t.Cleanup(func() {
		parser.EnableExperimentalFunctions = false
		parser.ExperimentalDurationExpr = false
		parser.EnableExtendedRangeSelectors = false
	})
	parser.EnableExperimentalFunctions = true
	parser.ExperimentalDurationExpr = true
	parser.EnableExtendedRangeSelectors = true

	for _, tc := range manualPipedCases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := promql.ToPiped(tc.query)
			require.NoError(t, err)
			require.Equal(t, tc.piped, got)
		})
	}

	for _, tc := range pipedCases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := promql.ToPiped(tc.query)
			require.NoError(t, err)
			require.Equal(t, tc.piped, got)
		})
	}
}

var manualPipedCases = []pipedCase{
	{
		query: `(
    kube_deployment_status_replicas_available{namespace='monitoring',cluster!~'o11y-.+'} > 1
) 
* 
histogram_sum(sum(rate(looping_time{location='us-east1', group_name='realtime'} [2m]))) 
/ 
(
    histogram_sum(sum(rate(looping_time{location='us-east1', group_name='realtime'} [2m]))) + 
    (
        histogram_sum(sum(rate(sleeping_time{location='us-east1', group_name='realtime'} [2m]))) OR on() vector(0)
    )
)`,
		piped: `let
  x1 = kube_deployment_status_replicas_available{cluster!~"o11y-.+",namespace="monitoring"} > 1
  x2 = looping_time{group_name="realtime",location="us-east1"}[2m] | rate | sum | histogram_sum
  x3 = sleeping_time{group_name="realtime",location="us-east1"}[2m] | rate | sum | histogram_sum
  x4 = vector(0)
in x1 * x2 / (x2 + (x3 or on () x4))`,
	},
	{
		query: `avg(sum by (group) (http_requests{job="api-server"}))`,
		piped: `http_requests{job="api-server"} | sum by (group)  | avg`,
	},
}
