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
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	instanceLabelPrefix = metaLabelPrefix + "instance_"

	instanceIDLabel        = instanceLabelPrefix + "id"
	instancePrivateIPv4    = instanceLabelPrefix + "private_ipv4"
	instancePublicIPv4     = instanceLabelPrefix + "public_ipv4"
	instancePublicIPv6     = instanceLabelPrefix + "public_ipv6"
	instanceImageNameLabel = instanceLabelPrefix + "image_name"
	instanceNameLabel      = instanceLabelPrefix + "name"
	instanceProjectLabel   = instanceLabelPrefix + "project_id"
	instanceStateLabel     = instanceLabelPrefix + "status"
	instanceTagsLabel      = instanceLabelPrefix + "tags"
	instanceTypeLabel      = instanceLabelPrefix + "type"
	instanceZoneLabel      = instanceLabelPrefix + "zone"
)

type instanceDiscovery struct {
	*refresh.Discovery
	client     *scw.Client
	port       int
	zone       string
	project    string
	accessKey  string
	secretKey  string
	nameFilter string
	tagsFilter []string
}

func newInstanceDiscovery(conf *SDConfig) (*instanceDiscovery, error) {
	d := &instanceDiscovery{
		port:       conf.Port,
		zone:       conf.Zone,
		project:    conf.Project,
		accessKey:  conf.AccessKey,
		secretKey:  string(conf.SecretKey),
		nameFilter: conf.NameFilter,
		tagsFilter: conf.TagsFilter,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "scaleway_sd", false, false)
	if err != nil {
		return nil, err
	}

	profile, err := loadProfile(conf)
	if err != nil {
		return nil, err
	}

	d.client, err = scw.NewClient(
		scw.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		}),
		scw.WithUserAgent(fmt.Sprintf("Prometheus/%s", version.Version)),
		scw.WithProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up scaleway client: %w", err)
	}

	return d, nil
}

func (d *instanceDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	api := instance.NewAPI(d.client)

	req := &instance.ListServersRequest{}

	if d.nameFilter != "" {
		req.Name = scw.StringPtr(d.nameFilter)
	}

	if d.tagsFilter != nil {
		req.Tags = d.tagsFilter
	}

	servers, err := api.ListServers(req, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	var targets []model.LabelSet
	for _, server := range servers.Servers {
		labels := model.LabelSet{
			instanceIDLabel:        model.LabelValue(server.ID),
			instanceImageNameLabel: model.LabelValue(server.Image.Name),
			instanceNameLabel:      model.LabelValue(server.Name),
			instanceProjectLabel:   model.LabelValue(server.Project),
			instanceStateLabel:     model.LabelValue(server.State),
			instanceTypeLabel:      model.LabelValue(server.CommercialType),
			instanceZoneLabel:      model.LabelValue(server.Zone.String()),
		}

		if len(server.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(server.Tags, separator) + separator
			labels[instanceTagsLabel] = model.LabelValue(tags)
		}

		if server.IPv6 != nil {
			labels[instancePublicIPv6] = model.LabelValue(server.IPv6.Address.String())
		}

		if server.PublicIP != nil {
			labels[instancePublicIPv4] = model.LabelValue(server.PublicIP.Address.String())
		}

		if server.PrivateIP != nil {
			labels[instancePrivateIPv4] = model.LabelValue(*server.PrivateIP)

			addr := net.JoinHostPort(*server.PrivateIP, strconv.FormatUint(uint64(d.port), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)

			targets = append(targets, labels)
		}

	}

	return []*targetgroup.Group{{Source: "scaleway", Targets: targets}}, nil
}
