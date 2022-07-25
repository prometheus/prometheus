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

package ovhcloud

import (
	"context"
	"fmt"
	"time"

	"inet.af/netaddr"

	"github.com/fatih/structs"
	"github.com/go-kit/log"
	"github.com/grafana/regexp"
	"github.com/ovh/go-ovh/ovh"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// metaLabelPrefix is the meta prefix used for all meta labels.
// in this discovery.
const (
	metaLabelPrefix = model.MetaLabelPrefix + "ovhcloud_"
)

var (
	ovhCloudApplicationKeyTest    = "TDPKJdwZwAQPwKX2"
	ovhCloudApplicationSecretTest = config.Secret("9ufkBmLaTQ9nz5yMUlg79taH0GNnzDjk")
	ovhCloudConsumerKeyTest       = "5mBuy6SUQcRw2ZUxg0cG68BoDKpED4KY"
)

func addFieldsOnLabels(fields []*structs.Field, labels model.LabelSet, prefix string) {
	for _, f := range fields {
		labelName := f.Tag("label")
		if labelName == "-" {
			// Skip labels with -
			continue
		}
		if labelName == "" {
			labelName = f.Tag("json")
		}

		if labelName != "" {
			labels[model.LabelName(prefix+labelName)] = model.LabelValue(fmt.Sprintf("%+v", f.Value()))
		}
	}
}

type refresher interface {
	refresh(context.Context) ([]*targetgroup.Group, error)
}

var DefaultSDConfig = SDConfig{
	Endpoint:        "ovh-eu",
	RefreshInterval: model.Duration(60 * time.Second),
}

// SDConfig sd config
type SDConfig struct {
	Endpoint          string         `yaml:"endpoint"`
	ApplicationKey    string         `yaml:"application_key"`
	ApplicationSecret config.Secret  `yaml:"application_secret"`
	ConsumerKey       string         `yaml:"consumer_key"`
	RefreshInterval   model.Duration `yaml:"refresh_interval"`
	Service           string         `yaml:"service"`
}

// IPs struct to store IPV4 and IPV6
type IPs struct {
	IPV4 string `json:"ipv4" label:"ipv4"`
	IPV6 string `json:"ipv6" label:"ipv6"`
}

// Name get name
func (c SDConfig) Name() string {
	return "ovhcloud"
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if c.Endpoint == "" {
		return errors.New("endpoint can not be empty")
	}
	if c.ApplicationKey == "" {
		return errors.New("application key can not be empty")
	}
	if c.ApplicationSecret == "" {
		return errors.New("application secret can not be empty")
	}
	if c.ConsumerKey == "" {
		return errors.New("consumer key can not be empty")
	}
	switch c.Service {
	case "dedicated_server", "vps":
		return nil
	default:
		return errors.Errorf("unknown service: %v", c.Service)
	}
}

// CreateClient get client
func CreateClient(config *SDConfig) (*ovh.Client, error) {
	return ovh.NewClient(config.Endpoint, config.ApplicationKey, string(config.ApplicationSecret), config.ConsumerKey)
}

// NewDiscoverer new discoverer
func (c *SDConfig) NewDiscoverer(options discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, options.Logger)
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// ParseIPList Parse ip list to store on IPV4 and IPV6 on IPs type
func ParseIPList(ipList []string) (*IPs, error) {
	var IPs IPs
	reg := regexp.MustCompile(`^([0-9a-f\.:]+)(\/(\d+))?$`)
	for _, ip := range ipList {
		netmask := reg.ReplaceAllString(ip, "${3}")
		ip = reg.ReplaceAllString(ip, "${1}")

		netIP, err := netaddr.ParseIP(ip)
		if err == nil && !netIP.IsUnspecified() {
			if netIP.Is4() {
				if netmask != "" && netmask != "32" {
					continue
				}
				IPs.IPV4 = ip
			} else if netIP.Is6() {
				if netmask != "" && netmask != "128" {
					continue
				}
				IPs.IPV6 = ip
			}
		}
	}

	if IPs.IPV4 == "" && IPs.IPV6 == "" {
		return nil, errors.New("could not parse IP addresses from list")
	}
	return &IPs, nil
}

func newRefresher(conf *SDConfig, logger log.Logger) (refresher, error) {
	switch conf.Service {
	case "vps":
		return newVpsDiscovery(conf, logger), nil
	case "dedicated_server":
		return newDedicatedServerDiscovery(conf, logger), nil
	}
	return nil, fmt.Errorf("unknown OVHcloud discovery service '%s'", conf.Service)
}

// NewDiscovery ovhcloud creates a discovery with refresher
func NewDiscovery(conf *SDConfig, logger log.Logger) (*refresh.Discovery, error) {
	r, err := newRefresher(conf, logger)
	if err != nil {
		return nil, err
	}

	return refresh.NewDiscovery(
		logger,
		"ovhcloud",
		time.Duration(conf.RefreshInterval),
		r.refresh,
	), nil
}
