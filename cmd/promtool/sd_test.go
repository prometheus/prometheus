// Copyright 2021 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"

	"github.com/stretchr/testify/require"
)

func TestSDCheckResult(t *testing.T) {
	targetGroups := []*targetgroup.Group{{
		Targets: []model.LabelSet{
			map[model.LabelName]model.LabelValue{"__address__": "localhost:8080", "foo": "bar"},
		},
	}}

	reg, err := relabel.NewRegexp("(.*)")
	require.NoError(t, err)

	scrapeConfig := &config.ScrapeConfig{
		ScrapeInterval: model.Duration(1 * time.Minute),
		ScrapeTimeout:  model.Duration(10 * time.Second),
		RelabelConfigs: []*relabel.Config{{
			SourceLabels: model.LabelNames{"foo"},
			Action:       relabel.Replace,
			TargetLabel:  "newfoo",
			Regex:        reg,
			Replacement:  "$1",
		}},
	}

	expectedSDCheckResult := []sdCheckResult{
		{
			DiscoveredLabels: labels.FromStrings(
				"__address__", "localhost:8080",
				"__scrape_interval__", "1m",
				"__scrape_timeout__", "10s",
				"foo", "bar",
			),
			Labels: labels.FromStrings(
				"__address__", "localhost:8080",
				"__scrape_interval__", "1m",
				"__scrape_timeout__", "10s",
				"foo", "bar",
				"instance", "localhost:8080",
				"newfoo", "bar",
			),
			Error: nil,
		},
	}

	require.Equal(t, expectedSDCheckResult, getSDCheckResult(targetGroups, scrapeConfig, true))
}
