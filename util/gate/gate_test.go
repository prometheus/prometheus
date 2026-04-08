// Copyright The Prometheus Authors
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

package gate

import (
	"context"
	"testing"
	"time"
)

func TestGate(t *testing.T) {
	obs := &mockObserver{}
	// Use NewWithObserver because we are passing an observer
	g := NewWithObserver(1, obs)

	ctx := context.Background()
	// 1. Acquire the first slot (no waiting)
	if err := g.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// 2. This goroutine will release the slot after 10ms
	go func() {
		time.Sleep(10 * time.Millisecond)
		g.Done()
	}()

	// 3. This will BLOCK for ~10ms until the goroutine calls Done()
	if err := g.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// 4. Verify that we recorded a wait duration
	if obs.observed <= 0 {
		t.Errorf("expected observed waiting time to be > 0, got %f", obs.observed)
	}
}

// mockObserver is a simple implementation of the Observer interface for testing.
type mockObserver struct {
	observed float64
}

func (m *mockObserver) Observe(v float64) {
	m.observed = v
}
