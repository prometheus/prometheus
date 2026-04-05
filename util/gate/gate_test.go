// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gate

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestGateWaitDurationMetricObserved(t *testing.T) {
	registry := prometheus.NewRegistry()
	gate := New(1, registry, "test")

	ctx := context.Background()
	err := gate.Start(ctx)
	require.NoError(t, err)
	gate.Done()

	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)

	metric := families[0]
	require.Equal(t, "test_gate_wait_duration_seconds", *metric.Name)
	require.Len(t, metric.Metric, 1)

	histogram := metric.Metric[0].Histogram
	require.NotNil(t, histogram)
	require.Greater(t, *histogram.SampleCount, uint64(0))
}

func TestGateWithNilRegisterer(t *testing.T) {
	gate := New(1, nil, "test")

	ctx := context.Background()
	err := gate.Start(ctx)
	require.NoError(t, err)
	gate.Done()
}

func TestGateCancellationNoMetricRecorded(t *testing.T) {
	registry := prometheus.NewRegistry()
	gate := New(1, registry, "test")

	ctx := context.Background()
	err := gate.Start(ctx)
	require.NoError(t, err)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err = gate.Start(cancelledCtx)
	require.Error(t, err)

	gate.Done()

	families, _ := registry.Gather()
	require.Len(t, families, 1)
	histogram := families[0].Metric[0].Histogram
	require.Equal(t, uint64(1), *histogram.SampleCount)
}
