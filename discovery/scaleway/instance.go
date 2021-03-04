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

	instanceIDLabel         = instanceLabelPrefix + "id"
	instanceIPIsPublicLabel = instanceLabelPrefix + "ip_is_public"
	instanceIPVersionLabel  = instanceLabelPrefix + "ip_version"
	instanceImageNameLabel  = instanceLabelPrefix + "image_name"
	instanceNameLabel       = instanceLabelPrefix + "name"
	instanceStateLabel      = instanceLabelPrefix + "status"
	instanceTagsLabel       = instanceLabelPrefix + "tags"
	instanceTypeLabel       = instanceLabelPrefix + "type"
	instanceZoneLabel       = instanceLabelPrefix + "zone"
)

type instanceDiscovery struct {
	*refresh.Discovery
	Client    *scw.Client
	Port      int
	Zone      string
	Project   string
	AccessKey string
	SecretKey string
	Name      string
	Tags      []string
}

func newInstanceDiscovery(conf *SDConfig) (*instanceDiscovery, error) {
	d := &instanceDiscovery{
		Port:      conf.Port,
		Zone:      conf.Zone,
		Project:   conf.Project,
		AccessKey: conf.AccessKey,
		SecretKey: string(conf.SecretKey),
		Name:      conf.FilterName,
		Tags:      conf.FilterTags,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "scaleway_sd", false, false)
	if err != nil {
		return nil, err
	}

	profile, err := LoadProfile(conf)
	if err != nil {
		return nil, err
	}
	d.Client, err = scw.NewClient(
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
	api := instance.NewAPI(d.Client)
	zone, _ := d.Client.GetDefaultZone()
	project, _ := d.Client.GetDefaultProjectID()

	req := &instance.ListServersRequest{
		Zone:    zone,
		Project: scw.StringPtr(project),
	}

	if d.Name != "" {
		req.Name = scw.StringPtr(d.Name)
	}

	if d.Tags != nil {
		req.Tags = d.Tags
	}

	servers, err := api.ListServers(req, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	var targets []model.LabelSet
	for _, server := range servers.Servers {
		labels := model.LabelSet{
			instanceIDLabel:        model.LabelValue(server.ID),
			instanceNameLabel:      model.LabelValue(server.Name),
			instanceImageNameLabel: model.LabelValue(server.Image.Name),
			instanceZoneLabel:      model.LabelValue(server.Zone.String()),
			instanceTypeLabel:      model.LabelValue(server.CommercialType),
			instanceStateLabel:     model.LabelValue(server.State),
		}

		if len(server.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(server.Tags, separator) + separator
			labels[instanceTagsLabel] = model.LabelValue(tags)
		}

		if server.PrivateIP != nil {
			labels[instanceIPVersionLabel] = "IPv4"
			labels[instanceIPIsPublicLabel] = "false"

			addr := net.JoinHostPort(*server.PrivateIP, strconv.FormatUint(uint64(d.Port), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)

			targets = append(targets, labels.Clone())
		}

		if server.IPv6 != nil {
			labels[instanceIPVersionLabel] = "IPv6"
			labels[instanceIPIsPublicLabel] = "true"

			addr := net.JoinHostPort(server.IPv6.Address.String(), strconv.FormatUint(uint64(d.Port), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)

			targets = append(targets, labels.Clone())
		}

		if server.PublicIP != nil {
			labels[instanceIPVersionLabel] = "IPv4"
			labels[instanceIPIsPublicLabel] = "true"

			addr := net.JoinHostPort(server.PublicIP.Address.String(), strconv.FormatUint(uint64(d.Port), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)

			targets = append(targets, labels.Clone())
		}
	}

	return []*targetgroup.Group{{Source: "scaleway", Targets: targets}}, nil
}
