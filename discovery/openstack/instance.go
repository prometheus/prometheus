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

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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
	openstackLabelInstanceFlavor = openstackLabelPrefix + "instance_flavor"
	openstackLabelInstanceID     = openstackLabelPrefix + "instance_id"
	openstackLabelInstanceImage  = openstackLabelPrefix + "instance_image"
	openstackLabelInstanceName   = openstackLabelPrefix + "instance_name"
	openstackLabelInstanceStatus = openstackLabelPrefix + "instance_status"
	openstackLabelPrivateIP      = openstackLabelPrefix + "private_ip"
	openstackLabelProjectID      = openstackLabelPrefix + "project_id"
	openstackLabelPublicIP       = openstackLabelPrefix + "public_ip"
	openstackLabelTagPrefix      = openstackLabelPrefix + "tag_"
	openstackLabelUserID         = openstackLabelPrefix + "user_id"
)

// InstanceDiscovery discovers OpenStack instances.
type InstanceDiscovery struct {
	provider     *gophercloud.ProviderClient
	authOpts     *gophercloud.AuthOptions
	region       string
	logger       log.Logger
	port         int
	allTenants   bool
	availability gophercloud.Availability
}

// NewInstanceDiscovery returns a new instance discovery.
func newInstanceDiscovery(provider *gophercloud.ProviderClient, opts *gophercloud.AuthOptions,
	port int, region string, allTenants bool, availability gophercloud.Availability, l log.Logger,
) *InstanceDiscovery {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &InstanceDiscovery{
		provider: provider, authOpts: opts,
		region: region, port: port, allTenants: allTenants, availability: availability, logger: l,
	}
}

type floatingIPKey struct {
	id    string
	fixed string
}

func (i *InstanceDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	i.provider.Context = ctx
	err := openstack.Authenticate(i.provider, *i.authOpts)
	if err != nil {
		return nil, fmt.Errorf("could not authenticate to OpenStack: %w", err)
	}

	client, err := openstack.NewComputeV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack compute session: %w", err)
	}

	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-floating-ips
	pagerFIP := floatingips.List(client)
	floatingIPList := make(map[floatingIPKey]string)
	floatingIPPresent := make(map[string]struct{})
	err = pagerFIP.EachPage(func(page pagination.Page) (bool, error) {
		result, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, fmt.Errorf("could not extract floatingips: %w", err)
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
		if ctx.Err() != nil {
			return false, fmt.Errorf("could not extract instances: %w", ctx.Err())
		}
		instanceList, err := servers.ExtractServers(page)
		if err != nil {
			return false, fmt.Errorf("could not extract instances: %w", err)
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
				openstackLabelProjectID:      model.LabelValue(s.TenantID),
				openstackLabelUserID:         model.LabelValue(s.UserID),
			}

			flavorId, ok := s.Flavor["id"].(string)
			if !ok {
				level.Warn(i.logger).Log("msg", "Invalid type for flavor id, expected string")
				continue
			}
			labels[openstackLabelInstanceFlavor] = model.LabelValue(flavorId)

			imageId, ok := s.Image["id"].(string)
			if ok {
				labels[openstackLabelInstanceImage] = model.LabelValue(imageId)
			}

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

	return []*targetgroup.Group{tg}, nil
}
