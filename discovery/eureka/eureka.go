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
	"context"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/util/strutil"
)

const (
	presentValue = model.LabelValue("true")

	// metaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "eureka_"

	appNameLabel                     = metaLabelPrefix + "app_name"
	appInstanceHostNameLabel         = metaLabelPrefix + "app_instance_hostname"
	appInstanceIpAddrLabel           = metaLabelPrefix + "app_instance_ipaddr"
	appInstanceStatusLabel           = metaLabelPrefix + "app_instance_status"
	appInstanceIdLabel               = metaLabelPrefix + "app_instance_id"
	appInstanceMetadataPrefix        = metaLabelPrefix + "app_instance_metadata_"
	appInstanceMetadataPresentPrefix = metaLabelPrefix + "app_instance_metadatapresent_"
)

// DefaultSDConfig is the default Eureka SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval: model.Duration(30 * time.Second),
}

// SDConfig is the configuration for applications running on Eureka.
type SDConfig struct {
	Servers          []string                     `yaml:"servers,omitempty"`
	RefreshInterval  model.Duration               `yaml:"refresh_interval,omitempty"`
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.Servers) == 0 {
		return errors.New("eureka_sd: must contain at least one Eureka server")
	}

	return c.HTTPClientConfig.Validate()
}

type applicationsClient func(ctx context.Context, servers []string, client *http.Client) (*Applications, error)

// Discovery provides service discovery based on a Eureka instance.
type Discovery struct {
	*refresh.Discovery
	client      *http.Client
	servers     []string
	lastRefresh map[string]*targetgroup.Group
	appsClient  applicationsClient
}

// New creates a new Eureka discovery for the given role.
func NewDiscovery(conf SDConfig, logger log.Logger) (*Discovery, error) {
	rt, err := config_util.NewRoundTripperFromConfig(conf.HTTPClientConfig, "marathon_sd", false)
	if err != nil {
		return nil, err
	}

	d := &Discovery{
		client:     &http.Client{Transport: rt},
		servers:    conf.Servers,
		appsClient: fetchApps,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"eureka",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	targetMap, err := d.fetchTargetGroups(ctx)
	if err != nil {
		return nil, err
	}

	all := make([]*targetgroup.Group, 0, len(targetMap))
	for _, tg := range targetMap {
		all = append(all, tg)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Remove services which did disappear.
	for source := range d.lastRefresh {
		_, ok := targetMap[source]
		if !ok {
			all = append(all, &targetgroup.Group{Source: source})
		}
	}

	d.lastRefresh = targetMap
	return all, nil
}

func (d *Discovery) fetchTargetGroups(ctx context.Context) (map[string]*targetgroup.Group, error) {
	apps, err := d.appsClient(ctx, d.servers, d.client)
	if err != nil {
		return nil, err
	}

	groups := appsToTargetGroups(apps)
	return groups, nil
}

// AppsToTargetGroups takes an array of Eureka Applications and converts them into target groups.
func appsToTargetGroups(apps *Applications) map[string]*targetgroup.Group {
	tgroups := map[string]*targetgroup.Group{}
	for _, a := range apps.Applications {
		group := createTargetGroup(&a)
		tgroups[group.Source] = group
	}
	return tgroups
}

func createTargetGroup(app *Application) *targetgroup.Group {
	var (
		targets = targetsForApp(app)
		appName = model.LabelValue(app.Name)
	)
	tg := &targetgroup.Group{
		Targets: targets,
		Labels: model.LabelSet{
			// FIXME
			//model.JobLabel: appName,
			appNameLabel: appName,
		},
		Source: app.Name,
	}

	return tg
}

func targetsForApp(app *Application) []model.LabelSet {
	targets := make([]model.LabelSet, 0, len(app.Instances))

	// Gather info about the app's 'instances'. Each instance is considered a task
	for _, t := range app.Instances {
		targetAddress := targetEndpoint(&t)
		target := model.LabelSet{
			model.AddressLabel:  model.LabelValue(targetAddress),
			model.InstanceLabel: model.LabelValue(t.InstanceID),

			appInstanceHostNameLabel: lv(t.HostName),
			appInstanceIpAddrLabel:   lv(t.IpAddr),
			appInstanceStatusLabel:   lv(t.Status),
			appInstanceIdLabel:       lv(t.InstanceID),
		}

		for k, v := range t.Metadata.Map {
			ln := strutil.SanitizeLabelName(k)
			target[model.LabelName(appInstanceMetadataPrefix+ln)] = lv(v)
			target[model.LabelName(appInstanceMetadataPresentPrefix+ln)] = presentValue
		}

		targets = append(targets, target)
	}
	return targets
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}

func targetEndpoint(instance *Instance) string {
	return net.JoinHostPort(instance.HostName, strconv.Itoa(instance.Port.Port))
}
