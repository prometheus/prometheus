// Copyright 2022 The Prometheus Authors
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
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
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

func newServerDiscovery(conf *SDConfig, logger log.Logger) (*serverDiscovery, error) {
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
	cfg.UserAgent = fmt.Sprintf("Prometheus/%s", version.Version)

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

	var targets []model.LabelSet
	for _, server := range *servers.Items {
		var ips []string
		ipsByNICName := make(map[string][]string)

		if server.Entities != nil && server.Entities.Nics != nil {
			for _, nic := range *server.Entities.Nics.Items {
				nicName := nicDefaultName
				if name := nic.Properties.Name; name != nil {
					nicName = *name
				}

				nicIPs := *nic.Properties.Ips
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
			model.AddressLabel:          model.LabelValue(addr),
			serverAvailabilityZoneLabel: model.LabelValue(*server.Properties.AvailabilityZone),
			serverCPUFamilyLabel:        model.LabelValue(*server.Properties.CpuFamily),
			serverServersIDLabel:        model.LabelValue(*servers.Id),
			serverIDLabel:               model.LabelValue(*server.Id),
			serverIPLabel:               model.LabelValue(join(ips, metaLabelSeparator)),
			serverLifecycleLabel:        model.LabelValue(*server.Metadata.State),
			serverNameLabel:             model.LabelValue(*server.Properties.Name),
			serverStateLabel:            model.LabelValue(*server.Properties.VmState),
			serverTypeLabel:             model.LabelValue(*server.Properties.Type),
		}

		for nicName, nicIPs := range ipsByNICName {
			name := serverNICIPLabelPrefix + strutil.SanitizeLabelName(nicName)
			labels[model.LabelName(name)] = model.LabelValue(join(nicIPs, metaLabelSeparator))
		}

		if server.Properties.BootCdrom != nil {
			labels[serverBootCDROMIDLabel] = model.LabelValue(*server.Properties.BootCdrom.Id)
		}

		if server.Properties.BootVolume != nil {
			labels[serverBootVolumeIDLabel] = model.LabelValue(*server.Properties.BootVolume.Id)
		}

		if server.Entities != nil && server.Entities.Volumes != nil {
			volumes := *server.Entities.Volumes.Items
			if len(volumes) > 0 {
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
