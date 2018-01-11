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
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/mwitkow/go-conntrack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/httputil"
	yaml_util "github.com/prometheus/prometheus/util/yaml"
)

const (
	tritonLabel             = model.MetaLabelPrefix + "triton_"
	tritonLabelMachineID    = tritonLabel + "machine_id"
	tritonLabelMachineAlias = tritonLabel + "machine_alias"
	tritonLabelMachineBrand = tritonLabel + "machine_brand"
	tritonLabelMachineImage = tritonLabel + "machine_image"
	tritonLabelServerID     = tritonLabel + "server_id"
	namespace               = "prometheus"
)

var (
	refreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_triton_refresh_failures_total",
			Help: "The number of Triton-SD scrape failures.",
		})
	refreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_triton_refresh_duration_seconds",
			Help: "The duration of a Triton-SD refresh in seconds.",
		})
	// DefaultSDConfig is the default Triton SD configuration.
	DefaultSDConfig = SDConfig{
		Port:            9163,
		RefreshInterval: model.Duration(60 * time.Second),
		Version:         1,
	}
)

// SDConfig is the configuration for Triton based service discovery.
type SDConfig struct {
	Account         string                `yaml:"account"`
	DNSSuffix       string                `yaml:"dns_suffix"`
	Endpoint        string                `yaml:"endpoint"`
	Port            int                   `yaml:"port"`
	RefreshInterval model.Duration        `yaml:"refresh_interval,omitempty"`
	TLSConfig       config_util.TLSConfig `yaml:"tls_config,omitempty"`
	Version         int                   `yaml:"version"`
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Account == "" {
		return fmt.Errorf("Triton SD configuration requires an account")
	}
	if c.DNSSuffix == "" {
		return fmt.Errorf("Triton SD configuration requires a dns_suffix")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("Triton SD configuration requires an endpoint")
	}
	if c.RefreshInterval <= 0 {
		return fmt.Errorf("Triton SD configuration requires RefreshInterval to be a positive integer")
	}
	return yaml_util.CheckOverflow(c.XXX, "triton_sd_config")
}

func init() {
	prometheus.MustRegister(refreshFailuresCount)
	prometheus.MustRegister(refreshDuration)
}

// DiscoveryResponse models a JSON response from the Triton discovery.
type DiscoveryResponse struct {
	Containers []struct {
		ServerUUID  string `json:"server_uuid"`
		VMAlias     string `json:"vm_alias"`
		VMBrand     string `json:"vm_brand"`
		VMImageUUID string `json:"vm_image_uuid"`
		VMUUID      string `json:"vm_uuid"`
	} `json:"containers"`
}

// Discovery periodically performs Triton-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	client   *http.Client
	interval time.Duration
	logger   log.Logger
	sdConfig *SDConfig
}

// New returns a new Discovery which periodically refreshes its targets.
func New(logger log.Logger, conf *SDConfig) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	tls, err := httputil.NewTLSConfig(conf.TLSConfig)
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

	return &Discovery{
		client:   client,
		interval: time.Duration(conf.RefreshInterval),
		logger:   logger,
		sdConfig: conf,
	}, nil
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer close(ch)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		level.Error(d.logger).Log("msg", "Refreshing targets failed", "err", err)
	} else {
		ch <- []*targetgroup.Group{tg}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				level.Error(d.logger).Log("msg", "Refreshing targets failed", "err", err)
			} else {
				ch <- []*targetgroup.Group{tg}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()
	defer func() {
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	var endpoint = fmt.Sprintf("https://%s:%d/v%d/discover", d.sdConfig.Endpoint, d.sdConfig.Port, d.sdConfig.Version)
	tg = &targetgroup.Group{
		Source: endpoint,
	}

	resp, err := d.client.Get(endpoint)
	if err != nil {
		return tg, fmt.Errorf("an error occurred when requesting targets from the discovery endpoint. %s", err)
	}

	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return tg, fmt.Errorf("an error occurred when reading the response body. %s", err)
	}

	dr := DiscoveryResponse{}
	err = json.Unmarshal(data, &dr)
	if err != nil {
		return tg, fmt.Errorf("an error occurred unmarshaling the disovery response json. %s", err)
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
		tg.Targets = append(tg.Targets, labels)
	}

	return tg, nil
}
