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

package tsdb

import (
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/tsdb/testutil"
)

func TestWriteAndReadbackTombStones(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}()

	ref := uint64(0)

	stones := newMemTombstones()
	// Generate the tombstones.
	for i := 0; i < 100; i++ {
		ref += uint64(rand.Int31n(10)) + 1
		numRanges := rand.Intn(5) + 1
		dranges := make(Intervals, 0, numRanges)
		mint := rand.Int63n(time.Now().UnixNano())
		for j := 0; j < numRanges; j++ {
			dranges = dranges.add(Interval{mint, mint + rand.Int63n(1000)})
			mint += rand.Int63n(1000) + 1
		}
		stones.addInterval(ref, dranges...)
	}

	_, err := writeTombstoneFile(log.NewNopLogger(), tmpdir, stones)
	testutil.Ok(t, err)

	restr, _, err := readTombstones(tmpdir)
	testutil.Ok(t, err)

	// Compare the two readers.
	testutil.Equals(t, stones, restr)
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
			new:   Interval{21, 23},
			exp:   Intervals{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: Intervals{{1, 2}, {3, 5}, {7, 7}},
			new:   Interval{6, 7},
			exp:   Intervals{{1, 2}, {3, 7}},
		},
		{
			exist: Intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   Interval{21, 25},
			exp:   Intervals{{1, 10}, {12, 30}},
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
	}

	for _, c := range cases {

		testutil.Equals(t, c.exp, c.exist.add(c.new))
	}
}

// TestMemTombstonesConcurrency to make sure they are safe to access from different goroutines.
func TestMemTombstonesConcurrency(t *testing.T) {
	tomb := newMemTombstones()
	totalRuns := 100
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		for x := 0; x < totalRuns; x++ {
			tomb.addInterval(uint64(x), Interval{int64(x), int64(x)})
		}
		wg.Done()
	}()
	go func() {
		for x := 0; x < totalRuns; x++ {
			_, err := tomb.Get(uint64(x))
			testutil.Ok(t, err)
		}
		wg.Done()
	}()
	wg.Wait()
}
