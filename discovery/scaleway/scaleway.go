// Copyright 2021 The Prometheus Authors
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

package scaleway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/scaleway/scaleway-sdk-go/scw"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// metaLabelPrefix is the meta prefix used for all meta labels.
// in this discovery.
const (
	metaLabelPrefix = model.MetaLabelPrefix + "scaleway_"
	separator       = ","
)

// role is the role of the target within the Scaleway Ecosystem.
type role string

// The valid options for role.
const (
	// Scaleway Elements Baremetal
	// https://www.scaleway.com/en/bare-metal-servers/
	roleBaremetal role = "baremetal"

	// Scaleway Elements Instance
	// https://www.scaleway.com/en/virtual-instances/
	roleInstance role = "instance"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case roleInstance, roleBaremetal:
		return nil
	default:
		return fmt.Errorf("unknown role %q", *c)
	}
}

// DefaultSDConfig is the default Scaleway Service Discovery configuration.
var DefaultSDConfig = SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
	Zone:             scw.ZoneFrPar1.String(),
	APIURL:           "https://api.scaleway.com",
}

type SDConfig struct {
	// Project: The Scaleway Project ID used to filter discovery on.
	Project string `yaml:"project_id"`

	// APIURL: URL of the Scaleway API to use.
	APIURL string `yaml:"api_url,omitempty"`
	// Zone: The zone of the scrape targets.
	// If you need to configure multiple zones use multiple scaleway_sd_configs
	Zone string `yaml:"zone"`
	// AccessKey used to authenticate on Scaleway APIs.
	AccessKey string `yaml:"access_key"`
	// SecretKey used to authenticate on Scaleway APIs.
	SecretKey config.Secret `yaml:"secret_key"`
	// SecretKey used to authenticate on Scaleway APIs.
	SecretKeyFile string `yaml:"secret_key_file"`
	// NameFilter to filter on during the ListServers.
	NameFilter string `yaml:"name_filter,omitempty"`
	// TagsFilter to filter on during the ListServers.
	TagsFilter []string `yaml:"tags_filter,omitempty"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
	// Role can be either instance or baremetal
	Role role `yaml:"role"`
}

func (c SDConfig) Name() string {
	return "scaleway"
}

// secretKeyForConfig returns a secret key that looks like a UUID, even if we
// take the actual secret from a file.
func (c SDConfig) secretKeyForConfig() string {
	if c.SecretKeyFile != "" {
		return "00000000-0000-0000-0000-000000000000"
	}
	return string(c.SecretKey)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if c.Role == "" {
		return errors.New("role missing (one of: instance, baremetal)")
	}

	if c.Project == "" {
		return errors.New("project_id is mandatory")
	}

	if c.SecretKey == "" && c.SecretKeyFile == "" {
		return errors.New("one of secret_key & secret_key_file must be configured")
	}

	if c.SecretKey != "" && c.SecretKeyFile != "" {
		return errors.New("at most one of secret_key & secret_key_file must be configured")
	}

	if c.AccessKey == "" {
		return errors.New("access_key is mandatory")
	}

	profile, err := loadProfile(c)
	if err != nil {
		return err
	}
	_, err = scw.NewClient(
		scw.WithProfile(profile),
	)
	if err != nil {
		return err
	}

	return c.HTTPClientConfig.Validate()
}

func (c SDConfig) NewDiscoverer(options discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(&c, options.Logger, options.Registerer)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.SecretKeyFile = config.JoinDir(dir, c.SecretKeyFile)
	c.HTTPClientConfig.SetDirectory(dir)
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// Discovery periodically performs Scaleway requests. It implements
// the Discoverer interface.
type Discovery struct{}

func NewDiscovery(conf *SDConfig, logger log.Logger, reg prometheus.Registerer) (*refresh.Discovery, error) {
	r, err := newRefresher(conf)
	if err != nil {
		return nil, err
	}

	return refresh.NewDiscovery(
		refresh.Options{
			Logger:   logger,
			Mech:     "scaleway",
			Interval: time.Duration(conf.RefreshInterval),
			RefreshF: r.refresh,
			Registry: reg,
		},
	), nil
}

type refresher interface {
	refresh(context.Context) ([]*targetgroup.Group, error)
}

func newRefresher(conf *SDConfig) (refresher, error) {
	switch conf.Role {
	case roleBaremetal:
		return newBaremetalDiscovery(conf)
	case roleInstance:
		return newInstanceDiscovery(conf)
	}
	return nil, errors.New("unknown Scaleway discovery role")
}

func loadProfile(sdConfig *SDConfig) (*scw.Profile, error) {
	// Profile coming from Prometheus Configuration file
	prometheusConfigProfile := &scw.Profile{
		DefaultZone:      scw.StringPtr(sdConfig.Zone),
		APIURL:           scw.StringPtr(sdConfig.APIURL),
		SecretKey:        scw.StringPtr(sdConfig.secretKeyForConfig()),
		AccessKey:        scw.StringPtr(sdConfig.AccessKey),
		DefaultProjectID: scw.StringPtr(sdConfig.Project),
		SendTelemetry:    scw.BoolPtr(false),
	}

	return prometheusConfigProfile, nil
}

type authTokenFileRoundTripper struct {
	authTokenFile string
	rt            http.RoundTripper
}

// newAuthTokenFileRoundTripper adds the auth token read from the file to a request.
func newAuthTokenFileRoundTripper(tokenFile string, rt http.RoundTripper) (http.RoundTripper, error) {
	// fail-fast if we can't read the file.
	_, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read auth token file %s: %w", tokenFile, err)
	}
	return &authTokenFileRoundTripper{tokenFile, rt}, nil
}

func (rt *authTokenFileRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	b, err := os.ReadFile(rt.authTokenFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read auth token file %s: %w", rt.authTokenFile, err)
	}
	authToken := strings.TrimSpace(string(b))

	request.Header.Set("X-Auth-Token", authToken)
	return rt.rt.RoundTrip(request)
}
