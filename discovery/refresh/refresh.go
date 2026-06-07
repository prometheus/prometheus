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

package refresh

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Options struct {
	Logger              *slog.Logger
	Mech                string
	SetName             string
	Interval            time.Duration
	RefreshF            func(ctx context.Context) ([]*targetgroup.Group, error)
	MetricsInstantiator discovery.RefreshMetricsInstantiator
}

// Discovery implements the Discoverer interface.
type Discovery struct {
	logger   *slog.Logger
	interval time.Duration
	refreshf func(ctx context.Context) ([]*targetgroup.Group, error)
	metrics  *discovery.RefreshMetrics
}

// NewDiscovery returns a Discoverer function that calls a refresh() function at every interval.
func NewDiscovery(opts Options) *Discovery {
	m := opts.MetricsInstantiator.Instantiate(opts.Mech, opts.SetName)

	var logger *slog.Logger
	if opts.Logger == nil {
		logger = promslog.NewNopLogger()
	} else {
		logger = opts.Logger
	}

	d := Discovery{
		logger:   logger,
		interval: opts.Interval,
		refreshf: opts.RefreshF,
		metrics:  m,
	}

	return &d
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Get an initial set right away.
	tgs, err := d.refresh(ctx)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			d.logger.Error("Unable to refresh target groups", "err", err.Error())
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
					d.logger.Error("Unable to refresh target groups", "err", err.Error())
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
		d.metrics.Duration.Observe(time.Since(now).Seconds())
		d.metrics.DurationHistogram.Observe(time.Since(now).Seconds())
	}()

	tgs, err := d.refreshf(ctx)
	if err != nil {
		d.metrics.Failures.Inc()
	}
	return tgs, err
}
