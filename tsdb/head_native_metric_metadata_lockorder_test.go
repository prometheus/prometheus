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

//go:build nativemetadatalockorder

package tsdb

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func resetLockOrderViolations() { resetLockOrder() }

func requireNoLockOrderViolations(t testing.TB) {
	t.Helper()
	require.Empty(t, takeLockOrderViolations())
}

func logLockOrderChecks(t testing.TB) {
	taken, checks := lockOrderCounts()
	t.Logf("lock order checked: %d series, %d stripe and %d repair locks taken, %d WAL writes", taken[lockSeries], taken[lockStripe], taken[lockRepair], checks)
}

// TestNativeMetricMetadataRepair_LockOrder checks that the instrumented locks
// record each kind of violation. The repair tests check, under this tag, that
// none occurs.
// lockOrderInstrumented reports whether locks are instrumented, which
// allocates.
const lockOrderInstrumented = true

func TestNativeMetricMetadataRepair_LockOrder(t *testing.T) {
	setup := func(t *testing.T) (*repairHarness, *memSeries) {
		x := newRepairHarness(t)
		ref := x.mustCommit(labels.FromStrings(labels.MetricName, "a"), 100, "a")
		resetLockOrderViolations()
		return x, x.h.series.getByID(ref)
	}
	for _, c := range []struct {
		name string
		do   func(x *repairHarness, s *memSeries)
		want string
	}{
		{
			name: "a series locked under the repair mutex",
			do: func(x *repairHarness, s *memSeries) {
				x.h.nativeMetadataRepairs.mtx.Lock()
				s.Lock()
				gced := s.isGCed()
				s.Unlock()
				x.h.nativeMetadataRepairs.mtx.Unlock()
				require.False(t, gced)
			},
			want: "a series lock taken while the repair mutex is held",
		},
		{
			name: "a stripe locked under the repair mutex",
			do: func(x *repairHarness, _ *memSeries) {
				x.h.nativeMetadataRepairs.mtx.Lock()
				x.h.series.locks[0].RLock()
				stripe := x.h.series.series[0]
				x.h.series.locks[0].RUnlock()
				x.h.nativeMetadataRepairs.mtx.Unlock()
				require.NotNil(t, stripe)
			},
			want: "a stripe lock taken while the repair mutex is held",
		},
		{
			name: "the repair mutex under a stripe lock",
			do: func(x *repairHarness, _ *memSeries) {
				x.h.series.locks[0].Lock()
				x.h.nativeMetadataRepairs.mtx.Lock()
				pending := len(x.h.nativeMetadataRepairs.pending)
				x.h.nativeMetadataRepairs.mtx.Unlock()
				x.h.series.locks[0].Unlock()
				require.Zero(t, pending)
			},
			want: "the repair mutex taken while a stripe lock is held",
		},
		{
			name: "repair code locking a series under a stripe lock",
			do: func(x *repairHarness, s *memSeries) {
				x.h.series.locks[0].RLock()
				require.True(t, x.h.lockPendingNativeMetadataSeries(s, nil))
				s.Unlock()
				x.h.series.locks[0].RUnlock()
			},
			want: "repair code locked a series while holding 1 stripe and 0 series locks",
		},
		{
			name: "a WAL write holding a series lock",
			do: func(x *repairHarness, s *memSeries) {
				s.Lock()
				require.NoError(t, x.h.wal.Log([]byte{0}))
				s.Unlock()
			},
			want: "a WAL write while holding 1 series, 0 stripe and 0 repair locks",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, s := setup(t)
			c.do(x, s)
			require.Equal(t, []string{c.want}, takeLockOrderViolations())
		})
	}
	t.Run("ordinary series locks from other code", func(t *testing.T) {
		// The Head's own GC locks series under stripe locks.
		x, _ := setup(t)
		x.h.gc()
		require.Empty(t, takeLockOrderViolations())
	})
}
