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

package config

import (
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
			SourceLabels:         model.LabelNames{"foo"},
			Action:               relabel.Replace,
			TargetLabel:          "newfoo",
			Regex:                reg,
			Replacement:          "$1",
			NameValidationScheme: model.UTF8Validation,
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

	testutil.RequireEqual(t, expectedSDCheckResult, getSDCheckResult(targetGroups, scrapeConfig))
}

func TestCheckSD(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		file     string
		jobName  string
		exitCode int
	}{
		{
			name:     "check SD with good job name",
			file:     "config_with_service_discovery_files.yml",
			jobName:  "prometheus",
			exitCode: SuccessExitCode,
		},
		{
			name:     "check SD with bad job name",
			file:     "config_with_service_discovery_files.yml",
			jobName:  "bad_job_name",
			exitCode: FailureExitCode,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exitCode := CheckSD("../testdata/"+test.file, test.jobName, 5*time.Second, nil)
			require.Equal(t, test.exitCode, exitCode)
		})
	}
}

func TestCheckSDWithOutput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		file      string
		jobName   string
		exitCode  int
		logOutput bool
	}{
		{
			name:     "check SD with good job name",
			file:     "config_with_service_discovery_files.yml",
			jobName:  "prometheus",
			exitCode: SuccessExitCode,
		},
		{
			name:      "check SD with bad job name",
			file:      "config_with_service_discovery_files.yml",
			jobName:   "bad_job_name",
			exitCode:  FailureExitCode,
			logOutput: true,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exitCode, output := CheckSDWithOutput("../testdata/"+test.file, test.jobName, 5*time.Second, nil)
			if test.logOutput {
				t.Logf("%s output:\n%s", test.name, output)
			}
			require.Equal(t, test.exitCode, exitCode)
		})
	}
}
