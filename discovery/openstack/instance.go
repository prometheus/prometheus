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

// InstanceDiscovery discovers OpenStack instances.
type InstanceDiscovery struct {
	authOpts *gophercloud.AuthOptions
	region   string
	interval time.Duration
	logger   log.Logger
	port     int
}

// NewInstanceDiscovery returns a new instance discovery.
func NewInstanceDiscovery(opts *gophercloud.AuthOptions,
	interval time.Duration, port int, region string, l log.Logger) *InstanceDiscovery {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &InstanceDiscovery{authOpts: opts,
		region: region, interval: interval, port: port, logger: l}
}

// Run implements the TargetProvider interface.
func (i *InstanceDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Get an initial set right away.
	tg, err := i.refresh()
	if err != nil {
		level.Error(i.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
	} else {
		select {
		case ch <- []*config.TargetGroup{tg}:
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
			case ch <- []*config.TargetGroup{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (i *InstanceDiscovery) refresh() (*config.TargetGroup, error) {
	var err error
	t0 := time.Now()
	defer func() {
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	provider, err := openstack.AuthenticatedClient(*i.authOpts)
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack session: %s", err)
	}
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: i.region,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack compute session: %s", err)
	}

	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-floating-ips
	pagerFIP := floatingips.List(client)
	floatingIPList := make(map[string][]string)
	err = pagerFIP.EachPage(func(page pagination.Page) (bool, error) {
		result, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, fmt.Errorf("could not extract floatingips: %s", err)
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
		return nil, err
	}

	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-servers
	opts := servers.ListOpts{}
	pager := servers.List(client, opts)
	tg := &config.TargetGroup{
		Source: fmt.Sprintf("OS_" + i.region),
	}
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		instanceList, err := servers.ExtractServers(page)
		if err != nil {
			return false, fmt.Errorf("could not extract instances: %s", err)
		}

		for _, s := range instanceList {
			labels := model.LabelSet{
				openstackLabelInstanceID: model.LabelValue(s.ID),
			}
			if len(s.Addresses) == 0 {
				level.Info(i.logger).Log("msg", "Got no IP address", "instance", s.ID)
				continue
			}
			for _, address := range s.Addresses {
				md, ok := address.([]interface{})
				if !ok {
					level.Warn(i.logger).Log("msg", "Invalid type for address, expected array")
					continue
				}
				if len(md) == 0 {
					level.Debug(i.logger).Log("msg", "Got no IP address", "instance", s.ID)
					continue
				}
				md1, ok := md[0].(map[string]interface{})
				if !ok {
					level.Warn(i.logger).Log("msg", "Invalid type for address, expected dict")
					continue
				}
				addr, ok := md1["addr"].(string)
				if !ok {
					level.Warn(i.logger).Log("msg", "Invalid type for address, expected string")
					continue
				}
				labels[openstackLabelPrivateIP] = model.LabelValue(addr)
				addr = net.JoinHostPort(addr, fmt.Sprintf("%d", i.port))
				labels[model.AddressLabel] = model.LabelValue(addr)
				// Only use first private IP
				break
			}
			if val, ok := floatingIPList[s.ID]; ok && len(val) > 0 {
				labels[openstackLabelPublicIP] = model.LabelValue(val[0])
			}
			labels[openstackLabelInstanceStatus] = model.LabelValue(s.Status)
			labels[openstackLabelInstanceName] = model.LabelValue(s.Name)
			id, ok := s.Flavor["id"].(string)
			if !ok {
				level.Warn(i.logger).Log("msg", "Invalid type for instance id, excepted string")
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
		return nil, err
	}

	return tg, nil
}
