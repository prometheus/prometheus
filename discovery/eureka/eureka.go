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

package eureka

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/context"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"

	"github.com/kangwoo/go-eureka-client/eureka"
)

const (
	// metaLabelPrefix is the meta prefix used for all meta labels in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "eureka_"

	// appLabel is used for the name of the app in Marathon.
	appLabel      model.LabelName = metaLabelPrefix + "app"
	instanceLabel model.LabelName = metaLabelPrefix + "instance"

	// Constants for instrumentation.
	namespace = "prometheus"
)

var (
	refreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_eureka_refresh_failures_total",
			Help:      "The number of Eureka-SD refresh failures.",
		})
	refreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "sd_eureka_refresh_duration_seconds",
			Help:      "The duration of a Eureka-SD refresh in seconds.",
		})
)

func init() {
	prometheus.MustRegister(refreshFailuresCount)
	prometheus.MustRegister(refreshDuration)
}

// Discovery provides service discovery based on a Marathon instance.
type Discovery struct {
	client          *eureka.Client
	servers         []string
	refreshInterval time.Duration
	lastRefresh     map[string]*config.TargetGroup
	metricsPath     string
	logger          log.Logger
}

// NewDiscovery returns a new Eureka Service Discovery.
func NewDiscovery(conf *config.EurekaSDConfig, logger log.Logger) (*Discovery, error) {
	client := eureka.NewClient(conf.Servers)

	return &Discovery{
		client:          client,
		servers:         conf.Servers,
		refreshInterval: time.Duration(conf.RefreshInterval),
		metricsPath:     conf.MetricsPath,
		logger:          logger,
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d.refreshInterval):
			err := d.updateServices(ctx, ch)
			if err != nil {
				level.Error(d.logger).Log("msg", "Error while updating services", "err", err)
			}
		}
	}
}

func (d *Discovery) updateServices(ctx context.Context, ch chan<- []*config.TargetGroup) (err error) {
	t0 := time.Now()
	defer func() {
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	targetMap, err := d.fetchTargetGroups()
	if err != nil {
		return err
	}

	all := make([]*config.TargetGroup, 0, len(targetMap))
	for _, tg := range targetMap {
		all = append(all, tg)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- all:
	}

	// Remove services which did disappear.
	for source := range d.lastRefresh {
		_, ok := targetMap[source]
		if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- []*config.TargetGroup{{Source: source}}:
				level.Debug(d.logger).Log("msg", "Removing group", "source", source)
			}
		}
	}

	d.lastRefresh = targetMap
	return nil
}

func (d *Discovery) fetchTargetGroups() (map[string]*config.TargetGroup, error) {
	apps, err := d.client.GetApplications()
	if err != nil {
		return nil, err
	}

	groups := d.appsToTargetGroups(apps)
	return groups, nil
}

// AppsToTargetGroups takes an array of Eureka Application and converts them into target groups.
func (d *Discovery) appsToTargetGroups(apps *eureka.Applications) map[string]*config.TargetGroup {
	tgroups := map[string]*config.TargetGroup{}
	for _, app := range apps.Applications {
		group := d.createTargetGroup(&app)
		tgroups[group.Source] = group
	}
	return tgroups
}

func (d *Discovery) createTargetGroup(app *eureka.Application) *config.TargetGroup {
	var (
		targets = d.targetsForApp(app)
		appName = model.LabelValue(app.Name)
	)
	tg := &config.TargetGroup{
		Targets: targets,
		Labels: model.LabelSet{
			model.JobLabel: appName,
		},
		Source: app.Name,
	}

	return tg
}

func (d *Discovery) targetsForApp(app *eureka.Application) []model.LabelSet {
	targets := make([]model.LabelSet, 0, len(app.Instances))
	for _, t := range app.Instances {
		if t.Metadata.Map["prometheus"] != "true" {
			continue
		}

		targetAddress := targetForInstance(&t)
		target := model.LabelSet{
			model.AddressLabel:     model.LabelValue(targetAddress),
			model.MetricsPathLabel: model.LabelValue(d.metricsPath),
			model.InstanceLabel:    model.LabelValue(t.InstanceId),
		}
		for ln, lv := range t.Metadata.Map {
			ln = strutil.SanitizeLabelName(ln)
			target[model.LabelName(ln)] = model.LabelValue(lv)
		}
		targets = append(targets, target)
	}
	return targets
}

func targetForInstance(instance *eureka.InstanceInfo) string {
	return net.JoinHostPort(instance.HostName, fmt.Sprintf("%d", instance.Port.Port))
}
