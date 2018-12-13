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

package config

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/azure"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/dns"
	"github.com/prometheus/prometheus/discovery/ec2"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/gce"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/marathon"
	"github.com/prometheus/prometheus/discovery/openstack"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/discovery/triton"
	"github.com/prometheus/prometheus/discovery/zookeeper"
)

var failedConfigs = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "prometheus_sd_configs_failed_total",
		Help: "Total number of service discovery configurations that failed to load.",
	},
	[]string{"name"},
)

func init() {
	prometheus.MustRegister(failedConfigs)
}

// ServiceDiscoveryConfig configures lists of different service discovery mechanisms.
type ServiceDiscoveryConfig struct {
	// List of labeled target groups for this job.
	StaticConfigs []*targetgroup.Group `yaml:"static_configs,omitempty"`
	// List of DNS service discovery configurations.
	DNSSDConfigs []*dns.SDConfig `yaml:"dns_sd_configs,omitempty"`
	// List of file service discovery configurations.
	FileSDConfigs []*file.SDConfig `yaml:"file_sd_configs,omitempty"`
	// List of Consul service discovery configurations.
	ConsulSDConfigs []*consul.SDConfig `yaml:"consul_sd_configs,omitempty"`
	// List of Serverset service discovery configurations.
	ServersetSDConfigs []*zookeeper.ServersetSDConfig `yaml:"serverset_sd_configs,omitempty"`
	// NerveSDConfigs is a list of Nerve service discovery configurations.
	NerveSDConfigs []*zookeeper.NerveSDConfig `yaml:"nerve_sd_configs,omitempty"`
	// MarathonSDConfigs is a list of Marathon service discovery configurations.
	MarathonSDConfigs []*marathon.SDConfig `yaml:"marathon_sd_configs,omitempty"`
	// List of Kubernetes service discovery configurations.
	KubernetesSDConfigs []*kubernetes.SDConfig `yaml:"kubernetes_sd_configs,omitempty"`
	// List of GCE service discovery configurations.
	GCESDConfigs []*gce.SDConfig `yaml:"gce_sd_configs,omitempty"`
	// List of EC2 service discovery configurations.
	EC2SDConfigs []*ec2.SDConfig `yaml:"ec2_sd_configs,omitempty"`
	// List of OpenStack service discovery configurations.
	OpenstackSDConfigs []*openstack.SDConfig `yaml:"openstack_sd_configs,omitempty"`
	// List of Azure service discovery configurations.
	AzureSDConfigs []*azure.SDConfig `yaml:"azure_sd_configs,omitempty"`
	// List of Triton service discovery configurations.
	TritonSDConfigs []*triton.SDConfig `yaml:"triton_sd_configs,omitempty"`
}

// Validate validates the ServiceDiscoveryConfig.
func (c *ServiceDiscoveryConfig) Validate() error {
	for _, cfg := range c.AzureSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in azure_sd_configs")
		}
	}
	for _, cfg := range c.ConsulSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in consul_sd_configs")
		}
	}
	for _, cfg := range c.DNSSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in dns_sd_configs")
		}
	}
	for _, cfg := range c.EC2SDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in ec2_sd_configs")
		}
	}
	for _, cfg := range c.FileSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in file_sd_configs")
		}
	}
	for _, cfg := range c.GCESDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in gce_sd_configs")
		}
	}
	for _, cfg := range c.KubernetesSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in kubernetes_sd_configs")
		}
	}
	for _, cfg := range c.MarathonSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in marathon_sd_configs")
		}
	}
	for _, cfg := range c.NerveSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in nerve_sd_configs")
		}
	}
	for _, cfg := range c.OpenstackSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in openstack_sd_configs")
		}
	}
	for _, cfg := range c.ServersetSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in serverset_sd_configs")
		}
	}
	for _, cfg := range c.StaticConfigs {
		if cfg == nil {
			return errors.New("empty or null section in static_configs")
		}
	}
	return nil
}

// ProviderSet represents a set of deduplicated SD providers.
type ProviderSet struct {
	logger    log.Logger
	name      string
	providers []*provider
}

// NewProviderSet creates an empty ProviderSet.
func NewProviderSet(name string, logger log.Logger) *ProviderSet {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &ProviderSet{
		logger:    logger,
		name:      name,
		providers: make([]*provider, 0),
	}
}

// Providers returns a slice of the current providers.
func (p *ProviderSet) Providers() []discovery.Provider {
	providers := make([]discovery.Provider, len(p.providers))
	for i := range providers {
		providers[i] = p.providers[i]
	}
	return providers
}

// Add creates and adds SD providers from a service discovery configuration.
// Providers with identical configuration are coalesced together.
func (p *ProviderSet) Add(name string, cfg ServiceDiscoveryConfig) {
	var added bool
	add := func(name string, cfg interface{}, f func() (discovery.Discoverer, error)) {
		if p.register(name, cfg, f) != nil {
			return
		}
		added = true
	}
	for _, c := range cfg.DNSSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return dns.NewDiscovery(*c, log.With(p.logger, "discovery", "dns")), nil
		})
	}
	for _, c := range cfg.FileSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return file.NewDiscovery(c, log.With(p.logger, "discovery", "file")), nil
		})
	}
	for _, c := range cfg.ConsulSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return consul.NewDiscovery(c, log.With(p.logger, "discovery", "consul"))
		})
	}
	for _, c := range cfg.MarathonSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return marathon.NewDiscovery(*c, log.With(p.logger, "discovery", "marathon"))
		})
	}
	for _, c := range cfg.KubernetesSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return kubernetes.New(log.With(p.logger, "discovery", "k8s"), c)
		})
	}
	for _, c := range cfg.ServersetSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return zookeeper.NewServersetDiscovery(c, log.With(p.logger, "discovery", "zookeeper"))
		})
	}
	for _, c := range cfg.NerveSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return zookeeper.NewNerveDiscovery(c, log.With(p.logger, "discovery", "nerve"))
		})
	}
	for _, c := range cfg.EC2SDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return ec2.NewDiscovery(c, log.With(p.logger, "discovery", "ec2")), nil
		})
	}
	for _, c := range cfg.OpenstackSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return openstack.NewDiscovery(c, log.With(p.logger, "discovery", "openstack"))
		})
	}
	for _, c := range cfg.GCESDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return gce.NewDiscovery(*c, log.With(p.logger, "discovery", "gce"))
		})
	}
	for _, c := range cfg.AzureSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return azure.NewDiscovery(c, log.With(p.logger, "discovery", "azure")), nil
		})
	}
	for _, c := range cfg.TritonSDConfigs {
		add(name, c, func() (discovery.Discoverer, error) {
			return triton.New(log.With(p.logger, "discovery", "triton"), c)
		})
	}
	if len(cfg.StaticConfigs) > 0 {
		add(name, name, func() (discovery.Discoverer, error) {

			return &StaticDiscoverer{cfg.StaticConfigs}, nil
		})
	}
	if !added {
		// Add an empty target group to force the refresh of the corresponding
		// scrape pool and to notify the receiver that this target set has no
		// current targets.
		// It can happen because the combined set of SD configurations is empty
		// or because we fail to instantiate all the SD configurations.
		add(name, name, func() (discovery.Discoverer, error) {
			return &StaticDiscoverer{TargetGroups: []*targetgroup.Group{&targetgroup.Group{}}}, nil
		})
	}
}

func (p *ProviderSet) register(subscriber string, cfg interface{}, newDiscoverer func() (discovery.Discoverer, error)) error {
	t := reflect.TypeOf(cfg).String()
	for _, prov := range p.providers {
		if reflect.DeepEqual(cfg, prov.config) {
			prov.addSubscriber(subscriber)
			return nil
		}
	}

	d, err := newDiscoverer()
	if err != nil {
		level.Error(p.logger).Log("msg", "Cannot create service discovery", "err", err, "type", t)
		failedConfigs.WithLabelValues(p.name).Inc()
		return err
	}

	p.providers = append(p.providers, &provider{
		Discoverer: d,
		id:         fmt.Sprintf("%s/%d", t, len(p.providers)),
		config:     cfg,
		subs:       []string{subscriber},
	})
	return nil
}

type provider struct {
	discovery.Discoverer
	config interface{}
	id     string
	subs   []string
}

func (p *provider) String() string {
	return p.id
}

func (p *provider) Subscribers() []string {
	return p.subs
}

func (p *provider) addSubscriber(s string) {
	p.subs = append(p.subs, s)
}

// StaticDiscoverer holds a list of target groups that never change.
type StaticDiscoverer struct {
	TargetGroups []*targetgroup.Group
}

// Run implements the Discoverer interface.
func (s *StaticDiscoverer) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// We still have to consider that the consumer exits right away in which case
	// the context will be canceled.
	select {
	case ch <- s.TargetGroups:
	case <-ctx.Done():
	}
	close(ch)
}
