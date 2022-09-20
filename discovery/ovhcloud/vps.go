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
	"net/url"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/ovh/go-ovh/ovh"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const vpsAPIPath = "/vps"

// Model struct from API
type Model struct {
	MaximumAdditionalIP int      `json:"maximumAdditionnalIp"`
	Offer               string   `json:"offer"`
	Datacenter          []string `json:"datacenter"`
	Vcore               int      `json:"vcore"`
	Version             string   `json:"version"`
	Name                string   `json:"name"`
	Disk                int      `json:"disk"`
	Memory              int      `json:"memory"`
}

// Vps struct from API
type Vps struct {
	IPs                IPs      `json:"ips"`
	Keymap             []string `json:"keymap"`
	Zone               string   `json:"zone"`
	Model              Model    `json:"model"`
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

const (
	vpsLabelPrefix = metaLabelPrefix + "vps_"
)

type vpsDiscovery struct {
	*refresh.Discovery
	config *SDConfig
	logger log.Logger
}

func newVpsDiscovery(conf *SDConfig, logger log.Logger) *vpsDiscovery {
	return &vpsDiscovery{config: conf, logger: logger}
}

func getVpsDetails(client *ovh.Client, vpsName string) (*Vps, error) {
	var vpsDetails Vps
	vpsNamePath := fmt.Sprintf("%s/%s", vpsAPIPath, url.QueryEscape(vpsName))

	err := client.Get(vpsNamePath, &vpsDetails)
	if err != nil {
		return nil, err
	}

	var ips []string
	err = client.Get(fmt.Sprintf("%s/ips", vpsNamePath), &ips)
	if err != nil {
		return nil, err
	}

	parsedIPs, err := ParseIPList(ips)
	if err != nil {
		return nil, err
	}
	vpsDetails.IPs = *parsedIPs

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

func (d *vpsDiscovery) getService() string {
	return "vps"
}

func (d *vpsDiscovery) getSource() string {
	return fmt.Sprintf("%s_%s", d.config.Name(), d.getService())
}

func (d *vpsDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	client, err := CreateClient(d.config)
	if err != nil {
		return nil, err
	}

	var vpsDetailedList []Vps
	vpsList, err := getVpsList(client)
	if err != nil {
		return nil, err
	}

	for _, vpsName := range vpsList {
		vpsDetailed, err := getVpsDetails(client, vpsName)
		if err != nil {
			err := level.Warn(d.logger).Log("msg", fmt.Sprintf("%s: Could not get details of %s", d.getSource(), vpsName), "err", err.Error())
			if err != nil {
				return nil, err
			}
			continue
		}
		vpsDetailedList = append(vpsDetailedList, *vpsDetailed)
	}

	var targets []model.LabelSet
	for _, server := range vpsDetailedList {
		defaultIP := server.IPs.IPV4
		if defaultIP == "" {
			defaultIP = server.IPs.IPV6
		}
		labels := model.LabelSet{
			model.AddressLabel:                     model.LabelValue(defaultIP),
			model.InstanceLabel:                    model.LabelValue(server.Name),
			vpsLabelPrefix + "offer":               model.LabelValue(server.Model.Offer),
			vpsLabelPrefix + "datacenter":          model.LabelValue(fmt.Sprintf("%+v", server.Model.Datacenter)),
			vpsLabelPrefix + "model_vcore":         model.LabelValue(fmt.Sprintf("%d", server.Model.Vcore)),
			vpsLabelPrefix + "maximumAdditionalIp": model.LabelValue(fmt.Sprintf("%d", server.Model.MaximumAdditionalIP)),
			vpsLabelPrefix + "version":             model.LabelValue(server.Model.Version),
			vpsLabelPrefix + "model_name":          model.LabelValue(server.Model.Name),
			vpsLabelPrefix + "disk":                model.LabelValue(fmt.Sprintf("%d", server.Model.Disk)),
			vpsLabelPrefix + "memory":              model.LabelValue(fmt.Sprintf("%d", server.Model.Memory)),
			vpsLabelPrefix + "zone":                model.LabelValue(server.Zone),
			vpsLabelPrefix + "displayName":         model.LabelValue(server.DisplayName),
			vpsLabelPrefix + "cluster":             model.LabelValue(server.Cluster),
			vpsLabelPrefix + "state":               model.LabelValue(server.State),
			vpsLabelPrefix + "name":                model.LabelValue(server.Name),
			vpsLabelPrefix + "netbootMode":         model.LabelValue(server.NetbootMode),
			vpsLabelPrefix + "memoryLimit":         model.LabelValue(fmt.Sprintf("%d", server.MemoryLimit)),
			vpsLabelPrefix + "offerType":           model.LabelValue(server.OfferType),
			vpsLabelPrefix + "vcore":               model.LabelValue(fmt.Sprintf("%d", server.Vcore)),
			vpsLabelPrefix + "ipv4":                model.LabelValue(server.IPs.IPV4),
			vpsLabelPrefix + "ipv6":                model.LabelValue(server.IPs.IPV6),
		}

		targets = append(targets, labels)
	}

	return []*targetgroup.Group{{Source: d.getSource(), Targets: targets}}, nil
}
