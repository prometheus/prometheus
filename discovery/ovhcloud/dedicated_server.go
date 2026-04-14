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

package ovhcloud

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"path"
	"strconv"

	"github.com/ovh/go-ovh/ovh"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	dedicatedServerAPIPath     = "/dedicated/server"
	dedicatedServerLabelPrefix = metaLabelPrefix + "dedicated_server_"
)

// dedicatedServer struct from API. Also contains IP addresses that are fetched
// independently.
type dedicatedServer struct {
	State           string `json:"state"`
	ips             []netip.Addr
	CommercialRange string `json:"commercialRange"`
	LinkSpeed       int    `json:"linkSpeed"`
	Rack            string `json:"rack"`
	NoIntervention  bool   `json:"noIntervention"`
	Os              string `json:"os"`
	SupportLevel    string `json:"supportLevel"`
	ServerID        int64  `json:"serverId"`
	Reverse         string `json:"reverse"`
	Datacenter      string `json:"datacenter"`
	Name            string `json:"name"`
}

type dedicatedServerDiscovery struct {
	*refresh.Discovery
	config *SDConfig
	logger *slog.Logger
}

func newDedicatedServerDiscovery(conf *SDConfig, logger *slog.Logger) *dedicatedServerDiscovery {
	return &dedicatedServerDiscovery{config: conf, logger: logger}
}

func getDedicatedServerList(client *ovh.Client) ([]string, error) {
	var dedicatedListName []string
	err := client.Get(dedicatedServerAPIPath, &dedicatedListName)
	if err != nil {
		return nil, err
	}

	return dedicatedListName, nil
}

func getDedicatedServerDetails(client *ovh.Client, serverName string) (*dedicatedServer, error) {
	var dedicatedServerDetails dedicatedServer
	err := client.Get(path.Join(dedicatedServerAPIPath, url.QueryEscape(serverName)), &dedicatedServerDetails)
	if err != nil {
		return nil, err
	}

	var ips []string
	err = client.Get(path.Join(dedicatedServerAPIPath, url.QueryEscape(serverName), "ips"), &ips)
	if err != nil {
		return nil, err
	}

	parsedIPs, err := parseIPList(ips)
	if err != nil {
		return nil, err
	}

	dedicatedServerDetails.ips = parsedIPs
	return &dedicatedServerDetails, nil
}

func (*dedicatedServerDiscovery) getService() string {
	return "dedicated_server"
}

func (d *dedicatedServerDiscovery) getSource() string {
	return fmt.Sprintf("%s_%s", d.config.Name(), d.getService())
}

func (d *dedicatedServerDiscovery) refresh(context.Context) ([]*targetgroup.Group, error) {
	client, err := createClient(d.config)
	if err != nil {
		return nil, err
	}
	var dedicatedServerDetailedList []dedicatedServer
	dedicatedServerList, err := getDedicatedServerList(client)
	if err != nil {
		return nil, err
	}
	for _, dedicatedServerName := range dedicatedServerList {
		dedicatedServer, err := getDedicatedServerDetails(client, dedicatedServerName)
		if err != nil {
			d.logger.Warn(fmt.Sprintf("%s: Could not get details of %s", d.getSource(), dedicatedServerName), "err", err.Error())
			continue
		}
		dedicatedServerDetailedList = append(dedicatedServerDetailedList, *dedicatedServer)
	}
	var targets []model.LabelSet

	for _, server := range dedicatedServerDetailedList {
		var ipv4, ipv6 string
		for _, ip := range server.ips {
			if ip.Is4() {
				ipv4 = ip.String()
			}
			if ip.Is6() {
				ipv6 = ip.String()
			}
		}
		defaultIP := ipv4
		if defaultIP == "" {
			defaultIP = ipv6
		}
		labels := model.LabelSet{
			model.AddressLabel:                              model.LabelValue(defaultIP),
			model.InstanceLabel:                             model.LabelValue(server.Name),
			dedicatedServerLabelPrefix + "state":            model.LabelValue(server.State),
			dedicatedServerLabelPrefix + "commercial_range": model.LabelValue(server.CommercialRange),
			dedicatedServerLabelPrefix + "link_speed":       model.LabelValue(strconv.Itoa(server.LinkSpeed)),
			dedicatedServerLabelPrefix + "rack":             model.LabelValue(server.Rack),
			dedicatedServerLabelPrefix + "no_intervention":  model.LabelValue(strconv.FormatBool(server.NoIntervention)),
			dedicatedServerLabelPrefix + "os":               model.LabelValue(server.Os),
			dedicatedServerLabelPrefix + "support_level":    model.LabelValue(server.SupportLevel),
			dedicatedServerLabelPrefix + "server_id":        model.LabelValue(strconv.FormatInt(server.ServerID, 10)),
			dedicatedServerLabelPrefix + "reverse":          model.LabelValue(server.Reverse),
			dedicatedServerLabelPrefix + "datacenter":       model.LabelValue(server.Datacenter),
			dedicatedServerLabelPrefix + "name":             model.LabelValue(server.Name),
			dedicatedServerLabelPrefix + "ipv4":             model.LabelValue(ipv4),
			dedicatedServerLabelPrefix + "ipv6":             model.LabelValue(ipv6),
		}
		targets = append(targets, labels)
	}

	return []*targetgroup.Group{{Source: d.getSource(), Targets: targets}}, nil
}
