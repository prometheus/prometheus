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
	"math/rand"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	failedConfigs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_sd_failed_configs",
			Help: "Current number of service discovery configurations that failed to load.",
		},
		[]string{"name"},
	)
	discoveredTargets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_sd_discovered_targets",
			Help: "Current number of discovered targets.",
		},
		[]string{"name", "config"},
	)
	receivedUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_received_updates_total",
			Help: "Total number of update events received from the SD providers.",
		},
		[]string{"name"},
	)
	delayedUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_updates_delayed_total",
			Help: "Total number of update events that couldn't be sent immediately.",
		},
		[]string{"name"},
	)
	sentUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_updates_total",
			Help: "Total number of update events sent to the SD consumers.",
		},
		[]string{"name"},
	)
)

func init() {
	prometheus.MustRegister(failedConfigs, discoveredTargets, receivedUpdates, delayedUpdates, sentUpdates)
}

type poolKey struct {
	setName  string
	provider string
}

// provider holds a Discoverer instance, its configuration, cancel func and its subscribers.
type provider struct {
	name   string
	c      context.CancelFunc
	d      Discoverer
	config interface{}

	mu   sync.RWMutex
	subs []string
}

func (p *provider) cancel() {
	if p.c != nil {
		p.c()
	}
}

func (p *provider) updateSubs(subs []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subs = subs
}

type keepProvider struct {
	p    *provider
	subs []string
}

// NewManager is the Discovery Manager constructor.
func NewManager(ctx context.Context, logger log.Logger, options ...func(*Manager)) *Manager {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	mgr := &Manager{
		logger:      logger,
		syncCh:      make(chan map[string][]*targetgroup.Group),
		targets:     make(map[poolKey]map[string]*targetgroup.Group),
		providers:   make(map[uint64]*provider),
		ctx:         ctx,
		updatert:    5 * time.Second,
		triggerSend: make(chan struct{}, 1),
		rnd:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, option := range options {
		option(mgr)
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

// Manager maintains a set of discovery providers and sends each update to a map channel.
// Targets are grouped by the target set name.
type Manager struct {
	logger log.Logger
	name   string
	mtx    sync.RWMutex
	ctx    context.Context

	// Some Discoverers(e.g. k8s) send only the updates for a given target group,
	// so we use map[tg.Source]*targetgroup.Group to know which group to update.
	targets map[poolKey]map[string]*targetgroup.Group

	// providers keeps track of SD providers. Provider's config hash is generally used as a map key.
	providers map[uint64]*provider
	// The sync channel sends the updates as a map where the key is the job value from the scrape config.
	syncCh chan map[string][]*targetgroup.Group

	// How long to wait before sending updates to the channel. The variable
	// should only be modified in unit tests.
	updatert time.Duration

	// The triggerSend channel signals to the manager that new updates have been received from providers.
	triggerSend chan struct{}

	// rnd stores a random number generator used for discovery provider naming.
	rnd *rand.Rand
}

// Run starts the background processing
func (m *Manager) Run() error {
	go m.sender()
	for range m.ctx.Done() {
		m.cancelDiscoverers()
		return m.ctx.Err()
	}
	return nil
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

	for pk := range m.targets {
		if _, ok := cfg[pk.setName]; !ok {
			discoveredTargets.DeleteLabelValues(m.name, pk.setName)
		}
	}
	var (
		failedCount     int
		keepProviders   = make(map[uint64]keepProvider)
		keepTargetPools = make(map[poolKey]struct{})
		oldProviders    = m.providers
	)
	m.providers = make(map[uint64]*provider)
	keep := func(p *provider, setName string, etag uint64) {
		if _, ok := keepProviders[etag]; !ok {
			keepProviders[etag] = keepProvider{
				p,
				[]string{},
			}
		}
		s := append(keepProviders[etag].subs, setName)
		keepProviders[etag] = keepProvider{
			p:    p,
			subs: s,
		}
		pk := poolKey{
			setName:  setName,
			provider: p.name,
		}
		keepTargetPools[pk] = struct{}{}
	}
	for setName, setCfg := range cfg {
		var added bool
		for _, c := range setCfg {
			etag, err := hashstructure.Hash(c, hashstructure.FormatV2, nil)
			if err != nil {
				return err
			}
			if p, ok := oldProviders[etag]; ok {
				keep(p, setName, etag)
				added = true
				continue
			}
			err = m.registerProvider(c, setName, etag)
			if err == nil {
				added = true
			} else {
				failedCount++
			}
			discoveredTargets.WithLabelValues(m.name, setName).Set(0)
		}
		if !added {
			// Add an empty target group to force the refresh of the corresponding
			// scrape pool and to notify the receiver that this target set has no
			// current targets.
			// It can happen because the combined set of SD configurations is empty
			// or because we fail to instantiate all the SD configurations.
			// Errors are purposefully ignored.
			etag, _ := hashstructure.Hash(setName, hashstructure.FormatV2, nil)
			_ = m.registerProvider(StaticConfig{{}}, setName, etag)
		}
	}
	failedConfigs.WithLabelValues(m.name).Set(float64(failedCount))
	// Providers gone from config should be canceled and their target pools wiped.
	for etag, p := range oldProviders {
		if _, ok := keepProviders[etag]; ok {
			continue
		}
		p.cancel()
		for _, s := range p.subs {
			delete(m.targets, poolKey{setName: s, provider: p.name}) // protected by m.mtx, already held at this point.
		}
	}
	// Delete those obsolete target pools still remaining.
	for pk := range m.targets {
		if _, ok := keepTargetPools[pk]; !ok {
			delete(m.targets, pk)
		}
	}
	// Start newly registered providers.
	for _, prov := range m.providers {
		m.startProvider(m.ctx, prov)
	}
	for etag, p := range keepProviders {
		p.p.updateSubs(p.subs)
		m.providers[etag] = p.p
	}
	// Currently downstream managers expect full target state upon config reload, so we must oblige.
	if len(keepProviders) > 0 {
		select {
		case m.triggerSend <- struct{}{}:
		default:
		}
	}
	return nil
}

// StartCustomProvider is used for sdtool. Only use this if you know what you're doing.
func (m *Manager) StartCustomProvider(ctx context.Context, name string, worker Discoverer) {
	p := &provider{
		name: name,
		d:    worker,
		subs: []string{name},
	}
	// At worst etag will be zero, that does not really hurt. So we ignore the error here.
	// Due to lack of config name is used to calculate etag.
	etag, _ := hashstructure.Hash(name, hashstructure.FormatV2, nil)
	m.providers[etag] = p
	m.startProvider(ctx, p)
}

func (m *Manager) startProvider(ctx context.Context, p *provider) {
	level.Debug(m.logger).Log("msg", "Starting provider", "provider", p.name, "subs", fmt.Sprintf("%v", p.subs))
	ctx, cancel := context.WithCancel(ctx)
	updates := make(chan []*targetgroup.Group)

	p.c = cancel

	go p.d.Run(ctx, updates)
	go m.updater(ctx, p, updates)
}

func (m *Manager) updater(ctx context.Context, p *provider, updates chan []*targetgroup.Group) {
	for {
		select {
		case <-ctx.Done():
			return
		case tgs, ok := <-updates:
			receivedUpdates.WithLabelValues(m.name).Inc()
			if !ok {
				level.Debug(m.logger).Log("msg", "Discoverer channel closed", "provider", p.name)
				return
			}

			p.mu.RLock()
			for _, s := range p.subs {
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
				sentUpdates.WithLabelValues(m.name).Inc()
				select {
				case m.syncCh <- m.allGroups():
				default:
					delayedUpdates.WithLabelValues(m.name).Inc()
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
	for _, p := range m.providers {
		p.cancel()
	}
}

func (m *Manager) updateGroup(poolKey poolKey, tgs []*targetgroup.Group) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

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
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	tSets := map[string][]*targetgroup.Group{}
	n := map[string]int{}
	for pkey, tsets := range m.targets {
		for _, tg := range tsets {
			// Even if the target group 'tg' is empty we still need to send it to the 'Scrape manager'
			// to signal that it needs to stop all scrape loops for this target set.
			tSets[pkey.setName] = append(tSets[pkey.setName], tg)
			n[pkey.setName] += len(tg.Targets)
		}
	}
	for setName, v := range n {
		discoveredTargets.WithLabelValues(m.name, setName).Set(float64(v))
	}
	return tSets
}

func (m *Manager) registerProvider(cfg Config, setName string, etag uint64) error {
	typ := cfg.Name()

	if p, ok := m.providers[etag]; ok {
		p.mu.Lock()
		p.subs = append(p.subs, setName)
		p.mu.Unlock()
		return nil
	}
	d, err := cfg.NewDiscoverer(DiscovererOptions{
		Logger: log.With(m.logger, "discovery", typ),
	})
	if err != nil {
		level.Error(m.logger).Log("msg", "Cannot create service discovery", "err", err, "type", typ)
		return err
	}
	m.providers[etag] = &provider{
		// Cryptographically secure random is an overkill here, those random values only serve informational purpose.
		name:   fmt.Sprintf("%s/%d", typ, m.rnd.Int63()),
		d:      d,
		config: cfg,
		subs:   []string{setName},
	}
	return nil
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
