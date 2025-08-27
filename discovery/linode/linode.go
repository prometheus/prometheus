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
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linode/linodego"
	"github.com/prometheus/client_golang/prometheus"
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
	linodeLabelGPUs               = linodeLabel + "gpus"
	linodeLabelHypervisor         = linodeLabel + "hypervisor"
	linodeLabelBackups            = linodeLabel + "backups"
	linodeLabelSpecsDiskBytes     = linodeLabel + "specs_disk_bytes"
	linodeLabelSpecsMemoryBytes   = linodeLabel + "specs_memory_bytes"
	linodeLabelSpecsVCPUs         = linodeLabel + "specs_vcpus"
	linodeLabelSpecsTransferBytes = linodeLabel + "specs_transfer_bytes"
	linodeLabelExtraIPs           = linodeLabel + "extra_ips"
	linodeLabelIPv6Ranges         = linodeLabel + "ipv6_ranges"

	// This is our events filter; when polling for changes, we care only about
	// events since our last refresh.
	// Docs: https://www.linode.com/docs/api/account/#events-list.
	filterTemplate = `{"created": {"+gte": "%s"}}`

	// Optional region filtering.
	regionFilterTemplate = `{"region": "%s"}`
)

// DefaultSDConfig is the default Linode SD configuration.
var DefaultSDConfig = SDConfig{
	TagSeparator:     ",",
	Port:             80,
	Region:           "",
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
	Region          string         `yaml:"region,omitempty"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return newDiscovererMetrics(reg, rmi)
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "linode" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
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
	region               string
	tagSeparator         string
	lastRefreshTimestamp time.Time
	pollCount            int
	lastResults          []*targetgroup.Group
	eventPollingEnabled  bool
	metrics              *linodeMetrics
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger *slog.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*linodeMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	d := &Discovery{
		port:                 conf.Port,
		region:               conf.Region,
		tagSeparator:         conf.TagSeparator,
		pollCount:            0,
		lastRefreshTimestamp: time.Now().UTC(),
		eventPollingEnabled:  true,
		metrics:              m,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "linode_sd")
	if err != nil {
		return nil, err
	}

	client := linodego.NewClient(
		&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		},
	)
	client.SetUserAgent(version.PrometheusUserAgent())
	d.client = &client

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "linode",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	needsRefresh := true
	ts := time.Now().UTC()

	if d.lastResults != nil && d.eventPollingEnabled {
		// Check to see if there have been any events. If so, refresh our data.
		eventsOpts := linodego.ListOptions{
			PageOptions: &linodego.PageOptions{Page: 1},
			PageSize:    25,
			Filter:      fmt.Sprintf(filterTemplate, d.lastRefreshTimestamp.Format("2006-01-02T15:04:05")),
		}
		events, err := d.client.ListEvents(ctx, &eventsOpts)
		if err != nil {
			var e *linodego.Error
			if !errors.As(err, &e) || e.Code != http.StatusUnauthorized {
				return nil, err
			}
			// If we get a 401, the token doesn't have `events:read_only` scope.
			// Disable event polling and fallback to doing a full refresh every interval.
			d.eventPollingEnabled = false
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
	// We need 3 of these because Linodego writes into the structure during pagination
	listInstancesOpts := linodego.ListOptions{
		PageSize: 500,
	}
	listIPAddressesOpts := linodego.ListOptions{
		PageSize: 500,
	}
	listIPv6RangesOpts := linodego.ListOptions{
		PageSize: 500,
	}

	// If region filter provided, use it to constrain results.
	if d.region != "" {
		listInstancesOpts.Filter = fmt.Sprintf(regionFilterTemplate, d.region)
		listIPAddressesOpts.Filter = fmt.Sprintf(regionFilterTemplate, d.region)
		listIPv6RangesOpts.Filter = fmt.Sprintf(regionFilterTemplate, d.region)
	}

	// Gather all linode instances.
	instances, err := d.client.ListInstances(ctx, &listInstancesOpts)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, err
	}

	// Gather detailed IP address info for all IPs on all linode instances.
	detailedIPs, err := d.client.ListIPAddresses(ctx, &listIPAddressesOpts)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, err
	}

	// Gather detailed IPv6 Range info for all linode instances.
	ipv6RangeList, err := d.client.ListIPv6Ranges(ctx, &listIPv6RangesOpts)
	if err != nil {
		d.metrics.failuresCount.Inc()
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
			extraIPs, ipv6Ranges                            []string
		)

		for _, ip := range instance.IPv4 {
			for _, detailedIP := range detailedIPs {
				if detailedIP.Address != ip.String() {
					continue
				}
				switch {
				case detailedIP.Public && publicIPv4 == "":
					publicIPv4 = detailedIP.Address

					if detailedIP.RDNS != "" && detailedIP.RDNS != "null" {
						publicIPv4RDNS = detailedIP.RDNS
					}
				case !detailedIP.Public && privateIPv4 == "":
					privateIPv4 = detailedIP.Address

					if detailedIP.RDNS != "" && detailedIP.RDNS != "null" {
						privateIPv4RDNS = detailedIP.RDNS
					}
				default:
					extraIPs = append(extraIPs, detailedIP.Address)
				}
			}
		}

		if instance.IPv6 != "" {
			slaac := strings.Split(instance.IPv6, "/")[0]
			for _, detailedIP := range detailedIPs {
				if detailedIP.Address != slaac {
					continue
				}
				publicIPv6 = detailedIP.Address

				if detailedIP.RDNS != "" && detailedIP.RDNS != "null" {
					publicIPv6RDNS = detailedIP.RDNS
				}
			}
			for _, ipv6Range := range ipv6RangeList {
				if ipv6Range.RouteTarget != slaac {
					continue
				}
				ipv6Ranges = append(ipv6Ranges, fmt.Sprintf("%s/%d", ipv6Range.Range, ipv6Range.Prefix))
			}
		}

		if instance.Backups.Enabled {
			backupsStatus = "enabled"
		} else {
			backupsStatus = "disabled"
		}

		labels := model.LabelSet{
			linodeLabelID:                 model.LabelValue(strconv.Itoa(instance.ID)),
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
			linodeLabelGPUs:               model.LabelValue(strconv.Itoa(instance.Specs.GPUs)),
			linodeLabelHypervisor:         model.LabelValue(instance.Hypervisor),
			linodeLabelBackups:            model.LabelValue(backupsStatus),
			linodeLabelSpecsDiskBytes:     model.LabelValue(strconv.FormatInt(int64(instance.Specs.Disk)<<20, 10)),
			linodeLabelSpecsMemoryBytes:   model.LabelValue(strconv.FormatInt(int64(instance.Specs.Memory)<<20, 10)),
			linodeLabelSpecsVCPUs:         model.LabelValue(strconv.Itoa(instance.Specs.VCPUs)),
			linodeLabelSpecsTransferBytes: model.LabelValue(strconv.FormatInt(int64(instance.Specs.Transfer)<<20, 10)),
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
			// IPv4,etc. We provide those extra IPs found here just like we do for instance
			// tags, we surround a separated list with the tagSeparator config.
			ips := d.tagSeparator + strings.Join(extraIPs, d.tagSeparator) + d.tagSeparator
			labels[linodeLabelExtraIPs] = model.LabelValue(ips)
		}

		if len(ipv6Ranges) > 0 {
			// This instance has more than one IPv6 Ranges routed to it we provide these
			// Ranges found here just like we do for instance tags, we surround a separated
			// list with the tagSeparator config.
			ips := d.tagSeparator + strings.Join(ipv6Ranges, d.tagSeparator) + d.tagSeparator
			labels[linodeLabelIPv6Ranges] = model.LabelValue(ips)
		}

		tg.Targets = append(tg.Targets, labels)
	}
	return []*targetgroup.Group{tg}, nil
}
