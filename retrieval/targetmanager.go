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
	"strings"
	"sync"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
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
	appender storage.SampleAppender

	mtx    sync.RWMutex
	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup

	// Providers by the scrape configs they are derived from.
	scrapeSets []*scrapeSet
}

// NewTargetManager creates a new TargetManager.
func NewTargetManager(app storage.SampleAppender) *TargetManager {
	return &TargetManager{
		appender: app,
	}
}

// Run starts background processing to handle target updates.
func (tm *TargetManager) Run() {
	log.Info("Starting target manager...")

	tm.mtx.Lock()
	tm.ctx, tm.cancel = context.WithCancel(context.Background())

	for _, ss := range tm.scrapeSets {
		tm.wg.Add(1)

		go func(ss *scrapeSet) {
			ss.run(tm.ctx)
			tm.wg.Done()
		}(ss)
	}

	tm.mtx.Unlock()
	tm.wg.Wait()
}

// Stop all background processing.
func (tm *TargetManager) Stop() {
	log.Infoln("Stopping target manager...")

	tm.mtx.Lock()

	// Cancel the base context, this will cause all in-flight scrapes to abort immmediately.
	// Started inserts will be finished before terminating.
	tm.cancel()
	tm.mtx.Unlock()

	// Wait for all provider sets to terminate.
	tm.wg.Wait()
}

// Pools returns the targets currently being scraped bucketed by their job name.
func (tm *TargetManager) Pools() map[string][]*Target {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	pools := map[string][]*Target{}

	// TODO(fabxc): this is just a hack to maintain compatibility for now.
	for _, ps := range tm.scrapeSets {
		for _, ts := range ps.tgroups {
			for _, t := range ts {
				job := string(t.Labels()[model.JobLabel])
				pools[job] = append(pools[job], t)
			}
		}
	}
	return pools
}

// ApplyConfig resets the manager's target providers and job configurations as defined
// by the new cfg. The state of targets that are valid in the new configuration remains unchanged.
// Returns true on success.
func (tm *TargetManager) ApplyConfig(cfg *config.Config) bool {
	tm.mtx.RLock()
	running := tm.ctx != nil
	tm.mtx.RUnlock()

	if running {
		tm.Stop()
		defer func() {
			go tm.Run()
		}()
	}

	tm.mtx.Lock()

	tm.scrapeSets = tm.scrapeSets[:0]

	for _, scfg := range cfg.ScrapeConfigs {
		tm.scrapeSets = append(tm.scrapeSets, newScrapeSet(tm.appender, scfg))
	}

	tm.mtx.Unlock()

	return true
}

// scrapeSet holds several TargetProviders for which the same scrape configuration
// is used. It runs the target providers and starts and stops scrapers as it
// receives target updates.
type scrapeSet struct {
	appender storage.SampleAppender

	config  *config.ScrapeConfig
	tgroups map[string]map[model.Fingerprint]*Target

	mtx sync.RWMutex
}

func newScrapeSet(app storage.SampleAppender, cfg *config.ScrapeConfig) *scrapeSet {
	return &scrapeSet{
		appender: app,
		config:   cfg,
		tgroups:  map[string]map[model.Fingerprint]*Target{},
	}
}

// run starts the target providers with the given context and consumes
// and handles their updates. If the context is done, it blocks until the
// target scrapers have terminated.
func (ss *scrapeSet) run(ctx context.Context) {
	var (
		providers = providersFromConfig(ss.config)
		wg        sync.WaitGroup
	)

	for name, prov := range providers {
		var (
			updates = make(chan []*config.TargetGroup)
		)

		wg.Add(1)
		// The update and stopping operations for the target provider handling are blocking.
		// Thus the run method only returns if all background processing is complete.
		go func(name string, prov TargetProvider) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					ss.stopScrapers(name)
					return
				case tgs := <-updates:
					for _, tg := range tgs {
						if err := ss.update(name, tg); err != nil {
							log.With("target_group", tg).Errorf("Target update failed: %s", err)
						}
					}
				}
			}
		}(name, prov)

		go prov.Run(ctx, updates)
	}

	wg.Wait()
}

// stopScrapers shuts down all active scrapers for a provider.
func (ss *scrapeSet) stopScrapers(name string) {
	var wg sync.WaitGroup

	ss.mtx.RLock()
	// TODO(fabxc): the prefixing is slightly hacky but this will be gone with subsequent changes.
	for source, tgroup := range ss.tgroups {
		if !strings.HasPrefix(source, name) {
			continue
		}
		for _, t := range tgroup {
			wg.Add(1)

			go func(t *Target) {
				t.StopScraper()
				wg.Done()
			}(t)
		}
	}
	ss.mtx.RUnlock()

	wg.Wait()
}

// update handles a target group update from a target provider identified by the name.
func (ss *scrapeSet) update(name string, tgroup *config.TargetGroup) error {
	var (
		source      = name + "/" + tgroup.Source
		prevTargets = ss.tgroups[source]
	)

	targets, err := targetsFromGroup(tgroup, ss.config)
	if err != nil {
		return err
	}

	ss.mtx.Lock()
	ss.tgroups[source] = targets

	for fp, tnew := range targets {
		// If the same target existed before, we let it run and replace
		// the new one with it.
		if told, ok := prevTargets[fp]; ok {
			targets[fp] = told
		} else {
			go tnew.RunScraper(ss.appender)
		}
	}
	ss.mtx.Unlock()

	var wg sync.WaitGroup
	for fp, told := range prevTargets {
		// A previous target is no longer in the group.
		if _, ok := targets[fp]; !ok {
			wg.Add(1)

			go func(told *Target) {
				told.StopScraper()
				wg.Done()
			}(told)
		}
	}
	// Wait for all potentially stopped scrapers to terminate.
	// This covers the case of flapping targets. If the server is under high load, a new scraper
	// may be active and tries to insert. The old scraper that didn't terminate yet could still
	// be inserting a previous sample set.
	wg.Wait()

	return nil
}

// providersFromConfig returns all TargetProviders configured in cfg.
func providersFromConfig(cfg *config.ScrapeConfig) map[string]TargetProvider {
	providers := map[string]TargetProvider{}

	app := func(mech string, i int, tp TargetProvider) {
		providers[fmt.Sprintf("%s/%d", mech, i)] = tp
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, discovery.NewDNSDiscovery(c))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, discovery.NewFileDiscovery(c))
	}
	for i, c := range cfg.ConsulSDConfigs {
		k, err := discovery.NewConsulDiscovery(c)
		if err != nil {
			log.Errorf("Cannot create Consul discovery: %s", err)
			continue
		}
		app("consul", i, k)
	}
	for i, c := range cfg.MarathonSDConfigs {
		app("marathon", i, discovery.NewMarathonDiscovery(c))
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
	if len(cfg.TargetGroups) > 0 {
		app("static", 0, NewStaticProvider(cfg.TargetGroups))
	}

	return providers
}

// targetsFromGroup builds targets based on the given TargetGroup and config.
func targetsFromGroup(tg *config.TargetGroup, cfg *config.ScrapeConfig) (map[model.Fingerprint]*Target, error) {
	targets := make(map[model.Fingerprint]*Target, len(tg.Targets))
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

		labels, err := Relabel(labels, cfg.RelabelConfigs...)
		if err != nil {
			return nil, fmt.Errorf("error while relabeling instance %d in target group %s: %s", i, tg, err)
		}
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
				panic(fmt.Errorf("targetsFromGroup: invalid scheme %q", cfg.Scheme))
			}
			labels[model.AddressLabel] = model.LabelValue(addr)
		}
		if err = config.CheckTargetAddress(labels[model.AddressLabel]); err != nil {
			return nil, err
		}

		for ln := range labels {
			// Meta labels are deleted after relabelling. Other internal labels propagate to
			// the target which decides whether they will be part of their label set.
			if strings.HasPrefix(string(ln), model.MetaLabelPrefix) {
				delete(labels, ln)
			}
		}
		tr, err := NewTarget(cfg, labels, preRelabelLabels)
		if err != nil {
			return nil, fmt.Errorf("error while creating instance %d in target group %s: %s", i, tg, err)
		}

		targets[tr.fingerprint()] = tr
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
