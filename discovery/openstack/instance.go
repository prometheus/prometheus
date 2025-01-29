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
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/utils"
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

type SupportedFeatures struct {
	flavorOriginalName bool // Available from 2.47
}

func getSupportedFeatures(minMinor, minMajor, maxMinor, maxMajor int) SupportedFeatures {
	features := SupportedFeatures{}

	// Feature: 'flavor.original_name' is supported from microversion 2.47 onwards
	if maxMajor > 2 || (maxMajor == 2 && maxMinor >= 47) {
		features.flavorOriginalName = true
	}

	return features
}

func setClientMicroversion(sc *gophercloud.ServiceClient, microversion string) *gophercloud.ServiceClient {
	microversioned := *sc
	microversioned.Microversion = microversion
	return &microversioned
}

type floatingIPKey struct {
	deviceID string
	fixed    string
}

func (i *InstanceDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := openstack.Authenticate(ctx, i.provider, *i.authOpts)
	if err != nil {
		return nil, fmt.Errorf("could not authenticate to OpenStack: %w", err)
	}

	client, err := openstack.NewComputeV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack compute session: %w", err)
	}

	supportedVersions, err := utils.GetSupportedMicroversions(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch supported microversions: %w", err)
	}

	// Determine available features
	features := getSupportedFeatures(supportedVersions.MinMinor, supportedVersions.MinMajor, supportedVersions.MaxMinor, supportedVersions.MaxMajor)

	minMicroversion := fmt.Sprintf("%d.%d", supportedVersions.MaxMajor, supportedVersions.MaxMinor)
	client = setClientMicroversion(client, minMicroversion)

	maxMicroversion := fmt.Sprintf("%d.%d", supportedVersions.MaxMajor, supportedVersions.MaxMinor)
	clientLatest := setClientMicroversion(client, maxMicroversion)

	networkClient, err := openstack.NewNetworkV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack network session: %w", err)
	}

	// OpenStack API reference
	// https://docs.openstack.org/api-ref/network/v2/index.html#list-ports
	portPages, err := ports.List(networkClient, ports.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list all ports: %w", err)
	}

	allPorts, err := ports.ExtractPorts(portPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Ports: %w", err)
	}

	portList := make(map[string]string)
	for _, port := range allPorts {
		portList[port.ID] = port.DeviceID
	}

	// OpenStack API reference
	// https://docs.openstack.org/api-ref/network/v2/index.html#list-floating-ips
	pagerFIP := floatingips.List(networkClient, floatingips.ListOpts{})
	floatingIPList := make(map[floatingIPKey]string)
	floatingIPPresent := make(map[string]struct{})
	err = pagerFIP.EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		result, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, fmt.Errorf("could not extract floatingips: %w", err)
		}
		for _, ip := range result {
			// Skip not associated ips
			if ip.PortID == "" || ip.FixedIP == "" {
				continue
			}

			// Fetch deviceID from portList
			deviceID, ok := portList[ip.PortID]
			if !ok {
				i.logger.Warn("Floating IP PortID not found in portList", "PortID", ip.PortID)
				continue
			}

			key := floatingIPKey{
				deviceID: deviceID,
				fixed:    ip.FixedIP,
			}

			floatingIPList[key] = ip.FloatingIP
			floatingIPPresent[ip.FloatingIP] = struct{}{}
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

	var latestServers []servers.Server
	if features.flavorOriginalName {
		// Fetch the server list using the client with the latest microversion
		allPages, err := servers.List(clientLatest, opts).AllPages(ctx)
		if err != nil {
			i.logger.Warn("Failed to fetch server list with latest microversion, falling back to flavor.id")
		} else {
			latestServers, err = servers.ExtractServers(allPages)
			if err != nil {
				i.logger.Warn("Failed to extract servers with latest microversion, falling back to flavor.id")
			}
		}
	}
	latestServerMap := make(map[string]servers.Server)
	if len(latestServers) > 0 {
		for _, server := range latestServers {
			latestServerMap[server.ID] = server
		}
	}

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

			// Check if we have enriched data for this server in the latestServerMap
			if features.flavorOriginalName {
				if latestServer, ok := latestServerMap[s.ID]; ok {
					flavorName, nameOk := latestServer.Flavor["original_name"].(string)
					if nameOk {
						labels[openstackLabelInstanceFlavor] = model.LabelValue(flavorName)
					} else {
						i.logger.Warn("flavor.original_name not found for server, falling back to flavor.id", "instance", s.ID)
					}
				}
			}

			// Fallback to using flavor.id from the original server list if flavor.original_name wasn't set
			if _, exists := labels[openstackLabelInstanceFlavor]; !exists {
				flavorID, idOk := s.Flavor["id"].(string)
				if idOk {
					labels[openstackLabelInstanceFlavor] = model.LabelValue(flavorID)
				} else {
					i.logger.Warn("Invalid type for both flavor.original_name and flavor.id in server, expected string", "instance", s.ID)
				}
			}

			imageID, ok := s.Image["id"].(string)
			if ok {
				labels[openstackLabelInstanceImage] = model.LabelValue(imageID)
			}

			for k, v := range s.Metadata {
				name := strutil.SanitizeLabelName(k)
				labels[openstackLabelTagPrefix+model.LabelName(name)] = model.LabelValue(v)
			}
			for pool, address := range s.Addresses {
				md, ok := address.([]interface{})
				if !ok {
					i.logger.Warn("Invalid type for address, expected array")
					continue
				}
				if len(md) == 0 {
					i.logger.Debug("Got no IP address", "instance", s.ID)
					continue
				}
				for _, address := range md {
					md1, ok := address.(map[string]interface{})
					if !ok {
						i.logger.Warn("Invalid type for address, expected dict")
						continue
					}
					addr, ok := md1["addr"].(string)
					if !ok {
						i.logger.Warn("Invalid type for address, expected string")
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
					if val, ok := floatingIPList[floatingIPKey{deviceID: s.ID, fixed: addr}]; ok {
						lbls[openstackLabelPublicIP] = model.LabelValue(val)
					}
					addr = net.JoinHostPort(addr, strconv.Itoa(i.port))
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
