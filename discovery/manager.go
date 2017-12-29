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
	"sort"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/prometheus/config"
	sd_config "github.com/prometheus/prometheus/discovery/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"github.com/prometheus/prometheus/discovery/azure"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/dns"
	"github.com/prometheus/prometheus/discovery/ec2"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/gce"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/marathon"
	"github.com/prometheus/prometheus/discovery/openstack"
	"github.com/prometheus/prometheus/discovery/triton"
	"github.com/prometheus/prometheus/discovery/zookeeper"
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

type poolKey struct {
	setName  string
	provider string
}

// byProvider implements sort.Interface for []poolKey based on the provider field.
// Sorting is needed so that we can have predictable tests.
type byProvider []poolKey

func (a byProvider) Len() int           { return len(a) }
func (a byProvider) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byProvider) Less(i, j int) bool { return a[i].provider < a[j].provider }

// NewManager is the Discovery Manager constructor
func NewManager(logger log.Logger) *Manager {
	return &Manager{
		logger:         logger,
		actionCh:       make(chan func(context.Context)),
		syncCh:         make(chan map[string][]*targetgroup.Group),
		targets:        make(map[poolKey][]*targetgroup.Group),
		discoverCancel: []context.CancelFunc{},
	}
}

// Manager maintains a set of discovery providers and sends each update to a channel used by other packages.
type Manager struct {
	logger         log.Logger
	actionCh       chan func(context.Context)
	discoverCancel []context.CancelFunc
	targets        map[poolKey][]*targetgroup.Group
	// The sync channels sends the updates in map[targetSetName] where targetSetName is the job value from the scrape config.
	syncCh chan map[string][]*targetgroup.Group
}

// Run starts the background processing
func (m *Manager) Run(ctx context.Context) error {
	for {
		select {
		case f := <-m.actionCh:
			f(ctx)
		case <-ctx.Done():
			m.cancelDiscoverers()
			return ctx.Err()
		}
	}
}

// SyncCh returns a read only channel used by all Discoverers to send target updates.
func (m *Manager) SyncCh() <-chan map[string][]*targetgroup.Group {
	return m.syncCh
}

// ApplyConfig removes all running discovery providers and starts new ones using the provided config.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	err := make(chan error)
	m.actionCh <- func(ctx context.Context) {
		m.cancelDiscoverers()
		for _, scfg := range cfg.ScrapeConfigs {
			for provName, prov := range m.providersFromConfig(scfg.ServiceDiscoveryConfig) {
				m.startProvider(ctx, poolKey{setName: scfg.JobName, provider: provName}, prov)
			}
		}
		close(err)
	}

	return <-err
}

func (m *Manager) startProvider(ctx context.Context, poolKey poolKey, worker Discoverer) {
	ctx, cancel := context.WithCancel(ctx)
	updates := make(chan []*targetgroup.Group)

	m.discoverCancel = append(m.discoverCancel, cancel)

	go worker.Run(ctx, updates)
	go m.runProvider(ctx, poolKey, updates)
}

func (m *Manager) runProvider(ctx context.Context, poolKey poolKey, updates chan []*targetgroup.Group) {
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
			m.addGroup(poolKey, tgs)
			m.syncCh <- m.allGroups(poolKey)
		}
	}
}

func (m *Manager) cancelDiscoverers() {
	for _, c := range m.discoverCancel {
		c()
	}
	m.targets = make(map[poolKey][]*targetgroup.Group)
	m.discoverCancel = nil
}

func (m *Manager) addGroup(poolKey poolKey, tg []*targetgroup.Group) {
	done := make(chan struct{})

	m.actionCh <- func(ctx context.Context) {
		if tg != nil {
			m.targets[poolKey] = tg
		}
		close(done)

	}
	<-done
}

func (m *Manager) allGroups(pk poolKey) map[string][]*targetgroup.Group {
	tSets := make(chan map[string][]*targetgroup.Group)

	m.actionCh <- func(ctx context.Context) {

		// Sorting by the poolKey is needed so that we can have predictable tests.
		var pKeys []poolKey
		for pk := range m.targets {
			pKeys = append(pKeys, pk)
		}
		sort.Sort(byProvider(pKeys))

		tSetsAll := map[string][]*targetgroup.Group{}
		for _, pk := range pKeys {
			for _, tg := range m.targets[pk] {
				if tg.Source != "" { // Don't add empty targets.
					tSetsAll[pk.setName] = append(tSetsAll[pk.setName], tg)
				}
			}
		}
		tSets <- tSetsAll
	}
	return <-tSets

}

func (m *Manager) providersFromConfig(cfg sd_config.ServiceDiscoveryConfig) map[string]Discoverer {
	providers := map[string]Discoverer{}

	app := func(mech string, i int, tp Discoverer) {
		providers[fmt.Sprintf("%s/%d", mech, i)] = tp
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, dns.NewDiscovery(*c, log.With(m.logger, "discovery", "dns")))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, file.NewDiscovery(c, log.With(m.logger, "discovery", "file")))
	}
	for i, c := range cfg.ConsulSDConfigs {
		k, err := consul.NewDiscovery(c, log.With(m.logger, "discovery", "consul"))
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot create Consul discovery", "err", err)
			continue
		}
		app("consul", i, k)
	}
	for i, c := range cfg.MarathonSDConfigs {
		t, err := marathon.NewDiscovery(*c, log.With(m.logger, "discovery", "marathon"))
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot create Marathon discovery", "err", err)
			continue
		}
		app("marathon", i, t)
	}
	for i, c := range cfg.KubernetesSDConfigs {
		k, err := kubernetes.New(log.With(m.logger, "discovery", "k8s"), c)
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot create Kubernetes discovery", "err", err)
			continue
		}
		app("kubernetes", i, k)
	}
	for i, c := range cfg.ServersetSDConfigs {
		app("serverset", i, zookeeper.NewServersetDiscovery(c, log.With(m.logger, "discovery", "zookeeper")))
	}
	for i, c := range cfg.NerveSDConfigs {
		app("nerve", i, zookeeper.NewNerveDiscovery(c, log.With(m.logger, "discovery", "nerve")))
	}
	for i, c := range cfg.EC2SDConfigs {
		app("ec2", i, ec2.NewDiscovery(c, log.With(m.logger, "discovery", "ec2")))
	}
	for i, c := range cfg.OpenstackSDConfigs {
		openstackd, err := openstack.NewDiscovery(c, log.With(m.logger, "discovery", "openstack"))
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot initialize OpenStack discovery", "err", err)
			continue
		}
		app("openstack", i, openstackd)
	}

	for i, c := range cfg.GCESDConfigs {
		gced, err := gce.NewDiscovery(*c, log.With(m.logger, "discovery", "gce"))
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot initialize GCE discovery", "err", err)
			continue
		}
		app("gce", i, gced)
	}
	for i, c := range cfg.AzureSDConfigs {
		app("azure", i, azure.NewDiscovery(c, log.With(m.logger, "discovery", "azure")))
	}
	for i, c := range cfg.TritonSDConfigs {
		t, err := triton.New(log.With(m.logger, "discovery", "trition"), c)
		if err != nil {
			level.Error(m.logger).Log("msg", "Cannot create Triton discovery", "err", err)
			continue
		}
		app("triton", i, t)
	}
	if len(cfg.StaticConfigs) > 0 {
		app("static", 0, NewStaticProvider(cfg.StaticConfigs))
	}

	return providers
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
