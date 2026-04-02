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

	"github.com/stretchr/testify/require"
)

// mockObserver implements the Observer interface for testing.
type mockObserver struct {
	mu           sync.Mutex
	observations []float64
}

func (m *mockObserver) Observe(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observations = append(m.observations, v)
}

func (m *mockObserver) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.observations)
}

func (m *mockObserver) lastObservation() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.observations) == 0 {
		return 0
	}
	return m.observations[len(m.observations)-1]
}

func TestNew(t *testing.T) {
	g := New(1)
	require.NotNil(t, g)
	require.NotNil(t, g.ch)
	require.Nil(t, g.waitDuration)
}

func TestNewWithMetric(t *testing.T) {
	observer := &mockObserver{}
	g := NewWithMetric(1, observer)
	require.NotNil(t, g)
	require.NotNil(t, g.ch)
	require.NotNil(t, g.waitDuration)
}

func TestGate_StartWithoutMetric(t *testing.T) {
	g := New(1)
	err := g.Start(context.Background())
	require.NoError(t, err)
	g.Done()
}

func TestGate_StartWithMetric(t *testing.T) {
	observer := &mockObserver{}
	g := NewWithMetric(1, observer)

	err := g.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, observer.count())

	// The observation should be very small (near 0) since there's no blocking
	lastObs := observer.lastObservation()
	require.GreaterOrEqual(t, lastObs, 0.0)
	require.Less(t, lastObs, 0.1) // Should be much less than 100ms

	g.Done()
}

func TestGate_WaitDuration(t *testing.T) {
	observer := &mockObserver{}
	g := NewWithMetric(1, observer)

	// Fill the gate
	err := g.Start(context.Background())
	require.NoError(t, err)

	// Start a goroutine that will block
	done := make(chan struct{})
	go func() {
		err := g.Start(context.Background())
		require.NoError(t, err)
		close(done)
		g.Done()
	}()

	// Let the goroutine block for a bit
	time.Sleep(50 * time.Millisecond)

	// Release the gate
	g.Done()

	// Wait for the goroutine to finish
	<-done

	// We should have 2 observations
	require.Equal(t, 2, observer.count())

	// The second observation should reflect the wait time (at least 50ms)
	lastObs := observer.lastObservation()
	require.GreaterOrEqual(t, lastObs, 0.05) // At least 50ms
}

func TestGate_ContextCancellation(t *testing.T) {
	observer := &mockObserver{}
	g := NewWithMetric(1, observer)

	// Fill the gate
	err := g.Start(context.Background())
	require.NoError(t, err)

	// Try to start with a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = g.Start(ctx)
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)

	// Should only have 1 observation (from the first Start)
	require.Equal(t, 1, observer.count())

	g.Done()
}

func TestGate_DonePanic(t *testing.T) {
	g := New(1)

	// Done without Start should panic
	require.Panics(t, func() {
		g.Done()
	})
}

func TestGate_Concurrency(t *testing.T) {
	observer := &mockObserver{}
	const concurrency = 3
	g := NewWithMetric(concurrency, observer)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			err := g.Start(context.Background())
			require.NoError(t, err)
			time.Sleep(10 * time.Millisecond)
			g.Done()
		}()
	}

	wg.Wait()

	// All goroutines should have completed
	require.Equal(t, numGoroutines, observer.count())
}
