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
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	openstackLabelLoadBalancerID                 = openstackLabelPrefix + "loadbalancer_id"
	openstackLabelLoadBalancerName               = openstackLabelPrefix + "loadbalancer_name"
	openstackLabelLoadBalancerOperatingStatus    = openstackLabelPrefix + "loadbalancer_operating_status"
	openstackLabelLoadBalancerProvisioningStatus = openstackLabelPrefix + "loadbalancer_provisioning_status"
	openstackLabelLoadBalancerAvailabilityZone   = openstackLabelPrefix + "loadbalancer_availability_zone"
	openstackLabelLoadBalancerFloatingIP         = openstackLabelPrefix + "loadbalancer_floating_ip"
	openstackLabelLoadBalancerVIP                = openstackLabelPrefix + "loadbalancer_vip"
	openstackLabelLoadBalancerProvider           = openstackLabelPrefix + "loadbalancer_provider"
	openstackLabelLoadBalancerTags               = openstackLabelPrefix + "loadbalancer_tags"
)

// LoadBalancerDiscovery discovers OpenStack load balancers.
type LoadBalancerDiscovery struct {
	provider     *gophercloud.ProviderClient
	authOpts     *gophercloud.AuthOptions
	region       string
	logger       *slog.Logger
	availability gophercloud.Availability
}

// NewLoadBalancerDiscovery returns a new loadbalancer discovery.
func newLoadBalancerDiscovery(provider *gophercloud.ProviderClient, opts *gophercloud.AuthOptions,
	region string, availability gophercloud.Availability, l *slog.Logger,
) *LoadBalancerDiscovery {
	if l == nil {
		l = promslog.NewNopLogger()
	}
	return &LoadBalancerDiscovery{
		provider: provider, authOpts: opts,
		region: region, availability: availability, logger: l,
	}
}

func (i *LoadBalancerDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := openstack.Authenticate(ctx, i.provider, *i.authOpts)
	if err != nil {
		return nil, fmt.Errorf("could not authenticate to OpenStack: %w", err)
	}

	client, err := openstack.NewLoadBalancerV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack load balancer session: %w", err)
	}

	networkClient, err := openstack.NewNetworkV2(i.provider, gophercloud.EndpointOpts{
		Region: i.region, Availability: i.availability,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create OpenStack network session: %w", err)
	}

	allPages, err := loadbalancers.List(client, loadbalancers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list load balancers: %w", err)
	}

	allLBs, err := loadbalancers.ExtractLoadBalancers(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract load balancers: %w", err)
	}

	// Fetch all listeners in one API call
	listenerPages, err := listeners.List(client, listeners.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list all listeners: %w", err)
	}

	allListeners, err := listeners.ExtractListeners(listenerPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract all listeners: %w", err)
	}

	// Create a map to group listeners by Load Balancer ID
	listenerMap := make(map[string][]listeners.Listener)
	for _, listener := range allListeners {
		// Iterate through each associated Load Balancer ID in the Loadbalancers array
		for _, lb := range listener.Loadbalancers {
			listenerMap[lb.ID] = append(listenerMap[lb.ID], listener)
		}
	}

	// Fetch all floating IPs
	fipPages, err := floatingips.List(networkClient, floatingips.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list all fips: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list floating IPs: %w", err)
	}

	allFIPs, err := floatingips.ExtractFloatingIPs(fipPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract floating IPs: %w", err)
	}

	// Create a map to associate floating IPs with their resource IDs
	fipMap := make(map[string]string) // Key: LoadBalancerID/PortID, Value: Floating IP
	for _, fip := range allFIPs {
		if fip.PortID != "" {
			fipMap[fip.PortID] = fip.FloatingIP
		}
	}

	tg := &targetgroup.Group{
		Source: "OS_" + i.region,
	}

	for _, lb := range allLBs {
		// Retrieve listeners for this load balancer from the map
		lbListeners, exists := listenerMap[lb.ID]
		if !exists || len(lbListeners) == 0 {
			i.logger.Debug("Got no listener", "loadbalancer", lb.ID)
			continue
		}

		// Variable to store the port of the first PROMETHEUS listener
		var listenerPort int
		hasPrometheusListener := false

		// Check if any listener has the PROMETHEUS protocol
		for _, listener := range lbListeners {
			if listener.Protocol == "PROMETHEUS" {
				hasPrometheusListener = true
				listenerPort = listener.ProtocolPort
				break
			}
		}

		// Skip LBs without PROMETHEUS listener protocol
		if !hasPrometheusListener {
			i.logger.Debug("Got no PROMETHEUS listener", "loadbalancer", lb.ID)
			continue
		}

		labels := model.LabelSet{}
		addr := net.JoinHostPort(lb.VipAddress, strconv.Itoa(listenerPort))
		labels[model.AddressLabel] = model.LabelValue(addr)
		labels[openstackLabelLoadBalancerID] = model.LabelValue(lb.ID)
		labels[openstackLabelLoadBalancerName] = model.LabelValue(lb.Name)
		labels[openstackLabelLoadBalancerOperatingStatus] = model.LabelValue(lb.OperatingStatus)
		labels[openstackLabelLoadBalancerProvisioningStatus] = model.LabelValue(lb.ProvisioningStatus)
		labels[openstackLabelLoadBalancerAvailabilityZone] = model.LabelValue(lb.AvailabilityZone)
		labels[openstackLabelLoadBalancerVIP] = model.LabelValue(lb.VipAddress)
		labels[openstackLabelLoadBalancerProvider] = model.LabelValue(lb.Provider)
		labels[openstackLabelProjectID] = model.LabelValue(lb.ProjectID)

		if len(lb.Tags) > 0 {
			labels[openstackLabelLoadBalancerTags] = model.LabelValue(strings.Join(lb.Tags, ","))
		}

		if floatingIP, exists := fipMap[lb.VipPortID]; exists {
			labels[openstackLabelLoadBalancerFloatingIP] = model.LabelValue(floatingIP)
		}

		tg.Targets = append(tg.Targets, labels)
	}

	if err != nil {
		return nil, err
	}

	return []*targetgroup.Group{tg}, nil
}
