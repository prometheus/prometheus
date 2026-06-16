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
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/outscale/osc-sdk-go/v3/pkg/middleware/auth"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
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
	SecretKeyFile    string                  `yaml:"secret_key_file"`
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
	if c.AccessKey == "" {
		return errors.New("outscale SD configuration requires access_key")
	}
	if c.SecretKey == "" && c.SecretKeyFile == "" {
		return errors.New("one of secret_key & secret_key_file must be configured")
	}
	if c.SecretKey != "" && c.SecretKeyFile != "" {
		return errors.New("at most one of secret_key & secret_key_file must be configured")
	}
	if c.Endpoint == "" {
		c.Endpoint = "https://api." + c.Region + ".outscale.com/api/v1"
	}
	return c.HTTPClientConfig.Validate()
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.SecretKeyFile = config.JoinDir(dir, c.SecretKeyFile)
	c.HTTPClientConfig.SetDirectory(dir)
}

func (c *SDConfig) SecretKeyValue() (config.Secret, error) {
	if c.SecretKeyFile == "" {
		return c.SecretKey, nil
	}
	secret, err := os.ReadFile(c.SecretKeyFile)
	if err != nil {
		return "", fmt.Errorf("unable to read outscale secret key file %s: %w", c.SecretKeyFile, err)
	}
	return config.Secret(strings.TrimSpace(string(secret))), nil
}

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// Discovery periodically performs Outscale API requests.
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

type outscaleClient struct {
	endpoint     string
	roundTripper http.RoundTripper
	timeout      time.Duration
}

func (c *outscaleClient) ReadVms(ctx context.Context, body osc.ReadVmsRequest, accessKey, secretKey string) (*osc.ReadVmsResponse, error) {
	req, err := osc.NewReadVmsRequest(c.endpoint, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", version.PrometheusUserAgent())

	securityProvider, err := auth.NewSecurityProviderAWSv4(accessKey, secretKey, "", "", "")
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: securityProvider.Decorate(c.roundTripper),
		Timeout:   c.timeout,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	parsed, err := osc.ParseReadVmsResp(resp)
	if err != nil {
		return nil, err
	}
	return parsed.Expect()
}

// loadClient builds an Outscale API client from SDConfig using the official osc-sdk-go (OAPI).
func loadClient(conf *SDConfig) (*outscaleClient, error) {
	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "outscale_sd")
	if err != nil {
		return nil, fmt.Errorf("outscale SD client config: %w", err)
	}

	return &outscaleClient{
		endpoint:     strings.TrimRight(conf.Endpoint, "/") + "/",
		roundTripper: rt,
		timeout:      time.Duration(conf.RefreshInterval),
	}, nil
}
