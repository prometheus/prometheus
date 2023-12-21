// Copyright 2019 The Prometheus Authors
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

package refresh

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Options struct {
	Logger   log.Logger
	Mech     string
	Interval time.Duration
	RefreshF func(ctx context.Context) ([]*targetgroup.Group, error)
	Registry prometheus.Registerer
	Metrics  []prometheus.Collector
}

// Discovery implements the Discoverer interface.
type Discovery struct {
	logger   log.Logger
	interval time.Duration
	refreshf func(ctx context.Context) ([]*targetgroup.Group, error)

	failures prometheus.Counter
	duration prometheus.Summary

	metricRegisterer discovery.MetricRegisterer
}

// NewDiscovery returns a Discoverer function that calls a refresh() function at every interval.
func NewDiscovery(opts Options) *Discovery {
	var logger log.Logger
	if opts.Logger == nil {
		logger = log.NewNopLogger()
	} else {
		logger = opts.Logger
	}

	d := Discovery{
		logger:   logger,
		interval: opts.Interval,
		refreshf: opts.RefreshF,
		failures: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_sd_refresh_failures_total",
				Help: "Number of refresh failures for the given SD mechanism.",
				ConstLabels: prometheus.Labels{
					"mechanism": opts.Mech,
				},
			}),
		duration: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Name:       "prometheus_sd_refresh_duration_seconds",
				Help:       "The duration of a refresh in seconds for the given SD mechanism.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
				ConstLabels: prometheus.Labels{
					"mechanism": opts.Mech,
				},
			}),
	}

	metrics := []prometheus.Collector{d.failures, d.duration}
	if opts.Metrics != nil {
		metrics = append(metrics, opts.Metrics...)
	}

	d.metricRegisterer = discovery.NewMetricRegisterer(opts.Registry, metrics)

	return &d
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	err := d.metricRegisterer.RegisterMetrics()
	if err != nil {
		level.Error(d.logger).Log("msg", "Unable to register metrics", "err", err.Error())
		return
	}
	defer d.metricRegisterer.UnregisterMetrics()

	// Get an initial set right away.
	tgs, err := d.refresh(ctx)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			level.Error(d.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
		}
	} else {
		select {
		case ch <- tgs:
		case <-ctx.Done():
			return
		}
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tgs, err := d.refresh(ctx)
			if err != nil {
				if !errors.Is(ctx.Err(), context.Canceled) {
					level.Error(d.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
				}
				continue
			}

			select {
			case ch <- tgs:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	now := time.Now()
	defer func() {
		d.duration.Observe(time.Since(now).Seconds())
	}()

	tgs, err := d.refreshf(ctx)
	if err != nil {
		d.failures.Inc()
	}
	return tgs, err
}
