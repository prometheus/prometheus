// Copyright The Prometheus Authors
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

package outscale

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	osc "github.com/outscale/osc-sdk-go/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
)

const (
	metaLabelPrefix = model.MetaLabelPrefix + "outscale_"
)

// DefaultSDConfig is the default Outscale Service Discovery configuration.
var DefaultSDConfig = SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
	Region:           "eu-west-2",
}

// SDConfig is the configuration for Outscale VM-based service discovery
// using the OUTSCALE API (OAPI).
type SDConfig struct {
	Endpoint         string                  `yaml:"endpoint,omitempty"`
	Region           string                  `yaml:"region"`
	AccessKey        string                  `yaml:"access_key"`
	SecretKey        config.Secret           `yaml:"secret_key"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	Port             int                     `yaml:"port"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &outscaleMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the discovery mechanism.
func (SDConfig) Name() string {
	return "outscale"
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Region == "" {
		return errors.New("outscale SD configuration requires a region")
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return errors.New("outscale SD configuration requires access_key and secret_key")
	}
	if c.Endpoint == "" {
		c.Endpoint = "https://api." + c.Region + ".outscale.com/api/v1"
	}
	return c.HTTPClientConfig.Validate()
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// Discovery periodically performs Outscale API requests. It implements the Discoverer interface.
type Discovery struct{}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*refresh.Discovery, error) {
	m, ok := opts.Metrics.(*outscaleMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	d, err := newVMDiscovery(conf)
	if err != nil {
		return nil, err
	}

	return refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "outscale",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	), nil
}

// loadClient builds an Outscale API client from SDConfig using the official osc-sdk-go (OAPI).
func loadClient(conf *SDConfig) (*osc.APIClient, *osc.Configuration, error) {
	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "outscale_sd")
	if err != nil {
		return nil, nil, fmt.Errorf("outscale SD client config: %w", err)
	}

	httpClient := &http.Client{
		Transport: rt,
		Timeout:   time.Duration(conf.RefreshInterval),
	}

	cfg := osc.NewConfiguration()
	cfg.HTTPClient = httpClient
	cfg.UserAgent = version.PrometheusUserAgent()
	// Use a single server URL (custom endpoint or default per-region).
	cfg.Servers = osc.ServerConfigurations{
		osc.ServerConfiguration{URL: conf.Endpoint, Description: "Outscale API"},
	}

	client := osc.NewAPIClient(cfg)
	return client, cfg, nil
}
