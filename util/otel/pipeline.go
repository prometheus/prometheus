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

// Utilities for using OpenTelemetry collector components. See [Use] for usage.
package otel

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
)

// Pipeline resembles an OpenTelemetry Collector metrics pipeline.
// Usage:
//
//   - Start: call once to start all components
//   - ConsumeMetrics: call for each metrics request
//   - Shutdown: call once no longer needed
//
// The pipeline can consist of any combination of steps, but MUST end in a [Sink]
type Pipeline interface {
	component.Component
	consumer.Metrics
}

type Step = func(set Settings, next consumer.Metrics) (consumer.Metrics, error)

// Sink creates the final step in a pipeline.
// Must be last, or steps coming after this are not invoked.
func Sink(ex consumer.Metrics) Step {
	return func(_ Settings, _ consumer.Metrics) (consumer.Metrics, error) {
		return ex, nil
	}
}

// Processor creates a step using the given [processor.Factory].
//
// If cfg is nil, [processor.Factory.CreateDefaultConfig] is used.
func Processor(fac processor.Factory, cfg component.Config) Step {
	if cfg == nil {
		cfg = fac.CreateDefaultConfig()
	}
	return func(set Settings, next consumer.Metrics) (consumer.Metrics, error) {
		return fac.CreateMetrics(context.Background(), set, cfg, next)
	}
}

// Use creates a new pipeline using the given steps.
// The pipeline is set up in a way such that steps are called in the order given here.
func Use(reg prometheus.Registerer, steps ...Step) (Pipeline, error) {
	pl := pipeline{steps: make([]consumer.Metrics, len(steps))}

	tel, err := TelemetrySettings(reg)
	if err != nil {
		return nil, err
	}

	var next consumer.Metrics
	for i := len(steps) - 1; i >= 0; i-- {
		set := Settings{
			TelemetrySettings: tel,
		}

		create := steps[i]
		pl.steps[i], err = create(set, next)
		if err != nil {
			return nil, err
		}
		next = pl.steps[i]
	}

	return pl, nil
}

var _ Pipeline = pipeline{}

type pipeline struct {
	steps []consumer.Metrics
}

func (pl pipeline) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	if pl.steps == nil {
		return nil
	}
	return pl.steps[0].ConsumeMetrics(ctx, md)
}

func (pl pipeline) Start(ctx context.Context, host component.Host) error {
	for _, step := range pl.steps {
		if c, ok := step.(component.Component); ok {
			err := c.Start(ctx, host)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (pl pipeline) Shutdown(ctx context.Context) error {
	var errs error
	for _, step := range pl.steps {
		if c, ok := step.(component.Component); ok {
			err := c.Shutdown(ctx)
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (pl pipeline) Capabilities() consumer.Capabilities {
	for _, step := range pl.steps {
		if step.Capabilities().MutatesData {
			return step.Capabilities()
		}
	}
	return consumer.Capabilities{MutatesData: false}
}

type Settings = processor.Settings
