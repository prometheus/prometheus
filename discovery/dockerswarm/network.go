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
	"github.com/prometheus/common/model"
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

func (d *Discovery) getNetworkLabels(ctx context.Context, networkID string) (model.LabelSet, error) {
	network, err := d.client.NetworkInspect(ctx, networkID, types.NetworkInspectOptions{})
	if err != nil {
		return nil, err
	}
	labels := model.LabelSet{
		swarmLabelNetworkID:       model.LabelValue(network.ID),
		swarmLabelNetworkName:     model.LabelValue(network.Name),
		swarmLabelNetworkScope:    model.LabelValue(network.Scope),
		swarmLabelNetworkInternal: model.LabelValue(fmt.Sprintf("%t", network.Internal)),
		swarmLabelNetworkIngress:  model.LabelValue(fmt.Sprintf("%t", network.Ingress)),
	}
	for k, v := range network.Labels {
		ln := strutil.SanitizeLabelName(k)
		labels[model.LabelName(swarmLabelNetworkLabelPrefix+ln)] = model.LabelValue(v)
	}
	return labels, nil
}
