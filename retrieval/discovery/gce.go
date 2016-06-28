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
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/compute/v1"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/prometheus/prometheus/config"
)

const (
	gceLabel             = model.MetaLabelPrefix + "gce_"
	gceLabelProject      = gceLabel + "project"
	gceLabelZone         = gceLabel + "zone"
	gceLabelNetwork      = gceLabel + "network"
	gceLabelSubnetwork   = gceLabel + "subnetwork"
	gceLabelPublicIP     = gceLabel + "public_ip"
	gceLabelPrivateIP    = gceLabel + "private_ip"
	gceLabelInstanceName = gceLabel + "instance_name"
)

// GCEDiscovery periodically performs GCE-SD requests. It implements
// the TargetProvider interface.
type GCEDiscovery struct {
	project  string
	zone     string
	filter   string
	client   *http.Client
	svc      *compute.Service
	isvc     *compute.InstancesService
	interval time.Duration
	port     int
}

// NewGCEDiscovery returns a new GCEDiscovery which periodically refreshes its targets.
func NewGCEDiscovery(conf *config.GCESDConfig) *GCEDiscovery {
	gd := &GCEDiscovery{
		project:  conf.Project,
		zone:     conf.Zone,
		filter:   conf.Filter,
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
	}
	var err error
	gd.client, err = google.DefaultClient(oauth2.NoContext, compute.ComputeReadonlyScope)
	if err != nil {
		log.Errorf("error setting up communication with GCE service: %s", err)
		return gd
	}
	gd.svc, err = compute.New(gd.client)
	if err != nil {
		log.Errorf("error setting up communication with GCE service: %s", err)
		return gd
	}
	gd.isvc = compute.NewInstancesService(gd.svc)
	return gd
}

// Run implements the TargetProvider interface.
func (gd *GCEDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	ticker := time.NewTicker(gd.interval)
	defer ticker.Stop()

	// Make sure gce setup was done correctly
	if gd.isvc == nil {
		log.Error(errors.New("GCE communication setup failed"))
		return
	}

	// Get an initial set right away.
	tg, err := gd.refresh()
	if err != nil {
		log.Error(err)
	} else {
		ch <- []*config.TargetGroup{tg}
	}

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

func (gd *GCEDiscovery) refresh() (*config.TargetGroup, error) {
	if gd.isvc == nil {
		return nil, errors.New("could not refresh gce targets because of previous gce communication setup error")
	}
	tg := &config.TargetGroup{
		Source: fmt.Sprintf("GCE_%s_%s", gd.project, gd.zone),
	}

	ilc := gd.isvc.List(gd.project, gd.zone)
	if len(gd.filter) > 0 {
		ilc = ilc.Filter(gd.filter)
	}
	err := ilc.Pages(nil, func(l *compute.InstanceList) error {
		for _, inst := range l.Items {
			if len(inst.NetworkInterfaces) == 0 {
				continue
			}
			labels := model.LabelSet{
				gceLabelProject:      model.LabelValue(gd.project),
				gceLabelZone:         model.LabelValue(inst.Zone),
				gceLabelInstanceName: model.LabelValue(inst.Name),
			}
			priIface := inst.NetworkInterfaces[0]
			labels[gceLabelNetwork] = model.LabelValue(priIface.Network)
			labels[gceLabelSubnetwork] = model.LabelValue(priIface.Subnetwork)
			labels[gceLabelPrivateIP] = model.LabelValue(priIface.NetworkIP)
			addr := fmt.Sprintf("%s:%d", priIface.NetworkIP, gd.port)
			labels[model.AddressLabel] = model.LabelValue(addr)

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
	return tg, nil
}
