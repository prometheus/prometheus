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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
	"golang.org/x/net/context"
)

const (
	tritonLabel             = model.MetaLabelPrefix + "triton_"
	tritonLabelMachineId    = tritonLabel + "machine_id"
	tritonLabelMachineAlias = tritonLabel + "machine_alias"
	tritonLabelMachineBrand = tritonLabel + "machine_brand"
	tritonLabelMachineImage = tritonLabel + "machine_image"
	tritonLabelServerId     = tritonLabel + "server_id"
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
)

func init() {
	prometheus.MustRegister(refreshFailuresCount)
	prometheus.MustRegister(refreshDuration)
}

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
// the TargetProvider interface.
type Discovery struct {
	client   *http.Client
	interval time.Duration
	logger   log.Logger
	sdConfig *config.TritonSDConfig
}

// New returns a new Discovery which periodically refreshes its targets.
func New(logger log.Logger, conf *config.TritonSDConfig) (*Discovery, error) {
	tls, err := httputil.NewTLSConfig(conf.TLSConfig)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{TLSClientConfig: tls}
	client := &http.Client{Transport: transport}

	return &Discovery{
		client:   client,
		interval: time.Duration(conf.RefreshInterval),
		logger:   logger,
		sdConfig: conf,
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		d.logger.With("err", err).Error("Refreshing targets failed")
	} else {
		ch <- []*config.TargetGroup{tg}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				d.logger.With("err", err).Error("Refreshing targets failed")
			} else {
				ch <- []*config.TargetGroup{tg}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *config.TargetGroup, err error) {
	t0 := time.Now()
	defer func() {
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	var endpoint = fmt.Sprintf("https://%s:%d/v%d/discover", d.sdConfig.Endpoint, d.sdConfig.Port, d.sdConfig.Version)
	tg = &config.TargetGroup{
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
			tritonLabelMachineId:    model.LabelValue(container.VMUUID),
			tritonLabelMachineAlias: model.LabelValue(container.VMAlias),
			tritonLabelMachineBrand: model.LabelValue(container.VMBrand),
			tritonLabelMachineImage: model.LabelValue(container.VMImageUUID),
			tritonLabelServerId:     model.LabelValue(container.ServerUUID),
		}
		addr := fmt.Sprintf("%s.%s:%d", container.VMUUID, d.sdConfig.DNSSuffix, d.sdConfig.Port)
		labels[model.AddressLabel] = model.LabelValue(addr)
		tg.Targets = append(tg.Targets, labels)
	}

	return tg, nil
}
