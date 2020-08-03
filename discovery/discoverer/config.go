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

package discoverer

// TODO(abursavich): CLEANUP: this file's content will be deleted

import (
	"github.com/pkg/errors"

	"github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/ec2"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/zookeeper"
)

// ServiceDiscoveryConfig configures lists of different service discovery mechanisms.
type ServiceDiscoveryConfig struct {
	// List of file service discovery configurations.
	FileSDConfigs []*file.SDConfig `yaml:"file_sd_configs,omitempty"`
	// List of Consul service discovery configurations.
	ConsulSDConfigs []*consul.SDConfig `yaml:"consul_sd_configs,omitempty"`
	// List of Serverset service discovery configurations.
	ServersetSDConfigs []*zookeeper.ServersetSDConfig `yaml:"serverset_sd_configs,omitempty"`
	// NerveSDConfigs is a list of Nerve service discovery configurations.
	NerveSDConfigs []*zookeeper.NerveSDConfig `yaml:"nerve_sd_configs,omitempty"`
	// List of Kubernetes service discovery configurations.
	KubernetesSDConfigs []*kubernetes.SDConfig `yaml:"kubernetes_sd_configs,omitempty"`
	// List of EC2 service discovery configurations.
	EC2SDConfigs []*ec2.SDConfig `yaml:"ec2_sd_configs,omitempty"`

	// List of additional service discovery configurations.
	Configs []Config `yaml:"-"`
}

// SetDirectory joins any relative file paths with dir.
func (c *ServiceDiscoveryConfig) SetDirectory(dir string) {
	for _, c := range c.KubernetesSDConfigs {
		c.SetDirectory(dir)
	}
	for _, c := range c.ConsulSDConfigs {
		c.SetDirectory(dir)
	}
	for _, c := range c.FileSDConfigs {
		c.SetDirectory(dir)
	}
	for _, c := range c.Configs {
		if v, ok := c.(config.DirectorySetter); ok {
			v.SetDirectory(dir)
		}
	}
}

// Validate validates the ServiceDiscoveryConfig.
func (c *ServiceDiscoveryConfig) Validate() error {
	for _, cfg := range c.ConsulSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in consul_sd_configs")
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
	for _, cfg := range c.KubernetesSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in kubernetes_sd_configs")
		}
	}
	for _, cfg := range c.NerveSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in nerve_sd_configs")
		}
	}
	for _, cfg := range c.ServersetSDConfigs {
		if cfg == nil {
			return errors.New("empty or null section in serverset_sd_configs")
		}
	}
	return nil
}
