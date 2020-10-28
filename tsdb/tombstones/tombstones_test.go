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
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestWriteAndReadbackTombstones(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

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
		stones.AddInterval(ref, dranges...)
	}

	_, err := WriteFile(log.NewNopLogger(), tmpdir, stones)
	require.NoError(t, err)

	restr, _, err := ReadTombstones(tmpdir)
	require.NoError(t, err)

	// Compare the two readers.
	require.Equal(t, stones, restr)
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
			tomb.AddInterval(uint64(x), Interval{int64(x), int64(x)})
		}
		wg.Done()
	}()
	go func() {
		for x := 0; x < totalRuns; x++ {
			_, err := tomb.Get(uint64(x))
			require.NoError(t, err)
		}
		wg.Done()
	}()
	wg.Wait()
}
