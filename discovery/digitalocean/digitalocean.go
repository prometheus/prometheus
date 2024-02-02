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

package digitalocean

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	doLabel            = model.MetaLabelPrefix + "digitalocean_"
	doLabelID          = doLabel + "droplet_id"
	doLabelName        = doLabel + "droplet_name"
	doLabelImage       = doLabel + "image"
	doLabelImageName   = doLabel + "image_name"
	doLabelPrivateIPv4 = doLabel + "private_ipv4"
	doLabelPublicIPv4  = doLabel + "public_ipv4"
	doLabelPublicIPv6  = doLabel + "public_ipv6"
	doLabelRegion      = doLabel + "region"
	doLabelSize        = doLabel + "size"
	doLabelStatus      = doLabel + "status"
	doLabelFeatures    = doLabel + "features"
	doLabelTags        = doLabel + "tags"
	doLabelVPC         = doLabel + "vpc"
	separator          = ","
)

// DefaultSDConfig is the default DigitalOcean SD configuration.
var DefaultSDConfig = SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &digitaloceanMetrics{
		refreshMetrics: rmi,
	}
}

// SDConfig is the configuration for DigitalOcean based service discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "digitalocean" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return c.HTTPClientConfig.Validate()
}

// Discovery periodically performs DigitalOcean requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client *godo.Client
	port   int
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*digitaloceanMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	d := &Discovery{
		port: conf.Port,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "digitalocean_sd")
	if err != nil {
		return nil, err
	}

	d.client, err = godo.New(
		&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		},
		godo.SetUserAgent(fmt.Sprintf("Prometheus/%s", version.Version)),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up digital ocean agent: %w", err)
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "digitalocean",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DigitalOcean",
	}

	droplets, err := d.listDroplets(ctx)
	if err != nil {
		return nil, err
	}
	for _, droplet := range droplets {
		if droplet.Networks == nil || len(droplet.Networks.V4) == 0 {
			continue
		}

		privateIPv4, err := droplet.PrivateIPv4()
		if err != nil {
			return nil, fmt.Errorf("error while reading private IPv4 of droplet %d: %w", droplet.ID, err)
		}
		publicIPv4, err := droplet.PublicIPv4()
		if err != nil {
			return nil, fmt.Errorf("error while reading public IPv4 of droplet %d: %w", droplet.ID, err)
		}
		publicIPv6, err := droplet.PublicIPv6()
		if err != nil {
			return nil, fmt.Errorf("error while reading public IPv6 of droplet %d: %w", droplet.ID, err)
		}

		labels := model.LabelSet{
			doLabelID:          model.LabelValue(fmt.Sprintf("%d", droplet.ID)),
			doLabelName:        model.LabelValue(droplet.Name),
			doLabelImage:       model.LabelValue(droplet.Image.Slug),
			doLabelImageName:   model.LabelValue(droplet.Image.Name),
			doLabelPrivateIPv4: model.LabelValue(privateIPv4),
			doLabelPublicIPv4:  model.LabelValue(publicIPv4),
			doLabelPublicIPv6:  model.LabelValue(publicIPv6),
			doLabelRegion:      model.LabelValue(droplet.Region.Slug),
			doLabelSize:        model.LabelValue(droplet.SizeSlug),
			doLabelStatus:      model.LabelValue(droplet.Status),
			doLabelVPC:         model.LabelValue(droplet.VPCUUID),
		}

		addr := net.JoinHostPort(publicIPv4, strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		if len(droplet.Features) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider feature positions.
			features := separator + strings.Join(droplet.Features, separator) + separator
			labels[doLabelFeatures] = model.LabelValue(features)
		}

		if len(droplet.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(droplet.Tags, separator) + separator
			labels[doLabelTags] = model.LabelValue(tags)
		}

		tg.Targets = append(tg.Targets, labels)
	}
	return []*targetgroup.Group{tg}, nil
}

func (d *Discovery) listDroplets(ctx context.Context) ([]godo.Droplet, error) {
	var (
		droplets []godo.Droplet
		opts     = &godo.ListOptions{}
	)
	for {
		paginatedDroplets, resp, err := d.client.Droplets.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("error while listing droplets page %d: %w", opts.Page, err)
		}
		droplets = append(droplets, paginatedDroplets...)
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}

		opts.Page = page + 1
	}
	return droplets, nil
}
