// Copyright 2017 The Prometheus Authors
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

package triton

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-kit/log"
	conntrack "github.com/mwitkow/go-conntrack"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	tritonLabel             = model.MetaLabelPrefix + "triton_"
	tritonLabelGroups       = tritonLabel + "groups"
	tritonLabelMachineID    = tritonLabel + "machine_id"
	tritonLabelMachineAlias = tritonLabel + "machine_alias"
	tritonLabelMachineBrand = tritonLabel + "machine_brand"
	tritonLabelMachineImage = tritonLabel + "machine_image"
	tritonLabelServerID     = tritonLabel + "server_id"
)

// DefaultSDConfig is the default Triton SD configuration.
var DefaultSDConfig = SDConfig{
	Role:            "container",
	Port:            9163,
	RefreshInterval: model.Duration(60 * time.Second),
	Version:         1,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for Triton based service discovery.
type SDConfig struct {
	Account         string           `yaml:"account"`
	Role            string           `yaml:"role"`
	DNSSuffix       string           `yaml:"dns_suffix"`
	Endpoint        string           `yaml:"endpoint"`
	Groups          []string         `yaml:"groups,omitempty"`
	Port            int              `yaml:"port"`
	RefreshInterval model.Duration   `yaml:"refresh_interval,omitempty"`
	TLSConfig       config.TLSConfig `yaml:"tls_config,omitempty"`
	Version         int              `yaml:"version"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "triton" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return New(opts.Logger, c)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.TLSConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Role != "container" && c.Role != "cn" {
		return errors.New("triton SD configuration requires role to be 'container' or 'cn'")
	}
	if c.Account == "" {
		return errors.New("triton SD configuration requires an account")
	}
	if c.DNSSuffix == "" {
		return errors.New("triton SD configuration requires a dns_suffix")
	}
	if c.Endpoint == "" {
		return errors.New("triton SD configuration requires an endpoint")
	}
	if c.RefreshInterval <= 0 {
		return errors.New("triton SD configuration requires RefreshInterval to be a positive integer")
	}
	return nil
}

// DiscoveryResponse models a JSON response from the Triton discovery.
type DiscoveryResponse struct {
	Containers []struct {
		Groups      []string `json:"groups"`
		ServerUUID  string   `json:"server_uuid"`
		VMAlias     string   `json:"vm_alias"`
		VMBrand     string   `json:"vm_brand"`
		VMImageUUID string   `json:"vm_image_uuid"`
		VMUUID      string   `json:"vm_uuid"`
	} `json:"containers"`
}

// ComputeNodeDiscoveryResponse models a JSON response from the Triton discovery /gz/ endpoint.
type ComputeNodeDiscoveryResponse struct {
	ComputeNodes []struct {
		ServerUUID     string `json:"server_uuid"`
		ServerHostname string `json:"server_hostname"`
	} `json:"cns"`
}

// Discovery periodically performs Triton-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client   *http.Client
	interval time.Duration
	sdConfig *SDConfig
}

// New returns a new Discovery which periodically refreshes its targets.
func New(logger log.Logger, conf *SDConfig) (*Discovery, error) {
	tls, err := config.NewTLSConfig(&conf.TLSConfig)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tls,
		DialContext: conntrack.NewDialContextFunc(
			conntrack.DialWithTracing(),
			conntrack.DialWithName("triton_sd"),
		),
	}
	client := &http.Client{Transport: transport}

	d := &Discovery{
		client:   client,
		interval: time.Duration(conf.RefreshInterval),
		sdConfig: conf,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"triton",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

// triton-cmon has two discovery endpoints:
// https://github.com/joyent/triton-cmon/blob/master/lib/endpoints/discover.js
//
// The default endpoint exposes "containers", otherwise called "virtual machines" in triton,
// which are (branded) zones running on the triton platform.
//
// The /gz/ endpoint exposes "compute nodes", also known as "servers" or "global zones",
// on which the "containers" are running.
//
// As triton is not internally consistent in using these names,
// the terms as used in triton-cmon are used here.

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	var endpointFormat string
	switch d.sdConfig.Role {
	case "container":
		endpointFormat = "https://%s:%d/v%d/discover"
	case "cn":
		endpointFormat = "https://%s:%d/v%d/gz/discover"
	default:
		return nil, fmt.Errorf("unknown role '%s' in configuration", d.sdConfig.Role)
	}
	endpoint := fmt.Sprintf(endpointFormat, d.sdConfig.Endpoint, d.sdConfig.Port, d.sdConfig.Version)
	if len(d.sdConfig.Groups) > 0 {
		groups := url.QueryEscape(strings.Join(d.sdConfig.Groups, ","))
		endpoint = fmt.Sprintf("%s?groups=%s", endpoint, groups)
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("an error occurred when requesting targets from the discovery endpoint: %w", err)
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("an error occurred when reading the response body: %w", err)
	}

	// The JSON response body is different so it needs to be processed/mapped separately.
	switch d.sdConfig.Role {
	case "container":
		return d.processContainerResponse(data, endpoint)
	case "cn":
		return d.processComputeNodeResponse(data, endpoint)
	default:
		return nil, fmt.Errorf("unknown role '%s' in configuration", d.sdConfig.Role)
	}
}

func (d *Discovery) processContainerResponse(data []byte, endpoint string) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: endpoint,
	}

	dr := DiscoveryResponse{}
	err := json.Unmarshal(data, &dr)
	if err != nil {
		return nil, fmt.Errorf("an error occurred unmarshaling the discovery response json: %w", err)
	}

	for _, container := range dr.Containers {
		labels := model.LabelSet{
			tritonLabelMachineID:    model.LabelValue(container.VMUUID),
			tritonLabelMachineAlias: model.LabelValue(container.VMAlias),
			tritonLabelMachineBrand: model.LabelValue(container.VMBrand),
			tritonLabelMachineImage: model.LabelValue(container.VMImageUUID),
			tritonLabelServerID:     model.LabelValue(container.ServerUUID),
		}
		addr := fmt.Sprintf("%s.%s:%d", container.VMUUID, d.sdConfig.DNSSuffix, d.sdConfig.Port)
		labels[model.AddressLabel] = model.LabelValue(addr)

		if len(container.Groups) > 0 {
			name := "," + strings.Join(container.Groups, ",") + ","
			labels[tritonLabelGroups] = model.LabelValue(name)
		}

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}

func (d *Discovery) processComputeNodeResponse(data []byte, endpoint string) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: endpoint,
	}

	dr := ComputeNodeDiscoveryResponse{}
	err := json.Unmarshal(data, &dr)
	if err != nil {
		return nil, fmt.Errorf("an error occurred unmarshaling the compute node discovery response json: %w", err)
	}

	for _, cn := range dr.ComputeNodes {
		labels := model.LabelSet{
			tritonLabelMachineID:    model.LabelValue(cn.ServerUUID),
			tritonLabelMachineAlias: model.LabelValue(cn.ServerHostname),
		}
		addr := fmt.Sprintf("%s.%s:%d", cn.ServerUUID, d.sdConfig.DNSSuffix, d.sdConfig.Port)
		labels[model.AddressLabel] = model.LabelValue(addr)

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}
