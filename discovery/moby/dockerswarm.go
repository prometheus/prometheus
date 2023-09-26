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

package moby

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	swarmLabel = model.MetaLabelPrefix + "dockerswarm_"
)

var userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

// DefaultDockerSwarmSDConfig is the default Docker Swarm SD configuration.
var DefaultDockerSwarmSDConfig = DockerSwarmSDConfig{
	RefreshInterval:  model.Duration(60 * time.Second),
	Port:             80,
	Filters:          []Filter{},
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&DockerSwarmSDConfig{})
}

// DockerSwarmSDConfig is the configuration for Docker Swarm based service discovery.
type DockerSwarmSDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	Host    string   `yaml:"host"`
	Role    string   `yaml:"role"`
	Port    int      `yaml:"port"`
	Filters []Filter `yaml:"filters"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
}

// Filter represent a filter that can be passed to Docker Swarm to reduce the
// amount of data received.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// Name returns the name of the Config.
func (*DockerSwarmSDConfig) Name() string { return "dockerswarm" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *DockerSwarmSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger)
}

// SetDirectory joins any relative file paths with dir.
func (c *DockerSwarmSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *DockerSwarmSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultDockerSwarmSDConfig
	type plain DockerSwarmSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Host == "" {
		return fmt.Errorf("host missing")
	}
	if _, err = url.Parse(c.Host); err != nil {
		return err
	}
	switch c.Role {
	case "services", "nodes", "tasks":
	case "":
		return fmt.Errorf("role missing (one of: tasks, services, nodes)")
	default:
		return fmt.Errorf("invalid role %s, expected tasks, services, or nodes", c.Role)
	}
	return c.HTTPClientConfig.Validate()
}

// Discovery periodically performs Docker Swarm requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client  *client.Client
	role    string
	port    int
	filters filters.Args
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *DockerSwarmSDConfig, logger log.Logger) (*Discovery, error) {
	var err error

	d := &Discovery{
		port: conf.Port,
		role: conf.Role,
	}

	hostURL, err := url.Parse(conf.Host)
	if err != nil {
		return nil, err
	}

	opts := []client.Opt{
		client.WithHost(conf.Host),
		client.WithAPIVersionNegotiation(),
	}

	d.filters = filters.NewArgs()
	for _, f := range conf.Filters {
		for _, v := range f.Values {
			d.filters.Add(f.Name, v)
		}
	}

	// There are other protocols than HTTP supported by the Docker daemon, like
	// unix, which are not supported by the HTTP client. Passing HTTP client
	// options to the Docker client makes those non-HTTP requests fail.
	if hostURL.Scheme == "http" || hostURL.Scheme == "https" {
		rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "dockerswarm_sd")
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			client.WithHTTPClient(&http.Client{
				Transport: rt,
				Timeout:   time.Duration(conf.RefreshInterval),
			}),
			client.WithScheme(hostURL.Scheme),
			client.WithHTTPHeaders(map[string]string{
				"User-Agent": userAgent,
			}),
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
