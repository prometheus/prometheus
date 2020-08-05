// Copyright 2020 The Prometheus Authors
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
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	// metaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "eureka_"

	appNameLabel              = metaLabelPrefix + "app_name"
	appInstanceHostNameLabel  = metaLabelPrefix + "app_instance_hostname"
	appInstanceIPAddrLabel    = metaLabelPrefix + "app_instance_ipaddr"
	appInstanceStatusLabel    = metaLabelPrefix + "app_instance_status"
	appInstanceIDLabel        = metaLabelPrefix + "app_instance_id"
	appInstanceMetadataPrefix = metaLabelPrefix + "app_instance_metadata_"
)

// DefaultSDConfig is the default Eureka SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval: model.Duration(30 * time.Second),
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for applications running on Eureka.
type SDConfig struct {
	Server           string                  `yaml:"server,omitempty"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "eureka" }

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
	if len(c.Server) == 0 {
		return errors.New("eureka_sd: empty or null eureka server")
	}

	return c.HTTPClientConfig.Validate()
}

type applicationsClient func(ctx context.Context, server string, client *http.Client) (*Applications, error)

// Discovery provides service discovery based on a Eureka instance.
type Discovery struct {
	*refresh.Discovery
	client     *http.Client
	server     string
	appsClient applicationsClient
}

// New creates a new Eureka discovery for the given role.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "eureka_sd", false, false)
	if err != nil {
		return nil, err
	}

	d := &Discovery{
		client:     &http.Client{Transport: rt},
		server:     conf.Server,
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

	return all, nil
}

func (d *Discovery) fetchTargetGroups(ctx context.Context) (map[string]*targetgroup.Group, error) {
	apps, err := d.appsClient(ctx, d.server, d.client)
	if err != nil {
		return nil, err
	}

	groups := appsToTargetGroups(apps)
	return groups, nil
}

// appsToTargetGroups takes an array of Eureka Applications and converts them into target groups.
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
			appNameLabel: appName,
		},
		Source: "eureka",
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
			appInstanceIPAddrLabel:   lv(t.IPAddr),
			appInstanceStatusLabel:   lv(t.Status),
			appInstanceIDLabel:       lv(t.InstanceID),
		}

		if t.Metadata != nil {
			for k, v := range t.Metadata.Map {
				ln := strutil.SanitizeLabelName(k)
				target[model.LabelName(appInstanceMetadataPrefix+ln)] = lv(v)
			}
		}

		targets = append(targets, target)
	}
	return targets
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}

func targetEndpoint(instance *Instance) string {
	var port string
	if instance.Metadata != nil &&
		len(instance.Metadata.Map["management.port"]) > 0 {
		port = instance.Metadata.Map["management.port"]
	} else {
		port = strconv.Itoa(instance.Port.Port)
	}
	return net.JoinHostPort(instance.HostName, port)
}
