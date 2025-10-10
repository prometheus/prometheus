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

package stackit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	stackitLabelPrefix           = model.MetaLabelPrefix + "stackit_"
	stackitLabelProject          = stackitLabelPrefix + "project"
	stackitLabelID               = stackitLabelPrefix + "id"
	stackitLabelName             = stackitLabelPrefix + "name"
	stackitLabelStatus           = stackitLabelPrefix + "status"
	stackitLabelPowerStatus      = stackitLabelPrefix + "power_status"
	stackitLabelAvailabilityZone = stackitLabelPrefix + "availability_zone"
	stackitLabelPublicIPv4       = stackitLabelPrefix + "public_ipv4"
)

var userAgent = version.PrometheusUserAgent()

// DefaultSDConfig is the default STACKIT SD configuration.
var DefaultSDConfig = SDConfig{
	Region:           "eu01",
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for STACKIT based service discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	Project               string         `yaml:"project"`
	RefreshInterval       model.Duration `yaml:"refresh_interval,omitempty"`
	Port                  int            `yaml:"port,omitempty"`
	Region                string         `yaml:"region,omitempty"`
	Endpoint              string         `yaml:"endpoint,omitempty"`
	ServiceAccountKey     string         `yaml:"service_account_key,omitempty"`
	PrivateKey            string         `yaml:"private_key,omitempty"`
	ServiceAccountKeyPath string         `yaml:"service_account_key_path,omitempty"`
	PrivateKeyPath        string         `yaml:"private_key_path,omitempty"`
	CredentialsFilePath   string         `yaml:"credentials_file_path,omitempty"`

	// For testing only
	tokenURL string
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &stackitMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "stackit" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
}

type refresher interface {
	refresh(context.Context) ([]*targetgroup.Group, error)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if c.Endpoint == "" && c.Region == "" {
		return errors.New("stackit_sd: endpoint and region missing")
	}

	if _, err = url.Parse(c.Endpoint); err != nil {
		return fmt.Errorf("stackit_sd: invalid endpoint %q: %w", c.Endpoint, err)
	}

	return c.HTTPClientConfig.Validate()
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// Discovery periodically performs STACKIT API requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*refresh.Discovery, error) {
	m, ok := opts.Metrics.(*stackitMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	r, err := newRefresher(conf, opts.Logger)
	if err != nil {
		return nil, err
	}

	return refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "stackit",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            r.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	), nil
}

func newRefresher(conf *SDConfig, l *slog.Logger) (refresher, error) {
	return newServerDiscovery(conf, l)
}
