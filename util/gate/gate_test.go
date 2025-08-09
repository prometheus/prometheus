// Copyright 2015 The Prometheus Authors
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
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// newTestGate creates a new Gate with associated testable metrics.
func newTestGate(length int) (*Gate, *prometheus.GaugeVec, *prometheus.CounterVec) {
	available := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_gate_available",
		Help: "Test available slots.",
	}, []string{"test"})

	waitDuration := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_gate_wait_duration_seconds",
		Help: "Test wait duration.",
	}, []string{"test"})

	gate := New(length, available.WithLabelValues("test"), waitDuration.WithLabelValues("test"))
	return gate, available, waitDuration
}

func TestGate_NoWait(t *testing.T) {
	g, available, waitDuration := newTestGate(1)

	require.Equal(t, 1.0, testutil.ToFloat64(available), "Initial available slots should be 1")

	err := g.Start(context.Background())
	require.NoError(t, err)

	require.Equal(t, 0.0, testutil.ToFloat64(available), "Available slots should be 0 after start")
	require.InDelta(t, 0.0, testutil.ToFloat64(waitDuration), 0.1, "Wait duration should be close to 0 when not waiting")

	g.Done()

	require.Equal(t, 1.0, testutil.ToFloat64(available), "Available slots should be 1 after done")
}

func TestGate_WithWait(t *testing.T) {
	g, available, waitDuration := newTestGate(1)

	err := g.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(available), "Available slots should be 0")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := g.Start(context.Background())
		require.NoError(t, err)
		defer g.Done()
	}()

	time.Sleep(100 * time.Millisecond)

	require.Equal(t, -1.0, testutil.ToFloat64(available), "Available slots should be -1 with one queued request")

	g.Done()
	wg.Wait()

	require.Equal(t, 1.0, testutil.ToFloat64(available), "Available slots should be 1 after all requests are done")
	require.GreaterOrEqual(t, testutil.ToFloat64(waitDuration), 0.1, "Wait duration should be at least 0.1s")
}

func TestGate_ContextCancellation(t *testing.T) {
	g, available, waitDuration := newTestGate(1)

	err := g.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0.0, testutil.ToFloat64(available), "Available slots should be 0")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := g.Start(ctx)
		require.Error(t, err)
		require.Equal(t, context.DeadlineExceeded, err)
	}()

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, -1.0, testutil.ToFloat64(available), "Available slots should be -1 with one queued request")

	wg.Wait()

	require.Equal(t, 0.0, testutil.ToFloat64(available), "Available slots should be 0 after context cancellation")
	require.GreaterOrEqual(t, testutil.ToFloat64(waitDuration), 0.1, "Wait duration should be at least 0.1s after cancellation")

	g.Done()

	require.Equal(t, 1.0, testutil.ToFloat64(available), "Available slots should be 1 after all requests are done")
}
