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

package digitalocean

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	doDBLabelID          = doLabel + "db_id"
	doDBLabelName        = doLabel + "db_name"
	doDBLabelEngine      = doLabel + "db_engine"
	doDBLabelStatus      = doLabel + "db_status"
	doDBLabelRegion      = doLabel + "db_region"
	doDBLabelHost        = doLabel + "db_host"
	doDBLabelPrivateHost = doLabel + "db_private_host"
	doDBLabelTagPrefix  = doLabel + "db_tag_"
)

// DefaultDBSDConfig is default DigitalOcean Database SD configuration.
var DefaultDBSDConfig = DBSDConfig{
	Port:             5432, // Default PostgreSQL port
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&DBSDConfig{})
}

// DBSDConfig is configuration for DigitalOcean Database based service discovery.
type DBSDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
}

// Name returns name of the Config.
func (*DBSDConfig) Name() string { return "digitalocean_db" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *DBSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDatabaseDiscovery(c, opts)
}

// SetDirectory joins any relative file paths with dir.
func (c *DBSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *DBSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultDBSDConfig
	type plain DBSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return c.HTTPClientConfig.Validate()
}

// DatabaseDiscovery periodically performs DigitalOcean Database requests. It implements
// the Discoverer interface.
type DatabaseDiscovery struct {
	*refresh.Discovery
	client *godo.Client
	port   int
}

// NewDatabaseDiscovery returns a new DatabaseDiscovery which periodically refreshes its targets.
func NewDatabaseDiscovery(conf *DBSDConfig, opts discovery.DiscovererOptions) (*DatabaseDiscovery, error) {
	m, ok := opts.Metrics.(*digitaloceanMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	d := &DatabaseDiscovery{
		port: conf.Port,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "digitalocean_db_sd")
	if err != nil {
		return nil, err
	}

	d.client, err = godo.New(
		&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		},
		godo.SetUserAgent(version.PrometheusUserAgent()),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up digital ocean agent: %w", err)
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "digitalocean_db",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *DatabaseDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: "DigitalOcean_Database",
	}

	databases, err := d.listDatabases(ctx)
	if err != nil {
		return nil, err
	}

	for _, db := range databases {
		if db.Connection == nil {
			continue
		}

		// Use private host if available, otherwise public host
		host := db.Connection.Host
		if db.Connection.PrivateHost != "" {
			host = db.Connection.PrivateHost
		}

		labels := model.LabelSet{
			doDBLabelID:          model.LabelValue(strconv.Itoa(db.ID)),
			doDBLabelName:        model.LabelValue(db.Name),
			doDBLabelEngine:      model.LabelValue(db.Engine),
			doDBLabelStatus:      model.LabelValue(db.Status),
			doDBLabelRegion:      model.LabelValue(db.Region),
			doDBLabelHost:        model.LabelValue(db.Connection.Host),
			doDBLabelPrivateHost: model.LabelValue(db.Connection.PrivateHost),
		}

		// Add tags as individual labels
		for _, tag := range db.Tags {
			labelKey := doDBLabelTagPrefix + tag.Key
			labels[model.LabelName(labelKey)] = model.LabelValue(tag.Value)
		}

		addr := net.JoinHostPort(host, strconv.FormatUint(uint64(d.port), 10))
		labels[model.AddressLabel] = model.LabelValue(addr)

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}

func (d *DatabaseDiscovery) listDatabases(ctx context.Context) ([]*godo.Database, error) {
	var (
		databases []*godo.Database
		opts      = &godo.ListOptions{}
	)

	for {
		paginatedDatabases, resp, err := d.client.Databases.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("error while listing databases page %d: %w", opts.Page, err)
		}
		databases = append(databases, paginatedDatabases...)

		// Handle DigitalOcean Database API pagination
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}

		opts.Page = page + 1
	}

	return databases, nil
}
