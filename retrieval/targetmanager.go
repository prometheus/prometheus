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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/log"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval/discovery"
	"github.com/prometheus/prometheus/storage"
)

var (
	scrapePoolSyncs = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scrape_pool_syncs_total",
			Help:      "Number of target synchronizations with the scrape pool.",
		},
	)
	targetGroupUpdates = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "target_group_updates_total",
			Help:      "Number of received target group updates.",
		},
	)
)

func init() {
	prometheus.MustRegister(scrapePoolSyncs)
	prometheus.MustRegister(targetGroupUpdates)
}

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
	pool *ScrapePool

	done    chan struct{}
	mtx     sync.RWMutex
	running bool

	providers map[*config.ScrapeConfig][]TargetProvider
	// Targets by their source ID.
	targets map[string][]*Target
}

// NewTargetManager creates a new TargetManager.
func NewTargetManager(app storage.SampleAppender) *TargetManager {
	tm := &TargetManager{
		targets: map[string][]*Target{},
		pool:    NewScrapePool(app),
	}
	return tm
}

// targetGroupUpdate is a potentially changed/new target group
// for the given scrape configuration.
type targetGroupUpdate struct {
	tg   *config.TargetGroup
	scfg *config.ScrapeConfig
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

func (tm *TargetManager) sync() {
	var all []*Target

	for _, targets := range tm.targets {
		all = append(all, targets...)
	}

	tm.pool.Sync(all)
}

// Run starts background processing to handle target updates.
func (tm *TargetManager) Run() {
	log.Info("Starting target manager...")

	sources := map[string]struct{}{}
	updates := []<-chan targetGroupUpdate{}

	tm.mtx.Lock()
	tm.done = make(chan struct{})

	for scfg, provs := range tm.providers {
		for _, prov := range provs {

			for _, src := range prov.Sources() {
				sources[src] = struct{}{}
			}

			ch := make(chan *config.TargetGroup)
			go prov.Run(ch, tm.done)

			tgupc := make(chan targetGroupUpdate)
			updates = append(updates, tgupc)

			go func(scfg *config.ScrapeConfig) {
				defer close(tgupc)
				for {
					select {
					case tg, ok := <-ch:
						if !ok {
							return
						}
						tgupc <- targetGroupUpdate{tg: tg, scfg: scfg}
					case <-tm.done:
						return
					}
				}
			}(scfg)
		}
	}

	// Remove target groups that disappeared since previous run.
	for src := range tm.targets {
		if _, ok := sources[src]; !ok {
			delete(tm.targets, src)
		}
	}

	tm.running = true

	tm.sync()

	tm.mtx.Unlock()

	// Merge all channels of incoming target group updates into a single
	// one and keep applying the updates.
	tm.handleUpdates(merge(tm.done, updates...), tm.done)
}

// handleUpdates receives target group updates and handles them in the
// context of the given job config.
func (tm *TargetManager) handleUpdates(ch <-chan targetGroupUpdate, done <-chan struct{}) {
	// To not sync constantly on a stream of updates, attempt a sync every second
	// if an update has been received.
	syncTicker := time.NewTicker(1 * time.Second)
	defer syncTicker.Stop()

	dirty := false

	for {
		select {
		case update, ok := <-ch:
			if !ok {
				return
			}
			if update.tg == nil {
				break
			}
			log.Debugf("Received potential update for target group %q", update.tg.Source)
			targetGroupUpdates.Inc()

			if err := tm.updateTargetGroup(update.tg, update.scfg); err != nil {
				log.Errorf("Error updating targets: %s", err)
			}
			dirty = true

		case <-syncTicker.C:
			if dirty {
				tm.sync()
				scrapePoolSyncs.Inc()
			}
			dirty = false

		case <-done:
			return
		}
	}
}

// Stop all background processing.
func (tm *TargetManager) Stop() {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	log.Info("Stopping target manager...")
	defer log.Info("Target manager stopped.")

	tm.stop()
	tm.pool.Stop()
}

func (tm *TargetManager) stop() {
	if tm.running {
		close(tm.done)
		tm.running = false
	}
}

// updateTargetGroup creates new targets for the group and replaces the old targets
// for the source ID.
func (tm *TargetManager) updateTargetGroup(tgroup *config.TargetGroup, cfg *config.ScrapeConfig) error {
	newTargets, err := targetsFromGroup(tgroup, cfg)
	if err != nil {
		return err
	}

	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	if len(newTargets) == 0 {
		delete(tm.targets, tgroup.Source)
		return nil
	}

	tm.targets[tgroup.Source] = newTargets
	return nil
}

// Pools returns the targets currently being scraped bucketed by their job name.
func (tm *TargetManager) Pools() map[string][]*Target {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()

	tm.pool.mtx.RLock()
	defer tm.pool.mtx.RUnlock()

	pools := map[string][]*Target{}

	for _, ts := range tm.targets {
		for _, t := range ts {
			job := string(t.BaseLabels()[model.JobLabel])

			// Populate target with scrape status.
			// TODO(fabxc): remove.
			t.Status = tm.pool.states[t.fingerprint()]

			pools[job] = append(pools[job], t)
		}
	}
	return pools
}

// ApplyConfig resets the manager's target providers and job configurations as defined
// by the new cfg. The state of targets that are valid in the new configuration remains unchanged.
// Returns true on success.
func (tm *TargetManager) ApplyConfig(cfg *config.Config) bool {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	if tm.running {
		tm.stop()
		defer func() { go tm.Run() }()
	}

	providers := make(map[*config.ScrapeConfig][]TargetProvider, len(cfg.ScrapeConfigs))

	for _, scfg := range cfg.ScrapeConfigs {
		providers[scfg] = providersFromConfig(scfg)
	}

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
		case tg, ok := <-ch2:
			if !ok {
				return
			}
			tg.Source = tp.prefix(tg.Source)
			ch <- tg
		case <-done:
			return
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
func targetsFromGroup(tg *config.TargetGroup, cfg *config.ScrapeConfig) ([]*Target, error) {
	targets := make([]*Target, 0, len(tg.Targets))

	for i, labels := range tg.Targets {
		addr := string(labels[model.AddressLabel])
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
			labels[model.AddressLabel] = model.LabelValue(addr)
		}
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

		for ln := range labels {
			// Meta labels are deleted after relabelling. Other internal labels propagate to
			// the target which decides whether they will be part of their label set.
			if strings.HasPrefix(string(ln), model.MetaLabelPrefix) {
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

// Sources returns the provider's sources.
func (sd *StaticProvider) Sources() (srcs []string) {
	for _, tg := range sd.TargetGroups {
		srcs = append(srcs, tg.Source)
	}
	return srcs
}
