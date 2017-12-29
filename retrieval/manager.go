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
		actionCh:      make(chan func()),
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
	actionCh      chan func()
	graceShut     chan struct{}
}

// Run starts background processing to handle target updates and reload the scraping loops.
func (m *ScrapeManager) Run(tsets <-chan map[string][]*targetgroup.Group) error {
	level.Info(m.logger).Log("msg", "Starting scrape manager...")

	for {
		select {
		case f := <-m.actionCh:
			f()
		case ts := <-tsets:
			if err := m.reload(ts); err != nil {
				level.Error(m.logger).Log("msg", "error reloading the scrape manager", "err", err)
			}
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
	done := make(chan struct{})
	m.actionCh <- func() {
		for _, scfg := range cfg.ScrapeConfigs {
			m.scrapeConfigs[scfg.JobName] = scfg
		}
		close(done)
	}
	<-done
	return nil
}

// TargetMap returns map of active and dropped targets and their corresponding scrape config job name.
func (m *ScrapeManager) TargetMap() map[string][]*Target {
	targetsMap := make(chan map[string][]*Target)
	m.actionCh <- func() {
		targets := make(map[string][]*Target)
		for jobName, sp := range m.scrapePools {
			sp.mtx.RLock()
			for _, t := range sp.targets {
				targets[jobName] = append(targets[jobName], t)
			}
			targets[jobName] = append(targets[jobName], sp.droppedTargets...)
			sp.mtx.RUnlock()
		}
		targetsMap <- targets
	}
	return <-targetsMap
}

// Targets returns the targets currently being scraped.
func (m *ScrapeManager) Targets() []*Target {
	targets := make(chan []*Target)
	m.actionCh <- func() {
		var t []*Target
		for _, p := range m.scrapePools {
			p.mtx.RLock()
			for _, tt := range p.targets {
				t = append(t, tt)
			}
			p.mtx.RUnlock()
		}
		targets <- t
	}
	return <-targets
}

func (m *ScrapeManager) reload(t map[string][]*targetgroup.Group) error {
	for tsetName, tgroup := range t {
		scrapeConfig, ok := m.scrapeConfigs[tsetName]
		if !ok {
			return fmt.Errorf("target set '%v' doesn't have valid config", tsetName)
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

		// Cleanup - check the config and cancel the scrape loops if it don't exist in the scrape config.
		jobs := make(map[string]struct{})

		for k := range m.scrapeConfigs {
			jobs[k] = struct{}{}
		}

		for name, sp := range m.scrapePools {
			if _, ok := jobs[name]; !ok {
				sp.stop()
				delete(m.scrapePools, name)
			}
		}
	}
	return nil
}
