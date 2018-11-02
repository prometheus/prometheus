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
	"fmt"
	"net"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/floatingips"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	openstackLabelPrefix         = model.MetaLabelPrefix + "openstack_"
	openstackLabelAddressPool    = openstackLabelPrefix + "address_pool"
	openstackLabelInstanceID     = openstackLabelPrefix + "instance_id"
	openstackLabelInstanceName   = openstackLabelPrefix + "instance_name"
	openstackLabelInstanceStatus = openstackLabelPrefix + "instance_status"
	openstackLabelInstanceFlavor = openstackLabelPrefix + "instance_flavor"
	openstackLabelPublicIP       = openstackLabelPrefix + "public_ip"
	openstackLabelPrivateIP      = openstackLabelPrefix + "private_ip"
	openstackLabelTagPrefix      = openstackLabelPrefix + "tag_"
)

// InstanceDiscovery discovers OpenStack instances.
type InstanceDiscovery struct {
	provider   *gophercloud.ProviderClient
	authOpts   *gophercloud.AuthOptions
	region     string
	interval   time.Duration
	logger     log.Logger
	port       int
	allTenants bool
}

// NewInstanceDiscovery returns a new instance discovery.
func NewInstanceDiscovery(provider *gophercloud.ProviderClient, opts *gophercloud.AuthOptions,
	interval time.Duration, port int, region string, allTenants bool, l log.Logger) *InstanceDiscovery {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &InstanceDiscovery{provider: provider, authOpts: opts,
		region: region, interval: interval, port: port, allTenants: allTenants, logger: l}
}

// Run implements the Discoverer interface.
func (i *InstanceDiscovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Get an initial set right away.
	tg, err := i.refresh()
	if err != nil {
		level.Error(i.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
	} else {
		select {
		case ch <- []*targetgroup.Group{tg}:
		case <-ctx.Done():
			return
		}
	}

	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tg, err := i.refresh()
			if err != nil {
				level.Error(i.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
				continue
			}

			select {
			case ch <- []*targetgroup.Group{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

type floatingIPKey struct {
	id    string
	fixed string
}

func (i *InstanceDiscovery) refresh() (*targetgroup.Group, error) {
	var err error
	t0 := time.Now()
	defer func() {
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	err = openstack.Authenticate(i.provider, *i.authOpts)
	if err != nil {
		return nil, fmt.Errorf("could not authenticate to OpenStack: %s", err)
	}
	client, err := openstack.NewComputeV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack compute session: %s", err)
	}

	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-floating-ips
	pagerFIP := floatingips.List(client)
	floatingIPList := make(map[floatingIPKey]string)
	floatingIPPresent := make(map[string]struct{})
	err = pagerFIP.EachPage(func(page pagination.Page) (bool, error) {
		result, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, fmt.Errorf("could not extract floatingips: %s", err)
		}
		for _, ip := range result {
			// Skip not associated ips
			if ip.InstanceID == "" || ip.FixedIP == "" {
				continue
			}
			floatingIPList[floatingIPKey{id: ip.InstanceID, fixed: ip.FixedIP}] = ip.IP
			floatingIPPresent[ip.IP] = struct{}{}
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-servers
	opts := servers.ListOpts{
		AllTenants: i.allTenants,
	}
	pager := servers.List(client, opts)
	tg := &targetgroup.Group{
		Source: fmt.Sprintf("OS_" + i.region),
	}
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		instanceList, err := servers.ExtractServers(page)
		if err != nil {
			return false, fmt.Errorf("could not extract instances: %s", err)
		}

		for _, s := range instanceList {
			if len(s.Addresses) == 0 {
				level.Info(i.logger).Log("msg", "Got no IP address", "instance", s.ID)
				continue
			}

			labels := model.LabelSet{
				openstackLabelInstanceID:     model.LabelValue(s.ID),
				openstackLabelInstanceStatus: model.LabelValue(s.Status),
				openstackLabelInstanceName:   model.LabelValue(s.Name),
			}

			id, ok := s.Flavor["id"].(string)
			if !ok {
				level.Warn(i.logger).Log("msg", "Invalid type for flavor id, expected string")
				continue
			}
			labels[openstackLabelInstanceFlavor] = model.LabelValue(id)
			for k, v := range s.Metadata {
				name := strutil.SanitizeLabelName(k)
				labels[openstackLabelTagPrefix+model.LabelName(name)] = model.LabelValue(v)
			}
			for pool, address := range s.Addresses {
				md, ok := address.([]interface{})
				if !ok {
					level.Warn(i.logger).Log("msg", "Invalid type for address, expected array")
					continue
				}
				if len(md) == 0 {
					level.Debug(i.logger).Log("msg", "Got no IP address", "instance", s.ID)
					continue
				}
				for _, address := range md {
					md1, ok := address.(map[string]interface{})
					if !ok {
						level.Warn(i.logger).Log("msg", "Invalid type for address, expected dict")
						continue
					}
					addr, ok := md1["addr"].(string)
					if !ok {
						level.Warn(i.logger).Log("msg", "Invalid type for address, expected string")
						continue
					}
					if _, ok := floatingIPPresent[addr]; ok {
						continue
					}
					lbls := make(model.LabelSet, len(labels))
					for k, v := range labels {
						lbls[k] = v
					}
					lbls[openstackLabelAddressPool] = model.LabelValue(pool)
					lbls[openstackLabelPrivateIP] = model.LabelValue(addr)
					if val, ok := floatingIPList[floatingIPKey{id: s.ID, fixed: addr}]; ok {
						lbls[openstackLabelPublicIP] = model.LabelValue(val)
					}
					addr = net.JoinHostPort(addr, fmt.Sprintf("%d", i.port))
					lbls[model.AddressLabel] = model.LabelValue(addr)

					tg.Targets = append(tg.Targets, lbls)
				}
			}
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return tg, nil
}
