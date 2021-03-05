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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/scaleway/scaleway-sdk-go/scw"
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
		return errors.Errorf("unknown role %q", *c)
	}
}

// DefaultSDConfig is the default Scaleway Service Discovery configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
	Zone:            scw.ZoneFrPar1.String(),
	APIURL:          "http://api.scaleway.com",
}

type SDConfig struct {
	// Project: The Scaleway Project ID used to filter discovery on.
	Project string `yaml:"project"`

	// APIURL: URL of the Scaleway API to use (will default to https://api.scaleway.com).
	APIURL string `yaml:"api_url"`
	// Zone: The zone of the scrape targets.
	// If you need to configure multiple zones use multiple scaleway_sd_configs
	Zone string `yaml:"zone"`
	// AccessKey used to authenticate on Scaleway APIs.
	AccessKey string `yaml:"access_key"`
	// SecretKey used to authenticate on Scaleway APIs.
	SecretKey config.Secret `yaml:"secret_key"`
	// FilterName to filter on during the ListServers.
	FilterName string `yaml:"name"`
	// FilterTags to filter on during the ListServers.
	FilterTags []string `yaml:"tags"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
	// Role can be either instance or baremetal
	Role role `yaml:"role"`
}

func (c SDConfig) Name() string {
	return "scaleway"
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

	return c.HTTPClientConfig.Validate()
}

func (c SDConfig) NewDiscoverer(options discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(&c, options.Logger)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// Discovery periodically performs Scaleway requests. It implements
// the Discoverer interface.
type Discovery struct {
}

func NewDiscovery(conf *SDConfig, logger log.Logger) (*refresh.Discovery, error) {
	r, err := newRefresher(conf)
	if err != nil {
		return nil, err
	}

	return refresh.NewDiscovery(
		logger,
		"scaleway",
		time.Duration(conf.RefreshInterval),
		r.refresh,
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

func LoadProfile(sdConfig *SDConfig) (*scw.Profile, error) {
	// Profile coming from Prometheus Configuration file
	prometheusConfigProfile := &scw.Profile{
		DefaultZone: scw.StringPtr(sdConfig.Zone),
		APIURL:      scw.StringPtr(sdConfig.APIURL),
	}

	if sdConfig.AccessKey != "" {
		prometheusConfigProfile.AccessKey = scw.StringPtr(sdConfig.AccessKey)
	}
	if sdConfig.SecretKey != "" {
		prometheusConfigProfile.SecretKey = scw.StringPtr(string(sdConfig.SecretKey))
	}
	if sdConfig.Project != "" {
		prometheusConfigProfile.DefaultProjectID = scw.StringPtr(sdConfig.Project)
	}

	return prometheusConfigProfile, nil
}
