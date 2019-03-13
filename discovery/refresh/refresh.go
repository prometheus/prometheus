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

type Option func(*Discovery)

// Discovery implements the Discoverer interface.
type Discovery struct {
	logger   log.Logger
	interval time.Duration
	refreshf func(ctx context.Context) ([]*targetgroup.Group, error)
}

// NewDiscovery returns a Discoverer function that calls a refresh() function at every interval.
func NewDiscovery(l log.Logger, interval time.Duration, refreshf func(ctx context.Context) ([]*targetgroup.Group, error), opts ...Option) *Discovery {
	if l == nil {
		l = log.NewNopLogger()
	}
	d := &Discovery{
		logger:   l,
		interval: interval,
		refreshf: refreshf,
	}

	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithDuration measures the duration of the refresh operation.
func WithDuration(o prometheus.Observer) func(*Discovery) {
	return func(d *Discovery) {
		refreshf := d.refreshf
		d.refreshf = func(ctx context.Context) ([]*targetgroup.Group, error) {
			now := time.Now()
			defer o.Observe(time.Since(now).Seconds())
			return refreshf(ctx)
		}
	}
}

// WithFailCount measures the number of failures for the refresh operation.
func WithFailCount(c prometheus.Counter) func(*Discovery) {
	return func(d *Discovery) {
		refreshf := d.refreshf
		d.refreshf = func(ctx context.Context) ([]*targetgroup.Group, error) {
			tgs, err := refreshf(ctx)
			if err != nil {
				c.Inc()
			}
			return tgs, err
		}
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Get an initial set right away.
	tgs, err := d.refreshf(ctx)
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
			tgs, err := d.refreshf(ctx)
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
