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
	"github.com/prometheus/prometheus/discovery/azure"
	"github.com/prometheus/prometheus/discovery/collins"
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
	// List of Collins service discovery configurations.
	CollinsSDConfigs []*collins.SDConfig `yaml:"collins_sd_configs,omitempty"`
	// List of Triton service discovery configurations.
	TritonSDConfigs []*triton.SDConfig `yaml:"triton_sd_configs,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ServiceDiscoveryConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ServiceDiscoveryConfig
	return unmarshal((*plain)(c))
}
