// Copyright The Prometheus Authors
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
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

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
	hetznerLabelHcloudLocation                      = hetznerHcloudLabelPrefix + "location"
	hetznerLabelHcloudLocationNetworkZone           = hetznerHcloudLabelPrefix + "location_network_zone"
	hetznerLabelHcloudDatacenterLocation            = hetznerHcloudLabelPrefix + "datacenter_location"              // Label name kept for backward compatibility
	hetznerLabelHcloudDatacenterLocationNetworkZone = hetznerHcloudLabelPrefix + "datacenter_location_network_zone" // Label name kept for backward compatibility
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
	client        *hcloud.Client
	port          int
	labelSelector string
}

// newHcloudDiscovery returns a new hcloudDiscovery which periodically refreshes its targets.
func newHcloudDiscovery(conf *SDConfig, _ *slog.Logger) (*hcloudDiscovery, error) {
	d := &hcloudDiscovery{
		port:          conf.Port,
		labelSelector: conf.LabelSelector,
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
	servers, err := d.client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{ListOpts: hcloud.ListOpts{
		PerPage:       50,
		LabelSelector: d.labelSelector,
	}})
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
			hetznerLabelRole:              model.LabelValue(HetznerRoleHcloud),
			hetznerLabelServerID:          model.LabelValue(strconv.FormatInt(server.ID, 10)),
			hetznerLabelServerName:        model.LabelValue(server.Name),
			hetznerLabelPublicIPv4:        model.LabelValue(server.PublicNet.IPv4.IP.String()),
			hetznerLabelPublicIPv6Network: model.LabelValue(server.PublicNet.IPv6.Network.String()),
			hetznerLabelServerStatus:      model.LabelValue(server.Status),

			hetznerLabelHcloudLocation:                      model.LabelValue(server.Location.Name),
			hetznerLabelHcloudLocationNetworkZone:           model.LabelValue(server.Location.NetworkZone),
			hetznerLabelHcloudDatacenterLocation:            model.LabelValue(server.Location.Name),        // Label name kept for backward compatibility
			hetznerLabelHcloudDatacenterLocationNetworkZone: model.LabelValue(server.Location.NetworkZone), // Label name kept for backward compatibility
			hetznerLabelHcloudType:                          model.LabelValue(server.ServerType.Name),
			hetznerLabelHcloudCPUCores:                      model.LabelValue(strconv.Itoa(server.ServerType.Cores)),
			hetznerLabelHcloudCPUType:                       model.LabelValue(server.ServerType.CPUType),
			hetznerLabelHcloudMemoryGB:                      model.LabelValue(strconv.Itoa(int(server.ServerType.Memory))),
			hetznerLabelHcloudDiskGB:                        model.LabelValue(strconv.Itoa(server.ServerType.Disk)),

			model.AddressLabel: model.LabelValue(net.JoinHostPort(server.PublicNet.IPv4.IP.String(), strconv.FormatUint(uint64(d.port), 10))),
		}

		// [hcloud.Server.Datacenter] is deprecated and will be removed after 1 July 2026.
		// See https://docs.hetzner.cloud/changelog#2025-12-16-phasing-out-datacenters
		if server.Datacenter != nil { // nolint: staticcheck
			labels[hetznerLabelDatacenter] = model.LabelValue(server.Datacenter.Name) // nolint: staticcheck
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
