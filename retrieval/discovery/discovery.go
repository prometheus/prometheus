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

	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval/discovery/azure"
	"github.com/prometheus/prometheus/retrieval/discovery/consul"
	"github.com/prometheus/prometheus/retrieval/discovery/dns"
	"github.com/prometheus/prometheus/retrieval/discovery/ec2"
	"github.com/prometheus/prometheus/retrieval/discovery/file"
	"github.com/prometheus/prometheus/retrieval/discovery/gce"
	"github.com/prometheus/prometheus/retrieval/discovery/kubernetes"
	"github.com/prometheus/prometheus/retrieval/discovery/marathon"
	"github.com/prometheus/prometheus/retrieval/discovery/zookeeper"
	"golang.org/x/net/context"
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

// ProvidersFromConfig returns all TargetProviders configured in cfg.
func ProvidersFromConfig(cfg *config.ScrapeConfig) map[string]TargetProvider {
	providers := map[string]TargetProvider{}

	app := func(mech string, i int, tp TargetProvider) {
		providers[fmt.Sprintf("%s/%d", mech, i)] = tp
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, dns.NewDiscovery(c))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, file.NewDiscovery(c))
	}
	for i, c := range cfg.ConsulSDConfigs {
		k, err := consul.NewDiscovery(c)
		if err != nil {
			log.Errorf("Cannot create Consul discovery: %s", err)
			continue
		}
		app("consul", i, k)
	}
	for i, c := range cfg.MarathonSDConfigs {
		m, err := marathon.NewDiscovery(c)
		if err != nil {
			log.Errorf("Cannot create Marathon discovery: %s", err)
			continue
		}
		app("marathon", i, m)
	}
	for i, c := range cfg.KubernetesSDConfigs {
		k, err := kubernetes.New(log.Base(), c)
		if err != nil {
			log.Errorf("Cannot create Kubernetes discovery: %s", err)
			continue
		}
		app("kubernetes", i, k)
	}
	for i, c := range cfg.ServersetSDConfigs {
		app("serverset", i, zookeeper.NewServersetDiscovery(c))
	}
	for i, c := range cfg.NerveSDConfigs {
		app("nerve", i, zookeeper.NewNerveDiscovery(c))
	}
	for i, c := range cfg.EC2SDConfigs {
		app("ec2", i, ec2.NewDiscovery(c))
	}
	for i, c := range cfg.GCESDConfigs {
		gced, err := gce.NewDiscovery(c)
		if err != nil {
			log.Errorf("Cannot initialize GCE discovery: %s", err)
			continue
		}
		app("gce", i, gced)
	}
	for i, c := range cfg.AzureSDConfigs {
		app("azure", i, azure.NewDiscovery(c))
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
