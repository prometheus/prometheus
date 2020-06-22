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
	swarmLabelTaskPrefix       = swarmLabel + "task_"
	swarmLabelTaskID           = swarmLabelTaskPrefix + "id"
	swarmLabelTaskLabelPrefix  = swarmLabelTaskPrefix + "label_"
	swarmLabelTaskDesiredState = swarmLabelTaskPrefix + "desired_state"
	swarmLabelTaskStatus       = swarmLabelTaskPrefix + "state"
	swarmLabelTaskContainerID  = swarmLabelTaskPrefix + "container_id"
	swarmLabelTaskSlot         = swarmLabelTaskPrefix + "slot"
	swarmLabelTaskPortMode     = swarmLabelTaskPrefix + "port_publish_mode"
)

func (d *Discovery) refreshTasks(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DockerSwarm",
	}

	tasks, err := d.client.TaskList(ctx, types.TaskListOptions{Filters: makeArgs(d.filters)})
	if err != nil {
		return nil, fmt.Errorf("error while listing swarm services: %w", err)
	}

	networkLabels := make(map[string]map[string]string, 1)
	serviceLabels := make(map[string]map[string]string, 1)
	servicePorts := make(map[string][]swarm.PortConfig, 1)
	nodeLabels := make(map[string]map[string]string, 1)

	for _, s := range tasks {
		if _, ok := nodeLabels[s.NodeID]; !ok {
			lbs, err := d.getNodeLabels(ctx, s.NodeID)
			if err != nil {
				return nil, fmt.Errorf("error while inspecting service %s: %w", s.NodeID, err)
			}
			nodeLabels[s.NodeID] = lbs
		}

		if _, ok := serviceLabels[s.ServiceID]; !ok {
			lbs, ports, err := d.getServiceLabelsAndPorts(ctx, s.ServiceID)
			if err != nil {
				return nil, fmt.Errorf("error while inspecting service %s: %w", s.ServiceID, err)
			}
			serviceLabels[s.ServiceID] = lbs
			servicePorts[s.ServiceID] = ports
		}

		commonLabels := map[string]string{
			swarmLabelTaskID:           s.ID,
			swarmLabelTaskDesiredState: string(s.DesiredState),
			swarmLabelTaskStatus:       string(s.Status.State),
			swarmLabelTaskSlot:         strconv.FormatInt(int64(s.Slot), 10),
		}

		if s.Status.ContainerStatus != nil {
			commonLabels[swarmLabelTaskContainerID] = s.Status.ContainerStatus.ContainerID
		}

		for k, v := range s.Labels {
			ln := strutil.SanitizeLabelName(k)
			commonLabels[swarmLabelTaskLabelPrefix+ln] = v
		}

		for k, v := range serviceLabels[s.ServiceID] {
			commonLabels[k] = v
		}

		for k, v := range nodeLabels[s.NodeID] {
			commonLabels[k] = v
		}

		for _, p := range s.Status.PortStatus.Ports {
			if p.Protocol != swarm.PortConfigProtocolTCP {
				continue
			}

			labels := model.LabelSet{
				model.LabelName(swarmLabelTaskPortMode): model.LabelValue(p.PublishMode),
			}

			for k, v := range commonLabels {
				labels[model.LabelName(k)] = model.LabelValue(v)
			}

			addr := net.JoinHostPort(string(labels[swarmLabelNodeAddress]), strconv.FormatUint(uint64(p.PublishedPort), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)
			tg.Targets = append(tg.Targets, labels)
		}

		for _, p := range servicePorts[s.ServiceID] {
			if p.Protocol != swarm.PortConfigProtocolTCP {
				continue
			}
			for _, network := range s.NetworksAttachments {
				for _, address := range network.Addresses {
					labels := model.LabelSet{
						model.LabelName(swarmLabelTaskPortMode): model.LabelValue(p.PublishMode),
					}

					for k, v := range commonLabels {
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					if _, ok := networkLabels[network.Network.ID]; !ok {
						lbs, err := d.getNetworkLabels(ctx, network.Network.ID)
						if err != nil {
							return nil, fmt.Errorf("error while inspecting network %s: %w", network.Network.ID, err)
						}
						networkLabels[network.Network.ID] = lbs
					}

					for k, v := range networkLabels[network.Network.ID] {
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					ip, _, err := net.ParseCIDR(address)
					if err != nil {
						return nil, fmt.Errorf("error while parsing address %s: %w", address, err)
					}
					addr := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(p.PublishedPort), 10))
					labels[model.AddressLabel] = model.LabelValue(addr)

					tg.Targets = append(tg.Targets, labels)
				}
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}
