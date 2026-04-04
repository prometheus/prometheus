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

type mockObserver struct {
	observed float64
}

func (m *mockObserver) Observe(v float64) {
	m.observed = v
}

func TestGate(t *testing.T) {
	obs := &mockObserver{}
	g := New(1, obs)

	ctx := context.Background()
	if err := g.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// This should block and record waiting time.
	go func() {
		time.Sleep(10 * time.Millisecond)
		g.Done()
	}()

	if err := g.Start(ctx); err != nil {
		t.Fatal(err)
	}

	if obs.observed <= 0 {
		t.Errorf("expected observed waiting time to be > 0, got %f", obs.observed)
	}
}
