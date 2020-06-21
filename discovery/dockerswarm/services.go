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
	swarmLabelServicePrefix                  = swarmLabel + "service_"
	swarmLabelServiceEndpointPortName        = swarmLabelServicePrefix + "endpoint_port_name"
	swarmLabelServiceEndpointPortPublishMode = swarmLabelServicePrefix + "endpoint_port_publish_mode"
	swarmLabelServiceID                      = swarmLabelServicePrefix + "id"
	swarmLabelServiceLabelPrefix             = swarmLabelServicePrefix + "label_"
	swarmLabelServiceName                    = swarmLabelServicePrefix + "name"
	swarmLabelServiceMode                    = swarmLabelServicePrefix + "mode"
	swarmLabelServiceUpdatingStatus          = swarmLabelServicePrefix + "updating_status"
	swarmLabelServiceTaskPrefix              = swarmLabelServicePrefix + "task_"
	swarmLabelServiceTaskContainerImage      = swarmLabelServiceTaskPrefix + "container_image"
	swarmLabelServiceTaskContainerHostname   = swarmLabelServiceTaskPrefix + "container_hostname"
	swarmLabelServiceVirtualIPAddrNetmask    = swarmLabelServicePrefix + "virtual_ip_address_netmask"

	swarmLabelValueModeGlobal     = model.LabelValue("global")
	swarmLabelValueModeReplicated = model.LabelValue("replicated")
)

func (d *Discovery) refreshServices(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DockerSwarm",
	}

	services, err := d.client.ServiceList(ctx, types.ServiceListOptions{Filters: makeArgs(d.filters)})
	if err != nil {
		return nil, fmt.Errorf("error while listing swarm services: %w", err)
	}

	networkLabels := make(map[string]model.LabelSet, 1)

	for _, s := range services {
		for _, e := range s.Endpoint.Ports {
			if e.Protocol != swarm.PortConfigProtocolTCP {
				continue
			}
			for _, p := range s.Endpoint.VirtualIPs {
				labels := model.LabelSet{
					swarmLabelServiceEndpointPortName:        model.LabelValue(e.Name),
					swarmLabelServiceMode:                    getServiceValueMode(s),
					swarmLabelServiceEndpointPortPublishMode: model.LabelValue(e.PublishMode),
					swarmLabelServiceID:                      model.LabelValue(s.ID),
					swarmLabelServiceName:                    model.LabelValue(s.Spec.Name),
					swarmLabelServiceTaskContainerHostname:   model.LabelValue(s.Spec.TaskTemplate.ContainerSpec.Hostname),
					swarmLabelServiceTaskContainerImage:      model.LabelValue(s.Spec.TaskTemplate.ContainerSpec.Image),
					swarmLabelServiceVirtualIPAddrNetmask:    model.LabelValue(p.Addr),
				}

				if s.Spec.Mode.Global != nil {
					labels[swarmLabelServiceMode] = swarmLabelValueModeGlobal
				}
				if s.Spec.Mode.Replicated != nil {
					if s.Spec.Mode.Global != nil {
						if err != nil {
							return nil, fmt.Errorf("service %s is both global and replicated: %w", s.ID, err)
						}
					}
					labels[swarmLabelServiceMode] = swarmLabelValueModeReplicated
				}

				if s.UpdateStatus != nil {
					labels[swarmLabelServiceUpdatingStatus] = model.LabelValue(s.UpdateStatus.State)
				}

				for k, v := range s.Spec.Labels {
					ln := strutil.SanitizeLabelName(k)
					labels[model.LabelName(swarmLabelServiceLabelPrefix+ln)] = model.LabelValue(v)
				}

				ip, _, err := net.ParseCIDR(p.Addr)
				if err != nil {
					return nil, fmt.Errorf("error while parsing address %s: %w", p.Addr, err)
				}
				addr := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(e.PublishedPort), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)

				if _, ok := networkLabels[p.NetworkID]; !ok {
					lbs, err := d.getNetworkLabels(ctx, p.NetworkID)
					if err != nil {
						return nil, fmt.Errorf("error while inspecting network %s: %w", p.NetworkID, err)
					}
					networkLabels[p.NetworkID] = lbs
				}

				for k, v := range networkLabels[p.NetworkID] {
					labels[k] = v
				}

				tg.Targets = append(tg.Targets, labels)
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}

func (d *Discovery) getServiceLabels(ctx context.Context, serviceID string) (model.LabelSet, error) {
	s, _, err := d.client.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
	if err != nil {
		return nil, err
	}
	labels := model.LabelSet{
		swarmLabelServiceID:   model.LabelValue(s.ID),
		swarmLabelServiceName: model.LabelValue(s.Spec.Name),
		swarmLabelServiceMode: getServiceValueMode(s),
	}
	for k, v := range s.Spec.Labels {
		ln := strutil.SanitizeLabelName(k)
		labels[model.LabelName(swarmLabelServiceLabelPrefix+ln)] = model.LabelValue(v)
	}
	return labels, nil
}

func getServiceValueMode(s swarm.Service) model.LabelValue {
	if s.Spec.Mode.Global != nil {
		return swarmLabelValueModeGlobal
	}
	if s.Spec.Mode.Replicated != nil {
		return swarmLabelValueModeReplicated
	}
	return model.LabelValue("")
}
