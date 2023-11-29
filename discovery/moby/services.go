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

package moby

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
)

func (d *Discovery) refreshServices(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DockerSwarm",
	}

	services, err := d.client.ServiceList(ctx, types.ServiceListOptions{Filters: d.filters})
	if err != nil {
		return nil, fmt.Errorf("error while listing swarm services: %w", err)
	}

	networkLabels, err := getNetworksLabels(ctx, d.client, swarmLabel)
	if err != nil {
		return nil, fmt.Errorf("error while computing swarm network labels: %w", err)
	}

	for _, s := range services {
		commonLabels := map[string]string{
			swarmLabelServiceID:                    s.ID,
			swarmLabelServiceName:                  s.Spec.Name,
			swarmLabelServiceTaskContainerHostname: s.Spec.TaskTemplate.ContainerSpec.Hostname,
			swarmLabelServiceTaskContainerImage:    s.Spec.TaskTemplate.ContainerSpec.Image,
		}
		commonLabels[swarmLabelServiceMode] = getServiceValueMode(s)

		if s.UpdateStatus != nil {
			commonLabels[swarmLabelServiceUpdatingStatus] = string(s.UpdateStatus.State)
		}

		for k, v := range s.Spec.Labels {
			ln := strutil.SanitizeLabelName(k)
			commonLabels[swarmLabelServiceLabelPrefix+ln] = v
		}

		for _, p := range s.Endpoint.VirtualIPs {
			var added bool
			ip, _, err := net.ParseCIDR(p.Addr)
			if err != nil {
				return nil, fmt.Errorf("error while parsing address %s: %w", p.Addr, err)
			}

			for _, e := range s.Endpoint.Ports {
				if e.Protocol != swarm.PortConfigProtocolTCP {
					continue
				}
				labels := model.LabelSet{
					swarmLabelServiceEndpointPortName:        model.LabelValue(e.Name),
					swarmLabelServiceEndpointPortPublishMode: model.LabelValue(e.PublishMode),
				}

				for k, v := range commonLabels {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				for k, v := range networkLabels[p.NetworkID] {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				addr := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(e.PublishedPort), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)

				tg.Targets = append(tg.Targets, labels)
				added = true
			}

			if !added {
				labels := model.LabelSet{}

				for k, v := range commonLabels {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				for k, v := range networkLabels[p.NetworkID] {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				addr := net.JoinHostPort(ip.String(), strconv.Itoa(d.port))
				labels[model.AddressLabel] = model.LabelValue(addr)

				tg.Targets = append(tg.Targets, labels)
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}

func (d *Discovery) getServicesLabelsAndPorts(ctx context.Context) (map[string]map[string]string, map[string][]swarm.PortConfig, error) {
	services, err := d.client.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, nil, err
	}
	servicesLabels := make(map[string]map[string]string, len(services))
	servicesPorts := make(map[string][]swarm.PortConfig, len(services))
	for _, s := range services {
		servicesLabels[s.ID] = map[string]string{
			swarmLabelServiceID:   s.ID,
			swarmLabelServiceName: s.Spec.Name,
		}

		servicesLabels[s.ID][swarmLabelServiceMode] = getServiceValueMode(s)

		for k, v := range s.Spec.Labels {
			ln := strutil.SanitizeLabelName(k)
			servicesLabels[s.ID][swarmLabelServiceLabelPrefix+ln] = v
		}
		servicesPorts[s.ID] = s.Endpoint.Ports
	}

	return servicesLabels, servicesPorts, nil
}

func getServiceValueMode(s swarm.Service) string {
	if s.Spec.Mode.Global != nil {
		return "global"
	}
	if s.Spec.Mode.Replicated != nil {
		return "replicated"
	}
	return ""
}
