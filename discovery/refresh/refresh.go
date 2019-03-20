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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	failuresCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_refresh_failures_total",
			Help: "Number of refresh failures for the given SD mechanism.",
		},
		[]string{"mechanism"},
	)
	duration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_refresh_duration_seconds",
			Help: "The duration of a refresh in seconds for the given SD mechanism.",
		},
		[]string{"mechanism"},
	)
)

func init() {
	prometheus.MustRegister(duration, failuresCount)
}

// Discovery implements the Discoverer interface.
type Discovery struct {
	logger   log.Logger
	interval time.Duration
	refreshf func(ctx context.Context) ([]*targetgroup.Group, error)

	failures prometheus.Counter
	duration prometheus.Observer
}

// NewDiscovery returns a Discoverer function that calls a refresh() function at every interval.
func NewDiscovery(l log.Logger, mech string, interval time.Duration, refreshf func(ctx context.Context) ([]*targetgroup.Group, error)) *Discovery {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &Discovery{
		logger:   l,
		interval: interval,
		refreshf: refreshf,
		failures: failuresCount.WithLabelValues(mech),
		duration: duration.WithLabelValues(mech),
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Get an initial set right away.
	tgs, err := d.refresh(ctx)
	if err != nil {
		level.Error(d.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
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
				level.Error(d.logger).Log("msg", "Unable to refresh target groups", "err", err.Error())
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
	defer d.duration.Observe(time.Since(now).Seconds())
	tgs, err := d.refreshf(ctx)
	if err != nil {
		d.failures.Inc()
	}
	return tgs, err
}
