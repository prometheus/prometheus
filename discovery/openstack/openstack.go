// Copyright The Prometheus Authors
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

package openstack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/mwitkow/go-conntrack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// DefaultSDConfig is the default OpenStack SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
	Availability:    "public",
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for OpenStack based service discovery.
type SDConfig struct {
	IdentityEndpoint            string           `yaml:"identity_endpoint"`
	Username                    string           `yaml:"username"`
	UserID                      string           `yaml:"userid"`
	Password                    config.Secret    `yaml:"password"`
	ProjectName                 string           `yaml:"project_name"`
	ProjectID                   string           `yaml:"project_id"`
	DomainName                  string           `yaml:"domain_name"`
	DomainID                    string           `yaml:"domain_id"`
	ApplicationCredentialName   string           `yaml:"application_credential_name"`
	ApplicationCredentialID     string           `yaml:"application_credential_id"`
	ApplicationCredentialSecret config.Secret    `yaml:"application_credential_secret"`
	Role                        Role             `yaml:"role"`
	Region                      string           `yaml:"region"`
	RefreshInterval             model.Duration   `yaml:"refresh_interval"`
	Port                        int              `yaml:"port"`
	AllTenants                  bool             `yaml:"all_tenants,omitempty"`
	TLSConfig                   config.TLSConfig `yaml:"tls_config,omitempty"`
	Availability                string           `yaml:"availability,omitempty"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &openstackMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "openstack" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.TLSConfig.SetDirectory(dir)
}

// Role is the role of the target in OpenStack.
type Role string

// The valid options for OpenStackRole.
const (
	// OpenStack document reference
	// https://docs.openstack.org/nova/pike/admin/arch.html#hypervisors
	OpenStackRoleHypervisor Role = "hypervisor"
	// OpenStack document reference
	// https://docs.openstack.org/horizon/pike/user/launch-instances.html
	OpenStackRoleInstance Role = "instance"
	// Openstack document reference
	// https://docs.openstack.org/openstacksdk/rocky/user/resources/load_balancer/index.html
	OpenStackRoleLoadBalancer Role = "loadbalancer"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(any) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case OpenStackRoleHypervisor, OpenStackRoleInstance, OpenStackRoleLoadBalancer:
		return nil
	default:
		return fmt.Errorf("unknown OpenStack SD role %q", *c)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	switch c.Availability {
	case "public", "internal", "admin":
	default:
		return fmt.Errorf("unknown availability %s, must be one of admin, internal or public", c.Availability)
	}

	if c.Role == "" {
		return errors.New("role missing (one of: instance, hypervisor, loadbalancer)")
	}
	if c.Region == "" {
		return errors.New("openstack SD configuration requires a region")
	}

	return nil
}

type refresher interface {
	refresh(context.Context) ([]*targetgroup.Group, error)
}

// NewDiscovery returns a new OpenStack Discoverer which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*refresh.Discovery, error) {
	m, ok := opts.Metrics.(*openstackMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	r, err := newRefresher(conf, opts.Logger)
	if err != nil {
		return nil, err
	}
	return refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "openstack",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            r.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	), nil
}

func newRefresher(conf *SDConfig, l *slog.Logger) (refresher, error) {
	var opts gophercloud.AuthOptions
	if conf.IdentityEndpoint == "" {
		var err error
		opts, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, err
		}
	} else {
		opts = gophercloud.AuthOptions{
			IdentityEndpoint:            conf.IdentityEndpoint,
			Username:                    conf.Username,
			UserID:                      conf.UserID,
			Password:                    string(conf.Password),
			TenantName:                  conf.ProjectName,
			TenantID:                    conf.ProjectID,
			DomainName:                  conf.DomainName,
			DomainID:                    conf.DomainID,
			ApplicationCredentialID:     conf.ApplicationCredentialID,
			ApplicationCredentialName:   conf.ApplicationCredentialName,
			ApplicationCredentialSecret: string(conf.ApplicationCredentialSecret),
		}
	}
	client, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, err
	}
	tls, err := config.NewTLSConfig(&conf.TLSConfig)
	if err != nil {
		return nil, err
	}
	client.HTTPClient = http.Client{
		Transport: &http.Transport{
			IdleConnTimeout: 2 * time.Duration(conf.RefreshInterval),
			TLSClientConfig: tls,
			DialContext: conntrack.NewDialContextFunc(
				conntrack.DialWithTracing(),
				conntrack.DialWithName("openstack_sd"),
			),
		},
		Timeout: time.Duration(conf.RefreshInterval),
	}
	availability := gophercloud.Availability(conf.Availability)
	switch conf.Role {
	case OpenStackRoleHypervisor:
		return newHypervisorDiscovery(client, &opts, conf.Port, conf.Region, availability, l), nil
	case OpenStackRoleInstance:
		return newInstanceDiscovery(client, &opts, conf.Port, conf.Region, conf.AllTenants, availability, l), nil
	case OpenStackRoleLoadBalancer:
		return newLoadBalancerDiscovery(client, &opts, conf.Region, availability, l), nil
	}
	return nil, errors.New("unknown OpenStack discovery role")
}
