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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteAndReadbackTombStones(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	ref := uint32(0)

	stones := make(map[uint32]intervals)
	// Generate the tombstones.
	for i := 0; i < 100; i++ {
		ref += uint32(rand.Int31n(10)) + 1
		numRanges := rand.Intn(5) + 1
		dranges := make(intervals, 0, numRanges)
		mint := rand.Int63n(time.Now().UnixNano())
		for j := 0; j < numRanges; j++ {
			dranges = dranges.add(interval{mint, mint + rand.Int63n(1000)})
			mint += rand.Int63n(1000) + 1
		}
		stones[ref] = dranges
	}

	require.NoError(t, writeTombstoneFile(tmpdir, newTombstoneReader(stones)))

	restr, err := readTombstones(tmpdir)
	require.NoError(t, err)
	exptr := newTombstoneReader(stones)
	// Compare the two readers.
	require.Equal(t, exptr, restr)
}

func TestAddingNewIntervals(t *testing.T) {
	cases := []struct {
		exist intervals
		new   interval

		exp intervals
	}{
		{
			new: interval{1, 2},
			exp: intervals{{1, 2}},
		},
		{
			exist: intervals{{1, 2}},
			new:   interval{1, 2},
			exp:   intervals{{1, 2}},
		},
		{
			exist: intervals{{1, 4}, {6, 6}},
			new:   interval{5, 6},
			exp:   intervals{{1, 6}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{21, 23},
			exp:   intervals{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: intervals{{1, 2}, {3, 5}, {7, 7}},
			new:   interval{6, 7},
			exp:   intervals{{1, 2}, {3, 7}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{21, 25},
			exp:   intervals{{1, 10}, {12, 30}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{18, 23},
			exp:   intervals{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{9, 23},
			exp:   intervals{{1, 23}, {25, 30}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{9, 230},
			exp:   intervals{{1, 230}},
		},
		{
			exist: intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   interval{1, 4},
			exp:   intervals{{1, 10}, {12, 20}, {25, 30}},
		},
		{
			exist: intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   interval{11, 14},
			exp:   intervals{{5, 20}, {25, 30}},
		},
	}

	for _, c := range cases {

		require.Equal(t, c.exp, c.exist.add(c.new))
	}
	return
}
