// Copyright 2016 The Prometheus Authors
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

package ecs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/ecs/client"
	"github.com/prometheus/prometheus/util/strutil"
)

var (
	ecsSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_ecs_refresh_failures_total",
			Help: "The number of ECS-SD scrape failures.",
		})
	ecsSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_ecs_refresh_duration_seconds",
			Help: "The duration of a ECS-SD refresh in seconds.",
		})
	ecsSDRetrievedTargets = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "prometheus_sd_ecs_retrieved_targets",
			Help: "The number of retrieved ECS-SD targets.",
		})
)

const (
	ecsLabel                   = model.MetaLabelPrefix + "ecs_"
	ecsLabelcluster            = ecsLabel + "cluster"
	ecsLabelService            = ecsLabel + "service"
	ecsLabelContainer          = ecsLabel + "container"
	ecsLabelImage              = ecsLabel + "image"
	ecsLabelContainerPort      = ecsLabel + "container_port_number"
	ecsLabelContainerPortProto = ecsLabel + "container_port_protocol"
	ecsLabelContainerLabel     = ecsLabel + "container_label_"
	ecsLabelNodeTag            = ecsLabel + "node_tag_"
)

func init() {
	prometheus.MustRegister(ecsSDRefreshFailuresCount)
	prometheus.MustRegister(ecsSDRefreshDuration)
	prometheus.MustRegister(ecsSDRetrievedTargets)
}

// Discovery periodically performs ECS-SD requests. It implements
// the TargetProvider interface.
type Discovery struct {
	client client.Retriever // the client used to retrieve the service instances from AWS

	source   string // the source of the services
	interval time.Duration
	port     int
}

// NewDiscovery returns a new Discovery
func NewDiscovery(conf *config.ECSSDConfig) (*Discovery, error) {
	c, err := client.NewAWSRetriever(conf.AccessKey, conf.SecretKey, conf.Region, conf.Profile)
	if err != nil {
		return nil, err
	}

	return &Discovery{
		interval: time.Duration(conf.RefreshInterval),
		client:   c,
		source:   conf.Region,
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	log.Debugf("Start running ECS discovery")

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// First discovery
	tg, err := d.refresh()
	if err != nil {
		log.Error(err)
	} else {
		select {
		case ch <- []*config.TargetGroup{tg}:
		case <-ctx.Done():
			return
		}
	}

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
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *config.TargetGroup, err error) {
	log.Debugf("Start discovery of new targets")
	tStart := time.Now()
	defer func() {
		ecsSDRefreshDuration.Observe(time.Since(tStart).Seconds())
		if tg != nil {
			ecsSDRetrievedTargets.Set(float64(len(tg.Targets)))
		} else {
			ecsSDRetrievedTargets.Set(0)
		}
		if err != nil {
			ecsSDRefreshFailuresCount.Inc()
		}
	}()

	tg = &config.TargetGroup{
		Source: d.source,
	}

	// Get the service instances
	sis, err := d.client.Retrieve()
	if err != nil {
		return nil, err
	}

	for _, s := range sis {

		// Constant labels
		l := model.LabelSet{
			model.AddressLabel:         model.LabelValue(s.Addr),
			ecsLabelcluster:            model.LabelValue(s.Cluster),
			ecsLabelService:            model.LabelValue(s.Service),
			ecsLabelImage:              model.LabelValue(s.Image),
			ecsLabelContainer:          model.LabelValue(s.Container),
			ecsLabelContainerPort:      model.LabelValue(s.ContainerPort),
			ecsLabelContainerPortProto: model.LabelValue(s.ContainerPortProto),
		}
		// Dynamic labels
		for k, v := range s.Tags {
			name := strutil.SanitizeLabelName(k)
			l[ecsLabelNodeTag+model.LabelName(name)] = model.LabelValue(v)
		}
		for k, v := range s.Labels {
			name := strutil.SanitizeLabelName(k)
			l[ecsLabelContainerLabel+model.LabelName(name)] = model.LabelValue(v)
		}

		tg.Targets = append(tg.Targets, l)
	}

	log.Debugf("Finished discovery with %d targets", len(tg.Targets))
	return
}
