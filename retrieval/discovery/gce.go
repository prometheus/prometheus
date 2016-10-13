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

package discovery

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

	// Constants for instrumentation.
	namespace = "prometheus"
)

var (
	gceSDScrapesCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gce_sd_scrapes_total",
			Help:      "The number of GCE-SD scrapes.",
		})
	gceSDScrapeFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gce_sd_scrape_failures_total",
			Help:      "The number of GCE-SD scrape failures.",
		})
	gceSDScrapeDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "gce_sd_scrape_duration",
			Help:      "The duration of a GCE-SD scrape in seconds.",
		})
)

func init() {
	prometheus.MustRegister(gceSDScrapesCount)
	prometheus.MustRegister(gceSDScrapeFailuresCount)
	prometheus.MustRegister(gceSDScrapeDuration)
}

// GCEDiscovery periodically performs GCE-SD requests. It implements
// the TargetProvider interface.
type GCEDiscovery struct {
	project      string
	zones        []string
	filter       string
	client       *http.Client
	svc          *compute.Service
	isvc         *compute.InstancesService
	zsvc         *compute.ZonesService
	interval     time.Duration
	port         int
	tagSeparator string
}

// NewGCEDiscovery returns a new GCEDiscovery which periodically refreshes its targets.
func NewGCEDiscovery(conf *config.GCESDConfig) (*GCEDiscovery, error) {
	gd := &GCEDiscovery{
		project:      conf.Project,
		zones:        conf.Zones,
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
	gd.zsvc = compute.NewZonesService(gd.svc)
	return gd, nil
}

// Run implements the TargetProvider interface.
func (gd *GCEDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	// Get an initial set right away.
	tg, err := gd.refresh()
	if err != nil {
		log.Error(err)
	} else {
		ch <- []*config.TargetGroup{tg}
	}

	ticker := time.NewTicker(gd.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tg, err := gd.refresh()
			if err != nil {
				log.Error(err)
			} else {
				ch <- []*config.TargetGroup{tg}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (gd *GCEDiscovery) refresh() (tg *config.TargetGroup, err error) {
	t0 := time.Now()
	defer func() {
		gceSDScrapeDuration.Observe(time.Since(t0).Seconds())
		gceSDScrapesCount.Inc()
		if err != nil {
			gceSDScrapeFailuresCount.Inc()
		}
	}()

	zones := gd.zones
	// if the list of zones in the config is empty we can assume that we are
	// supposed to scrape all availabe zones, otherwise this SD config should
	// not be defined at all.
	if len(zones) < 1 {
		zl, err := gd.zsvc.List(gd.project).Do()
		if err != nil {
			return tg, fmt.Errorf("error retrieving scrape target zones from gce: %s", err)
		}
		for _, zone := range zl.Items {
			zones = append(zones, zone.Name)
		}
	}

	tg = &config.TargetGroup{
		Source: fmt.Sprintf("GCE_%s_%s", gd.project, strings.Join(zones, "_")),
	}

	for _, zone := range zones {
		ilc := gd.isvc.List(gd.project, zone)
		if len(gd.filter) > 0 {
			ilc = ilc.Filter(gd.filter)
		}
		err = ilc.Pages(nil, func(l *compute.InstanceList) error {
			for _, inst := range l.Items {
				if len(inst.NetworkInterfaces) == 0 {
					continue
				}
				labels := model.LabelSet{
					gceLabelProject:        model.LabelValue(gd.project),
					gceLabelZone:           model.LabelValue(inst.Zone),
					gceLabelInstanceName:   model.LabelValue(inst.Name),
					gceLabelInstanceStatus: model.LabelValue(inst.Status),
				}
				priIface := inst.NetworkInterfaces[0]
				labels[gceLabelNetwork] = model.LabelValue(priIface.Network)
				labels[gceLabelSubnetwork] = model.LabelValue(priIface.Subnetwork)
				labels[gceLabelPrivateIP] = model.LabelValue(priIface.NetworkIP)
				addr := fmt.Sprintf("%s:%d", priIface.NetworkIP, gd.port)
				labels[model.AddressLabel] = model.LabelValue(addr)

				// tags in GCE are mostly used for networking rules (unlike e.g. AWS EC2 tags)
				if inst.Tags != nil && len(inst.Tags.Items) > 0 {
					// We surround the separated list with the separator as well. This way regular expressions
					// in relabeling rules don't have to consider tag positions.
					tags := gd.tagSeparator + strings.Join(inst.Tags.Items, gd.tagSeparator) + gd.tagSeparator
					labels[gceLabelTags] = model.LabelValue(tags)
				}

				// GCE metadata are free-form key-value pairs similar to AWS EC2 tags
				if inst.Metadata != nil && len(inst.Metadata.Items) > 0 {
					for _, i := range inst.Metadata.Items {
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
			return tg, fmt.Errorf("error retrieving scrape targets from gce: %s", err)
		}
	}
	return tg, nil
}
