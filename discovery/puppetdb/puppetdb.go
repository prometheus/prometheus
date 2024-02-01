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

package puppetdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	pdbLabel            = model.MetaLabelPrefix + "puppetdb_"
	pdbLabelQuery       = pdbLabel + "query"
	pdbLabelCertname    = pdbLabel + "certname"
	pdbLabelResource    = pdbLabel + "resource"
	pdbLabelType        = pdbLabel + "type"
	pdbLabelTitle       = pdbLabel + "title"
	pdbLabelExported    = pdbLabel + "exported"
	pdbLabelTags        = pdbLabel + "tags"
	pdbLabelFile        = pdbLabel + "file"
	pdbLabelEnvironment = pdbLabel + "environment"
	pdbLabelParameter   = pdbLabel + "parameter_"
	separator           = ","
)

var (
	// DefaultSDConfig is the default PuppetDB SD configuration.
	DefaultSDConfig = SDConfig{
		RefreshInterval:  model.Duration(60 * time.Second),
		Port:             80,
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	matchContentType = regexp.MustCompile(`^(?i:application\/json(;\s*charset=("utf-8"|utf-8))?)$`)
	userAgent        = fmt.Sprintf("Prometheus/%s", version.Version)
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for PuppetDB based discovery.
type SDConfig struct {
	HTTPClientConfig  config.HTTPClientConfig `yaml:",inline"`
	RefreshInterval   model.Duration          `yaml:"refresh_interval,omitempty"`
	URL               string                  `yaml:"url"`
	Query             string                  `yaml:"query"`
	IncludeParameters bool                    `yaml:"include_parameters"`
	Port              int                     `yaml:"port"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &puppetdbMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "puppetdb" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
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
	if c.URL == "" {
		return fmt.Errorf("URL is missing")
	}
	parsedURL, err := url.Parse(c.URL)
	if err != nil {
		return err
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL scheme must be 'http' or 'https'")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("host is missing in URL")
	}
	if c.Query == "" {
		return fmt.Errorf("query missing")
	}
	return c.HTTPClientConfig.Validate()
}

// Discovery provides service discovery functionality based
// on PuppetDB resources.
type Discovery struct {
	*refresh.Discovery
	url               string
	query             string
	port              int
	includeParameters bool
	client            *http.Client
}

// NewDiscovery returns a new PuppetDB discovery for the given config.
func NewDiscovery(conf *SDConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*puppetdbMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	client, err := config.NewClientFromConfig(conf.HTTPClientConfig, "http")
	if err != nil {
		return nil, err
	}
	client.Timeout = time.Duration(conf.RefreshInterval)

	u, err := url.Parse(conf.URL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "pdb/query/v4")

	d := &Discovery{
		url:               u.String(),
		port:              conf.Port,
		query:             conf.Query,
		includeParameters: conf.IncludeParameters,
		client:            client,
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "puppetdb",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	body := struct {
		Query string `json:"query"`
	}{d.query}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", d.url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if ct := resp.Header.Get("Content-Type"); !matchContentType.MatchString(ct) {
		return nil, fmt.Errorf("unsupported content type %s", resp.Header.Get("Content-Type"))
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var resources []Resource

	if err := json.Unmarshal(b, &resources); err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		// Use a pseudo-URL as source.
		Source: d.url + "?query=" + d.query,
	}

	for _, resource := range resources {
		labels := model.LabelSet{
			pdbLabelQuery:       model.LabelValue(d.query),
			pdbLabelCertname:    model.LabelValue(resource.Certname),
			pdbLabelResource:    model.LabelValue(resource.Resource),
			pdbLabelType:        model.LabelValue(resource.Type),
			pdbLabelTitle:       model.LabelValue(resource.Title),
			pdbLabelExported:    model.LabelValue(fmt.Sprintf("%t", resource.Exported)),
			pdbLabelFile:        model.LabelValue(resource.File),
			pdbLabelEnvironment: model.LabelValue(resource.Environment),
		}

		addr := net.JoinHostPort(resource.Certname, strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		if len(resource.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(resource.Tags, separator) + separator
			labels[pdbLabelTags] = model.LabelValue(tags)
		}

		// Parameters are not included by default. This should only be enabled
		// on select resources as it might expose secrets on the Prometheus UI
		// for certain resources.
		if d.includeParameters {
			for k, v := range resource.Parameters.toLabels() {
				labels[pdbLabelParameter+k] = v
			}
		}

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}
