// Copyright 2025 The Prometheus Authors
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

package upcloud

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	upcloudServerLabel             = model.MetaLabelPrefix + "upcloud_server_"
	upcloudServerLabelUUID         = upcloudServerLabel + "uuid"
	upcloudServerLabelTitle        = upcloudServerLabel + "title"
	upcloudServerLabelHostname     = upcloudServerLabel + "hostname"
	upcloudServerLabelZone         = upcloudServerLabel + "zone"
	upcloudServerLabelPlan         = upcloudServerLabel + "plan"
	upcloudServerLabelState        = upcloudServerLabel + "state"
	upcloudServerLabelCoreNumber   = upcloudServerLabel + "core_number"
	upcloudServerLabelMemoryAmount = upcloudServerLabel + "memory_amount"
	upcloudServerLabelTags         = upcloudServerLabel + "tags"
	upcloudServerLabelTag          = upcloudServerLabel + "tag_"
	upcloudServerLabelLabel        = upcloudServerLabel + "label_"
	upcloudServerLabelPublicIPv4   = upcloudServerLabel + "public_ipv4"
	upcloudServerLabelPrivateIPv4  = upcloudServerLabel + "private_ipv4"
	upcloudServerLabelPublicIPv6   = upcloudServerLabel + "public_ipv6"
	upcloudServerLabelPrivateIPv6  = upcloudServerLabel + "private_ipv6"
	separator                      = ","
)

// DefaultSDConfig is the default UpCloud SD configuration.
var DefaultSDConfig = SDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &upcloudMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "upcloud" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.HTTPClientConfig.Validate()
}

// Client is an interface that allows mocking the UpCloud API client for testing.
type Client interface {
	GetServers(ctx context.Context) (*upcloud.Servers, error)
	GetServerDetails(ctx context.Context, uuid string) (*upcloud.ServerDetails, error)
}

// upcloudClientImpl wraps the actual UpCloud service client.
type upcloudClientImpl struct {
	service *service.Service
}

func (c *upcloudClientImpl) GetServers(ctx context.Context) (*upcloud.Servers, error) {
	return c.service.GetServers(ctx)
}

func (c *upcloudClientImpl) GetServerDetails(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	return c.service.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: uuid})
}

// Discovery periodically performs UpCloud requests. It implements the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client Client
	port   int
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger *slog.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*upcloudMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if conf.Port == 0 {
		conf.Port = DefaultSDConfig.Port
	}

	d := &Discovery{
		port: conf.Port,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "upcloud_sd")
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: rt,
		Timeout:   time.Duration(conf.RefreshInterval),
	}

	upcloudClient, err := client.NewFromEnv(
		client.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, err
	}

	d.client = &upcloudClientImpl{
		service: service.New(upcloudClient),
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "upcloud",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "UpCloud",
	}

	servers, err := d.client.GetServers(ctx)
	if err != nil {
		return nil, err
	}

	for _, server := range servers.Servers {
		serverDetails, err := d.client.GetServerDetails(ctx, server.UUID)
		if err != nil {
			return nil, err
		}

		labels := model.LabelSet{
			upcloudServerLabelUUID:         model.LabelValue(serverDetails.UUID),
			upcloudServerLabelTitle:        model.LabelValue(serverDetails.Title),
			upcloudServerLabelHostname:     model.LabelValue(serverDetails.Hostname),
			upcloudServerLabelZone:         model.LabelValue(serverDetails.Zone),
			upcloudServerLabelPlan:         model.LabelValue(serverDetails.Plan),
			upcloudServerLabelState:        model.LabelValue(serverDetails.State),
			upcloudServerLabelCoreNumber:   model.LabelValue(strconv.Itoa(serverDetails.CoreNumber)),
			upcloudServerLabelMemoryAmount: model.LabelValue(strconv.Itoa(serverDetails.MemoryAmount)),
		}

		// Find the primary public IP address for the target address
		var primaryIP, primaryIPv4, primaryIPv6 string
		var publicIPs, privateIPs, publicIPv6s []string

		for _, ipAddress := range serverDetails.IPAddresses {
			switch ipAddress.Access {
			case upcloud.IPAddressAccessPublic:
				switch ipAddress.Family {
				case upcloud.IPAddressFamilyIPv4:
					publicIPs = append(publicIPs, ipAddress.Address)
					if primaryIPv4 == "" {
						primaryIPv4 = ipAddress.Address
					}
				case upcloud.IPAddressFamilyIPv6:
					if primaryIPv6 == "" {
						primaryIPv6 = ipAddress.Address
					}
					publicIPv6s = append(publicIPv6s, ipAddress.Address)
				}
			case upcloud.IPAddressAccessPrivate:
				// Only IPv4 addresses can be private in UpCloud
				if ipAddress.Family == upcloud.IPAddressFamilyIPv4 {
					privateIPs = append(privateIPs, ipAddress.Address)
				}
			}
		}

		// Prefer public IPv4 over public IPv6
		if primaryIPv4 != "" {
			primaryIP = primaryIPv4
		}

		// If no public IPv4 found, use public IPv6
		if primaryIP == "" && primaryIPv6 != "" {
			primaryIP = primaryIPv6
		}

		// If no public IP found, try first available private IP
		if primaryIP == "" && len(privateIPs) > 0 {
			primaryIP = privateIPs[0]
		}

		// Skip servers without any usable IP address
		if primaryIP == "" {
			slog.Info("Skipping server without any usable IP address", "server", serverDetails.Title)
			continue
		}

		// Set the target address
		addr := net.JoinHostPort(primaryIP, strconv.Itoa(d.port))
		labels[model.AddressLabel] = model.LabelValue(addr)

		// Add IP address labels
		if len(publicIPs) > 0 {
			labels[upcloudServerLabelPublicIPv4] = model.LabelValue(strings.Join(publicIPs, ","))
		}
		if len(privateIPs) > 0 {
			labels[upcloudServerLabelPrivateIPv4] = model.LabelValue(strings.Join(privateIPs, ","))
		}
		if len(publicIPv6s) > 0 {
			labels[upcloudServerLabelPublicIPv6] = model.LabelValue(strings.Join(publicIPv6s, ","))
		}

		// Add individual "tag" labels
		for _, tag := range serverDetails.Tags {
			name := strutil.SanitizeLabelName(tag)
			labels[upcloudServerLabelTag+model.LabelName(name)] = model.LabelValue("true")
		}

		// Add individual "label" labels
		for _, label := range serverDetails.Labels {
			name := strutil.SanitizeLabelName(label.Key)
			labels[upcloudServerLabelLabel+model.LabelName(name)] = model.LabelValue(label.Value)
		}

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}
