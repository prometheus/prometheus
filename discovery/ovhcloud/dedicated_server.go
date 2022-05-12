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

	"github.com/fatih/structs"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/ovh/go-ovh/ovh"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const dedicatedServerAPIPath = "/dedicated/server"

// DedicatedServer struct
type DedicatedServer struct {
	State           string `json:"state"`
	IPs             IPs    `json:"ips" label:"-"`
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

const (
	dedicatedServerLabelPrefix = metaLabelPrefix + "dedicatedServer_"
)

type dedicatedServerDiscovery struct {
	*refresh.Discovery
	config *SDConfig
	logger log.Logger
}

func newDedicatedServerDiscovery(conf *SDConfig, logger log.Logger) *dedicatedServerDiscovery {
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

func getDedicatedServerDetails(client *ovh.Client, serverName string) (*DedicatedServer, error) {
	var dedicatedServerDetails DedicatedServer
	err := client.Get(fmt.Sprintf("%s/%s", dedicatedServerAPIPath, url.QueryEscape(serverName)), &dedicatedServerDetails)
	if err != nil {
		return nil, err
	}

	var ips []string
	err = client.Get(fmt.Sprintf("%s/%s/ips", dedicatedServerAPIPath, url.QueryEscape(serverName)), &ips)
	if err != nil {
		return nil, err
	}

	parsedIPs, err := ParseIPList(ips)
	if err != nil {
		return nil, err
	}

	dedicatedServerDetails.IPs = *parsedIPs
	return &dedicatedServerDetails, nil
}

func (d *dedicatedServerDiscovery) getSource() string {
	return "ovhcloud_dedicated_server"
}

func (d *dedicatedServerDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	client, err := (d.config).CreateClient()
	if err != nil {
		return nil, err
	}
	var dedicatedServerDetailedList []DedicatedServer
	dedicatedServerList, err := getDedicatedServerList(client)
	if err != nil {
		return nil, err
	}
	for _, dedicatedServerName := range dedicatedServerList {
		dedicatedServer, err := getDedicatedServerDetails(client, dedicatedServerName)
		if err != nil {
			err := level.Warn(d.logger).Log("msg", fmt.Sprintf("%s: Could not get details of %s", d.getSource(), dedicatedServerName), "err", err.Error())
			if err != nil {
				return nil, err
			}
			continue
		}
		dedicatedServerDetailedList = append(dedicatedServerDetailedList, *dedicatedServer)
	}
	var targets []model.LabelSet

	for _, server := range dedicatedServerDetailedList {
		defaultIP := server.IPs.IPV4
		if defaultIP == "" {
			defaultIP = server.IPs.IPV6
		}
		labels := model.LabelSet{
			model.AddressLabel:  model.LabelValue(defaultIP),
			model.InstanceLabel: model.LabelValue(server.Name),
		}

		fields := structs.Fields(server)
		addFieldsOnLabels(fields, labels, dedicatedServerLabelPrefix)

		IPsFields := structs.Fields(server.IPs)
		addFieldsOnLabels(IPsFields, labels, dedicatedServerLabelPrefix)

		targets = append(targets, labels)
	}

	return []*targetgroup.Group{{Source: d.getSource(), Targets: targets}}, nil
}
