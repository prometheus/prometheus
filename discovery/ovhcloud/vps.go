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
	vpsAPIPath     = "/vps"
	vpsLabelPrefix = metaLabelPrefix + "vps_"
)

// Model struct from API.
type vpsModel struct {
	MaximumAdditionalIP int      `json:"maximumAdditionnalIp"`
	Offer               string   `json:"offer"`
	Datacenter          []string `json:"datacenter"`
	Vcore               int      `json:"vcore"`
	Version             string   `json:"version"`
	Name                string   `json:"name"`
	Disk                int      `json:"disk"`
	Memory              int      `json:"memory"`
}

// VPS struct from API. Also contains IP addresses that are fetched
// independently.
type virtualPrivateServer struct {
	ips                []netip.Addr
	Keymap             []string `json:"keymap"`
	Zone               string   `json:"zone"`
	Model              vpsModel `json:"model"`
	DisplayName        string   `json:"displayName"`
	MonitoringIPBlocks []string `json:"monitoringIpBlocks"`
	Cluster            string   `json:"cluster"`
	State              string   `json:"state"`
	Name               string   `json:"name"`
	NetbootMode        string   `json:"netbootMode"`
	MemoryLimit        int      `json:"memoryLimit"`
	OfferType          string   `json:"offerType"`
	Vcore              int      `json:"vcore"`
}

type vpsDiscovery struct {
	*refresh.Discovery
	config *SDConfig
	logger *slog.Logger
}

func newVpsDiscovery(conf *SDConfig, logger *slog.Logger) *vpsDiscovery {
	return &vpsDiscovery{config: conf, logger: logger}
}

func getVpsDetails(client *ovh.Client, vpsName string) (*virtualPrivateServer, error) {
	var vpsDetails virtualPrivateServer
	vpsNamePath := path.Join(vpsAPIPath, url.QueryEscape(vpsName))

	err := client.Get(vpsNamePath, &vpsDetails)
	if err != nil {
		return nil, err
	}

	var ips []string
	err = client.Get(path.Join(vpsNamePath, "ips"), &ips)
	if err != nil {
		return nil, err
	}

	parsedIPs, err := parseIPList(ips)
	if err != nil {
		return nil, err
	}
	vpsDetails.ips = parsedIPs

	return &vpsDetails, nil
}

func getVpsList(client *ovh.Client) ([]string, error) {
	var vpsListName []string

	err := client.Get(vpsAPIPath, &vpsListName)
	if err != nil {
		return nil, err
	}

	return vpsListName, nil
}

func (*vpsDiscovery) getService() string {
	return "vps"
}

func (d *vpsDiscovery) getSource() string {
	return fmt.Sprintf("%s_%s", d.config.Name(), d.getService())
}

func (d *vpsDiscovery) refresh(context.Context) ([]*targetgroup.Group, error) {
	client, err := createClient(d.config)
	if err != nil {
		return nil, err
	}

	var vpsDetailedList []virtualPrivateServer
	vpsList, err := getVpsList(client)
	if err != nil {
		return nil, err
	}

	for _, vpsName := range vpsList {
		vpsDetailed, err := getVpsDetails(client, vpsName)
		if err != nil {
			d.logger.Warn(fmt.Sprintf("%s: Could not get details of %s", d.getSource(), vpsName), "err", err.Error())
			continue
		}
		vpsDetailedList = append(vpsDetailedList, *vpsDetailed)
	}

	var targets []model.LabelSet
	for _, server := range vpsDetailedList {
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
			model.AddressLabel:                       model.LabelValue(defaultIP),
			model.InstanceLabel:                      model.LabelValue(server.Name),
			vpsLabelPrefix + "offer":                 model.LabelValue(server.Model.Offer),
			vpsLabelPrefix + "datacenter":            model.LabelValue(fmt.Sprintf("%+v", server.Model.Datacenter)),
			vpsLabelPrefix + "model_vcore":           model.LabelValue(strconv.Itoa(server.Model.Vcore)),
			vpsLabelPrefix + "maximum_additional_ip": model.LabelValue(strconv.Itoa(server.Model.MaximumAdditionalIP)),
			vpsLabelPrefix + "version":               model.LabelValue(server.Model.Version),
			vpsLabelPrefix + "model_name":            model.LabelValue(server.Model.Name),
			vpsLabelPrefix + "disk":                  model.LabelValue(strconv.Itoa(server.Model.Disk)),
			vpsLabelPrefix + "memory":                model.LabelValue(strconv.Itoa(server.Model.Memory)),
			vpsLabelPrefix + "zone":                  model.LabelValue(server.Zone),
			vpsLabelPrefix + "display_name":          model.LabelValue(server.DisplayName),
			vpsLabelPrefix + "cluster":               model.LabelValue(server.Cluster),
			vpsLabelPrefix + "state":                 model.LabelValue(server.State),
			vpsLabelPrefix + "name":                  model.LabelValue(server.Name),
			vpsLabelPrefix + "netboot_mode":          model.LabelValue(server.NetbootMode),
			vpsLabelPrefix + "memory_limit":          model.LabelValue(strconv.Itoa(server.MemoryLimit)),
			vpsLabelPrefix + "offer_type":            model.LabelValue(server.OfferType),
			vpsLabelPrefix + "vcore":                 model.LabelValue(strconv.Itoa(server.Vcore)),
			vpsLabelPrefix + "ipv4":                  model.LabelValue(ipv4),
			vpsLabelPrefix + "ipv6":                  model.LabelValue(ipv6),
		}

		targets = append(targets, labels)
	}

	return []*targetgroup.Group{{Source: d.getSource(), Targets: targets}}, nil
}
