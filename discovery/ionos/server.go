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

package ionos

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	ionoscloud "github.com/ionos-cloud/sdk-go/v6"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	serverLabelPrefix = metaLabelPrefix + "server_"

	serverAvailabilityZoneLabel = serverLabelPrefix + "availability_zone"
	serverBootCDROMIDLabel      = serverLabelPrefix + "boot_cdrom_id"
	serverBootImageIDLabel      = serverLabelPrefix + "boot_image_id"
	serverBootVolumeIDLabel     = serverLabelPrefix + "boot_volume_id"
	serverCPUFamilyLabel        = serverLabelPrefix + "cpu_family"
	serverIDLabel               = serverLabelPrefix + "id"
	serverIPLabel               = serverLabelPrefix + "ip"
	serverLifecycleLabel        = serverLabelPrefix + "lifecycle"
	serverNameLabel             = serverLabelPrefix + "name"
	serverNICIPLabelPrefix      = serverLabelPrefix + "nic_ip_"
	serverServersIDLabel        = serverLabelPrefix + "servers_id"
	serverStateLabel            = serverLabelPrefix + "state"
	serverTypeLabel             = serverLabelPrefix + "type"

	nicDefaultName = "unnamed"
)

type serverDiscovery struct {
	*refresh.Discovery
	client       *ionoscloud.APIClient
	port         int
	datacenterID string
}

func newServerDiscovery(conf *SDConfig, _ *slog.Logger) (*serverDiscovery, error) {
	d := &serverDiscovery{
		port:         conf.Port,
		datacenterID: conf.DatacenterID,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "ionos_sd")
	if err != nil {
		return nil, err
	}

	// Username, password and token are set via http client config.
	cfg := ionoscloud.NewConfiguration("", "", "", conf.ionosEndpoint)
	cfg.HTTPClient = &http.Client{
		Transport: rt,
		Timeout:   time.Duration(conf.RefreshInterval),
	}
	cfg.UserAgent = version.PrometheusUserAgent()

	d.client = ionoscloud.NewAPIClient(cfg)

	return d, nil
}

func (d *serverDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	api := d.client.ServersApi

	servers, _, err := api.DatacentersServersGet(ctx, d.datacenterID).
		Depth(3).
		Execute()
	if err != nil {
		return nil, err
	}

	// Items is omitted from the response when the datacenter holds no servers.
	var serverItems []ionoscloud.Server
	if servers.Items != nil {
		serverItems = *servers.Items
	}

	var targets []model.LabelSet
	for _, server := range serverItems {
		var ips []string
		ipsByNICName := make(map[string][]string)

		if server.Entities != nil && server.Entities.Nics != nil && server.Entities.Nics.Items != nil {
			for _, nic := range *server.Entities.Nics.Items {
				if nic.Properties == nil {
					continue
				}

				nicName := nicDefaultName
				if name := nic.Properties.Name; name != nil {
					nicName = *name
				}

				// An absent Ips is equivalent to an empty one.
				var nicIPs []string
				if nic.Properties.Ips != nil {
					nicIPs = *nic.Properties.Ips
				}
				ips = append(nicIPs, ips...)
				ipsByNICName[nicName] = append(nicIPs, ipsByNICName[nicName]...)
			}
		}

		// If a server has no IP addresses, it's being dropped from the targets.
		if len(ips) == 0 {
			continue
		}

		addr := net.JoinHostPort(ips[0], strconv.FormatUint(uint64(d.port), 10))
		labels := model.LabelSet{
			model.AddressLabel: model.LabelValue(addr),
			serverIPLabel:      model.LabelValue(join(ips, metaLabelSeparator)),
		}

		// Properties is a pointer and is absent from the response for servers
		// that expose none. Substituting the zero value keeps the per-field
		// checks below as the single place where a label is omitted.
		props := server.Properties
		if props == nil {
			props = &ionoscloud.ServerProperties{}
		}

		if props.AvailabilityZone != nil {
			labels[serverAvailabilityZoneLabel] = model.LabelValue(*props.AvailabilityZone)
		}
		if props.CpuFamily != nil {
			labels[serverCPUFamilyLabel] = model.LabelValue(*props.CpuFamily)
		}
		if servers.Id != nil {
			labels[serverServersIDLabel] = model.LabelValue(*servers.Id)
		}
		if server.Id != nil {
			labels[serverIDLabel] = model.LabelValue(*server.Id)
		}
		if server.Metadata != nil && server.Metadata.State != nil {
			labels[serverLifecycleLabel] = model.LabelValue(*server.Metadata.State)
		}
		if props.Name != nil {
			labels[serverNameLabel] = model.LabelValue(*props.Name)
		}
		if props.VmState != nil {
			labels[serverStateLabel] = model.LabelValue(*props.VmState)
		}
		if props.Type != nil {
			labels[serverTypeLabel] = model.LabelValue(*props.Type)
		}

		for nicName, nicIPs := range ipsByNICName {
			name := serverNICIPLabelPrefix + strutil.SanitizeLabelName(nicName)
			labels[model.LabelName(name)] = model.LabelValue(join(nicIPs, metaLabelSeparator))
		}

		if props.BootCdrom != nil && props.BootCdrom.Id != nil {
			labels[serverBootCDROMIDLabel] = model.LabelValue(*props.BootCdrom.Id)
		}

		if props.BootVolume != nil && props.BootVolume.Id != nil {
			labels[serverBootVolumeIDLabel] = model.LabelValue(*props.BootVolume.Id)
		}

		if server.Entities != nil && server.Entities.Volumes != nil && server.Entities.Volumes.Items != nil {
			volumes := *server.Entities.Volumes.Items
			if len(volumes) > 0 && volumes[0].Properties != nil {
				image := volumes[0].Properties.Image
				if image != nil {
					labels[serverBootImageIDLabel] = model.LabelValue(*image)
				}
			}
		}

		targets = append(targets, labels)
	}

	return []*targetgroup.Group{{Source: "ionos", Targets: targets}}, nil
}

// join returns strings.Join with additional separators at beginning and end.
func join(elems []string, sep string) string {
	return sep + strings.Join(elems, sep) + sep
}
