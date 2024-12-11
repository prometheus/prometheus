// Copyright 2022 The Prometheus Authors
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
package fake

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
)

var _ component.Component = Lifecycle{}

type Lifecycle struct {
	Startf func(context.Context, component.Host) error
	Stopf  func(context.Context) error
}

func (lc Lifecycle) Start(ctx context.Context, host component.Host) error {
	return lc.Startf(ctx, host)
}

func (lc Lifecycle) Shutdown(ctx context.Context) error {
	return lc.Stopf(ctx)
}

var _ consumer.Metrics = Consumer(nil)

type Consumer func(context.Context, pmetric.Metrics) error

func (fn Consumer) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	return fn(ctx, md)
}

func (fn Consumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{}
}

var _ processor.Metrics = Processor{}

type Processor struct {
	Lifecycle
	Consumer
}

func Factory(metrics func(context.Context, processor.Settings, component.Config, consumer.Metrics) (processor.Metrics, error)) processor.Factory {
	return factory{metrics: metrics}
}

type factory struct {
	processor.Factory
	metrics func(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Metrics) (processor.Metrics, error)
}

func (f factory) CreateMetrics(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Metrics) (processor.Metrics, error) {
	return f.metrics(ctx, set, cfg, next)
}

func (f factory) CreateDefaultConfig() component.Config {
	return nil
}
