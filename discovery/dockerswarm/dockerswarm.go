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
	Port:            80,
}

// SDConfig is the configuration for Docker Swarm based service discovery.
type SDConfig struct {
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`

	Host string `yaml:"host"`
	url  *url.URL
	Role string `yaml:"role"`
	Port int    `yaml:"port"`

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
	if c.Host == "" {
		return fmt.Errorf("host missing")
	}
	url, err := url.Parse(c.Host)
	if err != nil {
		return err
	}
	c.url = url
	switch c.Role {
	case "services", "nodes", "tasks":
	case "":
		return fmt.Errorf("role missing (one of: tasks, services, nodes)")
	default:
		return fmt.Errorf("invalid role %s, expected tasks, services, or nodes", c.Role)
	}
	return nil
}

// Discovery periodically performs Docker Swarm requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client *client.Client
	role   string
	port   int
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	var err error
	d := &Discovery{
		port: conf.Port,
		role: conf.Role,
	}

	// This is used in tests. In normal situations, it is set when Unmarshaling.
	if conf.url == nil {
		conf.url, err = url.Parse(conf.Host)
		if err != nil {
			return nil, err
		}
	}

	opts := []client.Opt{
		client.WithHost(conf.Host),
		client.WithAPIVersionNegotiation(),
	}

	// There are other protocols than HTTP supported by the Docker daemon, like
	// unix, which are not supported by the HTTP client. Passing HTTP client
	// options to the Docker client makes those non-HTTP requests fail.
	if conf.url.Scheme == "http" || conf.url.Scheme == "https" {
		rt, err := config_util.NewRoundTripperFromConfig(conf.HTTPClientConfig, "dockerswarm_sd", false)
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			client.WithHTTPClient(&http.Client{
				Transport: rt,
				Timeout:   time.Duration(conf.RefreshInterval),
			}),
			client.WithScheme(conf.url.Scheme),
		)
	}

	d.client, err = client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("error setting up docker swarm client: %w", err)
	}

	d.Discovery = refresh.NewDiscovery(
		logger,
		"dockerswarm",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	switch d.role {
	case "services":
		return d.refreshServices(ctx)
	case "nodes":
		return d.refreshNodes(ctx)
	case "tasks":
		return d.refreshTasks(ctx)
	default:
		panic(fmt.Errorf("unexpected role %s", d.role))
	}
}
