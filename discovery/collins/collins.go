// Copyright 2015 The Prometheus Authors
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

package collins

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"gopkg.in/tumblr/go-collins.v0/collins"
)

const (
	// CollinsMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	collinsLabel = model.MetaLabelPrefix + "collins_"
)

var (
	collinsSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_collins_refresh_failures_total",
			Help: "Number of Collins-SD refresh failures.",
		})
	collinsGroupSDRefreshDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_collins_group_refresh_duration_seconds",
			Help: "The duration of a Collins Group SD refresh in seconds.",
		},
		[]string{"group"},
	)

	// DefaultSDConfig is the default Collins SD configuration.
	DefaultSDConfig = SDConfig{
		// Default Developer credentials for collins
		// https://tumblr.github.io/collins/#credentials-default
		ClientUser:   "blake",
		ClientSecret: "admin:first",
		CollinsURL:   "http://localhost:9000",
		AttributeDiscovery: AttributeDiscovery{
			Labels: []string{"hostname", "nodeclass", "pool", "primary_role", "secondary_role"},
		},
		// Refresh Interval default is 15 minutes.
		// Collins assets should not change state frequently.
		RefreshInterval: model.Duration(15 * time.Minute),
		PortScrapeConfig: []*PortScrapeConfig{
			{
				AttrMatch: map[string]string{
					"nodeclass":    "nodeclass1",
					"primary_role": "primary_role1",
				},
				// Port to scrape for discovered assets.
				// Ex: 9100 is for preferred port for node_exporter.
				// https://github.com/prometheus/prometheus/wiki/Default-port-allocations
				Ports: []int{
					9100,
				},
			},
		},
	}
)

// SDConfig is the configuration for Collins based service discovery
type SDConfig struct {
	CollinsURL         string              `yaml:"collins_url,omitempty"`
	ClientUser         string              `yaml:"collins_user,omitempty"`
	ClientSecret       config_util.Secret  `yaml:"collins_pass,omitempty"`
	AttributeDiscovery AttributeDiscovery  `yaml:"attributes,omitempty"`
	Query              string              `yaml:"query"`
	RefreshInterval    model.Duration      `yaml:"refresh_interval,omitempty"`
	PortScrapeConfig   []*PortScrapeConfig `yaml:"port_scrape_config,omitempty"`
	Group              string              `yaml:"group"`
}

type AttributeDiscovery struct {
	Labels []string `yaml:"labels,omitempty"`
}

type PortScrapeConfig struct {
	AttrMatch map[string]string `yaml:"attribute_match,omitempty"`
	Ports     []int             `yaml:"ports"`
}

type Asset struct {
	collins.Asset
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Query == "" {
		return fmt.Errorf("collins SD configuration requires a collins query to search")
	}
	if c.Group == "" {
		return fmt.Errorf("collins SD configuration requires a scrape group per config")
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AttributeDiscovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = AttributeDiscovery{}
	type plain AttributeDiscovery
	return unmarshal((*plain)(c))
}

func init() {
	prometheus.MustRegister(collinsSDRefreshFailuresCount)
	prometheus.MustRegister(collinsGroupSDRefreshDuration)
}

// Discovery periodically performs Collins-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	cfg      *SDConfig
	interval time.Duration
	logger   log.Logger
}

// NewDiscovery returns a new CollinsDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &Discovery{
		cfg:      cfg,
		interval: time.Duration(cfg.RefreshInterval),
		logger:   logger,
	}
}

func (d *Discovery) fetchLabels() []string {
	labels := d.cfg.AttributeDiscovery.Labels
	if len(labels) == 0 {
		labels = DefaultSDConfig.AttributeDiscovery.Labels
	}
	return labels
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tg, err := d.refresh()
		if err != nil {
			level.Error(d.logger).Log("msg", "Unable to refresh during Collins discovery", "err", err)
		} else {
			select {
			case <-ctx.Done():
			case ch <- []*targetgroup.Group{tg}:
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// Fetches all ports we want to query for an asset based on port_scrape_config
// If any of the configs returns an empty port list, we drop all ports and return an empty list
func (ca Asset) fetchTargetPorts(portScrapeConfig []*PortScrapeConfig) []int {
	targetPorts := make([]int, 0, 10)
	for _, c := range portScrapeConfig {
		if ca.fetchPortsToScrape(c) {
			if len(c.Ports) == 0 {
				return c.Ports
			}
			targetPorts = append(targetPorts, c.Ports...)
		}
	}
	return targetPorts
}

func (ca Asset) searchAttribute(search string) string {
	if val, ok := ca.Attributes["0"][strings.ToUpper(search)]; ok {
		return val
	}
	return ""
}

// Check if an asset matches a given port scrape config
func (ca Asset) fetchPortsToScrape(portScrapeConfig *PortScrapeConfig) bool {

	if portScrapeConfig.AttrMatch != nil {
		for k, v := range portScrapeConfig.AttrMatch {
			val := ca.searchAttribute(k)
			if val == "" {
				return false
			}
			if !strings.EqualFold(val, v) {
				return false
			}
		}
	}
	return true
}

// Generates a labelset of collins metalabels (`__meta_collins`).
// - always try to generate `__address__` with the first address found in `ADDRESSES`.
// - return empty labelSet when no `ADDRESSES` found.
func (ca Asset) toLabelset(labelsToFetch []string) model.LabelSet {
	labels := make(model.LabelSet)

	if len(ca.Addresses) == 0 {
		// there is no way to actually monitor this asset...
		return model.LabelSet{}
	}

	// helper func for setting labels. forces lowercase and trimming
	setLabel := func(k, v string) {
		k = collinsLabel + strings.TrimSpace(strings.ToLower(k))
		v = strings.TrimSpace(v)
		labels[model.LabelName(k)] = model.LabelValue(v)
	}

	// Generate __meta_collins_status/state
	// these values are represented by different structs
	// and not the default Attributes Struct in Collins.
	setLabel("status", ca.Metadata.Status)
	setLabel("state", ca.Metadata.State.Name)

	// set host, __address__
	setLabel("host", ca.searchAttribute("hostname"))
	labels[model.AddressLabel] = model.LabelValue(ca.Addresses[0].Address)

	for _, labelToFetch := range labelsToFetch {
		labelToFetch := strings.TrimSpace(labelToFetch)
		if labelVal := ca.searchAttribute(labelToFetch); labelVal != "" {
			setLabel(labelToFetch, labelVal)
		}
	}
	return labels
}

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	defer level.Debug(d.logger).Log("msg", "Collins discovery completed")

	t0 := time.Now()
	defer func() {
		collinsGroupSDRefreshDuration.WithLabelValues(d.cfg.Group).Observe(time.Since(t0).Seconds())
		if err != nil {
			collinsSDRefreshFailuresCount.Inc()
		}
	}()
	tg = &targetgroup.Group{}
	client, err := collins.NewClient(d.cfg.ClientUser, string(d.cfg.ClientSecret), d.cfg.CollinsURL)
	if err != nil {
		return tg, fmt.Errorf("could not create collins client: %s", err)
	}

	query := collins.AssetFindOpts{
		Query:    d.cfg.Query,
		PageOpts: collins.PageOpts{Size: 99999},
	}

	assets, _, err := client.Assets.Find(&query)
	if err != nil {
		return nil, fmt.Errorf("could not fetch assets from Collins: %s", err)
	}

	level.Debug(d.logger).Log("msg", "Found assets during Collins discovery.", "count", len(assets))

	for _, asset := range assets {

		ca := Asset{asset}
		// determine which filters apply and get the list of ports we need to scrape
		ports := ca.fetchTargetPorts(d.cfg.PortScrapeConfig)

		// if no target ports defined, do not monitor. which is bad, but /shrug
		if len(ports) == 0 {
			level.Debug(d.logger).Log("msg", "Asset dropped due to no ports.", asset.Metadata.Tag)
			continue // go to the next asset
		}

		// generate labelset, as specified by attributes['labels']
		labelSet := ca.toLabelset(d.cfg.AttributeDiscovery.Labels)

		// if we get no labels... do not monitor. which is bad, but /shrug
		if len(labelSet) == 0 {
			level.Debug(d.logger).Log("msg", "Asset dropped due to no labels (including address!)", asset.Metadata.Tag)
			continue // go to the next asset
		}

		// modify __address__ to scrape ports as per port_scrape_config
		for _, port := range ports {
			portScrapeLabelSet := make(model.LabelSet)
			for k, v := range labelSet {
				// if the port is 80, then we do not need to modify and append it
				if k == model.AddressLabel && port != 80 {
					portScrapeLabelSet[k] = model.LabelValue(fmt.Sprintf("%s:%d", v, port))
				} else {
					portScrapeLabelSet[k] = model.LabelValue(v)
				}
			}
			tg.Targets = append(tg.Targets, portScrapeLabelSet)
		}
	}
	return tg, nil
}
