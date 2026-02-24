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

package stackit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/stackitcloud/stackit-sdk-go/core/auth"
	stackitconfig "github.com/stackitcloud/stackit-sdk-go/core/config"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	stackitLabelPrefix           = model.MetaLabelPrefix + "stackit_"
	stackitLabelProject          = stackitLabelPrefix + "project"
	stackitLabelRole             = stackitLabelPrefix + "role"
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

// discoveryBase holds shared fields used by different service discovery implementations.
type discoveryBase struct {
	httpClient  *http.Client
	apiEndpoint string
}

// SDConfig is the configuration for STACKIT based service discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	Project               string         `yaml:"project"`
	RefreshInterval       model.Duration `yaml:"refresh_interval,omitempty"`
	Port                  int            `yaml:"port,omitempty"`
	Role                  Role           `yaml:"role"`
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

// Role is the Role of the target resource within the STACKIT Ecosystem.
type Role string

// The valid options for role.
const (
	RoleServer       Role = "server"
	RolePostgresFlex Role = "postgres_flex"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleServer:
		return nil
	case RolePostgresFlex:
		return nil
	default:
		return fmt.Errorf("unknown role %q", *c)
	}
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
	if conf.Role == RoleServer {
		return newServerDiscovery(conf, l)
	}

	if conf.Role == RolePostgresFlex {
		return newPostgresFlexDiscovery(conf, l)
	}

	return nil, errors.New("unknown STACKIT discovery role")
}

// setupDiscoveryBase initializes the common components used by service discovery types.
// It returns a shared discoveryBase struct with the configured HTTP client and API endpoint.
func setupDiscoveryBase(conf *SDConfig, discoveryDescription string, defaultEndpointFunc func(*SDConfig) string) (*discoveryBase, error) {
	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "stackit_sd")
	if err != nil {
		return nil, err
	}

	endpoint := conf.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpointFunc(conf)
	}

	servers := stackitconfig.ServerConfigurations{
		{
			URL:         endpoint,
			Description: discoveryDescription,
		},
	}

	httpClient := &http.Client{
		Timeout:   time.Duration(conf.RefreshInterval),
		Transport: rt,
	}

	stackitConfiguration := &stackitconfig.Configuration{
		UserAgent:  userAgent,
		HTTPClient: httpClient,
		Servers:    servers,
		NoAuth:     conf.ServiceAccountKey == "" && conf.ServiceAccountKeyPath == "",

		ServiceAccountKey:     conf.ServiceAccountKey,
		PrivateKey:            conf.PrivateKey,
		ServiceAccountKeyPath: conf.ServiceAccountKeyPath,
		PrivateKeyPath:        conf.PrivateKeyPath,
		CredentialsFilePath:   conf.CredentialsFilePath,
	}

	if conf.tokenURL != "" {
		stackitConfiguration.TokenCustomUrl = conf.tokenURL
	}

	authRoundTripper, err := auth.SetupAuth(stackitConfiguration)
	if err != nil {
		return nil, fmt.Errorf("setting up authentication: %w", err)
	}

	httpClient.Transport = authRoundTripper

	return &discoveryBase{
		httpClient:  httpClient,
		apiEndpoint: conf.Endpoint,
	}, nil
}
