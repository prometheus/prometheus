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
package otel

import (
	"github.com/prometheus/client_golang/prometheus"

	promex "go.opentelemetry.io/otel/exporters/prometheus"
	otel "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdk "go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
)

func MeterProvider(reg prometheus.Registerer) (otel.MeterProvider, error) {
	if reg == nil {
		return noop.NewMeterProvider(), nil
	}

	ex, err := promex.New(promex.WithRegisterer(reg))
	if err != nil {
		return nil, err
	}

	return sdk.NewMeterProvider(sdk.WithReader(ex)), nil
}

func TelemetrySettings(reg prometheus.Registerer) (component.TelemetrySettings, error) {
	var set component.TelemetrySettings

	mp, err := MeterProvider(reg)
	if err != nil {
		return set, err
	}

	set.MeterProvider = mp
	set.LeveledMeterProvider = func(_ configtelemetry.Level) otel.MeterProvider {
		return mp
	}
	return set, nil
}
