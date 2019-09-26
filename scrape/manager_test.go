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
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"
	"github.com/prometheus/prometheus/util/testutil"
)

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
			err:     errors.New("no address"),
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
				RelabelConfigs: []*relabel.Config{
					{
						Action:       relabel.Replace,
						Regex:        relabel.MustNewRegexp("(.*)"),
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
				RelabelConfigs: []*relabel.Config{
					{
						Action:       relabel.Replace,
						Regex:        relabel.MustNewRegexp("(.*)"),
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
			err:     errors.New("invalid label value for \"custom\": \"\\xbd\""),
		},
	}
	for _, c := range cases {
		in := c.in.Copy()

		res, orig, err := populateLabels(c.in, c.cfg)
		testutil.ErrorEqual(err, c.err)
		testutil.Equals(t, c.in, in)
		testutil.Equals(t, c.res, res)
		testutil.Equals(t, c.resOrig, orig)
	}
}

func loadConfiguration(t *testing.T, c string) *config.Config {
	t.Helper()

	cfg := &config.Config{}
	if err := yaml.UnmarshalStrict([]byte(c), cfg); err != nil {
		t.Fatalf("Unable to load YAML config: %s", err)
	}
	return cfg
}

func noopLoop() loop {
	return &testLoop{
		startFunc: func(interval, timeout time.Duration, errc chan<- error) {},
		stopFunc:  func() {},
	}
}

func TestManagerApplyConfig(t *testing.T) {
	// Valid initial configuration.
	cfgText1 := `
scrape_configs:
 - job_name: job1
   static_configs:
   - targets: ["foo:9090"]
`
	// Invalid configuration.
	cfgText2 := `
scrape_configs:
 - job_name: job1
   scheme: https
   static_configs:
   - targets: ["foo:9090"]
   tls_config:
     ca_file: /not/existing/ca/file
`
	// Valid configuration.
	cfgText3 := `
scrape_configs:
 - job_name: job1
   scheme: https
   static_configs:
   - targets: ["foo:9090"]
`
	var (
		cfg1 = loadConfiguration(t, cfgText1)
		cfg2 = loadConfiguration(t, cfgText2)
		cfg3 = loadConfiguration(t, cfgText3)

		ch = make(chan struct{}, 1)
	)

	scrapeManager := NewManager(nil, nil)
	newLoop := func(scrapeLoopOptions) loop {
		ch <- struct{}{}
		return noopLoop()
	}
	sp := &scrapePool{
		appendable:    &nopAppendable{},
		activeTargets: map[uint64]*Target{},
		loops: map[uint64]loop{
			1: noopLoop(),
		},
		newLoop: newLoop,
		logger:  nil,
		config:  cfg1.ScrapeConfigs[0],
		client:  http.DefaultClient,
	}
	scrapeManager.scrapePools = map[string]*scrapePool{
		"job1": sp,
	}

	// Apply the initial configuration.
	if err := scrapeManager.ApplyConfig(cfg1); err != nil {
		t.Fatalf("unable to apply configuration: %s", err)
	}
	select {
	case <-ch:
		t.Fatal("reload happened")
	default:
	}

	// Apply a configuration for which the reload fails.
	if err := scrapeManager.ApplyConfig(cfg2); err == nil {
		t.Fatalf("expecting error but got none")
	}
	select {
	case <-ch:
		t.Fatal("reload happened")
	default:
	}

	// Apply a configuration for which the reload succeeds.
	if err := scrapeManager.ApplyConfig(cfg3); err != nil {
		t.Fatalf("unable to apply configuration: %s", err)
	}
	select {
	case <-ch:
	default:
		t.Fatal("reload didn't happen")
	}

	// Re-applying the same configuration shouldn't trigger a reload.
	if err := scrapeManager.ApplyConfig(cfg3); err != nil {
		t.Fatalf("unable to apply configuration: %s", err)
	}
	select {
	case <-ch:
		t.Fatal("reload happened")
	default:
	}
}

func TestManagerTargetsUpdates(t *testing.T) {
	m := NewManager(nil, nil)

	ts := make(chan map[string][]*targetgroup.Group)
	go m.Run(ts)
	defer m.Stop()

	tgSent := make(map[string][]*targetgroup.Group)
	for x := 0; x < 10; x++ {

		tgSent[strconv.Itoa(x)] = []*targetgroup.Group{
			{
				Source: strconv.Itoa(x),
			},
		}

		select {
		case ts <- tgSent:
		case <-time.After(10 * time.Millisecond):
			t.Error("Scrape manager's channel remained blocked after the set threshold.")
		}
	}

	m.mtxScrape.Lock()
	tsetActual := m.targetSets
	m.mtxScrape.Unlock()

	// Make sure all updates have been received.
	testutil.Equals(t, tgSent, tsetActual)

	select {
	case <-m.triggerReload:
	default:
		t.Error("No scrape loops reload was triggered after targets update.")
	}
}

func TestSetJitter(t *testing.T) {
	getConfig := func(prometheus string) *config.Config {
		cfgText := `
global:
 external_labels:
   prometheus: '` + prometheus + `'
`

		cfg := &config.Config{}
		if err := yaml.UnmarshalStrict([]byte(cfgText), cfg); err != nil {
			t.Fatalf("Unable to load YAML config cfgYaml: %s", err)
		}

		return cfg
	}

	scrapeManager := NewManager(nil, nil)

	// Load the first config.
	cfg1 := getConfig("ha1")
	if err := scrapeManager.setJitterSeed(cfg1.GlobalConfig.ExternalLabels); err != nil {
		t.Error(err)
	}
	jitter1 := scrapeManager.jitterSeed

	if jitter1 == 0 {
		t.Error("Jitter has to be a hash of uint64")
	}

	// Load the first config.
	cfg2 := getConfig("ha2")
	if err := scrapeManager.setJitterSeed(cfg2.GlobalConfig.ExternalLabels); err != nil {
		t.Error(err)
	}
	jitter2 := scrapeManager.jitterSeed

	if jitter1 == jitter2 {
		t.Error("Jitter should not be the same on different set of external labels")
	}
}
