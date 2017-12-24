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

package digitalocean

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"golang.org/x/oauth2"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	yaml_util "github.com/prometheus/prometheus/util/yaml"
)

const (
	dropletLabel               = model.MetaLabelPrefix + "digitalocean_"
	dropletLabelInstanceID     = dropletLabel + "instance_id"
	dropletLabelName           = dropletLabel + "instance_name"
	dropletLabelRegion         = dropletLabel + "region"
	dropletLabelInstanceStatus = dropletLabel + "instance_status"
	dropletLabelInstanceSize   = dropletLabel + "instance_size"
	dropletLabelPublicIPv4     = dropletLabel + "public_ipv4"
	dropletLabelPrivateIPv4    = dropletLabel + "private_ipv4"
	dropletLabelTag            = dropletLabel + "instance_tags"
)

var (
	digitaloceanSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_digitalocean_refresh_failures_total",
			Help: "The number of DigitalOcean SD scrape failures.",
		})
	digitaloceanSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_digitalocean_refresh_duration_seconds",
			Help: "The duration of a DigitalOcean SD refresh in seconds.",
		})

	DefaultSDConfig = SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(5 * time.Minute),
	}
)

// SDConfig is the configuration for DigitalOcean based service discovery.
type SDConfig struct {
	Port            int                `yaml:"port"`
	Token           config_util.Secret `yaml:"token,omitempty"`
	Region          string             `yaml:"region"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`

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

	return yaml_util.CheckOverflow(c.XXX, "digitalocean_sd_config")
}

func init() {
	prometheus.MustRegister(digitaloceanSDRefreshFailuresCount)
	prometheus.MustRegister(digitaloceanSDRefreshDuration)
}

// Discovery periodically performs DigitalOcean SD requests.
type Discovery struct {
	client   *godo.Client
	region   string
	interval time.Duration
	port     int
	logger   log.Logger
}

// TokenSource holds an OAuth token.
type TokenSource struct {
	AccessToken string
}

// Token returns an OAuth token.
func (t *TokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: t.AccessToken,
	}, nil
}

// NewDiscovery returns a new DigitalOcean Discovery which periodically
// refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	token := string(conf.Token)
	ts := &TokenSource{AccessToken: token}
	oauthClient := oauth2.NewClient(oauth2.NoContext, ts)
	c := godo.NewClient(oauthClient)

	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Discovery{
		client:   c,
		region:   conf.Region,
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
		logger:   logger,
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
		digitaloceanSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			digitaloceanSDRefreshFailuresCount.Inc()
		}
	}()

	droplets, err := listDroplets(d)
	if err != nil {
		return nil, fmt.Errorf("could not list DigitalOcean instances: %s", err)
	}

	tg, err = buildMetadata(d, droplets)
	if err != nil {
		return nil, fmt.Errorf("could not extract DigitalOcean metadata: %s", err)
	}

	return tg, nil
}

// listDroplets returns a list of godo.Droplets in the specified region.
func listDroplets(d *Discovery) ([]godo.Droplet, error) {
	ctx := context.TODO()
	dropletList := []godo.Droplet{}
	pageOpt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	for {
		droplets, resp, err := d.client.Droplets.List(ctx, pageOpt)
		if err != nil {
			return nil, err
		}

		for _, droplet := range droplets {
			if droplet.Region.Slug != d.region {
				continue
			}
			dropletList = append(dropletList, droplet)
		}

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}

		pageOpt.Page = page + 1
	}

	return dropletList, nil
}

// buildMetadata maps instance information retrieved from the
// DigitalOcean API onto metadata labels.
func buildMetadata(d *Discovery, droplets []godo.Droplet) (*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: fmt.Sprintf("DigitalOcean_%s", d.region),
	}

	for _, droplet := range droplets {
		privateIpAddr, err := droplet.PrivateIPv4()
		if err != nil {
			return nil, fmt.Errorf("could not list DigitalOcean instances: %s", err)
		}
		publicIpAddr, err := droplet.PublicIPv4()
		if err != nil {
			return nil, fmt.Errorf("could not list DigitalOcean instances: %s", err)
		}
		if privateIpAddr == "" {
			continue
		}

		labels := model.LabelSet{
			dropletLabelInstanceID:     model.LabelValue(strconv.Itoa(droplet.ID)),
			dropletLabelName:           model.LabelValue(droplet.Name),
			dropletLabelRegion:         model.LabelValue(droplet.Region.Slug),
			dropletLabelInstanceStatus: model.LabelValue(droplet.Status),
			dropletLabelInstanceSize:   model.LabelValue(droplet.Size.Slug),
			dropletLabelPublicIPv4:     model.LabelValue(publicIpAddr),
			dropletLabelPrivateIPv4:    model.LabelValue(privateIpAddr),
		}

		addr := fmt.Sprintf("%s:%d", privateIpAddr, d.port)
		labels[model.AddressLabel] = model.LabelValue(addr)

		if len(droplet.Tags) > 0 {
			sanitizedTags := []string{}
			for _, t := range droplet.Tags {
				st := strutil.SanitizeLabelName(t)
				sanitizedTags = append(sanitizedTags, st)
			}
			joinedTags := "," + strings.Join(sanitizedTags, ",") + ","
			labels[dropletLabelTag] = model.LabelValue(joinedTags)
		}

		tg.Targets = append(tg.Targets, labels)
	}

	return tg, nil
}
