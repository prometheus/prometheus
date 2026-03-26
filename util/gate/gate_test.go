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
	"time"
)

func TestGateStart(t *testing.T) {
	g := New(1)

	ctx := context.Background()
	if err := g.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g.Done()
}

func TestGateStartContextDone(t *testing.T) {
	g := New(1)

	// Fill the gate
	ctx := context.Background()
	if err := g.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try to start with a cancelled context
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := g.Start(cancelledCtx); err == nil {
		t.Fatal("expected error from cancelled context")
	}

	g.Done()
}

func TestGateDone(t *testing.T) {
	g := New(2)

	ctx := context.Background()
	if err := g.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := g.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	g.Done()
	g.Done()
}

func TestGateDonePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from Done() without Start()")
		}
	}()

	g := New(1)
	g.Done()
}

func TestGateWaitingDuration(t *testing.T) {
	g := New(1)
	ctx := context.Background()

	// Start first request to fill the gate
	if err := g.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start a goroutine that will wait
	done := make(chan error, 1)
	go func() {
		startTime := time.Now()
		err := g.Start(ctx)
		waitTime := time.Since(startTime)
		if waitTime < 50*time.Millisecond {
			t.Logf("wait time was %v (expected at least ~100ms delay)", waitTime)
		}
		done <- err
	}()

	// Give the goroutine time to block
	time.Sleep(100 * time.Millisecond)

	// Release the gate so the waiting goroutine can proceed
	g.Done()

	if err := <-done; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	g.Done()
}
