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

	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"

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
// Sources() is guaranteed to be called exactly once before each call to Run().
// On a call to Run() implementing types must send a valid target group for each of
// the sources they declared in the last call to Sources().
type TargetProvider interface {
	// Sources returns the source identifiers the provider is currently aware of.
	Sources() []string
	// Run hands a channel to the target provider through which it can send
	// updated target groups. The channel must be closed by the target provider
	// if no more updates will be sent.
	// On receiving from done Run must return.
	Run(up chan<- *config.TargetGroup, done <-chan struct{})
}

// TargetManager maintains a set of targets, starts and stops their scraping and
// creates the new targets based on the target groups it receives from various
// target providers.
type TargetManager struct {
	mtx            sync.RWMutex
	globalLabels   clientmodel.LabelSet
	sampleAppender storage.SampleAppender
	running        bool
	done           chan struct{}

	// Targets by their source ID.
	targets map[string][]*Target
	// Providers by the scrape configs they are derived from.
	providers map[*config.ScrapeConfig][]TargetProvider
}

// NewTargetManager creates a new TargetManager.
func NewTargetManager(sampleAppender storage.SampleAppender) *TargetManager {
	tm := &TargetManager{
		sampleAppender: sampleAppender,
		targets:        make(map[string][]*Target),
	}
	return tm
}

// merge multiple target group channels into a single output channel.
func merge(done <-chan struct{}, cs ...<-chan targetGroupUpdate) <-chan targetGroupUpdate {
	var wg sync.WaitGroup
	out := make(chan targetGroupUpdate)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to out until c or done is closed, then calls
	// wg.Done.
	redir := func(c <-chan targetGroupUpdate) {
		defer wg.Done()
		for n := range c {
			select {
			case out <- n:
			case <-done:
				return
			}
		}
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go redir(c)
	}

	// Close the out channel if all inbound channels are closed.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// targetGroupUpdate is a potentially changed/new target group
// for the given scrape configuration.
type targetGroupUpdate struct {
	tg   *config.TargetGroup
	scfg *config.ScrapeConfig
}

// Run starts background processing to handle target updates.
func (tm *TargetManager) Run() {
	log.Info("Starting target manager...")

	tm.done = make(chan struct{})

	sources := map[string]struct{}{}
	updates := []<-chan targetGroupUpdate{}

	for scfg, provs := range tm.providers {
		for _, prov := range provs {
			// Get an initial set of available sources so we don't remove
			// target groups from the last run that are still available.
			for _, src := range prov.Sources() {
				sources[src] = struct{}{}
			}

			tgc := make(chan *config.TargetGroup)
			// Run the target provider after cleanup of the stale targets is done.
			defer func(prov TargetProvider, tgc chan *config.TargetGroup) {
				go prov.Run(tgc, tm.done)
			}(prov, tgc)

			tgupc := make(chan targetGroupUpdate)
			updates = append(updates, tgupc)

			go func(scfg *config.ScrapeConfig) {
				defer close(tgupc)
				for {
					select {
					case tg := <-tgc:
						if tg == nil {
							break
						}
						tgupc <- targetGroupUpdate{tg: tg, scfg: scfg}
					case <-tm.done:
						return
					}
				}
			}(scfg)
		}
	}

	// Merge all channels of incoming target group updates into a single
	// one and keep applying the updates.
	go tm.handleUpdates(merge(tm.done, updates...), tm.done)

	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	// Remove old target groups that are no longer in the set of sources.
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
func (tm *TargetManager) handleUpdates(ch <-chan targetGroupUpdate, done <-chan struct{}) {
	for {
		select {
		case update := <-ch:
			if update.tg == nil {
				break
			}
			log.Debugf("Received potential update for target group %q", update.tg.Source)

			if err := tm.updateTargetGroup(update.tg, update.scfg); err != nil {
				log.Errorf("Error updating targets: %s", err)
			}
		case <-done:
			return
		}
	}
}

// Stop all background processing.
func (tm *TargetManager) Stop() {
	tm.mtx.RLock()
	if tm.running {
		defer tm.stop(true)
	}
	// Return the lock before calling tm.stop().
	defer tm.mtx.RUnlock()
}

// stop background processing of the target manager. If removeTargets is true,
// existing targets will be stopped and removed.
func (tm *TargetManager) stop(removeTargets bool) {
	log.Info("Stopping target manager...")
	defer log.Info("Target manager stopped.")

	close(tm.done)

	tm.mtx.Lock()
	defer tm.mtx.Unlock()

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
			go func(t *Target) {
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

	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	if !tm.running {
		return nil
	}

	oldTargets, ok := tm.targets[tgroup.Source]
	if ok {
		var wg sync.WaitGroup
		// Replace the old targets with the new ones while keeping the state
		// of intersecting targets.
		for i, tnew := range newTargets {
			var match *Target
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
			// Update the existing target and discard the new equivalent.
			// Otherwise start scraping the new target.
			if match != nil {
				// Updating is blocked during a scrape. We don't want those wait times
				// to build up.
				wg.Add(1)
				go func(t *Target) {
					match.Update(cfg, t.fullLabels(), t.metaLabels)
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
				go func(t *Target) {
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
		tm.targets[tgroup.Source] = newTargets
	} else {
		delete(tm.targets, tgroup.Source)
	}
	return nil
}

// Pools returns the targets currently being scraped bucketed by their job name.
func (tm *TargetManager) Pools() map[string][]*Target {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	pools := map[string][]*Target{}

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
// Returns true on success.
func (tm *TargetManager) ApplyConfig(cfg *config.Config) bool {
	tm.mtx.RLock()
	running := tm.running
	tm.mtx.RUnlock()

	if running {
		tm.stop(false)
		// Even if updating the config failed, we want to continue rather than stop scraping anything.
		defer tm.Run()
	}
	providers := map[*config.ScrapeConfig][]TargetProvider{}

	for _, scfg := range cfg.ScrapeConfigs {
		providers[scfg] = providersFromConfig(scfg)
	}

	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	tm.globalLabels = cfg.GlobalConfig.Labels
	tm.providers = providers
	return true
}

// prefixedTargetProvider wraps TargetProvider and prefixes source strings
// to make the sources unique across a configuration.
type prefixedTargetProvider struct {
	TargetProvider

	job       string
	mechanism string
	idx       int
}

func (tp *prefixedTargetProvider) prefix(src string) string {
	return fmt.Sprintf("%s:%s:%d:%s", tp.job, tp.mechanism, tp.idx, src)
}

func (tp *prefixedTargetProvider) Sources() []string {
	srcs := tp.TargetProvider.Sources()
	for i, src := range srcs {
		srcs[i] = tp.prefix(src)
	}

	return srcs
}

func (tp *prefixedTargetProvider) Run(ch chan<- *config.TargetGroup, done <-chan struct{}) {
	defer close(ch)

	ch2 := make(chan *config.TargetGroup)
	go tp.TargetProvider.Run(ch2, done)

	for {
		select {
		case <-done:
			return
		case tg := <-ch2:
			if tg == nil {
				break
			}
			tg.Source = tp.prefix(tg.Source)
			ch <- tg
		}
	}
}

// providersFromConfig returns all TargetProviders configured in cfg.
func providersFromConfig(cfg *config.ScrapeConfig) []TargetProvider {
	var providers []TargetProvider

	app := func(mech string, i int, tp TargetProvider) {
		providers = append(providers, &prefixedTargetProvider{
			job:            cfg.JobName,
			mechanism:      mech,
			idx:            i,
			TargetProvider: tp,
		})
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, discovery.NewDNSDiscovery(c))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, discovery.NewFileDiscovery(c))
	}
	for i, c := range cfg.ConsulSDConfigs {
		app("consul", i, discovery.NewConsulDiscovery(c))
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
	if len(cfg.TargetGroups) > 0 {
		app("static", 0, NewStaticProvider(cfg.TargetGroups))
	}

	return providers
}

// targetsFromGroup builds targets based on the given TargetGroup and config.
func (tm *TargetManager) targetsFromGroup(tg *config.TargetGroup, cfg *config.ScrapeConfig) ([]*Target, error) {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	targets := make([]*Target, 0, len(tg.Targets))
	for i, labels := range tg.Targets {
		addr := string(labels[clientmodel.AddressLabel])
		// If no port was provided, infer it based on the used scheme.
		if !strings.Contains(addr, ":") {
			switch cfg.Scheme {
			case "http":
				addr = fmt.Sprintf("%s:80", addr)
			case "https":
				addr = fmt.Sprintf("%s:443", addr)
			default:
				panic(fmt.Errorf("targetsFromGroup: invalid scheme %q", cfg.Scheme))
			}
			labels[clientmodel.AddressLabel] = clientmodel.LabelValue(addr)
		}
		for k, v := range cfg.Params {
			if len(v) > 0 {
				labels[clientmodel.LabelName(clientmodel.ParamLabelPrefix+k)] = clientmodel.LabelValue(v[0])
			}
		}
		// Copy labels into the labelset for the target if they are not
		// set already. Apply the labelsets in order of decreasing precedence.
		labelsets := []clientmodel.LabelSet{
			tg.Labels,
			{
				clientmodel.SchemeLabel:      clientmodel.LabelValue(cfg.Scheme),
				clientmodel.MetricsPathLabel: clientmodel.LabelValue(cfg.MetricsPath),
				clientmodel.JobLabel:         clientmodel.LabelValue(cfg.JobName),
			},
			tm.globalLabels,
		}
		for _, lset := range labelsets {
			for ln, lv := range lset {
				if _, ok := labels[ln]; !ok {
					labels[ln] = lv
				}
			}
		}

		if _, ok := labels[clientmodel.AddressLabel]; !ok {
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

		for ln := range labels {
			// Meta labels are deleted after relabelling. Other internal labels propagate to
			// the target which decides whether they will be part of their label set.
			if strings.HasPrefix(string(ln), clientmodel.MetaLabelPrefix) {
				delete(labels, ln)
			}
		}
		tr := NewTarget(cfg, labels, preRelabelLabels)
		targets = append(targets, tr)
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
	return &StaticProvider{
		TargetGroups: groups,
	}
}

// Run implements the TargetProvider interface.
func (sd *StaticProvider) Run(ch chan<- *config.TargetGroup, done <-chan struct{}) {
	defer close(ch)

	for _, tg := range sd.TargetGroups {
		select {
		case <-done:
			return
		case ch <- tg:
		}
	}
	<-done
}

// TargetGroups returns the provider's target groups.
func (sd *StaticProvider) Sources() (srcs []string) {
	for _, tg := range sd.TargetGroups {
		srcs = append(srcs, tg.Source)
	}
	return srcs
}
