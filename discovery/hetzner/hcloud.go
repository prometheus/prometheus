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

package hetzner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-kit/log"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	hetznerHcloudLabelPrefix                        = hetznerLabelPrefix + "hcloud_"
	hetznerLabelHcloudImageName                     = hetznerHcloudLabelPrefix + "image_name"
	hetznerLabelHcloudImageDescription              = hetznerHcloudLabelPrefix + "image_description"
	hetznerLabelHcloudImageOSVersion                = hetznerHcloudLabelPrefix + "image_os_version"
	hetznerLabelHcloudImageOSFlavor                 = hetznerHcloudLabelPrefix + "image_os_flavor"
	hetznerLabelHcloudPrivateIPv4                   = hetznerHcloudLabelPrefix + "private_ipv4_"
	hetznerLabelHcloudDatacenterLocation            = hetznerHcloudLabelPrefix + "datacenter_location"
	hetznerLabelHcloudDatacenterLocationNetworkZone = hetznerHcloudLabelPrefix + "datacenter_location_network_zone"
	hetznerLabelHcloudCPUCores                      = hetznerHcloudLabelPrefix + "cpu_cores"
	hetznerLabelHcloudCPUType                       = hetznerHcloudLabelPrefix + "cpu_type"
	hetznerLabelHcloudMemoryGB                      = hetznerHcloudLabelPrefix + "memory_size_gb"
	hetznerLabelHcloudDiskGB                        = hetznerHcloudLabelPrefix + "disk_size_gb"
	hetznerLabelHcloudType                          = hetznerHcloudLabelPrefix + "server_type"
	hetznerLabelHcloudLabel                         = hetznerHcloudLabelPrefix + "label_"
	hetznerLabelHcloudLabelPresent                  = hetznerHcloudLabelPrefix + "labelpresent_"
)

// Discovery periodically performs Hetzner Cloud requests. It implements
// the Discoverer interface.
type hcloudDiscovery struct {
	*refresh.Discovery
	client *hcloud.Client
	port   int
}

// newHcloudDiscovery returns a new hcloudDiscovery which periodically refreshes its targets.
func newHcloudDiscovery(conf *SDConfig, _ log.Logger) (*hcloudDiscovery, error) {
	d := &hcloudDiscovery{
		port: conf.Port,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "hetzner_sd")
	if err != nil {
		return nil, err
	}
	d.client = hcloud.NewClient(
		hcloud.WithApplication("Prometheus", version.Version),
		hcloud.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		}),
		hcloud.WithEndpoint(conf.hcloudEndpoint),
	)
	return d, nil
}

func (d *hcloudDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	servers, err := d.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}
	networks, err := d.client.Network.All(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]model.LabelSet, len(servers))
	for i, server := range servers {
		labels := model.LabelSet{
			hetznerLabelRole:              model.LabelValue(hetznerRoleHcloud),
			hetznerLabelServerID:          model.LabelValue(fmt.Sprintf("%d", server.ID)),
			hetznerLabelServerName:        model.LabelValue(server.Name),
			hetznerLabelDatacenter:        model.LabelValue(server.Datacenter.Name),
			hetznerLabelPublicIPv4:        model.LabelValue(server.PublicNet.IPv4.IP.String()),
			hetznerLabelPublicIPv6Network: model.LabelValue(server.PublicNet.IPv6.Network.String()),
			hetznerLabelServerStatus:      model.LabelValue(server.Status),

			hetznerLabelHcloudDatacenterLocation:            model.LabelValue(server.Datacenter.Location.Name),
			hetznerLabelHcloudDatacenterLocationNetworkZone: model.LabelValue(server.Datacenter.Location.NetworkZone),
			hetznerLabelHcloudType:                          model.LabelValue(server.ServerType.Name),
			hetznerLabelHcloudCPUCores:                      model.LabelValue(fmt.Sprintf("%d", server.ServerType.Cores)),
			hetznerLabelHcloudCPUType:                       model.LabelValue(server.ServerType.CPUType),
			hetznerLabelHcloudMemoryGB:                      model.LabelValue(fmt.Sprintf("%d", int(server.ServerType.Memory))),
			hetznerLabelHcloudDiskGB:                        model.LabelValue(fmt.Sprintf("%d", server.ServerType.Disk)),

			model.AddressLabel: model.LabelValue(net.JoinHostPort(server.PublicNet.IPv4.IP.String(), strconv.FormatUint(uint64(d.port), 10))),
		}

		if server.Image != nil {
			labels[hetznerLabelHcloudImageName] = model.LabelValue(server.Image.Name)
			labels[hetznerLabelHcloudImageDescription] = model.LabelValue(server.Image.Description)
			labels[hetznerLabelHcloudImageOSVersion] = model.LabelValue(server.Image.OSVersion)
			labels[hetznerLabelHcloudImageOSFlavor] = model.LabelValue(server.Image.OSFlavor)
		}

		for _, privateNet := range server.PrivateNet {
			for _, network := range networks {
				if privateNet.Network.ID == network.ID {
					networkLabel := model.LabelName(hetznerLabelHcloudPrivateIPv4 + strutil.SanitizeLabelName(network.Name))
					labels[networkLabel] = model.LabelValue(privateNet.IP.String())
				}
			}
		}
		for labelKey, labelValue := range server.Labels {
			presentLabel := model.LabelName(hetznerLabelHcloudLabelPresent + strutil.SanitizeLabelName(labelKey))
			labels[presentLabel] = model.LabelValue("true")

			label := model.LabelName(hetznerLabelHcloudLabel + strutil.SanitizeLabelName(labelKey))
			labels[label] = model.LabelValue(labelValue)
		}
		targets[i] = labels
	}
	return []*targetgroup.Group{{Source: "hetzner", Targets: targets}}, nil
}
