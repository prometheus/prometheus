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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/prometheus/prometheus/util/strutil"
)

const (
	labelNetworkPrefix      = "network_"
	labelNetworkID          = labelNetworkPrefix + "id"
	labelNetworkName        = labelNetworkPrefix + "name"
	labelNetworkScope       = labelNetworkPrefix + "scope"
	labelNetworkInternal    = labelNetworkPrefix + "internal"
	labelNetworkIngress     = labelNetworkPrefix + "ingress"
	labelNetworkLabelPrefix = labelNetworkPrefix + "label_"
)

func getNetworksLabels(ctx context.Context, client *client.Client, labelPrefix string) (map[string]map[string]string, error) {
	networks, err := client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	labels := make(map[string]map[string]string, len(networks))
	for _, network := range networks {
		labels[network.ID] = map[string]string{
			labelPrefix + labelNetworkID:       network.ID,
			labelPrefix + labelNetworkName:     network.Name,
			labelPrefix + labelNetworkScope:    network.Scope,
			labelPrefix + labelNetworkInternal: fmt.Sprintf("%t", network.Internal),
			labelPrefix + labelNetworkIngress:  fmt.Sprintf("%t", network.Ingress),
		}
		for k, v := range network.Labels {
			ln := strutil.SanitizeLabelName(k)
			labels[network.ID][labelPrefix+labelNetworkLabelPrefix+ln] = v
		}
	}

	return labels, nil
}
