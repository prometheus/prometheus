// Copyright 2015 The Prometheus Authors
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

package gce

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	gceLabel               = model.MetaLabelPrefix + "gce_"
	gceLabelProject        = gceLabel + "project"
	gceLabelZone           = gceLabel + "zone"
	gceLabelNetwork        = gceLabel + "network"
	gceLabelSubnetwork     = gceLabel + "subnetwork"
	gceLabelPublicIP       = gceLabel + "public_ip"
	gceLabelPrivateIP      = gceLabel + "private_ip"
	gceLabelInstanceID     = gceLabel + "instance_id"
	gceLabelInstanceName   = gceLabel + "instance_name"
	gceLabelInstanceStatus = gceLabel + "instance_status"
	gceLabelTags           = gceLabel + "tags"
	gceLabelMetadata       = gceLabel + "metadata_"
	gceLabelLabel          = gceLabel + "label_"
	gceLabelMachineType    = gceLabel + "machine_type"
)

// DefaultSDConfig is the default GCE SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	TagSeparator:    ",",
	RefreshInterval: model.Duration(60 * time.Second),
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for GCE based service discovery.
type SDConfig struct {
	// Project: The Google Cloud Project ID
	Project string `yaml:"project"`

	// Zone: The zone of the scrape targets.
	// If you need to configure multiple zones use multiple gce_sd_configs
	Zone string `yaml:"zone"`

	// Filter: Can be used optionally to filter the instance list by other criteria.
	// Syntax of this filter string is described here in the filter query parameter section:
	// https://cloud.google.com/compute/docs/reference/latest/instances/list
	Filter string `yaml:"filter,omitempty"`

	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	TagSeparator    string         `yaml:"tag_separator,omitempty"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &gceMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "gce" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(*c, opts)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Project == "" {
		return errors.New("GCE SD configuration requires a project")
	}
	if c.Zone == "" {
		return errors.New("GCE SD configuration requires a zone")
	}
	return nil
}

// Discovery periodically performs GCE-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	project      string
	zone         string
	filter       string
	client       *http.Client
	svc          *compute.Service
	isvc         *compute.InstancesService
	port         int
	tagSeparator string
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf SDConfig, opts discovery.DiscovererOptions) (*Discovery, error) {
	m, ok := opts.Metrics.(*gceMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	d := &Discovery{
		project:      conf.Project,
		zone:         conf.Zone,
		filter:       conf.Filter,
		port:         conf.Port,
		tagSeparator: conf.TagSeparator,
	}
	var err error
	d.client, err = google.DefaultClient(context.Background(), compute.ComputeReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("error setting up communication with GCE service: %w", err)
	}
	d.svc, err = compute.NewService(context.Background(), option.WithHTTPClient(d.client))
	if err != nil {
		return nil, fmt.Errorf("error setting up communication with GCE service: %w", err)
	}
	d.isvc = compute.NewInstancesService(d.svc)

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "gce",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: fmt.Sprintf("GCE_%s_%s", d.project, d.zone),
	}

	ilc := d.isvc.List(d.project, d.zone)
	if len(d.filter) > 0 {
		ilc = ilc.Filter(d.filter)
	}
	err := ilc.Pages(ctx, func(l *compute.InstanceList) error {
		for _, inst := range l.Items {
			if len(inst.NetworkInterfaces) == 0 {
				continue
			}
			labels := model.LabelSet{
				gceLabelProject:        model.LabelValue(d.project),
				gceLabelZone:           model.LabelValue(inst.Zone),
				gceLabelInstanceID:     model.LabelValue(strconv.FormatUint(inst.Id, 10)),
				gceLabelInstanceName:   model.LabelValue(inst.Name),
				gceLabelInstanceStatus: model.LabelValue(inst.Status),
				gceLabelMachineType:    model.LabelValue(inst.MachineType),
			}
			priIface := inst.NetworkInterfaces[0]
			labels[gceLabelNetwork] = model.LabelValue(priIface.Network)
			labels[gceLabelSubnetwork] = model.LabelValue(priIface.Subnetwork)
			labels[gceLabelPrivateIP] = model.LabelValue(priIface.NetworkIP)
			addr := fmt.Sprintf("%s:%d", priIface.NetworkIP, d.port)
			labels[model.AddressLabel] = model.LabelValue(addr)

			// Append named interface metadata for all interfaces
			for _, iface := range inst.NetworkInterfaces {
				gceLabelNetAddress := model.LabelName(fmt.Sprintf("%sinterface_ipv4_%s", gceLabel, strutil.SanitizeLabelName(iface.Name)))
				labels[gceLabelNetAddress] = model.LabelValue(iface.NetworkIP)
			}

			// Tags in GCE are usually only used for networking rules.
			if inst.Tags != nil && len(inst.Tags.Items) > 0 {
				// We surround the separated list with the separator as well. This way regular expressions
				// in relabeling rules don't have to consider tag positions.
				tags := d.tagSeparator + strings.Join(inst.Tags.Items, d.tagSeparator) + d.tagSeparator
				labels[gceLabelTags] = model.LabelValue(tags)
			}

			// GCE metadata are key-value pairs for user supplied attributes.
			if inst.Metadata != nil {
				for _, i := range inst.Metadata.Items {
					// Protect against occasional nil pointers.
					if i.Value == nil {
						continue
					}
					name := strutil.SanitizeLabelName(i.Key)
					labels[gceLabelMetadata+model.LabelName(name)] = model.LabelValue(*i.Value)
				}
			}

			// GCE labels are key-value pairs that group associated resources
			for key, value := range inst.Labels {
				name := strutil.SanitizeLabelName(key)
				labels[gceLabelLabel+model.LabelName(name)] = model.LabelValue(value)
			}

			if len(priIface.AccessConfigs) > 0 {
				ac := priIface.AccessConfigs[0]
				if ac.Type == "ONE_TO_ONE_NAT" {
					labels[gceLabelPublicIP] = model.LabelValue(ac.NatIP)
				}
			}
			tg.Targets = append(tg.Targets, labels)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving refresh targets from gce: %w", err)
	}
	return []*targetgroup.Group{tg}, nil
}
