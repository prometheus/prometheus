// Copyright 2023 The Prometheus Authors
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

package discovery

import (
	"context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type wrappedDiscoverer struct {
	instance model.LabelValue
	d        Discoverer
}

func NewWrappedDiscoverer(cfg Config, opts DiscovererOptions) (Discoverer, error) {
	d, err := cfg.NewDiscoverer(opts)
	if err != nil {
		return nil, err
	}

	iCfg, ok := cfg.(NamedConfig)
	// return the default config if it is not an instance config.
	if !ok {
		return d, nil
	}

	return &wrappedDiscoverer{
		instance: model.LabelValue(iCfg.InstanceName()),
		d:        d,
	}, nil
}

func (d *wrappedDiscoverer) Run(ctx context.Context, outCh chan<- []*targetgroup.Group) {
	inCh := make(chan []*targetgroup.Group, cap(outCh))

	go func() {
		for tgs := range inCh {
			for _, tg := range tgs {
				tg.Labels["__meta_sd_instance"] = d.instance
			}
			outCh <- tgs
		}
		close(outCh)
	}()
	// Run the underlying discoverer.
	d.d.Run(ctx, inCh)
}
