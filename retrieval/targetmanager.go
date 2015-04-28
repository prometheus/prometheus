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

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	pb "github.com/prometheus/prometheus/config/generated"
	"github.com/prometheus/prometheus/retrieval/discovery"
	"github.com/prometheus/prometheus/storage"
)

// A TargetProvider provides information about target groups. It maintains a set
// of sources from which TargetGroups can originate. Whenever a target provider
// detects a potential change, it sends the TargetGroup through its provided channel.
//
// The TargetProvider does not have to guarantee that an actual change happened.
// It does guarantee that it sends the new TargetGroup whenever a change happens.
// On startup it sends all TargetGroups it can see.
type TargetProvider interface {
	// Sources returns the source identifiers the provider is currently aware of.
	Sources() []string
	// Run hands a channel to the target provider through which it can send
	// updated target groups. The channel must be closed by the target provider
	// if no more updates will be sent.
	Run(chan<- *config.TargetGroup)
	// Stop terminates any potential computation of the target provider. The
	// channel received on Run must be closed afterwards.
	Stop()
}

// TargetManager maintains a set of targets, starts and stops their scraping and
// creates the new targets based on the target groups it receives from various
// target providers.
type TargetManager struct {
	m              sync.RWMutex
	globalLabels   clientmodel.LabelSet
	sampleAppender storage.SampleAppender
	running        bool

	// Targets by their source ID.
	targets map[string][]Target
	// Providers by the scrape configs they are derived from.
	providers map[*config.ScrapeConfig][]TargetProvider
}

// NewTargetManager creates a new TargetManager based on the given config.
func NewTargetManager(cfg config.Config, sampleAppender storage.SampleAppender) (*TargetManager, error) {
	tm := &TargetManager{
		sampleAppender: sampleAppender,
		targets:        make(map[string][]Target),
	}
	if err := tm.applyConfig(cfg); err != nil {
		return nil, err
	}
	return tm, nil
}

// Run starts background processing to handle target updates.
func (tm *TargetManager) Run() {
	glog.Info("Starting target manager...")

	sources := map[string]struct{}{}

	for scfg, provs := range tm.providers {
		for _, p := range provs {
			ch := make(chan *config.TargetGroup)
			go tm.handleTargetUpdates(scfg, ch)

			for _, src := range p.Sources() {
				src = fullSource(scfg, src)
				sources[src] = struct{}{}
			}

			// Run the target provider after cleanup of the stale targets is done.
			defer func(c chan *config.TargetGroup) {
				go p.Run(c)
			}(ch)
		}
	}

	tm.removeTargets(func(src string) bool {
		if _, ok := sources[src]; ok {
			return false
		}
		return true
	})

	tm.running = true
}

// handleTargetUpdates receives target group updates and handles them in the
// context of the given job config.
func (tm *TargetManager) handleTargetUpdates(cfg *config.ScrapeConfig, ch <-chan *config.TargetGroup) {
	for tg := range ch {
		glog.V(1).Infof("Received potential update for target group %q", tg.Source)

		if err := tm.updateTargetGroup(tg, cfg); err != nil {
			glog.Errorf("Error updating targets: %s", err)
		}
	}
}

// fullSource prepends the unique job name to the source.
//
// Thus, oscilliating label sets for targets with the same source,
// but providers from different configs, are prevented.
func fullSource(cfg *config.ScrapeConfig, src string) string {
	return cfg.GetJobName() + ":" + src
}

// Stop all background processing.
func (tm *TargetManager) Stop() {
	tm.stop(true)
}

// stop background processing of the target manager. If removeTargets is true,
// existing targets will be stopped and removed.
func (tm *TargetManager) stop(removeTargets bool) {
	tm.m.Lock()
	defer tm.m.Unlock()

	if !tm.running {
		return
	}

	glog.Info("Stopping target manager...")
	defer glog.Info("Target manager stopped.")

	for _, provs := range tm.providers {
		for _, p := range provs {
			p.Stop()
		}
	}

	if removeTargets {
		tm.removeTargets(nil)
	}

	tm.running = false
}

// removeTargets stops and removes targets for sources where f(source) is true
// or if f is nil. This method is not thread-safe.
func (tm *TargetManager) removeTargets(f func(string) bool) {
	if f == nil {
		f = func(string) bool { return true }
	}
	var wg sync.WaitGroup
	for src, targets := range tm.targets {
		if !f(src) {
			continue
		}
		wg.Add(len(targets))
		for _, target := range targets {
			go func(t Target) {
				t.StopScraper()
				wg.Done()
			}(target)
		}
		delete(tm.targets, src)
	}
	wg.Wait()
}

// updateTargetGroup creates new targets for the group and replaces the old targets
// for the source ID.
func (tm *TargetManager) updateTargetGroup(tgroup *config.TargetGroup, cfg *config.ScrapeConfig) error {
	newTargets, err := tm.targetsFromGroup(tgroup, cfg)
	if err != nil {
		return err
	}
	src := fullSource(cfg, tgroup.Source)

	tm.m.Lock()
	defer tm.m.Unlock()

	if !tm.running {
		return nil
	}

	oldTargets, ok := tm.targets[src]
	if ok {
		var wg sync.WaitGroup
		// Replace the old targets with the new ones while keeping the state
		// of intersecting targets.
		for i, tnew := range newTargets {
			var match Target
			for j, told := range oldTargets {
				if told == nil {
					continue
				}
				if tnew.InstanceIdentifier() == told.InstanceIdentifier() {
					match = told
					oldTargets[j] = nil
					break
				}
			}
			// Update the exisiting target and discard the new equivalent.
			// Otherwise start scraping the new target.
			if match != nil {
				// Updating is blocked during a scrape. We don't want those wait times
				// to build up.
				wg.Add(1)
				go func(t Target) {
					match.Update(cfg, t.Labels())
					wg.Done()
				}(tnew)
				newTargets[i] = match
			} else {
				go tnew.RunScraper(tm.sampleAppender)
			}
		}
		// Remove all old targets that disappeared.
		for _, told := range oldTargets {
			if told != nil {
				wg.Add(1)
				go func(t Target) {
					t.StopScraper()
					wg.Done()
				}(told)
			}
		}
		wg.Wait()
	} else {
		// The source ID is new, start all target scrapers.
		for _, tnew := range newTargets {
			go tnew.RunScraper(tm.sampleAppender)
		}
	}

	if len(newTargets) > 0 {
		tm.targets[src] = newTargets
	} else {
		delete(tm.targets, src)
	}
	return nil
}

// Pools returns the targets currently being scraped bucketed by their job name.
func (tm *TargetManager) Pools() map[string][]Target {
	tm.m.RLock()
	defer tm.m.RUnlock()

	pools := map[string][]Target{}

	for _, ts := range tm.targets {
		for _, t := range ts {
			job := string(t.BaseLabels()[clientmodel.JobLabel])
			pools[job] = append(pools[job], t)
		}
	}
	return pools
}

// ApplyConfig resets the manager's target providers and job configurations as defined
// by the new cfg. The state of targets that are valid in the new configuration remains unchanged.
func (tm *TargetManager) ApplyConfig(cfg config.Config) error {
	tm.stop(false)
	// Even if updating the config failed, we want to continue rather than stop scraping anything.
	defer tm.Run()

	if err := tm.applyConfig(cfg); err != nil {
		glog.Warningf("Error updating config, changes not applied: %s", err)
		return err
	}
	return nil
}

func (tm *TargetManager) applyConfig(cfg config.Config) error {
	// Only apply changes if everything was successful.
	providers := map[*config.ScrapeConfig][]TargetProvider{}

	for _, scfg := range cfg.ScrapeConfigs() {
		provs, err := ProvidersFromConfig(scfg)
		if err != nil {
			return err
		}
		providers[scfg] = provs
	}
	tm.m.Lock()
	defer tm.m.Unlock()

	tm.globalLabels = cfg.GlobalLabels()
	tm.providers = providers
	return nil
}

// targetsFromGroup builds targets based on the given TargetGroup and config.
func (tm *TargetManager) targetsFromGroup(tg *config.TargetGroup, cfg *config.ScrapeConfig) ([]Target, error) {
	tm.m.RLock()
	defer tm.m.RUnlock()

	targets := make([]Target, 0, len(tg.Targets))
	for i, labels := range tg.Targets {
		// Copy labels into the labelset for the target if they are not
		// set already. Apply the labelsets in order of decreasing precedence.
		labelsets := []clientmodel.LabelSet{
			tg.Labels,
			cfg.Labels(),
			tm.globalLabels,
		}
		for _, lset := range labelsets {
			for ln, lv := range lset {
				if _, ok := labels[ln]; !ok {
					labels[ln] = lv
				}
			}
		}

		address, ok := labels[clientmodel.AddressLabel]
		if !ok {
			return nil, fmt.Errorf("Instance %d in target group %s has no address", i, tg)
		}

		for ln := range labels {
			// Meta labels are deleted after relabelling. Other internal labels propagate to
			// the target which decides whether they will be part of their label set.
			if strings.HasPrefix(string(ln), clientmodel.MetaLabelPrefix) {
				delete(labels, ln)
			}
		}
		targets = append(targets, NewTarget(string(address), cfg, labels))

	}

	return targets, nil
}

// ProvidersFromConfig returns all TargetProviders configured in cfg.
func ProvidersFromConfig(cfg *config.ScrapeConfig) ([]TargetProvider, error) {
	var providers []TargetProvider

	for _, dnscfg := range cfg.DNSConfigs() {
		dnsSD := discovery.NewDNSDiscovery(dnscfg.GetName(), dnscfg.RefreshInterval())
		providers = append(providers, dnsSD)
	}
	if tgs := cfg.GetTargetGroup(); tgs != nil {
		static := NewStaticProvider(tgs)
		providers = append(providers, static)
	}
	return providers, nil
}

// StaticProvider holds a list of target groups that never change.
type StaticProvider struct {
	TargetGroups []*config.TargetGroup
}

// NewStaticProvider returns a StaticProvider configured with the given
// target groups.
func NewStaticProvider(groups []*pb.TargetGroup) *StaticProvider {
	prov := &StaticProvider{}

	for i, tg := range groups {
		g := &config.TargetGroup{
			Source: fmt.Sprintf("static:%d", i),
			Labels: clientmodel.LabelSet{},
		}
		for _, pair := range tg.GetLabels().GetLabel() {
			g.Labels[clientmodel.LabelName(pair.GetName())] = clientmodel.LabelValue(pair.GetValue())
		}
		for _, t := range tg.GetTarget() {
			g.Targets = append(g.Targets, clientmodel.LabelSet{
				clientmodel.AddressLabel: clientmodel.LabelValue(t),
			})
		}
		prov.TargetGroups = append(prov.TargetGroups, g)
	}
	return prov
}

// Run implements the TargetProvider interface.
func (sd *StaticProvider) Run(ch chan<- *config.TargetGroup) {
	for _, tg := range sd.TargetGroups {
		ch <- tg
	}
	close(ch) // This provider never sends any updates.
}

// Stop implements the TargetProvider interface.
func (sd *StaticProvider) Stop() {}

// TargetGroups returns the provider's target groups.
func (sd *StaticProvider) Sources() (srcs []string) {
	for _, tg := range sd.TargetGroups {
		srcs = append(srcs, tg.Source)
	}
	return srcs
}
