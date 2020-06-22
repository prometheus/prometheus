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

	"github.com/docker/docker/api/types"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	swarmLabelNetworkPrefix      = swarmLabel + "network_"
	swarmLabelNetworkID          = swarmLabelNetworkPrefix + "id"
	swarmLabelNetworkName        = swarmLabelNetworkPrefix + "name"
	swarmLabelNetworkScope       = swarmLabelNetworkPrefix + "scope"
	swarmLabelNetworkInternal    = swarmLabelNetworkPrefix + "internal"
	swarmLabelNetworkIngress     = swarmLabelNetworkPrefix + "ingress"
	swarmLabelNetworkLabelPrefix = swarmLabelNetworkPrefix + "label_"
)

func (d *Discovery) getNetworksLabels(ctx context.Context) (map[string]map[string]string, error) {
	networks, err := d.client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	labels := make(map[string]map[string]string, len(networks))
	for _, network := range networks {
		labels[network.ID] = map[string]string{
			swarmLabelNetworkID:       network.ID,
			swarmLabelNetworkName:     network.Name,
			swarmLabelNetworkScope:    network.Scope,
			swarmLabelNetworkInternal: fmt.Sprintf("%t", network.Internal),
			swarmLabelNetworkIngress:  fmt.Sprintf("%t", network.Ingress),
		}
		for k, v := range network.Labels {
			ln := strutil.SanitizeLabelName(k)
			labels[network.ID][swarmLabelNetworkLabelPrefix+ln] = v
		}
	}

	return labels, nil
}
