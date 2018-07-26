// Copyright 2013 The Prometheus Authors
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
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"

	yaml "gopkg.in/yaml.v2"
)

func mustNewRegexp(s string) config.Regexp {
	re, err := config.NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

func TestPopulateLabels(t *testing.T) {
	cases := []struct {
		in      labels.Labels
		cfg     *config.ScrapeConfig
		res     labels.Labels
		resOrig labels.Labels
		err     error
	}{
		// Regular population of scrape config options.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "value",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4:1000",
				model.InstanceLabel:    "1.2.3.4:1000",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "value",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4:1000",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "value",
			}),
		},
		// Pre-define/overwrite scrape config labels.
		// Leave out port and expect it to be defaulted to scheme.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4:80",
				model.InstanceLabel:    "1.2.3.4:80",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			}),
		},
		// Provide instance label. HTTPS port default for IPv6.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:  "[::1]",
				model.InstanceLabel: "custom-instance",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "[::1]:443",
				model.InstanceLabel:    "custom-instance",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:     "[::1]",
				model.InstanceLabel:    "custom-instance",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
			}),
		},
		// Address label missing.
		{
			in: labels.FromStrings("custom", "value"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res:     nil,
			resOrig: nil,
			err:     fmt.Errorf("no address"),
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
				RelabelConfigs: []*config.RelabelConfig{
					{
						Action:       config.RelabelReplace,
						Regex:        mustNewRegexp("(.*)"),
						SourceLabels: model.LabelNames{"custom"},
						Replacement:  "${1}",
						TargetLabel:  string(model.AddressLabel),
					},
				},
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "host:1234",
				model.InstanceLabel:    "host:1234",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
				RelabelConfigs: []*config.RelabelConfig{
					{
						Action:       config.RelabelReplace,
						Regex:        mustNewRegexp("(.*)"),
						SourceLabels: model.LabelNames{"custom"},
						Replacement:  "${1}",
						TargetLabel:  string(model.AddressLabel),
					},
				},
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "host:1234",
				model.InstanceLabel:    "host:1234",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
		},
		// Invalid UTF-8 in label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "\xbd",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res:     nil,
			resOrig: nil,
			err:     fmt.Errorf("invalid label value for \"custom\": \"\\xbd\""),
		},
	}
	for _, c := range cases {
		in := c.in.Copy()

		res, orig, err := populateLabels(c.in, c.cfg)
		testutil.Equals(t, c.err, err)
		testutil.Equals(t, c.in, in)
		testutil.Equals(t, c.res, res)
		testutil.Equals(t, c.resOrig, orig)
	}
}

// TestScrapeManagerReloadNoChange tests that no scrape reload happens when there is no config change.
func TestManagerReloadNoChange(t *testing.T) {
	tsetName := "test"

	cfgText := `
scrape_configs:
 - job_name: '` + tsetName + `'
   static_configs:
   - targets: ["foo:9090"]
   - targets: ["bar:9090"]
`
	cfg := &config.Config{}
	if err := yaml.UnmarshalStrict([]byte(cfgText), cfg); err != nil {
		t.Fatalf("Unable to load YAML config cfgYaml: %s", err)
	}

	scrapeManager := NewManager(nil, nil)
	// Load the current config.
	scrapeManager.ApplyConfig(cfg)

	// As reload never happens, new loop should never be called.
	newLoop := func(_ *Target, s scraper, _ int, _ bool, _ []*config.RelabelConfig) loop {
		t.Fatal("reload happened")
		return nil
	}

	sp := &scrapePool{
		appendable: &nopAppendable{},
		targets:    map[uint64]*Target{},
		loops: map[uint64]loop{
			1: &testLoop{},
		},
		newLoop: newLoop,
		logger:  nil,
		config:  cfg.ScrapeConfigs[0],
	}
	scrapeManager.scrapePools = map[string]*scrapePool{
		tsetName: sp,
	}

	scrapeManager.ApplyConfig(cfg)
}
