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

package puppetdb

import (
	"fmt"
	"time"

	"github.com/akira/go-puppetdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

const (
	// addressLabel is the name for the label containing a target's address.
	addressLabel = model.MetaLabelPrefix + "puppetdb_address"
)

var (
	puppetDBSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_puppetdb_refresh_failures_total",
			Help: "Number of PuppetDB-SD refresh failures.",
		})
	puppetDBSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_puppetdb_refresh_duration_seconds",
			Help: "The duration of a PuppetDB-SD refresh in seconds.",
		})
)

func init() {
	prometheus.MustRegister(puppetDBSDRefreshDuration)
	prometheus.MustRegister(puppetDBSDRefreshFailuresCount)
}

// PuppetDBDiscovery periodically performs PuppetDB-SD requests. It implements
// the TargetProvider interface.
type PuppetDBDiscovery struct {
	cfg      *config.PuppetDBSDConfig
	interval time.Duration
	host     string
	port     int
	query    string
}

// NewDiscovery returns a new PuppetDBDiscoverry which periodically refreshes its targets.
func NewDiscovery(conf *config.PuppetDBSDConfig) *PuppetDBDiscovery {
	return &PuppetDBDiscovery{
		interval: time.Duration(conf.RefreshInterval),
		host:     conf.Host,
		port:     conf.Port,
		query:    conf.Query,
	}
}

// Run implements the TargetProvider interface.
func (pd *PuppetDBDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	ticker := time.NewTicker(pd.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := pd.refresh()
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
			tg, err := pd.refresh()
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

// createPuppetDBClient is a helper function for creating a PuppetDB client.
func createPuppetDBClient(cfg config.PuppetDBSDConfig) (*puppetdb.Client, error) {
	url := fmt.Sprintf("http://%s:%s", cfg.Host, cfg.Port)
	client := puppetdb.NewClient(url, false)
	return client, nil
}

func (pd *PuppetDBDiscovery) refresh() (tg *config.TargetGroup, err error) {
	t0 := time.Now()
	defer func() {
		puppetDBSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			puppetDBSDRefreshFailuresCount.Inc()
		}
	}()
	tg = &config.TargetGroup{}
	client, err := createPuppetDBClient(*pd.cfg)
	if err != nil {
		return tg, fmt.Errorf("could not create PuppetDB client: %s", err)
	}

	query := make(map[string]string)
	query["query"] = pd.query
	url := ""
	ret := []puppetdb.FactJson{}
	err = client.Get(&ret, url, query)

	for _, f := range ret {
		tg.Targets = append(tg.Targets, model.LabelSet{
			addressLabel: model.LabelValue(f.Value),
		})
	}

	return
}
