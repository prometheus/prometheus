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

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestSDCheckResult(t *testing.T) {
	t.Parallel()

	reg, err := relabel.NewRegexp("(.*)")
	require.NoError(t, err)

	defaultLabels := func(address string, extra ...string) labels.Labels {
		return labels.FromStrings(append([]string{
			"__address__", address,
			"__always_scrape_classic_histograms__", "false",
			"__convert_classic_histograms_to_nhcb__", "false",
			"__scrape_interval__", "1m",
			"__scrape_native_histograms__", "false",
			"__scrape_timeout__", "10s",
		}, extra...)...)
	}

	// The expectations are written against the JSON that `promtool check service-discovery` prints.
	type jsonResult struct {
		DiscoveredLabels labels.Labels `json:"discoveredLabels"`
		Labels           labels.Labels `json:"labels"`
		Error            string        `json:"error,omitempty"`
	}

	for _, tc := range []struct {
		name           string
		targets        []model.LabelSet
		relabelConfigs []*relabel.Config
		expected       []jsonResult
	}{
		{
			name: "relabeling succeeds",
			targets: []model.LabelSet{
				{"__address__": "localhost:8080", "foo": "bar"},
			},
			relabelConfigs: []*relabel.Config{{
				SourceLabels:         model.LabelNames{"foo"},
				Action:               relabel.Replace,
				TargetLabel:          "newfoo",
				Regex:                reg,
				Replacement:          "$1",
				NameValidationScheme: model.UTF8Validation,
			}},
			expected: []jsonResult{
				{
					DiscoveredLabels: defaultLabels("localhost:8080", "foo", "bar"),
					Labels:           defaultLabels("localhost:8080", "foo", "bar", "instance", "localhost:8080", "newfoo", "bar"),
				},
			},
		},
		{
			// The error message must be part of the output.
			name: "relabeling removes the address of a target",
			targets: []model.LabelSet{
				{"__address__": "localhost:8080"},
			},
			relabelConfigs: []*relabel.Config{{
				Action:               relabel.Replace,
				TargetLabel:          "__address__",
				Regex:                reg,
				Replacement:          "",
				NameValidationScheme: model.UTF8Validation,
			}},
			expected: []jsonResult{
				{
					DiscoveredLabels: defaultLabels("localhost:8080"),
					Labels:           labels.EmptyLabels(),
					Error:            "no address",
				},
			},
		},
		{
			// Every failing target must be reported.
			name: "relabeling removes the address of every target",
			targets: []model.LabelSet{
				{"__address__": "localhost:8080"},
				{"__address__": "localhost:8081"},
			},
			relabelConfigs: []*relabel.Config{{
				Action:               relabel.Replace,
				TargetLabel:          "__address__",
				Regex:                reg,
				Replacement:          "",
				NameValidationScheme: model.UTF8Validation,
			}},
			expected: []jsonResult{
				{
					DiscoveredLabels: defaultLabels("localhost:8080"),
					Labels:           labels.EmptyLabels(),
					Error:            "no address",
				},
				{
					DiscoveredLabels: defaultLabels("localhost:8081"),
					Labels:           labels.EmptyLabels(),
					Error:            "no address",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			targetGroups := []*targetgroup.Group{{Targets: tc.targets}}
			scrapeConfig := &config.ScrapeConfig{
				ScrapeInterval: model.Duration(1 * time.Minute),
				ScrapeTimeout:  model.Duration(10 * time.Second),
				RelabelConfigs: tc.relabelConfigs,
			}

			out, err := json.Marshal(getSDCheckResult(targetGroups, scrapeConfig))
			require.NoError(t, err)

			var got []jsonResult
			require.NoError(t, json.Unmarshal(out, &got), "unexpected JSON: %s", out)
			testutil.RequireEqual(t, tc.expected, got)
		})
	}
}
