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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestGateStart(t *testing.T) {
	g := New(1)

	ctx := context.Background()
	require.NoError(t, g.Start(ctx))

	// Gate is full, second Start should block until context timeout.
	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	require.Error(t, g.Start(ctx2))

	g.Done()

	require.NoError(t, g.Start(ctx))
	g.Done()
}

func TestGateStartContextCanceled(t *testing.T) {
	g := New(1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.Error(t, g.Start(ctx))
}

func TestGateDonePanic(t *testing.T) {
	g := New(1)

	require.Panics(t, func() {
		g.Done()
	})
}

func TestNewInstrumentedRecordsWaitingDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	g := NewInstrumented(reg, 1)

	ctx := context.Background()
	require.NoError(t, g.Start(ctx))

	// Verify the metric was recorded.
	count := getSampleCount(t, reg, "gate_waiting_seconds")
	require.Equal(t, uint64(1), count, "expected 1 observation after Start")

	g.Done()
}

func TestInstrumentedGateWaitingDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Gate with concurrency 1.
	g := NewInstrumented(reg, 1)

	ctx := context.Background()
	require.NoError(t, g.Start(ctx))

	// Now the gate is full. Start a goroutine that will wait.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		require.NoError(t, g.Start(ctx))
		g.Done()
	}()

	// Let the goroutine block for a bit, then release the gate.
	time.Sleep(100 * time.Millisecond)
	g.Done()
	wg.Wait()

	// Should have 2 observations total now.
	count := getSampleCount(t, reg, "gate_waiting_seconds")
	require.Equal(t, uint64(2), count, "expected 2 observations total")
}

func TestNewWithoutInstrumentationDoesNotPanic(t *testing.T) {
	// Ensure the non-instrumented gate still works fine (no nil pointer panic).
	g := New(1)
	ctx := context.Background()
	require.NoError(t, g.Start(ctx))
	g.Done()
}

// getSampleCount gathers metrics from the registry and returns the sample
// count of the histogram with the given name.
func getSampleCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()

	metrics, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range metrics {
		if mf.GetName() == name {
			m := mf.GetMetric()
			require.NotEmpty(t, m)
			return getHistogramCount(m[0])
		}
	}
	t.Fatalf("metric %q not found in registry", name)
	return 0
}

func getHistogramCount(m *dto.Metric) uint64 {
	if h := m.GetHistogram(); h != nil {
		return h.GetSampleCount()
	}
	return 0
}
