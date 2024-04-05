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
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/osutil"
	"github.com/prometheus/prometheus/util/pool"
)

// NewManager is the Manager constructor.
func NewManager(o *Options, logger log.Logger, app storage.Appendable, registerer prometheus.Registerer) (*Manager, error) {
	if o == nil {
		o = &Options{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	sm, err := newScrapeMetrics(registerer)
	if err != nil {
		return nil, fmt.Errorf("failed to create scrape manager due to error: %w", err)
	}

	m := &Manager{
		append:        app,
		opts:          o,
		logger:        logger,
		scrapeConfigs: make(map[string]*config.ScrapeConfig),
		scrapePools:   make(map[string]*scrapePool),
		graceShut:     make(chan struct{}),
		triggerReload: make(chan struct{}, 1),
		metrics:       sm,
		buffers:       pool.New(1e3, 100e6, 3, func(sz int) interface{} { return make([]byte, 0, sz) }),
	}

	m.metrics.setTargetMetadataCacheGatherer(m)

	return m, nil
}

// Options are the configuration parameters to the scrape manager.
type Options struct {
	ExtraMetrics  bool
	NoDefaultPort bool
	// Option used by downstream scraper users like OpenTelemetry Collector
	// to help lookup metric metadata. Should be false for Prometheus.
	PassMetadataInContext bool
	// Option to enable the experimental in-memory metadata storage and append
	// metadata to the WAL.
	EnableMetadataStorage bool
	// Option to increase the interval used by scrape manager to throttle target groups updates.
	DiscoveryReloadInterval model.Duration
	// Option to enable the ingestion of the created timestamp as a synthetic zero sample.
	// See: https://github.com/prometheus/proposals/blob/main/proposals/2023-06-13_created-timestamp.md
	EnableCreatedTimestampZeroIngestion bool

	// Optional HTTP client options to use when scraping.
	HTTPClientOptions []config_util.HTTPClientOption

	// private option for testability.
	skipOffsetting bool
}

// Manager maintains a set of scrape pools and manages start/stop cycles
// when receiving new target groups from the discovery manager.
type Manager struct {
	opts      *Options
	logger    log.Logger
	append    storage.Appendable
	graceShut chan struct{}

	offsetSeed    uint64     // Global offsetSeed seed is used to spread scrape workload across HA setup.
	mtxScrape     sync.Mutex // Guards the fields below.
	scrapeConfigs map[string]*config.ScrapeConfig
	scrapePools   map[string]*scrapePool
	targetSets    map[string][]*targetgroup.Group
	buffers       *pool.Pool

	triggerReload chan struct{}

	metrics *scrapeMetrics
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

// UnregisterMetrics unregisters manager metrics.
func (m *Manager) UnregisterMetrics() {
	m.metrics.Unregister()
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
			m.metrics.targetScrapePools.Inc()
			sp, err := newScrapePool(scrapeConfig, m.append, m.offsetSeed, log.With(m.logger, "scrape_pool", setName), m.buffers, m.opts, m.metrics)
			if err != nil {
				m.metrics.targetScrapePoolsFailed.Inc()
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

// setOffsetSeed calculates a global offsetSeed per server relying on extra label set.
func (m *Manager) setOffsetSeed(labels labels.Labels) error {
	h := fnv.New64a()
	hostname, err := osutil.GetFQDN()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(h, "%s%s", hostname, labels.String()); err != nil {
		return err
	}
	m.offsetSeed = h.Sum64()
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

	scfgs, err := cfg.GetScrapeConfigs()
	if err != nil {
		return err
	}

	c := make(map[string]*config.ScrapeConfig)
	for _, scfg := range scfgs {
		c[scfg.JobName] = scfg
	}
	m.scrapeConfigs = c

	if err := m.setOffsetSeed(cfg.GlobalConfig.ExternalLabels); err != nil {
		return err
	}

	// Cleanup and reload pool if the configuration has changed.
	var failed bool
	for name, sp := range m.scrapePools {
		switch cfg, ok := m.scrapeConfigs[name]; {
		case !ok:
			sp.stop()
			delete(m.scrapePools, name)
		case !reflect.DeepEqual(sp.config, cfg):
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

// ScrapePools returns the list of all scrape pool names.
func (m *Manager) ScrapePools() []string {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	names := make([]string, 0, len(m.scrapePools))
	for name := range m.scrapePools {
		names = append(names, name)
	}
	return names
}

// TargetsActive returns the active targets currently being scraped.
func (m *Manager) TargetsActive() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = sp.ActiveTargets()
	}
	return targets
}

// TargetsDropped returns the dropped targets during relabelling, subject to KeepDroppedTargets limit.
func (m *Manager) TargetsDropped() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = sp.DroppedTargets()
	}
	return targets
}

func (m *Manager) TargetsDroppedCounts() map[string]int {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	counts := make(map[string]int, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		counts[tset] = sp.droppedTargetsCount
	}
	return counts
}
