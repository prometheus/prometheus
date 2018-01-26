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

package retrieval

import (
	"fmt"
	"reflect"
	"sync"
	"time"

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

// NewScrapeManager is the ScrapeManager constructor
func NewScrapeManager(logger log.Logger, app Appendable) *ScrapeManager {

	return &ScrapeManager{
		append:        app,
		logger:        logger,
		scrapeConfigs: make(map[string]*config.ScrapeConfig),
		scrapePools:   make(map[string]*scrapePool),
		graceShut:     make(chan struct{}),
	}
}

// ScrapeManager maintains a set of scrape pools and manages start/stop cycles
// when receiving new target groups form the discovery manager.
type ScrapeManager struct {
	logger        log.Logger
	append        Appendable
	scrapeConfigs map[string]*config.ScrapeConfig
	scrapePools   map[string]*scrapePool
	mtx           sync.RWMutex
	graceShut     chan struct{}
}

// Run starts background processing to handle target updates and reload the scraping loops.
func (m *ScrapeManager) Run(tsets <-chan map[string][]*targetgroup.Group) error {
	level.Info(m.logger).Log("msg", "Starting scrape manager...")

	for {
		// Throttle syncing to once per five seconds.
		select {
		case <-m.graceShut:
			return nil
		case <-time.After(5 * time.Second):
		}

		select {
		case ts := <-tsets:
			m.reload(ts)
		case <-m.graceShut:
			return nil
		}
	}
}

// Stop cancels all running scrape pools and blocks until all have exited.
func (m *ScrapeManager) Stop() {
	for _, sp := range m.scrapePools {
		sp.stop()
	}
	close(m.graceShut)
}

// ApplyConfig resets the manager's target providers and job configurations as defined by the new cfg.
func (m *ScrapeManager) ApplyConfig(cfg *config.Config) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
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

// TargetMap returns map of active and dropped targets and their corresponding scrape config job name.
func (m *ScrapeManager) TargetMap() map[string][]*Target {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	targets := make(map[string][]*Target)
	for jobName, sp := range m.scrapePools {
		sp.mtx.RLock()
		for _, t := range sp.targets {
			targets[jobName] = append(targets[jobName], t)
		}
		targets[jobName] = append(targets[jobName], sp.droppedTargets...)
		sp.mtx.RUnlock()
	}

	return targets
}

// Targets returns the targets currently being scraped.
func (m *ScrapeManager) Targets() []*Target {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var targets []*Target
	for _, p := range m.scrapePools {
		p.mtx.RLock()
		for _, tt := range p.targets {
			targets = append(targets, tt)
		}
		p.mtx.RUnlock()
	}

	return targets
}

func (m *ScrapeManager) reload(t map[string][]*targetgroup.Group) {
	for tsetName, tgroup := range t {
		scrapeConfig, ok := m.scrapeConfigs[tsetName]
		if !ok {
			level.Error(m.logger).Log("msg", "error reloading target set", "err", fmt.Sprintf("invalid config id:%v", tsetName))
			continue
		}

		// Scrape pool doesn't exist so start a new one.
		existing, ok := m.scrapePools[tsetName]
		if !ok {
			sp := newScrapePool(scrapeConfig, m.append, log.With(m.logger, "scrape_pool", tsetName))
			m.scrapePools[tsetName] = sp
			sp.Sync(tgroup)

		} else {
			existing.Sync(tgroup)
		}
	}
}
