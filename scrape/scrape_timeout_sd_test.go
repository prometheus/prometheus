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

package scrape

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	_ "github.com/prometheus/prometheus/discovery/http"
	"github.com/prometheus/prometheus/model/labels"
)

func mustScrapeConfig(t *testing.T, c, job string) *config.ScrapeConfig {
	t.Helper()
	cfg := loadConfiguration(t, c)
	for _, sc := range cfg.ScrapeConfigs {
		if sc.JobName == job {
			return sc
		}
	}
	t.Fatalf("scrape config %q not found", job)
	return nil
}

// Case B from issue #19730: service discovery provides a 5m per-target
// interval while the job pins only its own 40s interval. The inherited
// global timeout (1m) must not be capped to the job-level interval at
// config load time, because the final per-target interval is 5m.
func TestScrapeTimeoutResolvedPerTarget(t *testing.T) {
	cfg := mustScrapeConfig(t, `
global:
  scrape_interval: 1m
  scrape_timeout: 1m
scrape_configs:
  - job_name: example
    scrape_interval: 40s
    http_sd_configs:
      - url: http://sd.example/targets
`, "example")
	require.Equal(t, model.Duration(time.Minute), cfg.ScrapeTimeout, "the inherited global timeout must not be capped to the job interval when the interval can be overridden per target")
	lb := labels.NewBuilder(labels.EmptyLabels())
	res, err := PopulateLabels(lb, cfg, model.LabelSet{
		model.AddressLabel:        "target.example:9100",
		model.ScrapeIntervalLabel: "5m", // Provided per target by service discovery.
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "5m", res.Get(model.ScrapeIntervalLabel))
	require.Equal(t, "1m", res.Get(model.ScrapeTimeoutLabel))
}

// Case A from issue #19730: an explicit job timeout of 4m is valid when
// service discovery overrides the interval to 5m per target, so the
// configuration must load and the target must use the reported pair.
func TestScrapeTimeoutWithSDIntervalLoads(t *testing.T) {
	cfg := mustScrapeConfig(t, `
global:
  scrape_interval: 1m
scrape_configs:
  - job_name: example
    scrape_timeout: 4m
    http_sd_configs:
      - url: http://sd.example/targets
`, "example")
	lb := labels.NewBuilder(labels.EmptyLabels())
	res, err := PopulateLabels(lb, cfg, model.LabelSet{
		model.AddressLabel:        "target.example:9100",
		model.ScrapeIntervalLabel: "5m",
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "5m", res.Get(model.ScrapeIntervalLabel))
	require.Equal(t, "4m", res.Get(model.ScrapeTimeoutLabel))
}

// Jobs that cannot override the interval per target keep the load-time
// pair check.
func TestScrapeTimeoutPinnedIntervalStillRejected(t *testing.T) {
	_, err := config.Load(`
scrape_configs:
  - job_name: example
    scrape_interval: 5s
    scrape_timeout: 6s
`, promslog.NewNopLogger())
	require.EqualError(t, err, `scrape timeout greater than scrape interval for scrape config with job name "example"`)
}

// An explicitly relabeled timeout stays subject to the per-target pair
// check instead of being silently capped.
func TestExplicitRelabeledTimeoutStillRejected(t *testing.T) {
	cfg := mustScrapeConfig(t, `
global:
  scrape_interval: 1m
scrape_configs:
  - job_name: example
    relabel_configs:
      - target_label: __scrape_timeout__
        replacement: 4m
`, "example")
	lb := labels.NewBuilder(labels.EmptyLabels())
	_, err := PopulateLabels(lb, cfg, model.LabelSet{
		model.AddressLabel: "target.example:9100",
	}, nil)
	require.EqualError(t, err, `scrape timeout cannot be greater than scrape interval ("4m" > "1m")`)
}
