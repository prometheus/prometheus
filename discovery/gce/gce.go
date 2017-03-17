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
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/compute/v1"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/prometheus/prometheus/config"
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
	gceLabelInstanceName   = gceLabel + "instance_name"
	gceLabelInstanceStatus = gceLabel + "instance_status"
	gceLabelTags           = gceLabel + "tags"
	gceLabelMetadata       = gceLabel + "metadata_"
)

var (
	gceSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_gce_refresh_failures_total",
			Help: "The number of GCE-SD refresh failures.",
		})
	gceSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_gce_refresh_duration",
			Help: "The duration of a GCE-SD refresh in seconds.",
		})
)

func init() {
	prometheus.MustRegister(gceSDRefreshFailuresCount)
	prometheus.MustRegister(gceSDRefreshDuration)
}

// Discovery periodically performs GCE-SD requests. It implements
// the TargetProvider interface.
type Discovery struct {
	project      string
	zone         string
	filter       string
	client       *http.Client
	svc          *compute.Service
	isvc         *compute.InstancesService
	interval     time.Duration
	port         int
	tagSeparator string
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *config.GCESDConfig) (*Discovery, error) {
	gd := &Discovery{
		project:      conf.Project,
		zone:         conf.Zone,
		filter:       conf.Filter,
		interval:     time.Duration(conf.RefreshInterval),
		port:         conf.Port,
		tagSeparator: conf.TagSeparator,
	}
	var err error
	gd.client, err = google.DefaultClient(oauth2.NoContext, compute.ComputeReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("error setting up communication with GCE service: %s", err)
	}
	gd.svc, err = compute.New(gd.client)
	if err != nil {
		return nil, fmt.Errorf("error setting up communication with GCE service: %s", err)
	}
	gd.isvc = compute.NewInstancesService(gd.svc)
	return gd, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		log.Error(err)
	} else {
		select {
		case ch <- []*config.TargetGroup{tg}:
		case <-ctx.Done():
		}
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				log.Error(err)
				continue
			}
			select {
			case ch <- []*config.TargetGroup{tg}:
			case <-ctx.Done():
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *config.TargetGroup, err error) {
	t0 := time.Now()
	defer func() {
		gceSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			gceSDRefreshFailuresCount.Inc()
		}
	}()

	tg = &config.TargetGroup{
		Source: fmt.Sprintf("GCE_%s_%s", d.project, d.zone),
	}

	ilc := d.isvc.List(d.project, d.zone)
	if len(d.filter) > 0 {
		ilc = ilc.Filter(d.filter)
	}
	err = ilc.Pages(nil, func(l *compute.InstanceList) error {
		for _, inst := range l.Items {
			if len(inst.NetworkInterfaces) == 0 {
				continue
			}
			labels := model.LabelSet{
				gceLabelProject:        model.LabelValue(d.project),
				gceLabelZone:           model.LabelValue(inst.Zone),
				gceLabelInstanceName:   model.LabelValue(inst.Name),
				gceLabelInstanceStatus: model.LabelValue(inst.Status),
			}
			priIface := inst.NetworkInterfaces[0]
			labels[gceLabelNetwork] = model.LabelValue(priIface.Network)
			labels[gceLabelSubnetwork] = model.LabelValue(priIface.Subnetwork)
			labels[gceLabelPrivateIP] = model.LabelValue(priIface.NetworkIP)
			addr := fmt.Sprintf("%s:%d", priIface.NetworkIP, d.port)
			labels[model.AddressLabel] = model.LabelValue(addr)

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
		return tg, fmt.Errorf("error retrieving refresh targets from gce: %s", err)
	}
	return tg, nil
}
