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
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	instanceLabelPrefix = metaLabelPrefix + "instance_"

	instanceBootTypeLabel          = instanceLabelPrefix + "boot_type"
	instanceHostnameLabel          = instanceLabelPrefix + "hostname"
	instanceIDLabel                = instanceLabelPrefix + "id"
	instanceImageArchLabel         = instanceLabelPrefix + "image_arch"
	instanceImageIDLabel           = instanceLabelPrefix + "image_id"
	instanceImageNameLabel         = instanceLabelPrefix + "image_name"
	instanceLocationClusterID      = instanceLabelPrefix + "location_cluster_id"
	instanceLocationHypervisorID   = instanceLabelPrefix + "location_hypervisor_id"
	instanceLocationNodeID         = instanceLabelPrefix + "location_node_id"
	instanceNameLabel              = instanceLabelPrefix + "name"
	instanceOrganizationLabel      = instanceLabelPrefix + "organization_id"
	instancePrivateIPv4Label       = instanceLabelPrefix + "private_ipv4"
	instanceProjectLabel           = instanceLabelPrefix + "project_id"
	instancePublicIPv4Label        = instanceLabelPrefix + "public_ipv4"
	instancePublicIPv6Label        = instanceLabelPrefix + "public_ipv6"
	instanceSecurityGroupIDLabel   = instanceLabelPrefix + "security_group_id"
	instanceSecurityGroupNameLabel = instanceLabelPrefix + "security_group_name"
	instanceStateLabel             = instanceLabelPrefix + "status"
	instanceTagsLabel              = instanceLabelPrefix + "tags"
	instanceTypeLabel              = instanceLabelPrefix + "type"
	instanceZoneLabel              = instanceLabelPrefix + "zone"
	instanceRegionLabel            = instanceLabelPrefix + "region"
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
		secretKey:  conf.secretKeyForConfig(),
		nameFilter: conf.NameFilter,
		tagsFilter: conf.TagsFilter,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "scaleway_sd")
	if err != nil {
		return nil, err
	}

	if conf.SecretKeyFile != "" {
		rt, err = newAuthTokenFileRoundTripper(conf.SecretKeyFile, rt)
		if err != nil {
			return nil, err
		}
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
			instanceBootTypeLabel:     model.LabelValue(server.BootType),
			instanceHostnameLabel:     model.LabelValue(server.Hostname),
			instanceIDLabel:           model.LabelValue(server.ID),
			instanceNameLabel:         model.LabelValue(server.Name),
			instanceOrganizationLabel: model.LabelValue(server.Organization),
			instanceProjectLabel:      model.LabelValue(server.Project),
			instanceStateLabel:        model.LabelValue(server.State),
			instanceTypeLabel:         model.LabelValue(server.CommercialType),
			instanceZoneLabel:         model.LabelValue(server.Zone.String()),
		}

		if server.Image != nil {
			labels[instanceImageArchLabel] = model.LabelValue(server.Image.Arch)
			labels[instanceImageIDLabel] = model.LabelValue(server.Image.ID)
			labels[instanceImageNameLabel] = model.LabelValue(server.Image.Name)
		}

		if server.Location != nil {
			labels[instanceLocationClusterID] = model.LabelValue(server.Location.ClusterID)
			labels[instanceLocationHypervisorID] = model.LabelValue(server.Location.HypervisorID)
			labels[instanceLocationNodeID] = model.LabelValue(server.Location.NodeID)
		}

		if server.SecurityGroup != nil {
			labels[instanceSecurityGroupIDLabel] = model.LabelValue(server.SecurityGroup.ID)
			labels[instanceSecurityGroupNameLabel] = model.LabelValue(server.SecurityGroup.Name)
		}

		if region, err := server.Zone.Region(); err == nil {
			labels[instanceRegionLabel] = model.LabelValue(region.String())
		}

		if len(server.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(server.Tags, separator) + separator
			labels[instanceTagsLabel] = model.LabelValue(tags)
		}

		if server.IPv6 != nil {
			labels[instancePublicIPv6Label] = model.LabelValue(server.IPv6.Address.String())
		}

		if server.PublicIP != nil {
			labels[instancePublicIPv4Label] = model.LabelValue(server.PublicIP.Address.String())
		}

		if server.PrivateIP != nil {
			labels[instancePrivateIPv4Label] = model.LabelValue(*server.PrivateIP)

			addr := net.JoinHostPort(*server.PrivateIP, strconv.FormatUint(uint64(d.port), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)

			targets = append(targets, labels)
		}
	}

	return []*targetgroup.Group{{Source: "scaleway", Targets: targets}}, nil
}
