// Copyright 2015 The Prometheus Authors
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
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	instanceLabelPrefix = metaLabelPrefix + "instance_"

	instanceNameLabel           = instanceLabelPrefix + "name"
	instanceImageNameLabel      = instanceLabelPrefix + "image_name"
	instanceStateLabel          = instanceLabelPrefix + "status"
	instanceIDLabel             = instanceLabelPrefix + "id"
	instanceZoneLabel           = instanceLabelPrefix + "zone"
	instanceCommercialTypeLabel = instanceLabelPrefix + "commercial_type"
	instanceTagsLabel           = instanceLabelPrefix + "tags"
	instancePrivateIPLabel      = instanceLabelPrefix + "private_ip"
	instancePublicIPLabel       = instanceLabelPrefix + "public_ip"
	instanceIPv6Label           = instanceLabelPrefix + "ipv6"
)

func (d *Discovery) listInstanceServers(ctx context.Context) ([]model.LabelSet, error) {
	api := instance.NewAPI(d.client)
	zone, _ := d.client.GetDefaultZone()
	project, _ := d.client.GetDefaultProjectID()

	servers, err := api.ListServers(&instance.ListServersRequest{
		Zone:    zone,
		Project: scw.StringPtr(project),
	}, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	var targets []model.LabelSet
	for _, server := range servers.Servers {
		labels := model.LabelSet{
			instanceIDLabel:             model.LabelValue(server.ID),
			instanceNameLabel:           model.LabelValue(server.Name),
			instanceImageNameLabel:      model.LabelValue(server.Image.Name),
			instanceZoneLabel:           model.LabelValue(server.Zone.String()),
			instanceCommercialTypeLabel: model.LabelValue(server.CommercialType),
			instanceStateLabel:          model.LabelValue(server.State),
		}

		if len(server.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(server.Tags, separator) + separator
			labels[instanceTagsLabel] = model.LabelValue(tags)
		}

		if server.PrivateIP != nil {
			labels[instancePrivateIPLabel] = model.LabelValue(*server.PrivateIP)
		}

		if server.PublicIP != nil {
			labels[instancePublicIPLabel] = model.LabelValue(server.PublicIP.Address.String())
		}

		if server.IPv6 != nil {
			labels[instanceIPv6Label] = model.LabelValue(server.IPv6.Address.String())
		}

		addr := net.JoinHostPort(server.PublicIP.Address.String(), strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		targets = append(targets, labels)
	}

	return targets, nil
}
