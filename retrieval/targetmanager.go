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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/relabel"
	"github.com/prometheus/prometheus/retrieval/discovery"
	"github.com/prometheus/prometheus/storage"
)

// A TargetProvider provides information about target groups. It maintains a set
// of sources from which TargetGroups can originate. Whenever a target provider
// detects a potential change, it sends the TargetGroup through its provided channel.
//
// The TargetProvider does not have to guarantee that an actual change happened.
// It does guarantee that it sends the new TargetGroup whenever a change happens.
//
// Providers must initially send all known target groups as soon as it can.
type TargetProvider interface {
	// Run hands a channel to the target provider through which it can send
	// updated target groups. The channel must be closed by the target provider
	// if no more updates will be sent.
	// On receiving from done Run must return.
	Run(ctx context.Context, up chan<- []*config.TargetGroup)
}

// TargetManager maintains a set of targets, starts and stops their scraping and
// creates the new targets based on the target groups it receives from various
// target providers.
type TargetManager struct {
	appender      storage.SampleAppender
	scrapeConfigs []*config.ScrapeConfig

	mtx    sync.RWMutex
	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup

	// Set of unqiue targets by scrape configuration.
	targetSets map[string]*targetSet
}

// NewTargetManager creates a new TargetManager.
func NewTargetManager(app storage.SampleAppender) *TargetManager {
	return &TargetManager{
		appender:   app,
		targetSets: map[string]*targetSet{},
	}
}

// Run starts background processing to handle target updates.
func (tm *TargetManager) Run() {
	log.Info("Starting target manager...")

	tm.mtx.Lock()

	tm.ctx, tm.cancel = context.WithCancel(context.Background())
	tm.reload()

	tm.mtx.Unlock()

	tm.wg.Wait()
}

// Stop all background processing.
func (tm *TargetManager) Stop() {
	log.Infoln("Stopping target manager...")

	tm.mtx.Lock()
	// Cancel the base context, this will cause all target providers to shut down
	// and all in-flight scrapes to abort immmediately.
	// Started inserts will be finished before terminating.
	tm.cancel()
	tm.mtx.Unlock()

	// Wait for all scrape inserts to complete.
	tm.wg.Wait()

	log.Debugln("Target manager stopped")
}

func (tm *TargetManager) reload() {
	jobs := map[string]struct{}{}

	// Start new target sets and update existing ones.
	for _, scfg := range tm.scrapeConfigs {
		jobs[scfg.JobName] = struct{}{}

		ts, ok := tm.targetSets[scfg.JobName]
		if !ok {
			ts = newTargetSet(scfg, tm.appender)
			tm.targetSets[scfg.JobName] = ts

			tm.wg.Add(1)

			go func(ts *targetSet) {
				ts.runScraping(tm.ctx)
				tm.wg.Done()
			}(ts)
		} else {
			ts.reload(scfg)
		}
		ts.runProviders(tm.ctx, providersFromConfig(scfg))
	}

	// Remove old target sets. Waiting for stopping is already guaranteed
	// by the goroutine that started the target set.
	for name, ts := range tm.targetSets {
		if _, ok := jobs[name]; !ok {
			ts.cancel()
			delete(tm.targetSets, name)
		}
	}
}

// Pools returns the targets currently being scraped bucketed by their job name.
func (tm *TargetManager) Pools() map[string]Targets {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	pools := map[string]Targets{}

	// TODO(fabxc): this is just a hack to maintain compatibility for now.
	for _, ps := range tm.targetSets {
		ps.scrapePool.mtx.RLock()

		for _, t := range ps.scrapePool.targets {
			job := string(t.Labels()[model.JobLabel])
			pools[job] = append(pools[job], t)
		}

		ps.scrapePool.mtx.RUnlock()
	}
	for _, targets := range pools {
		sort.Sort(targets)
	}
	return pools
}

// ApplyConfig resets the manager's target providers and job configurations as defined
// by the new cfg. The state of targets that are valid in the new configuration remains unchanged.
func (tm *TargetManager) ApplyConfig(cfg *config.Config) error {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	tm.scrapeConfigs = cfg.ScrapeConfigs

	if tm.ctx != nil {
		tm.reload()
	}
	return nil
}

// targetSet holds several TargetProviders for which the same scrape configuration
// is used. It maintains target groups from all given providers and sync them
// to a scrape pool.
type targetSet struct {
	mtx sync.RWMutex

	// Sets of targets by a source string that is unique across target providers.
	tgroups   map[string][]*Target
	providers map[string]TargetProvider

	scrapePool *scrapePool
	config     *config.ScrapeConfig

	syncCh          chan struct{}
	cancelScraping  func()
	cancelProviders func()
}

func newTargetSet(cfg *config.ScrapeConfig, app storage.SampleAppender) *targetSet {
	ts := &targetSet{
		tgroups:    map[string][]*Target{},
		scrapePool: newScrapePool(cfg, app),
		syncCh:     make(chan struct{}, 1),
		config:     cfg,
	}
	return ts
}

func (ts *targetSet) cancel() {
	ts.mtx.RLock()
	defer ts.mtx.RUnlock()

	if ts.cancelScraping != nil {
		ts.cancelScraping()
	}
	if ts.cancelProviders != nil {
		ts.cancelProviders()
	}
}

func (ts *targetSet) reload(cfg *config.ScrapeConfig) {
	ts.mtx.Lock()
	ts.config = cfg
	ts.mtx.Unlock()

	ts.scrapePool.reload(cfg)
}

func (ts *targetSet) runScraping(ctx context.Context) {
	ctx, ts.cancelScraping = context.WithCancel(ctx)

	ts.scrapePool.init(ctx)

Loop:
	for {
		// Throttle syncing to once per five seconds.
		select {
		case <-ctx.Done():
			break Loop
		case <-time.After(5 * time.Second):
		}

		select {
		case <-ctx.Done():
			break Loop
		case <-ts.syncCh:
			ts.mtx.RLock()
			ts.sync()
			ts.mtx.RUnlock()
		}
	}

	// We want to wait for all pending target scrapes to complete though to ensure there'll
	// be no more storage writes after this point.
	ts.scrapePool.stop()
}

func (ts *targetSet) sync() {
	var all []*Target
	for _, targets := range ts.tgroups {
		all = append(all, targets...)
	}
	ts.scrapePool.sync(all)
}

func (ts *targetSet) runProviders(ctx context.Context, providers map[string]TargetProvider) {
	// Lock for the entire time. This may mean up to 5 seconds until the full initial set
	// is retrieved and applied.
	// We could release earlier with some tweaks, but this is easier to reason about.
	ts.mtx.Lock()
	defer ts.mtx.Unlock()

	var wg sync.WaitGroup

	if ts.cancelProviders != nil {
		ts.cancelProviders()
	}
	ctx, ts.cancelProviders = context.WithCancel(ctx)

	for name, prov := range providers {
		wg.Add(1)

		updates := make(chan []*config.TargetGroup)

		go func(name string, prov TargetProvider) {
			select {
			case <-ctx.Done():
			case initial, ok := <-updates:
				// Handle the case that a target provider exits and closes the channel
				// before the context is done.
				if !ok {
					break
				}
				// First set of all targets the provider knows.
				for _, tgroup := range initial {
					if tgroup == nil {
						continue
					}
					targets, err := targetsFromGroup(tgroup, ts.config)
					if err != nil {
						log.With("target_group", tgroup).Errorf("Target update failed: %s", err)
						continue
					}
					ts.tgroups[name+"/"+tgroup.Source] = targets
				}
			case <-time.After(5 * time.Second):
				// Initial set didn't arrive. Act as if it was empty
				// and wait for updates later on.
			}

			wg.Done()

			// Start listening for further updates.
			for {
				select {
				case <-ctx.Done():
					return
				case tgs, ok := <-updates:
					// Handle the case that a target provider exits and closes the channel
					// before the context is done.
					if !ok {
						return
					}
					for _, tg := range tgs {
						if err := ts.update(name, tg); err != nil {
							log.With("target_group", tg).Errorf("Target update failed: %s", err)
						}
					}
				}
			}
		}(name, prov)

		go prov.Run(ctx, updates)
	}

	// We wait for a full initial set of target groups before releasing the mutex
	// to ensure the initial sync is complete and there are no races with subsequent updates.
	wg.Wait()
	// Just signal that there are initial sets to sync now. Actual syncing must only
	// happen in the runScraping loop.
	select {
	case ts.syncCh <- struct{}{}:
	default:
	}
}

// update handles a target group update from a target provider identified by the name.
func (ts *targetSet) update(name string, tgroup *config.TargetGroup) error {
	if tgroup == nil {
		return nil
	}
	targets, err := targetsFromGroup(tgroup, ts.config)
	if err != nil {
		return err
	}

	ts.mtx.Lock()
	defer ts.mtx.Unlock()

	ts.tgroups[name+"/"+tgroup.Source] = targets

	select {
	case ts.syncCh <- struct{}{}:
	default:
	}

	return nil
}

// providersFromConfig returns all TargetProviders configured in cfg.
func providersFromConfig(cfg *config.ScrapeConfig) map[string]TargetProvider {
	providers := map[string]TargetProvider{}

	app := func(mech string, i int, tp TargetProvider) {
		providers[fmt.Sprintf("%s/%d", mech, i)] = tp
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, discovery.NewDNS(c))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, discovery.NewFileDiscovery(c))
	}
	for i, c := range cfg.ConsulSDConfigs {
		k, err := discovery.NewConsul(c)
		if err != nil {
			log.Errorf("Cannot create Consul discovery: %s", err)
			continue
		}
		app("consul", i, k)
	}
	for i, c := range cfg.MarathonSDConfigs {
		app("marathon", i, discovery.NewMarathon(c))
	}
	for i, c := range cfg.KubernetesSDConfigs {
		k, err := discovery.NewKubernetesDiscovery(c)
		if err != nil {
			log.Errorf("Cannot create Kubernetes discovery: %s", err)
			continue
		}
		app("kubernetes", i, k)
	}
	for i, c := range cfg.ServersetSDConfigs {
		app("serverset", i, discovery.NewServersetDiscovery(c))
	}
	for i, c := range cfg.NerveSDConfigs {
		app("nerve", i, discovery.NewNerveDiscovery(c))
	}
	for i, c := range cfg.EC2SDConfigs {
		app("ec2", i, discovery.NewEC2Discovery(c))
	}
	for i, c := range cfg.AzureSDConfigs {
		app("azure", i, discovery.NewAzureDiscovery(c))
	}
	if len(cfg.StaticConfigs) > 0 {
		app("static", 0, NewStaticProvider(cfg.StaticConfigs))
	}

	return providers
}

// targetsFromGroup builds targets based on the given TargetGroup and config.
// Panics if target group is nil.
func targetsFromGroup(tg *config.TargetGroup, cfg *config.ScrapeConfig) ([]*Target, error) {
	targets := make([]*Target, 0, len(tg.Targets))

	for i, labels := range tg.Targets {
		for k, v := range cfg.Params {
			if len(v) > 0 {
				labels[model.LabelName(model.ParamLabelPrefix+k)] = model.LabelValue(v[0])
			}
		}
		// Copy labels into the labelset for the target if they are not
		// set already. Apply the labelsets in order of decreasing precedence.
		labelsets := []model.LabelSet{
			tg.Labels,
			{
				model.SchemeLabel:      model.LabelValue(cfg.Scheme),
				model.MetricsPathLabel: model.LabelValue(cfg.MetricsPath),
				model.JobLabel:         model.LabelValue(cfg.JobName),
			},
		}
		for _, lset := range labelsets {
			for ln, lv := range lset {
				if _, ok := labels[ln]; !ok {
					labels[ln] = lv
				}
			}
		}

		if _, ok := labels[model.AddressLabel]; !ok {
			return nil, fmt.Errorf("instance %d in target group %s has no address", i, tg)
		}

		preRelabelLabels := labels

		labels := relabel.Process(labels, cfg.RelabelConfigs...)

		// Check if the target was dropped.
		if labels == nil {
			continue
		}
		// If no port was provided, infer it based on the used scheme.
		addr := string(labels[model.AddressLabel])
		if !strings.Contains(addr, ":") {
			switch labels[model.SchemeLabel] {
			case "http", "":
				addr = fmt.Sprintf("%s:80", addr)
			case "https":
				addr = fmt.Sprintf("%s:443", addr)
			default:
				return nil, fmt.Errorf("invalid scheme: %q", cfg.Scheme)
			}
			labels[model.AddressLabel] = model.LabelValue(addr)
		}
		if err := config.CheckTargetAddress(labels[model.AddressLabel]); err != nil {
			return nil, err
		}

		for ln := range labels {
			// Meta labels are deleted after relabelling. Other internal labels propagate to
			// the target which decides whether they will be part of their label set.
			if strings.HasPrefix(string(ln), model.MetaLabelPrefix) {
				delete(labels, ln)
			}
		}

		if _, ok := labels[model.InstanceLabel]; !ok {
			labels[model.InstanceLabel] = labels[model.AddressLabel]
		}

		targets = append(targets, NewTarget(labels, preRelabelLabels, cfg.Params))
	}

	return targets, nil
}

// StaticProvider holds a list of target groups that never change.
type StaticProvider struct {
	TargetGroups []*config.TargetGroup
}

// NewStaticProvider returns a StaticProvider configured with the given
// target groups.
func NewStaticProvider(groups []*config.TargetGroup) *StaticProvider {
	for i, tg := range groups {
		tg.Source = fmt.Sprintf("%d", i)
	}
	return &StaticProvider{groups}
}

// Run implements the TargetProvider interface.
func (sd *StaticProvider) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// We still have to consider that the consumer exits right away in which case
	// the context will be canceled.
	select {
	case ch <- sd.TargetGroups:
	case <-ctx.Done():
	}
	close(ch)
}
