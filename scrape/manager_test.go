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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/util/runutil"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestPopulateLabels(t *testing.T) {
	cases := []struct {
		in            labels.Labels
		cfg           *config.ScrapeConfig
		noDefaultPort bool
		res           labels.Labels
		resOrig       labels.Labels
		err           string
	}{
		// Regular population of scrape config options.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "value",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.InstanceLabel:       "1.2.3.4:1000",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
				"custom":                  "value",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				"custom":                  "value",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
		},
		// Pre-define/overwrite scrape config labels.
		// Leave out port and expect it to be defaulted to scheme.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.SchemeLabel:         "http",
				model.MetricsPathLabel:    "/custom",
				model.JobLabel:            "custom-job",
				model.ScrapeIntervalLabel: "2s",
				model.ScrapeTimeoutLabel:  "2s",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:80",
				model.InstanceLabel:       "1.2.3.4:80",
				model.SchemeLabel:         "http",
				model.MetricsPathLabel:    "/custom",
				model.JobLabel:            "custom-job",
				model.ScrapeIntervalLabel: "2s",
				model.ScrapeTimeoutLabel:  "2s",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.SchemeLabel:         "http",
				model.MetricsPathLabel:    "/custom",
				model.JobLabel:            "custom-job",
				model.ScrapeIntervalLabel: "2s",
				model.ScrapeTimeoutLabel:  "2s",
			}),
		},
		// Provide instance label. HTTPS port default for IPv6.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:  "[::1]",
				model.InstanceLabel: "custom-instance",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "[::1]:443",
				model.InstanceLabel:       "custom-instance",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "[::1]",
				model.InstanceLabel:       "custom-instance",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
		},
		// Address label missing.
		{
			in: labels.FromStrings("custom", "value"),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "no address",
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
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
				model.AddressLabel:        "host:1234",
				model.InstanceLabel:       "host:1234",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
				"custom":                  "host:1234",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
				"custom":                  "host:1234",
			}),
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
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
				model.AddressLabel:        "host:1234",
				model.InstanceLabel:       "host:1234",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
				"custom":                  "host:1234",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
				"custom":                  "host:1234",
			}),
		},
		// Invalid UTF-8 in label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "\xbd",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "invalid label value for \"custom\": \"\\xbd\"",
		},
		// Invalid duration in interval label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "2notseconds",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "error parsing scrape interval: unknown unit \"notseconds\" in duration \"2notseconds\"",
		},
		// Invalid duration in timeout label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:       "1.2.3.4:1000",
				model.ScrapeTimeoutLabel: "2notseconds",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "error parsing scrape timeout: unknown unit \"notseconds\" in duration \"2notseconds\"",
		},
		// 0 interval in timeout label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "0s",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "scrape interval cannot be 0",
		},
		// 0 duration in timeout label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:       "1.2.3.4:1000",
				model.ScrapeTimeoutLabel: "0s",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "scrape timeout cannot be 0",
		},
		// Timeout less than interval.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "2s",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res:     labels.EmptyLabels(),
			resOrig: labels.EmptyLabels(),
			err:     "scrape timeout cannot be greater than scrape interval (\"2s\" > \"1s\")",
		},
		// Don't attach default port.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			noDefaultPort: true,
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.InstanceLabel:       "1.2.3.4",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
		},
		// Remove default port (http).
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:80",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "http",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			noDefaultPort: true,
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.InstanceLabel:       "1.2.3.4:80",
				model.SchemeLabel:         "http",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:80",
				model.SchemeLabel:         "http",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
		},
		// Remove default port (https).
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:443",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			noDefaultPort: true,
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.InstanceLabel:       "1.2.3.4:443",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:443",
				model.SchemeLabel:         "https",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "job",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
		},
	}
	for _, c := range cases {
		in := c.in.Copy()

		res, orig, err := PopulateLabels(labels.NewBuilder(c.in), c.cfg, c.noDefaultPort)
		if c.err != "" {
			require.EqualError(t, err, c.err)
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, c.in, in)
		testutil.RequireEqual(t, c.res, res)
		testutil.RequireEqual(t, c.resOrig, orig)
	}
}

func loadConfiguration(t testing.TB, c string) *config.Config {
	t.Helper()

	cfg := &config.Config{}
	err := yaml.UnmarshalStrict([]byte(c), cfg)
	require.NoError(t, err, "Unable to load YAML config.")

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

		testRegistry = prometheus.NewRegistry()
	)

	opts := Options{}
	scrapeManager, err := NewManager(&opts, nil, nil, testRegistry)
	require.NoError(t, err)
	newLoop := func(scrapeLoopOptions) loop {
		ch <- struct{}{}
		return noopLoop()
	}
	sp := &scrapePool{
		appendable: &nopAppendable{},
		activeTargets: map[uint64]*Target{
			1: {},
		},
		loops: map[uint64]loop{
			1: noopLoop(),
		},
		newLoop:     newLoop,
		logger:      nil,
		config:      cfg1.ScrapeConfigs[0],
		client:      http.DefaultClient,
		metrics:     scrapeManager.metrics,
		symbolTable: labels.NewSymbolTable(),
	}
	scrapeManager.scrapePools = map[string]*scrapePool{
		"job1": sp,
	}

	// Apply the initial configuration.
	err = scrapeManager.ApplyConfig(cfg1)
	require.NoError(t, err, "Unable to apply configuration.")
	select {
	case <-ch:
		require.FailNow(t, "Reload happened.")
	default:
	}

	// Apply a configuration for which the reload fails.
	err = scrapeManager.ApplyConfig(cfg2)
	require.Error(t, err, "Expecting error but got none.")
	select {
	case <-ch:
		require.FailNow(t, "Reload happened.")
	default:
	}

	// Apply a configuration for which the reload succeeds.
	err = scrapeManager.ApplyConfig(cfg3)
	require.NoError(t, err, "Unable to apply configuration.")
	select {
	case <-ch:
	default:
		require.FailNow(t, "Reload didn't happen.")
	}

	// Re-applying the same configuration shouldn't trigger a reload.
	err = scrapeManager.ApplyConfig(cfg3)
	require.NoError(t, err, "Unable to apply configuration.")
	select {
	case <-ch:
		require.FailNow(t, "Reload happened.")
	default:
	}
}

func TestManagerTargetsUpdates(t *testing.T) {
	opts := Options{}
	testRegistry := prometheus.NewRegistry()
	m, err := NewManager(&opts, nil, nil, testRegistry)
	require.NoError(t, err)

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
			require.Fail(t, "Scrape manager's channel remained blocked after the set threshold.")
		}
	}

	m.mtxScrape.Lock()
	tsetActual := m.targetSets
	m.mtxScrape.Unlock()

	// Make sure all updates have been received.
	require.Equal(t, tgSent, tsetActual)

	select {
	case <-m.triggerReload:
	default:
		require.Fail(t, "No scrape loops reload was triggered after targets update.")
	}
}

func TestSetOffsetSeed(t *testing.T) {
	getConfig := func(prometheus string) *config.Config {
		cfgText := `
global:
 external_labels:
   prometheus: '` + prometheus + `'
`

		cfg := &config.Config{}
		err := yaml.UnmarshalStrict([]byte(cfgText), cfg)
		require.NoError(t, err, "Unable to load YAML config cfgYaml.")

		return cfg
	}

	opts := Options{}
	testRegistry := prometheus.NewRegistry()
	scrapeManager, err := NewManager(&opts, nil, nil, testRegistry)
	require.NoError(t, err)

	// Load the first config.
	cfg1 := getConfig("ha1")
	err = scrapeManager.setOffsetSeed(cfg1.GlobalConfig.ExternalLabels)
	require.NoError(t, err)
	offsetSeed1 := scrapeManager.offsetSeed

	require.NotZero(t, offsetSeed1, "Offset seed has to be a hash of uint64.")

	// Load the first config.
	cfg2 := getConfig("ha2")
	require.NoError(t, scrapeManager.setOffsetSeed(cfg2.GlobalConfig.ExternalLabels))
	offsetSeed2 := scrapeManager.offsetSeed

	require.NotEqual(t, offsetSeed1, offsetSeed2, "Offset seed should not be the same on different set of external labels.")
}

func TestManagerScrapePools(t *testing.T) {
	cfgText1 := `
scrape_configs:
- job_name: job1
  static_configs:
  - targets: ["foo:9090"]
- job_name: job2
  static_configs:
  - targets: ["foo:9091", "foo:9092"]
`
	cfgText2 := `
scrape_configs:
- job_name: job1
  static_configs:
  - targets: ["foo:9090", "foo:9094"]
- job_name: job3
  static_configs:
  - targets: ["foo:9093"]
`
	var (
		cfg1         = loadConfiguration(t, cfgText1)
		cfg2         = loadConfiguration(t, cfgText2)
		testRegistry = prometheus.NewRegistry()
	)

	reload := func(scrapeManager *Manager, cfg *config.Config) {
		newLoop := func(scrapeLoopOptions) loop {
			return noopLoop()
		}
		scrapeManager.scrapePools = map[string]*scrapePool{}
		for _, sc := range cfg.ScrapeConfigs {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			sp := &scrapePool{
				appendable:    &nopAppendable{},
				activeTargets: map[uint64]*Target{},
				loops: map[uint64]loop{
					1: noopLoop(),
				},
				newLoop: newLoop,
				logger:  nil,
				config:  sc,
				client:  http.DefaultClient,
				cancel:  cancel,
			}
			for _, c := range sc.ServiceDiscoveryConfigs {
				staticConfig := c.(discovery.StaticConfig)
				for _, group := range staticConfig {
					for i := range group.Targets {
						sp.activeTargets[uint64(i)] = &Target{}
					}
				}
			}
			scrapeManager.scrapePools[sc.JobName] = sp
		}
	}

	opts := Options{}
	scrapeManager, err := NewManager(&opts, nil, nil, testRegistry)
	require.NoError(t, err)

	reload(scrapeManager, cfg1)
	require.ElementsMatch(t, []string{"job1", "job2"}, scrapeManager.ScrapePools())

	reload(scrapeManager, cfg2)
	require.ElementsMatch(t, []string{"job1", "job3"}, scrapeManager.ScrapePools())
}

// TestManagerCTZeroIngestion tests scrape manager for CT cases.
func TestManagerCTZeroIngestion(t *testing.T) {
	const mName = "expected_counter"

	for _, tc := range []struct {
		name                  string
		counterSample         *dto.Counter
		enableCTZeroIngestion bool
	}{
		{
			name: "disabled with CT on counter",
			counterSample: &dto.Counter{
				Value: proto.Float64(1.0),
				// Timestamp does not matter as long as it exists in this test.
				CreatedTimestamp: timestamppb.Now(),
			},
		},
		{
			name: "enabled with CT on counter",
			counterSample: &dto.Counter{
				Value: proto.Float64(1.0),
				// Timestamp does not matter as long as it exists in this test.
				CreatedTimestamp: timestamppb.Now(),
			},
			enableCTZeroIngestion: true,
		},
		{
			name: "enabled without CT on counter",
			counterSample: &dto.Counter{
				Value: proto.Float64(1.0),
			},
			enableCTZeroIngestion: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &collectResultAppender{}
			scrapeManager, err := NewManager(
				&Options{
					EnableCreatedTimestampZeroIngestion: tc.enableCTZeroIngestion,
					skipOffsetting:                      true,
				},
				log.NewLogfmtLogger(os.Stderr),
				&collectResultAppendable{app},
				prometheus.NewRegistry(),
			)
			require.NoError(t, err)

			require.NoError(t, scrapeManager.ApplyConfig(&config.Config{
				GlobalConfig: config.GlobalConfig{
					// Disable regular scrapes.
					ScrapeInterval: model.Duration(9999 * time.Minute),
					ScrapeTimeout:  model.Duration(5 * time.Second),
					// Ensure the proto is chosen. We need proto as it's the only protocol
					// with the CT parsing support.
					ScrapeProtocols: []config.ScrapeProtocol{config.PrometheusProto},
				},
				ScrapeConfigs: []*config.ScrapeConfig{{JobName: "test"}},
			}))

			once := sync.Once{}
			// Start fake HTTP target to that allow one scrape only.
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fail := true
					once.Do(func() {
						fail = false
						w.Header().Set("Content-Type", `application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited`)

						ctrType := dto.MetricType_COUNTER
						w.Write(protoMarshalDelimited(t, &dto.MetricFamily{
							Name:   proto.String(mName),
							Type:   &ctrType,
							Metric: []*dto.Metric{{Counter: tc.counterSample}},
						}))
					})

					if fail {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}),
			)
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			// Add fake target directly into tsets + reload. Normally users would use
			// Manager.Run and wait for minimum 5s refresh interval.
			scrapeManager.updateTsets(map[string][]*targetgroup.Group{
				"test": {{
					Targets: []model.LabelSet{{
						model.SchemeLabel:  model.LabelValue(serverURL.Scheme),
						model.AddressLabel: model.LabelValue(serverURL.Host),
					}},
				}},
			})
			scrapeManager.reload()

			var got []float64
			// Wait for one scrape.
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			require.NoError(t, runutil.Retry(100*time.Millisecond, ctx.Done(), func() error {
				app.mtx.Lock()
				defer app.mtx.Unlock()

				// Check if scrape happened and grab the relevant samples, they have to be there - or it's a bug
				// and it's not worth waiting.
				for _, f := range app.resultFloats {
					if f.metric.Get(model.MetricNameLabel) == mName {
						got = append(got, f.f)
					}
				}
				if len(app.resultFloats) > 0 {
					return nil
				}
				return fmt.Errorf("expected some samples, got none")
			}), "after 1 minute")
			scrapeManager.Stop()

			// Check for zero samples, assuming we only injected always one sample.
			// Did it contain CT to inject? If yes, was CT zero enabled?
			if tc.counterSample.CreatedTimestamp.IsValid() && tc.enableCTZeroIngestion {
				require.Len(t, got, 2)
				require.Equal(t, 0.0, got[0])
				require.Equal(t, tc.counterSample.GetValue(), got[1])
				return
			}

			// Expect only one, valid sample.
			require.Len(t, got, 1)
			require.Equal(t, tc.counterSample.GetValue(), got[0])
		})
	}
}

func TestUnregisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Check that all metrics can be unregistered, allowing a second manager to be created.
	for i := 0; i < 2; i++ {
		opts := Options{}
		manager, err := NewManager(&opts, nil, nil, reg)
		require.NotNil(t, manager)
		require.NoError(t, err)
		// Unregister all metrics.
		manager.UnregisterMetrics()
	}
}

func applyConfig(
	t *testing.T,
	config string,
	scrapeManager *Manager,
	discoveryManager *discovery.Manager,
) {
	t.Helper()

	cfg := loadConfiguration(t, config)
	require.NoError(t, scrapeManager.ApplyConfig(cfg))

	c := make(map[string]discovery.Configs)
	scfgs, err := cfg.GetScrapeConfigs()
	require.NoError(t, err)
	for _, v := range scfgs {
		c[v.JobName] = v.ServiceDiscoveryConfigs
	}
	require.NoError(t, discoveryManager.ApplyConfig(c))
}

func runManagers(t *testing.T, ctx context.Context) (*discovery.Manager, *Manager) {
	t.Helper()

	reg := prometheus.NewRegistry()
	sdMetrics, err := discovery.RegisterSDMetrics(reg, discovery.NewRefreshMetrics(reg))
	require.NoError(t, err)
	discoveryManager := discovery.NewManager(
		ctx,
		log.NewNopLogger(),
		reg,
		sdMetrics,
		discovery.Updatert(100*time.Millisecond),
	)
	scrapeManager, err := NewManager(
		&Options{DiscoveryReloadInterval: model.Duration(100 * time.Millisecond)},
		nil,
		nopAppendable{},
		prometheus.NewRegistry(),
	)
	require.NoError(t, err)
	go discoveryManager.Run()
	go scrapeManager.Run(discoveryManager.SyncCh())
	return discoveryManager, scrapeManager
}

func writeIntoFile(t *testing.T, content, filePattern string) *os.File {
	t.Helper()

	file, err := os.CreateTemp("", filePattern)
	require.NoError(t, err)
	_, err = file.WriteString(content)
	require.NoError(t, err)
	return file
}

func requireTargets(
	t *testing.T,
	scrapeManager *Manager,
	jobName string,
	waitToAppear bool,
	expectedTargets []string,
) {
	t.Helper()

	require.Eventually(t, func() bool {
		targets, ok := scrapeManager.TargetsActive()[jobName]
		if !ok {
			if waitToAppear {
				return false
			}
			t.Fatalf("job %s shouldn't be dropped", jobName)
		}
		if expectedTargets == nil {
			return targets == nil
		}
		if len(targets) != len(expectedTargets) {
			return false
		}
		sTargets := []string{}
		for _, t := range targets {
			sTargets = append(sTargets, t.String())
		}
		sort.Strings(expectedTargets)
		sort.Strings(sTargets)
		for i, t := range sTargets {
			if t != expectedTargets[i] {
				return false
			}
		}
		return true
	}, 1*time.Second, 100*time.Millisecond)
}

// TestTargetDisappearsAfterProviderRemoved makes sure that when a provider is dropped, (only) its targets are dropped.
func TestTargetDisappearsAfterProviderRemoved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	myJob := "my-job"
	myJobSDTargetURL := "my:9876"
	myJobStaticTargetURL := "my:5432"

	sdFileContent := fmt.Sprintf(`[{"targets": ["%s"]}]`, myJobSDTargetURL)
	sDFile := writeIntoFile(t, sdFileContent, "*targets.json")

	baseConfig := `
scrape_configs:
- job_name: %s
  static_configs:
  - targets: ['%s']
  file_sd_configs:
  - files: ['%s']
`

	discoveryManager, scrapeManager := runManagers(t, ctx)
	defer scrapeManager.Stop()

	applyConfig(
		t,
		fmt.Sprintf(
			baseConfig,
			myJob,
			myJobStaticTargetURL,
			sDFile.Name(),
		),
		scrapeManager,
		discoveryManager,
	)
	// Make sure the jobs targets are taken into account
	requireTargets(
		t,
		scrapeManager,
		myJob,
		true,
		[]string{
			fmt.Sprintf("http://%s/metrics", myJobSDTargetURL),
			fmt.Sprintf("http://%s/metrics", myJobStaticTargetURL),
		},
	)

	// Apply a new config where a provider is removed
	baseConfig = `
scrape_configs:
- job_name: %s
  static_configs:
  - targets: ['%s']
`
	applyConfig(
		t,
		fmt.Sprintf(
			baseConfig,
			myJob,
			myJobStaticTargetURL,
		),
		scrapeManager,
		discoveryManager,
	)
	// Make sure the corresponding target was dropped
	requireTargets(
		t,
		scrapeManager,
		myJob,
		false,
		[]string{
			fmt.Sprintf("http://%s/metrics", myJobStaticTargetURL),
		},
	)

	// Apply a new config with no providers
	baseConfig = `
scrape_configs:
- job_name: %s
`
	applyConfig(
		t,
		fmt.Sprintf(
			baseConfig,
			myJob,
		),
		scrapeManager,
		discoveryManager,
	)
	// Make sure the corresponding target was dropped
	requireTargets(
		t,
		scrapeManager,
		myJob,
		false,
		nil,
	)
}

// TestOnlyProviderStaleTargetsAreDropped makes sure that when a job has only one provider with multiple targets
// and when the provider can no longer discover some of those targets, only those stale targets are dropped.
func TestOnlyProviderStaleTargetsAreDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobName := "my-job"
	jobTarget1URL := "foo:9876"
	jobTarget2URL := "foo:5432"

	sdFile1Content := fmt.Sprintf(`[{"targets": ["%s"]}]`, jobTarget1URL)
	sdFile2Content := fmt.Sprintf(`[{"targets": ["%s"]}]`, jobTarget2URL)
	sDFile1 := writeIntoFile(t, sdFile1Content, "*targets.json")
	sDFile2 := writeIntoFile(t, sdFile2Content, "*targets.json")

	baseConfig := `
scrape_configs:
- job_name: %s
  file_sd_configs:
  - files: ['%s', '%s']
`
	discoveryManager, scrapeManager := runManagers(t, ctx)
	defer scrapeManager.Stop()

	applyConfig(
		t,
		fmt.Sprintf(baseConfig, jobName, sDFile1.Name(), sDFile2.Name()),
		scrapeManager,
		discoveryManager,
	)

	// Make sure the job's targets are taken into account
	requireTargets(
		t,
		scrapeManager,
		jobName,
		true,
		[]string{
			fmt.Sprintf("http://%s/metrics", jobTarget1URL),
			fmt.Sprintf("http://%s/metrics", jobTarget2URL),
		},
	)

	// Apply the same config for the same job but with a non existing file to make the provider
	// unable to discover some targets
	applyConfig(
		t,
		fmt.Sprintf(baseConfig, jobName, sDFile1.Name(), "/idontexistdoi.json"),
		scrapeManager,
		discoveryManager,
	)

	// The old target should get dropped
	requireTargets(
		t,
		scrapeManager,
		jobName,
		false,
		[]string{fmt.Sprintf("http://%s/metrics", jobTarget1URL)},
	)
}

// TestProviderStaleTargetsAreDropped makes sure that when a job has only one provider and when that provider
// should no longer discover targets, the targets of that provider are dropped.
// See: https://github.com/prometheus/prometheus/issues/12858
func TestProviderStaleTargetsAreDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobName := "my-job"
	jobTargetURL := "foo:9876"

	sdFileContent := fmt.Sprintf(`[{"targets": ["%s"]}]`, jobTargetURL)
	sDFile := writeIntoFile(t, sdFileContent, "*targets.json")

	baseConfig := `
scrape_configs:
- job_name: %s
  file_sd_configs:
  - files: ['%s']
`
	discoveryManager, scrapeManager := runManagers(t, ctx)
	defer scrapeManager.Stop()

	applyConfig(
		t,
		fmt.Sprintf(baseConfig, jobName, sDFile.Name()),
		scrapeManager,
		discoveryManager,
	)

	// Make sure the job's targets are taken into account
	requireTargets(
		t,
		scrapeManager,
		jobName,
		true,
		[]string{
			fmt.Sprintf("http://%s/metrics", jobTargetURL),
		},
	)

	// Apply the same config for the same job but with a non existing file to make the provider
	// unable to discover some targets
	applyConfig(
		t,
		fmt.Sprintf(baseConfig, jobName, "/idontexistdoi.json"),
		scrapeManager,
		discoveryManager,
	)

	// The old target should get dropped
	requireTargets(
		t,
		scrapeManager,
		jobName,
		false,
		nil,
	)
}

// TestOnlyStaleTargetsAreDropped makes sure that when a job has multiple providers, when aone of them should no,
// longer discover targets, only the stale targets of that provier are dropped.
func TestOnlyStaleTargetsAreDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	myJob := "my-job"
	myJobSDTargetURL := "my:9876"
	myJobStaticTargetURL := "my:5432"
	otherJob := "other-job"
	otherJobTargetURL := "other:1234"

	sdFileContent := fmt.Sprintf(`[{"targets": ["%s"]}]`, myJobSDTargetURL)
	sDFile := writeIntoFile(t, sdFileContent, "*targets.json")

	baseConfig := `
scrape_configs:
- job_name: %s
  static_configs:
  - targets: ['%s']
  file_sd_configs:
  - files: ['%s']
- job_name: %s
  static_configs:
  - targets: ['%s']
`

	discoveryManager, scrapeManager := runManagers(t, ctx)
	defer scrapeManager.Stop()

	// Apply the initial config with an existing file
	applyConfig(
		t,
		fmt.Sprintf(
			baseConfig,
			myJob,
			myJobStaticTargetURL,
			sDFile.Name(),
			otherJob,
			otherJobTargetURL,
		),
		scrapeManager,
		discoveryManager,
	)
	// Make sure the jobs targets are taken into account
	requireTargets(
		t,
		scrapeManager,
		myJob,
		true,
		[]string{
			fmt.Sprintf("http://%s/metrics", myJobSDTargetURL),
			fmt.Sprintf("http://%s/metrics", myJobStaticTargetURL),
		},
	)
	requireTargets(
		t,
		scrapeManager,
		otherJob,
		true,
		[]string{fmt.Sprintf("http://%s/metrics", otherJobTargetURL)},
	)

	// Apply the same config with a non existing file for myJob
	applyConfig(
		t,
		fmt.Sprintf(
			baseConfig,
			myJob,
			myJobStaticTargetURL,
			"/idontexistdoi.json",
			otherJob,
			otherJobTargetURL,
		),
		scrapeManager,
		discoveryManager,
	)

	// Only the SD target should get dropped for myJob
	requireTargets(
		t,
		scrapeManager,
		myJob,
		false,
		[]string{
			fmt.Sprintf("http://%s/metrics", myJobStaticTargetURL),
		},
	)
	// The otherJob should keep its target
	requireTargets(
		t,
		scrapeManager,
		otherJob,
		false,
		[]string{fmt.Sprintf("http://%s/metrics", otherJobTargetURL)},
	)
}
