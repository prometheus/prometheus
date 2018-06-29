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
	"reflect"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/storage"
)

// Appendable returns an Appender.
type Appendable interface {
	Appender() (storage.Appender, error)
}

// NewManager is the Manager constructor
func NewManager(logger log.Logger, app Appendable) *Manager {
	return &Manager{
		append:        app,
		logger:        logger,
		scrapeConfigs: make(map[string]*config.ScrapeConfig),
		scrapePools:   make(map[string]*scrapePool),
		graceShut:     make(chan struct{}),
		targetsAll:    make(map[string][]*Target),
	}
}

// Manager maintains a set of scrape pools and manages start/stop cycles
// when receiving new target groups form the discovery manager.
type Manager struct {
	logger    log.Logger
	append    Appendable
	graceShut chan struct{}

	mtxTargets     sync.Mutex // Guards the fields below.
	targetsActive  []*Target
	targetsDropped []*Target
	targetsAll     map[string][]*Target

	mtxScrape     sync.Mutex // Guards the fields below.
	scrapeConfigs map[string]*config.ScrapeConfig
	scrapePools   map[string]*scrapePool
}

// Run starts background processing to handle target updates and reload the scraping loops.
func (m *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) error {
	for {
		select {
		case ts := <-tsets:
			m.reload(ts)
		case <-m.graceShut:
			return nil
		}
	}
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

// ApplyConfig resets the manager's target providers and job configurations as defined by the new cfg.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	c := make(map[string]*config.ScrapeConfig)
	for _, scfg := range cfg.ScrapeConfigs {
		c[scfg.JobName] = scfg
	}
	m.scrapeConfigs = c

	// Cleanup and reload pool if config has changed.
	for name, sp := range m.scrapePools {
		if cfg, ok := m.scrapeConfigs[name]; !ok {
			sp.stop()
			delete(m.scrapePools, name)
		} else if !reflect.DeepEqual(sp.config, cfg) {
			sp.reload(cfg)
		}
	}

	return nil
}

// TargetsAll returns active and dropped targets grouped by job_name.
func (m *Manager) TargetsAll() map[string][]*Target {
	m.mtxTargets.Lock()
	defer m.mtxTargets.Unlock()
	return m.targetsAll
}

// TargetsActive returns the active targets currently being scraped.
func (m *Manager) TargetsActive() []*Target {
	m.mtxTargets.Lock()
	defer m.mtxTargets.Unlock()
	return m.targetsActive
}

// TargetsDropped returns the dropped targets during relabelling.
func (m *Manager) TargetsDropped() []*Target {
	m.mtxTargets.Lock()
	defer m.mtxTargets.Unlock()
	return m.targetsDropped
}

func (m *Manager) targetsUpdate(active, dropped map[string][]*Target) {
	m.mtxTargets.Lock()
	defer m.mtxTargets.Unlock()

	m.targetsAll = make(map[string][]*Target)
	m.targetsActive = nil
	m.targetsDropped = nil
	for jobName, targets := range active {
		m.targetsAll[jobName] = append(m.targetsAll[jobName], targets...)
		m.targetsActive = append(m.targetsActive, targets...)

	}
	for jobName, targets := range dropped {
		m.targetsAll[jobName] = append(m.targetsAll[jobName], targets...)
		m.targetsDropped = append(m.targetsDropped, targets...)
	}
}

func (m *Manager) reload(t map[string][]*targetgroup.Group) {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	tDropped := make(map[string][]*Target)
	tActive := make(map[string][]*Target)

	for tsetName, tgroup := range t {
		var sp *scrapePool
		if existing, ok := m.scrapePools[tsetName]; !ok {
			scrapeConfig, ok := m.scrapeConfigs[tsetName]
			if !ok {
				level.Error(m.logger).Log("msg", "error reloading target set", "err", fmt.Sprintf("invalid config id:%v", tsetName))
				continue
			}
			sp = newScrapePool(scrapeConfig, m.append, log.With(m.logger, "scrape_pool", tsetName))
			m.scrapePools[tsetName] = sp
		} else {
			sp = existing
		}
		tActive[tsetName], tDropped[tsetName] = sp.Sync(tgroup)
	}
	m.targetsUpdate(tActive, tDropped)
}
