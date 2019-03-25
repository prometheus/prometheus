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

	"github.com/go-kit/kit/log"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/hypervisors"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	openstackLabelHypervisorHostIP   = openstackLabelPrefix + "hypervisor_host_ip"
	openstackLabelHypervisorHostName = openstackLabelPrefix + "hypervisor_hostname"
	openstackLabelHypervisorStatus   = openstackLabelPrefix + "hypervisor_status"
	openstackLabelHypervisorState    = openstackLabelPrefix + "hypervisor_state"
	openstackLabelHypervisorType     = openstackLabelPrefix + "hypervisor_type"
)

// HypervisorDiscovery discovers OpenStack hypervisors.
type HypervisorDiscovery struct {
	provider *gophercloud.ProviderClient
	authOpts *gophercloud.AuthOptions
	region   string
	logger   log.Logger
	port     int
}

// newHypervisorDiscovery returns a new hypervisor discovery.
func newHypervisorDiscovery(provider *gophercloud.ProviderClient, opts *gophercloud.AuthOptions,
	port int, region string, l log.Logger) *HypervisorDiscovery {
	return &HypervisorDiscovery{provider: provider, authOpts: opts,
		region: region, port: port, logger: l}
}

func (h *HypervisorDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	h.provider.Context = ctx
	err := openstack.Authenticate(h.provider, *h.authOpts)
	if err != nil {
		return nil, errors.Wrap(err, "could not authenticate to OpenStack")
	}
	client, err := openstack.NewComputeV2(h.provider, gophercloud.EndpointOpts{
		Region: h.region,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create OpenStack compute session")
	}

	tg := &targetgroup.Group{
		Source: fmt.Sprintf("OS_" + h.region),
	}
	// OpenStack API reference
	// https://developer.openstack.org/api-ref/compute/#list-hypervisors-details
	pagerHypervisors := hypervisors.List(client)
	err = pagerHypervisors.EachPage(func(page pagination.Page) (bool, error) {
		hypervisorList, err := hypervisors.ExtractHypervisors(page)
		if err != nil {
			return false, errors.Wrap(err, "could not extract hypervisors")
		}
		for _, hypervisor := range hypervisorList {
			labels := model.LabelSet{}
			addr := net.JoinHostPort(hypervisor.HostIP, fmt.Sprintf("%d", h.port))
			labels[model.AddressLabel] = model.LabelValue(addr)
			labels[openstackLabelHypervisorHostName] = model.LabelValue(hypervisor.HypervisorHostname)
			labels[openstackLabelHypervisorHostIP] = model.LabelValue(hypervisor.HostIP)
			labels[openstackLabelHypervisorStatus] = model.LabelValue(hypervisor.Status)
			labels[openstackLabelHypervisorState] = model.LabelValue(hypervisor.State)
			labels[openstackLabelHypervisorType] = model.LabelValue(hypervisor.HypervisorType)
			tg.Targets = append(tg.Targets, labels)
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return []*targetgroup.Group{tg}, nil
}
