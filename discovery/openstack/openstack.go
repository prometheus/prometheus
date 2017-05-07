// Copyright 2015 The Prometheus Authors
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
	"net"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	openstackLabel               = model.MetaLabelPrefix + "openstack_"
	openstackLabelInstanceID     = openstackLabel + "instance_id"
	openstackLabelInstanceName   = openstackLabel + "instance_name"
	openstackLabelInstanceState  = openstackLabel + "instance_state"
	openstackLabelInstanceFlavor = openstackLabel + "instance_flavor"
	openstackLabelTag            = openstackLabel + "tag_"
)

var (
	openstackSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_openstack_refresh_failures_total",
			Help: "The number of OpenStack-SD scrape failures.",
		})
	openstackSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_openstack_refresh_duration_seconds",
			Help: "The duration of a OpenStack-SD refresh in seconds.",
		})
)

func init() {
	prometheus.MustRegister(openstackSDRefreshFailuresCount)
	prometheus.MustRegister(openstackSDRefreshDuration)
}

// Discovery periodically performs OpenStack-SD requests. It implements
// the TargetProvider interface.
type Discovery struct {
	os       *gophercloud.AuthOptions
	region   string
	interval time.Duration
	port     int
}

// NewDiscovery returns a new OpenStackDiscovery which periodically refreshes its targets.
func NewDiscovery(conf *config.OpenstackSDConfig) (*Discovery, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: conf.IdentityEndpoint,
		Username:         conf.Username,
		UserID:           conf.UserID,
		Password:         conf.Password,
		TenantName:       conf.TenantName,
		TenantID:         conf.TenantID,
		DomainName:       conf.DomainName,
		DomainID:         conf.DomainID,
	}

	return &Discovery{
		os:       &opts,
		region:   conf.Region,
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		log.Error(err)
	} else {
		select {
		case ch <- []*config.TargetGroup{tg}:
		case <-ctx.Done():
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				log.Error(err)
				continue
			}

			select {
			case ch <- []*config.TargetGroup{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *config.TargetGroup, err error) {
	t0 := time.Now()
	defer func() {
		openstackSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			openstackSDRefreshFailuresCount.Inc()
		}
	}()
	provider, err := openstack.AuthenticatedClient(*d.os)

	if err != nil {
		return nil, fmt.Errorf("could not create openstack session: %s", err)
	}
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: d.region,
	})

	if err != nil {
		return nil, fmt.Errorf("could not create openstack compute session: %s", err)
	}

	opts := servers.ListOpts{}
	pager := servers.List(client, opts)

	tg = &config.TargetGroup{}

	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		serverList, err := servers.ExtractServers(page)
		if err != nil {
			return false, fmt.Errorf("could not extract servers: %s", err)
		}

		for _, s := range serverList {
			labels := model.LabelSet{
				openstackLabelInstanceID: model.LabelValue(s.ID),
			}

			for _, address := range s.Addresses {
				md, ok := address.([]interface{})

				if !ok || len(md) < 1 {
					continue
				}

				md1, ok := md[0].(map[string]interface{})
				if !ok {
					continue
				}

				addr := net.JoinHostPort(md1["addr"].(string), fmt.Sprintf("%d", d.port))

				labels[model.AddressLabel] = model.LabelValue(addr)
				break
			}

			labels[openstackLabelInstanceState] = model.LabelValue(s.Status)
			labels[openstackLabelInstanceName] = model.LabelValue(s.Name)
			labels[openstackLabelInstanceFlavor] = model.LabelValue(s.Flavor["id"].(string))

			for k, v := range s.Metadata {
				name := strutil.SanitizeLabelName(k)
				labels[openstackLabelTag+model.LabelName(name)] = model.LabelValue(v)
			}

			tg.Targets = append(tg.Targets, labels)
		}
		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("could not describe instances: %s", err)
	}

	return tg, nil
}
