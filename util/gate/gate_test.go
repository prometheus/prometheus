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

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestGateStart(t *testing.T) {
	g := New(1, nil)

	require.NoError(t, g.Start(context.Background()))
	g.Done()
}

func TestGateStartBlocksWhenFull(t *testing.T) {
	g := New(1, nil)

	require.NoError(t, g.Start(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := g.Start(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	g.Done()
}

func TestGateWaitDurationObserved(t *testing.T) {
	reg := prometheus.NewRegistry()
	g := New(1, reg)

	require.NoError(t, g.Start(context.Background()))
	g.Done()

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var hist *dto.Histogram
	for _, mf := range mfs {
		if mf.GetName() == "gate_wait_duration_seconds" {
			hist = mf.GetMetric()[0].GetHistogram()
			break
		}
	}
	require.NotNil(t, hist, "expected gate_wait_duration_seconds histogram to be present")
	require.Equal(t, uint64(1), hist.GetSampleCount())
}

func TestGateWaitDurationReflectsActualWait(t *testing.T) {
	reg := prometheus.NewRegistry()
	g := New(1, reg)

	// Fill the gate.
	require.NoError(t, g.Start(context.Background()))

	// Release after a short delay so the next Start observes non-zero wait.
	go func() {
		time.Sleep(100 * time.Millisecond)
		g.Done()
	}()

	require.NoError(t, g.Start(context.Background()))
	g.Done()

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var hist *dto.Histogram
	for _, mf := range mfs {
		if mf.GetName() == "gate_wait_duration_seconds" {
			hist = mf.GetMetric()[0].GetHistogram()
			break
		}
	}
	require.NotNil(t, hist)
	// Two Start calls observed: one fast, one ~100ms.
	require.Equal(t, uint64(2), hist.GetSampleCount())
	require.Greater(t, hist.GetSampleSum(), 0.05, "expected meaningful wait time to be recorded")
}

func TestGateDonePanicsWhenNoneStarted(t *testing.T) {
	g := New(1, nil)
	require.Panics(t, func() { g.Done() })
}
