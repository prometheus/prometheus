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
	"log/slog"
	"net"
	"strconv"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

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
	logger       *slog.Logger
	port         int
	allTenants   bool
	availability gophercloud.Availability
}

// NewInstanceDiscovery returns a new instance discovery.
func newInstanceDiscovery(provider *gophercloud.ProviderClient, opts *gophercloud.AuthOptions,
	port int, region string, allTenants bool, availability gophercloud.Availability, l *slog.Logger,
) *InstanceDiscovery {
	if l == nil {
		l = promslog.NewNopLogger()
	}
	return &InstanceDiscovery{
		provider: provider, authOpts: opts,
		region: region, port: port, allTenants: allTenants, availability: availability, logger: l,
	}
}

type floatingIPKey struct {
	portID string
	fixed  string
}

func (i *InstanceDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := openstack.Authenticate(ctx, i.provider, *i.authOpts)
	if err != nil {
		return nil, fmt.Errorf("could not authenticate to OpenStack: %w", err)
	}

	computeClient, err := openstack.NewComputeV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack compute session: %w", err)
	}
	networkClient, err := openstack.NewNetworkV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack networking session: %w", err)
	}

	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-floating-ips
	pagerFIP := floatingips.List(networkClient, nil)
	floatingIPList := make(map[floatingIPKey]string)
	floatingIPPresent := make(map[string]struct{})
	err = pagerFIP.EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		result, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, fmt.Errorf("could not extract floatingips: %w", err)
		}
		for _, fip := range result {
			// Skip not associated ips
			if fip.FixedIP == "" {
				continue
			}
			floatingIPList[floatingIPKey{portID: fip.PortID, fixed: fip.FixedIP}] = fip.FloatingIP
			floatingIPPresent[fip.FloatingIP] = struct{}{}
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
	pager := servers.List(computeClient, opts)
	tg := &targetgroup.Group{
		Source: "OS_" + i.region,
	}
	err = pager.EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		if ctx.Err() != nil {
			return false, fmt.Errorf("could not extract instances: %w", ctx.Err())
		}
		instanceList, err := servers.ExtractServers(page)
		if err != nil {
			return false, fmt.Errorf("could not extract instances: %w", err)
		}

		for _, s := range instanceList {
			if len(s.Addresses) == 0 {
				i.logger.Info("Got no IP address", "instance", s.ID)
				continue
			}

			labels := model.LabelSet{
				openstackLabelInstanceID:     model.LabelValue(s.ID),
				openstackLabelInstanceStatus: model.LabelValue(s.Status),
				openstackLabelInstanceName:   model.LabelValue(s.Name),
				openstackLabelProjectID:      model.LabelValue(s.TenantID),
				openstackLabelUserID:         model.LabelValue(s.UserID),
			}

			flavorName, nameOk := s.Flavor["original_name"].(string)
			// "original_name" is only available for microversion >= 2.47. It was added in favor of "id".
			if !nameOk {
				flavorID, idOk := s.Flavor["id"].(string)
				if !idOk {
					i.logger.Warn("Invalid type for both flavor original_name and flavor id, expected string")
					continue
				}
				labels[openstackLabelInstanceFlavor] = model.LabelValue(flavorID)
			} else {
				labels[openstackLabelInstanceFlavor] = model.LabelValue(flavorName)
			}

			imageID, ok := s.Image["id"].(string)
			if ok {
				labels[openstackLabelInstanceImage] = model.LabelValue(imageID)
			}

			for k, v := range s.Metadata {
				name := strutil.SanitizeLabelName(k)
				labels[openstackLabelTagPrefix+model.LabelName(name)] = model.LabelValue(v)
			}

			allPages, err := ports.List(networkClient, ports.ListOpts{DeviceID: s.ID}).AllPages(ctx)
			if err != nil {
				return false, fmt.Errorf("could not get instance ports: %w", err)
			}

			allPorts, err := ports.ExtractPorts(allPages)
			if err != nil {
				return false, fmt.Errorf("could not get instance ports: %w", err)
			}
			if len(allPorts) == 0 {
				i.logger.Debug("Got no IP address", "instance", s.ID)
				continue
			}

			for _, port := range allPorts {
				network, err := networks.Get(ctx, networkClient, port.NetworkID).Extract()
				if err != nil {
					return false, fmt.Errorf("could not get the network of the instance port: %w", err)
				}

				for _, fixedIP := range port.FixedIPs {
					if _, ok := floatingIPPresent[fixedIP.IPAddress]; ok {
						continue
					}
					lbls := make(model.LabelSet, len(labels))
					for k, v := range labels {
						lbls[k] = v
					}
					lbls[openstackLabelAddressPool] = model.LabelValue(network.Name)
					lbls[openstackLabelPrivateIP] = model.LabelValue(fixedIP.IPAddress)
					if val, ok := floatingIPList[floatingIPKey{portID: port.ID, fixed: fixedIP.IPAddress}]; ok {
						lbls[openstackLabelPublicIP] = model.LabelValue(val)
					}
					hostport := net.JoinHostPort(fixedIP.IPAddress, strconv.Itoa(i.port))
					lbls[model.AddressLabel] = model.LabelValue(hostport)

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
