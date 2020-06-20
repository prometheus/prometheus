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

package dockerswarm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/go-kit/kit/log"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	swarmLabel = model.MetaLabelPrefix + "dockerswarm_"
)

// DefaultSDConfig is the default Docker Swarm SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval: model.Duration(60 * time.Second),
	Host:            "http://127.0.0.1:2375/",
	Kind:            "services",
	Port:            80,
}

type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// SDConfig is the configuration for Docker Swarm based service discovery.
type SDConfig struct {
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`

	Host    string `yaml:"host"`
	url     *url.URL
	Filters []Filter `yaml:"filters"`
	Kind    string   `yaml:"kind"`
	Port    int      `yaml:"port"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	url, err := url.Parse(c.Host)
	if err != nil {
		return err
	}
	c.url = url
	if _, ok := (map[string]struct{}{"services": {}, "nodes": {}, "tasks": {}})[c.Kind]; !ok {
		return fmt.Errorf("invalid kind %s, expected services or nodes", c.Kind)
	}
	return nil
}

// Discovery periodically performs Docker Swarm requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client  *client.Client
	port    int
	filters []Filter `yaml:"filters"`
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	d := &Discovery{
		port:    conf.Port,
		filters: conf.Filters,
	}

	rt, err := config_util.NewRoundTripperFromConfig(conf.HTTPClientConfig, "dockerswarm_sd", false)
	if err != nil {
		return nil, err
	}

	// This is used in tests. In normal situation, it is set when Unmarshaling.
	if conf.url == nil {
		conf.url, err = url.Parse(conf.Host)
		if err != nil {
			return nil, err
		}
	}

	d.client, err = client.NewClientWithOpts(
		client.WithHost(conf.Host),
		client.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		}),
		client.WithScheme(conf.url.Scheme),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up docker swarm client: %w", err)
	}

	var r func(context.Context) ([]*targetgroup.Group, error)
	switch conf.Kind {
	case "services":
		r = d.refreshServices
	case "nodes":
		r = d.refreshNodes
	case "tasks":
		r = d.refreshTasks
	}

	d.Discovery = refresh.NewDiscovery(
		logger,
		"dockerswarm",
		time.Duration(conf.RefreshInterval),
		r,
	)
	return d, nil
}

func makeArgs(fs []Filter) filters.Args {
	args := filters.NewArgs()
	for _, f := range fs {
		for _, v := range f.Values {
			args.Add(f.Name, v)
		}
	}
	return args
}
