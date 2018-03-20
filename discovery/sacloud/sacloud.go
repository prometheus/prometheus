// Copyright 2018 The Prometheus Authors
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

package sacloud

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/sacloud/libsacloud/api"
	"github.com/sacloud/libsacloud/sacloud"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	yaml_util "github.com/prometheus/prometheus/util/yaml"
)

const (
	sacloudLabel              = model.MetaLabelPrefix + "sacloud_"
	sacloudLabelZone          = sacloudLabel + "zone"
	sacloudLabelResourceID    = sacloudLabel + "resource_id"
	sacloudLabelName          = sacloudLabel + "name"
	sacloudLabelInstanceState = sacloudLabel + "instance_state"
	sacloudLabelInstanceType  = sacloudLabel + "instance_type"
	sacloudLabelIP            = sacloudLabel + "ip"
	sacloudLabelTag           = sacloudLabel + "instances_tags"
)

var (
	sacloudSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_sacloud_refresh_failures_total",
			Help: "The number of sacloud-SD scrape failures.",
		})
	sacloudSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_sacloud_refresh_duration_seconds",
			Help: "The duration of a sacloud-SD refresh in seconds.",
		})
	// DefaultSDConfig is the default sacloud SD configuration.
	DefaultSDConfig = SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

// SDConfig is the configuration for sacloud based service discovery.
type SDConfig struct {
	Port            int                `yaml:"port"`
	Zone            string             `yaml:"zone"`
	Token           string             `yaml:"token"`
	Secret          config_util.Secret `yaml:"secret"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`
	FilterTag       string             `yaml:"filter_tag,omitempty"`

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

	return yaml_util.CheckOverflow(c.XXX, "sacloud_sd_config")
}

func init() {
	prometheus.MustRegister(sacloudSDRefreshFailuresCount)
	prometheus.MustRegister(sacloudSDRefreshDuration)
}

// Discovery periodically performs sacloud SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	client    *api.Client
	zone      string
	interval  time.Duration
	port      int
	filterTag string
	logger    log.Logger
}

// NewDiscovery returns a new sacloud Discovery which periodically
// refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	secret := string(conf.Secret)
	c := api.NewClient(conf.Token, secret, conf.Zone)

	// [TODO] set UserAgent

	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Discovery{
		client:    c,
		zone:      conf.Zone,
		interval:  time.Duration(conf.RefreshInterval),
		port:      conf.Port,
		logger:    logger,
		filterTag: conf.FilterTag,
	}
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
	} else {
		select {
		case ch <- []*targetgroup.Group{tg}:
		case <-ctx.Done():
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
				continue
			}

			select {
			case ch <- []*targetgroup.Group{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()
	defer func() {
		sacloudSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			sacloudSDRefreshFailuresCount.Inc()
		}
	}()

	servers, err := listServers(d)
	if err != nil {
		return nil, fmt.Errorf("could not list sacloud instances: %s", err)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("target node is not found")
	}

	tg, err = buildMetadata(d, servers)
	if err != nil {
		return nil, fmt.Errorf("could not extract sacloud metadata: %s", err)
	}

	return tg, nil
}

// listServers return a list of sacloud.Server
func listServers(d *Discovery) ([]sacloud.Server, error) {
	// if set SDConfig.FilterTag, monitoring only tagged server.
	res, err := d.client.Server.WithTag(d.filterTag).Find()
	if err != nil {
		return nil, err
	}

	return list, nil
}

// buildMetadata maps instance information retrieved from the
// sacloud API onto metadata labels.
func buildMetadata(d *Discovery, servers []sacloud.Server) (*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: fmt.Sprintf("sacloud_%s", d.zone),
	}

	for _, s := range servers {
		IPAddress := s.GetFirstInterface().GetUserIPAddress() // maybe private address (connect to switch)
		if IPAddress == "" {
			IPAddress = s.GetFirstInterface().GetIPAddress() // public ip set by sacloud
		}
		if IPAddress == "" {
			continue
		}

		labels := model.LabelSet{
			sacloudLabelZone:          model.LabelValue(s.GetZoneName()),
			sacloudLabelResourceID:    model.LabelValue(s.Resource.GetStrID()),
			sacloudLabelName:          model.LabelValue(s.Name),
			sacloudLabelInstanceState: model.LabelValue(s.GetInstanceStatus()),
			sacloudLabelIP:            model.LabelValue(IPAddress),
		}

		if len(s.Tags) > 0 {
			sanitizedTags := []string{}
			for _, t := range s.Tags {
				st := strutil.SanitizeLabelName(t)
				sanitizedTags = append(sanitizedTags, st)
			}
			joinedTags := "," + strings.Join(sanitizedTags, ",") + ","
			labels[sacloudLabelTag] = model.LabelValue(joinedTags)
		}

		labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(IPAddress, fmt.Sprintf("%d", d.port)))

		tg.Targets = append(tg.Targets, labels)
	}

	return tg, nil
}
