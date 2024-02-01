// Copyright 2020 The Prometheus Authors
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

package hetzner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	hetznerLabelPrefix            = model.MetaLabelPrefix + "hetzner_"
	hetznerLabelRole              = hetznerLabelPrefix + "role"
	hetznerLabelServerID          = hetznerLabelPrefix + "server_id"
	hetznerLabelServerName        = hetznerLabelPrefix + "server_name"
	hetznerLabelServerStatus      = hetznerLabelPrefix + "server_status"
	hetznerLabelDatacenter        = hetznerLabelPrefix + "datacenter"
	hetznerLabelPublicIPv4        = hetznerLabelPrefix + "public_ipv4"
	hetznerLabelPublicIPv6Network = hetznerLabelPrefix + "public_ipv6_network"
)

// DefaultSDConfig is the default Hetzner SD configuration.
var DefaultSDConfig = SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for Hetzner based service discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
	Role            Role           `yaml:"role"`
	hcloudEndpoint  string         // For tests only.
	robotEndpoint   string         // For tests only.
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &hetznerMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "hetzner" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
}

type refresher interface {
	refresh(context.Context) ([]*targetgroup.Group, error)
}

// Role is the Role of the target within the Hetzner Ecosystem.
type Role string

// The valid options for role.
const (
	// Hetzner Robot Role (Dedicated Server)
	// https://robot.hetzner.com
	HetznerRoleRobot Role = "robot"
	// Hetzner Cloud Role
	// https://console.hetzner.cloud
	HetznerRoleHcloud Role = "hcloud"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case HetznerRoleRobot, HetznerRoleHcloud:
		return nil
	default:
		return fmt.Errorf("unknown role %q", *c)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if c.Role == "" {
		return errors.New("role missing (one of: robot, hcloud)")
	}
	return c.HTTPClientConfig.Validate()
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// Discovery periodically performs Hetzner requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (*refresh.Discovery, error) {
	m, ok := metrics.(*hetznerMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	r, err := newRefresher(conf, logger)
	if err != nil {
		return nil, err
	}

	return refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "hetzner",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            r.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	), nil
}

func newRefresher(conf *SDConfig, l log.Logger) (refresher, error) {
	switch conf.Role {
	case HetznerRoleHcloud:
		if conf.hcloudEndpoint == "" {
			conf.hcloudEndpoint = hcloud.Endpoint
		}
		return newHcloudDiscovery(conf, l)
	case HetznerRoleRobot:
		if conf.robotEndpoint == "" {
			conf.robotEndpoint = "https://robot-ws.your-server.de"
		}
		return newRobotDiscovery(conf, l)
	}
	return nil, errors.New("unknown Hetzner discovery role")
}
