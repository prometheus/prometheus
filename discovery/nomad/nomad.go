// Copyright 2022 The Prometheus Authors
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

package nomad

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	nomad "github.com/hashicorp/nomad/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	// nomadLabel is the name for the label containing a target.
	nomadLabel = model.MetaLabelPrefix + "nomad_"
	// serviceLabel is the name of the label containing the service name.
	nomadAddress        = nomadLabel + "address"
	nomadService        = nomadLabel + "service"
	nomadNamespace      = nomadLabel + "namespace"
	nomadNodeID         = nomadLabel + "node_id"
	nomadDatacenter     = nomadLabel + "dc"
	nomadServiceAddress = nomadService + "_address"
	nomadServicePort    = nomadService + "_port"
	nomadServiceID      = nomadService + "_id"
	nomadTags           = nomadLabel + "tags"
)

// DefaultSDConfig is the default nomad SD configuration.
var DefaultSDConfig = SDConfig{
	AllowStale:       true,
	HTTPClientConfig: config.DefaultHTTPClientConfig,
	Namespace:        "default",
	RefreshInterval:  model.Duration(60 * time.Second),
	Region:           "global",
	Server:           "http://localhost:4646",
	TagSeparator:     ",",
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for nomad based service discovery.
type SDConfig struct {
	AllowStale       bool                    `yaml:"allow_stale"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	Namespace        string                  `yaml:"namespace"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval"`
	Region           string                  `yaml:"region"`
	Server           string                  `yaml:"server"`
	TagSeparator     string                  `yaml:"tag_separator,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "nomad" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Registerer)
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
	if strings.TrimSpace(c.Server) == "" {
		return errors.New("nomad SD configuration requires a server address")
	}
	return c.HTTPClientConfig.Validate()
}

// Discovery periodically performs nomad requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	allowStale      bool
	client          *nomad.Client
	namespace       string
	refreshInterval time.Duration
	region          string
	server          string
	tagSeparator    string
	failuresCount   prometheus.Counter
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger, reg prometheus.Registerer) (*Discovery, error) {
	d := &Discovery{
		allowStale:      conf.AllowStale,
		namespace:       conf.Namespace,
		refreshInterval: time.Duration(conf.RefreshInterval),
		region:          conf.Region,
		server:          conf.Server,
		tagSeparator:    conf.TagSeparator,
		failuresCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_sd_nomad_failures_total",
				Help: "Number of nomad service discovery refresh failures.",
			}),
	}

	HTTPClient, err := config.NewClientFromConfig(conf.HTTPClientConfig, "nomad_sd")
	if err != nil {
		return nil, err
	}

	config := nomad.Config{
		Address:    conf.Server,
		HttpClient: HTTPClient,
		Namespace:  conf.Namespace,
		Region:     conf.Region,
	}

	client, err := nomad.NewClient(&config)
	if err != nil {
		return nil, err
	}
	d.client = client

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:   logger,
			Mech:     "nomad",
			Interval: time.Duration(conf.RefreshInterval),
			RefreshF: d.refresh,
			Registry: reg,
			Metrics:  []prometheus.Collector{d.failuresCount},
		},
	)
	return d, nil
}

func (d *Discovery) refresh(context.Context) ([]*targetgroup.Group, error) {
	opts := &nomad.QueryOptions{
		AllowStale: d.allowStale,
	}
	stubs, _, err := d.client.Services().List(opts)
	if err != nil {
		d.failuresCount.Inc()
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: "Nomad",
	}

	for _, stub := range stubs {
		for _, service := range stub.Services {
			instances, _, err := d.client.Services().Get(service.ServiceName, opts)
			if err != nil {
				d.failuresCount.Inc()
				return nil, fmt.Errorf("failed to fetch services: %w", err)
			}

			for _, instance := range instances {
				labels := model.LabelSet{
					nomadAddress:        model.LabelValue(instance.Address),
					nomadDatacenter:     model.LabelValue(instance.Datacenter),
					nomadNodeID:         model.LabelValue(instance.NodeID),
					nomadNamespace:      model.LabelValue(instance.Namespace),
					nomadServiceAddress: model.LabelValue(instance.Address),
					nomadServiceID:      model.LabelValue(instance.ID),
					nomadServicePort:    model.LabelValue(strconv.Itoa(instance.Port)),
					nomadService:        model.LabelValue(instance.ServiceName),
				}
				addr := net.JoinHostPort(instance.Address, strconv.FormatInt(int64(instance.Port), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)

				if len(instance.Tags) > 0 {
					tags := d.tagSeparator + strings.Join(instance.Tags, d.tagSeparator) + d.tagSeparator
					labels[nomadTags] = model.LabelValue(tags)
				}

				tg.Targets = append(tg.Targets, labels)
			}
		}
	}
	return []*targetgroup.Group{tg}, nil
}
