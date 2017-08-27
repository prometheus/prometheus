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
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/config"
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
	"golang.org/x/net/context"
)

// A TargetProvider provides information about target groups. It maintains a set
// of sources from which TargetGroups can originate. Whenever a target provider
// detects a potential change, it sends the TargetGroup through its provided channel.
//
// The TargetProvider does not have to guarantee that an actual change happened.
// It does guarantee that it sends the new TargetGroup whenever a change happens.
//
// TargetProviders should initially send a full set of all discoverable TargetGroups.
type TargetProvider interface {
	// Run hands a channel to the target provider through which it can send
	// updated target groups.
	// Must returns if the context gets canceled. It should not close the update
	// channel on returning.
	Run(ctx context.Context, up chan<- []*config.TargetGroup)
}

// ProvidersFromConfig returns all TargetProviders configured in cfg.
func ProvidersFromConfig(cfg config.ServiceDiscoveryConfig, logger log.Logger) map[string]TargetProvider {
	providers := map[string]TargetProvider{}

	app := func(mech string, i int, tp TargetProvider) {
		providers[fmt.Sprintf("%s/%d", mech, i)] = tp
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, dns.NewDiscovery(c, logger))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, file.NewDiscovery(c, logger))
	}
	for i, c := range cfg.ConsulSDConfigs {
		k, err := consul.NewDiscovery(c, logger)
		if err != nil {
			logger.Errorf("Cannot create Consul discovery: %s", err)
			continue
		}
		app("consul", i, k)
	}
	for i, c := range cfg.MarathonSDConfigs {
		m, err := marathon.NewDiscovery(c, logger)
		if err != nil {
			logger.Errorf("Cannot create Marathon discovery: %s", err)
			continue
		}
		app("marathon", i, m)
	}
	for i, c := range cfg.KubernetesSDConfigs {
		k, err := kubernetes.New(logger, c)
		if err != nil {
			logger.Errorf("Cannot create Kubernetes discovery: %s", err)
			continue
		}
		app("kubernetes", i, k)
	}
	for i, c := range cfg.ServersetSDConfigs {
		app("serverset", i, zookeeper.NewServersetDiscovery(c, logger))
	}
	for i, c := range cfg.NerveSDConfigs {
		app("nerve", i, zookeeper.NewNerveDiscovery(c, logger))
	}
	for i, c := range cfg.EC2SDConfigs {
		app("ec2", i, ec2.NewDiscovery(c, logger))
	}
	for i, c := range cfg.OpenstackSDConfigs {
		openstackd, err := openstack.NewDiscovery(c)
		if err != nil {
			log.Errorf("Cannot initialize OpenStack discovery: %s", err)
			continue
		}
		app("openstack", i, openstackd)
	}

	for i, c := range cfg.GCESDConfigs {
		gced, err := gce.NewDiscovery(c, logger)
		if err != nil {
			logger.Errorf("Cannot initialize GCE discovery: %s", err)
			continue
		}
		app("gce", i, gced)
	}
	for i, c := range cfg.AzureSDConfigs {
		app("azure", i, azure.NewDiscovery(c, logger))
	}
	for i, c := range cfg.TritonSDConfigs {
		t, err := triton.New(logger.With("sd", "triton"), c)
		if err != nil {
			logger.Errorf("Cannot create Triton discovery: %s", err)
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

// Syncer receives updates complete sets of TargetGroups.
type Syncer interface {
	Sync([]*config.TargetGroup)
}

// TargetSet holds a set of target groups that is continously updated by target providers.
// The full set is flushed to a Syncer.
type TargetSet struct {
	providers map[string]TargetProvider
	syncer    Syncer
	syncCh    chan struct{}

	mtx    sync.Mutex
	groups map[string]*config.TargetGroup
}

// NewTargetSet returns a new target set backed by a set of named providers.
func NewTargetSet(s Syncer, ps map[string]TargetProvider) *TargetSet {
	return &TargetSet{
		syncer:    s,
		providers: ps,
		groups:    map[string]*config.TargetGroup{},
		syncCh:    make(chan struct{}),
	}
}

// Runner lets you run something.
type Runner interface {
	Run(context.Context)
}

// SwapRunner allows running a runner that can be swapped out with a new runner.
// The new runner is only started after the first one has terminated.
type SwapRunner struct {
	swapc chan Runner
}

func NewSwapRunner(r Runner) *SwapRunner {
	return &SwapRunner{swapc: make(chan Runner)}
}

func (sr *SwapRunner) Swap(r Runner) {
	sr.swapc <- r
}

func (sr *SwapRunner) Run(ctx context.Context) {
	var cancel func()
	var subCtx context.Context

	termc := make(chan struct{})

	for {
		select {
		case <-ctx.Done():
			return
		case r := <-sr.swapc:
			if cancel != nil {
				cancel()
				<-termc
			}

			subCtx, cancel = context.WithCancel(ctx)

			go func() {
				r.Run(subCtx)
				termc <- struct{}{}
			}()
		}
	}
}

// Run starts retrieving updates from target providers and flushes them to the syncer.
// It stops synchronization and returns when the context is canceled.
func (ts *TargetSet) Run(ctx context.Context) {
	// Start all target providers. We don't care about their completion before
	// returning ourselves.
	updates := make(chan []*config.TargetGroup)

	for name, p := range ts.providers {
		go p.Run(ctx, updates)
		go ts.handleUpdates(name, updates)
	}

	// We propagate changes to the syncer but throttle it to once every 10 seconds.
	for {
		select {
		case <-ctx.Done():
			return
		case <-ts.syncCh:
			ts.syncer.Sync(ts.list())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (ts *TargetSet) list() []*config.TargetGroup {
	ts.mtx.Lock()
	defer ts.mtx.Unlock()

	all := make([]*config.TargetGroup, 0, len(ts.groups))

	for _, tg := range ts.groups {
		all = append(all, tg)
	}
	return all
}

func (ts *TargetSet) handleUpdates(name string, updates <-chan []*config.TargetGroup) {
	for tgs := range updates {
		ts.mtx.Lock()

		for _, tg := range tgs {
			if tg == nil {
				continue
			}
			ts.groups[name+"/"+tg.Source] = tg
		}

		ts.mtx.Unlock()

		select {
		case ts.syncCh <- struct{}{}:
		default:
		}
	}
}
