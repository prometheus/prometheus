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

package linode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/linode/linodego"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	linodeLabel                   = model.MetaLabelPrefix + "linode_"
	linodeLabelID                 = linodeLabel + "instance_id"
	linodeLabelName               = linodeLabel + "instance_label"
	linodeLabelImage              = linodeLabel + "image"
	linodeLabelPrivateIPv4        = linodeLabel + "private_ipv4"
	linodeLabelPublicIPv4         = linodeLabel + "public_ipv4"
	linodeLabelPublicIPv6         = linodeLabel + "public_ipv6"
	linodeLabelPrivateIPv4RDNS    = linodeLabel + "private_ipv4_rdns"
	linodeLabelPublicIPv4RDNS     = linodeLabel + "public_ipv4_rdns"
	linodeLabelPublicIPv6RDNS     = linodeLabel + "public_ipv6_rdns"
	linodeLabelRegion             = linodeLabel + "region"
	linodeLabelType               = linodeLabel + "type"
	linodeLabelStatus             = linodeLabel + "status"
	linodeLabelTags               = linodeLabel + "tags"
	linodeLabelGroup              = linodeLabel + "group"
	linodeLabelHypervisor         = linodeLabel + "hypervisor"
	linodeLabelBackups            = linodeLabel + "backups"
	linodeLabelSpecsDiskBytes     = linodeLabel + "specs_disk_bytes"
	linodeLabelSpecsMemoryBytes   = linodeLabel + "specs_memory_bytes"
	linodeLabelSpecsVCPUs         = linodeLabel + "specs_vcpus"
	linodeLabelSpecsTransferBytes = linodeLabel + "specs_transfer_bytes"
	linodeLabelExtraIPs           = linodeLabel + "extra_ips"

	// This is our events filter; when polling for changes, we care only about
	// events since our last refresh.
	// Docs: https://www.linode.com/docs/api/account/#events-list
	filterTemplate = `{"created": {"+gte": "%s"}}`
)

// DefaultSDConfig is the default Linode SD configuration.
var DefaultSDConfig = SDConfig{
	TagSeparator:     ",",
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for Linode based service discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
	TagSeparator    string         `yaml:"tag_separator,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "linode" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger)
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
	return c.HTTPClientConfig.Validate()
}

// Discovery periodically performs Linode requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client               *linodego.Client
	port                 int
	tagSeparator         string
	lastRefreshTimestamp time.Time
	pollCount            int
	lastResults          []*targetgroup.Group
	eventPollingEnabled  bool
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	d := &Discovery{
		port:                 conf.Port,
		tagSeparator:         conf.TagSeparator,
		pollCount:            0,
		lastRefreshTimestamp: time.Now().UTC(),
		eventPollingEnabled:  true,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "linode_sd", config.WithHTTP2Disabled())
	if err != nil {
		return nil, err
	}

	client := linodego.NewClient(
		&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		},
	)
	client.SetUserAgent(fmt.Sprintf("Prometheus/%s", version.Version))
	d.client = &client

	d.Discovery = refresh.NewDiscovery(
		logger,
		"linode",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	needsRefresh := true
	ts := time.Now().UTC()

	if d.lastResults != nil && d.eventPollingEnabled {
		// Check to see if there have been any events. If so, refresh our data.
		opts := linodego.NewListOptions(1, fmt.Sprintf(filterTemplate, d.lastRefreshTimestamp.Format("2006-01-02T15:04:05")))
		events, err := d.client.ListEvents(ctx, opts)
		if err != nil {
			var e *linodego.Error
			if errors.As(err, &e) && e.Code == http.StatusUnauthorized {
				// If we get a 401, the token doesn't have `events:read_only` scope.
				// Disable event polling and fallback to doing a full refresh every interval.
				d.eventPollingEnabled = false
			} else {
				return nil, err
			}
		} else {
			// Event polling tells us changes the Linode API is aware of. Actions issued outside of the Linode API,
			// such as issuing a `shutdown` at the VM's console instead of using the API to power off an instance,
			// can potentially cause us to return stale data. Just in case, trigger a full refresh after ~10 polling
			// intervals of no events.
			d.pollCount++

			if len(events) == 0 && d.pollCount < 10 {
				needsRefresh = false
			}
		}
	}

	if needsRefresh {
		newData, err := d.refreshData(ctx)
		if err != nil {
			return nil, err
		}
		d.pollCount = 0
		d.lastResults = newData
	}

	d.lastRefreshTimestamp = ts

	return d.lastResults, nil
}

func (d *Discovery) refreshData(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "Linode",
	}

	// Gather all linode instances.
	instances, err := d.client.ListInstances(ctx, &linodego.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Gather detailed IP address info for all IPs on all linode instances.
	detailedIPs, err := d.client.ListIPAddresses(ctx, &linodego.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		if len(instance.IPv4) == 0 {
			continue
		}

		var (
			privateIPv4, publicIPv4, publicIPv6             string
			privateIPv4RDNS, publicIPv4RDNS, publicIPv6RDNS string
			backupsStatus                                   string
			extraIPs                                        []string
		)

		for _, ip := range instance.IPv4 {
			for _, detailedIP := range detailedIPs {
				if detailedIP.Address != ip.String() {
					continue
				}

				if detailedIP.Public && publicIPv4 == "" {
					publicIPv4 = detailedIP.Address

					if detailedIP.RDNS != "" && detailedIP.RDNS != "null" {
						publicIPv4RDNS = detailedIP.RDNS
					}
				} else if !detailedIP.Public && privateIPv4 == "" {
					privateIPv4 = detailedIP.Address

					if detailedIP.RDNS != "" && detailedIP.RDNS != "null" {
						privateIPv4RDNS = detailedIP.RDNS
					}
				} else {
					extraIPs = append(extraIPs, detailedIP.Address)
				}
			}
		}

		if instance.IPv6 != "" {
			for _, detailedIP := range detailedIPs {
				if detailedIP.Address != strings.Split(instance.IPv6, "/")[0] {
					continue
				}

				publicIPv6 = detailedIP.Address

				if detailedIP.RDNS != "" && detailedIP.RDNS != "null" {
					publicIPv6RDNS = detailedIP.RDNS
				}
			}
		}

		if instance.Backups.Enabled {
			backupsStatus = "enabled"
		} else {
			backupsStatus = "disabled"
		}

		labels := model.LabelSet{
			linodeLabelID:                 model.LabelValue(fmt.Sprintf("%d", instance.ID)),
			linodeLabelName:               model.LabelValue(instance.Label),
			linodeLabelImage:              model.LabelValue(instance.Image),
			linodeLabelPrivateIPv4:        model.LabelValue(privateIPv4),
			linodeLabelPublicIPv4:         model.LabelValue(publicIPv4),
			linodeLabelPublicIPv6:         model.LabelValue(publicIPv6),
			linodeLabelPrivateIPv4RDNS:    model.LabelValue(privateIPv4RDNS),
			linodeLabelPublicIPv4RDNS:     model.LabelValue(publicIPv4RDNS),
			linodeLabelPublicIPv6RDNS:     model.LabelValue(publicIPv6RDNS),
			linodeLabelRegion:             model.LabelValue(instance.Region),
			linodeLabelType:               model.LabelValue(instance.Type),
			linodeLabelStatus:             model.LabelValue(instance.Status),
			linodeLabelGroup:              model.LabelValue(instance.Group),
			linodeLabelHypervisor:         model.LabelValue(instance.Hypervisor),
			linodeLabelBackups:            model.LabelValue(backupsStatus),
			linodeLabelSpecsDiskBytes:     model.LabelValue(fmt.Sprintf("%d", instance.Specs.Disk<<20)),
			linodeLabelSpecsMemoryBytes:   model.LabelValue(fmt.Sprintf("%d", instance.Specs.Memory<<20)),
			linodeLabelSpecsVCPUs:         model.LabelValue(fmt.Sprintf("%d", instance.Specs.VCPUs)),
			linodeLabelSpecsTransferBytes: model.LabelValue(fmt.Sprintf("%d", instance.Specs.Transfer<<20)),
		}

		addr := net.JoinHostPort(publicIPv4, strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		if len(instance.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := d.tagSeparator + strings.Join(instance.Tags, d.tagSeparator) + d.tagSeparator
			labels[linodeLabelTags] = model.LabelValue(tags)
		}

		if len(extraIPs) > 0 {
			// This instance has more than one of at least one type of IP address (public, private,
			// IPv4, IPv6, etc. We provide those extra IPs found here just like we do for instance
			// tags, we surround a separated list with the tagSeparator config.
			ips := d.tagSeparator + strings.Join(extraIPs, d.tagSeparator) + d.tagSeparator
			labels[linodeLabelExtraIPs] = model.LabelValue(ips)
		}

		tg.Targets = append(tg.Targets, labels)
	}
	return []*targetgroup.Group{tg}, nil
}
