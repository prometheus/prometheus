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
	swarmLabelTaskPrefix           = swarmLabel + "task_"
	swarmLabelTaskID               = swarmLabelTaskPrefix + "id"
	swarmLabelTaskDesiredState     = swarmLabelTaskPrefix + "desired_state"
	swarmLabelTaskStatus           = swarmLabelTaskPrefix + "state"
	swarmLabelTaskContainerID      = swarmLabelTaskPrefix + "container_id"
	swarmLabelTaskSlot             = swarmLabelTaskPrefix + "slot"
	swarmLabelTaskPortMode         = swarmLabelTaskPrefix + "port_publish_mode"
	swarmLabelContainerLabelPrefix = swarmLabel + "container_label_"
)

func (d *Discovery) refreshTasks(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DockerSwarm",
	}

	tasks, err := d.client.TaskList(ctx, types.TaskListOptions{Filters: d.filters})
	if err != nil {
		return nil, fmt.Errorf("error while listing swarm services: %w", err)
	}

	serviceLabels, servicePorts, err := d.getServicesLabelsAndPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while computing services labels and ports: %w", err)
	}

	nodeLabels, err := d.getNodesLabels(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while computing nodes labels and ports: %w", err)
	}

	networkLabels, err := getNetworksLabels(ctx, d.client, swarmLabel)
	if err != nil {
		return nil, fmt.Errorf("error while computing swarm network labels: %w", err)
	}

	for _, s := range tasks {
		commonLabels := map[string]string{
			swarmLabelTaskID:           s.ID,
			swarmLabelTaskDesiredState: string(s.DesiredState),
			swarmLabelTaskStatus:       string(s.Status.State),
			swarmLabelTaskSlot:         strconv.FormatInt(int64(s.Slot), 10),
		}

		if s.Status.ContainerStatus != nil {
			commonLabels[swarmLabelTaskContainerID] = s.Status.ContainerStatus.ContainerID
		}

		if s.Spec.ContainerSpec != nil {
			for k, v := range s.Spec.ContainerSpec.Labels {
				ln := strutil.SanitizeLabelName(k)
				commonLabels[swarmLabelContainerLabelPrefix+ln] = v
			}
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
				swarmLabelTaskPortMode: model.LabelValue(p.PublishMode),
			}

			for k, v := range commonLabels {
				labels[model.LabelName(k)] = model.LabelValue(v)
			}

			addr := net.JoinHostPort(string(labels[swarmLabelNodeAddress]), strconv.FormatUint(uint64(p.PublishedPort), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)
			tg.Targets = append(tg.Targets, labels)
		}

		for _, network := range s.NetworksAttachments {
			for _, address := range network.Addresses {
				var added bool

				ip, _, err := net.ParseCIDR(address)
				if err != nil {
					return nil, fmt.Errorf("error while parsing address %s: %w", address, err)
				}

				for _, p := range servicePorts[s.ServiceID] {
					if p.Protocol != swarm.PortConfigProtocolTCP {
						continue
					}
					labels := model.LabelSet{
						swarmLabelTaskPortMode: model.LabelValue(p.PublishMode),
					}

					for k, v := range commonLabels {
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					for k, v := range networkLabels[network.Network.ID] {
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					addr := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(p.PublishedPort), 10))
					labels[model.AddressLabel] = model.LabelValue(addr)

					tg.Targets = append(tg.Targets, labels)
					added = true
				}
				if !added {
					labels := model.LabelSet{}

					for k, v := range commonLabels {
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					for k, v := range networkLabels[network.Network.ID] {
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					addr := net.JoinHostPort(ip.String(), strconv.Itoa(d.port))
					labels[model.AddressLabel] = model.LabelValue(addr)

					tg.Targets = append(tg.Targets, labels)
				}
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}
