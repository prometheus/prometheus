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

func TestGate(t *testing.T) {
	t.Run("basic functionality", func(t *testing.T) {
		g := New(2)

		// First two should succeed immediately
		require.NoError(t, g.Start(context.Background()))
		require.NoError(t, g.Start(context.Background()))

		// Release one
		g.Done()

		// Should succeed after release
		require.NoError(t, g.Start(context.Background()))

		// Release all
		g.Done()
		g.Done()
	})

	t.Run("context cancellation", func(t *testing.T) {
		g := New(1)
		require.NoError(t, g.Start(context.Background()))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := g.Start(ctx)
		require.ErrorIs(t, err, context.Canceled)

		g.Done()
	})

	t.Run("context timeout", func(t *testing.T) {
		g := New(1)
		require.NoError(t, g.Start(context.Background()))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := g.Start(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		g.Done()
	})

	t.Run("panic on extra done", func(t *testing.T) {
		g := New(1)
		require.NoError(t, g.Start(context.Background()))
		g.Done()

		require.Panics(t, func() {
			g.Done()
		})
	})
}

// mockObserver is a simple implementation of prometheus.Observer for testing.
type mockObserver struct {
	mu     sync.Mutex
	values []float64
}

func (m *mockObserver) Observe(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = append(m.values, v)
}

func (m *mockObserver) getValues() []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]float64(nil), m.values...)
}

func TestInstrumentedGate(t *testing.T) {
	t.Run("records waiting duration", func(t *testing.T) {
		obs := &mockObserver{}
		g := NewInstrumented(1, obs)

		// First request should have very low wait time
		require.NoError(t, g.Start(context.Background()))

		values := obs.getValues()
		require.Len(t, values, 1)
		require.Less(t, values[0], 0.001) // Should be < 1ms

		g.Done()
	})

	t.Run("records longer wait when gate is full", func(t *testing.T) {
		obs := &mockObserver{}
		g := NewInstrumented(1, obs)

		// Fill the gate
		require.NoError(t, g.Start(context.Background()))

		// Start a goroutine that will wait
		done := make(chan struct{})
		go func() {
			defer close(done)
			require.NoError(t, g.Start(context.Background()))
		}()

		// Wait a bit then release
		time.Sleep(50 * time.Millisecond)
		g.Done()

		// Wait for the goroutine
		<-done

		values := obs.getValues()
		require.Len(t, values, 2)

		// Second request should have waited at least 50ms
		require.GreaterOrEqual(t, values[1], 0.05)

		g.Done()
	})

	t.Run("no metric recorded on context cancellation", func(t *testing.T) {
		obs := &mockObserver{}
		g := NewInstrumented(1, obs)

		// Fill the gate
		require.NoError(t, g.Start(context.Background()))

		// Try with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := g.Start(ctx)
		require.ErrorIs(t, err, context.Canceled)

		// Should only have 1 observation (the successful one)
		values := obs.getValues()
		require.Len(t, values, 1)

		g.Done()
	})
}
