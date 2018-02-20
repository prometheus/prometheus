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
	"sync"
	"time"

	"github.com/go-kit/kit/log"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Discoverer provides information about target groups. It maintains a set
// of sources from which TargetGroups can originate. Whenever a discovery provider
// detects a potential change, it sends the TargetGroup through its channel.
//
// Discoverer does not know if an actual change happened.
// It does guarantee that it sends the new TargetGroup whenever a change happens.
//
// Discoverers should initially send a full set of all discoverable TargetGroups.
type Discoverer interface {
	// Run hands a channel to the discovery provider(consul,dns etc) through which it can send
	// updated target groups.
	// Must returns if the context gets canceled. It should not close the update
	// channel on returning.
	Run(ctx context.Context, up chan<- []*targetgroup.Group)
}

// Key represents an unique key to identify the Discoverer groups
type Key struct {
	GroupName      string
	DiscovererName string
}

// NewManager is the Discovery Manager constructor
func NewManager(ctx context.Context, logger log.Logger) *Manager {
	return &Manager{
		logger:         logger,
		syncCh:         make(chan map[string][]*targetgroup.Group),
		targets:        make(map[Key]map[string]*targetgroup.Group),
		discoverers:    map[Key]Discoverer{},
		discoverCancel: []context.CancelFunc{},
		ctx:            ctx,
	}
}

// Manager maintains a set of discovery providers and sends each update to a map channel.
// Targets are grouped by the target set name.
type Manager struct {
	logger         log.Logger
	mtx            sync.RWMutex
	ctx            context.Context
	discoverers    map[Key]Discoverer
	discoverCancel []context.CancelFunc
	// Some Discoverers(eg. k8s) send only the updates for a given target group
	// so we use map[tg.Source]*targetgroup.Group to know which group to update.
	targets map[Key]map[string]*targetgroup.Group
	// The sync channels sends the updates in map[targetSetName] where targetSetName is the job value from the scrape config.
	syncCh chan map[string][]*targetgroup.Group
	// True if updates were received in the last 5 seconds.
	recentlyUpdated bool
	// Protects recentlyUpdated.
	recentlyUpdatedMtx sync.Mutex
}

// Run starts the background processing
func (m *Manager) Run() error {
	for {
		select {
		case <-m.ctx.Done():
			m.cancelDiscoverers()
			return m.ctx.Err()
		}
	}
}

// SyncCh returns a read only channel used by all Discoverers to send target updates.
func (m *Manager) SyncCh() <-chan map[string][]*targetgroup.Group {
	return m.syncCh
}

// Register re-registers all Discoverers.
func (m *Manager) Register(d map[Key]Discoverer) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.discoverers = make(map[Key]Discoverer)
	for k, v := range d {
		m.discoverers[k] = v
	}
}

// ApplyConfig removes all running discovery providers and starts new ones using the registered discoverers.
func (m *Manager) ApplyConfig() error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.cancelDiscoverers()
	for key, worker := range m.discoverers {
		m.startProvider(m.ctx, key, worker)
	}
	return nil
}

func (m *Manager) startProvider(ctx context.Context, key Key, worker Discoverer) {
	ctx, cancel := context.WithCancel(ctx)
	updates := make(chan []*targetgroup.Group)

	m.discoverCancel = append(m.discoverCancel, cancel)

	go worker.Run(ctx, updates)
	go m.runUpdater(ctx)
	go m.runProvider(ctx, key, updates)
}

func (m *Manager) runProvider(ctx context.Context, key Key, updates chan []*targetgroup.Group) {
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
			m.updateGroup(key, tgs)
			m.recentlyUpdatedMtx.Lock()
			m.recentlyUpdated = true
			m.recentlyUpdatedMtx.Unlock()
		}
	}
}

func (m *Manager) runUpdater(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.recentlyUpdatedMtx.Lock()
			if m.recentlyUpdated {
				m.syncCh <- m.allGroups()
				m.recentlyUpdated = false
			}
			m.recentlyUpdatedMtx.Unlock()
		}
	}
}

func (m *Manager) cancelDiscoverers() {
	for _, c := range m.discoverCancel {
		c()
	}
	m.targets = make(map[Key]map[string]*targetgroup.Group)
	m.discoverCancel = nil
}

func (m *Manager) updateGroup(key Key, tgs []*targetgroup.Group) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, tg := range tgs {
		if tg != nil { // Some Discoverers send nil target group so need to check for it to avoid panics.
			if _, ok := m.targets[key]; !ok {
				m.targets[key] = make(map[string]*targetgroup.Group)
			}
			m.targets[key][tg.Source] = tg
		}
	}
}

func (m *Manager) allGroups() map[string][]*targetgroup.Group {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	tSets := map[string][]*targetgroup.Group{}
	for pkey, tsets := range m.targets {
		for _, tg := range tsets {
			// Even if the target group 'tg' is empty we still need to send it to the 'Scrape manager'
			// to signal that it needs to stop all scrape loops for this target set.
			tSets[pkey.GroupName] = append(tSets[pkey.GroupName], tg)
		}
	}
	return tSets
}

// StaticProvider holds a list of target groups that never change.
type StaticProvider struct {
	TargetGroups []*targetgroup.Group
}

// NewStaticProvider returns a StaticProvider configured with the given
// target groups.
func NewStaticProvider(groups []*targetgroup.Group) *StaticProvider {
	for i, tg := range groups {
		tg.Source = fmt.Sprintf("%d", i)
	}
	return &StaticProvider{groups}
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
