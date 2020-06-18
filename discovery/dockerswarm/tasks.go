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
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	swarmLabelTaskPrefix       = swarmLabel + "task_"
	swarmLabelTaskID           = swarmLabelTaskPrefix + "id"
	swarmLabelTaskAddr         = swarmLabelTaskPrefix + "address_netmask"
	swarmLabelTaskLabelPrefix  = swarmLabelTaskPrefix + "label_"
	swarmLabelTaskServiceID    = swarmLabelTaskPrefix + "service_id"
	swarmLabelTaskNodeID       = swarmLabelTaskPrefix + "node_id"
	swarmLabelTaskDesiredState = swarmLabelTaskPrefix + "desired_state"
	swarmLabelTaskStatus       = swarmLabelTaskPrefix + "state"
	swarmLabelTaskContainerID  = swarmLabelTaskPrefix + "container_id"
	swarmLabelTaskSlot         = swarmLabelTaskPrefix + "slot"
)

func (d *Discovery) refreshTasks(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DockerSwarm",
	}

	tasks, err := d.client.TaskList(ctx, types.TaskListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while listing swarm services: %w", err)
	}

	for _, s := range tasks {
		for _, network := range s.NetworksAttachments {
			for _, address := range network.Addresses {
				labels := model.LabelSet{
					model.LabelName(swarmLabelTaskID):           model.LabelValue(s.ID),
					model.LabelName(swarmLabelTaskServiceID):    model.LabelValue(s.ServiceID),
					model.LabelName(swarmLabelTaskNodeID):       model.LabelValue(s.NodeID),
					model.LabelName(swarmLabelTaskDesiredState): model.LabelValue(s.DesiredState),
					model.LabelName(swarmLabelTaskStatus):       model.LabelValue(s.Status.State),
					model.LabelName(swarmLabelTaskSlot):         model.LabelValue(fmt.Sprintf("%v", s.Slot)),
				}

				if s.Status.ContainerStatus != nil {
					labels[model.LabelName(swarmLabelTaskContainerID)] = model.LabelValue(s.Status.ContainerStatus.ContainerID)
				}

				for k, v := range s.Labels {
					ln := strutil.SanitizeLabelName(k)
					labels[model.LabelName(swarmLabelTaskLabelPrefix+ln)] = model.LabelValue(v)
				}

				labels[model.LabelName(swarmLabelTaskAddr)] = model.LabelValue(address)
				ip, _, err := net.ParseCIDR(address)
				if err != nil {
					return nil, fmt.Errorf("error while parsing address %s: %w", address, err)
				}
				addr := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(d.port), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)
				tg.Targets = append(tg.Targets, labels)
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}
