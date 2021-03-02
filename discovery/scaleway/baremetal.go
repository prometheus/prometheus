// Copyright 2021 The Prometheus Authors
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

package scaleway

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/scaleway/scaleway-sdk-go/api/baremetal/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	baremetalLabelPrefix = metaLabelPrefix + "baremetal_"

	baremetalIDLabel        = baremetalLabelPrefix + "id"
	baremetalZoneLabel      = baremetalLabelPrefix + "zone"
	baremetalNameLabel      = baremetalLabelPrefix + "name"
	baremetalStatusLabel    = baremetalLabelPrefix + "status"
	baremetalTagsLabel      = baremetalLabelPrefix + "tags"
	baremetalProjectIDLabel = baremetalLabelPrefix + "project_id"
)

func (d *Discovery) listBaremetalServers(ctx context.Context) ([]model.LabelSet, error) {
	api := baremetal.NewAPI(d.client)

	req := &baremetal.ListServersRequest{
		Zone:      scw.Zone(d.zone),
		ProjectID: scw.StringPtr(d.project),
	}

	if d.name != "" {
		req.Name = scw.StringPtr(d.name)
	}

	if d.tags != nil {
		req.Tags = d.tags
	}

	servers, err := api.ListServers(req, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	var targets []model.LabelSet
	for _, server := range servers.Servers {
		labels := model.LabelSet{
			baremetalIDLabel:        model.LabelValue(server.ID),
			baremetalNameLabel:      model.LabelValue(server.Name),
			baremetalZoneLabel:      model.LabelValue(server.Zone.String()),
			baremetalStatusLabel:    model.LabelValue(server.Status),
			baremetalProjectIDLabel: model.LabelValue(server.ProjectID),
		}

		if len(server.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(server.Tags, separator) + separator
			labels[baremetalTagsLabel] = model.LabelValue(tags)
		}

		addr := net.JoinHostPort(server.IPs[0].Address.String(), strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		targets = append(targets, labels)
	}
	return targets, nil
}
