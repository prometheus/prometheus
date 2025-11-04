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
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/runutil"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestPopulateLabels(t *testing.T) {
	cases := []struct {
		in      model.LabelSet
		cfg     *config.ScrapeConfig
		res     labels.Labels
		resOrig labels.Labels
		err     string
	}{
		// Regular population of scrape config options.
		{
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "value",
			},
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
			in: model.LabelSet{
				model.AddressLabel:        "1.2.3.4",
				model.SchemeLabel:         "http",
				model.MetricsPathLabel:    "/custom",
				model.JobLabel:            "custom-job",
				model.ScrapeIntervalLabel: "2s",
				model.ScrapeTimeoutLabel:  "2s",
			},
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4",
				model.InstanceLabel:       "1.2.3.4",
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
			in: model.LabelSet{
				model.AddressLabel:  "[::1]",
				model.InstanceLabel: "custom-instance",
			},
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "[::1]",
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
			in: model.LabelSet{"custom": "value"},
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
			in: model.LabelSet{"custom": "host:1234"},
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
				RelabelConfigs: []*relabel.Config{
					{
						Action:               relabel.Replace,
						Regex:                relabel.MustNewRegexp("(.*)"),
						SourceLabels:         model.LabelNames{"custom"},
						Replacement:          "${1}",
						TargetLabel:          string(model.AddressLabel),
						NameValidationScheme: model.UTF8Validation,
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
			in: model.LabelSet{"custom": "host:1234"},
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
				RelabelConfigs: []*relabel.Config{
					{
						Action:               relabel.Replace,
						Regex:                relabel.MustNewRegexp("(.*)"),
						SourceLabels:         model.LabelNames{"custom"},
						Replacement:          "${1}",
						TargetLabel:          string(model.AddressLabel),
						NameValidationScheme: model.UTF8Validation,
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
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "\xbd",
			},
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
			in: model.LabelSet{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "2notseconds",
			},
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
			in: model.LabelSet{
				model.AddressLabel:       "1.2.3.4:1000",
				model.ScrapeTimeoutLabel: "2notseconds",
			},
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
			in: model.LabelSet{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "0s",
			},
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
			in: model.LabelSet{
				model.AddressLabel:       "1.2.3.4:1000",
				model.ScrapeTimeoutLabel: "0s",
			},
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
			in: model.LabelSet{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "2s",
			},
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
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4",
			},
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
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
		// verify that the default port is not removed (http).
		{
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4:80",
			},
			cfg: &config.ScrapeConfig{
				Scheme:         "http",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:80",
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
		// verify that the default port is not removed (https).
		{
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4:443",
			},
			cfg: &config.ScrapeConfig{
				Scheme:         "https",
				MetricsPath:    "/metrics",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:443",
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
		in := maps.Clone(c.in)
		lb := labels.NewBuilder(labels.EmptyLabels())
		c.cfg.MetricNameValidationScheme = model.UTF8Validation
		for i := range c.cfg.RelabelConfigs {
			c.cfg.RelabelConfigs[i].NameValidationScheme = model.UTF8Validation
		}
		res, err := PopulateLabels(lb, c.cfg, c.in, nil)
		if c.err != "" {
			require.EqualError(t, err, c.err)
		} else {
			require.NoError(t, err)
			testutil.RequireEqual(t, c.res, res)
			PopulateDiscoveredLabels(lb, c.cfg, c.in, nil)
			testutil.RequireEqual(t, c.resOrig, lb.Labels())
		}
		require.Equal(t, c.in, in) // Check this wasn't altered by PopulateLabels().
	}
}

func loadConfiguration(t testing.TB, c string) *config.Config {
	t.Helper()

	cfg, err := config.Load(c, promslog.NewNopLogger())
	require.NoError(t, err)
	return cfg
}

func noopLoop() loop {
	return &testLoop{
		startFunc: func(_, _ time.Duration, _ chan<- error) {},
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
	scrapeManager, err := NewManager(&opts, nil, nil, nil, testRegistry)
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
	m, err := NewManager(&opts, nil, nil, nil, testRegistry)
	require.NoError(t, err)

	ts := make(chan map[string][]*targetgroup.Group)
	go m.Run(ts)
	defer m.Stop()

	tgSent := make(map[string][]*targetgroup.Group)
	for x := range 10 {
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
	scrapeManager, err := NewManager(&opts, nil, nil, nil, testRegistry)
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
	scrapeManager, err := NewManager(&opts, nil, nil, nil, testRegistry)
	require.NoError(t, err)

	reload(scrapeManager, cfg1)
	require.ElementsMatch(t, []string{"job1", "job2"}, scrapeManager.ScrapePools())

	reload(scrapeManager, cfg2)
	require.ElementsMatch(t, []string{"job1", "job3"}, scrapeManager.ScrapePools())
}

func setupTestServer(t *testing.T, typ string, toWrite []byte) *httptest.Server {
	once := sync.Once{}

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fail := true
			once.Do(func() {
				fail = false
				w.Header().Set("Content-Type", typ)
				w.Write(toWrite)
			})

			if fail {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}),
	)

	t.Cleanup(func() { server.Close() })

	return server
}

// TestManagerCTZeroIngestion tests scrape manager for various CT cases.
func TestManagerCTZeroIngestion(t *testing.T) {
	t.Parallel()
	const (
		// _total suffix is required, otherwise expfmt with OMText will mark metric as "unknown"
		expectedMetricName        = "expected_metric_total"
		expectedCreatedMetricName = "expected_metric_created"
		expectedSampleValue       = 17.0
	)

	for _, testFormat := range []config.ScrapeProtocol{config.PrometheusProto, config.OpenMetricsText1_0_0} {
		t.Run(fmt.Sprintf("format=%s", testFormat), func(t *testing.T) {
			for _, testWithCT := range []bool{false, true} {
				t.Run(fmt.Sprintf("withCT=%v", testWithCT), func(t *testing.T) {
					for _, testCTZeroIngest := range []bool{false, true} {
						t.Run(fmt.Sprintf("ctZeroIngest=%v", testCTZeroIngest), func(t *testing.T) {
							ctx, cancel := context.WithCancel(context.Background())
							defer cancel()

							sampleTs := time.Now()
							ctTs := time.Time{}
							if testWithCT {
								ctTs = sampleTs.Add(-2 * time.Minute)
							}

							// TODO(bwplotka): Add more types than just counter?
							encoded := prepareTestEncodedCounter(t, testFormat, expectedMetricName, expectedSampleValue, sampleTs, ctTs)

							app := &collectResultAppender{}
							discoveryManager, scrapeManager := runManagers(t, ctx, &Options{
								EnableCreatedTimestampZeroIngestion: testCTZeroIngest,
								skipOffsetting:                      true,
							}, &collectResultAppendable{app})
							defer scrapeManager.Stop()

							server := setupTestServer(t, config.ScrapeProtocolsHeaders[testFormat], encoded)
							serverURL, err := url.Parse(server.URL)
							require.NoError(t, err)

							testConfig := fmt.Sprintf(`
global:
  # Disable regular scrapes.
  scrape_interval: 9999m
  scrape_timeout: 5s

scrape_configs:
- job_name: test
  honor_timestamps: true
  static_configs:
  - targets: ['%s']
`, serverURL.Host)
							applyConfig(t, testConfig, scrapeManager, discoveryManager)

							// Wait for one scrape.
							ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
							defer cancel()
							require.NoError(t, runutil.Retry(100*time.Millisecond, ctx.Done(), func() error {
								app.mtx.Lock()
								defer app.mtx.Unlock()

								// Check if scrape happened and grab the relevant samples.
								if len(app.resultFloats) > 0 {
									return nil
								}
								return errors.New("expected some float samples, got none")
							}), "after 1 minute")

							// Verify results.
							// Verify what we got vs expectations around CT injection.
							samples := findSamplesForMetric(app.resultFloats, expectedMetricName)
							if testWithCT && testCTZeroIngest {
								require.Len(t, samples, 2)
								require.Equal(t, 0.0, samples[0].f)
								require.Equal(t, timestamp.FromTime(ctTs), samples[0].t)
								require.Equal(t, expectedSampleValue, samples[1].f)
								require.Equal(t, timestamp.FromTime(sampleTs), samples[1].t)
							} else {
								require.Len(t, samples, 1)
								require.Equal(t, expectedSampleValue, samples[0].f)
								require.Equal(t, timestamp.FromTime(sampleTs), samples[0].t)
							}

							// Verify what we got vs expectations around additional _created series for OM text.
							// enableCTZeroInjection also kills that _created line.
							createdSeriesSamples := findSamplesForMetric(app.resultFloats, expectedCreatedMetricName)
							if testFormat == config.OpenMetricsText1_0_0 && testWithCT && !testCTZeroIngest {
								// For OM Text, when counter has CT, and feature flag disabled we should see _created lines.
								require.Len(t, createdSeriesSamples, 1)
								// Conversion taken from common/expfmt.writeOpenMetricsFloat.
								// We don't check the ct timestamp as explicit ts was not implemented in expfmt.Encoder,
								// but exists in OM https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#:~:text=An%20example%20with%20a%20Metric%20with%20no%20labels%2C%20and%20a%20MetricPoint%20with%20a%20timestamp%20and%20a%20created
								// We can implement this, but we want to potentially get rid of OM 1.0 CT lines
								require.Equal(t, float64(timestamppb.New(ctTs).AsTime().UnixNano())/1e9, createdSeriesSamples[0].f)
							} else {
								require.Empty(t, createdSeriesSamples)
							}
						})
					}
				})
			}
		})
	}
}

func prepareTestEncodedCounter(t *testing.T, format config.ScrapeProtocol, mName string, v float64, ts, ct time.Time) (encoded []byte) {
	t.Helper()

	counter := &dto.Counter{Value: proto.Float64(v)}
	if !ct.IsZero() {
		counter.CreatedTimestamp = timestamppb.New(ct)
	}
	ctrType := dto.MetricType_COUNTER
	inputMetric := &dto.MetricFamily{
		Name: proto.String(mName),
		Type: &ctrType,
		Metric: []*dto.Metric{{
			TimestampMs: proto.Int64(timestamp.FromTime(ts)),
			Counter:     counter,
		}},
	}
	switch format {
	case config.PrometheusProto:
		return protoMarshalDelimited(t, inputMetric)
	case config.OpenMetricsText1_0_0:
		buf := &bytes.Buffer{}
		require.NoError(t, expfmt.NewEncoder(buf, expfmt.NewFormat(expfmt.TypeOpenMetrics), expfmt.WithCreatedLines(), expfmt.WithUnit()).Encode(inputMetric))
		_, _ = buf.WriteString("# EOF")

		t.Log("produced OM text to expose:", buf.String())
		return buf.Bytes()
	default:
		t.Fatalf("not implemented format: %v", format)
		return nil
	}
}

func findSamplesForMetric(floats []floatSample, metricName string) (ret []floatSample) {
	for _, f := range floats {
		if f.metric.Get(model.MetricNameLabel) == metricName {
			ret = append(ret, f)
		}
	}
	return ret
}

// generateTestHistogram generates the same thing as tsdbutil.GenerateTestHistogram,
// but in the form of dto.Histogram.
func generateTestHistogram(i int) *dto.Histogram {
	helper := tsdbutil.GenerateTestHistogram(int64(i))
	h := &dto.Histogram{}
	h.SampleCount = proto.Uint64(helper.Count)
	h.SampleSum = proto.Float64(helper.Sum)
	h.Schema = proto.Int32(helper.Schema)
	h.ZeroThreshold = proto.Float64(helper.ZeroThreshold)
	h.ZeroCount = proto.Uint64(helper.ZeroCount)
	h.PositiveSpan = make([]*dto.BucketSpan, len(helper.PositiveSpans))
	for i, span := range helper.PositiveSpans {
		h.PositiveSpan[i] = &dto.BucketSpan{
			Offset: proto.Int32(span.Offset),
			Length: proto.Uint32(span.Length),
		}
	}
	h.PositiveDelta = helper.PositiveBuckets
	h.NegativeSpan = make([]*dto.BucketSpan, len(helper.NegativeSpans))
	for i, span := range helper.NegativeSpans {
		h.NegativeSpan[i] = &dto.BucketSpan{
			Offset: proto.Int32(span.Offset),
			Length: proto.Uint32(span.Length),
		}
	}
	h.NegativeDelta = helper.NegativeBuckets
	return h
}

func TestManagerCTZeroIngestionHistogram(t *testing.T) {
	t.Parallel()
	const mName = "expected_histogram"

	for _, tc := range []struct {
		name                  string
		inputHistSample       *dto.Histogram
		enableCTZeroIngestion bool
	}{
		{
			name: "disabled with CT on histogram",
			inputHistSample: func() *dto.Histogram {
				h := generateTestHistogram(0)
				h.CreatedTimestamp = timestamppb.Now()
				return h
			}(),
			enableCTZeroIngestion: false,
		},
		{
			name: "enabled with CT on histogram",
			inputHistSample: func() *dto.Histogram {
				h := generateTestHistogram(0)
				h.CreatedTimestamp = timestamppb.Now()
				return h
			}(),
			enableCTZeroIngestion: true,
		},
		{
			name: "enabled without CT on histogram",
			inputHistSample: func() *dto.Histogram {
				h := generateTestHistogram(0)
				return h
			}(),
			enableCTZeroIngestion: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			app := &collectResultAppender{}
			discoveryManager, scrapeManager := runManagers(t, ctx, &Options{
				EnableCreatedTimestampZeroIngestion: tc.enableCTZeroIngestion,
				skipOffsetting:                      true,
			}, &collectResultAppendable{app})
			defer scrapeManager.Stop()

			once := sync.Once{}
			// Start fake HTTP target to that allow one scrape only.
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fail := true
					once.Do(func() {
						fail = false
						w.Header().Set("Content-Type", `application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited`)

						ctrType := dto.MetricType_HISTOGRAM
						w.Write(protoMarshalDelimited(t, &dto.MetricFamily{
							Name:   proto.String(mName),
							Type:   &ctrType,
							Metric: []*dto.Metric{{Histogram: tc.inputHistSample}},
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

			testConfig := fmt.Sprintf(`
global:
  # Disable regular scrapes.
  scrape_interval: 9999m
  scrape_timeout: 5s

scrape_configs:
- job_name: test
  scrape_native_histograms: true
  static_configs:
  - targets: ['%s']
`, serverURL.Host)
			applyConfig(t, testConfig, scrapeManager, discoveryManager)

			var got []histogramSample

			// Wait for one scrape.
			ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()
			require.NoError(t, runutil.Retry(100*time.Millisecond, ctx.Done(), func() error {
				app.mtx.Lock()
				defer app.mtx.Unlock()

				// Check if scrape happened and grab the relevant histograms, they have to be there - or it's a bug
				// and it's not worth waiting.
				for _, h := range app.resultHistograms {
					if h.metric.Get(model.MetricNameLabel) == mName {
						got = append(got, h)
					}
				}
				if len(app.resultHistograms) > 0 {
					return nil
				}
				return errors.New("expected some histogram samples, got none")
			}), "after 1 minute")

			// Check for zero samples, assuming we only injected always one histogram sample.
			// Did it contain CT to inject? If yes, was CT zero enabled?
			if tc.inputHistSample.CreatedTimestamp.IsValid() && tc.enableCTZeroIngestion {
				require.Len(t, got, 2)
				// Zero sample.
				require.Equal(t, histogram.Histogram{}, *got[0].h)
				// Quick soft check to make sure it's the same sample or at least not zero.
				require.Equal(t, tc.inputHistSample.GetSampleSum(), got[1].h.Sum)
				return
			}

			// Expect only one, valid sample.
			require.Len(t, got, 1)
			// Quick soft check to make sure it's the same sample or at least not zero.
			require.Equal(t, tc.inputHistSample.GetSampleSum(), got[0].h.Sum)
		})
	}
}

func TestUnregisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Check that all metrics can be unregistered, allowing a second manager to be created.
	for range 2 {
		opts := Options{}
		manager, err := NewManager(&opts, nil, nil, nil, reg)
		require.NotNil(t, manager)
		require.NoError(t, err)
		// Unregister all metrics.
		manager.UnregisterMetrics()
	}
}

// TestNHCBAndCTZeroIngestion verifies that both ConvertClassicHistogramsToNHCBEnabled
// and EnableCreatedTimestampZeroIngestion can be used simultaneously without errors.
// This test addresses issue #17216 by ensuring the previously blocking check has been removed.
// The test verifies that the presence of exemplars in the input does not cause errors,
// although exemplars are not preserved during NHCB conversion (as documented below).
func TestNHCBAndCTZeroIngestion(t *testing.T) {
	t.Parallel()

	const (
		mName = "test_histogram"
		// The expected sum of the histogram, as defined by the test's OpenMetrics exposition data.
		// This value (45.5) is the sum reported in the test_histogram_sum metric below.
		expectedHistogramSum = 45.5
	)

	ctx := t.Context()

	app := &collectResultAppender{}
	discoveryManager, scrapeManager := runManagers(t, ctx, &Options{
		EnableCreatedTimestampZeroIngestion: true,
		skipOffsetting:                      true,
	}, &collectResultAppendable{app})
	defer scrapeManager.Stop()

	once := sync.Once{}
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fail := true
			once.Do(func() {
				fail = false
				w.Header().Set("Content-Type", `application/openmetrics-text`)

				// Expose a histogram with created timestamp and exemplars to verify no parsing errors occur.
				fmt.Fprint(w, `# HELP test_histogram A histogram with created timestamp and exemplars
# TYPE test_histogram histogram
test_histogram_bucket{le="0.0"} 1
test_histogram_bucket{le="1.0"} 10 # {trace_id="trace-1"} 0.5 123456789
test_histogram_bucket{le="2.0"} 20 # {trace_id="trace-2"} 1.5 123456780
test_histogram_bucket{le="+Inf"} 30 # {trace_id="trace-3"} 2.5
test_histogram_count 30
test_histogram_sum 45.5
test_histogram_created 1520430001
# EOF
`)
			})

			if fail {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Configuration with both convert_classic_histograms_to_nhcb enabled and CT zero ingestion enabled.
	testConfig := fmt.Sprintf(`
global:
  # Use a very long scrape_interval to prevent automatic scraping during the test.
  scrape_interval: 9999m
  scrape_timeout: 5s

scrape_configs:
- job_name: test
  convert_classic_histograms_to_nhcb: true
  static_configs:
  - targets: ['%s']
`, serverURL.Host)

	applyConfig(t, testConfig, scrapeManager, discoveryManager)

	// Verify that the scrape pool was created (proves the blocking check was removed).
	require.Eventually(t, func() bool {
		scrapeManager.mtxScrape.Lock()
		defer scrapeManager.mtxScrape.Unlock()
		_, exists := scrapeManager.scrapePools["test"]
		return exists
	}, 5*time.Second, 100*time.Millisecond, "scrape pool should be created for job 'test'")

	// Helper function to get matching histograms to avoid race conditions.
	getMatchingHistograms := func() []histogramSample {
		app.mtx.Lock()
		defer app.mtx.Unlock()

		var got []histogramSample
		for _, h := range app.resultHistograms {
			if h.metric.Get(model.MetricNameLabel) == mName {
				got = append(got, h)
			}
		}
		return got
	}

	require.Eventually(t, func() bool {
		return len(getMatchingHistograms()) > 0
	}, 1*time.Minute, 100*time.Millisecond, "expected histogram samples, got none")

	// Verify that samples were ingested (proving both features work together).
	got := getMatchingHistograms()

	// With CT zero ingestion enabled and a created timestamp present, we expect 2 samples:
	// one zero sample and one actual sample.
	require.Len(t, got, 2, "expected 2 histogram samples (zero sample + actual sample)")
	require.Equal(t, histogram.Histogram{}, *got[0].h, "first sample should be zero sample")
	require.InDelta(t, expectedHistogramSum, got[1].h.Sum, 1e-9, "second sample should retain the expected sum")
	require.Len(t, app.resultExemplars, 2, "expected 2 exemplars from histogram buckets")
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

func runManagers(t *testing.T, ctx context.Context, opts *Options, app storage.Appendable) (*discovery.Manager, *Manager) {
	t.Helper()

	if opts == nil {
		opts = &Options{}
	}
	opts.DiscoveryReloadInterval = model.Duration(100 * time.Millisecond)
	if app == nil {
		app = nopAppendable{}
	}

	reg := prometheus.NewRegistry()
	sdMetrics, err := discovery.RegisterSDMetrics(reg, discovery.NewRefreshMetrics(reg))
	require.NoError(t, err)
	discoveryManager := discovery.NewManager(
		ctx,
		promslog.NewNopLogger(),
		reg,
		sdMetrics,
		discovery.Updatert(100*time.Millisecond),
	)
	scrapeManager, err := NewManager(
		opts,
		nil,
		nil,
		app,
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
		return slices.Equal(sTargets, expectedTargets)
	}, 1*time.Second, 100*time.Millisecond)
}

// TestTargetDisappearsAfterProviderRemoved makes sure that when a provider is dropped, (only) its targets are dropped.
func TestTargetDisappearsAfterProviderRemoved(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

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

	discoveryManager, scrapeManager := runManagers(t, ctx, nil, nil)
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
	t.Parallel()
	ctx := t.Context()

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
	discoveryManager, scrapeManager := runManagers(t, ctx, nil, nil)
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
	ctx := t.Context()

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
	discoveryManager, scrapeManager := runManagers(t, ctx, nil, nil)
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

// TestOnlyStaleTargetsAreDropped makes sure that when a job has multiple providers, when one of them should no
// longer discover targets, only the stale targets of that provider are dropped.
func TestOnlyStaleTargetsAreDropped(t *testing.T) {
	ctx := t.Context()

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

	discoveryManager, scrapeManager := runManagers(t, ctx, nil, nil)
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

func TestManagerDisableEndOfRunStalenessMarkers(t *testing.T) {
	cfgText := `
scrape_configs:
 - job_name: one
   scrape_interval: 1m
   scrape_timeout: 1m
 - job_name: two
   scrape_interval: 1m
   scrape_timeout: 1m
`

	cfg := loadConfiguration(t, cfgText)

	m, err := NewManager(&Options{}, nil, nil, &nopAppendable{}, prometheus.NewRegistry())
	require.NoError(t, err)
	defer m.Stop()
	require.NoError(t, m.ApplyConfig(cfg))

	// Pass targets to the manager.
	tgs := map[string][]*targetgroup.Group{
		"one": {{Targets: []model.LabelSet{{"__address__": "h1"}, {"__address__": "h2"}, {"__address__": "h3"}}}},
		"two": {{Targets: []model.LabelSet{{"__address__": "h4"}}}},
	}
	m.updateTsets(tgs)
	m.reload()

	activeTargets := m.TargetsActive()
	targetsToDisable := []*Target{
		activeTargets["one"][0],
		activeTargets["one"][2],
	}

	// Disable end of run staleness markers for some targets.
	m.DisableEndOfRunStalenessMarkers("one", targetsToDisable)
	// This should be a no-op
	m.DisableEndOfRunStalenessMarkers("non-existent-job", targetsToDisable)

	// Check that the end of run staleness markers are disabled for the correct targets.
	for _, group := range []string{"one", "two"} {
		for _, tg := range activeTargets[group] {
			loop := m.scrapePools[group].loops[tg.hash()].(*scrapeLoop)
			expectedDisabled := slices.Contains(targetsToDisable, tg)
			require.Equal(t, expectedDisabled, loop.disabledEndOfRunStalenessMarkers.Load())
		}
	}
}
