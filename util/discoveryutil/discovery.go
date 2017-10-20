// Copyright 2017 The Prometheus Authors
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

package discoveryutil

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/prometheus/config"
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
	"github.com/prometheus/prometheus/discovery/triton"
	"github.com/prometheus/prometheus/discovery/zookeeper"
)

// ProvidersFromConfig returns all TargetProviders configured in cfg.
func ProvidersFromConfig(cfg config.ServiceDiscoveryConfig, logger log.Logger) map[string]discovery.TargetProvider {
	providers := map[string]discovery.TargetProvider{}

	app := func(mech string, i int, tp discovery.TargetProvider) {
		providers[fmt.Sprintf("%s/%d", mech, i)] = tp
	}

	for i, c := range cfg.DNSSDConfigs {
		app("dns", i, dns.NewDiscovery(c, log.With(logger, "discovery", "dns")))
	}
	for i, c := range cfg.FileSDConfigs {
		app("file", i, file.NewDiscovery(c, log.With(logger, "discovery", "file")))
	}
	for i, c := range cfg.ConsulSDConfigs {
		k, err := consul.NewDiscovery(c, log.With(logger, "discovery", "consul"))
		if err != nil {
			level.Error(logger).Log("msg", "Cannot create Consul discovery", "err", err)
			continue
		}
		app("consul", i, k)
	}
	for i, c := range cfg.MarathonSDConfigs {
		m, err := marathon.NewDiscovery(c, log.With(logger, "discovery", "marathon"))
		if err != nil {
			level.Error(logger).Log("msg", "Cannot create Marathon discovery", "err", err)
			continue
		}
		app("marathon", i, m)
	}
	for i, c := range cfg.KubernetesSDConfigs {
		k, err := kubernetes.New(log.With(logger, "discovery", "k8s"), c)
		if err != nil {
			level.Error(logger).Log("msg", "Cannot create Kubernetes discovery", "err", err)
			continue
		}
		app("kubernetes", i, k)
	}
	for i, c := range cfg.ServersetSDConfigs {
		app("serverset", i, zookeeper.NewServersetDiscovery(c, log.With(logger, "discovery", "zookeeper")))
	}
	for i, c := range cfg.NerveSDConfigs {
		app("nerve", i, zookeeper.NewNerveDiscovery(c, log.With(logger, "discovery", "nerve")))
	}
	for i, c := range cfg.EC2SDConfigs {
		app("ec2", i, ec2.NewDiscovery(c, log.With(logger, "discovery", "ec2")))
	}
	for i, c := range cfg.OpenstackSDConfigs {
		openstackd, err := openstack.NewDiscovery(c, log.With(logger, "discovery", "openstack"))
		if err != nil {
			level.Error(logger).Log("msg", "Cannot initialize OpenStack discovery", "err", err)
			continue
		}
		app("openstack", i, openstackd)
	}

	for i, c := range cfg.GCESDConfigs {
		gced, err := gce.NewDiscovery(c, log.With(logger, "discovery", "gce"))
		if err != nil {
			level.Error(logger).Log("msg", "Cannot initialize GCE discovery", "err", err)
			continue
		}
		app("gce", i, gced)
	}
	for i, c := range cfg.AzureSDConfigs {
		app("azure", i, azure.NewDiscovery(c, log.With(logger, "discovery", "azure")))
	}
	for i, c := range cfg.TritonSDConfigs {
		t, err := triton.New(log.With(logger, "discovery", "trition"), c)
		if err != nil {
			level.Error(logger).Log("msg", "Cannot create Triton discovery", "err", err)
			continue
		}
		app("triton", i, t)
	}
	if len(cfg.StaticConfigs) > 0 {
		app("static", 0, discovery.NewStaticProvider(cfg.StaticConfigs))
	}

	return providers
}
