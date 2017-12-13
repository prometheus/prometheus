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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
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
)

func init() {
	prometheus.MustRegister(refreshFailuresCount)
	prometheus.MustRegister(refreshDuration)
}

// Discovery periodically performs OpenStack-SD requests. It implements
// the TargetProvider interface.
type Discovery interface {
	Run(ctx context.Context, ch chan<- []*config.TargetGroup)
	refresh() (tg *config.TargetGroup, err error)
}

// NewDiscovery returns a new OpenStackDiscovery which periodically refreshes its targets.
func NewDiscovery(conf *config.OpenstackSDConfig, l log.Logger) (Discovery, error) {
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
	switch conf.Role {
	case config.OpenStackRoleHypervisor:
		hypervisor := NewHypervisorDiscovery(&opts,
			time.Duration(conf.RefreshInterval), conf.Port, conf.Region, l)
		return hypervisor, nil
	case config.OpenStackRoleInstance:
		instance := NewInstanceDiscovery(&opts,
			time.Duration(conf.RefreshInterval), conf.Port, conf.Region, l)
		return instance, nil
	default:
		return nil, errors.New("unknown OpenStack discovery role")
	}
}
