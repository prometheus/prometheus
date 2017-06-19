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
	"net"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/floatingips"
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
	openstackLabelPrefix         = model.MetaLabelPrefix + "openstack_"
	openstackLabelInstanceID     = openstackLabelPrefix + "instance_id"
	openstackLabelInstanceName   = openstackLabelPrefix + "instance_name"
	openstackLabelInstanceStatus = openstackLabelPrefix + "instance_status"
	openstackLabelInstanceFlavor = openstackLabelPrefix + "instance_flavor"
	openstackLabelPublicIP       = openstackLabelPrefix + "public_ip"
	openstackLabelPrivateIP      = openstackLabelPrefix + "private_ip"
	openstackLabelTagPrefix      = openstackLabelPrefix + "tag_"
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
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
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

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

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
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	provider, err := openstack.AuthenticatedClient(*d.authOpts)

	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack session: %s", err)
	}
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: d.region,
	})

	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack compute session: %s", err)
	}

	opts := servers.ListOpts{}
	pager := servers.List(client, opts)

	tg = &config.TargetGroup{
		Source: fmt.Sprintf("OS_%s", d.region),
	}

	pagerFIP := floatingips.List(client)
	floatingIPList := make(map[string][]string)

	err = pagerFIP.EachPage(func(page pagination.Page) (bool, error) {
		result, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			log.Warn(err)
		}
		for _, ip := range result {
			// Skip not associated ips
			if ip.InstanceID != "" {
				floatingIPList[ip.InstanceID] = append(floatingIPList[ip.InstanceID], ip.IP)
			}
		}
		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("could not describe floating IPs: %s", err)
	}

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
				if !ok {
					log.Warn("Invalid type for address, expected array")
					continue
				}

				if len(md) == 0 {
					log.Debugf("Got no IP address for instance %s", s.ID)
					continue
				}

				md1, ok := md[0].(map[string]interface{})
				if !ok {
					log.Warn("Invalid type for address, expected dict")
					continue
				}

				addr, ok := md1["addr"].(string)
				if !ok {
					log.Warn("Invalid type for address, expected string")
					continue
				}

				labels[openstackLabelPrivateIP] = model.LabelValue(addr)

				addr = net.JoinHostPort(addr, fmt.Sprintf("%d", d.port))

				labels[model.AddressLabel] = model.LabelValue(addr)

				// Only use first private IP
				break
			}

			if val, ok := floatingIPList[s.ID]; ok {
				if len(val) > 0 {
					labels[openstackLabelPublicIP] = model.LabelValue(val[0])
				}
			}

			labels[openstackLabelInstanceStatus] = model.LabelValue(s.Status)
			labels[openstackLabelInstanceName] = model.LabelValue(s.Name)
			id, ok := s.Flavor["id"].(string)
			if !ok {
				log.Warn("Invalid type for instance id, excepted string")
				continue
			}
			labels[openstackLabelInstanceFlavor] = model.LabelValue(id)

			for k, v := range s.Metadata {
				name := strutil.SanitizeLabelName(k)
				labels[openstackLabelTagPrefix+model.LabelName(name)] = model.LabelValue(v)
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
