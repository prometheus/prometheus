// Copyright 2017 The Prometheus Authors
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

package tombstones

import (
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

func TestWriteAndReadbackTombstones(t *testing.T) {
	tmpdir := t.TempDir()

	ref := uint64(0)

	stones := NewMemTombstones()
	// Generate the tombstones.
	for i := 0; i < 100; i++ {
		ref += uint64(rand.Int31n(10)) + 1
		numRanges := rand.Intn(5) + 1
		dranges := make(Intervals, 0, numRanges)
		mint := rand.Int63n(time.Now().UnixNano())
		for j := 0; j < numRanges; j++ {
			dranges = dranges.Add(Interval{mint, mint + rand.Int63n(1000)})
			mint += rand.Int63n(1000) + 1
		}
		stones.AddInterval(storage.SeriesRef(ref), dranges...)
	}

	_, err := WriteFile(log.NewNopLogger(), tmpdir, stones)
	require.NoError(t, err)

	restr, _, err := ReadTombstones(tmpdir)
	require.NoError(t, err)

	// Compare the two readers.
	require.Equal(t, stones, restr)
}

func TestDeletingTombstones(t *testing.T) {
	stones := NewMemTombstones()

	ref := storage.SeriesRef(42)
	mint := rand.Int63n(time.Now().UnixNano())
	dranges := make(Intervals, 0, 1)
	dranges = dranges.Add(Interval{mint, mint + rand.Int63n(1000)})
	stones.AddInterval(ref, dranges...)
	stones.AddInterval(storage.SeriesRef(43), dranges...)

	intervals, err := stones.Get(ref)
	require.NoError(t, err)
	require.Equal(t, intervals, dranges)

	stones.DeleteTombstones(map[storage.SeriesRef]struct{}{ref: {}})

	intervals, err = stones.Get(ref)
	require.NoError(t, err)
	require.Empty(t, intervals)
}

func TestTruncateBefore(t *testing.T) {
	cases := []struct {
		before  Intervals
		beforeT int64
		after   Intervals
	}{
		{
			before:  Intervals{{1, 2}, {4, 10}, {12, 100}},
			beforeT: 3,
			after:   Intervals{{4, 10}, {12, 100}},
		},
		{
			before:  Intervals{{1, 2}, {4, 10}, {12, 100}, {200, 1000}},
			beforeT: 900,
			after:   Intervals{{200, 1000}},
		},
		{
			before:  Intervals{{1, 2}, {4, 10}, {12, 100}, {200, 1000}},
			beforeT: 2000,
			after:   nil,
		},
		{
			before:  Intervals{{1, 2}, {4, 10}, {12, 100}, {200, 1000}},
			beforeT: 0,
			after:   Intervals{{1, 2}, {4, 10}, {12, 100}, {200, 1000}},
		},
	}
	for _, c := range cases {
		ref := storage.SeriesRef(42)
		stones := NewMemTombstones()
		stones.AddInterval(ref, c.before...)

		stones.TruncateBefore(c.beforeT)
		ts, err := stones.Get(ref)
		require.NoError(t, err)
		require.Equal(t, c.after, ts)
	}
}

func TestAddingNewIntervals(t *testing.T) {
	cases := []struct {
		exist Intervals
		new   Interval

		exp Intervals
	}{
		{
			new: Interval{1, 2},
			exp: Intervals{{1, 2}},
		},
		{
			exist: Intervals{{1, 2}},
			new:   Interval{1, 2},
			exp:   Intervals{{1, 2}},
		},
		{
			exist: Intervals{{1, 4}, {6, 6}},
			new:   Interval{5, 6},
			exp:   Intervals{{1, 6}},
		},
		{
			exist: Intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   Interval{21, 25},
			exp:   Intervals{{1, 10}, {12, 30}},
		},
		{
			exist: Intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   Interval{22, 23},
			exp:   Intervals{{1, 10}, {12, 20}, {22, 23}, {25, 30}},
		},
		{
			exist: Intervals{{1, 2}, {3, 5}, {7, 7}},
			new:   Interval{6, 7},
			exp:   Intervals{{1, 2}, {3, 7}},
		},
		{
			exist: Intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   Interval{18, 23},
			exp:   Intervals{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: Intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   Interval{9, 23},
			exp:   Intervals{{1, 23}, {25, 30}},
		},
		{
			exist: Intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   Interval{9, 230},
			exp:   Intervals{{1, 230}},
		},
		{
			exist: Intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   Interval{1, 4},
			exp:   Intervals{{1, 10}, {12, 20}, {25, 30}},
		},
		{
			exist: Intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   Interval{11, 14},
			exp:   Intervals{{5, 20}, {25, 30}},
		},
		{
			exist: Intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   Interval{1, 3},
			exp:   Intervals{{1, 3}, {5, 10}, {12, 20}, {25, 30}},
		},
		{
			exist: Intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   Interval{35, 40},
			exp:   Intervals{{5, 10}, {12, 20}, {25, 30}, {35, 40}},
		},
		{
			new: Interval{math.MinInt64, 2},
			exp: Intervals{{math.MinInt64, 2}},
		},
		{
			exist: Intervals{{math.MinInt64, 2}},
			new:   Interval{9, math.MaxInt64},
			exp:   Intervals{{math.MinInt64, 2}, {9, math.MaxInt64}},
		},
		{
			exist: Intervals{{9, math.MaxInt64}},
			new:   Interval{math.MinInt64, 2},
			exp:   Intervals{{math.MinInt64, 2}, {9, math.MaxInt64}},
		},
		{
			exist: Intervals{{9, math.MaxInt64}},
			new:   Interval{math.MinInt64, 10},
			exp:   Intervals{{math.MinInt64, math.MaxInt64}},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			require.Equal(t, c.exp, c.exist.Add(c.new))
		})
	}
}

// TestMemTombstonesConcurrency to make sure they are safe to access from different goroutines.
func TestMemTombstonesConcurrency(t *testing.T) {
	tomb := NewMemTombstones()
	totalRuns := 100
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		for x := 0; x < totalRuns; x++ {
			tomb.AddInterval(storage.SeriesRef(x), Interval{int64(x), int64(x)})
		}
		wg.Done()
	}()
	go func() {
		for x := 0; x < totalRuns; x++ {
			_, err := tomb.Get(storage.SeriesRef(x))
			require.NoError(t, err)
		}
		wg.Done()
	}()
	wg.Wait()
}
