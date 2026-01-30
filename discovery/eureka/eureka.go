// Copyright The Prometheus Authors
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
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
	metaLabelPrefix            = model.MetaLabelPrefix + "eureka_"
	metaAppInstanceLabelPrefix = metaLabelPrefix + "app_instance_"

	appNameLabel                            = metaLabelPrefix + "app_name"
	appInstanceHostNameLabel                = metaAppInstanceLabelPrefix + "hostname"
	appInstanceHomePageURLLabel             = metaAppInstanceLabelPrefix + "homepage_url"
	appInstanceStatusPageURLLabel           = metaAppInstanceLabelPrefix + "statuspage_url"
	appInstanceHealthCheckURLLabel          = metaAppInstanceLabelPrefix + "healthcheck_url"
	appInstanceIPAddrLabel                  = metaAppInstanceLabelPrefix + "ip_addr"
	appInstanceVipAddressLabel              = metaAppInstanceLabelPrefix + "vip_address"
	appInstanceSecureVipAddressLabel        = metaAppInstanceLabelPrefix + "secure_vip_address"
	appInstanceStatusLabel                  = metaAppInstanceLabelPrefix + "status"
	appInstancePortLabel                    = metaAppInstanceLabelPrefix + "port"
	appInstancePortEnabledLabel             = metaAppInstanceLabelPrefix + "port_enabled"
	appInstanceSecurePortLabel              = metaAppInstanceLabelPrefix + "secure_port"
	appInstanceSecurePortEnabledLabel       = metaAppInstanceLabelPrefix + "secure_port_enabled"
	appInstanceDataCenterInfoNameLabel      = metaAppInstanceLabelPrefix + "datacenterinfo_name"
	appInstanceDataCenterInfoMetadataPrefix = metaAppInstanceLabelPrefix + "datacenterinfo_metadata_"
	appInstanceCountryIDLabel               = metaAppInstanceLabelPrefix + "country_id"
	appInstanceIDLabel                      = metaAppInstanceLabelPrefix + "id"
	appInstanceMetadataPrefix               = metaAppInstanceLabelPrefix + "metadata_"
)

// DefaultSDConfig is the default Eureka SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval:  model.Duration(30 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
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

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &eurekaMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "eureka" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
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
	if len(c.Server) == 0 {
		return errors.New("eureka_sd: empty or null eureka server")
	}
	url, err := url.Parse(c.Server)
	if err != nil {
		return err
	}
	if len(url.Scheme) == 0 || len(url.Host) == 0 {
		return errors.New("eureka_sd: invalid eureka server URL")
	}
	return c.HTTPClientConfig.Validate()
}

// Discovery provides service discovery based on a Eureka instance.
type Discovery struct {
	*refresh.Discovery
	client *http.Client
	server string
}

// NewDiscovery creates a new Eureka discovery for the given role.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*Discovery, error) {
	m, ok := opts.Metrics.(*eurekaMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "eureka_sd")
	if err != nil {
		return nil, err
	}

	d := &Discovery{
		client: &http.Client{Transport: rt},
		server: conf.Server,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "eureka",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	apps, err := fetchApps(ctx, d.server, d.client)
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: "eureka",
	}

	for _, app := range apps.Applications {
		targets := targetsForApp(&app)
		tg.Targets = append(tg.Targets, targets...)
	}
	return []*targetgroup.Group{tg}, nil
}

func targetsForApp(app *Application) []model.LabelSet {
	targets := make([]model.LabelSet, 0, len(app.Instances))

	// Gather info about the app's 'instances'. Each instance is considered a task.
	for _, t := range app.Instances {
		var targetAddress string
		if t.Port != nil {
			targetAddress = net.JoinHostPort(t.HostName, strconv.Itoa(t.Port.Port))
		} else {
			targetAddress = net.JoinHostPort(t.HostName, "80")
		}

		target := model.LabelSet{
			model.AddressLabel:  lv(targetAddress),
			model.InstanceLabel: lv(t.InstanceID),

			appNameLabel:                     lv(app.Name),
			appInstanceHostNameLabel:         lv(t.HostName),
			appInstanceHomePageURLLabel:      lv(t.HomePageURL),
			appInstanceStatusPageURLLabel:    lv(t.StatusPageURL),
			appInstanceHealthCheckURLLabel:   lv(t.HealthCheckURL),
			appInstanceIPAddrLabel:           lv(t.IPAddr),
			appInstanceVipAddressLabel:       lv(t.VipAddress),
			appInstanceSecureVipAddressLabel: lv(t.SecureVipAddress),
			appInstanceStatusLabel:           lv(t.Status),
			appInstanceCountryIDLabel:        lv(strconv.Itoa(t.CountryID)),
			appInstanceIDLabel:               lv(t.InstanceID),
		}

		if t.Port != nil {
			target[appInstancePortLabel] = lv(strconv.Itoa(t.Port.Port))
			target[appInstancePortEnabledLabel] = lv(strconv.FormatBool(t.Port.Enabled))
		}

		if t.SecurePort != nil {
			target[appInstanceSecurePortLabel] = lv(strconv.Itoa(t.SecurePort.Port))
			target[appInstanceSecurePortEnabledLabel] = lv(strconv.FormatBool(t.SecurePort.Enabled))
		}

		if t.DataCenterInfo != nil {
			target[appInstanceDataCenterInfoNameLabel] = lv(t.DataCenterInfo.Name)

			if t.DataCenterInfo.Metadata != nil {
				for _, m := range t.DataCenterInfo.Metadata.Items {
					ln := strutil.SanitizeLabelName(m.XMLName.Local)
					target[model.LabelName(appInstanceDataCenterInfoMetadataPrefix+ln)] = lv(m.Content)
				}
			}
		}

		if t.Metadata != nil {
			for _, m := range t.Metadata.Items {
				ln := strutil.SanitizeLabelName(m.XMLName.Local)
				target[model.LabelName(appInstanceMetadataPrefix+ln)] = lv(m.Content)
			}
		}

		targets = append(targets, target)
	}
	return targets
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}
