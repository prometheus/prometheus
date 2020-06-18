// Copyright 2020 The Prometheus Authors
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

package dockerswarm

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	swarmLabelServicePrefix                   = swarmLabel + "service_"
	swarmLabelServiceEndpointPortName         = swarmLabelServicePrefix + "endpoint_port_name"
	swarmLabelServiceEndpointPortPublishMode  = swarmLabelServicePrefix + "endpoint_port_publish_mode"
	swarmLabelServiceID                       = swarmLabelServicePrefix + "id"
	swarmLabelServiceLabelPrefix              = swarmLabelServicePrefix + "label_"
	swarmLabelServiceName                     = swarmLabelServicePrefix + "name"
	swarmLabelServiceUpdatingStatus           = swarmLabelServicePrefix + "updating_status"
	swarmLabelServiceTaskPrefix               = swarmLabelServicePrefix + "task_"
	swarmLabelServiceTaskContainerImage       = swarmLabelServiceTaskPrefix + "container_image"
	swarmLabelServiceTaskContainerHostname    = swarmLabelServiceTaskPrefix + "container_hostname"
	swarmLabelServiceTaskContainerLabelPrefix = swarmLabelServiceTaskPrefix + "container_label_"
	swarmLabelServiceVirtualIPAddrNetmask     = swarmLabelServicePrefix + "virtual_ip_address_netmask"
	swarmLabelServiceVirtualIPNetworkID       = swarmLabelServicePrefix + "virtual_ip_network_id"
)

func (d *Discovery) refreshServices(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DockerSwarm",
	}

	services, err := d.client.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while listing swarm services: %w", err)
	}

	for _, s := range services {
		for _, e := range s.Endpoint.Ports {
			if e.Protocol != swarm.PortConfigProtocolTCP {
				continue
			}
			for _, p := range s.Endpoint.VirtualIPs {
				labels := model.LabelSet{
					swarmLabelServiceID:                      model.LabelValue(s.ID),
					swarmLabelServiceName:                    model.LabelValue(s.Spec.Name),
					swarmLabelServiceEndpointPortName:        model.LabelValue(e.Name),
					swarmLabelServiceEndpointPortPublishMode: model.LabelValue(e.PublishMode),
					swarmLabelServiceVirtualIPAddrNetmask:    model.LabelValue(p.Addr),
					swarmLabelServiceVirtualIPNetworkID:      model.LabelValue(p.NetworkID),
					swarmLabelServiceTaskContainerImage:      model.LabelValue(s.Spec.TaskTemplate.ContainerSpec.Image),
					swarmLabelServiceTaskContainerHostname:   model.LabelValue(s.Spec.TaskTemplate.ContainerSpec.Hostname),
				}

				if s.UpdateStatus != nil {
					labels[swarmLabelServiceUpdatingStatus] = model.LabelValue(s.UpdateStatus.State)
				}
				for k, v := range s.Spec.Labels {
					ln := strutil.SanitizeLabelName(k)
					labels[model.LabelName(swarmLabelServiceLabelPrefix+ln)] = model.LabelValue(v)
				}
				for k, v := range s.Spec.TaskTemplate.ContainerSpec.Labels {
					ln := strutil.SanitizeLabelName(k)
					labels[model.LabelName(swarmLabelServiceTaskContainerLabelPrefix+ln)] = model.LabelValue(v)
				}

				ip, _, err := net.ParseCIDR(p.Addr)
				if err != nil {
					return nil, fmt.Errorf("error while parsing address %s: %w", p.Addr, err)
				}
				addr := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(e.PublishedPort), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)

				tg.Targets = append(tg.Targets, labels)
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}
