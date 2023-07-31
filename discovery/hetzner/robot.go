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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	hetznerRobotLabelPrefix    = hetznerLabelPrefix + "robot_"
	hetznerLabelRobotProduct   = hetznerRobotLabelPrefix + "product"
	hetznerLabelRobotCancelled = hetznerRobotLabelPrefix + "cancelled"
)

var userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

// Discovery periodically performs Hetzner Robot requests. It implements
// the Discoverer interface.
type robotDiscovery struct {
	*refresh.Discovery
	client   *http.Client
	port     int
	endpoint string
}

// newRobotDiscovery returns a new robotDiscovery which periodically refreshes its targets.
func newRobotDiscovery(conf *SDConfig, _ log.Logger) (*robotDiscovery, error) {
	d := &robotDiscovery{
		port:     conf.Port,
		endpoint: conf.robotEndpoint,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "hetzner_sd")
	if err != nil {
		return nil, err
	}
	d.client = &http.Client{
		Transport: rt,
		Timeout:   time.Duration(conf.RefreshInterval),
	}

	return d, nil
}

func (d *robotDiscovery) refresh(context.Context) ([]*targetgroup.Group, error) {
	req, err := http.NewRequest("GET", d.endpoint+"/server", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("User-Agent", userAgent)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("non 2xx status '%d' response during hetzner service discovery with role robot", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var servers serversList
	err = json.Unmarshal(b, &servers)
	if err != nil {
		return nil, err
	}

	targets := make([]model.LabelSet, len(servers))
	for i, server := range servers {
		labels := model.LabelSet{
			hetznerLabelRole:           model.LabelValue(HetznerRoleRobot),
			hetznerLabelServerID:       model.LabelValue(strconv.Itoa(server.Server.ServerNumber)),
			hetznerLabelServerName:     model.LabelValue(server.Server.ServerName),
			hetznerLabelDatacenter:     model.LabelValue(strings.ToLower(server.Server.Dc)),
			hetznerLabelPublicIPv4:     model.LabelValue(server.Server.ServerIP),
			hetznerLabelServerStatus:   model.LabelValue(server.Server.Status),
			hetznerLabelRobotProduct:   model.LabelValue(server.Server.Product),
			hetznerLabelRobotCancelled: model.LabelValue(fmt.Sprintf("%t", server.Server.Canceled)),

			model.AddressLabel: model.LabelValue(net.JoinHostPort(server.Server.ServerIP, strconv.FormatUint(uint64(d.port), 10))),
		}
		for _, subnet := range server.Server.Subnet {
			ip := net.ParseIP(subnet.IP)
			if ip.To4() == nil {
				labels[hetznerLabelPublicIPv6Network] = model.LabelValue(fmt.Sprintf("%s/%s", subnet.IP, subnet.Mask))
				break
			}

		}
		targets[i] = labels
	}
	return []*targetgroup.Group{{Source: "hetzner", Targets: targets}}, nil
}

type serversList []struct {
	Server struct {
		ServerIP     string `json:"server_ip"`
		ServerNumber int    `json:"server_number"`
		ServerName   string `json:"server_name"`
		Dc           string `json:"dc"`
		Status       string `json:"status"`
		Product      string `json:"product"`
		Canceled     bool   `json:"cancelled"`
		Subnet       []struct {
			IP   string `json:"ip"`
			Mask string `json:"mask"`
		} `json:"subnet"`
	} `json:"server"`
}
