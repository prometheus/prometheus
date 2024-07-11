// Copyright 2023 The Prometheus Authors
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

package tailscale

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"tailscale.com/client/tailscale"
)

const (
	tsLabel                                 = model.MetaLabelPrefix + "tailscale_"
	deviceLabel                             = tsLabel + "device_"
	tsLabelDeviceAddresses                  = deviceLabel + "addresses"
	tsLabelDeviceID                         = deviceLabel + "id"
	tsLabelDeviceName                       = deviceLabel + "name"
	tsLabelDeviceHostname                   = deviceLabel + "hostname"
	tsLabelDeviceUser                       = deviceLabel + "user"
	tsLabelDeviceClientVersion              = deviceLabel + "client_version"
	tsLabelDeviceOS                         = deviceLabel + "os"
	tsLabelDeviceUpdateAvailable            = deviceLabel + "update_available"
	tsLabelDeviceAuthorized                 = deviceLabel + "authorized"
	tsLabelDeviceIsExternal                 = deviceLabel + "is_external"
	tsLabelDeviceKeyExpiryDisabled          = deviceLabel + "key_expiry_disabled"
	tsLabelDeviceBlocksIncommingConnections = deviceLabel + "blocks_incoming_connections"
	tsLabelDeviceEnabledRoutes              = deviceLabel + "enabled_routes"
	tsLabelDeviceAdvertisedRoutes           = deviceLabel + "advertised_routes"
	tsLabelDeviceTags                       = deviceLabel + "tags"
)

var (
	DefaultSDConfig = SDConfig{
		RefreshInterval: model.Duration(60 * time.Second),
		URL:             "https://api.tailscale.com",
		Tailnet:         "-",
		TagSeparator:    ",",
	}
	failuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_tailscale_failures_total",
			Help: "Number of tailscale service discovery refresh failures.",
		})
)

func init() {
	tailscale.I_Acknowledge_This_API_Is_Unstable = true
	discovery.RegisterConfig(&SDConfig{})
	prometheus.MustRegister(failuresCount)
}

type SDConfig struct {
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	TagSeparator    string         `yaml:"tag_separator,omitempty"`
	Tailnet         string         `yaml:"tailnet"`
	URL             string         `yaml:"api_url"`
	APIToken        string         `yaml:"api_token,omitempty"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &tailscaleMetrics{
		refreshMetrics: rmi,
	}
}

func (c *SDConfig) Name() string { return "tailscale" }

func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.URL == "" {
		return fmt.Errorf("URL is missing")
	}
	parsedURL, err := url.Parse(c.URL)
	if err != nil {
		return err
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL scheme must be 'http' or 'https'")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("host is missing in URL")
	}
	if c.APIToken == "" {
		return fmt.Errorf("API token is missing")
	}
	return nil
}

type Discovery struct {
	*refresh.Discovery
	refreshInterval time.Duration
	port            int
	tagSeparator    string
	url             string
	client          *tailscale.Client
}

func NewDiscovery(conf *SDConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*tailscaleMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	if conf.URL == "" {
		conf.URL = "https://api.tailscale.com"
	}

	tsClient := tailscale.NewClient(
		conf.Tailnet,
		tailscale.APIKey(conf.APIToken),
	)
	tsClient.BaseURL = conf.URL

	d := &Discovery{
		port:            conf.Port,
		client:          tsClient,
		url:             conf.URL,
		tagSeparator:    conf.TagSeparator,
		refreshInterval: time.Duration(conf.RefreshInterval),
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "tailscale",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tsDevices, err := d.client.Devices(ctx, tailscale.DeviceDefaultFields)
	if err != nil {
		failuresCount.Inc()
		return nil, err
	}

	tg := &targetgroup.Group{
		// Use a pseudo-URL as source.
		Source: d.url,
	}

	for _, device := range tsDevices {
		labels := model.LabelSet{
			tsLabelDeviceID:                         model.LabelValue(device.DeviceID),
			tsLabelDeviceOS:                         model.LabelValue(device.OS),
			tsLabelDeviceClientVersion:              model.LabelValue(device.ClientVersion),
			tsLabelDeviceName:                       model.LabelValue(device.Name),
			tsLabelDeviceHostname:                   model.LabelValue(device.Hostname),
			tsLabelDeviceUser:                       model.LabelValue(device.User),
			tsLabelDeviceIsExternal:                 model.LabelValue(strconv.FormatBool(device.IsExternal)),
			tsLabelDeviceAuthorized:                 model.LabelValue(strconv.FormatBool(device.Authorized)),
			tsLabelDeviceUpdateAvailable:            model.LabelValue(strconv.FormatBool(device.UpdateAvailable)),
			tsLabelDeviceKeyExpiryDisabled:          model.LabelValue(strconv.FormatBool(device.KeyExpiryDisabled)),
			tsLabelDeviceBlocksIncommingConnections: model.LabelValue(strconv.FormatBool(device.BlocksIncomingConnections)),
		}

		addr := net.JoinHostPort(device.Addresses[0], strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		if len(device.Addresses) > 0 {
			addresses := d.tagSeparator + strings.Join(device.Addresses, d.tagSeparator) + d.tagSeparator
			labels[tsLabelDeviceAddresses] = model.LabelValue(addresses)
		}

		if len(device.Tags) > 0 {
			tags := d.tagSeparator + strings.Join(device.Tags, d.tagSeparator) + d.tagSeparator
			labels[tsLabelDeviceTags] = model.LabelValue(tags)
		}
		tg.Targets = append(tg.Targets, labels)

		if len(device.EnabledRoutes) > 0 {
			tags := d.tagSeparator + strings.Join(device.EnabledRoutes, d.tagSeparator) + d.tagSeparator
			labels[tsLabelDeviceEnabledRoutes] = model.LabelValue(tags)
		}
		if len(device.AdvertisedRoutes) > 0 {
			tags := d.tagSeparator + strings.Join(device.AdvertisedRoutes, d.tagSeparator) + d.tagSeparator
			labels[tsLabelDeviceAdvertisedRoutes] = model.LabelValue(tags)
		}
	}

	return []*targetgroup.Group{tg}, nil
}
