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
	"hash/fnv"
	"reflect"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/osutil"
)

var targetMetadataCache = newMetadataMetricsCollector()

// MetadataMetricsCollector is a Custom Collector for the metadata cache metrics.
type MetadataMetricsCollector struct {
	CacheEntries *prometheus.Desc
	CacheBytes   *prometheus.Desc

	scrapeManager *Manager
}

func newMetadataMetricsCollector() *MetadataMetricsCollector {
	return &MetadataMetricsCollector{
		CacheEntries: prometheus.NewDesc(
			"prometheus_target_metadata_cache_entries",
			"Total number of metric metadata entries in the cache",
			[]string{"scrape_job"},
			nil,
		),
		CacheBytes: prometheus.NewDesc(
			"prometheus_target_metadata_cache_bytes",
			"The number of bytes that are currently used for storing metric metadata in the cache",
			[]string{"scrape_job"},
			nil,
		),
	}
}

func (mc *MetadataMetricsCollector) registerManager(m *Manager) {
	mc.scrapeManager = m
}

// Describe sends the metrics descriptions to the channel.
func (mc *MetadataMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mc.CacheEntries
	ch <- mc.CacheBytes
}

// Collect creates and sends the metrics for the metadata cache.
func (mc *MetadataMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	if mc.scrapeManager == nil {
		return
	}

	for tset, targets := range mc.scrapeManager.TargetsActive() {
		var size, length int
		for _, t := range targets {
			size += t.MetadataSize()
			length += t.MetadataLength()
		}

		ch <- prometheus.MustNewConstMetric(
			mc.CacheEntries,
			prometheus.GaugeValue,
			float64(length),
			tset,
		)

		ch <- prometheus.MustNewConstMetric(
			mc.CacheBytes,
			prometheus.GaugeValue,
			float64(size),
			tset,
		)
	}
}

// NewManager is the Manager constructor
func NewManager(o *Options, logger log.Logger, app storage.Appendable) *Manager {
	if o == nil {
		o = &Options{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}
	m := &Manager{
		append:        app,
		opts:          o,
		logger:        logger,
		scrapeConfigs: make(map[string]*config.ScrapeConfig),
		scrapePools:   make(map[string]*scrapePool),
		graceShut:     make(chan struct{}),
		triggerReload: make(chan struct{}, 1),
	}
	targetMetadataCache.registerManager(m)

	return m
}

// Options are the configuration parameters to the scrape manager.
type Options struct {
	ExtraMetrics  bool
	NoDefaultPort bool
	// Option used by downstream scraper users like OpenTelemetry Collector
	// to help lookup metric metadata. Should be false for Prometheus.
	PassMetadataInContext bool
	// Option to increase the interval used by scrape manager to throttle target groups updates.
	DiscoveryReloadInterval model.Duration

	// Optional HTTP client options to use when scraping.
	HTTPClientOptions []config_util.HTTPClientOption
}

// Manager maintains a set of scrape pools and manages start/stop cycles
// when receiving new target groups from the discovery manager.
type Manager struct {
	opts      *Options
	logger    log.Logger
	append    storage.Appendable
	graceShut chan struct{}

	jitterSeed    uint64     // Global jitterSeed seed is used to spread scrape workload across HA setup.
	mtxScrape     sync.Mutex // Guards the fields below.
	scrapeConfigs map[string]*config.ScrapeConfig
	scrapePools   map[string]*scrapePool
	targetSets    map[string][]*targetgroup.Group

	triggerReload chan struct{}
}

// Run receives and saves target set updates and triggers the scraping loops reloading.
// Reloading happens in the background so that it doesn't block receiving targets updates.
func (m *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) error {
	go m.reloader()
	for {
		select {
		case ts := <-tsets:
			m.updateTsets(ts)

			select {
			case m.triggerReload <- struct{}{}:
			default:
			}

		case <-m.graceShut:
			return nil
		}
	}
}

func (m *Manager) reloader() {
	reloadIntervalDuration := m.opts.DiscoveryReloadInterval
	if reloadIntervalDuration < model.Duration(5*time.Second) {
		reloadIntervalDuration = model.Duration(5 * time.Second)
	}

	ticker := time.NewTicker(time.Duration(reloadIntervalDuration))

	defer ticker.Stop()

	for {
		select {
		case <-m.graceShut:
			return
		case <-ticker.C:
			select {
			case <-m.triggerReload:
				m.reload()
			case <-m.graceShut:
				return
			}
		}
	}
}

func (m *Manager) reload() {
	m.mtxScrape.Lock()
	var wg sync.WaitGroup
	for setName, groups := range m.targetSets {
		if _, ok := m.scrapePools[setName]; !ok {
			scrapeConfig, ok := m.scrapeConfigs[setName]
			if !ok {
				level.Error(m.logger).Log("msg", "error reloading target set", "err", "invalid config id:"+setName)
				continue
			}
			sp, err := newScrapePool(scrapeConfig, m.append, m.jitterSeed, log.With(m.logger, "scrape_pool", setName), m.opts)
			if err != nil {
				level.Error(m.logger).Log("msg", "error creating new scrape pool", "err", err, "scrape_pool", setName)
				continue
			}
			m.scrapePools[setName] = sp
		}

		wg.Add(1)
		// Run the sync in parallel as these take a while and at high load can't catch up.
		go func(sp *scrapePool, groups []*targetgroup.Group) {
			sp.Sync(groups)
			wg.Done()
		}(m.scrapePools[setName], groups)

	}
	m.mtxScrape.Unlock()
	wg.Wait()
}

// setJitterSeed calculates a global jitterSeed per server relying on extra label set.
func (m *Manager) setJitterSeed(labels labels.Labels) error {
	h := fnv.New64a()
	hostname, err := osutil.GetFQDN()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(h, "%s%s", hostname, labels.String()); err != nil {
		return err
	}
	m.jitterSeed = h.Sum64()
	return nil
}

// Stop cancels all running scrape pools and blocks until all have exited.
func (m *Manager) Stop() {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	for _, sp := range m.scrapePools {
		sp.stop()
	}
	close(m.graceShut)
}

func (m *Manager) updateTsets(tsets map[string][]*targetgroup.Group) {
	m.mtxScrape.Lock()
	m.targetSets = tsets
	m.mtxScrape.Unlock()
}

// ApplyConfig resets the manager's target providers and job configurations as defined by the new cfg.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	c := make(map[string]*config.ScrapeConfig)
	for _, scfg := range cfg.ScrapeConfigs {
		c[scfg.JobName] = scfg
	}
	m.scrapeConfigs = c

	if err := m.setJitterSeed(cfg.GlobalConfig.ExternalLabels); err != nil {
		return err
	}

	// Cleanup and reload pool if the configuration has changed.
	var failed bool
	for name, sp := range m.scrapePools {
		if cfg, ok := m.scrapeConfigs[name]; !ok {
			sp.stop()
			delete(m.scrapePools, name)
		} else if !reflect.DeepEqual(sp.config, cfg) {
			err := sp.reload(cfg)
			if err != nil {
				level.Error(m.logger).Log("msg", "error reloading scrape pool", "err", err, "scrape_pool", name)
				failed = true
			}
		}
	}

	if failed {
		return errors.New("failed to apply the new configuration")
	}
	return nil
}

// TargetsAll returns active and dropped targets grouped by job_name.
func (m *Manager) TargetsAll() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = append(sp.ActiveTargets(), sp.DroppedTargets()...)
	}
	return targets
}

// TargetsActive returns the active targets currently being scraped.
func (m *Manager) TargetsActive() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	var (
		wg  sync.WaitGroup
		mtx sync.Mutex
	)

	targets := make(map[string][]*Target, len(m.scrapePools))
	wg.Add(len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		// Running in parallel limits the blocking time of scrapePool to scrape
		// interval when there's an update from SD.
		go func(tset string, sp *scrapePool) {
			mtx.Lock()
			targets[tset] = sp.ActiveTargets()
			mtx.Unlock()
			wg.Done()
		}(tset, sp)
	}
	wg.Wait()
	return targets
}

// TargetsDropped returns the dropped targets during relabelling.
func (m *Manager) TargetsDropped() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = sp.DroppedTargets()
	}
	return targets
}
