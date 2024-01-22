// Copyright 2016 The Prometheus Authors
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

package discovery

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type poolKey struct {
	setName  string
	provider string
}

// Provider holds a Discoverer instance, its configuration, cancel func and its subscribers.
type Provider struct {
	name   string
	d      Discoverer
	config interface{}

	cancel context.CancelFunc
	// done should be called after cleaning up resources associated with cancelled provider.
	done func()

	mu   sync.RWMutex
	subs map[string]struct{}

	// newSubs is used to temporary store subs to be used upon config reload completion.
	newSubs map[string]struct{}
}

// Discoverer return the Discoverer of the provider.
func (p *Provider) Discoverer() Discoverer {
	return p.d
}

// IsStarted return true if Discoverer is started.
func (p *Provider) IsStarted() bool {
	return p.cancel != nil
}

func (p *Provider) Config() interface{} {
	return p.config
}

// Registers the metrics needed for SD mechanisms.
// Does not register the metrics for the Discovery Manager.
// TODO(ptodev): Add ability to unregister the metrics?
func CreateAndRegisterSDMetrics(reg prometheus.Registerer) (map[string]DiscovererMetrics, error) {
	// Some SD mechanisms use the "refresh" package, which has its own metrics.
	refreshSdMetrics := NewRefreshMetrics(reg)

	// Register the metrics specific for each SD mechanism, and the ones for the refresh package.
	sdMetrics, err := RegisterSDMetrics(reg, refreshSdMetrics)
	if err != nil {
		return nil, fmt.Errorf("failed to register service discovery metrics: %w", err)
	}

	return sdMetrics, nil
}

// NewManager is the Discovery Manager constructor.
func NewManager(ctx context.Context, logger log.Logger, registerer prometheus.Registerer, sdMetrics map[string]DiscovererMetrics, options ...func(*Manager)) *Manager {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	mgr := &Manager{
		logger:      logger,
		syncCh:      make(chan map[string][]*targetgroup.Group),
		targets:     make(map[poolKey]map[string]*targetgroup.Group),
		ctx:         ctx,
		updatert:    5 * time.Second,
		triggerSend: make(chan struct{}, 1),
		registerer:  registerer,
		sdMetrics:   sdMetrics,
	}
	for _, option := range options {
		option(mgr)
	}

	// Register the metrics.
	// We have to do this after setting all options, so that the name of the Manager is set.
	if metrics, err := NewManagerMetrics(registerer, mgr.name); err == nil {
		mgr.metrics = metrics
	} else {
		level.Error(logger).Log("msg", "Failed to create discovery manager metrics", "manager", mgr.name, "err", err)
		return nil
	}

	return mgr
}

// Name sets the name of the manager.
func Name(n string) func(*Manager) {
	return func(m *Manager) {
		m.mtx.Lock()
		defer m.mtx.Unlock()
		m.name = n
	}
}

// HTTPClientOptions sets the list of HTTP client options to expose to
// Discoverers. It is up to Discoverers to choose to use the options provided.
func HTTPClientOptions(opts ...config.HTTPClientOption) func(*Manager) {
	return func(m *Manager) {
		m.httpOpts = opts
	}
}

// Manager maintains a set of discovery providers and sends each update to a map channel.
// Targets are grouped by the target set name.
type Manager struct {
	logger   log.Logger
	name     string
	httpOpts []config.HTTPClientOption
	mtx      sync.RWMutex
	ctx      context.Context

	// Some Discoverers(e.g. k8s) send only the updates for a given target group,
	// so we use map[tg.Source]*targetgroup.Group to know which group to update.
	targets    map[poolKey]map[string]*targetgroup.Group
	targetsMtx sync.Mutex

	// providers keeps track of SD providers.
	providers []*Provider
	// The sync channel sends the updates as a map where the key is the job value from the scrape config.
	syncCh chan map[string][]*targetgroup.Group

	// How long to wait before sending updates to the channel. The variable
	// should only be modified in unit tests.
	updatert time.Duration

	// The triggerSend channel signals to the Manager that new updates have been received from providers.
	triggerSend chan struct{}

	// lastProvider counts providers registered during Manager's lifetime.
	lastProvider uint

	// A registerer for all service discovery metrics.
	registerer prometheus.Registerer

	metrics   *Metrics
	sdMetrics map[string]DiscovererMetrics
}

// Providers returns the currently configured SD providers.
func (m *Manager) Providers() []*Provider {
	return m.providers
}

// Run starts the background processing.
func (m *Manager) Run() error {
	go m.sender()
	<-m.ctx.Done()
	m.cancelDiscoverers()
	return m.ctx.Err()
}

// SyncCh returns a read only channel used by all the clients to receive target updates.
func (m *Manager) SyncCh() <-chan map[string][]*targetgroup.Group {
	return m.syncCh
}

// ApplyConfig checks if discovery provider with supplied config is already running and keeps them as is.
// Remaining providers are then stopped and new required providers are started using the provided config.
func (m *Manager) ApplyConfig(cfg map[string]Configs) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var failedCount int
	for name, scfg := range cfg {
		failedCount += m.registerProviders(scfg, name)
	}
	m.metrics.FailedConfigs.Set(float64(failedCount))

	var (
		wg sync.WaitGroup
		// keep shows if we keep any providers after reload.
		keep         bool
		newProviders []*Provider
	)
	for _, prov := range m.providers {
		// Cancel obsolete providers.
		if len(prov.newSubs) == 0 {
			wg.Add(1)
			prov.done = func() {
				wg.Done()
			}
			prov.cancel()
			continue
		}
		newProviders = append(newProviders, prov)
		// refTargets keeps reference targets used to populate new subs' targets
		var refTargets map[string]*targetgroup.Group
		prov.mu.Lock()

		m.targetsMtx.Lock()
		for s := range prov.subs {
			keep = true
			refTargets = m.targets[poolKey{s, prov.name}]
			// Remove obsolete subs' targets.
			if _, ok := prov.newSubs[s]; !ok {
				delete(m.targets, poolKey{s, prov.name})
				m.metrics.DiscoveredTargets.DeleteLabelValues(m.name, s)
			}
		}
		// Set metrics and targets for new subs.
		for s := range prov.newSubs {
			if _, ok := prov.subs[s]; !ok {
				m.metrics.DiscoveredTargets.WithLabelValues(s).Set(0)
			}
			if l := len(refTargets); l > 0 {
				m.targets[poolKey{s, prov.name}] = make(map[string]*targetgroup.Group, l)
				for k, v := range refTargets {
					m.targets[poolKey{s, prov.name}][k] = v
				}
			}
		}
		m.targetsMtx.Unlock()

		prov.subs = prov.newSubs
		prov.newSubs = map[string]struct{}{}
		prov.mu.Unlock()
		if !prov.IsStarted() {
			m.startProvider(m.ctx, prov)
		}
	}
	// Currently downstream managers expect full target state upon config reload, so we must oblige.
	// While startProvider does pull the trigger, it may take some time to do so, therefore
	// we pull the trigger as soon as possible so that downstream managers can populate their state.
	// See https://github.com/prometheus/prometheus/pull/8639 for details.
	if keep {
		select {
		case m.triggerSend <- struct{}{}:
		default:
		}
	}
	m.providers = newProviders
	wg.Wait()

	return nil
}

// StartCustomProvider is used for sdtool. Only use this if you know what you're doing.
func (m *Manager) StartCustomProvider(ctx context.Context, name string, worker Discoverer) {
	p := &Provider{
		name: name,
		d:    worker,
		subs: map[string]struct{}{
			name: {},
		},
	}
	m.providers = append(m.providers, p)
	m.startProvider(ctx, p)
}

func (m *Manager) startProvider(ctx context.Context, p *Provider) {
	level.Debug(m.logger).Log("msg", "Starting provider", "provider", p.name, "subs", fmt.Sprintf("%v", p.subs))
	ctx, cancel := context.WithCancel(ctx)
	updates := make(chan []*targetgroup.Group)

	p.cancel = cancel

	go p.d.Run(ctx, updates)
	go m.updater(ctx, p, updates)
}

// cleaner cleans resources associated with provider.
func (m *Manager) cleaner(p *Provider) {
	m.targetsMtx.Lock()
	p.mu.RLock()
	for s := range p.subs {
		delete(m.targets, poolKey{s, p.name})
	}
	p.mu.RUnlock()
	m.targetsMtx.Unlock()
	if p.done != nil {
		p.done()
	}
}

func (m *Manager) updater(ctx context.Context, p *Provider, updates chan []*targetgroup.Group) {
	// Ensure targets from this provider are cleaned up.
	defer m.cleaner(p)
	for {
		select {
		case <-ctx.Done():
			return
		case tgs, ok := <-updates:
			m.metrics.ReceivedUpdates.Inc()
			if !ok {
				level.Debug(m.logger).Log("msg", "Discoverer channel closed", "provider", p.name)
				// Wait for provider cancellation to ensure targets are cleaned up when expected.
				<-ctx.Done()
				return
			}

			p.mu.RLock()
			for s := range p.subs {
				m.updateGroup(poolKey{setName: s, provider: p.name}, tgs)
			}
			p.mu.RUnlock()

			select {
			case m.triggerSend <- struct{}{}:
			default:
			}
		}
	}
}

func (m *Manager) sender() {
	ticker := time.NewTicker(m.updatert)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C: // Some discoverers send updates too often, so we throttle these with the ticker.
			select {
			case <-m.triggerSend:
				m.metrics.SentUpdates.Inc()
				select {
				case m.syncCh <- m.allGroups():
				default:
					m.metrics.DelayedUpdates.Inc()
					level.Debug(m.logger).Log("msg", "Discovery receiver's channel was full so will retry the next cycle")
					select {
					case m.triggerSend <- struct{}{}:
					default:
					}
				}
			default:
			}
		}
	}
}

func (m *Manager) cancelDiscoverers() {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for _, p := range m.providers {
		if p.cancel != nil {
			p.cancel()
		}
	}
}

func (m *Manager) updateGroup(poolKey poolKey, tgs []*targetgroup.Group) {
	m.targetsMtx.Lock()
	defer m.targetsMtx.Unlock()

	if _, ok := m.targets[poolKey]; !ok {
		m.targets[poolKey] = make(map[string]*targetgroup.Group)
	}
	for _, tg := range tgs {
		if tg != nil { // Some Discoverers send nil target group so need to check for it to avoid panics.
			m.targets[poolKey][tg.Source] = tg
		}
	}
}

func (m *Manager) allGroups() map[string][]*targetgroup.Group {
	tSets := map[string][]*targetgroup.Group{}
	n := map[string]int{}

	m.targetsMtx.Lock()
	defer m.targetsMtx.Unlock()
	for pkey, tsets := range m.targets {
		for _, tg := range tsets {
			// Even if the target group 'tg' is empty we still need to send it to the 'Scrape manager'
			// to signal that it needs to stop all scrape loops for this target set.
			tSets[pkey.setName] = append(tSets[pkey.setName], tg)
			n[pkey.setName] += len(tg.Targets)
		}
	}
	for setName, v := range n {
		m.metrics.DiscoveredTargets.WithLabelValues(setName).Set(float64(v))
	}
	return tSets
}

// registerProviders returns a number of failed SD config.
func (m *Manager) registerProviders(cfgs Configs, setName string) int {
	var (
		failed int
		added  bool
	)
	add := func(cfg Config) {
		for _, p := range m.providers {
			if reflect.DeepEqual(cfg, p.config) {
				p.newSubs[setName] = struct{}{}
				added = true
				return
			}
		}
		typ := cfg.Name()
		d, err := cfg.NewDiscoverer(DiscovererOptions{
			Logger:            log.With(m.logger, "discovery", typ, "config", setName),
			HTTPClientOptions: m.httpOpts,
			Metrics:           m.sdMetrics[typ],
		})
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot create service discovery", "err", err, "type", typ, "config", setName)
			failed++
			return
		}
		m.providers = append(m.providers, &Provider{
			name:   fmt.Sprintf("%s/%d", typ, m.lastProvider),
			d:      d,
			config: cfg,
			newSubs: map[string]struct{}{
				setName: {},
			},
		})
		m.lastProvider++
		added = true
	}
	for _, cfg := range cfgs {
		add(cfg)
	}
	if !added {
		// Add an empty target group to force the refresh of the corresponding
		// scrape pool and to notify the receiver that this target set has no
		// current targets.
		// It can happen because the combined set of SD configurations is empty
		// or because we fail to instantiate all the SD configurations.
		add(StaticConfig{{}})
	}
	return failed
}

// StaticProvider holds a list of target groups that never change.
type StaticProvider struct {
	TargetGroups []*targetgroup.Group
}

// Run implements the Worker interface.
func (sd *StaticProvider) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// We still have to consider that the consumer exits right away in which case
	// the context will be canceled.
	select {
	case ch <- sd.TargetGroups:
	case <-ctx.Done():
	}
	close(ch)
}
