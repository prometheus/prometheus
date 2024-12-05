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
package otel_test

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"

	"github.com/prometheus/prometheus/util/otel"
	"github.com/prometheus/prometheus/util/otel/fake"
)

func Example() {
	var steps []otel.Step

	for i := range 3 {
		f := fake.Factory(func(ctx context.Context, _ processor.Settings, _ component.Config, next consumer.Metrics) (processor.Metrics, error) {
			return fake.Processor{
				Lifecycle: fake.Lifecycle{
					Startf: func(context.Context, component.Host) error {
						fmt.Printf("proc%d: start\n", i)
						return nil
					},
					Stopf: func(context.Context) error {
						fmt.Printf("proc%d: stop\n", i)
						return nil
					},
				},
				Consumer: fake.Consumer(func(ctx context.Context, md pmetric.Metrics) error {
					fmt.Printf("proc%d: consume\n", i)
					md.ResourceMetrics().At(0).
						ScopeMetrics().At(0).
						Metrics().At(0).
						Sum().DataPoints().AppendEmpty().
						SetIntValue(int64(i))
					return next.ConsumeMetrics(ctx, md)
				}),
			}, nil
		})
		steps = append(steps, otel.Processor(f, nil))
	}

	steps = append(steps, otel.Sink(fake.Consumer(func(ctx context.Context, md pmetric.Metrics) error {
		data, _ := (&pmetric.JSONMarshaler{}).MarshalMetrics(md)
		fmt.Println("sink:", string(data))
		return nil
	})))

	pl, err := otel.Use(nil, steps...)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	if err := pl.Start(ctx, nil); err != nil {
		panic(err)
	}

	md := pmetric.NewMetrics()
	md.ResourceMetrics().AppendEmpty().
		ScopeMetrics().AppendEmpty().
		Metrics().AppendEmpty().
		SetEmptySum()

	err = pl.ConsumeMetrics(ctx, md)
	if err != nil {
		panic(err)
	}

	if err := pl.Shutdown(ctx); err != nil {
		panic(err)
	}

	// Output:
	// proc0: start
	// proc1: start
	// proc2: start
	// proc0: consume
	// proc1: consume
	// proc2: consume
	// sink: {"resourceMetrics":[{"resource":{},"scopeMetrics":[{"scope":{},"metrics":[{"sum":{"dataPoints":[{"asInt":"0"},{"asInt":"1"},{"asInt":"2"}]}}]}]}]}
	// proc0: stop
	// proc1: stop
	// proc2: stop
}
