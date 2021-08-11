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

package xds

import (
	"fmt"
	"net/url"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/util/osutil"
	"github.com/prometheus/prometheus/util/strutil"
)

var (
	// DefaultKumaSDConfig is the default Kuma MADS SD configuration.
	DefaultKumaSDConfig = KumaSDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		RefreshInterval:  model.Duration(15 * time.Second),
		FetchTimeout:     model.Duration(2 * time.Minute),
	}

	kumaFetchFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_kuma_fetch_failures_total",
			Help:      "The number of Kuma MADS fetch call failures.",
		})
	kumaFetchSkipUpdateCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_kuma_fetch_skipped_updates_total",
			Help:      "The number of Kuma MADS fetch calls that result in no updates to the targets.",
		})
	kumaFetchDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "sd_kuma_fetch_duration_seconds",
			Help:       "The duration of a Kuma MADS fetch call.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
	)
)

const (
	// kumaMetaLabelPrefix is the meta prefix used for all kuma meta labels.
	kumaMetaLabelPrefix = model.MetaLabelPrefix + "kuma_"

	// kumaMeshLabel is the name of the label that holds the mesh name.
	kumaMeshLabel = kumaMetaLabelPrefix + "mesh"
	// kumaServiceLabel is the name of the label that holds the service name.
	kumaServiceLabel = kumaMetaLabelPrefix + "service"
	// kumaDataplaneLabel is the name of the label that holds the dataplane name.
	kumaDataplaneLabel = kumaMetaLabelPrefix + "dataplane"
	// kumaUserLabelPrefix is the name of the label that namespaces all user-defined labels.
	kumaUserLabelPrefix = kumaMetaLabelPrefix + "label_"
)

const (
	KumaMadsV1ResourceTypeURL = "type.googleapis.com/kuma.observability.v1.MonitoringAssignment"
	KumaMadsV1ResourceType    = "monitoringassignments"
)

type KumaSDConfig = SDConfig

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *KumaSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultKumaSDConfig
	type plainKumaConf KumaSDConfig
	err := unmarshal((*plainKumaConf)(c))
	if err != nil {
		return err
	}

	if len(c.Server) == 0 {
		return errors.Errorf("kuma SD server must not be empty: %s", c.Server)
	}
	parsedURL, err := url.Parse(c.Server)
	if err != nil {
		return err
	}

	if len(parsedURL.Scheme) == 0 || len(parsedURL.Host) == 0 {
		return errors.Errorf("kuma SD server must not be empty and have a scheme: %s", c.Server)
	}

	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return nil
}

func (c *KumaSDConfig) Name() string {
	return "kuma"
}

// SetDirectory joins any relative file paths with dir.
func (c *KumaSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

func (c *KumaSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	logger := opts.Logger
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return NewKumaHTTPDiscovery(c, logger)
}

func convertKumaV1MonitoringAssignment(assignment *MonitoringAssignment) []model.LabelSet {
	commonLabels := convertKumaUserLabels(assignment.Labels)

	commonLabels[kumaMeshLabel] = model.LabelValue(assignment.Mesh)
	commonLabels[kumaServiceLabel] = model.LabelValue(assignment.Service)

	var targets []model.LabelSet

	for _, madsTarget := range assignment.Targets {
		targetLabels := convertKumaUserLabels(madsTarget.Labels).Merge(commonLabels)

		targetLabels[kumaDataplaneLabel] = model.LabelValue(madsTarget.Name)
		targetLabels[model.AddressLabel] = model.LabelValue(madsTarget.Address)
		targetLabels[model.InstanceLabel] = model.LabelValue(madsTarget.Name)
		targetLabels[model.SchemeLabel] = model.LabelValue(madsTarget.Scheme)
		targetLabels[model.MetricsPathLabel] = model.LabelValue(madsTarget.MetricsPath)

		targets = append(targets, targetLabels)
	}

	return targets
}

func convertKumaUserLabels(labels map[string]string) model.LabelSet {
	labelSet := model.LabelSet{}
	for key, value := range labels {
		name := kumaUserLabelPrefix + strutil.SanitizeLabelName(key)
		labelSet[model.LabelName(name)] = model.LabelValue(value)
	}
	return labelSet
}

// kumaMadsV1ResourceParser is an xds.resourceParser.
func kumaMadsV1ResourceParser(resources []*anypb.Any, typeURL string) ([]model.LabelSet, error) {
	if typeURL != KumaMadsV1ResourceTypeURL {
		return nil, errors.Errorf("recieved invalid typeURL for Kuma MADS v1 Resource: %s", typeURL)
	}

	var targets []model.LabelSet

	for _, resource := range resources {
		assignment := &MonitoringAssignment{}

		if err := anypb.UnmarshalTo(resource, assignment, protoUnmarshalOptions); err != nil {
			return nil, err
		}

		targets = append(targets, convertKumaV1MonitoringAssignment(assignment)...)
	}

	return targets, nil
}

func NewKumaHTTPDiscovery(conf *KumaSDConfig, logger log.Logger) (discovery.Discoverer, error) {
	// Default to "prometheus" if hostname is unavailable.
	clientID, err := osutil.GetFQDN()
	if err != nil {
		level.Debug(logger).Log("msg", "error getting FQDN", "err", err)
		clientID = "prometheus"
	}

	clientConfig := &HTTPResourceClientConfig{
		HTTPClientConfig: conf.HTTPClientConfig,
		ExtraQueryParams: url.Values{
			"fetch-timeout": {conf.FetchTimeout.String()},
		},
		// Allow 15s of buffer over the timeout sent to the xDS server for connection overhead.
		Timeout:         time.Duration(conf.FetchTimeout) + (15 * time.Second),
		ResourceType:    KumaMadsV1ResourceType,
		ResourceTypeURL: KumaMadsV1ResourceTypeURL,
		Server:          conf.Server,
		ClientID:        clientID,
	}

	client, err := NewHTTPResourceClient(clientConfig, ProtocolV3)
	if err != nil {
		return nil, fmt.Errorf("kuma_sd: %w", err)
	}

	d := &fetchDiscovery{
		client:               client,
		logger:               logger,
		refreshInterval:      time.Duration(conf.RefreshInterval),
		source:               "kuma",
		parseResources:       kumaMadsV1ResourceParser,
		fetchFailuresCount:   kumaFetchFailuresCount,
		fetchSkipUpdateCount: kumaFetchSkipUpdateCount,
		fetchDuration:        kumaFetchDuration,
	}

	return d, nil
}
