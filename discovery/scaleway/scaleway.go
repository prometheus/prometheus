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
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// metaLabelPrefix is the meta prefix used for all meta labels.
// in this discovery.
const (
	metaLabelPrefix = model.MetaLabelPrefix + "scw_"
	separator       = ","
)

// DefaultSDConfig is the default Scaleway Service Discovery configuration.
var DefaultSDConfig = SDConfig{
	Zone:            "fr-par-1",
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
}

type SDConfig struct {
	// Project: The Scaleway Project ID
	Project string `yaml:"project"`

	// Zone: The zone of the scrape targets.
	// If you need to configure multiple zones use multiple scaleway_sd_configs
	Zone string `yaml:"zone"`
	// Access Key
	AccessKey string `yaml:"access_key"`
	// Secret Key
	SecretKey string `yaml:"secret_key"`

	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
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
	*refresh.Discovery
	client    *scw.Client
	port      int
	zone      string
	project   string
	accessKey string
	secretKey string
}

func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	d := &Discovery{
		port:      conf.Port,
		zone:      conf.Zone,
		project:   conf.Project,
		accessKey: conf.AccessKey,
		secretKey: conf.SecretKey,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "scaleway_sd", false, false)
	if err != nil {
		return nil, err
	}

	profile, err := d.loadProfile()
	if err != nil {
		return nil, err
	}
	d.client, err = scw.NewClient(
		scw.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		}),
		scw.WithUserAgent(fmt.Sprintf("Prometheus/%s", version.Version)),
		scw.WithProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up scaleway client: %w", err)
	}

	d.Discovery = refresh.NewDiscovery(
		logger,
		"scaleway",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "scaleway",
	}

	// instance fetching
	instanceServerLabelSet, err := d.listInstanceServers(ctx)
	if err != nil {
		return nil, err
	}
	tg.Targets = append(tg.Targets, instanceServerLabelSet...)

	// baremetal fetching
	baremetalServerLabelSet, err := d.listBaremetalServers(ctx)
	if err != nil {
		return nil, err
	}
	tg.Targets = append(tg.Targets, baremetalServerLabelSet...)

	return []*targetgroup.Group{tg}, nil
}

func (d *Discovery) loadProfile() (*scw.Profile, error) {
	scwConfig, err := scw.LoadConfig()
	// If the config file do not exist, don't return an error as we may find config in ENV or flags.
	if _, isNotFoundError := err.(*scw.ConfigFileNotFoundError); isNotFoundError {
		scwConfig = &scw.Config{}
	} else if err != nil {
		return nil, err
	}

	// By default we set default zone and region to fr-par
	defaultZoneProfile := &scw.Profile{
		DefaultRegion: scw.StringPtr(scw.RegionFrPar.String()),
		DefaultZone:   scw.StringPtr(scw.ZoneFrPar1.String()),
	}

	activeProfile, err := scwConfig.GetActiveProfile()
	if err != nil {
		return nil, err
	}
	envProfile := scw.LoadEnvProfile()

	providerProfile := &scw.Profile{}
	if d != nil {
		if d.accessKey != "" {
			providerProfile.AccessKey = scw.StringPtr(d.accessKey)
		}
		if d.secretKey != "" {
			providerProfile.SecretKey = scw.StringPtr(d.secretKey)
		}
		if d.project != "" {
			providerProfile.DefaultProjectID = scw.StringPtr(d.project)
		}
		if d.zone != "" {
			providerProfile.DefaultZone = scw.StringPtr(d.zone)
		}
	}

	profile := scw.MergeProfiles(defaultZoneProfile, activeProfile, providerProfile, envProfile)

	return profile, nil
}
