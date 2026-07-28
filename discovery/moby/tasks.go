// Copyright The Prometheus Authors
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
	"maps"
	"net"
	"strconv"

	mobynetwork "github.com/moby/moby/api/types/network"
	mobyswarm "github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
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

	tasks, err := d.client.TaskList(ctx, client.TaskListOptions{Filters: d.filters})
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

	containerLabels := d.getContainerLabels(ctx, tasks.Items)

	for _, s := range tasks.Items {
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
			if s.Status.ContainerStatus != nil {
				for k, v := range containerLabels[s.Status.ContainerStatus.ContainerID] {
					commonLabels[k] = v
				}
			}
			// Then apply container spec labels (higher priority, may override image labels).
			for k, v := range s.Spec.ContainerSpec.Labels {
				ln := strutil.SanitizeLabelName(k)
				commonLabels[swarmLabelContainerLabelPrefix+ln] = v
			}
		}

		maps.Copy(commonLabels, serviceLabels[s.ServiceID])

		maps.Copy(commonLabels, nodeLabels[s.NodeID])

		for _, p := range s.Status.PortStatus.Ports {
			if p.Protocol != mobynetwork.TCP {
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

				ip, _, err := net.ParseCIDR(address.String())
				if err != nil {
					return nil, fmt.Errorf("error while parsing address %s: %w", address, err)
				}

				for _, p := range servicePorts[s.ServiceID] {
					if p.Protocol != mobynetwork.TCP {
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

// getContainerLabels fetches labels by inspecting the task container IDs.
// Errors fetching individual containers are skipped so transient state does not
// abort the full discovery cycle.
func (d *Discovery) getContainerLabels(ctx context.Context, tasks []mobyswarm.Task) map[string]map[string]string {
	labelsByContainerID := make(map[string]map[string]string)

	for _, t := range tasks {
		if t.Status.ContainerStatus == nil {
			continue
		}

		containerID := t.Status.ContainerStatus.ContainerID
		if containerID == "" {
			continue
		}
		if _, alreadyLoaded := labelsByContainerID[containerID]; alreadyLoaded {
			continue
		}

		c, err := d.client.ContainerInspect(ctx, containerID)
		if err != nil || c.Config == nil {
			continue
		}

		containerLabels := make(map[string]string, len(c.Config.Labels))
		for k, v := range c.Config.Labels {
			ln := strutil.SanitizeLabelName(k)
			containerLabels[swarmLabelContainerLabelPrefix+ln] = v
		}
		labelsByContainerID[containerID] = containerLabels
	}

	return labelsByContainerID
}
