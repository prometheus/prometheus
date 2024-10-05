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

package moby

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	dockerLabel                     = model.MetaLabelPrefix + "docker_"
	dockerLabelContainerPrefix      = dockerLabel + "container_"
	dockerLabelContainerID          = dockerLabelContainerPrefix + "id"
	dockerLabelContainerName        = dockerLabelContainerPrefix + "name"
	dockerLabelContainerNetworkMode = dockerLabelContainerPrefix + "network_mode"
	dockerLabelContainerLabelPrefix = dockerLabelContainerPrefix + "label_"
	dockerLabelNetworkPrefix        = dockerLabel + "network_"
	dockerLabelNetworkIP            = dockerLabelNetworkPrefix + "ip"
	dockerLabelPortPrefix           = dockerLabel + "port_"
	dockerLabelPortPrivate          = dockerLabelPortPrefix + "private"
	dockerLabelPortPublic           = dockerLabelPortPrefix + "public"
	dockerLabelPortPublicIP         = dockerLabelPortPrefix + "public_ip"
)

// DefaultDockerSDConfig is the default Docker SD configuration.
var DefaultDockerSDConfig = DockerSDConfig{
	RefreshInterval:    model.Duration(60 * time.Second),
	Port:               80,
	Filters:            []Filter{},
	HostNetworkingHost: "localhost",
	HTTPClientConfig:   config.DefaultHTTPClientConfig,
	MatchFirstNetwork:  true,
}

func init() {
	discovery.RegisterConfig(&DockerSDConfig{})
}

// DockerSDConfig is the configuration for Docker (non-swarm) based service discovery.
type DockerSDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	Host               string   `yaml:"host"`
	Port               int      `yaml:"port"`
	Filters            []Filter `yaml:"filters"`
	HostNetworkingHost string   `yaml:"host_networking_host"`

	RefreshInterval   model.Duration `yaml:"refresh_interval"`
	MatchFirstNetwork bool           `yaml:"match_first_network"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*DockerSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &dockerMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*DockerSDConfig) Name() string { return "docker" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *DockerSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDockerDiscovery(c, opts.Logger, opts.Metrics)
}

// SetDirectory joins any relative file paths with dir.
func (c *DockerSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *DockerSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultDockerSDConfig
	type plain DockerSDConfig
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
	return c.HTTPClientConfig.Validate()
}

type DockerDiscovery struct {
	*refresh.Discovery
	client             *client.Client
	port               int
	hostNetworkingHost string
	filters            filters.Args
	matchFirstNetwork  bool
}

// NewDockerDiscovery returns a new DockerDiscovery which periodically refreshes its targets.
func NewDockerDiscovery(conf *DockerSDConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (*DockerDiscovery, error) {
	m, ok := metrics.(*dockerMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	d := &DockerDiscovery{
		port:               conf.Port,
		hostNetworkingHost: conf.HostNetworkingHost,
		matchFirstNetwork:  conf.MatchFirstNetwork,
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
		rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "docker_sd")
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
		return nil, fmt.Errorf("error setting up docker client: %w", err)
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "docker",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *DockerDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "Docker",
	}

	containers, err := d.client.ContainerList(ctx, container.ListOptions{Filters: d.filters})
	if err != nil {
		return nil, fmt.Errorf("error while listing containers: %w", err)
	}

	networkLabels, err := getNetworksLabels(ctx, d.client, dockerLabel)
	if err != nil {
		return nil, fmt.Errorf("error while computing network labels: %w", err)
	}

	allContainers := make(map[string]types.Container)
	for _, c := range containers {
		allContainers[c.ID] = c
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}

		commonLabels := map[string]string{
			dockerLabelContainerID:          c.ID,
			dockerLabelContainerName:        c.Names[0],
			dockerLabelContainerNetworkMode: c.HostConfig.NetworkMode,
		}

		for k, v := range c.Labels {
			ln := strutil.SanitizeLabelName(k)
			commonLabels[dockerLabelContainerLabelPrefix+ln] = v
		}

		networks := c.NetworkSettings.Networks
		containerNetworkMode := container.NetworkMode(c.HostConfig.NetworkMode)
		if len(networks) == 0 {
			// Try to lookup shared networks
			for {
				if containerNetworkMode.IsContainer() {
					tmpContainer, exists := allContainers[containerNetworkMode.ConnectedContainer()]
					if !exists {
						break
					}
					networks = tmpContainer.NetworkSettings.Networks
					containerNetworkMode = container.NetworkMode(tmpContainer.HostConfig.NetworkMode)
					if len(networks) > 0 {
						break
					}
				} else {
					break
				}
			}
		}

		if d.matchFirstNetwork && len(networks) > 1 {
			// Sort networks by name and take first non-nil network.
			keys := make([]string, 0, len(networks))
			for k, n := range networks {
				if n != nil {
					keys = append(keys, k)
				}
			}
			if len(keys) > 0 {
				sort.Strings(keys)
				firstNetworkMode := keys[0]
				firstNetwork := networks[firstNetworkMode]
				networks = map[string]*network.EndpointSettings{firstNetworkMode: firstNetwork}
			}
		}

		for _, n := range networks {
			if n == nil {
				continue
			}

			var added bool

			for _, p := range c.Ports {
				if p.Type != "tcp" {
					continue
				}

				labels := model.LabelSet{
					dockerLabelNetworkIP:   model.LabelValue(n.IPAddress),
					dockerLabelPortPrivate: model.LabelValue(strconv.FormatUint(uint64(p.PrivatePort), 10)),
				}

				if p.PublicPort > 0 {
					labels[dockerLabelPortPublic] = model.LabelValue(strconv.FormatUint(uint64(p.PublicPort), 10))
					labels[dockerLabelPortPublicIP] = model.LabelValue(p.IP)
				}

				for k, v := range commonLabels {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				for k, v := range networkLabels[n.NetworkID] {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				addr := net.JoinHostPort(n.IPAddress, strconv.FormatUint(uint64(p.PrivatePort), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)
				tg.Targets = append(tg.Targets, labels)
				added = true
			}

			if !added {
				// Use fallback port when no exposed ports are available or if all are non-TCP
				labels := model.LabelSet{
					dockerLabelNetworkIP: model.LabelValue(n.IPAddress),
				}

				for k, v := range commonLabels {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				for k, v := range networkLabels[n.NetworkID] {
					labels[model.LabelName(k)] = model.LabelValue(v)
				}

				// Containers in host networking mode don't have ports,
				// so they only end up here, not in the previous loop.
				var addr string
				if c.HostConfig.NetworkMode != "host" {
					addr = net.JoinHostPort(n.IPAddress, strconv.FormatUint(uint64(d.port), 10))
				} else {
					addr = d.hostNetworkingHost
				}

				labels[model.AddressLabel] = model.LabelValue(addr)
				tg.Targets = append(tg.Targets, labels)
			}
		}
	}

	return []*targetgroup.Group{tg}, nil
}
