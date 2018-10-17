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

package openstack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/mwitkow/go-conntrack"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	refreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_openstack_refresh_failures_total",
			Help: "The number of OpenStack-SD scrape failures.",
		})
	refreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_openstack_refresh_duration_seconds",
			Help: "The duration of an OpenStack-SD refresh in seconds.",
		})
	// DefaultSDConfig is the default OpenStack SD configuration.
	DefaultSDConfig = SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

// SDConfig is the configuration for OpenStack based service discovery.
type SDConfig struct {
	IdentityEndpoint string                `yaml:"identity_endpoint"`
	Username         string                `yaml:"username"`
	UserID           string                `yaml:"userid"`
	Password         config_util.Secret    `yaml:"password"`
	ProjectName      string                `yaml:"project_name"`
	ProjectID        string                `yaml:"project_id"`
	DomainName       string                `yaml:"domain_name"`
	DomainID         string                `yaml:"domain_id"`
	Role             Role                  `yaml:"role"`
	Region           string                `yaml:"region"`
	RefreshInterval  model.Duration        `yaml:"refresh_interval,omitempty"`
	Port             int                   `yaml:"port"`
	AllTenants       bool                  `yaml:"all_tenants,omitempty"`
	TLSConfig        config_util.TLSConfig `yaml:"tls_config,omitempty"`
}

// OpenStackRole is role of the target in OpenStack.
type Role string

// The valid options for OpenStackRole.
const (
	// OpenStack document reference
	// https://docs.openstack.org/nova/pike/admin/arch.html#hypervisors
	OpenStackRoleHypervisor Role = "hypervisor"
	// OpenStack document reference
	// https://docs.openstack.org/horizon/pike/user/launch-instances.html
	OpenStackRoleInstance Role = "instance"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case OpenStackRoleHypervisor, OpenStackRoleInstance:
		return nil
	default:
		return fmt.Errorf("unknown OpenStack SD role %q", *c)
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
		return fmt.Errorf("role missing (one of: instance, hypervisor)")
	}
	if c.Region == "" {
		return fmt.Errorf("Openstack SD configuration requires a region")
	}
	return nil
}

func init() {
	prometheus.MustRegister(refreshFailuresCount)
	prometheus.MustRegister(refreshDuration)
}

// Discovery periodically performs OpenStack-SD requests. It implements
// the Discoverer interface.
type Discovery interface {
	Run(ctx context.Context, ch chan<- []*targetgroup.Group)
	refresh() (tg *targetgroup.Group, err error)
}

// NewDiscovery returns a new OpenStackDiscovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, l log.Logger) (Discovery, error) {
	var opts gophercloud.AuthOptions
	if conf.IdentityEndpoint == "" {
		var err error
		opts, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, err
		}
	} else {
		opts = gophercloud.AuthOptions{
			IdentityEndpoint: conf.IdentityEndpoint,
			Username:         conf.Username,
			UserID:           conf.UserID,
			Password:         string(conf.Password),
			TenantName:       conf.ProjectName,
			TenantID:         conf.ProjectID,
			DomainName:       conf.DomainName,
			DomainID:         conf.DomainID,
		}
	}
	client, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, err
	}
	tls, err := config_util.NewTLSConfig(&conf.TLSConfig)
	if err != nil {
		return nil, err
	}
	client.HTTPClient = http.Client{
		Transport: &http.Transport{
			IdleConnTimeout: 5 * time.Duration(conf.RefreshInterval),
			TLSClientConfig: tls,
			DialContext: conntrack.NewDialContextFunc(
				conntrack.DialWithTracing(),
				conntrack.DialWithName("openstack_sd"),
			),
		},
		Timeout: 5 * time.Duration(conf.RefreshInterval),
	}
	switch conf.Role {
	case OpenStackRoleHypervisor:
		hypervisor := NewHypervisorDiscovery(client, &opts,
			time.Duration(conf.RefreshInterval), conf.Port, conf.Region, l)
		return hypervisor, nil
	case OpenStackRoleInstance:
		instance := NewInstanceDiscovery(client, &opts,
			time.Duration(conf.RefreshInterval), conf.Port, conf.Region, conf.AllTenants, l)
		return instance, nil
	default:
		return nil, errors.New("unknown OpenStack discovery role")
	}
}
