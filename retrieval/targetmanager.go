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
	"sync"

	"github.com/prometheus/common/log"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/storage"
)

// TargetManager maintains a set of targets, starts and stops their scraping and
// creates the new targets based on the target groups it receives from various
// target providers.
type TargetManager struct {
	append Appendable
	logger log.Logger

	wg  sync.WaitGroup
	ctx context.Context

	mtx           sync.RWMutex
	scrapeConfigs []*config.ScrapeConfig
	pools         map[string]poolEntry
}

type poolEntry struct {
	cancel    func()
	pool      *scrapePool
	discovery *discovery.SwapRunner
}

// Appendable opens new storage append transactions.
type Appendable interface {
	Appender() (storage.Appender, error)
}

// NewTargetManager creates a new TargetManager.
func NewTargetManager(ctx context.Context, app Appendable, logger log.Logger) *TargetManager {
	return &TargetManager{
		append: app,
		pools:  map[string]poolEntry{},
		ctx:    ctx,
		logger: logger,
	}
}

// Run starts background processing to handle target updates.
func (tm *TargetManager) Run(ctx context.Context) {
	tm.logger.Info("Starting target manager...")

	tm.reload()

	<-ctx.Done()

	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	tm.logger.Infoln("Stopping target manager...")
	// Wait for all target sets to gracefully abort the scrapes and finish
	// in-progress writes.
	for _, p := range tm.pools {
		p.pool.wait()
	}
	tm.logger.Infoln("Target manager stopped.")
}

// ApplyConfig resets the manager's target providers and job configurations as defined
// by the new cfg. The state of targets that are valid in the new configuration remains unchanged.
func (tm *TargetManager) ApplyConfig(cfg *config.Config) error {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	tm.scrapeConfigs = cfg.ScrapeConfigs

	return nil
}

func (tm *TargetManager) reload() {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	jobs := map[string]struct{}{}

	// Start new target sets and update existing ones.
	for _, scfg := range tm.scrapeConfigs {
		jobs[scfg.JobName] = struct{}{}

		sp, ok := tm.pools[scfg.JobName]
		if ok {
			sp.updateConfig(scfg)
		} else {
			ctx, cancel := context.WithCancel(tm.ctx)
			sp = newScrapePool(ctx, scfg, tm.append, tm.logger)

			tm.pools[scfg.JobName] = poolEntry{
				pool:      sp,
				discovery: discovery.NewSwapRunner(ctx),
				cancel:    cancel,
			}
		}

		// The new discovery target set triggers a sync against the scrape pool.
		// Thus, the targets are ensured to be re-evaluated against the new configuration.
		providers := discovery.ProvidersFromConfig(scfg.ServiceDiscoveryConfig, tm.logger)
		sp.discovery.Swap(discovery.NewTargetSet(sp.pool, providers))
	}

	// Remove old target sets. Waiting for scrape pools to complete pending
	// scrape inserts is already guaranteed by the goroutine that started the target set.
	for name, p := range tm.pools {
		if _, ok := jobs[name]; !ok {
			p.cancel()
			delete(tm.pools, name)
		}
	}
}

// Targets returns the targets currently being scraped.
func (tm *TargetManager) Targets() (targets []*Target) {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	for _, sp := range tm.pools {
		targets = append(targets, sp.getTargets()...)
	}
	return targets
}
