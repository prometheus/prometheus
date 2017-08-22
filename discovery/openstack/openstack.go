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
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"

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
type Discovery struct {
	authOpts *gophercloud.AuthOptions
	region   string
	interval time.Duration
	port     int
	role     string
}

// NewDiscovery returns a new OpenStackDiscovery which periodically refreshes its targets.
func NewDiscovery(conf *config.OpenstackSDConfig) (*Discovery, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: conf.IdentityEndpoint,
		Username:         conf.Username,
		UserID:           conf.UserID,
		Password:         string(conf.Password),
		TenantName:       conf.ProjectName,
		TenantID:         conf.ProjectID,
		DomainName:       conf.DomainName,
		DomainID:         conf.DomainID,
	}

	return &Discovery{
		authOpts: &opts,
		region:   conf.Region,
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
		role:     conf.Role,
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	switch d.role {
	case "instance":
		instance := NewInstance(d.authOpts, d.interval, d.port, d.region)
		instance.Run(ctx, ch)
	case "host":
		host := NewHost(d.authOpts, d.interval, d.port, d.region)
		host.Run(ctx, ch)
	default:
		fmt.Errorf("unknown OpenStack discovery kind %q", d.role)
	}

	<-ctx.Done()
}
